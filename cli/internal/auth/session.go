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
	p, err := sessionPath()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}

	s.SavedAt = time.Now().UTC()
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

	if plain, ok, err := decryptSessionJSON(b); err != nil {
		return Session{}, false, fmt.Errorf("cannot decrypt session; please login again: %w", err)
	} else if ok {
		var s Session
		if err := json.Unmarshal(plain, &s); err != nil {
			return Session{}, false, fmt.Errorf("invalid session file: %w", err)
		}
		if s.AccessToken == "" {
			return Session{}, false, nil
		}
		return s, true, nil
	}

	// Legacy plaintext session.json: load and migrate to encrypted on next save.
	var s Session
	if err := json.Unmarshal(b, &s); err != nil {
		return Session{}, false, fmt.Errorf("invalid session file: %w", err)
	}
	if s.AccessToken == "" {
		return Session{}, false, nil
	}

	// Best-effort migration to encrypted format (ignore errors to avoid breaking existing installs).
	_ = SaveSession(s)

	return s, true, nil
}
