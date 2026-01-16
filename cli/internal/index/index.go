package index

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type Index struct {
	Version   int               `json:"version"`
	ScanRoot  string            `json:"scanRoot"`
	UpdatedAt string            `json:"updatedAt"`
	Staged    map[string]string `json:"staged"`
}

func DefaultPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, ".sentra", "index.json"), nil
}

func Load(filePath string) (Index, bool, error) {
	b, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return Index{}, false, nil
		}
		return Index{}, false, err
	}

	var idx Index
	if err := json.Unmarshal(b, &idx); err != nil {
		return Index{}, false, err
	}

	if idx.Version == 0 {
		idx.Version = 1
	}
	if idx.Staged == nil {
		idx.Staged = map[string]string{}
	}

	return idx, true, nil
}

func Save(filePath string, idx Index) error {
	if idx.Version == 0 {
		idx.Version = 1
	}
	if idx.UpdatedAt == "" {
		idx.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	if idx.Staged == nil {
		idx.Staged = map[string]string{}
	}

	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		return err
	}

	b, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return err
	}

	tmpPath := filePath + ".tmp"
	if err := os.WriteFile(tmpPath, b, 0o644); err != nil {
		return err
	}

	if err := os.Rename(tmpPath, filePath); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}

	return nil
}
