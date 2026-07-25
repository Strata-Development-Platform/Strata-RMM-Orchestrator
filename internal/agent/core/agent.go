package core

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

type AgentState int

const (
	StateStopped AgentState = iota
	StateStarting
	StateRunning
	StateStopping
)

type Agent struct {
	mu       sync.RWMutex
	state    AgentState
	cfg      *Config
	logger   Logger
	identity *Identity
	store    *Store
	started  time.Time

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

type Logger interface {
	Info(msg string, keysAndValues ...interface{})
	Error(msg string, keysAndValues ...interface{})
	Warn(msg string, keysAndValues ...interface{})
	Debug(msg string, keysAndValues ...interface{})
}

type Collector interface {
	Name() string
	Interval() time.Duration
	Collect(ctx context.Context) ([]MetricSample, error)
	Start(ctx context.Context) error
	Stop() error
}

type MetricSample struct {
	Name      string
	Value     float64
	Tags      map[string]string
	Timestamp time.Time
}

type Event struct {
	Type      string
	Message   string
	Tags      map[string]string
	Timestamp time.Time
}

func New(cfg *Config, logger Logger) *Agent {
	return &Agent{
		cfg:    cfg,
		logger: logger,
		state:  StateStopped,
	}
}

func (a *Agent) Start(ctx context.Context) error {
	a.mu.Lock()
	if a.state != StateStopped {
		a.mu.Unlock()
		return fmt.Errorf("agent already started (state: %v)", a.state)
	}
	a.state = StateStarting
	a.mu.Unlock()

	defer func() {
		a.mu.Lock()
		if a.state == StateStarting {
			a.state = StateStopped
		}
		a.mu.Unlock()
	}()

	if err := a.cfg.Validate(); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}

	os.MkdirAll(a.cfg.Agent.DataDir, 0700)

	identMgr := NewIdentityManager(a.cfg.Agent.DataDir)
	ident, err := identMgr.LoadOrCreate(a.cfg.Agent.TenantID, a.cfg.Agent.EnrollmentToken)
	if err != nil {
		return fmt.Errorf("identity setup: %w", err)
	}
	a.identity = ident
	a.logger.Info("agent identity established", "agent_id", ident.AgentID, "tenant_id", ident.TenantID)

	store, err := NewStore(a.cfg.Store.Path)
	if err != nil {
		return fmt.Errorf("store setup: %w", err)
	}
	a.store = store

	ctx, a.cancel = context.WithCancel(ctx)
	a.started = time.Now()

	a.mu.Lock()
	a.state = StateRunning
	a.mu.Unlock()

	a.logger.Info("agent started successfully",
		"agent_id", a.identity.AgentID,
		"tenant_id", a.identity.TenantID,
	)

	return nil
}

func (a *Agent) Stop() error {
	a.mu.Lock()
	if a.state != StateRunning {
		a.mu.Unlock()
		return nil
	}
	a.state = StateStopping
	a.mu.Unlock()

	if a.cancel != nil {
		a.cancel()
	}

	a.wg.Wait()

	if a.store != nil {
		a.store.Close()
	}

	a.mu.Lock()
	a.state = StateStopped
	a.mu.Unlock()

	a.logger.Info("agent stopped")
	return nil
}

func (a *Agent) Run(ctx context.Context) error {
	if err := a.Start(ctx); err != nil {
		return err
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-ctx.Done():
	case <-sigCh:
	}

	return a.Stop()
}

func (a *Agent) State() AgentState {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.state
}

func (a *Agent) Identity() *Identity {
	return a.identity
}

func (a *Agent) Config() *Config {
	return a.cfg
}

func (a *Agent) Store() *Store {
	return a.store
}

func (a *Agent) Uptime() time.Duration {
	return time.Since(a.started)
}

func (a *Agent) Health() map[string]interface{} {
	a.mu.RLock()
	defer a.mu.RUnlock()

	queueSize := 0
	if a.store != nil {
		qs, err := a.store.QueueSize()
		if err == nil {
			queueSize = qs
		}
	}

	return map[string]interface{}{
		"state":      a.state,
		"agent_id":   a.identity.AgentID,
		"tenant_id":  a.identity.TenantID,
		"uptime":     a.Uptime().String(),
		"queue_size": queueSize,
	}
}
