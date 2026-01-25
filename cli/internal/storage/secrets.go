package storage

import (
	"encoding/json"
	"strings"

	"github.com/zalando/go-keyring"
)

const (
	keyringService = "sentra"
)

type storedSecret struct {
	SecretAccessKey string `json:"secret_access_key"`
	SessionToken    string `json:"session_token,omitempty"`
}

func SecretRefForID(id string) string {
	id = strings.TrimSpace(id)
	return "storage:" + id
}

func SaveSecret(ref string, secretKey string, sessionToken string) error {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return keyring.ErrNotFound
	}
	b, err := json.Marshal(storedSecret{SecretAccessKey: strings.TrimSpace(secretKey), SessionToken: strings.TrimSpace(sessionToken)})
	if err != nil {
		return err
	}
	return keyring.Set(keyringService, ref, string(b))
}

func LoadSecret(ref string) (secretKey string, sessionToken string, ok bool, err error) {
	ref = strings.TrimSpace(ref)
	v, err := keyring.Get(keyringService, ref)
	if err != nil {
		if err == keyring.ErrNotFound {
			return "", "", false, nil
		}
		return "", "", false, err
	}
	if strings.TrimSpace(v) == "" {
		return "", "", false, nil
	}
	var s storedSecret
	if err := json.Unmarshal([]byte(v), &s); err != nil {
		return "", "", false, err
	}
	if strings.TrimSpace(s.SecretAccessKey) == "" {
		return "", "", false, nil
	}
	return s.SecretAccessKey, s.SessionToken, true, nil
}

func DeleteSecret(ref string) error {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil
	}
	err := keyring.Delete(keyringService, ref)
	if err == keyring.ErrNotFound {
		return nil
	}
	return err
}

// Internal aliases for backward compatibility.
var loadSecret = LoadSecret
var deleteSecret = DeleteSecret
