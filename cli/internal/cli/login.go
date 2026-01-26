package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mgeovany/sentra/cli/internal/auth"
	"github.com/mgeovany/sentra/cli/internal/storage"
)

func runLogin() error {
	auth.LoadDotEnv()

	supabaseURL := strings.TrimSpace(os.Getenv("SUPABASE_URL"))
	anonKey := strings.TrimSpace(os.Getenv("SUPABASE_ANON_KEY"))
	if supabaseURL == "" {
		supabaseURL = defaultHostedSupabaseURL
	}
	if anonKey == "" {
		anonKey = defaultHostedSupabaseAnonKey
	}

	verifier, err := auth.NewCodeVerifier()
	if err != nil {
		return err
	}
	challenge := auth.CodeChallengeS256(verifier)

	// Use a random loopback port by default to reduce spoof/race attempts.
	port := 0
	if v := strings.TrimSpace(os.Getenv("SENTRA_AUTH_PORT")); v != "" {
		p, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("invalid auth port: %w", err)
		}
		port = p
	}

	addr := fmt.Sprintf("127.0.0.1:%d", port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("cannot listen on %s: %w", addr, err)
	}
	defer func() { _ = ln.Close() }()

	// Resolve the actual port (important when using 0).
	tcpAddr, ok := ln.Addr().(*net.TCPAddr)
	if !ok || tcpAddr == nil || tcpAddr.Port == 0 {
		return fmt.Errorf("cannot resolve listener port")
	}

	nonce, err := auth.NewState()
	if err != nil {
		return err
	}

	redirectToURL := &url.URL{
		Scheme: "http",
		Host:   fmt.Sprintf("127.0.0.1:%d", tcpAddr.Port),
		Path:   "/callback",
	}
	qRedirect := redirectToURL.Query()
	qRedirect.Set("sentra_state", nonce)
	redirectToURL.RawQuery = qRedirect.Encode()
	redirectTo := redirectToURL.String()

	oauth := auth.SupabaseOAuth{SupabaseURL: supabaseURL, AnonKey: anonKey, Provider: "google"}
	authURL, err := oauth.AuthorizeURL(redirectTo, challenge)
	if err != nil {
		return err
	}

	fmt.Println("Please open this link to login:")
	fmt.Println(authURL)
	fmt.Println()
	fmt.Println("Waiting for browser callback...")

	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)
	var cbOnce sync.Once

	srv := &http.Server{}
	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()

		// Hardening: validate a CLI-generated nonce embedded in redirect_to.
		if got := q.Get("sentra_state"); got == "" || got != nonce {
			w.WriteHeader(http.StatusBadRequest)
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			_, _ = w.Write([]byte("Login failed: invalid callback state.\n"))
			select {
			case errCh <- errors.New("invalid callback state"):
			default:
			}
			return
		}

		// Only accept the first valid callback to reduce spoofing/races.
		handled := false
		cbOnce.Do(func() { handled = true })
		if !handled {
			w.WriteHeader(http.StatusConflict)
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			_, _ = w.Write([]byte("Callback already handled. You can close this tab."))
			return
		}

		code := q.Get("code")
		if code == "" {
			// Some setups return errors in query string.
			errDesc := q.Get("error_description")
			errName := q.Get("error")
			errCode := q.Get("error_code")
			w.WriteHeader(http.StatusBadRequest)
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			if errDesc != "" || errName != "" || errCode != "" {
				_, _ = fmt.Fprintf(w,
					"Login failed.\n\nAuthentication error: %s\n\nPlease try again or contact support if the problem persists.\n",
					errDesc,
				)
				select {
				case errCh <- fmt.Errorf("oauth error: %s (%s) %s", errName, errCode, errDesc):
				default:
				}
			} else {
				_, _ = w.Write([]byte("Login failed: authentication incomplete.\n"))
				select {
				case errCh <- errors.New("authentication incomplete"):
				default:
				}
			}
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("Login successful. You can close this tab and return to the CLI."))
		select {
		case codeCh <- code:
		default:
		}

		// Close the listener immediately after the first successful callback.
		// Do it asynchronously to avoid blocking the handler while writing.
		go func() { _ = srv.Shutdown(context.Background()) }()
	})

	srv.Handler = mux
	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			select {
			case errCh <- err:
			default:
			}
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	var authCode string
	select {
	case authCode = <-codeCh:
	case err := <-errCh:
		_ = srv.Shutdown(context.Background())
		return err
	case <-ctx.Done():
		_ = srv.Shutdown(context.Background())
		return errors.New("login timed out")
	}

	_ = srv.Shutdown(context.Background())

	// OAuth provider returns URL-encoded code sometimes; normalize.
	if decoded, decodeErr := url.QueryUnescape(authCode); decodeErr == nil {
		authCode = decoded
	}

	exchangeCtx, cancelExchange := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancelExchange()
	tr, err := oauth.ExchangePKCE(exchangeCtx, authCode, verifier)
	if err != nil {
		return err
	}

	if err := auth.SaveSession(auth.Session{
		AccessToken:  tr.AccessToken,
		RefreshToken: tr.RefreshToken,
		TokenType:    tr.TokenType,
		ExpiresIn:    tr.ExpiresIn,
	}); err != nil {
		return err
	}

	if _, err := auth.EnsureConfig(); err != nil {
		return err
	}

	claims, err := auth.ParseAccessTokenClaims(tr.AccessToken)
	if err == nil {
		_ = auth.SetUserID(claims.Sub)
	}

	// Persist server URL so users don't need to set env vars.
	{
		cfg, _, err := auth.LoadConfig()
		if err != nil {
			return err
		}
		if cfg.MachineID == "" {
			if cfg2, ensureErr := auth.EnsureConfig(); ensureErr == nil {
				cfg = cfg2
			} else {
				return ensureErr
			}
		}
		if serverURL, err := serverURLFromEnv(); err == nil {
			cfg.ServerURL = serverURL
			if err := auth.SaveConfig(cfg); err != nil {
				return err
			}
		}
	}

	// Persist server URL so users don't need to set env vars.
	{
		cfg, _, err := auth.LoadConfig()
		if err != nil {
			return err
		}
		if cfg.MachineID == "" {
			if cfg2, ensureErr := auth.EnsureConfig(); ensureErr == nil {
				cfg = cfg2
			} else {
				return ensureErr
			}
		}
		if serverURL, err := serverURLFromEnv(); err == nil {
			cfg.ServerURL = serverURL
			if err := auth.SaveConfig(cfg); err != nil {
				return err
			}
		}
	}

	// Storage choice: hosted provider vs BYOS (S3-compatible).
	{
		r := bufio.NewReader(os.Stdin)
		fmt.Println("Choose storage mode for encrypted .env blobs:")
		choice, err := promptSelect(r, []string{
			"Use Sentra storage (recommended)",
			"Use my storage (AWS S3 / Cloudflare R2 / MinIO / custom S3)",
		})
		if err != nil {
			return err
		}

		cfg, _, err := auth.LoadConfig()
		if err != nil {
			return err
		}
		if cfg.MachineID == "" {
			if cfg2, ensureErr := auth.EnsureConfig(); ensureErr == nil {
				cfg = cfg2
			} else {
				return ensureErr
			}
		}

		switch choice {
		case 1:
			cfg.StorageMode = "hosted"
			_ = storage.DeleteConfig() // best-effort: disable BYOS if previously configured
			if err := auth.SaveConfig(cfg); err != nil {
				return err
			}
		case 2:
			cfg.StorageMode = "byos"
			if err := auth.SaveConfig(cfg); err != nil {
				return err
			}
			if _, ok, err := storage.LoadConfig(); err != nil {
				return err
			} else if !ok {
				fmt.Println("Let's set up your storage provider.")
				if err := runStorageSetup(); err != nil {
					return err
				}
			}
		default:
			return errors.New("invalid storage mode")
		}
	}

	// Best-effort: register machine with the server if reachable.
	{
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := registerMachine(ctx, tr.AccessToken); err != nil {
			// Keep login output clean; registration is optional and can be diagnosed via `sentra doctor`.
			if v := strings.TrimSpace(os.Getenv("SENTRA_VERBOSE")); v == "1" || strings.EqualFold(v, "true") {
				fmt.Printf("Note: remote machine registration skipped: %v\n", err)
				fmt.Println("Hint: run `sentra doctor` to diagnose server connectivity issues.")
			}
		}
	}

	fmt.Println("logged in")
	return nil
}
