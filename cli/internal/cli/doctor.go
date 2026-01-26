package cli

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mgeovany/sentra/cli/internal/auth"
	"github.com/mgeovany/sentra/cli/internal/commit"
	"github.com/zalando/go-keyring"
)

type doctorDiag struct {
	fails int
	warns int
}

func errorsIsKeyringNotFound(err error) bool {
	return errors.Is(err, keyring.ErrNotFound)
}

func (d *doctorDiag) okf(format string, args ...any) {
	fmt.Printf("✔ "+format+"\n", args...)
}

func (d *doctorDiag) warnf(format string, args ...any) {
	d.warns++
	fmt.Printf("⚠ "+format+"\n", args...)
}

func (d *doctorDiag) failf(format string, args ...any) {
	d.fails++
	fmt.Printf("✖ "+format+"\n", args...)
}

func runDoctor() error {
	auth.LoadDotEnv()

	var d doctorDiag
	fmt.Println("sentra doctor")
	fmt.Println()

	// --- Auth ---
	fmt.Println("Auth")

	supabaseURL := strings.TrimSpace(os.Getenv("SUPABASE_URL"))
	anonKey := strings.TrimSpace(os.Getenv("SUPABASE_ANON_KEY"))
	if supabaseURL == "" {
		supabaseURL = defaultHostedSupabaseURL
	}
	if anonKey == "" {
		anonKey = defaultHostedSupabaseAnonKey
	}
	d.okf("authentication service configured")

	if cfg, ok, err := auth.LoadConfig(); err != nil {
		d.warnf("cannot read auth config: %v", err)
	} else if !ok {
		d.warnf("no local auth config (~/.sentra/config.json)")
	} else if strings.TrimSpace(cfg.MachineID) == "" {
		d.warnf("missing machine id in auth config")
	} else {
		d.okf("machine id configured")
	}

	sess, sessOK, err := auth.LoadSession()
	if err != nil {
		d.failf("cannot load session: %v", err)
	} else if !sessOK {
		d.warnf("not logged in (run: sentra login)")
	} else {
		if claims, err := auth.ParseAccessTokenClaims(sess.AccessToken); err == nil {
			who := strings.TrimSpace(claims.Email)
			if who == "" {
				who = strings.TrimSpace(claims.Sub)
			}
			if who != "" {
				d.okf("session user: %s", who)
			} else {
				d.okf("session loaded")
			}
		} else {
			d.warnf("cannot parse access token claims: %v", err)
		}

		exp := sess.ExpiresAt()
		now := time.Now().UTC()
		if exp.IsZero() {
			d.warnf("session expiry unknown")
		} else {
			until := time.Until(exp)
			if now.After(exp) {
				until = -time.Since(exp)
			}
			if until < 0 {
				// Try to refresh if possible.
				if supabaseURL != "" && anonKey != "" && strings.TrimSpace(sess.RefreshToken) != "" {
					oauth := auth.SupabaseOAuth{SupabaseURL: supabaseURL, AnonKey: anonKey, Provider: "google"}
					ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
					defer cancel()
					if _, err := auth.EnsureSession(ctx, oauth); err != nil {
						d.failf("session expired and refresh failed: %v", err)
					} else {
						d.okf("session refreshed")
					}
				} else {
					d.failf("session expired (run: sentra login)")
				}
			} else if until < 2*time.Minute {
				d.warnf("session expires soon (%s)", until.Round(time.Second))
			} else {
				d.okf("session valid (%s remaining)", until.Round(time.Second))
			}
		}
	}

	fmt.Println()

	// --- Server ---
	fmt.Println("Server")
	serverURL, err := serverURLFromEnv()
	if err != nil {
		d.failf("invalid server url: %v", err)
		fmt.Println()
		return fmt.Errorf("doctor: %d issue(s) found", d.fails)
	}
	d.okf("url: %s", serverURL)

	client := &http.Client{Timeout: 5 * time.Second}
	start := time.Now().UTC()
	resp, err := client.Get(serverURL + "/health")
	elapsed := time.Since(start)

	var serverDate time.Time
	if err != nil {
		d.failf("cannot reach server /health: %v", err)
	} else {
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			d.failf("server /health returned %s", resp.Status)
		} else {
			d.okf("/health ok (%s RTT)", elapsed.Round(time.Millisecond))
		}
		if v := strings.TrimSpace(resp.Header.Get("Date")); v != "" {
			if t, err := http.ParseTime(v); err == nil {
				serverDate = t.UTC()
			}
		}
	}

	if sessOK {
		req, _ := http.NewRequest("GET", serverURL+"/users/me", nil)
		req.Header.Set("Authorization", "Bearer "+sess.AccessToken)
		resp, err := client.Do(req)
		if err != nil {
			d.warnf("/users/me unreachable: %v", err)
		} else {
			_ = resp.Body.Close()
			switch resp.StatusCode {
			case http.StatusOK:
				d.okf("/users/me auth ok")
			case http.StatusUnauthorized, http.StatusForbidden:
				d.failf("/users/me auth failed (%s)", resp.Status)
			default:
				d.warnf("/users/me returned %s", resp.Status)
			}
		}
	} else {
		d.warnf("skipping /users/me (not logged in)")
	}

	fmt.Println()

	// --- Storage ---
	fmt.Println("Storage")
	homeDir, err := os.UserHomeDir()
	if err != nil {
		d.failf("cannot resolve home dir: %v", err)
	} else {
		sentraDir := filepath.Join(homeDir, ".sentra")
		if _, err := os.Stat(sentraDir); err != nil {
			if os.IsNotExist(err) {
				d.warnf("%s does not exist yet (will be created on first use)", sentraDir)
			} else {
				d.warnf("cannot stat %s: %v", sentraDir, err)
			}
		} else {
			d.okf("state dir: %s", sentraDir)
		}

		if commitsDir, err := commit.Dir(); err == nil {
			if _, err := os.Stat(commitsDir); err != nil {
				if os.IsNotExist(err) {
					d.warnf("no commits yet (%s missing)", commitsDir)
				} else {
					d.warnf("cannot stat commits dir: %v", err)
				}
			} else {
				d.okf("commits dir: %s", commitsDir)
			}
		}
	}

	// Best-effort keychain diagnostics (no writes).
	if _, err := keyring.Get("sentra", "session"); err == nil || errorsIsKeyringNotFound(err) {
		d.okf("keychain: available")
	} else {
		d.warnf("keychain unavailable: %v", err)
		d.warnf("hint: set SENTRA_ALLOW_INSECURE_SESSION_FILE=true to allow encrypted file fallback")
	}
	if _, err := keyring.Get("sentra", "session-key"); err != nil && !errorsIsKeyringNotFound(err) {
		// Don't fail: encryption key can still fallback to ~/.sentra/session.key.
		d.warnf("keychain session key unavailable: %v", err)
	}

	fmt.Println()

	// --- Clock drift ---
	fmt.Println("Clock drift")
	if serverDate.IsZero() {
		d.warnf("server time unavailable (missing Date header)")
	} else {
		// Estimate local time at response midpoint.
		localMid := start.Add(elapsed / 2).UTC()
		drift := serverDate.Sub(localMid)
		abs := drift
		if abs < 0 {
			abs = -abs
		}
		if abs > 60*time.Second {
			d.failf("clock drift too high: %s (RTT %s)", drift.Round(time.Millisecond), elapsed.Round(time.Millisecond))
			d.warnf("fix: enable NTP and ensure your system clock is correct")
		} else if abs > 5*time.Second {
			d.warnf("clock drift: %s (RTT %s)", drift.Round(time.Millisecond), elapsed.Round(time.Millisecond))
			d.warnf("fix: enable NTP and ensure your system clock is correct")
		} else {
			d.okf("clock drift ok: %s (RTT %s)", drift.Round(time.Millisecond), elapsed.Round(time.Millisecond))
		}
	}

	fmt.Println()
	if d.fails == 0 {
		if d.warns > 0 {
			fmt.Printf("done (%d warning(s))\n", d.warns)
		} else {
			fmt.Println("done")
		}
		return nil
	}
	fmt.Printf("done (%d failure(s), %d warning(s))\n", d.fails, d.warns)
	return fmt.Errorf("doctor: %d issue(s) found", d.fails)
}
