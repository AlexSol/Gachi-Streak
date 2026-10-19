package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/AlexSol/gachi-streak/internal/api"
)

// APIClient talks to gachi-server.
type APIClient struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
}

// NewAPIClient builds a client from config.
func NewAPIClient(cfg Config) *APIClient {
	return &APIClient{
		BaseURL: strings.TrimRight(cfg.ServerURL, "/"),
		Token:   cfg.Token,
		HTTP:    &http.Client{Timeout: 15 * time.Second},
	}
}

// Status calls GET /status.
func (c *APIClient) Status(ctx context.Context) (api.StatusResponse, error) {
	var out api.StatusResponse
	return out, c.do(ctx, http.MethodGet, "/status", nil, &out)
}

// Ping calls POST /ping.
func (c *APIClient) Ping(ctx context.Context, note string) (api.PingResponse, error) {
	var out api.PingResponse
	body, _ := json.Marshal(api.PingRequest{Note: note})
	return out, c.do(ctx, http.MethodPost, "/ping", body, &out)
}

func (c *APIClient) do(ctx context.Context, method, path string, body []byte, out any) error {
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		var e api.Error
		if json.Unmarshal(data, &e) == nil && e.Error != "" {
			return fmt.Errorf("server: %s (%d)", e.Error, resp.StatusCode)
		}
		return fmt.Errorf("server: %s", resp.Status)
	}
	return json.Unmarshal(data, out)
}

// FindMe returns the caller's own status from a /status response.
func FindMe(s api.StatusResponse) (api.UserStatus, bool) {
	for _, u := range s.Users {
		if u.Name == s.Me {
			return u, true
		}
	}
	return api.UserStatus{}, false
}
