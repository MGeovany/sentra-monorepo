package config

import (
	"os"
)

type Config struct {
	Port string
	Host string

	SupabaseURL            string
	SupabaseServiceRoleKey string
}

func FromEnv() Config {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	host := os.Getenv("SERVER_HOST")
	if host == "" {
		// Secure default: bind only to loopback.
		// Cloud Run (and most managed platforms) require binding to 0.0.0.0.
		if os.Getenv("PORT") != "" {
			host = "0.0.0.0"
		} else {
			host = "127.0.0.1"
		}
	}

	return Config{
		Port: port,
		Host: host,

		SupabaseURL:            os.Getenv("SUPABASE_URL"),
		SupabaseServiceRoleKey: os.Getenv("SUPABASE_SERVICE_ROLE_KEY"),
	}
}
