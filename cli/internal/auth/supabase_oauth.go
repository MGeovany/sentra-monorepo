package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type SupabaseOAuth struct {
	SupabaseURL string
	AnonKey     string
	Provider    string
	HTTPClient  *http.Client
}

type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
}

func (s SupabaseOAuth) AuthorizeURL(redirectTo, codeChallenge string) (string, error) {
	base := strings.TrimRight(strings.TrimSpace(s.SupabaseURL), "/")
	if base == "" {
		return "", fmt.Errorf("SUPABASE_URL is required")
	}
	if strings.TrimSpace(s.AnonKey) == "" {
		return "", fmt.Errorf("SUPABASE_ANON_KEY is required")
	}
	provider := s.Provider
	if provider == "" {
		provider = "github"
	}

	u, err := url.Parse(base + "/auth/v1/authorize")
	if err != nil {
		return "", err
	}

	q := u.Query()
	q.Set("provider", provider)
	q.Set("redirect_to", redirectTo)
	// Required by Supabase to enable PKCE flow.
	q.Set("flow_type", "pkce")
	q.Set("code_challenge", codeChallenge)
	q.Set("code_challenge_method", "s256")
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func (s SupabaseOAuth) ExchangePKCE(ctx context.Context, authCode, codeVerifier string) (TokenResponse, error) {
	base := strings.TrimRight(strings.TrimSpace(s.SupabaseURL), "/")
	if base == "" {
		return TokenResponse{}, fmt.Errorf("SUPABASE_URL is required")
	}
	if strings.TrimSpace(s.AnonKey) == "" {
		return TokenResponse{}, fmt.Errorf("SUPABASE_ANON_KEY is required")
	}
	if s.HTTPClient == nil {
		s.HTTPClient = &http.Client{Timeout: 10 * time.Second}
	}

	u := base + "/auth/v1/token?grant_type=pkce"
	payload := map[string]any{
		"auth_code":     authCode,
		"code_verifier": codeVerifier,
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return TokenResponse{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(b))
	if err != nil {
		return TokenResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("apikey", s.AnonKey)
	req.Header.Set("Authorization", "Bearer "+s.AnonKey)

	resp, err := s.HTTPClient.Do(req)
	if err != nil {
		return TokenResponse{}, err
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return TokenResponse{}, fmt.Errorf("token exchange failed: status=%d body=%s", resp.StatusCode, string(respBody))
	}

	var out TokenResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		return TokenResponse{}, err
	}
	if out.AccessToken == "" {
		return TokenResponse{}, fmt.Errorf("token exchange failed: empty access_token")
	}

	return out, nil
}
