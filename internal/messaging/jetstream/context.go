package jetstream

import (
	"fmt"

	"github.com/nats-io/nats.go"
)

// Context wraps the JetStream context and provides convenience methods.
type Context struct {
	js nats.JetStreamContext
}

// New creates a new JetStream context from a NATS connection.
func New(nc *nats.Conn) (*Context, error) {
	js, err := nc.JetStream()
	if err != nil {
		return nil, fmt.Errorf("create jetstream context: %w", err)
	}
	return &Context{js: js}, nil
}

// JS returns the underlying nats.JetStreamContext.
func (c *Context) JS() nats.JetStreamContext {
	return c.js
}

// Publish publishes a message to a JetStream stream.
func (c *Context) Publish(subject string, data []byte, opts ...nats.PubOpt) (*nats.PubAck, error) {
	return c.js.Publish(subject, data, opts...)
}

// PublishAsync publishes a message asynchronously and returns a future.
func (c *Context) PublishAsync(subject string, data []byte) (<-chan struct{}, error) {
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

// Subscribe creates a JetStream subscription with the given options.
func (c *Context) Subscribe(subject string, cb nats.MsgHandler, opts ...nats.SubOpt) (*nats.Subscription, error) {
	return c.js.Subscribe(subject, cb, opts...)
}

// QueueSubscribe creates a JetStream queue subscription.
func (c *Context) QueueSubscribe(subject, queue string, cb nats.MsgHandler, opts ...nats.SubOpt) (*nats.Subscription, error) {
	return c.js.QueueSubscribe(subject, queue, cb, opts...)
}

// BindConsumer binds to an existing consumer.
func (c *Context) BindConsumer(stream, consumer string) (*nats.ConsumerInfo, error) {
	return c.js.ConsumerInfo(stream, consumer)
}
