package auth

import "os"

func removeLegacySessionFiles() error {
	// Best-effort cleanup; ignore errors where practical.
	if p, err := sessionPath(); err == nil {
		_ = os.Remove(p)
	}
	if p, err := sessionKeyPath(); err == nil {
		_ = os.Remove(p)
	}
	return nil
}
