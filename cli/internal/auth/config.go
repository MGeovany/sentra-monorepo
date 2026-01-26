package auth

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
)

type Config struct {
	MachineID string `json:"machine_id"`
	UserID    string `json:"user_id,omitempty"`
	// ServerURL is the remote API base URL (https://... or http://127.0.0.1:port for local dev).
	ServerURL string `json:"server_url,omitempty"`
	// StorageMode controls whether the CLI uploads encrypted blobs to user-managed
	// object storage (BYOS) or sends blobs inline for the hosted provider.
	// Values: "hosted" (default) | "byos".
	StorageMode string    `json:"storage_mode,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	Version     int       `json:"version"`
}

func configPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".sentra", "config.json"), nil
}

func EnsureConfig() (Config, error) {
	cfg, ok, err := LoadConfig()
	if err != nil {
		return Config{}, err
	}
	if ok {
		return cfg, nil
	}

	cfg = Config{
		MachineID: uuid.NewString(),
		CreatedAt: time.Now().UTC(),
		Version:   1,
	}
	if err := SaveConfig(cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func LoadConfig() (Config, bool, error) {
	p, err := configPath()
	if err != nil {
		return Config{}, false, err
	}

	b, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return Config{}, false, nil
		}
		return Config{}, false, err
	}

	var cfg Config
	if err := json.Unmarshal(b, &cfg); err != nil {
		return Config{}, false, fmt.Errorf("invalid config file: %w", err)
	}
	if cfg.Version == 0 {
		cfg.Version = 1
	}
	if cfg.MachineID == "" {
		return Config{}, false, nil
	}

	return cfg, true, nil
}

func SaveConfig(cfg Config) error {
	p, err := configPath()
	if err != nil {
		return err
	}

	if cfg.Version == 0 {
		cfg.Version = 1
	}

	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}

	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(p, b, 0o600)
}
