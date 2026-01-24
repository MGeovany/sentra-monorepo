package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/mgeovany/sentra/cli/internal/auth"
)

type registerMachineRequest struct {
	MachineID     string `json:"machine_id"`
	MachineName   string `json:"machine_name"`
	DevicePubKey  string `json:"device_pub_key"`
	DeviceKeyType string `json:"device_key_type"`
}

func registerMachine(ctx context.Context, accessToken string) error {
	cfg, err := auth.EnsureConfig()
	if err != nil {
		return err
	}
	if strings.TrimSpace(cfg.MachineID) == "" {
		return fmt.Errorf("missing machine_id")
	}

	name, _ := os.Hostname()
	name = strings.TrimSpace(name)
	if name == "" {
		name = "unknown"
	}

	serverURL, err := serverURLFromEnv()
	if err != nil {
		return err
	}

	log.Printf("serverURL: %s", serverURL)

	endpoint := serverURL + "/machines/register"

	pub, err := auth.GetOrCreateDevicePublicKey()
	if err != nil {
		return err
	}

	payload := registerMachineRequest{
		MachineID:     cfg.MachineID,
		MachineName:   name,
		DevicePubKey:  pub,
		DeviceKeyType: "ed25519",
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/plain")
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(accessToken))

	// Device binding: sign this request with the device key.
	ts := fmt.Sprintf("%d", time.Now().UTC().Unix())
	nonce := uuid.NewString()
	sig, err := auth.SignDeviceRequest(cfg.MachineID, ts, nonce, http.MethodPost, "/machines/register", b)
	if err != nil {
		return err
	}
	req.Header.Set("X-Sentra-Machine-ID", cfg.MachineID)
	req.Header.Set("X-Sentra-Timestamp", ts)
	req.Header.Set("X-Sentra-Nonce", nonce)
	req.Header.Set("X-Sentra-Signature", sig)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("machine register failed: status=%d body=%s", resp.StatusCode, string(respBody))
	}

	return nil
}
