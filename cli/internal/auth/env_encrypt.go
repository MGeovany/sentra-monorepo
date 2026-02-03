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
	// Legacy local-only encryption. Not portable across machines.
	envEncCipherLegacy = "ed25519+aes-256-gcm-v1"

	// Portable encryption using a per-user vault key.
	// The vault key is wrapped with a user passphrase and stored remotely.
	envEncCipherVault = "sentra-v1"
)

func encryptAESGCM(key []byte, plain []byte) (b64Ciphertext string, size int, err error) {
	if len(key) != 32 {
		return "", 0, fmt.Errorf("invalid encryption key length")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", 0, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", 0, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", 0, err
	}

	ct := gcm.Seal(nil, nonce, plain, nil)
	out := append(nonce, ct...)
	return base64.RawURLEncoding.EncodeToString(out), len(plain), nil
}

func decryptAESGCM(key []byte, b64Ciphertext string) ([]byte, error) {
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

// EncryptEnvBlobWithKey encrypts plaintext bytes with a caller-provided 32-byte key.
// This is the portable path used for cross-device sync.
func EncryptEnvBlobWithKey(key []byte, plain []byte) (cipherName string, b64Ciphertext string, size int, err error) {
	b64, sz, err := encryptAESGCM(key, plain)
	if err != nil {
		return "", "", 0, err
	}
	return envEncCipherVault, b64, sz, nil
}

// DecryptEnvBlobWithKey decrypts ciphertext using a caller-provided 32-byte key.
func DecryptEnvBlobWithKey(cipherName string, key []byte, b64Ciphertext string) ([]byte, error) {
	cipherName = strings.TrimSpace(cipherName)
	if cipherName != envEncCipherVault {
		return nil, fmt.Errorf("unsupported cipher: %s", cipherName)
	}
	return decryptAESGCM(key, b64Ciphertext)
}

// EncryptEnvBlobLegacy encrypts plaintext bytes using a per-installation symmetric key.
// This is client-side encryption for "opaque blob" storage but is NOT portable.
func EncryptEnvBlobLegacy(plain []byte) (cipherName string, b64Ciphertext string, size int, err error) {
	key, err := getOrCreateSessionKey()
	if err != nil {
		return "", "", 0, err
	}
	b64, sz, err := encryptAESGCM(key, plain)
	if err != nil {
		return "", "", 0, err
	}
	return envEncCipherLegacy, b64, sz, nil
}

// DecryptEnvBlobLegacy decrypts ciphertext using the per-installation key.
func DecryptEnvBlobLegacy(cipherName string, b64Ciphertext string) ([]byte, error) {
	cipherName = strings.TrimSpace(cipherName)
	if cipherName == "" {
		cipherName = envEncCipherLegacy
	}
	if cipherName != envEncCipherLegacy {
		return nil, fmt.Errorf("unsupported cipher: %s", cipherName)
	}
	key, err := getOrCreateSessionKey()
	if err != nil {
		return nil, err
	}
	return decryptAESGCM(key, b64Ciphertext)
}
