package cli

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/mgeovany/sentra/cli/internal/auth"
	"github.com/mgeovany/sentra/cli/internal/commit"
	"github.com/mgeovany/sentra/cli/internal/storage"
	"github.com/minio/minio-go/v7"
)

type missingCommitFile struct {
	Path string
	Abs  string
}

type missingCommitFilesError struct {
	CommitID  string
	Message   string
	ScanRoot  string
	Missing   []missingCommitFile
	CauseHint error
}

func (e missingCommitFilesError) Error() string {
	id := strings.TrimSpace(e.CommitID)
	msg := strings.TrimSpace(e.Message)
	scanRoot := strings.TrimSpace(e.ScanRoot)

	out := strings.Builder{}
	out.WriteString("push failed: missing env file(s) referenced by a local commit\n")
	if id != "" {
		out.WriteString("- Commit: ")
		out.WriteString(id)
		out.WriteString("\n")
	}
	if msg != "" {
		out.WriteString("- Message: ")
		out.WriteString(msg)
		out.WriteString("\n")
	}
	if scanRoot != "" {
		out.WriteString("- Scan root: ")
		out.WriteString(scanRoot)
		out.WriteString("\n")
	}
	out.WriteString("- Missing:\n")
	for _, m := range e.Missing {
		p := strings.TrimSpace(m.Path)
		a := strings.TrimSpace(m.Abs)
		if p == "" {
			continue
		}
		out.WriteString("  - ")
		out.WriteString(p)
		if a != "" {
			out.WriteString(" (expected at ")
			out.WriteString(a)
			out.WriteString(")")
		}
		out.WriteString("\n")
	}
	out.WriteString("Fix:\n")
	out.WriteString("  1) Restore the missing file(s) and re-run: sentra push\n")
	if id != "" {
		out.WriteString("  2) Or drop the broken commit: sentra log prune ")
		out.WriteString(id)
		out.WriteString(" (or sentra log rm ")
		out.WriteString(id)
		out.WriteString(")\n")
	}
	if e.CauseHint != nil {
		out.WriteString("\nDetails: ")
		out.WriteString(strings.TrimSpace(e.CauseHint.Error()))
		out.WriteString("\n")
	}
	return strings.TrimSpace(out.String())
}

func buildPushRequestV1(ctx context.Context, scanRoot, machineID, machineName string, vaultKey []byte, c commit.Commit, s3cfg storage.S3Config, s3 *minio.Client, byos bool, userID string) ([]pushRequestV1, error) {
	pathsByRoot := map[string][]string{}
	for p := range c.Files {
		root := projectRootFromPath(p)
		if root == "" {
			continue
		}
		pathsByRoot[root] = append(pathsByRoot[root], p)
	}
	if len(pathsByRoot) == 0 {
		return nil, fmt.Errorf("cannot determine project root")
	}

	roots := make([]string, 0, len(pathsByRoot))
	for root := range pathsByRoot {
		roots = append(roots, root)
	}
	sort.Strings(roots)

	clientID := strings.TrimSpace(c.ID)
	if _, err := uuid.Parse(clientID); err != nil {
		// Fallback: convert legacy timestamp-based IDs to UUIDs.
		// Commits should be migrated on load, but this provides safety for edge cases.
		clientID = uuid.NewSHA1(uuid.NameSpaceOID, []byte(clientID)).String()
	}

	out := make([]pushRequestV1, 0, len(roots))
	var missing []missingCommitFile
	for _, root := range roots {
		paths := pathsByRoot[root]
		sort.Strings(paths)

		files := make([]pushFileV1, 0, len(paths))
		for _, p := range paths {
			abs := filepath.Join(scanRoot, filepath.FromSlash(p))
			plain, err := os.ReadFile(abs)
			if err != nil {
				if os.IsNotExist(err) {
					missing = append(missing, missingCommitFile{Path: p, Abs: abs})
					continue
				}
				return nil, fmt.Errorf("cannot read %s: %w", p, err)
			}

			shaPlain := auth.SHA256Hex(plain)
			cipherName, blobB64, size, err := auth.EncryptEnvBlobWithKey(vaultKey, plain)
			if err != nil {
				return nil, err
			}

			var blob string
			var st *pushStorageV1
			if byos {
				if s3 == nil {
					return nil, fmt.Errorf("storage client not available")
				}
				raw, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(blobB64))
				if err != nil {
					return nil, err
				}
				key := s3ObjectKey(userID, root, p, shaPlain)
				if err := storage.PutObject(ctx, s3, s3cfg, key, raw); err != nil {
					return nil, fmt.Errorf("s3 upload failed (%s): %w", p, err)
				}
				st = &pushStorageV1{
					Provider: "s3",
					Bucket:   s3cfg.Bucket,
					Key:      key,
					Endpoint: s3cfg.Endpoint,
					Region:   s3cfg.Region,
				}
			} else {
				blob = blobB64
			}

			files = append(files, pushFileV1{
				Path:      p,
				SHA256:    shaPlain,
				Size:      size,
				Encrypted: true,
				Cipher:    cipherName,
				Blob:      blob,
				Storage:   st,
			})
		}

		out = append(out, pushRequestV1{
			V:       1,
			Project: pushProjectV1{Root: strings.TrimSpace(root)},
			Machine: pushMachineV1{ID: machineID, Name: machineName},
			Commit: pushCommitV1{
				ClientID: clientID,
				Message:  strings.TrimSpace(c.Message),
			},
			Files: files,
		})
	}

	if len(missing) > 0 {
		return nil, missingCommitFilesError{
			CommitID:  strings.TrimSpace(c.ID),
			Message:   strings.TrimSpace(c.Message),
			ScanRoot:  scanRoot,
			Missing:   missing,
			CauseHint: errors.New("a tracked env file was deleted or moved after creating the commit"),
		}
	}

	return out, nil
}

func s3ObjectKey(userID string, root string, path string, shaPlain string) string {
	userID = strings.TrimSpace(userID)
	root = strings.TrimSpace(root)
	path = strings.TrimSpace(path)
	shaPlain = strings.TrimSpace(shaPlain)

	h := sha256.Sum256([]byte(path))
	pathHash := hex.EncodeToString(h[:])

	// Stable + safe key; content remains encrypted client-side.
	return "sentra/v1/" + userID + "/" + root + "/" + shaPlain + "/" + pathHash + ".bin"
}

func projectRootFromPath(p string) string {
	p = strings.TrimSpace(p)
	p = strings.TrimPrefix(p, "./")
	p = strings.TrimPrefix(p, "/")
	if p == "" {
		return ""
	}
	parts := strings.Split(p, "/")
	if len(parts) == 0 {
		return ""
	}
	return strings.TrimSpace(parts[0])
}
