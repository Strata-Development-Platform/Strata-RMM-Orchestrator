package monitoring

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
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

type batchBuffer struct {
	metrics []timescale.MetricRow
	events  []timescale.EventRow
	mu      sync.Mutex
	ticker  *time.Ticker
	done    chan struct{}
}

func NewBatchBuffer(tickerDuration time.Duration) *batchBuffer {
	return &batchBuffer{
		ticker: time.NewTicker(tickerDuration),
		done:   make(chan struct{}),
	}
}

func (b *batchBuffer) AddMetric(row timescale.MetricRow) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.metrics = append(b.metrics, row)
}

func (b *batchBuffer) AddEvent(row timescale.EventRow) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.events = append(b.events, row)
}

func (b *batchBuffer) Flush(ctx context.Context, tsdb *timescale.Client) error {
	b.mu.Lock()
	metrics := b.metrics
	events := b.events
	b.metrics = nil
	b.events = nil
	b.mu.Unlock()

	var errs []string
	if len(metrics) > 0 {
		if err := tsdb.InsertMetrics(ctx, metrics); err != nil {
			errs = append(errs, fmt.Sprintf("insert metrics batch (%d rows): %v", len(metrics), err))
		}
	}
	if len(events) > 0 {
		if err := tsdb.InsertEvents(ctx, events); err != nil {
			errs = append(errs, fmt.Sprintf("insert events batch (%d rows): %v", len(events), err))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("batch flush errors: %s", strings.Join(errs, "; "))
	}
	return nil
}

func (b *batchBuffer) StartLoop(ctx context.Context, tsdb *timescale.Client) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-b.done:
				return
			case <-b.ticker.C:
				if err := b.Flush(ctx, tsdb); err != nil {
					// Log error but don't stop the loop
					fmt.Printf("batch flush error: %v\n", err)
				}
			}
		}
	}()
}

func (b *batchBuffer) Stop() {
	close(b.done)
	b.ticker.Stop()
}

func NewIngestService(nc *nats.Conn, tsdb *timescale.Client, logger *zap.Logger) (*IngestService, error) {
	js, err := nc.JetStream()
	if err != nil {
		return nil, fmt.Errorf("create jetstream context: %w", err)
	}

	return &IngestService{
		nc:     nc,
		js:     js,
		tsdb:   tsdb,
		logger: logger,
		batch:  NewBatchBuffer(5 * time.Second),
	}, nil
}

