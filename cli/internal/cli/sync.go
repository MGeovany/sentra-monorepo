package cli

import (
	"context"
	"encoding/base64"
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
	"github.com/mgeovany/sentra/cli/internal/storage"
)

// sentra sync
// Downloads latest env files from remote and writes them into local repos under scan root.
func runSync(args []string) error {
	if len(args) != 0 {
		return errors.New("usage: sentra sync")
	}

	verbosef("Starting sync operation...")
	sess, err := ensureRemoteSession()
	if err != nil {
		return err
	}
	if strings.TrimSpace(sess.AccessToken) == "" {
		return errors.New("not logged in (run: sentra login)")
	}
	verbosef("Session loaded: user authenticated")

	scanRoot, err := resolveScanRoot()
	if err != nil {
		return err
	}
	verbosef("Scan root: %s", scanRoot)

	serverURL, err := serverURLFromEnv()
	if err != nil {
		return err
	}
	verbosef("Server URL: %s", serverURL)

	sp := startSpinner("Fetching projects from remote...")
	projects, err := fetchRemoteProjects(serverURL, sess.AccessToken)
	if err != nil {
		sp.StopInfo("")
		return err
	}
	if len(projects) == 0 {
		sp.StopSuccess("✔ 0 projects")
		fmt.Println("✔ 0 projects")
		verbosef("No projects found on remote")
		return nil
	}
	sp.StopSuccess(fmt.Sprintf("✔ %d project(s) found", len(projects)))
	verbosef("Found %d remote project(s)", len(projects))

	// Stable order.
	sort.Slice(projects, func(i, j int) bool {
		return strings.TrimSpace(projects[i].RootPath) < strings.TrimSpace(projects[j].RootPath)
	})

	written := 0
	scanned := 0
	skippedMissing := 0
	sp2 := startSpinner("Syncing projects...")
	for i, p := range projects {
		root := strings.TrimSpace(p.RootPath)
		if root == "" {
			continue
		}
		sp2.Set(fmt.Sprintf("Syncing %s (%d/%d)...", root, i+1, len(projects)))
		localRepo := filepath.Join(scanRoot, filepath.FromSlash(root))
		verbosef("Checking local repo: %s", localRepo)
		if !isDir(localRepo) {
			verbosef("Skipping %s: local directory not found", root)
			skippedMissing++
			continue
		}

		verbosef("Fetching files for project: %s", root)
		files, err := fetchRemoteExport(serverURL, sess.AccessToken, root)
		if err != nil {
			sp2.StopInfo("")
			return err
		}
		if len(files) == 0 {
			verbosef("No files found for project: %s", root)
			continue
		}
		verbosef("Found %d file(s) for project: %s", len(files), root)

		scanned++
		for _, f := range files {
			verbosef("Processing file: %s (size: %d bytes, cipher: %s)", f.Path, f.Size, f.Cipher)
			plain, err := decryptRemoteExportFile(f)
			if err != nil {
				sp2.StopInfo("")
				return err
			}
			verbosef("Decrypted file: %s (%d bytes)", f.Path, len(plain))

			// Server returns full file path (e.g. "root/.env"); write into scanRoot.
			rel := filepath.ToSlash(strings.TrimSpace(f.Path))
			if rel == "" || strings.HasPrefix(rel, "/") || strings.HasPrefix(rel, "\\") {
				return fmt.Errorf("invalid file path received from server")
			}
			rel = strings.TrimPrefix(rel, "./")
			rel = filepath.Clean(rel)
			rel = filepath.ToSlash(rel)
			if rel == "." || rel == "" || strings.HasPrefix(rel, "../") {
				return fmt.Errorf("invalid file path received from server")
			}
			if !strings.HasPrefix(rel, root+"/") {
				return fmt.Errorf("unexpected file path received from server")
			}

			outPath := filepath.Join(scanRoot, filepath.FromSlash(rel))
			verbosef("Writing file to: %s", outPath)
			if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
				sp2.StopInfo("")
				return err
			}
			if err := os.WriteFile(outPath, plain, 0o600); err != nil {
				sp2.StopInfo("")
				return err
			}
			written++
			verbosef("Successfully wrote file: %s", outPath)
		}
	}
	sp2.StopSuccess(fmt.Sprintf("✔ synced %d env file(s) across %d project(s)", written, scanned))
	if skippedMissing > 0 {
		warnf("⚠ %d project(s) missing locally under %s", skippedMissing, scanRoot)
		verbosef("Missing projects were skipped (not found in scan root)")
	}
	verbosef("Sync completed: %d file(s) written, %d project(s) synced, %d skipped", written, scanned, skippedMissing)
	return nil
}

func fetchRemoteProjects(serverURL string, accessToken string) ([]remoteProject, error) {
	endpoint := strings.TrimRight(strings.TrimSpace(serverURL), "/") + "/projects"
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(accessToken))

	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("failed to fetch projects")
	}

	var projects []remoteProject
	if err := json.Unmarshal(body, &projects); err != nil {
		return nil, err
	}
	return projects, nil
}

func fetchRemoteExport(serverURL string, accessToken string, root string) ([]remoteExportFile, error) {
	u, err := url.Parse(strings.TrimRight(strings.TrimSpace(serverURL), "/") + "/export")
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("root", strings.TrimSpace(root))
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(accessToken))

	client := &http.Client{Timeout: 45 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("export failed")
	}

	var files []remoteExportFile
	if err := json.Unmarshal(body, &files); err != nil {
		return nil, err
	}
	sort.Slice(files, func(i, j int) bool { return strings.TrimSpace(files[i].Path) < strings.TrimSpace(files[j].Path) })
	return files, nil
}

func decryptRemoteExportFile(f remoteExportFile) ([]byte, error) {
	cipherName := strings.TrimSpace(f.Cipher)
	blobB64 := strings.TrimSpace(f.BlobB64)
	if blobB64 == "" && strings.TrimSpace(f.StorageKey) != "" {
		s3cfg, _, enabled, err := storage.ResolveS3()
		if err != nil {
			return nil, err
		}
		if !enabled {
			return nil, fmt.Errorf("sync requires storage setup (run: sentra storage setup)")
		}
		if strings.TrimSpace(f.StorageBucket) != "" {
			s3cfg.Bucket = strings.TrimSpace(f.StorageBucket)
		}
		if strings.TrimSpace(f.StorageEndpoint) != "" {
			s3cfg.Endpoint = strings.TrimSpace(f.StorageEndpoint)
		}
		if strings.TrimSpace(f.StorageRegion) != "" {
			s3cfg.Region = strings.TrimSpace(f.StorageRegion)
		}
		s3c, err := storage.NewS3Client(s3cfg)
		if err != nil {
			return nil, fmt.Errorf("failed to connect to storage (%s)", strings.TrimSpace(f.Path))
		}
		raw, err := storage.GetObject(context.Background(), s3c, s3cfg, strings.TrimSpace(f.StorageKey))
		if err != nil {
			return nil, fmt.Errorf("failed to download from storage (%s)", strings.TrimSpace(f.Path))
		}
		blobB64 = base64.RawURLEncoding.EncodeToString(raw)
	}

	plain, err := auth.DecryptEnvBlob(cipherName, blobB64)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt file (%s)", strings.TrimSpace(f.Path))
	}
	return plain, nil
}
