package cli

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/mgeovany/sentra/cli/internal/auth"
	"github.com/zalando/go-keyring"
)

const vaultKeychainUserPrefix = "vault-key:"

func vaultKeychainUser(userID string) string {
	return vaultKeychainUserPrefix + strings.TrimSpace(userID)
}

func getVaultKeyFromKeychain(userID string) ([]byte, bool) {
	v, err := keyring.Get("sentra", vaultKeychainUser(userID))
	if err != nil {
		return nil, false
	}
	b, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(v))
	if err != nil || len(b) != 32 {
		return nil, false
	}
	return b, true
}

func saveVaultKeyToKeychain(userID string, key []byte) {
	if len(key) != 32 {
		return
	}
	_ = keyring.Set("sentra", vaultKeychainUser(userID), base64.RawURLEncoding.EncodeToString(key))
}

func promptVaultPassphrase(confirm bool) (string, error) {
	if v := strings.TrimSpace(os.Getenv("SENTRA_VAULT_PASSPHRASE")); v != "" {
		return v, nil
	}

	r := bufio.NewReader(os.Stdin)
	if !confirm {
		return promptSecret(r, "Vault passphrase")
	}
	pass1, err := promptSecret(r, "Create vault passphrase")
	if err != nil {
		return "", err
	}
	pass2, err := promptSecret(r, "Confirm vault passphrase")
	if err != nil {
		return "", err
	}
	if pass1 != pass2 {
		return "", errors.New("passphrases do not match")
	}
	return pass1, nil
}

func userIDFromAccessToken(accessToken string) (string, error) {
	claims, err := auth.ParseAccessTokenClaims(accessToken)
	if err != nil {
		return "", err
	}
	uid := strings.TrimSpace(claims.Sub)
	if uid == "" {
		return "", errors.New("missing user id")
	}
	return uid, nil
}

func fetchVaultEnvelope(ctx context.Context, serverURL string, accessToken string) (auth.VaultKeyEnvelopeV1, bool, error) {
	endpoint := strings.TrimRight(strings.TrimSpace(serverURL), "/") + "/vault/key"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return auth.VaultKeyEnvelopeV1{}, false, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(accessToken))

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return auth.VaultKeyEnvelopeV1{}, false, err
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusNotFound {
		return auth.VaultKeyEnvelopeV1{}, false, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := oneLine(string(body))
		if msg == "" {
			msg = strings.TrimSpace(http.StatusText(resp.StatusCode))
		}
		return auth.VaultKeyEnvelopeV1{}, false, fmt.Errorf("vault key fetch failed: %s", msg)
	}

	var env auth.VaultKeyEnvelopeV1
	if err := json.Unmarshal(body, &env); err != nil {
		return auth.VaultKeyEnvelopeV1{}, false, err
	}
	return env, true, nil
}

func putVaultEnvelope(ctx context.Context, serverURL string, accessToken string, env auth.VaultKeyEnvelopeV1) error {
	endpoint := strings.TrimRight(strings.TrimSpace(serverURL), "/") + "/vault/key"
	b, err := json.Marshal(env)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, endpoint, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/plain")
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(accessToken))

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if resp.StatusCode == http.StatusNotFound {
			return errors.New("server does not support portable vault keys yet; deploy the updated Sentra server")
		}
		msg := oneLine(string(body))
		if msg == "" {
			msg = strings.TrimSpace(http.StatusText(resp.StatusCode))
		}
		return fmt.Errorf("vault key save failed: %s", msg)
	}
	return nil
}

// ensureVaultKey returns a 32-byte per-user key used to encrypt/decrypt env blobs.
// It is portable across machines by wrapping it with a user passphrase and storing the wrapper remotely.
func ensureVaultKey(serverURL string, accessToken string) ([]byte, error) {
	uid, err := userIDFromAccessToken(accessToken)
	if err != nil {
		return nil, err
	}
	if k, ok := getVaultKeyFromKeychain(uid); ok {
		return k, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	env, ok, err := fetchVaultEnvelope(ctx, serverURL, accessToken)
	if err != nil {
		return nil, err
	}

	if !ok {
		// First device: create and register.
		pass, err := promptVaultPassphrase(true)
		if err != nil {
			return nil, err
		}
		k := make([]byte, 32)
		if _, err := rand.Read(k); err != nil {
			return nil, err
		}
		env, err := auth.NewVaultKeyEnvelopeV1(pass, k)
		if err != nil {
			return nil, err
		}
		if err := putVaultEnvelope(ctx, serverURL, accessToken, env); err != nil {
			return nil, err
		}
		saveVaultKeyToKeychain(uid, k)
		return k, nil
	}

	pass, err := promptVaultPassphrase(false)
	if err != nil {
		return nil, err
	}
	k, err := env.Unwrap(pass)
	if err != nil {
		return nil, errors.New("failed to unlock vault key (wrong passphrase?)")
	}
	saveVaultKeyToKeychain(uid, k)
	return k, nil
}
