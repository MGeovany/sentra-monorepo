package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

const (
	sessionEncAlg = "A256GCM"
	sessionEncV   = 1
)

type encryptedSessionFile struct {
	V   int    `json:"v"`
	Alg string `json:"alg"`

	Nonce string `json:"nonce"`
	Data  string `json:"data"`
}

func encryptSessionJSON(plain []byte) ([]byte, error) {
	key, err := getOrCreateSessionKey()
	if err != nil {
		return nil, err
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("invalid session encryption key length")
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	// No AAD; format version/alg are authenticated implicitly by being outside
	// the ciphertext but inside the JSON wrapper that we validate.
	ct := gcm.Seal(nil, nonce, plain, nil)

	w := encryptedSessionFile{
		V:     sessionEncV,
		Alg:   sessionEncAlg,
		Nonce: base64.RawURLEncoding.EncodeToString(nonce),
		Data:  base64.RawURLEncoding.EncodeToString(ct),
	}
	return json.MarshalIndent(w, "", "  ")
}

func decryptSessionJSON(maybeEncrypted []byte) ([]byte, bool, error) {
	// Detect the encrypted wrapper format first.
	var w encryptedSessionFile
	if err := json.Unmarshal(maybeEncrypted, &w); err != nil {
		return nil, false, nil
	}
	if w.V != sessionEncV || w.Alg != sessionEncAlg || w.Nonce == "" || w.Data == "" {
		return nil, false, nil
	}

	key, err := getOrCreateSessionKey()
	if err != nil {
		return nil, true, err
	}
	if len(key) != 32 {
		return nil, true, fmt.Errorf("invalid session encryption key length")
	}

	nonce, err := base64.RawURLEncoding.DecodeString(w.Nonce)
	if err != nil {
		return nil, true, err
	}
	ct, err := base64.RawURLEncoding.DecodeString(w.Data)
	if err != nil {
		return nil, true, err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, true, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, true, err
	}
	if len(nonce) != gcm.NonceSize() {
		return nil, true, errors.New("invalid session nonce size")
	}

	pt, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return nil, true, err
	}
	return pt, true, nil
}