func (s *IngestService) Start(ctx context.Context) error {
	s.logger.Info("starting metrics ingestion service with JetStream")

	// Create JetStream consumers with durable subscriptions and explicit ack
	metricsSub, err := s.js.Subscribe(jetstream.SubjectMetrics, s.handleAgentMetricsJS,
		nats.Durable(jetstream.ConsumerMetrics),
		nats.AckWait(30*time.Second),
		nats.MaxDeliver(10),
		nats.ManualAck(),
	)
	if err != nil {
		return fmt.Errorf("subscribing to metrics: %w", err)
	}
	s.subs = append(s.subs, metricsSub)

	eventsSub, err := s.js.Subscribe(jetstream.SubjectEvents, s.handleAgentEventsJS,
		nats.Durable(jetstream.ConsumerEvents),
		nats.AckWait(30*time.Second),
		nats.MaxDeliver(10),
		nats.ManualAck(),
	)
	if err != nil {
		return fmt.Errorf("subscribing to events: %w", err)
	}
	s.subs = append(s.subs, eventsSub)

	heartbeatSub, err := s.js.Subscribe(jetstream.SubjectHeartbeat, s.handleAgentHeartbeatJS,
		nats.Durable(jetstream.ConsumerHeartbeats),
		nats.AckWait(30*time.Second),
		nats.MaxDeliver(10),
		nats.ManualAck(),
	)
	if err != nil {
		return fmt.Errorf("subscribing to heartbeats: %w", err)
	}
	s.subs = append(s.subs, heartbeatSub)

	probeSNMPSub, err := s.js.Subscribe(jetstream.SubjectProbeSNMP, s.handleProbeSNMPJS,
		nats.Durable(jetstream.ConsumerProbes),
		nats.AckWait(30*time.Second),
		nats.MaxDeliver(10),
		nats.ManualAck(),
	)
	if err != nil {
		return fmt.Errorf("subscribing to probe snmp: %w", err)
	}
	s.subs = append(s.subs, probeSNMPSub)

	probeFlowSub, err := s.js.Subscribe(jetstream.SubjectProbeFlow, s.handleProbeFlowJS,
		nats.Durable(jetstream.ConsumerProbes),
		nats.AckWait(30*time.Second),
		nats.MaxDeliver(10),
		nats.ManualAck(),
	)
	if err != nil {
		return fmt.Errorf("subscribing to probe flow: %w", err)
	}
	s.subs = append(s.subs, probeFlowSub)

	probeDiscSub, err := s.js.Subscribe(jetstream.SubjectProbeDiscovery, s.handleProbeDiscoveryJS,
		nats.Durable(jetstream.ConsumerDiscovery),
		nats.AckWait(30*time.Second),
		nats.MaxDeliver(10),
		nats.ManualAck(),
	)
	if err != nil {
		return fmt.Errorf("subscribing to probe discovery: %w", err)
	}
	s.subs = append(s.subs, probeDiscSub)

	s.logger.Info("metrics ingestion subscriptions active",
		zap.Int("subscriptions", len(s.subs)),
	)

	go s.batch.StartLoop(ctx, s.tsdb)

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

func (s *IngestService) handleAgentMetricsJS(m *nats.Msg) {
	defer func() { _ = m.Ack() }()

	var payload struct {
		Samples []struct {
			Name      string            `json:"name"`
			Value     float64           `json:"value"`
			Tags      map[string]string `json:"tags"`
			Timestamp int64             `json:"timestamp"`
		} `json:"samples"`
	}

	if err := json.Unmarshal(m.Data, &payload); err != nil {
		s.logger.Warn("invalid metrics payload", zap.Error(err))
		return
	}

	tenantID, agentID := extractTenantAgent(m.Subject)
	deviceID, err := s.resolveDeviceID(tenantID, agentID)
	if err != nil {
		s.logger.Warn("rejecting metrics for unknown agent", zap.String("tenant_id", tenantID), zap.String("agent_id", agentID), zap.Error(err))
		return
	}

	var rows []timescale.MetricRow
	for _, sample := range payload.Samples {
		ts := time.Unix(sample.Timestamp, 0)
		if sample.Timestamp == 0 {
			ts = time.Now()
		}
		rows = append(rows, timescale.MetricRow{
			Time:       ts,
			TenantID:   tenantID,
			DeviceID:   deviceID,
			MetricName: sample.Name,
			Value:      sample.Value,
			Tags:       sample.Tags,
		})
	}

	for _, row := range rows {
		s.batch.AddMetric(row)
	}
}

func (s *IngestService) handleAgentEventsJS(m *nats.Msg) {
	defer func() { _ = m.Ack() }()

	var payload struct {
		Type      string            `json:"type"`
		Message   string            `json:"message"`
		Tags      map[string]string `json:"tags"`
		Timestamp int64             `json:"timestamp"`
	}

	if err := json.Unmarshal(m.Data, &payload); err != nil {
		s.logger.Warn("invalid event payload", zap.Error(err))
		return
	}

	tenantID, agentID := extractTenantAgent(m.Subject)
	deviceID, err := s.resolveDeviceID(tenantID, agentID)
	if err != nil {
		s.logger.Warn("rejecting event for unknown agent", zap.String("tenant_id", tenantID), zap.String("agent_id", agentID), zap.Error(err))
		return
	}

	ts := time.Unix(payload.Timestamp, 0)
	if payload.Timestamp == 0 {
		ts = time.Now()
	}

	row := timescale.EventRow{
		Time:      ts,
		TenantID:  tenantID,
		DeviceID:  deviceID,
		EventType: payload.Type,
		Message:   payload.Message,
		Tags:      payload.Tags,
	}

	s.batch.AddEvent(row)
}

func (s *IngestService) handleAgentHeartbeatJS(m *nats.Msg) {
	defer func() { _ = m.Ack() }()

	var payload struct {
		AgentID string `json:"agent_id"`
		Time    int64  `json:"time"`
		Status  string `json:"status"`
	}

	if err := json.Unmarshal(m.Data, &payload); err != nil {
		s.logger.Warn("invalid heartbeat payload", zap.Error(err))
		return
	}

	tenantID, agentID := extractTenantAgent(m.Subject)
	if payload.AgentID != "" && payload.AgentID != agentID {
		s.logger.Warn("rejecting heartbeat with mismatched agent identity")
		return
	}
	deviceID, err := s.resolveDeviceID(tenantID, agentID)
	if err != nil {
		s.logger.Warn("rejecting heartbeat for unknown agent", zap.String("tenant_id", tenantID), zap.String("agent_id", agentID), zap.Error(err))
		return
	}

	row := timescale.HeartbeatRow{
		Time:     time.Now(),
		TenantID: tenantID,
		DeviceID: deviceID,
		Status:   payload.Status,
	}

	if err := s.tsdb.RecordHeartbeat(context.Background(), row); err != nil {
		s.logger.Error("recording heartbeat", zap.Error(err))
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := s.tsdb.DB().ExecContext(ctx, `
		UPDATE devices SET last_heartbeat = NOW(), status = 'online', updated_at = NOW()
		WHERE tenant_id = $1 AND id = $2
	`, tenantID, deviceID); err != nil {
		s.logger.Error("updating device heartbeat state", zap.Error(err))
	}
}

// Batch flush loop - previously a stub, now functional
func (s *IngestService) batchFlushLoop(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.batch.Flush(ctx, s.tsdb); err != nil {
				s.logger.Error("batch flush error", zap.Error(err))
			}
		}
	}
}

func extractTenantAgent(subject string) (tenantID, agentID string) {
	// subject format: tenant.{tenantID}.agent.{agentID}.metrics
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

// Probe handlers
func (s *IngestService) handleProbeSNMPJS(m *nats.Msg) {
	defer func() { _ = m.Ack() }()
	tenantID := extractProbeTenant(m.Subject)
	if tenantID == "" {
		return
	}
	s.logger.Debug("probe snmp data received", zap.String("subject", m.Subject))
}

func (s *IngestService) handleProbeFlowJS(m *nats.Msg) {
	defer func() { _ = m.Ack() }()
	tenantID := extractProbeTenant(m.Subject)
	if tenantID == "" {
		return
	}
	s.logger.Debug("probe flow data received", zap.String("subject", m.Subject))
}

func (s *IngestService) handleProbeDiscoveryJS(m *nats.Msg) {
	defer func() { _ = m.Ack() }()
	tenantID := extractProbeTenant(m.Subject)
	if tenantID == "" {
		return
	}
	s.logger.Debug("probe discovery data received", zap.String("subject", m.Subject))
}

func extractProbeTenant(subject string) string {
	// subject format: tenant.{tenantID}.probe.{probeID}.{type}
	parts := tokenize(subject, '.')
	if len(parts) >= 4 && parts[0] == "tenant" {
		return parts[1]
	}
	return ""
}
