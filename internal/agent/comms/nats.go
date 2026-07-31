package comms

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/strata-rmm/strata-rmm-orchestrator/internal/agent/core"
	"github.com/strata-rmm/strata-rmm-orchestrator/internal/reconnect"
)

type Client struct {
	conn   *nats.Conn
	cfg    *core.NATSConfig
	ident  *core.Identity
	logger core.Logger
	subs   []*nats.Subscription
	done   chan struct{}
}

func NewClient(cfg *core.NATSConfig, ident *core.Identity, logger core.Logger) *Client {
	return &Client{
		cfg:    cfg,
		ident:  ident,
		logger: logger,
		done:   make(chan struct{}),
	}
}

func (c *Client) Connect(ctx context.Context) error {
	opts := []nats.Option{
		nats.Name(fmt.Sprintf("StrataRMM-Agent-%s", c.ident.AgentID)),
		nats.CustomReconnectDelay(func(attempts int) time.Duration {
			return reconnect.Delay(c.cfg.ReconnectWait, 2*time.Minute, attempts, nil)
		}),
		nats.MaxReconnects(c.cfg.MaxReconnects),
		nats.RetryOnFailedConnect(true),
		nats.DisconnectErrHandler(func(nc *nats.Conn, err error) {
			c.logger.Warn("nats disconnected", "error", err)
		}),
		nats.ReconnectHandler(func(nc *nats.Conn) {
			c.logger.Info("nats reconnected", "url", nc.ConnectedUrl())
		}),
		nats.ClosedHandler(func(nc *nats.Conn) {
			c.logger.Info("nats connection closed")
		}),
		nats.ErrorHandler(func(nc *nats.Conn, sub *nats.Subscription, err error) {
			c.logger.Error("nats error", "error", err, "subject", sub.Subject)
		}),
	}

	if c.cfg.Token != "" {
		opts = append(opts, nats.Token(c.cfg.Token))
	}

	if (c.cfg.CertFile == "") != (c.cfg.KeyFile == "") {
		return fmt.Errorf("tls config: cert_file and key_file must be configured together")
	}
	tlsRequired := c.cfg.CAFile != "" || c.cfg.CertFile != ""
	for _, rawURL := range c.cfg.URLs {
		u, err := url.Parse(rawURL)
		if err == nil && (u.Scheme == "tls" || u.Scheme == "nats+tls") {
			tlsRequired = true
		}
	}
	if tlsRequired {
		tlsConfig, err := c.tlsConfig()
		if err != nil {
			return fmt.Errorf("tls config: %w", err)
		}
		opts = append(opts, nats.Secure(tlsConfig))
	}

	url := nats.DefaultURL
	if len(c.cfg.URLs) > 0 {
		url = c.cfg.URLs[0]
	}

	conn, err := nats.Connect(url, opts...)
	if err != nil {
		return fmt.Errorf("connecting to NATS: %w", err)
	}
	c.conn = conn

	c.logger.Info("connected to NATS", "url", conn.ConnectedUrl(), "server", conn.ConnectedServerId())
	return nil
}

func (c *Client) tlsConfig() (*tls.Config, error) {
	tlsCfg := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}
	if c.cfg.CertFile != "" {
		cert, err := tls.LoadX509KeyPair(c.cfg.CertFile, c.cfg.KeyFile)
		if err != nil {
			return nil, err
		}
		tlsCfg.Certificates = []tls.Certificate{cert}
	}

	if c.cfg.CAFile != "" {
		caCert, err := loadCA(c.cfg.CAFile)
		if err != nil {
			return nil, err
		}
		tlsCfg.RootCAs = caCert
	}

	return tlsCfg, nil
}

func loadCA(path string) (*x509.CertPool, error) {
	caData, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caData) {
		return nil, fmt.Errorf("no CA certs found in %s", path)
	}
	return pool, nil
}

func (c *Client) Subjects() *Subjects {
	return &Subjects{
		tenantID: c.ident.TenantID,
		agentID:  c.ident.AgentID,
	}
}

func (c *Client) JetStream() (nats.JetStreamContext, error) {
	return c.conn.JetStream()
}

func (c *Client) Conn() *nats.Conn {
	return c.conn
}

