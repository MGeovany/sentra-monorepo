package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"

	"golang.org/x/crypto/argon2"
)

// VaultKeyEnvelopeV1 stores an encrypted (wrapped) vault key.
// The server stores this envelope as an opaque JSON blob.
type VaultKeyEnvelopeV1 struct {
	V int `json:"v"`

	KDF      string `json:"kdf"`
	SaltB64  string `json:"salt_b64"`
	Time     uint32 `json:"t"`
	MemoryKB uint32 `json:"m"`
	Threads  uint8  `json:"p"`
	KeyLen   uint32 `json:"key_len"`

	WrappedKeyB64 string `json:"wrapped_key_b64"`
}

const (
	vaultKDFArgon2id = "argon2id"
)

func NewVaultKeyEnvelopeV1(passphrase string, vaultKey []byte) (VaultKeyEnvelopeV1, error) {
	passphrase = strings.TrimSpace(passphrase)
	if len(vaultKey) != 32 {
		return VaultKeyEnvelopeV1{}, fmt.Errorf("invalid vault key length")
	}
	if len(passphrase) < 8 {
		return VaultKeyEnvelopeV1{}, errors.New("passphrase must be at least 8 characters")
	}

	// Params are chosen to be reasonable for a CLI (interactive) flow.
	// Memory is in KiB.
	t := uint32(2)
	m := uint32(128 * 1024)
	p := uint8(1)
	keyLen := uint32(32)

	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return VaultKeyEnvelopeV1{}, err
	}

	derived := argon2.IDKey([]byte(passphrase), salt, t, m, p, keyLen)
	if len(derived) != 32 {
		return VaultKeyEnvelopeV1{}, fmt.Errorf("invalid derived key length")
	}

	block, err := aes.NewCipher(derived)
	if err != nil {
		return VaultKeyEnvelopeV1{}, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return VaultKeyEnvelopeV1{}, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return VaultKeyEnvelopeV1{}, err
	}
	ct := gcm.Seal(nil, nonce, vaultKey, nil)
	out := append(nonce, ct...)

	return VaultKeyEnvelopeV1{
		V: 1,

		KDF:      vaultKDFArgon2id,
		SaltB64:  base64.RawURLEncoding.EncodeToString(salt),
		Time:     t,
		MemoryKB: m,
		Threads:  p,
		KeyLen:   keyLen,

		WrappedKeyB64: base64.RawURLEncoding.EncodeToString(out),
	}, nil
}

func (e VaultKeyEnvelopeV1) Unwrap(passphrase string) ([]byte, error) {
	passphrase = strings.TrimSpace(passphrase)
	if e.V != 1 {
		return nil, fmt.Errorf("unsupported envelope version: %d", e.V)
	}
	if strings.TrimSpace(e.KDF) != vaultKDFArgon2id {
		return nil, fmt.Errorf("unsupported kdf: %s", strings.TrimSpace(e.KDF))
	}
	if passphrase == "" {
		return nil, errors.New("missing passphrase")
	}
	if e.KeyLen != 32 {
		return nil, fmt.Errorf("unsupported key length: %d", e.KeyLen)
	}

	salt, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(e.SaltB64))
	if err != nil {
		return nil, err
	}
	wrapped, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(e.WrappedKeyB64))
	if err != nil {
		return nil, err
	}

	derived := argon2.IDKey([]byte(passphrase), salt, e.Time, e.MemoryKB, e.Threads, e.KeyLen)
	if len(derived) != 32 {
		return nil, fmt.Errorf("invalid derived key length")
	}

	block, err := aes.NewCipher(derived)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(wrapped) < gcm.NonceSize() {
		return nil, fmt.Errorf("invalid wrapped key")
	}
	nonce := wrapped[:gcm.NonceSize()]
	ct := wrapped[gcm.NonceSize():]
	pt, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return nil, err
	}
	if len(pt) != 32 {
		return nil, fmt.Errorf("invalid vault key length")
	}
	return pt, nil
}
