package platform

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"
)

// PolicyScheduler periodically triggers policy re-evaluation.
type PolicyScheduler struct {
	interval time.Duration
	engine   *PolicyEnforcementEngine
	logger   *zap.Logger
	stopCh   chan struct{}
	mu       sync.Mutex
	running  bool
}

// NewPolicyScheduler creates a new policy scheduler.
func NewPolicyScheduler(interval time.Duration, engine *PolicyEnforcementEngine, logger *zap.Logger) *PolicyScheduler {
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	return &PolicyScheduler{
		interval: interval,
		engine:   engine,
		logger:   logger,
		stopCh:   make(chan struct{}),
	}
}

// Start begins the periodic policy re-evaluation loop.
func (s *PolicyScheduler) Start(ctx context.Context) {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.mu.Unlock()

	go s.runLoop(ctx)
}

// Stop signals the scheduler to stop.
func (s *PolicyScheduler) Stop() {
	close(s.stopCh)
	s.mu.Lock()
	s.running = false
	s.mu.Unlock()
}

func (s *PolicyScheduler) runLoop(ctx context.Context) {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	s.logger.Info("policy scheduler started", zap.Duration("interval", s.interval))

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("policy scheduler stopped: context done")
			return
		case <-s.stopCh:
			s.logger.Info("policy scheduler stopped")
			return
		case <-ticker.C:
			s.evaluatePolicies(ctx)
		}
	}
}

// evaluatePolicies triggers a full policy re-evaluation cycle.
func (s *PolicyScheduler) evaluatePolicies(ctx context.Context) {
	start := time.Now()

	// Get all MSP IDs that have active policies
	mspIDs, err := s.getActiveMSPIDs(ctx)
	if err != nil {
		s.logger.Error("getting active MSPs", zap.Error(err))
		return
	}

	for _, mspID := range mspIDs {
		select {
		case <-ctx.Done():
			return
		default:
			if err := s.engine.ApplyPoliciesToDevices(ctx, mspID); err != nil {
				s.logger.Error("evaluating policies for MSP", zap.String("msp", mspID), zap.Error(err))
			}
		}
	}

	elapsed := time.Since(start)
	s.logger.Info("policy evaluation cycle complete",
		zap.Int("msp_count", len(mspIDs)),
		zap.Duration("duration", elapsed))
}

func (s *PolicyScheduler) getActiveMSPIDs(ctx context.Context) ([]string, error) {
	rows, err := s.engine.db.QueryContext(ctx, `
		SELECT DISTINCT msp_id FROM policies WHERE status = 'active' AND published_version IS NOT NULL
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return ids, nil
}
