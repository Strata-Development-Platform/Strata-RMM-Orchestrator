package jobs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"go.uber.org/zap"

	jsmsg "github.com/strata-rmm/strata-rmm-orchestrator/internal/messaging/jetstream"
)

type CommandHandler func(ctx context.Context, cmd *CommandEnvelope) (status string, exitCode int, result []byte, err error)

type HandlerRegistry struct {
	mu       sync.RWMutex
	handlers map[string]CommandHandler
}

func NewHandlerRegistry() *HandlerRegistry {
	return &HandlerRegistry{handlers: make(map[string]CommandHandler)}
}
func (r *HandlerRegistry) Register(commandType string, handler CommandHandler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.handlers[commandType] = handler
}
func (r *HandlerRegistry) Get(commandType string) (CommandHandler, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	handler, ok := r.handlers[commandType]
	return handler, ok
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

type resultReceipt struct {
	MessageID string `json:"message_id"`
}
type cancelEnvelope struct {
	EventID  string `json:"event_id"`
	TargetID string `json:"target_id"`
}

type JobDispatcher struct {
	nc       *nats.Conn
	ledger   *ReceiptLedger
	registry *HandlerRegistry
	logger   *zap.Logger
	tenantID string
	agentID  string

	mu      sync.Mutex
	cancels map[string]context.CancelFunc
	subs    []*nats.Subscription
	wg      sync.WaitGroup
	stop    context.CancelFunc
}

func NewJobDispatcher(nc *nats.Conn, ledger *ReceiptLedger, registry *HandlerRegistry, logger *zap.Logger, tenantID, agentID string) *JobDispatcher {
	return &JobDispatcher{nc: nc, ledger: ledger, registry: registry, logger: logger, tenantID: tenantID, agentID: agentID, cancels: make(map[string]context.CancelFunc)}
}

func genericJobDurableName(tenantID, agentID string) string {
	sum := sha256.Sum256([]byte(tenantID + "\x00" + agentID + "\x00generic-jobs"))
	return "jobs_" + hex.EncodeToString(sum[:12])
}

func (d *JobDispatcher) Start(ctx context.Context) error {
	runCtx, stop := context.WithCancel(ctx)
	d.stop = stop

	// Result receipts and cancellation requests are transient control messages.
	// Command delivery itself is JetStream durable below so endpoint disconnects
	// cannot lose queued work.
	handlers := []struct {
		subject string
		handler nats.MsgHandler
	}{
		{fmt.Sprintf("tenant.%s.agent.%s.result.ack", d.tenantID, d.agentID), func(msg *nats.Msg) { d.handleResultReceipt(msg.Data) }},
		{fmt.Sprintf("tenant.%s.cmd.%s.cancel", d.tenantID, d.agentID), func(msg *nats.Msg) { d.handleCancellation(msg.Data) }},
	}
	for _, item := range handlers {
		sub, err := d.nc.Subscribe(item.subject, item.handler)
		if err != nil {
			_ = d.Stop()
			return fmt.Errorf("subscribe %s: %w", item.subject, err)
		}
		d.subs = append(d.subs, sub)
	}

	js, err := d.nc.JetStream()
	if err != nil {
		_ = d.Stop()
		return fmt.Errorf("generic job JetStream context: %w", err)
	}
	commandSubject := fmt.Sprintf("tenant.%s.cmd.%s", d.tenantID, d.agentID)
	commandSub, err := js.Subscribe(commandSubject, func(msg *nats.Msg) {
		d.wg.Add(1)
		go func() {
			defer d.wg.Done()
			d.handleDurableCommand(runCtx, msg)
		}()
	},
		nats.Durable(genericJobDurableName(d.tenantID, d.agentID)),
		nats.ManualAck(),
		nats.AckExplicit(),
		nats.DeliverAll(),
		nats.BindStream(jsmsg.StreamCommands),
	)
	if err != nil {
		_ = d.Stop()
		return fmt.Errorf("subscribe durable generic commands: %w", err)
	}
	d.subs = append(d.subs, commandSub)

	if err := d.nc.Flush(); err != nil {
		_ = d.Stop()
		return fmt.Errorf("flush subscriptions: %w", err)
	}
	if err := d.replayResults(runCtx); err != nil {
		d.logger.Warn("replay results", zap.Error(err))
	}
	d.wg.Add(1)
	go d.resultReplayLoop(runCtx)
	d.logger.Info("durable job dispatcher started")
	return nil
}

func (d *JobDispatcher) Stop() error {
	if d.stop != nil {
		d.stop()
	}
	d.mu.Lock()
	for _, cancel := range d.cancels {
		cancel()
	}
	d.cancels = make(map[string]context.CancelFunc)
	d.mu.Unlock()
	var first error
	for _, sub := range d.subs {
		if err := sub.Unsubscribe(); err != nil && first == nil {
			first = err
		}
	}
	d.wg.Wait()
	return first
}

// handleCommand is retained as a direct package boundary for focused unit tests.
// Production command delivery enters through handleDurableCommand.
func (d *JobDispatcher) handleCommand(parent context.Context, data []byte) {
	cmd, err := validateCommand(data, d.tenantID, d.agentID)
	if err != nil {
		d.logger.Warn("rejecting command", zap.Error(err))
		return
	}
	ackSubject := fmt.Sprintf("tenant.%s.agent.%s.ack", d.tenantID, d.agentID)
	if receipt, getErr := d.ledger.GetReceipt(cmd.EventID); getErr == nil {
		if err := PublishAcknowledgement(d.nc, ackSubject, cmd.EventID, cmd.JobID, cmd.TargetID, cmd.MSPID, cmd.DeviceID, d.agentID, cmd.CorrelationID, cmd.Attempt, "duplicate"); err != nil {
			d.logger.Error("publish duplicate acknowledgement", zap.Error(err))
		}
		if len(receipt.ResultEnvelope) > 0 && !receipt.ResultAcked {
			if err := d.nc.Publish(fmt.Sprintf("tenant.%s.agent.%s.result", d.tenantID, d.agentID), receipt.ResultEnvelope); err != nil {
				d.logger.Error("republish duplicate result", zap.Error(err))
			}
		}
		return
	}

	receipt := &CommandReceipt{
		EventID: cmd.EventID, JobID: cmd.JobID, TargetID: cmd.TargetID, MSPID: cmd.MSPID,
		ClientID: cmd.ClientID, SiteID: cmd.SiteID, DeviceID: cmd.DeviceID, AgentID: d.agentID,
		CorrelationID: cmd.CorrelationID, Attempt: cmd.Attempt, CommandType: cmd.CommandType,
		ReceivedAt: time.Now().UTC().Format(time.RFC3339), State: StateReceived,
	}
	if err := d.ledger.RecordReceipt(receipt); err != nil {
		d.logger.Error("persist command before acknowledgement", zap.Error(err))
		return
	}
	if err := PublishAcknowledgement(d.nc, ackSubject, cmd.EventID, cmd.JobID, cmd.TargetID, cmd.MSPID, cmd.DeviceID, d.agentID, cmd.CorrelationID, cmd.Attempt, "accepted"); err != nil {
		d.logger.Error("publish accepted acknowledgement", zap.Error(err))
		return
	}
	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		d.execute(parent, *cmd)
	}()
}

