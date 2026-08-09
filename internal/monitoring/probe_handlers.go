package monitoring

import (
	"github.com/nats-io/nats.go"
	"go.uber.org/zap"
)

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
	parts := tokenize(subject, '.')
	if len(parts) >= 4 && parts[0] == "tenant" {
		return parts[1]
	}
	return ""
}
