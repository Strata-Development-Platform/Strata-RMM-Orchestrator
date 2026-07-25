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

	agentMetricsSub, err := s.nats.Subscribe("tenant.>.agent.>.metrics", s.handleAgentMetrics)
	if err != nil {
		return fmt.Errorf("subscribing to agent metrics: %w", err)
	}
	s.subs = append(s.subs, agentMetricsSub)

	agentEventsSub, err := s.nats.Subscribe("tenant.>.agent.>.events", s.handleAgentEvents)
	if err != nil {
		return fmt.Errorf("subscribing to agent events: %w", err)
	}
	s.subs = append(s.subs, agentEventsSub)

	heartbeatSub, err := s.nats.Subscribe("tenant.>.agent.>.heartbeat", s.handleAgentHeartbeat)
	if err != nil {
		return fmt.Errorf("subscribing to heartbeats: %w", err)
	}
	s.subs = append(s.subs, heartbeatSub)

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

	tenantID, deviceID := extractTenantDevice(m.Subject)
	if tenantID == "" || deviceID == "" {
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

	tenantID, deviceID := extractTenantDevice(m.Subject)

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

	tenantID, deviceID := extractTenantDevice(m.Subject)

	row := timescale.HeartbeatRow{
		Time:     time.Now(),
		TenantID: tenantID,
		DeviceID: deviceID,
		Status:   payload.Status,
	}

	if err := s.tsdb.RecordHeartbeat(context.Background(), row); err != nil {
		s.logger.Error("recording heartbeat", zap.Error(err))
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

func extractTenantDevice(subject string) (tenantID, deviceID string) {
	// subject format: tenant.{tenantID}.agent.{deviceID}.metrics
	parts := tokenize(subject, '.')
	if len(parts) >= 4 && parts[0] == "tenant" {
		return parts[1], parts[3]
	}
	return "", ""
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