func (d *JobDispatcher) handleDurableCommand(parent context.Context, msg *nats.Msg) {
	cmd, err := validateCommand(msg.Data, d.tenantID, d.agentID)
	if err != nil {
		d.logger.Warn("terminating invalid durable command", zap.Error(err))
		_ = msg.Term()
		return
	}

	progressDone := make(chan struct{})
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-progressDone:
				return
			case <-parent.Done():
				return
			case <-ticker.C:
				_ = msg.InProgress()
			}
		}
	}()
	defer close(progressDone)

	ackSubject := fmt.Sprintf("tenant.%s.agent.%s.ack", d.tenantID, d.agentID)
	receipt, getErr := d.ledger.GetReceipt(cmd.EventID)
	if getErr == nil {
		if err := PublishAcknowledgement(d.nc, ackSubject, cmd.EventID, cmd.JobID, cmd.TargetID, cmd.MSPID, cmd.DeviceID, d.agentID, cmd.CorrelationID, cmd.Attempt, "duplicate"); err != nil {
			d.logger.Error("publish duplicate acknowledgement", zap.Error(err))
			_ = msg.Nak()
			return
		}
		if len(receipt.ResultEnvelope) > 0 {
			if !receipt.ResultAcked {
				if err := d.nc.Publish(fmt.Sprintf("tenant.%s.agent.%s.result", d.tenantID, d.agentID), receipt.ResultEnvelope); err != nil {
					d.logger.Error("republish duplicate result", zap.Error(err))
					_ = msg.Nak()
					return
				}
			}
			_ = msg.Ack()
			return
		}

		switch receipt.State {
		case StateReceived:
			d.execute(parent, *cmd)
		case StateRunning:
			d.mu.Lock()
			_, active := d.cancels[cmd.EventID]
			d.mu.Unlock()
			if active {
				_ = msg.InProgress()
				return
			}
			// A running receipt with no in-process execution is an ambiguous crash
			// boundary. Never repeat the endpoint side effect automatically.
			d.publishTerminal(*cmd, StateFailed, -1, nil, "execution outcome unknown after agent restart; reconciliation required")
		case StateCancelled:
			d.publishTerminal(*cmd, StateCancelled, -1, nil, "command cancelled before execution")
		default:
			_ = msg.Nak()
			return
		}
	} else {
		receipt = &CommandReceipt{
			EventID: cmd.EventID, JobID: cmd.JobID, TargetID: cmd.TargetID, MSPID: cmd.MSPID,
			ClientID: cmd.ClientID, SiteID: cmd.SiteID, DeviceID: cmd.DeviceID, AgentID: d.agentID,
			CorrelationID: cmd.CorrelationID, Attempt: cmd.Attempt, CommandType: cmd.CommandType,
			ReceivedAt: time.Now().UTC().Format(time.RFC3339), State: StateReceived,
		}
		if err := d.ledger.RecordReceipt(receipt); err != nil {
			d.logger.Error("persist durable command before acknowledgement", zap.Error(err))
			_ = msg.Nak()
			return
		}
		if err := PublishAcknowledgement(d.nc, ackSubject, cmd.EventID, cmd.JobID, cmd.TargetID, cmd.MSPID, cmd.DeviceID, d.agentID, cmd.CorrelationID, cmd.Attempt, "accepted"); err != nil {
			d.logger.Error("publish durable command acknowledgement", zap.Error(err))
			_ = msg.Nak()
			return
		}
		d.execute(parent, *cmd)
	}

	terminal, err := d.ledger.GetReceipt(cmd.EventID)
	if err != nil || len(terminal.ResultEnvelope) == 0 {
		if err != nil {
			d.logger.Error("read durable terminal receipt", zap.Error(err))
		}
		_ = msg.Nak()
		return
	}
	_ = msg.Ack()
}

