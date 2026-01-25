package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/mgeovany/sentra/cli/internal/auth"
	"github.com/mgeovany/sentra/cli/internal/commit"
)

func runPush() error {
	sess, err := ensureRemoteSession()
	if err != nil {
		return err
	}

	// Ensure this machine is registered before pushing.
	{
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := registerMachine(ctx, sess.AccessToken); err != nil {
			return err
		}
	}

	commits, err := commit.List()
	if err != nil {
		return err
	}

	var pending []commit.Commit
	for _, c := range commits {
		if c.PushedAt == "" {
			pending = append(pending, c)
		}
	}

	if len(pending) == 0 {
		fmt.Println("✔ nothing to push")
		return nil
	}

	sort.Slice(pending, func(i, j int) bool { return pending[i].ID < pending[j].ID })

	cfg, err := auth.EnsureConfig()
	if err != nil {
		return err
	}
	machineID := strings.TrimSpace(cfg.MachineID)
	if machineID == "" {
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
	endpoint := serverURL + "/push"

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	scanRoot := filepath.Join(homeDir, "dev")

	client := &http.Client{Timeout: 20 * time.Second}

	now := time.Now().UTC().Format(time.RFC3339)
	for _, c := range pending {
		reqs, err := buildPushRequestV1(scanRoot, machineID, name, c)
		if err != nil {
			return err
		}

		for _, reqBody := range reqs {
			b, err := json.Marshal(reqBody)
			if err != nil {
				return err
			}

			// Retry on 429 using Retry-After.
			for attempt := 0; attempt < 5; attempt++ {
				hreq, err := http.NewRequestWithContext(context.Background(), http.MethodPost, endpoint, bytes.NewReader(b))
				if err != nil {
					return err
				}
				hreq.Header.Set("Content-Type", "application/json")
				hreq.Header.Set("Accept", "application/json")
				hreq.Header.Set("Authorization", "Bearer "+strings.TrimSpace(sess.AccessToken))

				ts := fmt.Sprintf("%d", time.Now().UTC().Unix())
				nonce := uuid.NewString()
				sig, err := auth.SignDeviceRequest(machineID, ts, nonce, http.MethodPost, "/push", b)
				if err != nil {
					return err
				}
				hreq.Header.Set("X-Sentra-Machine-ID", machineID)
				hreq.Header.Set("X-Sentra-Timestamp", ts)
				hreq.Header.Set("X-Sentra-Nonce", nonce)
				hreq.Header.Set("X-Sentra-Signature", sig)

				resp, err := client.Do(hreq)
				if err != nil {
					return err
				}
				respBody, _ := io.ReadAll(resp.Body)
				_ = resp.Body.Close()

				if resp.StatusCode == http.StatusTooManyRequests {
					retryAfter := 2 * time.Second
					if v := strings.TrimSpace(resp.Header.Get("Retry-After")); v != "" {
						if n, parseErr := strconv.Atoi(v); parseErr == nil && n > 0 {
							retryAfter = time.Duration(n) * time.Second
						}
					}
					time.Sleep(retryAfter)
					continue
				}

				if resp.StatusCode < 200 || resp.StatusCode >= 300 {
					return fmt.Errorf("push failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(respBody)))
				}

				break
			}
		}

		c.PushedAt = now
		if err := commit.Update(c); err != nil {
			return err
		}
		fmt.Printf("✔ pushed commit %s\n", c.ID)
	}

	return nil
}
