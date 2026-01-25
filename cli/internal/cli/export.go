package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/mgeovany/sentra/cli/internal/auth"
)

type remoteExportFile struct {
	CommitID string `json:"commit_id"`
	Path     string `json:"file_path"`
	SHA256   string `json:"sha256"`
	Size     int    `json:"size"`
	Cipher   string `json:"cipher"`
	BlobB64  string `json:"blob_b64"`
}

func runExport(args []string) error {
	root, at, err := parseExportArgs(args)
	if err != nil {
		return err
	}

	sess, err := ensureRemoteSession()
	if err != nil {
		return err
	}
	if strings.TrimSpace(sess.AccessToken) == "" {
		return errors.New("not logged in (run: sentra login)")
	}

	serverURL, err := serverURLFromEnv()
	if err != nil {
		return err
	}

	u, err := url.Parse(serverURL + "/export")
	if err != nil {
		return err
	}
	q := u.Query()
	q.Set("root", root)
	if at != "" {
		q.Set("at", at)
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, u.String(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(sess.AccessToken))

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("export failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var files []remoteExportFile
	if err := json.Unmarshal(body, &files); err != nil {
		return err
	}
	if len(files) == 0 {
		fmt.Println("✔ 0 files")
		return nil
	}

	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })

	baseDir := filepath.Join("sentra-export", root)
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return err
	}

	written := 0
	for _, f := range files {
		plain, err := auth.DecryptEnvBlob(strings.TrimSpace(f.Cipher), strings.TrimSpace(f.BlobB64))
		if err != nil {
			return fmt.Errorf("cannot decrypt %s: %w", f.Path, err)
		}

		rel := strings.TrimSpace(f.Path)
		rel = strings.TrimPrefix(rel, root+"/")
		rel = filepath.ToSlash(rel)
		rel = strings.TrimPrefix(rel, "/")
		rel = filepath.Clean(rel)
		rel = filepath.ToSlash(rel)
		if rel == "." || rel == "" || strings.HasPrefix(rel, "../") {
			return fmt.Errorf("unsafe file path from server: %q", f.Path)
		}
		if strings.HasPrefix(rel, "/") || strings.HasPrefix(rel, "\\") {
			return fmt.Errorf("unsafe file path from server: %q", f.Path)
		}

		outPath := filepath.Join(baseDir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(outPath, plain, 0o600); err != nil {
			return err
		}
		written++
	}

	fmt.Printf("✔ exported %d files to %s\n", written, baseDir)
	return nil
}

func parseExportArgs(args []string) (root string, at string, err error) {
	if len(args) < 1 {
		return "", "", errors.New("usage: sentra export <project> [--at <commit>]")
	}

	root = projectRootFromPath(args[0])
	root = strings.TrimSpace(root)
	if root == "" {
		return "", "", errors.New("usage: sentra export <project> [--at <commit>]")
	}

	if len(args) == 1 {
		return root, "", nil
	}
	if len(args) != 3 || args[1] != "--at" {
		return "", "", errors.New("usage: sentra export <project> [--at <commit>]")
	}
	at = strings.TrimSpace(args[2])
	if at == "" {
		return "", "", errors.New("usage: sentra export <project> [--at <commit>]")
	}
	return root, at, nil
}
