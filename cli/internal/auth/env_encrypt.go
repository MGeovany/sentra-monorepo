package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"strings"
)

const (
	envEncCipher = "ed25519+aes-256-gcm-v1"
)

// EncryptEnvBlob encrypts plaintext bytes using a per-installation symmetric key.
// This is client-side encryption for "opaque blob" storage.
func EncryptEnvBlob(plain []byte) (cipherName string, b64Ciphertext string, size int, err error) {
	key, err := getOrCreateSessionKey()
	if err != nil {
		return "", "", 0, err
	}
	if len(key) != 32 {
		return "", "", 0, fmt.Errorf("invalid encryption key length")
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", "", 0, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", "", 0, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", "", 0, err
	}

	ct := gcm.Seal(nil, nonce, plain, nil)
	out := append(nonce, ct...)
	// Return plaintext size, not ciphertext size, since the push schema validates
	// against plaintext size limits (1 MiB). The ciphertext includes nonce + GCM tag overhead.
	return envEncCipher, base64.RawURLEncoding.EncodeToString(out), len(plain), nil
}

func DecryptEnvBlob(cipherName string, b64Ciphertext string) ([]byte, error) {
	cipherName = strings.TrimSpace(cipherName)
	if cipherName == "" {
		cipherName = envEncCipher
	}
	if cipherName != envEncCipher {
		return nil, fmt.Errorf("unsupported cipher: %s", cipherName)
	}

	key, err := getOrCreateSessionKey()
	if err != nil {
		return nil, err
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("invalid encryption key length")
	}

	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(b64Ciphertext))
	if err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(raw) < gcm.NonceSize() {
		return nil, fmt.Errorf("invalid ciphertext")
	}
	nonce := raw[:gcm.NonceSize()]
	ct := raw[gcm.NonceSize():]
	pt, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return nil, err
	}
	return pt, nil
}
