package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/mgeovany/sentra/cli/internal/auth"
)

func ensureRemoteSession() error {
	auth.LoadDotEnv()

	supabaseURL := strings.TrimSpace(os.Getenv("SUPABASE_URL"))
	anonKey := strings.TrimSpace(os.Getenv("SUPABASE_ANON_KEY"))
	if supabaseURL == "" || anonKey == "" {
		return errors.New("missing SUPABASE_URL or SUPABASE_ANON_KEY")
	}

	oauth := auth.SupabaseOAuth{SupabaseURL: supabaseURL, AnonKey: anonKey, Provider: "github"}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	s, err := auth.EnsureSession(ctx, oauth)
	if err == nil {
		// Keep local config aligned with current user.
		if claims, parseErr := auth.ParseAccessTokenClaims(s.AccessToken); parseErr == nil {
			_ = auth.SetUserID(claims.Sub)
		}

		// Ensure this machine is registered before remote operations.
		regCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := registerMachine(regCtx, s.AccessToken); err != nil {
			return fmt.Errorf("could not register user/machine with remote: %w", err)
		}

		return nil
	}

	if errors.Is(err, auth.ErrNoSession) {
		fmt.Println("please login to push changes to remote")
		return runLogin()
	}

	// If refresh failed for any reason, ask user to login again.
	fmt.Println("session expired; please login again")
	return runLogin()
}
