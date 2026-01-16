package auth

// SetUserID stores the authenticated user id in config.
// It keeps existing machine_id stable.
func SetUserID(userID string) error {
	if userID == "" {
		return nil
	}

	cfg, _, err := LoadConfig()
	if err != nil {
		return err
	}

	// Ensure config exists.
	if cfg.MachineID == "" {
		var ensureErr error
		cfg, ensureErr = EnsureConfig()
		if ensureErr != nil {
			return ensureErr
		}
	}

	if cfg.UserID == userID {
		return nil
	}

	cfg.UserID = userID
	return SaveConfig(cfg)
}
