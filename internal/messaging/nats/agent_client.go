package nats

import (
	"context"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
)

// Client wraps the JetStream context for agent-side publishing.
type Client struct {
	conn *nats.Conn
	js   nats.JetStreamContext
}

// NewClient creates a new JetStream client from a NATS connection.
func NewClient(nc *nats.Conn) (*Client, error) {
	js, err := nc.JetStream()
	if err != nil {
		return nil, fmt.Errorf("create jetstream client: %w", err)
	}
	return &Client{conn: nc, js: js}, nil
}

// Conn returns the underlying NATS connection.
func (c *Client) Conn() *nats.Conn {
	return c.conn
}

// JS returns the underlying JetStreamContext.
func (c *Client) JS() nats.JetStreamContext {
	return c.js
}

// Publish publishes a message to a JetStream stream with a timeout.
func (c *Client) Publish(ctx context.Context, subject string, data []byte, timeout ...uint) error {
	_, err := c.js.Publish(subject, data)
	return err
}

// PublishSync is an alias for Publish.
func (c *Client) PublishSync(ctx context.Context, subject string, data []byte) error {
	return c.Publish(ctx, subject, data)
}

// PublishAsync publishes a message asynchronously.
func (c *Client) PublishAsync(subject string, data []byte) (<-chan struct{}, error) {
	future, err := c.js.PublishAsync(subject, data)
	if err != nil {
		return nil, err
	}
	done := make(chan struct{})
	go func() {
		select {
		case <-future.Ok():
			close(done)
		case <-future.Err():
			close(done)
		}
	}()
	return done, nil
}

// Request sends a request and waits for a response.
func (c *Client) Request(ctx context.Context, subject string, data []byte, timeout uint) (*nats.Msg, error) {
	return c.conn.Request(subject, data, time.Duration(int64(timeout)))
}
