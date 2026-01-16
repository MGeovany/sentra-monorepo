package config

import "os"

type Config struct {
	Port string

	SupabaseURL            string
	SupabaseServiceRoleKey string
	SupabaseMachinesTable  string
}

func FromEnv() Config {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	machinesTable := os.Getenv("SUPABASE_MACHINES_TABLE")
	if machinesTable == "" {
		machinesTable = "machines"
	}

	return Config{
		Port: port,

		SupabaseURL:            os.Getenv("SUPABASE_URL"),
		SupabaseServiceRoleKey: os.Getenv("SUPABASE_SERVICE_ROLE_KEY"),
		SupabaseMachinesTable:  machinesTable,
	}
}
