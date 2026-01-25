package repo

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/mgeovany/sentra/server/internal/supabase"
)

type IdempotencyStatus string

const (
	IdempotencyInProgress IdempotencyStatus = "in_progress"
	IdempotencyDone       IdempotencyStatus = "done"
)

type IdempotencyRecord struct {
	Status       IdempotencyStatus
	ResponseJSON json.RawMessage
}

type IdempotencyStore interface {
	Create(ctx context.Context, userID, scope, key string, ttl time.Duration) (created bool, err error)
	Get(ctx context.Context, userID, scope, key string) (IdempotencyRecord, bool, error)
	SetDone(ctx context.Context, userID, scope, key string, response any) error
	Delete(ctx context.Context, userID, scope, key string) error
}

type DisabledIdempotencyStore struct{}

func (DisabledIdempotencyStore) Create(ctx context.Context, userID, scope, key string, ttl time.Duration) (bool, error) {
	return false, ErrDBNotConfigured
}

func (DisabledIdempotencyStore) Get(ctx context.Context, userID, scope, key string) (IdempotencyRecord, bool, error) {
	return IdempotencyRecord{}, false, ErrDBNotConfigured
}

func (DisabledIdempotencyStore) SetDone(ctx context.Context, userID, scope, key string, response any) error {
	return ErrDBNotConfigured
}

func (DisabledIdempotencyStore) Delete(ctx context.Context, userID, scope, key string) error {
	return ErrDBNotConfigured
}

type SupabaseIdempotencyStore struct {
	client *supabase.Client
	table  string
}

func NewSupabaseIdempotencyStore(client *supabase.Client, table string) SupabaseIdempotencyStore {
	if table == "" {
		table = "idempotency_keys"
	}
	return SupabaseIdempotencyStore{client: client, table: table}
}

func (s SupabaseIdempotencyStore) Create(ctx context.Context, userID, scope, key string, ttl time.Duration) (bool, error) {
	if s.client == nil {
		return false, ErrDBNotConfigured
	}
	userID = strings.TrimSpace(userID)
	scope = strings.TrimSpace(scope)
	key = strings.TrimSpace(key)
	if userID == "" || scope == "" || key == "" {
		return false, fmt.Errorf("invalid idempotency create payload")
	}
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}

	// Let Postgres compute timestamps, but pass expires_at so rows can be cleaned later.
	expiresAt := time.Now().UTC().Add(ttl).Format(time.RFC3339)
	payload := map[string]any{
		"user_id":    userID,
		"scope":      scope,
		"idem_key":   key,
		"status":     string(IdempotencyInProgress),
		"expires_at": expiresAt,
	}

	resp, body, err := s.client.PostJSON(ctx, s.client.PostgRESTURL(s.table), payload, map[string]string{
		"Prefer": "return=minimal",
	})
	if err != nil {
		return false, err
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return true, nil
	}
	if resp.StatusCode == http.StatusConflict {
		return false, nil
	}
	return false, fmt.Errorf("supabase insert idempotency failed: status=%d body=%s", resp.StatusCode, string(body))
}

func (s SupabaseIdempotencyStore) Get(ctx context.Context, userID, scope, key string) (IdempotencyRecord, bool, error) {
	if s.client == nil {
		return IdempotencyRecord{}, false, ErrDBNotConfigured
	}
	userID = strings.TrimSpace(userID)
	scope = strings.TrimSpace(scope)
	key = strings.TrimSpace(key)
	if userID == "" || scope == "" || key == "" {
		return IdempotencyRecord{}, false, fmt.Errorf("invalid idempotency get payload")
	}

	u, err := url.Parse(s.client.PostgRESTURL(s.table))
	if err != nil {
		return IdempotencyRecord{}, false, err
	}
	q := u.Query()
	q.Set("user_id", "eq."+userID)
	q.Set("scope", "eq."+scope)
	q.Set("idem_key", "eq."+key)
	q.Set("select", "status,response_json")
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return IdempotencyRecord{}, false, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("apikey", s.client.APIKey())
	req.Header.Set("Authorization", "Bearer "+s.client.APIKey())

	resp, err := s.client.Do(req)
	if err != nil {
		return IdempotencyRecord{}, false, err
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return IdempotencyRecord{}, false, fmt.Errorf("supabase select idempotency failed: status=%d body=%s", resp.StatusCode, string(b))
	}

	var out []struct {
		Status       string          `json:"status"`
		ResponseJSON json.RawMessage `json:"response_json"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return IdempotencyRecord{}, false, err
	}
	if len(out) == 0 {
		return IdempotencyRecord{}, false, nil
	}

	st := IdempotencyStatus(strings.TrimSpace(out[0].Status))
	return IdempotencyRecord{Status: st, ResponseJSON: out[0].ResponseJSON}, true, nil
}

func (s SupabaseIdempotencyStore) SetDone(ctx context.Context, userID, scope, key string, response any) error {
	if s.client == nil {
		return ErrDBNotConfigured
	}
	userID = strings.TrimSpace(userID)
	scope = strings.TrimSpace(scope)
	key = strings.TrimSpace(key)
	if userID == "" || scope == "" || key == "" {
		return fmt.Errorf("invalid idempotency update payload")
	}

	respJSON, err := json.Marshal(response)
	if err != nil {
		return err
	}

	u, err := url.Parse(s.client.PostgRESTURL(s.table))
	if err != nil {
		return err
	}
	q := u.Query()
	q.Set("user_id", "eq."+userID)
	q.Set("scope", "eq."+scope)
	q.Set("idem_key", "eq."+key)
	u.RawQuery = q.Encode()

	patch := map[string]any{
		"status":        string(IdempotencyDone),
		"response_json": json.RawMessage(respJSON),
	}
	b, err := json.Marshal(patch)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, u.String(), bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Prefer", "return=minimal")
	req.Header.Set("apikey", s.client.APIKey())
	req.Header.Set("Authorization", "Bearer "+s.client.APIKey())

	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("supabase patch idempotency failed: status=%d body=%s", resp.StatusCode, string(respBody))
	}
	return nil
}

func (s SupabaseIdempotencyStore) Delete(ctx context.Context, userID, scope, key string) error {
	if s.client == nil {
		return ErrDBNotConfigured
	}
	userID = strings.TrimSpace(userID)
	scope = strings.TrimSpace(scope)
	key = strings.TrimSpace(key)
	if userID == "" || scope == "" || key == "" {
		return fmt.Errorf("invalid idempotency delete payload")
	}

	u, err := url.Parse(s.client.PostgRESTURL(s.table))
	if err != nil {
		return err
	}
	q := u.Query()
	q.Set("user_id", "eq."+userID)
	q.Set("scope", "eq."+scope)
	q.Set("idem_key", "eq."+key)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, u.String(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Prefer", "return=minimal")
	req.Header.Set("apikey", s.client.APIKey())
	req.Header.Set("Authorization", "Bearer "+s.client.APIKey())

	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// If row is missing, treat as success.
		if resp.StatusCode == http.StatusNotFound {
			return nil
		}
		return fmt.Errorf("supabase delete idempotency failed: status=%d body=%s", resp.StatusCode, string(b))
	}
	return nil
}

var _ IdempotencyStore = SupabaseIdempotencyStore{}
