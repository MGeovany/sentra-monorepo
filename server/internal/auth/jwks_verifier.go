package auth

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var ErrAuthNotConfigured = errors.New("auth not configured")

type User struct {
	ID    string `json:"id"`
	Email string `json:"email,omitempty"`
	Role  string `json:"role,omitempty"`
}

type Verifier interface {
	Verify(token string) (User, error)
}

type DisabledVerifier struct{}

func (DisabledVerifier) Verify(token string) (User, error) {
	return User{}, ErrAuthNotConfigured
}

type supabaseClaims struct {
	jwt.RegisteredClaims
	Email string `json:"email,omitempty"`
	Role  string `json:"role,omitempty"`
}

type JWKSVerifier struct {
	jwksURL          string
	expectedIssuer   string
	expectedAudience string

	mu        sync.RWMutex
	keys      map[string]any
	lastFetch time.Time

	httpClient *http.Client
}

func NewJWKSVerifier(supabaseURL string) *JWKSVerifier {
	base := strings.TrimRight(strings.TrimSpace(supabaseURL), "/")
	jwksURL := ""
	issuer := ""
	if base != "" {
		jwksURL = base + "/auth/v1/.well-known/jwks.json"
		issuer = base + "/auth/v1"
	}

	return &JWKSVerifier{
		jwksURL:          jwksURL,
		expectedIssuer:   issuer,
		expectedAudience: "authenticated",
		keys:             map[string]any{},
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (v *JWKSVerifier) Verify(tokenString string) (User, error) {
	if v.jwksURL == "" {
		return User{}, ErrAuthNotConfigured
	}

	// Fast path: parse with existing keys.
	claims := &supabaseClaims{}
	parser := jwt.NewParser(jwt.WithValidMethods([]string{"RS256", "ES256"}))
	parsed, err := parser.ParseWithClaims(tokenString, claims, v.keyFunc)
	if err == nil && parsed != nil && parsed.Valid {
		if err := v.validateClaims(claims); err != nil {
			return User{}, err
		}
		return User{ID: claims.Subject, Email: claims.Email, Role: claims.Role}, nil
	}

	// If key not found / stale keys, refresh once.
	if refreshErr := v.refresh(); refreshErr != nil {
		return User{}, err
	}

	claims = &supabaseClaims{}
	parsed, err = parser.ParseWithClaims(tokenString, claims, v.keyFunc)
	if err != nil {
		return User{}, err
	}
	if parsed == nil || !parsed.Valid {
		return User{}, fmt.Errorf("invalid token")
	}

	if err := v.validateClaims(claims); err != nil {
		return User{}, err
	}

	return User{ID: claims.Subject, Email: claims.Email, Role: claims.Role}, nil
}

func (v *JWKSVerifier) validateClaims(c *supabaseClaims) error {
	if c == nil {
		return fmt.Errorf("invalid token claims")
	}
	if strings.TrimSpace(c.Subject) == "" {
		return fmt.Errorf("invalid token: missing sub")
	}
	if v.expectedIssuer != "" && c.Issuer != v.expectedIssuer {
		return fmt.Errorf("invalid token: unexpected iss")
	}
	if v.expectedAudience != "" {
		ok := false
		for _, aud := range c.Audience {
			if aud == v.expectedAudience {
				ok = true
				break
			}
		}
		if !ok {
			return fmt.Errorf("invalid token: unexpected aud")
		}
	}
	return nil
}

func (v *JWKSVerifier) keyFunc(token *jwt.Token) (any, error) {
	kid, _ := token.Header["kid"].(string)
	if kid == "" {
		return nil, fmt.Errorf("missing kid")
	}

	v.mu.RLock()
	k, ok := v.keys[kid]
	v.mu.RUnlock()
	if !ok || k == nil {
		return nil, fmt.Errorf("unknown kid")
	}

	return k, nil
}

type jwksPayload struct {
	Keys []jwk `json:"keys"`
}

type jwk struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	Alg string `json:"alg"`
	Use string `json:"use"`

	// RSA
	N string `json:"n"`
	E string `json:"e"`

	// EC
	Crv string `json:"crv"`
	X   string `json:"x"`
	Y   string `json:"y"`
}

func (v *JWKSVerifier) refresh() error {
	// Simple throttling.
	v.mu.RLock()
	last := v.lastFetch
	v.mu.RUnlock()
	if !last.IsZero() && time.Since(last) < 30*time.Second {
		return nil
	}

	resp, err := v.httpClient.Get(v.jwksURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("jwks fetch failed: %s", resp.Status)
	}

	var p jwksPayload
	if err := json.NewDecoder(resp.Body).Decode(&p); err != nil {
		return err
	}

	keys := make(map[string]any, len(p.Keys))
	for _, k := range p.Keys {
		if k.Kid == "" {
			continue
		}

		switch k.Kty {
		case "RSA":
			if k.N == "" || k.E == "" {
				continue
			}
			pub, err := jwkToRSAPublicKey(k.N, k.E)
			if err != nil {
				continue
			}
			keys[k.Kid] = pub
		case "EC":
			// Supabase currently publishes ES256 (P-256).
			if k.Crv != "P-256" || k.X == "" || k.Y == "" {
				continue
			}
			pub, err := jwkToECPublicKey(k.X, k.Y)
			if err != nil {
				continue
			}
			keys[k.Kid] = pub
		}
	}

	v.mu.Lock()
	v.keys = keys
	v.lastFetch = time.Now()
	v.mu.Unlock()

	return nil
}

func jwkToRSAPublicKey(nB64, eB64 string) (*rsa.PublicKey, error) {
	nBytes, err := base64.RawURLEncoding.DecodeString(nB64)
	if err != nil {
		return nil, err
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(eB64)
	if err != nil {
		return nil, err
	}

	n := new(big.Int).SetBytes(nBytes)
	e := 0
	for _, b := range eBytes {
		e = e<<8 + int(b)
	}
	if e == 0 {
		return nil, fmt.Errorf("invalid exponent")
	}

	return &rsa.PublicKey{N: n, E: e}, nil
}

func jwkToECPublicKey(xB64, yB64 string) (*ecdsa.PublicKey, error) {
	xBytes, err := base64.RawURLEncoding.DecodeString(xB64)
	if err != nil {
		return nil, err
	}
	yBytes, err := base64.RawURLEncoding.DecodeString(yB64)
	if err != nil {
		return nil, err
	}

	x := new(big.Int).SetBytes(xBytes)
	y := new(big.Int).SetBytes(yBytes)

	curve := elliptic.P256()
	if !curve.IsOnCurve(x, y) {
		return nil, fmt.Errorf("point not on curve")
	}

	return &ecdsa.PublicKey{Curve: curve, X: x, Y: y}, nil
}
