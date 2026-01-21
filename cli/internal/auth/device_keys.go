package auth

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"strings"

	"github.com/zalando/go-keyring"
)

const (
	deviceKeyringUser = "device-ed25519"
	deviceSigVersion  = "v1"
)

func canonicalDeviceMessage(machineID, timestamp, method, path string, body []byte) []byte {
	// Must match server canonicalization.
	b := make([]byte, 0, 64+len(body))
	b = append(b, deviceSigVersion...)
	b = append(b, '\n')
	b = append(b, timestamp...)
	b = append(b, '\n')
	b = append(b, strings.ToUpper(method)...)
	b = append(b, '\n')
	b = append(b, path...)
	b = append(b, '\n')
	b = append(b, machineID...)
	b = append(b, '\n')
	b = append(b, body...)
	return b
}

func GetOrCreateDevicePrivateKey() (ed25519.PrivateKey, error) {
	v, err := keyring.Get(keyringService, deviceKeyringUser)
	if err == nil && strings.TrimSpace(v) != "" {
		raw, decErr := base64.RawURLEncoding.DecodeString(strings.TrimSpace(v))
		if decErr != nil {
			return nil, decErr
		}
		if len(raw) != ed25519.PrivateKeySize {
			return nil, errors.New("invalid device private key length")
		}
		return ed25519.PrivateKey(raw), nil
	}
	if err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return nil, err
	}

	_, priv, genErr := ed25519.GenerateKey(rand.Reader)
	if genErr != nil {
		return nil, genErr
	}
	enc := base64.RawURLEncoding.EncodeToString([]byte(priv))
	if setErr := keyring.Set(keyringService, deviceKeyringUser, enc); setErr != nil {
		return nil, setErr
	}
	return priv, nil
}

func GetOrCreateDevicePublicKey() (string, error) {
	priv, err := GetOrCreateDevicePrivateKey()
	if err != nil {
		return "", err
	}
	pub := priv.Public().(ed25519.PublicKey)
	return base64.RawURLEncoding.EncodeToString([]byte(pub)), nil
}

func SignDeviceRequest(machineID, timestamp, method, path string, body []byte) (string, error) {
	priv, err := GetOrCreateDevicePrivateKey()
	if err != nil {
		return "", err
	}

	msg := canonicalDeviceMessage(machineID, timestamp, method, path, body)
	sig := ed25519.Sign(priv, msg)
	return base64.RawURLEncoding.EncodeToString(sig), nil
}
