package repo

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/mgeovany/sentra/server/internal/supabase"
)

type ExportFile struct {
	CommitID string `json:"commit_id"`
	FilePath string `json:"file_path"`
	SHA256   string `json:"sha256"`
	Size     int    `json:"size"`
	Cipher   string `json:"cipher"`
	BlobB64  string `json:"blob_b64"`
}

type ExportStore interface {
	Export(ctx context.Context, userID string, root string, at string) ([]ExportFile, error)
}

type DisabledExportStore struct{}

func (DisabledExportStore) Export(ctx context.Context, userID string, root string, at string) ([]ExportFile, error) {
	return nil, ErrDBNotConfigured
}

type SupabaseExportStore struct {
	client *supabase.Client
	fn     string
}

func NewSupabaseExportStore(client *supabase.Client, fn string) SupabaseExportStore {
	if fn == "" {
		fn = "sentra_export_v1"
	}
	return SupabaseExportStore{client: client, fn: fn}
}

func (s SupabaseExportStore) Export(ctx context.Context, userID string, root string, at string) ([]ExportFile, error) {
	if s.client == nil {
		return nil, ErrDBNotConfigured
	}
	userID = strings.TrimSpace(userID)
	root = strings.TrimSpace(root)
	at = strings.TrimSpace(at)
	if userID == "" || root == "" {
		return nil, fmt.Errorf("invalid export request")
	}

	url := s.client.RPCURL(s.fn)
	body := map[string]any{
		"p_user_id": userID,
		"p_root":    root,
		"p_at":      at,
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
		return nil, fmt.Errorf("supabase rpc export failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var out []ExportFile
	if err := supabase.UnmarshalJSON(respBody, &out); err != nil {
		return nil, err
	}
	return out, nil
}

var _ = http.MethodPost
