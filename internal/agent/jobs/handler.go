package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"go.uber.org/zap"
)

type CommandHandler func(ctx context.Context, cmd *CommandEnvelope) (status string, exitCode int, result []byte, err error)

type HandlerRegistry struct {
	mu       sync.RWMutex
	handlers map[string]CommandHandler
}

func NewHandlerRegistry() *HandlerRegistry {
	return &HandlerRegistry{
		handlers: make(map[string]CommandHandler),
	}
}

func (r *HandlerRegistry) Register(commandType string, handler CommandHandler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.handlers[commandType] = handler
}

func (r *HandlerRegistry) Get(commandType string) (CommandHandler, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	h, ok := r.handlers[commandType]
	return h, ok
}

func (r *HandlerRegistry) IsSupported(commandType string) bool {
	_, ok := r.Get(commandType)
	return ok
}

type CommandEnvelope struct {
	SchemaVersion int             `json:"schema_version"`
	EventID       string          `json:"event_id"`
	JobID         string          `json:"job_id"`
	TargetID      string          `json:"target_id"`
	MSPID         string          `json:"msp_id"`
	ClientID      string          `json:"client_id,omitempty"`
	SiteID        string          `json:"site_id,omitempty"`
	DeviceID      string          `json:"device_id"`
	AgentID       string          `json:"agent_id,omitempty"`
	CorrelationID string          `json:"correlation_id"`
	Attempt       int             `json:"attempt"`
	IssuedAt      string          `json:"issued_at"`
	ExpiresAt     string          `json:"expires_at"`
	CommandType   string          `json:"command_type"`
	Payload       json.RawMessage `json:"payload"`
}

type JobDispatcher struct {
	nc       *nats.Conn
	ledger   *ReceiptLedger
	registry *HandlerRegistry
	logger   *zap.Logger
	tenantID string
	agentID  string
	mu       sync.Mutex
	ackSub   *nats.Subscription
	subs     []*nats.Subscription
}

func NewJobDispatcher(nc *nats.Conn, ledger *ReceiptLedger, registry *HandlerRegistry, logger *zap.Logger, tenantID, agentID string) *JobDispatcher {
	return &JobDispatcher{
		nc:       nc,
		ledger:   ledger,
		registry: registry,
		logger:   logger,
		tenantID: tenantID,
		agentID:  agentID,
	}
}

func (d *JobDispatcher) Start(ctx context.Context) error {
	subject := fmt.Sprintf("tenant.%s.cmd.%s", d.tenantID, d.agentID)
	sub, err := d.nc.Subscribe(subject, func(msg *nats.Msg) {
		d.handleCommand(msg.Data)
	})
	if err != nil {
		return fmt.Errorf("subscribe commands: %w", err)
	}
	d.subs = append(d.subs, sub)
	d.logger.Info("job dispatcher started", zap.String("subject", subject))

	// Replay unacknowledged results
	go d.replayResults(ctx)

	return nil
}

func (d *JobDispatcher) Stop() {
	for _, sub := range d.subs {
		sub.Unsubscribe()
	}
}

