package jetstream

import (
	"context"
	"fmt"

	"github.com/nats-io/nats.go"
	"go.uber.org/zap"
)

// ConsumerManager handles JetStream consumer creation and validation.
type ConsumerManager struct {
	js   nats.JetStreamContext
	cfg  Config
	log  *zap.Logger
}

// NewConsumerManager creates a new consumer manager.
func NewConsumerManager(js nats.JetStreamContext, cfg Config, log *zap.Logger) *ConsumerManager {
	return &ConsumerManager{js: js, cfg: cfg, log: log}
}

// EnsureConsumers creates all required consumers if they don't exist.
func (m *ConsumerManager) EnsureConsumers(ctx context.Context) error {
	consumers := m.requiredConsumers()
	for _, cc := range consumers {
		if err := m.ensureConsumer(ctx, cc); err != nil {
			return fmt.Errorf("ensure consumer %s/%s: %w", cc.Stream, cc.Name, err)
		}
	}
	return nil
}

func (m *ConsumerManager) ensureConsumer(ctx context.Context, cc *ConsumerConfig) error {
	// Try to get existing consumer info
	_, err := m.js.ConsumerInfo(cc.Stream, cc.Name)
	if err == nil {
		m.log.Info("jetstream consumer exists", zap.String("stream", cc.Stream), zap.String("consumer", cc.Name))
		return nil
	}

	// Consumer doesn't exist, create it
	_, err = m.js.AddConsumer(cc.Stream, &nats.ConsumerConfig{
		Durable:   cc.Durable,
		Name:      cc.Name,
		AckPolicy: m.ackPolicyToNats(cc.AckPolicy),
		AckWait:   cc.AckWait,
		MaxDeliver: cc.MaxDeliver,
		ReplayPolicy: m.replayPolicyToNats(cc.Replay),
		RateLimit: cc.RateLimit,
	})
	if err != nil {
		m.log.Warn("jetstream consumer add failed (may already exist)",
			zap.String("stream", cc.Stream),
			zap.String("consumer", cc.Name),
			zap.Error(err))
		return nil // Consumer may have been created by another node
	}

	m.log.Info("jetstream consumer created",
		zap.String("stream", cc.Stream),
		zap.String("consumer", cc.Name))
	return nil
}

func (m *ConsumerManager) requiredConsumers() []*ConsumerConfig {
	return []*ConsumerConfig{
		m.cfg.ConsumerConfigFor(StreamMetrics, ConsumerMetrics),
		m.cfg.ConsumerConfigFor(StreamEvents, ConsumerEvents),
		m.cfg.ConsumerConfigFor(StreamHeartbeats, ConsumerHeartbeats),
		m.cfg.ConsumerConfigFor(StreamCmdResults, ConsumerCmdResults),
		m.cfg.ConsumerConfigFor(StreamProbes, ConsumerProbes),
		m.cfg.ConsumerConfigFor(StreamDiscovery, ConsumerDiscovery),
		m.cfg.ConsumerConfigFor(StreamAgentSession, ConsumerAgentReplay),
		m.cfg.ConsumerConfigFor(StreamIntegrations, ConsumerIntegrations),
	}
}

func (m *ConsumerManager) ackPolicyToNats(s string) nats.AckPolicy {
	switch s {
	case "all":
		return nats.AckAllPolicy
	case "none":
		return nats.AckNonePolicy
	default:
		return nats.AckExplicitPolicy
	}
}

func (m *ConsumerManager) replayPolicyToNats(s string) nats.ReplayPolicy {
	switch s {
	case "original":
		return nats.ReplayOriginalPolicy
	default:
		return nats.ReplayInstantPolicy
	}
}
