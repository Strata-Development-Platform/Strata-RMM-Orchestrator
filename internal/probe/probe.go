package probe

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"go.uber.org/zap"
)

type Probe struct {
	ID       string
	TenantID string
	NATS     *nats.Conn
	Logger   *zap.Logger
	Config   Config

	mu        sync.RWMutex
	targets   []SNMPTarget
	discovery *DiscoveryEngine
	flow      *FlowCollector
	cancel    context.CancelFunc
}

type Config struct {
	ProbeID           string        `yaml:"probe_id"`
	TenantID          string        `yaml:"tenant_id"`
	NATSURL           string        `yaml:"nats_url"`
	SNMPTargets       []SNMPTarget  `yaml:"snmp_targets"`
	DiscoveryEnabled  bool          `yaml:"discovery_enabled"`
	DiscoverySubnets  []string      `yaml:"discovery_subnets"`
	FlowEnabled       bool          `yaml:"flow_enabled"`
	FlowPort          int           `yaml:"flow_port"`
	FlowProtocols     []string      `yaml:"flow_protocols"`
	PollInterval      time.Duration `yaml:"poll_interval"`
	DiscoveryInterval time.Duration `yaml:"discovery_interval"`
}

type SNMPTarget struct {
	Host      string            `yaml:"host"`
	Port      int               `yaml:"port"`
	Version   string            `yaml:"version"` // v1, v2c, v3
	Community string            `yaml:"community"`
	V3        SNMPV3Config      `yaml:"v3"`
	OIDs      []string          `yaml:"oids"`
	Interval  time.Duration     `yaml:"interval"`
	Labels    map[string]string `yaml:"labels"`
}

type SNMPV3Config struct {
	Username  string `yaml:"username"`
	AuthProto string `yaml:"auth_proto"` // MD5, SHA
	AuthPass  string `yaml:"auth_pass"`
	PrivProto string `yaml:"priv_proto"` // DES, AES
	PrivPass  string `yaml:"priv_pass"`
	Context   string `yaml:"context"`
}

func New(id, tenantID string, nc *nats.Conn, cfg Config, logger *zap.Logger) *Probe {
	return &Probe{
		ID:       id,
		TenantID: tenantID,
		NATS:     nc,
		Logger:   logger,
		Config:   cfg,
	}
}

func (p *Probe) Start(ctx context.Context) error {
	ctx, p.cancel = context.WithCancel(ctx)
	p.Logger.Info("starting network probe",
		zap.String("probe_id", p.ID),
		zap.String("tenant_id", p.TenantID),
	)

	if len(p.Config.SNMPTargets) > 0 {
		p.targets = p.Config.SNMPTargets
		go p.snmpPollLoop(ctx)
		p.Logger.Info("SNMP polling started", zap.Int("targets", len(p.targets)))
	}

	if p.Config.DiscoveryEnabled {
		p.discovery = NewDiscoveryEngine(p, p.Config.DiscoverySubnets)
		go p.discovery.Run(ctx)
		p.Logger.Info("network discovery started", zap.Strings("subnets", p.Config.DiscoverySubnets))
	}

	if p.Config.FlowEnabled {
		p.flow = NewFlowCollector(p, p.Config.FlowPort, p.Config.FlowProtocols)
		go p.flow.Start(ctx)
		p.Logger.Info("flow collector started", zap.Int("port", p.Config.FlowPort))
	}

	go p.healthReportLoop(ctx)

	return nil
}

func (p *Probe) Stop() {
	if p.cancel != nil {
		p.cancel()
	}
	if p.flow != nil {
		p.flow.Stop()
	}
	p.Logger.Info("probe stopped")
}

func (p *Probe) snmpPollLoop(ctx context.Context) {
	tickers := make(map[string]*time.Ticker)
	tickerChs := make(map[string]<-chan time.Time)

	for _, t := range p.targets {
		interval := t.Interval
		if interval <= 0 {
			interval = 5 * time.Minute
		}
		t := t
		tkr := time.NewTicker(interval)
		tickers[t.Host] = tkr
		tickerChs[t.Host] = tkr.C
		go func(t SNMPTarget) {
			p.collectSNMP(ctx, t)
			for {
				select {
				case <-ctx.Done():
					return
				case <-tickerChs[t.Host]:
					p.collectSNMP(ctx, t)
				}
			}
		}(t)
	}

	<-ctx.Done()
	for _, tkr := range tickers {
		tkr.Stop()
	}
}

func (p *Probe) collectSNMP(ctx context.Context, target SNMPTarget) {
	p.Logger.Debug("polling SNMP target", zap.String("host", target.Host))

	results, err := PollSNMP(ctx, target)
	if err != nil {
		p.Logger.Warn("SNMP poll failed",
			zap.String("host", target.Host),
			zap.Error(err),
		)
		p.publishEvent("snmp_error", map[string]interface{}{
			"target": target.Host,
			"error":  err.Error(),
		})
		return
	}

	for _, r := range results {
		p.recordSNMPPoll(target, r)
	}
}

func (p *Probe) recordSNMPPoll(target SNMPTarget, result SNMPResult) {
	payload := map[string]interface{}{
		"probe_id":  p.ID,
		"tenant_id": p.TenantID,
		"target_ip": target.Host,
		"oid":       result.OID,
		"value":     result.Value,
		"type":      result.Type,
		"uptime":    result.Uptime,
		"time":      time.Now().UTC(),
	}
	data, _ := json.Marshal(payload)

	subject := fmt.Sprintf("tenant.%s.probe.%s.snmp", p.TenantID, p.ID)
	if err := p.NATS.Publish(subject, data); err != nil {
		p.Logger.Warn("publish snmp poll", zap.Error(err))
	}
}

func (p *Probe) publishAlert(alertType string, data map[string]interface{}) {
	payload := map[string]interface{}{
		"type":      alertType,
		"probe_id":  p.ID,
		"tenant_id": p.TenantID,
		"time":      time.Now().UTC(),
		"data":      data,
	}
	msg, _ := json.Marshal(payload)
	subject := fmt.Sprintf("tenant.%s.probe.%s.event", p.TenantID, p.ID)
	p.NATS.Publish(subject, msg)
}

func (p *Probe) publishEvent(eventType string, data map[string]interface{}) {
	p.publishAlert(eventType, data)
}

func (p *Probe) healthReportLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			payload := map[string]interface{}{
				"probe_id":     p.ID,
				"tenant_id":    p.TenantID,
				"status":       "online",
				"snmp_targets": len(p.Config.SNMPTargets),
				"discovery":    p.Config.DiscoveryEnabled,
				"flow":         p.Config.FlowEnabled,
				"time":         time.Now().UTC(),
			}
			data, _ := json.Marshal(payload)
			subject := fmt.Sprintf("tenant.%s.probe.%s.heartbeat", p.TenantID, p.ID)
			p.NATS.Publish(subject, data)
		}
	}
}
