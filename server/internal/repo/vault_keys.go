package repo

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/mgeovany/sentra/server/internal/supabase"
)

type VaultKeyStore interface {
	Get(ctx context.Context, userID string) ([]byte, bool, error)
	Upsert(ctx context.Context, userID string, doc []byte) error
}

type DisabledVaultKeyStore struct{}

func (DisabledVaultKeyStore) Get(ctx context.Context, userID string) ([]byte, bool, error) {
	return nil, false, ErrDBNotConfigured
}

func (DisabledVaultKeyStore) Upsert(ctx context.Context, userID string, doc []byte) error {
	return ErrDBNotConfigured
}

type SupabaseVaultKeyStore struct {
	client *supabase.Client
	table  string
}

func NewSupabaseVaultKeyStore(client *supabase.Client, table string) SupabaseVaultKeyStore {
	if table == "" {
		table = "vault_keys"
	}
	return SupabaseVaultKeyStore{client: client, table: table}
}

func (s SupabaseVaultKeyStore) Get(ctx context.Context, userID string) ([]byte, bool, error) {
	if s.client == nil {
		return nil, false, ErrDBNotConfigured
	}
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, false, fmt.Errorf("invalid vault key get request")
	}

	u, err := url.Parse(s.client.PostgRESTURL(s.table))
	if err != nil {
		return nil, false, err
	}
	q := u.Query()
	q.Set("user_id", "eq."+userID)
	q.Set("select", "doc")
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("apikey", s.client.APIKey())
	req.Header.Set("Authorization", "Bearer "+s.client.APIKey())

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			return nil, false, ErrDBMisconfigured
		}
		return nil, false, fmt.Errorf("supabase select vault_keys failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(b)))
	}

	var out []struct {
		Doc any `json:"doc"`
	}
	if err := supabase.UnmarshalJSON(b, &out); err != nil {
		return nil, false, err
	}
	if len(out) == 0 {
		return nil, false, nil
	}

	// Re-marshal doc to preserve exact JSON for the client.
	docBytes, err := supabase.MarshalJSON(out[0].Doc)
	if err != nil {
		return nil, false, err
	}
	if len(docBytes) == 0 {
		return nil, false, nil
	}
	return docBytes, true, nil
}

func (s SupabaseVaultKeyStore) Upsert(ctx context.Context, userID string, doc []byte) error {
	if s.client == nil {
		return ErrDBNotConfigured
	}
	userID = strings.TrimSpace(userID)
	if userID == "" || len(doc) == 0 {
		return fmt.Errorf("invalid vault key upsert payload")
	}

	u, err := url.Parse(s.client.PostgRESTURL(s.table))
	if err != nil {
		return err
	}
	q := u.Query()
	q.Set("on_conflict", "user_id")
	u.RawQuery = q.Encode()

	payload := map[string]any{
		"user_id": userID,
		"doc":     supabase.RawJSON(doc),
	}
	headers := map[string]string{
		"Prefer": "resolution=merge-duplicates,return=minimal",
	}

	resp, body, err := s.client.PostJSON(ctx, u.String(), payload, headers)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return ErrDBMisconfigured
	}
	return fmt.Errorf("supabase upsert vault_keys failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
}
