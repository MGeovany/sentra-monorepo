package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/mgeovany/sentra/cli/internal/auth"
)

type registerMachineRequest struct {
	MachineID   string `json:"machine_id"`
	MachineName string `json:"machine_name"`
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

	serverURL := strings.TrimSpace(os.Getenv("SENTRA_SERVER_URL"))
	if serverURL == "" {
		serverURL = "http://localhost:8080"
	}
	serverURL = strings.TrimRight(serverURL, "/")

	endpoint := serverURL + "/machines/register"

	payload := registerMachineRequest{
		MachineID:   cfg.MachineID,
		MachineName: name,
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

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("machine register failed: status=%d body=%s", resp.StatusCode, string(respBody))
	}

	return nil
}
