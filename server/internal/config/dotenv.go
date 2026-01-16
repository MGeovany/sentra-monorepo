package config

import "github.com/joho/godotenv"

// LoadDotEnv loads environment variables from .env files if present.
// It is safe to call in production: missing files are ignored.
func LoadDotEnv() {
	// Support running from repo root or from within `server/`.
	// `godotenv.Load(a, b)` stops on the first missing file, so try separately.
	// If the variable already exists in the environment, it will not be overridden.
	_ = godotenv.Load(".env")
	_ = godotenv.Load("../.env")
}
