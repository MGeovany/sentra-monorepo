package cli

import (
	"os"
	"strings"
)

func isTruthyEnv(key string) bool {
	v := strings.TrimSpace(os.Getenv(key))
	return v == "1" || strings.EqualFold(v, "true") || strings.EqualFold(v, "yes")
}

// isVerbose controls verbose logging.
//
// Historical env vars:
// - SENTRA_VERBOSE=1|true
// - SENTRA_LOG=debug|trace|verbose
func isVerbose() bool {
	if isTruthyEnv("SENTRA_VERBOSE") || isTruthyEnv("SENTRA_DEBUG") {
		return true
	}

	lvl := strings.ToLower(strings.TrimSpace(os.Getenv("SENTRA_LOG")))
	if lvl == "" {
		lvl = strings.ToLower(strings.TrimSpace(os.Getenv("SENTRA_LOG_LEVEL")))
	}
	if lvl == "1" || lvl == "true" {
		return true
	}
	switch lvl {
	case "debug", "trace", "verbose":
		return true
	default:
		return false
	}
}
