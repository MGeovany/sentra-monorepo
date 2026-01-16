package auth

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/zalando/go-keyring"
)

const (
	keyringService = "sentra"
	keyringUser    = "session-key"
)

func getOrCreateSessionKey() ([]byte, error) {
	// 1) Prefer OS keyring.
	if k, err := getOrCreateSessionKeyKeyring(); err == nil && len(k) == 32 {
		return k, nil
	}

	// 2) Fallback: local key file (0600). This still protects against leaking the
	// session.json file alone (e.g., partial backups/sync), but not against a
	// same-user compromise.
	return getOrCreateSessionKeyFile()
}

func getOrCreateSessionKeyKeyring() ([]byte, error) {
	v, err := keyring.Get(keyringService, keyringUser)
	if err == nil && strings.TrimSpace(v) != "" {
		b, decErr := base64.RawURLEncoding.DecodeString(strings.TrimSpace(v))
		if decErr != nil {
			return nil, decErr
		}
		if len(b) != 32 {
			return nil, fmt.Errorf("invalid keyring key length")
		}
		return b, nil
	}

	// Not found or backend unavailable.
	if err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return nil, err
	}

	k := make([]byte, 32)
	if _, err := rand.Read(k); err != nil {
		return nil, err
	}
	enc := base64.RawURLEncoding.EncodeToString(k)
	if err := keyring.Set(keyringService, keyringUser, enc); err != nil {
		return nil, err
	}
	return k, nil
}

func sessionKeyPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".sentra", "session.key"), nil
}

func getOrCreateSessionKeyFile() ([]byte, error) {
	p, err := sessionKeyPath()
	if err != nil {
		return nil, err
	}
	if b, err := os.ReadFile(p); err == nil {
		s := strings.TrimSpace(string(b))
		raw, decErr := base64.RawURLEncoding.DecodeString(s)
		if decErr != nil {
			return nil, decErr
		}
		if len(raw) != 32 {
			return nil, fmt.Errorf("invalid session.key length")
		}
		return raw, nil
	}

	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return nil, err
	}

	k := make([]byte, 32)
	if _, err := rand.Read(k); err != nil {
		return nil, err
	}
	enc := base64.RawURLEncoding.EncodeToString(k)
	if err := os.WriteFile(p, []byte(enc), 0o600); err != nil {
		return nil, err
	}
	return k, nil
}
