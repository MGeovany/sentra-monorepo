package repo

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/mgeovany/sentra/server/internal/supabase"
)

var (
	ErrDBNotConfigured = errors.New("db not configured")
)

type MachineStore interface {
	Register(ctx context.Context, userID, machineID, machineName string) error
}

type DisabledMachineStore struct{}

func (DisabledMachineStore) Register(ctx context.Context, userID, machineID, machineName string) error {
	return ErrDBNotConfigured
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

func (s SupabaseMachineStore) Register(ctx context.Context, userID, machineID, machineName string) error {
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
		"user_id":      userID,
		"machine_id":   machineID,
		"machine_name": machineName,
	}

	headers := map[string]string{
		"Prefer": "resolution=merge-duplicates,return=minimal",
	}

	resp, body, err := s.client.PostJSON(ctx, u.String(), payload, headers)
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusNoContent {
		return nil
	}

	return fmt.Errorf("supabase upsert machines failed: status=%d body=%s", resp.StatusCode, string(body))
}
