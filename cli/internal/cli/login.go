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
	supabaseBase := strings.TrimRight(strings.TrimSpace(supabaseURL), "/")

	verifier, err := auth.NewCodeVerifier()
	if err != nil {
		return err
	}
	challenge := auth.CodeChallengeS256(verifier)

	port := 53124
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

	redirectTo := fmt.Sprintf("http://localhost:%d/callback", port)

	fmt.Println("Add this Redirect URL in Supabase (Authentication → URL Configuration → Redirect URLs):")
	fmt.Println(redirectTo)
	fmt.Println()
	fmt.Println("Ensure your GitHub OAuth App 'Authorization callback URL' is set to the Supabase callback (not localhost):")
	fmt.Println(supabaseBase + "/auth/v1/callback")
	fmt.Println()
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

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()

		// NOTE: Supabase manages OAuth `state` internally and expects it to be in
		// a specific format. We do not supply our own `state`.
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
				errCh <- fmt.Errorf("oauth error: %s (%s) %s", errName, errCode, errDesc)
			} else {
				_, _ = w.Write([]byte("Login failed: missing oauth code.\n"))
				errCh <- errors.New("missing oauth code")
			}
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("Login successful. You can close this tab and return to the CLI."))
		codeCh <- code
	})

	srv := &http.Server{Handler: mux}
	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
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

	fmt.Println("✔ logged in")
	return nil
}