func validateCommand(data []byte, tenantID, agentID string) (*CommandEnvelope, error) {
	var cmd CommandEnvelope
	if err := json.Unmarshal(data, &cmd); err != nil {
		return nil, fmt.Errorf("malformed command: %w", err)
	}
	if cmd.SchemaVersion != 1 || cmd.EventID == "" || cmd.JobID == "" || cmd.TargetID == "" ||
		cmd.DeviceID == "" || cmd.CorrelationID == "" || cmd.CommandType == "" || cmd.Attempt < 1 {
		return nil, fmt.Errorf("invalid command envelope")
	}
	if cmd.MSPID != tenantID || (cmd.AgentID != "" && cmd.AgentID != agentID) {
		return nil, fmt.Errorf("command ownership mismatch")
	}
	if cmd.ExpiresAt != "" {
		expiresAt, err := time.Parse(time.RFC3339, cmd.ExpiresAt)
		if err != nil || !time.Now().Before(expiresAt) {
			return nil, fmt.Errorf("command expired or has invalid expiry")
		}
	}
	return &cmd, nil
}

func (d *JobDispatcher) execute(parent context.Context, cmd CommandEnvelope) {
	handler, ok := d.registry.Get(cmd.CommandType)
	if !ok {
		d.publishTerminal(cmd, StateFailed, -1, nil, "unsupported command type")
		return
	}
	ctx, cancel := context.WithCancel(parent)
	if cmd.ExpiresAt != "" {
		if deadline, err := time.Parse(time.RFC3339, cmd.ExpiresAt); err == nil {
			ctx, cancel = context.WithDeadline(parent, deadline)
		}
	}
	d.mu.Lock()
	d.cancels[cmd.EventID] = cancel
	d.cancels[cmd.TargetID] = cancel
	d.mu.Unlock()
	defer func() {
		cancel()
		d.mu.Lock()
		delete(d.cancels, cmd.EventID)
		delete(d.cancels, cmd.TargetID)
		d.mu.Unlock()
	}()

	started, err := d.ledger.BeginExecution(cmd.EventID)
	if err != nil {
		d.logger.Error("mark command running", zap.Error(err))
		return
	}
	if !started {
		receipt, getErr := d.ledger.GetReceipt(cmd.EventID)
		if getErr != nil {
			d.logger.Error("read command state before execution", zap.Error(getErr))
			return
		}
		if receipt.State == StateCancelled {
			d.publishTerminal(cmd, StateCancelled, -1, nil, "command cancelled before execution")
		}
		return
	}

	startedAt := time.Now()
	status, exitCode, result, runErr := handler(ctx, &cmd)
	errorText := ""
	if runErr != nil {
		status, errorText = StateFailed, runErr.Error()
	}
	if ctx.Err() == context.Canceled {
		status, errorText = StateCancelled, "command cancelled"
	} else if ctx.Err() == context.DeadlineExceeded {
		status, errorText = StateExpired, "command deadline exceeded"
	}
	if status != StateSucceeded && status != StateFailed && status != StateCancelled && status != StateExpired {
		status, errorText = StateFailed, "handler returned invalid status"
	}
	d.publishTerminalAt(cmd, status, exitCode, result, errorText, startedAt)
}

