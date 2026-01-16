package auth

import "github.com/joho/godotenv"

// LoadDotEnv loads local .env configuration for the CLI.
// Missing file is ignored.
func LoadDotEnv() {
	// Support running from repo root or from within `cli/`.
	// `godotenv.Load(a, b)` stops on the first missing file, so try separately.
	_ = godotenv.Load(".env")
	_ = godotenv.Load("../.env")
}
