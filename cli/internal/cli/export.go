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

type remoteExportFile struct {
	CommitID        string `json:"commit_id"`
	Path            string `json:"file_path"`
	SHA256          string `json:"sha256"`
	Size            int    `json:"size"`
	Cipher          string `json:"cipher"`
	BlobB64         string `json:"blob_b64"`
	StorageProvider string `json:"storage_provider"`
	StorageBucket   string `json:"storage_bucket"`
	StorageKey      string `json:"storage_key"`
	StorageEndpoint string `json:"storage_endpoint"`
	StorageRegion   string `json:"storage_region"`
}

func runExport(args []string) error {
	verbosef("Starting export operation...")
	root, at, err := parseExportArgs(args)
	if err != nil {
		return err
	}
	verbosef("Project root: %s", root)
	if at != "" {
		verbosef("Exporting at commit: %s", at)
	} else {
		verbosef("Exporting latest commit")
	}

	sess, err := ensureRemoteSession()
	if err != nil {
		return err
	}
	if strings.TrimSpace(sess.AccessToken) == "" {
		return errors.New("not logged in (run: sentra login)")
	}
	verbosef("Session loaded: user authenticated")

	serverURL, err := serverURLFromEnv()
	if err != nil {
		return err
	}
	verbosef("Server URL: %s", serverURL)

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
	verbosef("Export URL: %s", u.String())

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
		return fmt.Errorf("export failed")
	}

	var files []remoteExportFile
	if err := json.Unmarshal(body, &files); err != nil {
		return err
	}
	if len(files) == 0 {
		fmt.Println("✔ 0 files")
		verbosef("No files found to export")
		return nil
	}
	verbosef("Received %d file(s) from server", len(files))

	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })

	baseDir := filepath.Join("sentra-export", root)
	verbosef("Export directory: %s", baseDir)
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return err
	}

	written := 0
	for i, f := range files {
		verbosef("Processing file %d/%d: %s (size: %d bytes, cipher: %s)", i+1, len(files), f.Path, f.Size, f.Cipher)
		cipherName := strings.TrimSpace(f.Cipher)
		blobB64 := strings.TrimSpace(f.BlobB64)
		if blobB64 == "" && strings.TrimSpace(f.StorageKey) != "" {
			verbosef("File stored in BYOS, downloading from storage: %s", f.StorageKey)
			s3cfg, _, enabled, err := storage.ResolveS3()
			if err != nil {
				return err
			}
			if !enabled {
				return fmt.Errorf("export requires storage setup (run: sentra storage setup)")
			}
			// Prefer server-provided location if present.
			if strings.TrimSpace(f.StorageBucket) != "" {
				s3cfg.Bucket = strings.TrimSpace(f.StorageBucket)
				verbosef("Using storage bucket: %s", s3cfg.Bucket)
			}
			if strings.TrimSpace(f.StorageEndpoint) != "" {
				s3cfg.Endpoint = strings.TrimSpace(f.StorageEndpoint)
				verbosef("Using storage endpoint: %s", s3cfg.Endpoint)
			}
			if strings.TrimSpace(f.StorageRegion) != "" {
				s3cfg.Region = strings.TrimSpace(f.StorageRegion)
				verbosef("Using storage region: %s", s3cfg.Region)
			}
			// Recreate S3 client after applying server-provided overrides
			// since MinIO clients are bound to endpoint/region at construction.
			s3c, err := storage.NewS3Client(s3cfg)
			if err != nil {
				return fmt.Errorf("failed to connect to storage (%s)", f.Path)
			}
			verbosef("Downloading from storage: %s", f.StorageKey)
			raw, err := storage.GetObject(context.Background(), s3c, s3cfg, strings.TrimSpace(f.StorageKey))
			if err != nil {
				return fmt.Errorf("failed to download from storage (%s)", f.Path)
			}
			verbosef("Downloaded %d bytes from storage", len(raw))
			blobB64 = base64.RawURLEncoding.EncodeToString(raw)
		}

		verbosef("Decrypting file: %s", f.Path)
		plain, err := auth.DecryptEnvBlob(cipherName, blobB64)
		if err != nil {
			return fmt.Errorf("failed to decrypt file (%s)", f.Path)
		}
		verbosef("Decrypted file: %s (%d bytes)", f.Path, len(plain))

		rel := strings.TrimSpace(f.Path)
		rel = strings.TrimPrefix(rel, root+"/")
		rel = filepath.ToSlash(rel)
		rel = strings.TrimPrefix(rel, "/")
		rel = filepath.Clean(rel)
		rel = filepath.ToSlash(rel)
		if rel == "." || rel == "" || strings.HasPrefix(rel, "../") {
			return fmt.Errorf("invalid file path received from server")
		}
		if strings.HasPrefix(rel, "/") || strings.HasPrefix(rel, "\\") {
			return fmt.Errorf("invalid file path received from server")
		}

		outPath := filepath.Join(baseDir, filepath.FromSlash(rel))
		verbosef("Writing file to: %s", outPath)
		if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(outPath, plain, 0o600); err != nil {
			return err
		}
		written++
		verbosef("Successfully exported file: %s", outPath)
	}

	fmt.Printf("✔ exported %d files to %s\n", written, baseDir)
	verbosef("Export completed: %d file(s) written to %s", written, baseDir)
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
