package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/zalando/go-keyring"
)

const (
	keyringSessionUser = "session"
)

func allowInsecureSessionFile() bool {
	v := strings.TrimSpace(os.Getenv("SENTRA_ALLOW_INSECURE_SESSION_FILE"))
	if v == "" {
		return false
	}
	switch strings.ToLower(v) {
	case "1", "true", "yes", "y":
		return true
	default:
		return false
	}
}

func loadSessionKeyring() (Session, bool, error) {
	v, err := keyring.Get(keyringService, keyringSessionUser)
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return Session{}, false, nil
		}
		return Session{}, false, err
	}
	if strings.TrimSpace(v) == "" {
		return Session{}, false, nil
	}

	var s Session
	if err := json.Unmarshal([]byte(v), &s); err != nil {
		// Corrupt entry: treat as missing (caller will ask user to login).
		return Session{}, false, fmt.Errorf("invalid session in keychain: %w", err)
	}
	if s.AccessToken == "" {
		return Session{}, false, nil
	}
	return s, true, nil
}

func saveSessionKeyring(s Session, preserveSavedAt bool) error {
	// SavedAt is used as an expiry fallback when the access token has no usable exp.
	if !preserveSavedAt || s.SavedAt.IsZero() {
		s.SavedAt = time.Now().UTC()
	}

	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return keyring.Set(keyringService, keyringSessionUser, string(b))
}
