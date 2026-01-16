package cli

import (
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
)

func runLogin() error {
	auth.LoadDotEnv()

	supabaseURL := strings.TrimSpace(os.Getenv("SUPABASE_URL"))
	anonKey := strings.TrimSpace(os.Getenv("SUPABASE_ANON_KEY"))
	if supabaseURL == "" || anonKey == "" {
		return errors.New("missing SUPABASE_URL or SUPABASE_ANON_KEY")
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
			return fmt.Errorf("invalid SENTRA_AUTH_PORT: %w", err)
		}
		port = p
	}

	addr := fmt.Sprintf("127.0.0.1:%d", port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("cannot listen on %s: %w", addr, err)
	}
	defer ln.Close()

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

	oauth := auth.SupabaseOAuth{SupabaseURL: supabaseURL, AnonKey: anonKey, Provider: "github"}
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
				_, _ = w.Write([]byte(fmt.Sprintf(
					"Login failed.\n\nSupabase returned:\n- error: %s\n- error_code: %s\n- error_description: %s\n\nFix: verify the GitHub provider settings in Supabase and the GitHub OAuth App callback URL.\n",
					errName,
					errCode,
					errDesc,
				)))
				select {
				case errCh <- fmt.Errorf("oauth error: %s (%s) %s", errName, errCode, errDesc):
				default:
				}
			} else {
				_, _ = w.Write([]byte("Login failed: missing oauth code.\n"))
				select {
				case errCh <- errors.New("missing oauth code"):
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

	// Supabase returns URL-encoded code sometimes; normalize.
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

	// Best-effort: register machine with the server if reachable.
	{
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := registerMachine(ctx, tr.AccessToken); err != nil {
			fmt.Printf("Warning: could not register user/machine with remote: %v\n", err)
			fmt.Println("Hint: check SENTRA_SERVER_URL and ensure the server has SUPABASE_URL and SUPABASE_SERVICE_ROLE_KEY.")
		}
	}

	fmt.Println("logged in")
	return nil
}
