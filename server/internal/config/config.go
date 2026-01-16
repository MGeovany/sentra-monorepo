package config

import "os"

type Config struct {
	Port string

	SupabaseURL            string
	SupabaseServiceRoleKey string
	SupabaseDummyTable     string
}

func FromEnv() Config {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	table := os.Getenv("SUPABASE_DUMMY_TABLE")
	if table == "" {
		table = "sentra_health_checks"
	}

	return Config{
		Port: port,

		SupabaseURL:            os.Getenv("SUPABASE_URL"),
		SupabaseServiceRoleKey: os.Getenv("SUPABASE_SERVICE_ROLE_KEY"),
		SupabaseDummyTable:     table,
	}
}
