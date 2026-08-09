package monitoring

import (
	"context"
	"encoding/json"
	"time"

	"github.com/nats-io/nats.go"
	"go.uber.org/zap"

	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/timescale"
)

func (s *IngestService) handleAgentMetricsJS(m *nats.Msg) {
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
		_ = m.Term()
		return
	}

	tenantID, agentID := extractTenantAgent(m.Subject)
	deviceID, err := s.resolveDeviceID(tenantID, agentID)
	if err != nil {
		s.logger.Warn("rejecting metrics for unknown agent", zap.String("tenant_id", tenantID), zap.String("agent_id", agentID), zap.Error(err))
		_ = m.Nak()
		return
	}

	rows := make([]timescale.MetricRow, 0, len(payload.Samples))
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

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if len(rows) > 0 {
		if err := s.tsdb.InsertMetrics(ctx, rows); err != nil {
			s.logger.Error("persisting metrics before acknowledgement", zap.Error(err), zap.Int("rows", len(rows)))
			_ = m.Nak()
			return
		}
	}
	if err := m.Ack(); err != nil {
		s.logger.Warn("acknowledging persisted metrics", zap.Error(err))
	}
}

func (s *IngestService) handleAgentEventsJS(m *nats.Msg) {
	var payload struct {
		Type      string            `json:"type"`
		Message   string            `json:"message"`
		Tags      map[string]string `json:"tags"`
		Timestamp int64             `json:"timestamp"`
	}

	if err := json.Unmarshal(m.Data, &payload); err != nil {
		s.logger.Warn("invalid event payload", zap.Error(err))
		_ = m.Term()
		return
	}

	tenantID, agentID := extractTenantAgent(m.Subject)
	deviceID, err := s.resolveDeviceID(tenantID, agentID)
	if err != nil {
		s.logger.Warn("rejecting event for unknown agent", zap.String("tenant_id", tenantID), zap.String("agent_id", agentID), zap.Error(err))
		_ = m.Nak()
		return
	}

	ts := time.Unix(payload.Timestamp, 0)
	if payload.Timestamp == 0 {
		ts = time.Now()
	}
	row := timescale.EventRow{Time: ts, TenantID: tenantID, DeviceID: deviceID, EventType: payload.Type, Message: payload.Message, Tags: payload.Tags}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := s.tsdb.InsertEvents(ctx, []timescale.EventRow{row}); err != nil {
		s.logger.Error("persisting event before acknowledgement", zap.Error(err))
		_ = m.Nak()
		return
	}
	if err := m.Ack(); err != nil {
		s.logger.Warn("acknowledging persisted event", zap.Error(err))
	}
}

func (s *IngestService) handleAgentHeartbeatJS(m *nats.Msg) {
	var payload struct {
		AgentID string `json:"agent_id"`
		Time    int64  `json:"time"`
		Status  string `json:"status"`
	}

	if err := json.Unmarshal(m.Data, &payload); err != nil {
		s.logger.Warn("invalid heartbeat payload", zap.Error(err))
		_ = m.Term()
		return
	}

	tenantID, agentID := extractTenantAgent(m.Subject)
	if payload.AgentID != "" && payload.AgentID != agentID {
		s.logger.Warn("rejecting heartbeat with mismatched agent identity")
		_ = m.Term()
		return
	}
	deviceID, err := s.resolveDeviceID(tenantID, agentID)
	if err != nil {
		s.logger.Warn("rejecting heartbeat for unknown agent", zap.String("tenant_id", tenantID), zap.String("agent_id", agentID), zap.Error(err))
		_ = m.Nak()
		return
	}

	row := timescale.HeartbeatRow{Time: time.Now(), TenantID: tenantID, DeviceID: deviceID, Status: payload.Status}
	if err := s.tsdb.RecordHeartbeat(context.Background(), row); err != nil {
		s.logger.Error("recording heartbeat", zap.Error(err))
		_ = m.Nak()
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := s.tsdb.DB().ExecContext(ctx, `
		UPDATE devices SET last_heartbeat = NOW(), status = 'online', updated_at = NOW()
		WHERE tenant_id = $1 AND id = $2
	`, tenantID, deviceID); err != nil {
		s.logger.Error("updating device heartbeat state", zap.Error(err))
		_ = m.Nak()
		return
	}
	if err := m.Ack(); err != nil {
		s.logger.Warn("acknowledging persisted heartbeat", zap.Error(err))
	}
}
