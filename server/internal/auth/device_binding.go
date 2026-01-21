package auth

import (
	"crypto/ed25519"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	deviceSigVersion = "v1"
)

func canonicalDeviceMessage(machineID, timestamp, method, path string, body []byte) []byte {
	// Keep this canonical and stable across versions.
	// Format: v1\n<ts>\n<METHOD>\n<path>\n<machine_id>\n<body>
	b := make([]byte, 0, 64+len(body))
	b = append(b, deviceSigVersion...)
	b = append(b, '\n')
	b = append(b, timestamp...)
	b = append(b, '\n')
	b = append(b, strings.ToUpper(method)...)
	b = append(b, '\n')
	b = append(b, path...)
	b = append(b, '\n')
	b = append(b, machineID...)
	b = append(b, '\n')
	b = append(b, body...)
	return b
}

func VerifyDeviceSignature(devicePubKeyB64, machineID, timestamp, method, path string, body []byte, sigB64 string) error {
	devicePubKeyB64 = strings.TrimSpace(devicePubKeyB64)
	sigB64 = strings.TrimSpace(sigB64)
	machineID = strings.TrimSpace(machineID)
	timestamp = strings.TrimSpace(timestamp)
	if devicePubKeyB64 == "" || sigB64 == "" || machineID == "" || timestamp == "" {
		return errors.New("missing device signature fields")
	}

	tsInt, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return errors.New("invalid timestamp")
	}
	ts := time.Unix(tsInt, 0).UTC()
	// Prevent replays: accept only a small window.
	if d := time.Since(ts); d < -30*time.Second || d > 5*time.Minute {
		return errors.New("stale timestamp")
	}

	pubRaw, err := base64.RawURLEncoding.DecodeString(devicePubKeyB64)
	if err != nil {
		return fmt.Errorf("invalid device pubkey: %w", err)
	}
	if len(pubRaw) != ed25519.PublicKeySize {
		return errors.New("invalid device pubkey length")
	}
	pub := ed25519.PublicKey(pubRaw)

	sigRaw, err := base64.RawURLEncoding.DecodeString(sigB64)
	if err != nil {
		return fmt.Errorf("invalid signature: %w", err)
	}
	if len(sigRaw) != ed25519.SignatureSize {
		return errors.New("invalid signature length")
	}

	msg := canonicalDeviceMessage(machineID, timestamp, method, path, body)
	if subtle.ConstantTimeByteEq(boolToByte(ed25519.Verify(pub, msg, sigRaw)), 1) != 1 {
		return errors.New("invalid signature")
	}
	return nil
}

func boolToByte(b bool) byte {
	if b {
		return 1
	}
	return 0
}
