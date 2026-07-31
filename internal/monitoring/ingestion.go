package monitoring

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
	"go.uber.org/zap"

	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/timescale"
)

type IngestService struct {
	nats     *nats.Conn
	tsdb     *timescale.Client
	logger   *zap.Logger
	tenantID string
	subs     []*nats.Subscription
}

func NewIngestService(nc *nats.Conn, tsdb *timescale.Client, logger *zap.Logger) *IngestService {
	return &IngestService{
		nats:   nc,
		tsdb:   tsdb,
		logger: logger,
	}
}

func (s *IngestService) Start(ctx context.Context) error {
	s.logger.Info("starting metrics ingestion service")

	agentMetricsSub, err := s.nats.Subscribe("tenant.*.agent.*.metrics", s.handleAgentMetrics)
	if err != nil {
		return fmt.Errorf("subscribing to agent metrics: %w", err)
	}
	s.subs = append(s.subs, agentMetricsSub)

	agentEventsSub, err := s.nats.Subscribe("tenant.*.agent.*.events", s.handleAgentEvents)
	if err != nil {
		return fmt.Errorf("subscribing to agent events: %w", err)
	}
	s.subs = append(s.subs, agentEventsSub)

	heartbeatSub, err := s.nats.Subscribe("tenant.*.agent.*.heartbeat", s.handleAgentHeartbeat)
	if err != nil {
		return fmt.Errorf("subscribing to heartbeats: %w", err)
	}
	s.subs = append(s.subs, heartbeatSub)

	probeSNMPSub, err := s.nats.Subscribe("tenant.*.probe.*.snmp", s.handleProbeSNMP)
	if err != nil {
		return fmt.Errorf("subscribing to probe snmp: %w", err)
	}
	s.subs = append(s.subs, probeSNMPSub)

	probeFlowSub, err := s.nats.Subscribe("tenant.*.probe.*.flow", s.handleProbeFlow)
	if err != nil {
		return fmt.Errorf("subscribing to probe flow: %w", err)
	}
	s.subs = append(s.subs, probeFlowSub)

	probeDiscSub, err := s.nats.Subscribe("tenant.*.probe.*.discovery", s.handleProbeDiscovery)
	if err != nil {
		return fmt.Errorf("subscribing to probe discovery: %w", err)
	}
	s.subs = append(s.subs, probeDiscSub)

	s.logger.Info("metrics ingestion subscriptions active",
		zap.Int("subscriptions", len(s.subs)),
	)

	go s.batchFlushLoop(ctx)

	return nil
}

func (s *IngestService) Stop() {
	for _, sub := range s.subs {
		if err := sub.Unsubscribe(); err != nil {
			s.logger.Warn("unsubscribing", zap.Error(err))
		}
	}
}

func (s *IngestService) handleAgentMetrics(m *nats.Msg) {
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

	if len(rows) > 0 {
		if err := s.tsdb.InsertMetrics(context.Background(), rows); err != nil {
			s.logger.Error("inserting metrics", zap.Error(err))
		}
	}
}

func (s *IngestService) handleAgentEvents(m *nats.Msg) {
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

	if err := s.tsdb.InsertEvents(context.Background(), []timescale.EventRow{row}); err != nil {
		s.logger.Error("inserting event", zap.Error(err))
	}
}

func (s *IngestService) handleAgentHeartbeat(m *nats.Msg) {
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

func (s *IngestService) batchFlushLoop(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
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

func (s *IngestService) handleProbeSNMP(m *nats.Msg) {
	tenantID := extractProbeTenant(m.Subject)
	if tenantID == "" {
		return
	}
	s.logger.Debug("probe snmp data received", zap.String("subject", m.Subject))
}

func (s *IngestService) handleProbeFlow(m *nats.Msg) {
	tenantID := extractProbeTenant(m.Subject)
	if tenantID == "" {
		return
	}
	s.logger.Debug("probe flow data received", zap.String("subject", m.Subject))
}

func (s *IngestService) handleProbeDiscovery(m *nats.Msg) {
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
