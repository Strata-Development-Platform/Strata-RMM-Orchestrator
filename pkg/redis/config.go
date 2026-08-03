package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type PoolConfig struct {
	URL         string
	PoolSize    int
	MinIdleConns int
	MaxRetries  int
	DialTimeout time.Duration
	ReadTimeout time.Duration
	WriteTimeout time.Duration
}

type Client struct {
	rdb *redis.Client
	cfg PoolConfig
}

func NewClient(ctx context.Context, cfg PoolConfig) (*Client, error) {
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

	rdb := redis.NewClient(&redis.Options{
		Addr:         cfg.URL,
		PoolSize:     cfg.PoolSize,
		MinIdleConns: cfg.MinIdleConns,
		MaxRetries:   cfg.MaxRetries,
		DialTimeout:  cfg.DialTimeout,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
	})

	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("pinging redis: %w", err)
	}

	return &Client{rdb: rdb, cfg: cfg}, nil
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
