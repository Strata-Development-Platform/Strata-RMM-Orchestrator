package monitoring

import (
	"context"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
	"go.uber.org/zap"

	"github.com/strata-rmm/strata-rmm-orchestrator/internal/messaging/jetstream"
	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/timescale"
)

type IngestService struct {
	nc     *nats.Conn
	js     nats.JetStreamContext
	tsdb   *timescale.Client
	logger *zap.Logger
	subs   []*nats.Subscription
	batch  *batchBuffer
}

func NewIngestService(nc *nats.Conn, tsdb *timescale.Client, logger *zap.Logger) (*IngestService, error) {
	js, err := nc.JetStream()
	if err != nil {
		return nil, fmt.Errorf("create jetstream context: %w", err)
	}
	if err := jetstream.NewStreamManager(js, jetstream.Default(), logger).EnsureStreams(context.Background()); err != nil {
		return nil, fmt.Errorf("ensure required jetstream streams: %w", err)
	}
	return &IngestService{nc: nc, js: js, tsdb: tsdb, logger: logger, batch: NewBatchBuffer(5 * time.Second)}, nil
}

func (s *IngestService) Start(ctx context.Context) error {
	s.logger.Info("starting metrics ingestion service with JetStream")

	subscriptions := []struct {
		subject string
		durable string
		handler nats.MsgHandler
	}{
		{jetstream.SubjectMetrics, jetstream.ConsumerMetrics, s.handleAgentMetricsJS},
		{jetstream.SubjectEvents, jetstream.ConsumerEvents, s.handleAgentEventsJS},
		{jetstream.SubjectHeartbeat, jetstream.ConsumerHeartbeats, s.handleAgentHeartbeatJS},
		{jetstream.SubjectProbeSNMP, jetstream.ConsumerProbes, s.handleProbeSNMPJS},
		{jetstream.SubjectProbeFlow, jetstream.ConsumerProbes, s.handleProbeFlowJS},
		{jetstream.SubjectProbeDiscovery, jetstream.ConsumerDiscovery, s.handleProbeDiscoveryJS},
	}

	for _, spec := range subscriptions {
		sub, err := s.js.Subscribe(spec.subject, spec.handler,
			nats.Durable(spec.durable),
			nats.AckWait(30*time.Second),
			nats.MaxDeliver(10),
			nats.ManualAck(),
		)
		if err != nil {
			return fmt.Errorf("subscribing to %s: %w", spec.subject, err)
		}
		s.subs = append(s.subs, sub)
	}

	s.logger.Info("metrics ingestion subscriptions active", zap.Int("subscriptions", len(s.subs)))
	return nil
}

func (s *IngestService) Stop() {
	s.batch.Stop()
	for _, sub := range s.subs {
		if err := sub.Unsubscribe(); err != nil {
			s.logger.Warn("unsubscribing", zap.Error(err))
		}
	}
}

func extractTenantAgent(subject string) (tenantID, agentID string) {
	parts := tokenize(subject, '.')
	if len(parts) >= 5 && parts[0] == "tenant" && parts[2] == "agent" && parts[1] != "" && parts[3] != "" {
		return parts[1], parts[3]
	}
	return "", ""
}

func (s *IngestService) resolveDeviceID(tenantID, agentID string) (string, error) {
	if tenantID == "" || agentID == "" {
		return "", fmt.Errorf("invalid telemetry subject")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var deviceID string
	if err := s.tsdb.DB().QueryRowContext(ctx, `
		SELECT id::text FROM devices
		WHERE tenant_id = $1 AND agent_id = $2 AND status != 'disabled'
	`, tenantID, agentID).Scan(&deviceID); err != nil {
		return "", fmt.Errorf("resolve registered device: %w", err)
	}
	return deviceID, nil
}

func tokenize(s string, sep byte) []string {
	var parts []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == sep {
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	parts = append(parts, s[start:])
	return parts
}
