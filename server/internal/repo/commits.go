package repo

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/mgeovany/sentra/server/internal/supabase"
)

type CommitInfo struct {
	CommitID    string   `json:"commit_id"`
	CreatedAt   string   `json:"created_at"`
	Message     string   `json:"message"`
	MachineName string   `json:"machine_name"`
	MachineID   string   `json:"machine_id"`
	Files       []string `json:"files"`

	ProjectID   string `json:"project_id"`
	ProjectRoot string `json:"project_root"`
	ProjectName string `json:"project_name"`
	FileCount   int    `json:"file_count"`
}

type CommitStore interface {
	ListCommits(ctx context.Context, userID string, root string) ([]CommitInfo, error)
}

type DisabledCommitStore struct{}

func (DisabledCommitStore) ListCommits(ctx context.Context, userID string, root string) ([]CommitInfo, error) {
	return nil, ErrDBNotConfigured
}

type SupabaseCommitStore struct {
	client *supabase.Client
	fn     string
}

func NewSupabaseCommitStore(client *supabase.Client, fn string) SupabaseCommitStore {
	if fn == "" {
		fn = "sentra_commits_v1"
	}
	return SupabaseCommitStore{client: client, fn: fn}
}

func (s SupabaseCommitStore) ListCommits(ctx context.Context, userID string, root string) ([]CommitInfo, error) {
	if s.client == nil {
		return nil, ErrDBNotConfigured
	}
	userID = strings.TrimSpace(userID)
	root = strings.TrimSpace(root)
	if userID == "" || root == "" {
		return nil, fmt.Errorf("invalid commits request")
	}

	url := s.client.RPCURL(s.fn)
	body := map[string]any{
		"p_user_id": userID,
		"p_root":    root,
	}
	headers := map[string]string{
		"Accept": "application/json",
		"Prefer": "return=representation",
	}

	resp, respBody, err := s.client.PostJSON(ctx, url, body, headers)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("supabase rpc commits failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var out []CommitInfo
	if err := supabase.UnmarshalJSON(respBody, &out); err != nil {
		return nil, err
	}
	return out, nil
}

var _ = http.MethodPost
