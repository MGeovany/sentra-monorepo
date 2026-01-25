package repo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/mgeovany/sentra/server/internal/supabase"
)

var (
	ErrDBNotConfigured = errors.New("db not configured")
	ErrTooManyMachines = errors.New("too many machines")
)

type MachineStore interface {
	Register(ctx context.Context, userID, machineID, machineName, devicePubKey string) error
	DevicePubKey(ctx context.Context, userID, machineID string) (string, bool, error)
}

type DisabledMachineStore struct{}

func (DisabledMachineStore) Register(ctx context.Context, userID, machineID, machineName, devicePubKey string) error {
	return ErrDBNotConfigured
}

func (DisabledMachineStore) DevicePubKey(ctx context.Context, userID, machineID string) (string, bool, error) {
	return "", false, ErrDBNotConfigured
}

type SupabaseMachineStore struct {
	client *supabase.Client
	table  string
}

func NewSupabaseMachineStore(client *supabase.Client, table string) SupabaseMachineStore {
	if table == "" {
		table = "machines"
	}
	return SupabaseMachineStore{client: client, table: table}
}

func (s SupabaseMachineStore) Register(ctx context.Context, userID, machineID, machineName, devicePubKey string) error {
	if s.client == nil {
		return ErrDBNotConfigured
	}
	userID = strings.TrimSpace(userID)
	machineID = strings.TrimSpace(machineID)
	machineName = strings.TrimSpace(machineName)
	if userID == "" || machineID == "" || machineName == "" {
		return fmt.Errorf("invalid machine registration payload")
	}

	u, err := url.Parse(s.client.PostgRESTURL(s.table))
	if err != nil {
		return err
	}
	q := u.Query()
	q.Set("on_conflict", "user_id,machine_id")
	u.RawQuery = q.Encode()

	payload := map[string]any{
		"user_id":         userID,
		"machine_id":      machineID,
		"machine_name":    machineName,
		"device_pub_key":  strings.TrimSpace(devicePubKey),
		"device_key_type": "ed25519",
	}

	headers := map[string]string{
		"Prefer": "resolution=merge-duplicates,return=minimal",
	}

	resp, body, err := s.client.PostJSON(ctx, u.String(), payload, headers)
	if err != nil {
		return err
	}
	// PostgREST can return 200/201/204 for successful upserts depending on
	// configuration and Prefer headers.
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}

	// Map known DB-enforced limits to typed errors.
	var apiErr struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &apiErr); err == nil {
		if strings.Contains(strings.ToLower(strings.TrimSpace(apiErr.Message)), "too many machines") {
			return ErrTooManyMachines
		}
		if strings.TrimSpace(apiErr.Code) == "P0001" && strings.Contains(strings.ToLower(apiErr.Message), "too many machines") {
			return ErrTooManyMachines
		}
	}

	return fmt.Errorf("supabase upsert machines failed: status=%d body=%s", resp.StatusCode, string(body))
}

func (s SupabaseMachineStore) DevicePubKey(ctx context.Context, userID, machineID string) (string, bool, error) {
	if s.client == nil {
		return "", false, ErrDBNotConfigured
	}
	userID = strings.TrimSpace(userID)
	machineID = strings.TrimSpace(machineID)
	if userID == "" || machineID == "" {
		return "", false, fmt.Errorf("invalid machine lookup payload")
	}

	// Query PostgREST:
	// /rest/v1/machines?user_id=eq.<>&machine_id=eq.<> &select=device_pub_key
	u, err := url.Parse(s.client.PostgRESTURL(s.table))
	if err != nil {
		return "", false, err
	}
	q := u.Query()
	q.Set("user_id", "eq."+userID)
	q.Set("machine_id", "eq."+machineID)
	q.Set("select", "device_pub_key")
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", false, err
	}
	// Reuse Supabase auth headers.
	req.Header.Set("Accept", "application/json")
	req.Header.Set("apikey", s.client.APIKey())
	req.Header.Set("Authorization", "Bearer "+s.client.APIKey())

	resp, err := s.client.Do(req)
	if err != nil {
		return "", false, err
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", false, fmt.Errorf("supabase select machines failed: status=%d body=%s", resp.StatusCode, string(b))
	}

	var out []struct {
		DevicePubKey string `json:"device_pub_key"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return "", false, err
	}
	if len(out) == 0 {
		return "", false, nil
	}
	pk := strings.TrimSpace(out[0].DevicePubKey)
	if pk == "" {
		return "", false, nil
	}
	return pk, true, nil
}