func (c *Client) Publish(ctx context.Context, subject string, data []byte) error {
	return c.conn.Publish(subject, data)
}

func (c *Client) Request(ctx context.Context, subject string, data []byte, timeout time.Duration) (*nats.Msg, error) {
	return c.conn.Request(subject, data, timeout)
}

func (c *Client) Subscribe(subject string, handler nats.MsgHandler) (*nats.Subscription, error) {
	sub, err := c.conn.Subscribe(subject, handler)
	if err != nil {
		return nil, err
	}
	c.subs = append(c.subs, sub)
	return sub, nil
}

func (c *Client) QueueSubscribe(subject, queue string, handler nats.MsgHandler) (*nats.Subscription, error) {
	sub, err := c.conn.QueueSubscribe(subject, queue, handler)
	if err != nil {
		return nil, err
	}
	c.subs = append(c.subs, sub)
	return sub, nil
}

func (c *Client) IsConnected() bool {
	return c.conn != nil && c.conn.IsConnected()
}

func (c *Client) Close() {
	for _, sub := range c.subs {
		sub.Unsubscribe()
	}
	if c.conn != nil {
		c.conn.Drain()
		c.conn.Close()
	}
	close(c.done)
}

type Subjects struct {
	tenantID string
	agentID  string
}

func (s *Subjects) AgentHeartbeat() string {
	return fmt.Sprintf("tenant.%s.agent.%s.heartbeat", s.tenantID, s.agentID)
}

func (s *Subjects) AgentMetrics() string {
	return fmt.Sprintf("tenant.%s.agent.%s.metrics", s.tenantID, s.agentID)
}

func (s *Subjects) AgentEvents() string {
	return fmt.Sprintf("tenant.%s.agent.%s.events", s.tenantID, s.agentID)
}

func (s *Subjects) AgentCommands() string {
	return fmt.Sprintf("tenant.%s.cmd.%s", s.tenantID, s.agentID)
}

func (s *Subjects) AgentConfig() string {
	return fmt.Sprintf("tenant.%s.config.%s", s.tenantID, s.agentID)
}

func (s *Subjects) TenantHeartbeat() string {
	return fmt.Sprintf("tenant.%s.heartbeat", s.tenantID)
}

func (s *Subjects) PlatformAlerts() string {
	return fmt.Sprintf("tenant.%s.alerts", s.tenantID)
}

type CommsHandler struct {
	client   *Client
	store    *core.Store
	subjects *Subjects
	logger   core.Logger
	stopCh   chan struct{}
}

func NewCommsHandler(client *Client, store *core.Store, logger core.Logger) *CommsHandler {
	return &CommsHandler{
		client:   client,
		store:    store,
		subjects: client.Subjects(),
		logger:   logger,
		stopCh:   make(chan struct{}),
	}
}

func (h *CommsHandler) Start(ctx context.Context) error {
	h.replayQueued(ctx)
	h.sendHeartbeat(ctx)
	go h.heartbeatLoop(ctx)
	go h.replayLoop(ctx)
	return nil
}

func (h *CommsHandler) Stop() {
	close(h.stopCh)
}

func (h *CommsHandler) PublishMetrics(ctx context.Context, samples []core.MetricSample) {
	if !h.client.IsConnected() {
		h.queueMetrics(ctx, samples)
		return
	}

	payload := encodeMetrics(samples)
	if err := h.client.Publish(ctx, h.subjects.AgentMetrics(), payload); err != nil {
		h.logger.Error("publishing metrics", "error", err)
		h.queueMetrics(ctx, samples)
	}
}

func (h *CommsHandler) PublishEvent(ctx context.Context, event core.Event) {
	if !h.client.IsConnected() {
		h.queueEvent(ctx, event)
		return
	}

	payload := encodeEvent(event)
	if err := h.client.Publish(ctx, h.subjects.AgentEvents(), payload); err != nil {
		h.logger.Error("publishing event", "error", err)
		h.queueEvent(ctx, event)
	}
}

func (h *CommsHandler) queueMetrics(ctx context.Context, samples []core.MetricSample) {
	for _, s := range samples {
		sm := core.StoredMetric{
			Name:      s.Name,
			Value:     s.Value,
			Tags:      s.Tags,
			Timestamp: s.Timestamp,
		}
		if err := h.store.QueueMetric(sm); err != nil {
			h.logger.Error("queuing metric", "error", err)
		}
	}
}

