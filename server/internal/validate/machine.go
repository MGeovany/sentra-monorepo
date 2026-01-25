package validate

import (
	"errors"
	"regexp"
	"strings"
)

const (
	// Keep these tight to avoid DoS/log injection and keep indexes sane.
	maxMachineIDLen    = 64
	maxMachineNameLen  = 128
	maxDevicePubKeyLen = 256
)

var (
	// Allow UUIDs and other safe, ASCII identifiers.
	// Disallow whitespace and control characters.
	machineIDRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)
)

func MachineID(s string) error {
	s = strings.TrimSpace(s)
	if s == "" {
		return errors.New("missing machine_id")
	}
	if len(s) > maxMachineIDLen {
		return errors.New("machine_id too long")
	}
	if !machineIDRe.MatchString(s) {
		return errors.New("invalid machine_id")
	}
	return nil
}

func MachineName(s string) error {
	s = strings.TrimSpace(s)
	if s == "" {
		return errors.New("missing machine_name")
	}
	if len(s) > maxMachineNameLen {
		return errors.New("machine_name too long")
	}
	// Block log injection / weird separators.
	for i := 0; i < len(s); i++ {
		if s[i] < 0x20 || s[i] == 0x7f {
			return errors.New("invalid machine_name")
		}
	}
	return nil
}

func DevicePubKeyB64(s string) error {
	s = strings.TrimSpace(s)
	if s == "" {
		return errors.New("missing device_pub_key")
	}
	if len(s) > maxDevicePubKeyLen {
		return errors.New("device_pub_key too long")
	}
	for i := 0; i < len(s); i++ {
		// base64url charset: A-Z a-z 0-9 - _
		c := s[i]
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' || c == '_' {
			continue
		}
		return errors.New("invalid device_pub_key")
	}
	return nil
}
