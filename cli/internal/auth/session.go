package auth

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type Session struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`

	// Meta
	SavedAt time.Time `json:"saved_at"`
}

func (s Session) ExpiresAt() time.Time {
	// Prefer JWT exp if available.
	if c, err := ParseAccessTokenClaims(s.AccessToken); err == nil && c.Exp > 0 {
		return time.Unix(c.Exp, 0).UTC()
	}

	if !s.SavedAt.IsZero() && s.ExpiresIn > 0 {
		return s.SavedAt.Add(time.Duration(s.ExpiresIn) * time.Second)
	}

	return time.Time{}
}

func (s Session) NeedsRefresh(now time.Time) bool {
	exp := s.ExpiresAt()
	if exp.IsZero() {
		// Unknown expiry; don't force refresh.
		return false
	}

	// Refresh a bit early.
	return now.After(exp.Add(-60 * time.Second))
}

func sessionPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".sentra", "session.json"), nil
}

func SaveSession(s Session) error {
	// Prefer storing the full session in the OS credential store (Keychain/Secret Service/CredMan).
	// This avoids leaving usable tokens on disk.
	if err := saveSessionKeyring(s, false); err == nil {
		// Best-effort cleanup of legacy/on-disk session material.
		_ = removeLegacySessionFiles()
		return nil
	} else if !allowInsecureSessionFile() {
		return fmt.Errorf("secure session store unavailable (keychain/credential manager): %w", err)
	}

	return saveSession(s, false)
}

func saveSession(s Session, preserveSavedAt bool) error {
	p, err := sessionPath()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}

	// SavedAt is used as an expiry fallback when the access token has no usable exp.
	// During migration we preserve an existing SavedAt to avoid widening the refresh window.
	if !preserveSavedAt || s.SavedAt.IsZero() {
		s.SavedAt = time.Now().UTC()
	}

	plain, err := json.Marshal(s)
	if err != nil {
		return err
	}

	enc, err := encryptSessionJSON(plain)
	if err != nil {
		return err
	}

	if err := os.WriteFile(p, enc, 0o600); err != nil {
		return err
	}
	return nil
}

func LoadSession() (Session, bool, error) {
	// 1) Prefer OS credential store.
	s, ok, err := loadSessionKeyring()
	if err != nil {
		return Session{}, false, err
	}
	if ok {
		if s.AccessToken == "" {
			return Session{}, false, nil
		}
		return s, true, nil
	}

	// 2) Backward-compatible file fallback (optional).
	p, err := sessionPath()
	if err != nil {
		return Session{}, false, err
	}

	b, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return Session{}, false, nil
		}
		return Session{}, false, err
	}

	plain, ok, err := decryptSessionJSON(b)
	if err != nil {
		return Session{}, false, fmt.Errorf("cannot decrypt session; please login again: %w", err)
	}
	if ok {
		var sess Session
		if err := json.Unmarshal(plain, &sess); err != nil {
			return Session{}, false, fmt.Errorf("invalid session file: %w", err)
		}
		if sess.AccessToken == "" {
			return Session{}, false, nil
		}

		// Try to migrate into the OS keychain; if that succeeds, remove on-disk material.
		if err := saveSessionKeyring(sess, true); err == nil {
			_ = removeLegacySessionFiles()
		}

		return sess, true, nil
	}

	// Legacy plaintext session.json: load and migrate.
	var sess Session
	if err := json.Unmarshal(b, &sess); err != nil {
		return Session{}, false, fmt.Errorf("invalid session file: %w", err)
	}
	if sess.AccessToken == "" {
		return Session{}, false, nil
	}

	// Best-effort migration to encrypted format (ignore errors to avoid breaking existing installs).
	// Preserve the original SavedAt so we don't extend token freshness for legacy sessions
	// that lack a usable JWT exp claim.
	_ = saveSession(sess, true)

	// Attempt to migrate into the OS keychain as well.
	if err := saveSessionKeyring(sess, true); err == nil {
		_ = removeLegacySessionFiles()
	}

	return sess, true, nil
}
