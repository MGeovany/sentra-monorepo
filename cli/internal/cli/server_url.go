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
	v := strings.TrimSpace(os.Getenv("SENTRA_SERVER_URL"))
	if v != "" {
		return validateServerURL(v)
	}

	// If user already logged in before, keep using the saved server URL.
	if cfg, ok, err := auth.LoadConfig(); err == nil && ok {
		if saved := strings.TrimSpace(cfg.ServerURL); saved != "" {
			return validateServerURL(saved)
		}
	}

	// Local dev convenience: if explicitly provided, use localhost.
	if port := strings.TrimSpace(os.Getenv("SERVER_PORT")); port != "" {
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