func (d *JobDispatcher) handleCommand(data []byte) {
	var cmd CommandEnvelope
	if err := json.Unmarshal(data, &cmd); err != nil {
		d.logger.Warn("malformed command", zap.Error(err))
		return
	}

	if cmd.SchemaVersion <= 0 || cmd.SchemaVersion > 1 {
		d.logger.Warn("unsupported schema version", zap.Int("version", cmd.SchemaVersion))
		return
	}

	if cmd.MSPID != d.tenantID {
		d.logger.Warn("msp_id mismatch", zap.String("got", cmd.MSPID), zap.String("expected", d.tenantID))
		return
	}

	if cmd.AgentID != "" && cmd.AgentID != d.agentID {
		d.logger.Warn("agent_id mismatch", zap.String("got", cmd.AgentID), zap.String("expected", d.agentID))
		return
	}

	if d.ledger.IsDuplicate(cmd.EventID) {
		receipt, err := d.ledger.GetReceipt(cmd.EventID)
		if err == nil && (receipt.State == StateSucceeded || receipt.State == StateFailed) {
			resendResult := map[string]interface{}{
				"schema_version": 1,
				"event_id":       cmd.EventID,
				"job_id":         cmd.JobID,
				"target_id":      cmd.TargetID,
				"msp_id":         cmd.MSPID,
				"device_id":      cmd.DeviceID,
				"agent_id":       d.agentID,
				"status":         receipt.State,
				"duplicate":      true,
			}
			resData, _ := json.Marshal(resendResult)
			ackSubject := fmt.Sprintf("tenant.%s.agent.%s.ack", d.tenantID, d.agentID)
			PublishAcknowledgement(d.nc, ackSubject, cmd.EventID, cmd.JobID, cmd.TargetID, cmd.MSPID, cmd.DeviceID, d.agentID, cmd.CorrelationID, cmd.Attempt, "duplicate")
			d.nc.Publish(fmt.Sprintf("tenant.%s.agent.%s.result", d.tenantID, d.agentID), resData)
		}
		return
	}

	ackSubject := fmt.Sprintf("tenant.%s.agent.%s.ack", d.tenantID, d.agentID)
	PublishAcknowledgement(d.nc, ackSubject, cmd.EventID, cmd.JobID, cmd.TargetID, cmd.MSPID, cmd.DeviceID, d.agentID, cmd.CorrelationID, cmd.Attempt, "accepted")

	receipt := &CommandReceipt{
		EventID:       cmd.EventID,
		JobID:         cmd.JobID,
		TargetID:     cmd.TargetID,
		CorrelationID: cmd.CorrelationID,
		Attempt:      cmd.Attempt,
		CommandType:  cmd.CommandType,
		ReceivedAt:   time.Now().UTC().Format(time.RFC3339),
		State:        StateReceived,
	}
	d.ledger.RecordReceipt(receipt)

	go d.execute(cmd)
}

func (d *JobDispatcher) execute(cmd CommandEnvelope) {
	handler, ok := d.registry.Get(cmd.CommandType)
	if !ok {
		ackSubject := fmt.Sprintf("tenant.%s.agent.%s.ack", d.tenantID, d.agentID)
		PublishAcknowledgement(d.nc, ackSubject, cmd.EventID, cmd.JobID, cmd.TargetID, cmd.MSPID, cmd.DeviceID, d.agentID, cmd.CorrelationID, cmd.Attempt, "unsupported")
		d.ledger.MarkComplete(cmd.EventID, StateFailed, "")
		return
	}

	d.ledger.MarkRunning(cmd.EventID)

	ctx := context.Background()
	status, exitCode, result, err := handler(ctx, &cmd)
	if err != nil {
		status = StateFailed
	}

	resultSubject := fmt.Sprintf("tenant.%s.agent.%s.result", d.tenantID, d.agentID)
	now := time.Now()
	startedAt, _ := time.Parse(time.RFC3339, cmd.IssuedAt)
	if startedAt.IsZero() {
		startedAt = now.Add(-time.Second)
	}

	msgID, pubErr := PublishResult(d.nc, resultSubject, "", cmd.EventID, cmd.JobID, cmd.TargetID,
		cmd.MSPID, cmd.DeviceID, d.agentID, cmd.CorrelationID, cmd.Attempt,
		status, exitCode, result, "", startedAt, now)

	finalStatus := status
	if pubErr != nil {
		finalStatus = StateSucceeded
	}

	d.ledger.MarkComplete(cmd.EventID, finalStatus, msgID)
}

func (d *JobDispatcher) replayResults(ctx context.Context) {
	receipts := d.ledger.GetUnacknowledgedResults()
	for _, receipt := range receipts {
		select {
		case <-ctx.Done():
			return
		default:
			resultSubject := fmt.Sprintf("tenant.%s.agent.%s.result", d.tenantID, d.agentID)
			resendData := map[string]interface{}{
				"schema_version": 1,
				"message_id":     receipt.ResultMsgID,
				"event_id":       receipt.EventID,
				"job_id":         receipt.JobID,
				"target_id":     receipt.TargetID,
				"msp_id":         d.tenantID,
				"device_id":      "",
				"agent_id":       d.agentID,
				"status":         receipt.State,
				"retransmitted":  true,
			}
			data, _ := json.Marshal(resendData)
			d.nc.Publish(resultSubject, data)
			d.logger.Info("retransmitted unacknowledged result", zap.String("event", receipt.EventID))
		}
	}
}
