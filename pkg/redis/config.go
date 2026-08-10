package redis

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

type PoolConfig struct {
	URL          string
	PoolSize     int
	MinIdleConns int
	MaxRetries   int
	DialTimeout  time.Duration
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

type Client struct {
	rdb *redis.Client
	cfg PoolConfig
}

func NewClient(ctx context.Context, cfg PoolConfig) (*Client, error) {
	opts, err := optionsFromPoolConfig(cfg)
	if err != nil {
		return nil, err
	}

	rdb := redis.NewClient(opts)
	if err := rdb.Ping(ctx).Err(); err != nil {
		_ = rdb.Close()
		return nil, fmt.Errorf("pinging redis: %w", err)
	}

	return &Client{rdb: rdb, cfg: cfg}, nil
}

func optionsFromPoolConfig(cfg PoolConfig) (*redis.Options, error) {
	if cfg.PoolSize <= 0 {
		cfg.PoolSize = 10
	}
	if cfg.MinIdleConns < 0 {
		cfg.MinIdleConns = 2
	}
	if cfg.MaxRetries <= 0 {
		cfg.MaxRetries = 3
	}
	if cfg.DialTimeout <= 0 {
		cfg.DialTimeout = 5 * time.Second
	}
	if cfg.ReadTimeout <= 0 {
		cfg.ReadTimeout = 3 * time.Second
	}
	if cfg.WriteTimeout <= 0 {
		cfg.WriteTimeout = 3 * time.Second
	}

	raw := strings.TrimSpace(cfg.URL)
	if raw == "" {
		return nil, fmt.Errorf("redis URL is required")
	}

	var opts *redis.Options
	if !strings.Contains(raw, "://") {
		opts = &redis.Options{Addr: raw}
	} else {
		u, err := url.Parse(raw)
		if err != nil {
			return nil, fmt.Errorf("parse redis URL: %w", err)
		}
		switch u.Scheme {
		case "redis", "rediss":
			opts, err = redis.ParseURL(raw)
			if err != nil {
				return nil, fmt.Errorf("parse redis URL: %w", err)
			}
		case "tcp", "tls":
			if u.Host == "" {
				return nil, fmt.Errorf("redis URL is missing host")
			}
			opts = &redis.Options{Addr: u.Host}
			if u.User != nil {
				opts.Username = u.User.Username()
				if password, ok := u.User.Password(); ok {
					opts.Password = password
				}
			}
			if u.Scheme == "tls" {
				opts.TLSConfig = &tls.Config{
					MinVersion: tls.VersionTLS12,
					ServerName: u.Hostname(),
				}
			}
		default:
			return nil, fmt.Errorf("unsupported redis URL scheme %q", u.Scheme)
		}
	}

	opts.PoolSize = cfg.PoolSize
	opts.MinIdleConns = cfg.MinIdleConns
	opts.MaxRetries = cfg.MaxRetries
	opts.DialTimeout = cfg.DialTimeout
	opts.ReadTimeout = cfg.ReadTimeout
	opts.WriteTimeout = cfg.WriteTimeout
	return opts, nil
}

func (c *Client) RDB() *redis.Client {
	return c.rdb
}

func (c *Client) Close() error {
	return c.rdb.Close()
}

func (c *Client) Health(ctx context.Context) error {
	return c.rdb.Ping(ctx).Err()
}
