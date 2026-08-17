package jetstream

import (
	"context"
	"fmt"

	"github.com/nats-io/nats.go"
	"go.uber.org/zap"
)

// StreamManager handles JetStream stream creation and validation.
type StreamManager struct {
	js  nats.JetStreamContext
	cfg Config
	log *zap.Logger
}

// NewStreamManager creates a new stream manager.
func NewStreamManager(js nats.JetStreamContext, cfg Config, log *zap.Logger) *StreamManager {
	return &StreamManager{js: js, cfg: cfg, log: log}
}

// EnsureStreams creates all required streams if they don't exist, or validates existing ones.
func (m *StreamManager) EnsureStreams(ctx context.Context) error {
	streams := m.requiredStreams()
	for _, sc := range streams {
		if err := m.ensureStream(ctx, sc); err != nil {
			return fmt.Errorf("ensure stream %s: %w", sc.Name, err)
		}
	}
	return nil
}

func (m *StreamManager) ensureStream(ctx context.Context, sc *StreamConfig) error {
	info, err := m.js.StreamInfo(sc.Name)
	if err == nil {
		m.log.Info("jetstream stream exists", zap.String("stream", sc.Name), zap.Strings("subjects", info.Config.Subjects))
		return nil
	}

	_, err = m.js.AddStream(&nats.StreamConfig{
		Name:        sc.Name,
		Subjects:    sc.Subjects,
		MaxAge:      sc.MaxAge,
		MaxBytes:    sc.MaxBytes,
		MaxMsgs:     sc.MaxMsgs,
		Retention:   m.retentionToNats(sc.Retention),
		Storage:     m.storageToNats(sc.Storage),
		Replicas:    sc.Replicas,
		Discard:     m.discardToNats(sc.Discard),
		AllowRollup: sc.AllowRollup,
	})
	if err != nil {
		m.log.Warn("jetstream stream add failed (may already exist)", zap.String("stream", sc.Name), zap.Error(err))
		return nil
	}

	m.log.Info("jetstream stream created", zap.String("stream", sc.Name), zap.Strings("subjects", sc.Subjects))
	return nil
}

func (m *StreamManager) requiredStreams() []*StreamConfig {
	return []*StreamConfig{
		m.cfg.StreamConfigFor(StreamMetrics, []string{SubjectMetrics}),
		m.cfg.StreamConfigFor(StreamEvents, []string{SubjectEvents}),
		m.cfg.StreamConfigFor(StreamHeartbeats, []string{SubjectHeartbeat}),
		m.cfg.StreamConfigFor(StreamCommands, []string{SubjectCommands}),
		m.cfg.StreamConfigFor(StreamCmdResults, []string{SubjectCmdResult, SubjectCmdAck}),
		m.cfg.StreamConfigFor(StreamEndpointResults, []string{SubjectScriptResult, SubjectSoftwareResult}),
		m.cfg.StreamConfigFor(StreamProbes, []string{SubjectProbeSNMP, SubjectProbeFlow}),
		m.cfg.StreamConfigFor(StreamDiscovery, []string{SubjectProbeDiscovery}),
		m.cfg.StreamConfigFor(StreamAgentSession, []string{"strata.agent.*"}),
		m.cfg.StreamConfigFor(StreamIntegrations, []string{
			"integrations.edr.*",
			"integrations.backup.*",
			"integrations.psa.*",
		}),
	}
}

func (m *StreamManager) retentionToNats(s string) nats.RetentionPolicy {
	switch s {
	case "interest":
		return nats.InterestPolicy
	default:
		return nats.LimitsPolicy
	}
}

func (m *StreamManager) storageToNats(s string) nats.StorageType {
	switch s {
	case "memory":
		return nats.MemoryStorage
	default:
		return nats.FileStorage
	}
}

func (m *StreamManager) discardToNats(s string) nats.DiscardPolicy {
	switch s {
	case "old":
		return nats.DiscardOld
	default:
		return nats.DiscardNew
	}
}
