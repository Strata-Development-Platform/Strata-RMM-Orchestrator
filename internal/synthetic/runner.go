package synthetic

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

const maxResponseBytes = 1 << 20

type Config struct {
	BaseURL  string
	Email    string
	Password string
	TenantID string
	Timeout  time.Duration
}

type Result struct {
	Name       string        `json:"name"`
	Success    bool          `json:"success"`
	StatusCode int           `json:"status_code,omitempty"`
	Duration   time.Duration `json:"duration"`
	Error      string        `json:"error,omitempty"`
}

type Runner struct {
	config Config
	client *http.Client
}

func New(config Config) (*Runner, error) {
	base, err := url.Parse(config.BaseURL)
	if err != nil || base.Scheme == "" || base.Host == "" {
		return nil, fmt.Errorf("synthetic base URL is invalid")
	}
	if base.Scheme != "https" && base.Hostname() != "localhost" && base.Hostname() != "127.0.0.1" {
		return nil, fmt.Errorf("synthetic base URL must use HTTPS")
	}
	if base.User != nil || base.RawQuery != "" || (base.Path != "" && base.Path != "/") {
		return nil, fmt.Errorf("synthetic base URL must not contain credentials, query parameters, or a path")
	}
	if config.Email == "" || config.Password == "" || config.TenantID == "" {
		return nil, fmt.Errorf("synthetic email, password, and tenant ID are required")
	}
	if config.Timeout <= 0 {
		config.Timeout = 10 * time.Second
	}
	config.BaseURL = strings.TrimRight(config.BaseURL, "/")
	return &Runner{config: config, client: &http.Client{
		Timeout: config.Timeout,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}}, nil
}

func (r *Runner) Run(ctx context.Context) []Result {
	results := []Result{
		r.get(ctx, "public_liveness", "/health/live", ""),
		r.get(ctx, "public_readiness", "/health/ready", ""),
	}
	token, login := r.login(ctx)
	results = append(results, login)
	if !login.Success {
		return results
	}
	results = append(results,
		r.get(ctx, "authenticated_api", "/api/v1/auth/me", token),
		r.get(ctx, "agent_path", "/api/v2/devices", token),
		r.get(ctx, "storage_path", "/api/v1/recordings/"+url.PathEscape(r.config.TenantID), token),
	)
	return results
}

func (r *Runner) login(ctx context.Context) (string, Result) {
	body, err := json.Marshal(map[string]string{"email": r.config.Email, "password": r.config.Password})
	if err != nil {
		return "", Result{Name: "login", Error: "encode request"}
	}
	started := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.config.BaseURL+"/api/v1/auth/login", bytes.NewReader(body))
	if err != nil {
		return "", Result{Name: "login", Duration: time.Since(started), Error: "build request"}
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := r.client.Do(req)
	if err != nil {
		return "", Result{Name: "login", Duration: time.Since(started), Error: "request failed"}
	}
	defer resp.Body.Close()
	result := Result{Name: "login", StatusCode: resp.StatusCode, Duration: time.Since(started)}
	if resp.StatusCode != http.StatusOK {
		result.Error = "unexpected status"
		return "", result
	}
	var payload struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(&payload); err != nil || payload.Token == "" {
		result.Error = "invalid login response"
		return "", result
	}
	result.Success = true
	return payload.Token, result
}

func (r *Runner) get(ctx context.Context, name, path, token string) Result {
	started := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.config.BaseURL+path, nil)
	if err != nil {
		return Result{Name: name, Duration: time.Since(started), Error: "build request"}
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := r.client.Do(req)
	if err != nil {
		return Result{Name: name, Duration: time.Since(started), Error: "request failed"}
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxResponseBytes))
	result := Result{Name: name, StatusCode: resp.StatusCode, Duration: time.Since(started)}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		result.Error = "unexpected status"
		return result
	}
	result.Success = true
	return result
}
