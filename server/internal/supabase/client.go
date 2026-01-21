package supabase

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

type Client struct {
	baseURL    *url.URL
	httpClient *http.Client
	apiKey     string
}

func (c *Client) APIKey() string {
	return c.apiKey
}

func (c *Client) Do(req *http.Request) (*http.Response, error) {
	return c.httpClient.Do(req)
}

func New(baseURL, apiKey string) (*Client, error) {
	baseURL = strings.TrimSpace(baseURL)
	apiKey = strings.TrimSpace(apiKey)
	if baseURL == "" {
		return nil, fmt.Errorf("SUPABASE_URL is required")
	}
	if apiKey == "" {
		return nil, fmt.Errorf("SUPABASE_SERVICE_ROLE_KEY is required")
	}

	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, err
	}

	return &Client{
		baseURL: u,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		apiKey: apiKey,
	}, nil
}

func (c *Client) PostgRESTURL(path string) string {
	// e.g. https://xxxx.supabase.co/rest/v1/<table>
	u := *c.baseURL
	u.Path = strings.TrimSuffix(u.Path, "/") + "/rest/v1/" + strings.TrimPrefix(path, "/")
	return u.String()
}

func (c *Client) PostJSON(ctx context.Context, url string, body any, headers map[string]string) (*http.Response, []byte, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return nil, nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return nil, nil, err
	}

	// Supabase requirements for REST
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("apikey", c.apiKey)
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return resp, nil, readErr
	}

	return resp, respBody, nil
}
