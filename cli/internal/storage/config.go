package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Provider string

const (
	ProviderAWSS3   Provider = "aws_s3"
	ProviderR2      Provider = "cloudflare_r2"
	ProviderMinIO   Provider = "minio"
	ProviderCustom  Provider = "custom_s3"
	ProviderUnknown Provider = "unknown"
)

type AuthMethod string

const (
	AuthAWSProfile AuthMethod = "aws_profile"
	AuthStatic     AuthMethod = "access_key"
	AuthEnvOnly    AuthMethod = "env"
)

type SecretLocation string

const (
	SecretNone    SecretLocation = "none"
	SecretKeyring SecretLocation = "keyring"
	SecretFile    SecretLocation = "file"
)

type Config struct {
	Version int    `json:"version"`
	ID      string `json:"id"`

	Provider Provider `json:"provider"`
	Bucket   string   `json:"bucket"`
	Region   string   `json:"region,omitempty"`
	Endpoint string   `json:"endpoint,omitempty"`
	UseSSL   bool     `json:"use_ssl"`

	AuthMethod AuthMethod `json:"auth_method"`

	// AuthAWSProfile
	AWSProfile         string `json:"aws_profile,omitempty"`
	AWSCredentialsFile string `json:"aws_credentials_file,omitempty"`

	// AuthStatic
	SecretLocation SecretLocation `json:"secret_location,omitempty"`
	SecretRef      string         `json:"secret_ref,omitempty"`
	AccessKeyID    string         `json:"access_key_id,omitempty"`
	SecretKey      string         `json:"secret_access_key,omitempty"`
	SessionToken   string         `json:"session_token,omitempty"`

	CreatedAt string `json:"created_at"`
}

func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".sentra", "storage.json"), nil
}

func LoadConfig() (Config, bool, error) {
	p, err := DefaultPath()
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
		return Config{}, false, err
	}
	if cfg.Version == 0 {
		cfg.Version = 1
	}
	return cfg, true, nil
}

func SaveConfig(cfg Config) error {
	if cfg.Version == 0 {
		cfg.Version = 1
	}
	if strings.TrimSpace(cfg.ID) == "" {
		cfg.ID = uuid.NewString()
	}
	if strings.TrimSpace(cfg.CreatedAt) == "" {
		cfg.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}

	p, err := DefaultPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}

	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(p, b, 0o600); err != nil {
		return err
	}
	return nil
}

func DeleteConfig() error {
	cfg, ok, err := LoadConfig()
	if err != nil {
		return err
	}
	if ok && cfg.SecretLocation == SecretKeyring && strings.TrimSpace(cfg.SecretRef) != "" {
		_ = deleteSecret(cfg.SecretRef)
	}

	p, err := DefaultPath()
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return nil
}

func (c Config) Validate() error {
	if strings.TrimSpace(c.Bucket) == "" {
		return fmt.Errorf("missing bucket")
	}
	if strings.TrimSpace(c.Endpoint) == "" {
		return fmt.Errorf("missing endpoint")
	}
	switch c.AuthMethod {
	case AuthAWSProfile:
		return nil
	case AuthEnvOnly:
		return nil
	case AuthStatic:
		if strings.TrimSpace(c.AccessKeyID) == "" {
			return fmt.Errorf("missing access key id")
		}
		if c.SecretLocation == SecretKeyring {
			if strings.TrimSpace(c.SecretRef) == "" {
				return fmt.Errorf("missing secret_ref")
			}
			return nil
		}
		if c.SecretLocation == SecretFile {
			if strings.TrimSpace(c.SecretKey) == "" {
				return fmt.Errorf("missing secret key")
			}
			return nil
		}
		return fmt.Errorf("invalid secret_location")
	default:
		return fmt.Errorf("invalid auth_method")
	}
}