func (d *JobDispatcher) publishTerminal(cmd CommandEnvelope, status string, exitCode int, result []byte, errorText string) {
	d.publishTerminalAt(cmd, status, exitCode, result, errorText, time.Now())
}
func (d *JobDispatcher) publishTerminalAt(cmd CommandEnvelope, status string, exitCode int, result []byte, errorText string, startedAt time.Time) {
	msgID, envelope, err := MarshalResult("", cmd.EventID, cmd.JobID, cmd.TargetID, cmd.MSPID, cmd.ClientID, cmd.SiteID, cmd.DeviceID, d.agentID, cmd.CorrelationID, cmd.Attempt, status, exitCode, result, errorText, startedAt, time.Now())
	if err != nil {
		d.logger.Error("marshal command result", zap.Error(err))
		return
	}
	// Persist the exact result before attempting delivery.
	if err := d.ledger.MarkComplete(cmd.EventID, status, msgID, envelope); err != nil {
		d.logger.Error("persist command result", zap.Error(err))
		return
	}
	if err := d.nc.Publish(fmt.Sprintf("tenant.%s.agent.%s.result", d.tenantID, d.agentID), envelope); err != nil {
		d.logger.Warn("publish command result; retained for replay", zap.Error(err))
	}
}

func (d *JobDispatcher) handleResultReceipt(data []byte) {
	var receipt resultReceipt
	if err := json.Unmarshal(data, &receipt); err != nil || receipt.MessageID == "" {
		d.logger.Warn("invalid result receipt")
		return
	}
	if err := d.ledger.MarkResultAcknowledged(receipt.MessageID); err != nil {
		d.logger.Warn("record result receipt", zap.Error(err))
	}
}
func (d *JobDispatcher) handleCancellation(data []byte) {
	var request cancelEnvelope
	if err := json.Unmarshal(data, &request); err != nil || (request.EventID == "" && request.TargetID == "") {
		return
	}
	d.mu.Lock()
	cancel := d.cancels[request.EventID]
	if cancel == nil {
		cancel = d.cancels[request.TargetID]
	}
	d.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if err := d.ledger.MarkCancelled(request.EventID, request.TargetID); err != nil {
		d.logger.Warn("persist command cancellation", zap.Error(err))
	}
}
func (d *JobDispatcher) replayResults(ctx context.Context) error {
	receipts, err := d.ledger.GetUnacknowledgedResults()
	if err != nil {
		return err
	}
	for _, receipt := range receipts {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := d.nc.Publish(fmt.Sprintf("tenant.%s.agent.%s.result", d.tenantID, d.agentID), receipt.ResultEnvelope); err != nil {
			return err
		}
	}
	return nil
}

func (d *JobDispatcher) resultReplayLoop(ctx context.Context) {
	defer d.wg.Done()
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := d.replayResults(ctx); err != nil && ctx.Err() == nil {
				d.logger.Warn("periodic result replay", zap.Error(err))
			}
		}
	}
}
