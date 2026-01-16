package auth

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

type AccessTokenClaims struct {
	Sub   string `json:"sub"`
	Email string `json:"email,omitempty"`
	Exp   int64  `json:"exp,omitempty"`
}

func ParseAccessTokenClaims(jwtToken string) (AccessTokenClaims, error) {
	jwtToken = strings.TrimSpace(jwtToken)
	parts := strings.Split(jwtToken, ".")
	if len(parts) < 2 {
		return AccessTokenClaims{}, fmt.Errorf("invalid jwt")
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return AccessTokenClaims{}, err
	}

	var c AccessTokenClaims
	if err := json.Unmarshal(payloadBytes, &c); err != nil {
		return AccessTokenClaims{}, err
	}
	if c.Sub == "" {
		return AccessTokenClaims{}, fmt.Errorf("jwt missing sub")
	}

	return c, nil
}
