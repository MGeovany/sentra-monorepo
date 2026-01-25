package commit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Commit struct {
	ID        string            `json:"id"`
	CreatedAt string            `json:"createdAt"`
	Message   string            `json:"message"`
	Files     map[string]string `json:"files"`
	PushedAt  string            `json:"pushedAt,omitempty"`
	Version   int               `json:"version"`
}

func Dir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, ".sentra", "commits"), nil
}

func New(message string, files map[string]string) Commit {
	now := time.Now().UTC()
	id := uuid.NewString()

	copyFiles := make(map[string]string, len(files))
	for k, v := range files {
		copyFiles[k] = v
	}

	return Commit{
		ID:        id,
		CreatedAt: now.Format(time.RFC3339),
		Message:   strings.TrimSpace(message),
		Files:     copyFiles,
		Version:   1,
	}
}

func Save(c Commit) (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}

	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return "", err
	}

	filePath := filepath.Join(dir, c.ID+".json")
	tmpPath := filePath + ".tmp"
	if err := os.WriteFile(tmpPath, b, 0o644); err != nil {
		return "", err
	}
	if err := os.Rename(tmpPath, filePath); err != nil {
		_ = os.Remove(tmpPath)
		return "", err
	}

	return filePath, nil
}

func List() ([]Commit, error) {
	dir, err := Dir()
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var commits []Commit
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}

		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		var c Commit
		if err := json.Unmarshal(b, &c); err != nil {
			return nil, err
		}
		if c.Version == 0 {
			c.Version = 1
		}
		// Migrate legacy timestamp-based IDs to UUIDs (one-time migration).
		if _, err := uuid.Parse(c.ID); err != nil {
			oldID := c.ID
			c.ID = uuid.NewSHA1(uuid.NameSpaceOID, []byte(c.ID)).String()
			// Save migrated commit with new UUID-based ID.
			_, _ = Save(c)
			// Remove old file if it exists and has different name.
			oldPath := filepath.Join(dir, oldID+".json")
			if oldPath != filepath.Join(dir, c.ID+".json") {
				_ = os.Remove(oldPath)
			}
		}
		commits = append(commits, c)
	}

	sort.Slice(commits, func(i, j int) bool { return commits[i].ID < commits[j].ID })
	return commits, nil
}

func Update(c Commit) error {
	_, err := Save(c)
	return err
}
