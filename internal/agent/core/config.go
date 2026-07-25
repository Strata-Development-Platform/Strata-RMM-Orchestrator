package core

import (
	"fmt"
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
	TenantID        string `yaml:"tenant_id"`
	AgentID         string `yaml:"agent_id"`
	DeploymentID    string `yaml:"deployment_id"`
	EnrollmentToken string `yaml:"enrollment_token"`
	RegisterURL     string `yaml:"register_url"`
	LogLevel        string `yaml:"log_level"`
	DataDir         string `yaml:"data_dir"`
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
	Type string `yaml:"type"`
	Path string `yaml:"path"`
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
			Type: "bbolt",
			Path: filepath.Join(defaultDataDir(), "agent.db"),
		},
		Update: UpdateConfig{
			Enabled:       true,
			CheckInterval: 24 * time.Hour,
			Channel:       "stable",
			ManifestURL:   "https://releases.strata-rmm.io",
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
	return c.Validate()
}
