package cli

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"

	"github.com/mgeovany/sentra/cli/internal/auth"
)

const defaultHostedServerURL = "https://sentra-server-198360446701.us-east1.run.app"

func serverURLFromEnv() (string, error) {
	// Load .env file first to ensure environment variables are available.
	auth.LoadDotEnv()

	// Highest priority: explicit server URL from environment.
	v := strings.TrimSpace(os.Getenv("SENTRA_SERVER_URL"))
	if v != "" {
		return validateServerURL(v)
	}

	// Check saved config first to honor user's previous choice.
	// This prevents PORT from unrelated tools (CI, other services) from overriding saved config.
	cfg, ok, err := auth.LoadConfig()
	if err == nil && ok {
		if saved := strings.TrimSpace(cfg.ServerURL); saved != "" {
			// Safety: never get stuck on a saved localhost URL.
			if u, perr := url.Parse(saved); perr == nil {
				if isLoopbackHost(u.Hostname()) {
					cfg.ServerURL = ""
					_ = auth.SaveConfig(cfg)
				} else {
					return validateServerURL(saved)
				}
			} else {
				return validateServerURL(saved)
			}
		}
	}

	// Local dev convenience: if PORT is explicitly provided and no saved config exists,
	// use localhost. This allows local development without requiring SENTRA_SERVER_URL.
	if port := strings.TrimSpace(os.Getenv("PORT")); port != "" {
		return "http://127.0.0.1:" + port, nil
	}

	// Default for end-users.
	return defaultHostedServerURL, nil
}

func validateServerURL(v string) (string, error) {
	u, err := url.Parse(v)
	if err != nil {
		return "", fmt.Errorf("invalid server URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("invalid server URL scheme: %s", u.Scheme)
	}
	host := u.Hostname()
	if host == "" {
		return "", fmt.Errorf("invalid server URL host")
	}

	if u.Scheme == "http" && !isLoopbackHost(host) {
		return "", fmt.Errorf("insecure connection: HTTP is only allowed for localhost connections")
	}

	return strings.TrimRight(v, "/"), nil
}

func isLoopbackHost(host string) bool {
	host = strings.TrimSpace(strings.ToLower(host))
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	if ip != nil {
		return ip.IsLoopback()
	}
	return false
}
