package core

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Agent   AgentConfig   `yaml:"agent"`
	NATS    NATSConfig    `yaml:"nats"`
	Collect CollectConfig `yaml:"collect"`
	Store   StoreConfig   `yaml:"store"`
	Update  UpdateConfig  `yaml:"update"`
}

type AgentConfig struct {
	TenantID        string            `yaml:"tenant_id"`
	AgentID         string            `yaml:"agent_id"`
	DeploymentID    string            `yaml:"deployment_id,omitempty"`
	EnrollmentToken string            `yaml:"enrollment_token,omitempty"`
	RegisterURL     string            `yaml:"register_url"`
	LogLevel        string            `yaml:"log_level"`
	DataDir         string            `yaml:"data_dir"`
	Tags            map[string]string `yaml:"tags"`
}

type NATSConfig struct {
	URLs          []string      `yaml:"urls"`
	Token         string        `yaml:"token"`
	CertFile      string        `yaml:"cert_file"`
	KeyFile       string        `yaml:"key_file"`
	CAFile        string        `yaml:"ca_file"`
	ReconnectWait time.Duration `yaml:"reconnect_wait"`
	MaxReconnects int           `yaml:"max_reconnects"`
}

type CollectConfig struct {
	Interval     time.Duration `yaml:"interval"`
	EnableSystem bool          `yaml:"enable_system"`
	EnableHW     bool          `yaml:"enable_hardware"`
	EnableSW     bool          `yaml:"enable_software"`
	EnableNet    bool          `yaml:"enable_network"`
	EnableSvc    bool          `yaml:"enable_services"`
}

type StoreConfig struct {
	Type          string `yaml:"type"`
	Path          string `yaml:"path"`
	QueueMaxItems int    `yaml:"queue_max_items"`
}

type UpdateConfig struct {
	Enabled       bool          `yaml:"enabled"`
	CheckInterval time.Duration `yaml:"check_interval"`
	Channel       string        `yaml:"channel"`
	ManifestURL   string        `yaml:"manifest_url"`
	VerifyKey     string        `yaml:"verify_key"`
}

func DefaultConfig() *Config {
	return &Config{
		Agent: AgentConfig{
			LogLevel: "info",
			DataDir:  defaultDataDir(),
		},
		NATS: NATSConfig{
			URLs:          []string{"nats://localhost:4222"},
			ReconnectWait: 5 * time.Second,
			MaxReconnects: -1,
		},
		Collect: CollectConfig{
			Interval:     60 * time.Second,
			EnableSystem: true,
			EnableHW:     true,
			EnableSW:     true,
			EnableNet:    true,
			EnableSvc:    true,
		},
		Store: StoreConfig{
			Type:          "bbolt",
			Path:          filepath.Join(defaultDataDir(), "agent.db"),
			QueueMaxItems: 10000,
		},
		Update: UpdateConfig{
			Enabled:       true,
			CheckInterval: 24 * time.Hour,
			Channel:       "stable",
			ManifestURL:   "https://releases.example.com", // Set STRATA_MANIFEST_URL env var or configure in agent.yaml
		},
	}
}

func defaultDataDir() string {
	if d := os.Getenv("STRATA_RMM_DATA_DIR"); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "/var/lib/strata-rmm"
	}
	return filepath.Join(home, ".strata-rmm")
}

func (c *Config) Validate() error {
	if c.Agent.TenantID == "" {
		return fmt.Errorf("agent.tenant_id is required")
	}
	if len(c.NATS.URLs) == 0 {
		return fmt.Errorf("nats.urls is required")
	}
	for _, raw := range c.NATS.URLs {
		u, err := url.Parse(raw)
		if err != nil || u.Host == "" {
			return fmt.Errorf("nats.urls contains an invalid URL")
		}
		if u.Scheme == "nats" && u.Hostname() != "localhost" && u.Hostname() != "127.0.0.1" {
			return fmt.Errorf("nats.urls must use encrypted transport outside local development")
		}
		if u.Scheme != "nats" && u.Scheme != "tls" && u.Scheme != "nats+tls" {
			return fmt.Errorf("nats.urls contains an unsupported scheme")
		}
	}
	if c.NATS.Token == "" && c.NATS.CertFile == "" {
		return fmt.Errorf("nats token or client certificate is required")
	}
	if (c.NATS.CertFile == "") != (c.NATS.KeyFile == "") {
		return fmt.Errorf("nats client certificate and key must be configured together")
	}
	if c.Collect.Interval < time.Second {
		return fmt.Errorf("collect.interval must be at least 1s")
	}
	if c.Store.QueueMaxItems <= 0 {
		return fmt.Errorf("store.queue_max_items must be greater than zero")
	}
	return nil
}

// ValidateBootstrap accepts either an already-enrolled runtime configuration or
// a one-time enrollment configuration. Runtime validation remains strict and is
// performed after registration has populated the tenant and messaging identity.
func (c *Config) ValidateBootstrap() error {
	if c.Store.QueueMaxItems <= 0 {
		return fmt.Errorf("store.queue_max_items must be greater than zero")
	}
	if c.Agent.TenantID != "" && c.Agent.EnrollmentToken != "" {
		return fmt.Errorf("agent.enrollment_token cannot be combined with agent.tenant_id; orchestrator registration is required")
	}
	if c.Agent.TenantID != "" {
		return c.Validate()
	}
	if c.Agent.EnrollmentToken == "" && c.Agent.DeploymentID == "" {
		return fmt.Errorf("agent.enrollment_token is required before enrollment")
	}
	if c.Agent.RegisterURL == "" {
		return fmt.Errorf("agent.register_url is required before enrollment")
	}
	u, err := url.Parse(c.Agent.RegisterURL)
	if err != nil || u.Host == "" {
		return fmt.Errorf("agent.register_url must be an absolute URL")
	}
	if u.Scheme != "https" && u.Hostname() != "localhost" && u.Hostname() != "127.0.0.1" {
		return fmt.Errorf("agent.register_url must use HTTPS outside local development")
	}
	if c.Collect.Interval < time.Second {
		return fmt.Errorf("collect.interval must be at least 1s")
	}
	return nil
}

func (c *Config) Load(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading config: %w", err)
	}
	if err := yaml.Unmarshal(data, c); err != nil {
		return fmt.Errorf("parsing config: %w", err)
	}
	return c.ValidateBootstrap()
}

// SaveRuntime atomically replaces a bootstrap configuration with the enrolled
// runtime configuration. One-time enrollment material is deliberately omitted.
func (c *Config) SaveRuntime(path string) error {
	if err := c.Validate(); err != nil {
		return err
	}
	runtimeConfig := *c
	runtimeConfig.Agent.EnrollmentToken = ""
	runtimeConfig.Agent.DeploymentID = ""
	data, err := yaml.Marshal(&runtimeConfig)
	if err != nil {
		return fmt.Errorf("encoding runtime config: %w", err)
	}
	tempPath := path + ".tmp"
	if err := os.WriteFile(tempPath, data, 0600); err != nil {
		return fmt.Errorf("writing runtime config: %w", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("replacing bootstrap config: %w", err)
	}
	return nil
}
