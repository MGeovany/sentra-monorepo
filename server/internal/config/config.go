package config

import (
	"os"
)

type Config struct {
	Port string
	Host string

	SupabaseURL            string
	SupabaseServiceRoleKey string
	SupabaseMachinesTable  string
}

func FromEnv() Config {
	port := os.Getenv("SERVER_PORT")
	if port == "" {
		port = "8080"
	}

	host := os.Getenv("SERVER_HOST")
	if host == "" {
		// Secure default: bind only to loopback.
		host = "127.0.0.1"
	}

	machinesTable := os.Getenv("SUPABASE_MACHINES_TABLE")
	if machinesTable == "" {
		machinesTable = "machines"
	}

	return Config{
		Port: port,
		Host: host,

		SupabaseURL:            os.Getenv("SUPABASE_URL"),
		SupabaseServiceRoleKey: os.Getenv("SUPABASE_SERVICE_ROLE_KEY"),
		SupabaseMachinesTable:  machinesTable,
	}
}
