package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/mgeovany/sentra/cli/internal/auth"
	"github.com/mgeovany/sentra/cli/internal/commit"
	"github.com/mgeovany/sentra/cli/internal/storage"
	"github.com/minio/minio-go/v7"
)

func isVerbose() bool {
	v := strings.TrimSpace(os.Getenv("SENTRA_VERBOSE"))
	return v == "1" || strings.EqualFold(v, "true")
}

func oneLine(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	// Avoid log poisoning / ugly multiline errors.
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\t", " ")
	for strings.Contains(s, "  ") {
		s = strings.ReplaceAll(s, "  ", " ")
	}
	const max = 220
	if len(s) > max {
		s = s[:max] + "..."
	}
	return s
}

func runPush() error {
	verbosef("Starting push operation...")
	sess, err := ensureRemoteSession()
	if err != nil {
		return err
	}
	verbosef("Session loaded: user authenticated")

	// Ensure this machine is registered before pushing.
	{
		sp := startSpinner("Registering machine...")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := registerMachine(ctx, sess.AccessToken); err != nil {
			sp.StopInfo("")
			return err
		}
		sp.StopSuccess("✔ machine registered")
		verbosef("Machine registration completed")
	}

	commits, err := commit.List()
	if err != nil {
		return err
	}
	verbosef("Loaded %d total commit(s) from local storage", len(commits))

	var pending []commit.Commit
	for _, c := range commits {
		if c.PushedAt == "" {
			pending = append(pending, c)
		}
	}

	if len(pending) == 0 {
		successf("✔ nothing to push")
		verbosef("All commits have already been pushed")
		return nil
	}

	verbosef("Found %d pending commit(s) to push", len(pending))
	sort.Slice(pending, func(i, j int) bool { return pending[i].ID < pending[j].ID })

	cfg, err := auth.EnsureConfig()
	if err != nil {
		return err
	}
	machineID := strings.TrimSpace(cfg.MachineID)
	if machineID == "" {
		return fmt.Errorf("machine not registered; please run: sentra login")
	}
	verbosef("Machine ID: %s", machineID)

	name, _ := os.Hostname()
	name = strings.TrimSpace(name)
	if name == "" {
		name = "unknown"
	}
	verbosef("Machine name: %s", name)

	serverURL, err := serverURLFromEnv()
	if err != nil {
		return err
	}
	endpoint := serverURL + "/push"
	verbosef("Server URL: %s", serverURL)
	verbosef("Push endpoint: %s", endpoint)

	scanRoot, err := resolveScanRoot()
	if err != nil {
		return err
	}

	client := &http.Client{Timeout: 20 * time.Second}

	now := time.Now().UTC().Format(time.RFC3339)
	for i, c := range pending {
		shortID := c.ID
		if len(shortID) > 8 {
			shortID = shortID[:8]
		}
		sp := startSpinner(fmt.Sprintf("Pushing commit %s (%d/%d)...", shortID, i+1, len(pending)))
		verbosef("Processing commit %s: %d file(s), message: %s", c.ID, len(c.Files), oneLine(c.Message))

		userID := strings.TrimSpace(cfg.UserID)
		if userID == "" {
			if claims, err := auth.ParseAccessTokenClaims(sess.AccessToken); err == nil {
				userID = strings.TrimSpace(claims.Sub)
			}
		}
		if userID == "" {
			sp.StopInfo("")
			return fmt.Errorf("not logged in; please run: sentra login")
		}
		verbosef("User ID: %s", userID)

		storageMode := strings.TrimSpace(cfg.StorageMode)
		if storageMode == "" {
			storageMode = "hosted"
		}
		verbosef("Storage mode: %s", storageMode)

		var (
			s3cfg storage.S3Config
			s3c   *minio.Client
			byos  bool
		)
		if storageMode == "byos" {
			var enabled bool
			s3cfg, s3c, enabled, err = storage.ResolveS3()
			if err != nil {
				return err
			}
			if !enabled {
				sp.StopInfo("")
				return fmt.Errorf("storage not configured (run: sentra storage setup)")
			}
			byos = true
			verbosef("BYOS storage: bucket=%s, endpoint=%s, region=%s", s3cfg.Bucket, s3cfg.Endpoint, s3cfg.Region)
		}

		verbosef("Building push request for commit %s...", c.ID)
		reqs, err := buildPushRequestV1(context.Background(), scanRoot, machineID, name, c, s3cfg, s3c, byos, userID)
		if err != nil {
			sp.StopInfo("")
			return err
		}
		verbosef("Built %d push request(s) for %d project(s)", len(reqs), len(reqs))

		for _, reqBody := range reqs {
			verbosef("Pushing to project: %s (%d file(s))", reqBody.Project.Root, len(reqBody.Files))
			b, err := json.Marshal(reqBody)
			if err != nil {
				return err
			}
			verbosef("Request payload size: %d bytes", len(b))

			// Stable per (user, project.root, commit.client_id) so retries can be cheap.
			idemKey := uuid.NewSHA1(uuid.NameSpaceOID, []byte("push:"+userID+":"+strings.TrimSpace(reqBody.Project.Root)+":"+strings.TrimSpace(reqBody.Commit.ClientID))).String()
			verbosef("Idempotency key: %s", idemKey)

			// Retry on 429 using Retry-After.
			for attempt := 0; attempt < 5; attempt++ {
				if attempt > 0 {
					verbosef("Retry attempt %d/5", attempt+1)
				}
				startTime := time.Now()
				hreq, err := http.NewRequestWithContext(context.Background(), http.MethodPost, endpoint, bytes.NewReader(b))
				if err != nil {
					sp.StopInfo("")
					return err
				}
				hreq.Header.Set("Content-Type", "application/json")
				hreq.Header.Set("Accept", "application/json")
				hreq.Header.Set("Authorization", "Bearer "+strings.TrimSpace(sess.AccessToken))
				hreq.Header.Set("X-Idempotency-Key", idemKey)

				ts := fmt.Sprintf("%d", time.Now().UTC().Unix())
				nonce := uuid.NewString()
				sig, err := auth.SignDeviceRequest(machineID, ts, nonce, http.MethodPost, "/push", b)
				if err != nil {
					sp.StopInfo("")
					return err
				}
				hreq.Header.Set("X-Sentra-Machine-ID", machineID)
				hreq.Header.Set("X-Sentra-Timestamp", ts)
				hreq.Header.Set("X-Sentra-Nonce", nonce)
				hreq.Header.Set("X-Sentra-Signature", sig)
				verbosef("Sending HTTP POST to %s", endpoint)

				resp, err := client.Do(hreq)
				elapsed := time.Since(startTime)
				if err != nil {
					sp.StopInfo("")
					verbosef("Request failed after %v: %v", elapsed, err)
					return err
				}
				respBody, _ := io.ReadAll(resp.Body)
				_ = resp.Body.Close()
				verbosef("Response received: status=%d, size=%d bytes, elapsed=%v", resp.StatusCode, len(respBody), elapsed)

				if resp.StatusCode == http.StatusTooManyRequests {
					retryAfter := 2 * time.Second
					if v := strings.TrimSpace(resp.Header.Get("Retry-After")); v != "" {
						if n, parseErr := strconv.Atoi(v); parseErr == nil && n > 0 {
							retryAfter = time.Duration(n) * time.Second
						}
					}
					verbosef("Rate limited, retrying after %v", retryAfter)
					time.Sleep(retryAfter)
					continue
				}

				if resp.StatusCode < 200 || resp.StatusCode >= 300 {
					msg := oneLine(string(respBody))
					if msg == "" {
						msg = strings.TrimSpace(http.StatusText(resp.StatusCode))
					}
					if isVerbose() {
						sp.StopInfo("")
						verbosef("Response body: %s", string(respBody))
						return fmt.Errorf("push failed: status=%d msg=%s", resp.StatusCode, msg)
					}
					sp.StopInfo("")
					return fmt.Errorf("push failed: server returned %d (%s)", resp.StatusCode, msg)
				}

				verbosef("Successfully pushed project %s", reqBody.Project.Root)
				break
			}
		}

		c.PushedAt = now
		if err := commit.Update(c); err != nil {
			sp.StopInfo("")
			return err
		}
		sp.StopSuccess(fmt.Sprintf("✔ pushed commit %s", shortID))
		verbosef("Commit %s marked as pushed at %s", c.ID, now)
	}

	return nil
}
