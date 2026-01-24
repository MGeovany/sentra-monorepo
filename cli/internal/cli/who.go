package cli

import (
	"errors"
	"fmt"

	"github.com/mgeovany/sentra/cli/internal/auth"
)

func runWho() error {
	// Prefer the access token claims (email/sub) when available.
	if s, ok, err := auth.LoadSession(); err != nil {
		return err
	} else if ok {
		if c, err := auth.ParseAccessTokenClaims(s.AccessToken); err == nil {
			if c.Email != "" {
				fmt.Println(c.Email)
				return nil
			}
			if c.Sub != "" {
				fmt.Println(c.Sub)
				return nil
			}
		}
	}

	// Fallback to locally stored config user id.
	if cfg, ok, err := auth.LoadConfig(); err != nil {
		return err
	} else if ok && cfg.UserID != "" {
		fmt.Println(cfg.UserID)
		return nil
	}

	return errors.New("not logged in (run: sentra login)")
}