func (h *CommsHandler) queueEvent(ctx context.Context, event core.Event) {
	se := core.StoredEvent{
		Type:      event.Type,
		Message:   event.Message,
		Tags:      event.Tags,
		Timestamp: event.Timestamp,
	}
	if err := h.store.QueueEvent(se); err != nil {
		h.logger.Error("queuing event", "error", err)
	}
}

func (h *CommsHandler) replayQueued(ctx context.Context) {
	if !h.client.IsConnected() {
		return
	}
	metrics, err := h.store.PeekMetrics(1000)
	if err != nil {
		h.logger.Error("reading queued metrics", "error", err)
		return
	}
	events, err := h.store.PeekEvents(1000)
	if err != nil {
		h.logger.Error("reading queued events", "error", err)
		return
	}

	metricsReplayed := 0
	eventsReplayed := 0
	for _, queued := range metrics {
		m := queued.Metric
		sample := core.MetricSample{
			Name: m.Name, Value: m.Value,
			Tags: m.Tags, Timestamp: m.Timestamp,
		}
		payload := encodeMetrics([]core.MetricSample{sample})
		if err := h.client.Publish(ctx, h.subjects.AgentMetrics(), payload); err != nil {
			h.logger.Warn("replaying queued metric", "error", err)
			break
		}
		if err := h.store.AckMetrics([]string{queued.Key}); err != nil {
			h.logger.Error("acknowledging queued metric", "error", err)
			break
		}
		metricsReplayed++
	}
	for _, queued := range events {
		e := queued.Event
		event := core.Event{
			Type: e.Type, Message: e.Message,
			Tags: e.Tags, Timestamp: e.Timestamp,
		}
		payload := encodeEvent(event)
		if err := h.client.Publish(ctx, h.subjects.AgentEvents(), payload); err != nil {
			h.logger.Warn("replaying queued event", "error", err)
			break
		}
		if err := h.store.AckEvents([]string{queued.Key}); err != nil {
			h.logger.Error("acknowledging queued event", "error", err)
			break
		}
		eventsReplayed++
	}

	if metricsReplayed > 0 || eventsReplayed > 0 {
		h.logger.Info("replayed queued data",
			"metrics", metricsReplayed,
			"events", eventsReplayed,
		)
	}
}

func (h *CommsHandler) replayLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-h.stopCh:
			return
		case <-ticker.C:
			h.replayQueued(ctx)
		}
	}
}

func (h *CommsHandler) heartbeatLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-h.stopCh:
			return
		case <-ticker.C:
			h.sendHeartbeat(ctx)
		}
	}
}

func (h *CommsHandler) sendHeartbeat(ctx context.Context) {
	if !h.client.IsConnected() {
		return
	}
	hb := encodeHeartbeat(h.client.ident.AgentID)
	h.client.Publish(ctx, h.subjects.AgentHeartbeat(), hb)
}

func encodeMetrics(samples []core.MetricSample) []byte {
	type wireMetric struct {
		Name      string            `json:"name"`
		Value     float64           `json:"value"`
		Tags      map[string]string `json:"tags,omitempty"`
		Timestamp int64             `json:"timestamp"`
	}
	wireSamples := make([]wireMetric, 0, len(samples))
	for _, sample := range samples {
		wireSamples = append(wireSamples, wireMetric{
			Name: sample.Name, Value: sample.Value, Tags: sample.Tags,
			Timestamp: sample.Timestamp.Unix(),
		})
	}
	return mustJSON(map[string]interface{}{
		"samples": wireSamples,
	})
}

func encodeEvent(event core.Event) []byte {
	return mustJSON(map[string]interface{}{
		"type":      event.Type,
		"message":   event.Message,
		"tags":      event.Tags,
		"timestamp": event.Timestamp.Unix(),
	})
}

func encodeHeartbeat(agentID string) []byte {
	return mustJSON(map[string]interface{}{
		"agent_id": agentID,
		"time":     time.Now().UTC().Unix(),
		"status":   "ok",
	})
}

func mustJSON(v interface{}) []byte {
	data, err := json.Marshal(v)
	if err != nil {
		return []byte("{}")
	}
	return data
}
