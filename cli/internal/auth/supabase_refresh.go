package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

func (s SupabaseOAuth) Refresh(ctx context.Context, refreshToken string) (TokenResponse, error) {
	base := strings.TrimRight(strings.TrimSpace(s.SupabaseURL), "/")
	if base == "" {
		return TokenResponse{}, fmt.Errorf("SUPABASE_URL is required")
	}
	if strings.TrimSpace(s.AnonKey) == "" {
		return TokenResponse{}, fmt.Errorf("SUPABASE_ANON_KEY is required")
	}
	if strings.TrimSpace(refreshToken) == "" {
		return TokenResponse{}, fmt.Errorf("refresh token is required")
	}
	if s.HTTPClient == nil {
		s.HTTPClient = &http.Client{Timeout: 10 * time.Second}
	}

	u := base + "/auth/v1/token?grant_type=refresh_token"
	payload := map[string]any{
		"refresh_token": refreshToken,
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
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return TokenResponse{}, fmt.Errorf("refresh failed: status=%d body=%s", resp.StatusCode, string(respBody))
	}

	var out TokenResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		return TokenResponse{}, err
	}
	if out.AccessToken == "" {
		return TokenResponse{}, fmt.Errorf("refresh failed: empty access_token")
	}

	return out, nil
}
