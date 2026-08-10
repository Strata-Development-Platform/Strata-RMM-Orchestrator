package platform

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"
)

// SmartGroupSync runs periodic re-evaluation of all smart groups.
// It follows the same pattern as ScriptScheduleRunner.
type SmartGroupSync struct {
	interval time.Duration
	srv      *APIServer
	logger   *zap.Logger
	stopCh   chan struct{}
	doneCh   chan struct{}
	mu       sync.Mutex
	running  bool
}

// NewSmartGroupSync creates a new SmartGroupSync with the given evaluation interval.
// If interval is zero or negative, it defaults to 5 minutes.
func NewSmartGroupSync(interval time.Duration, srv *APIServer, logger *zap.Logger) *SmartGroupSync {
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	return &SmartGroupSync{
		interval: interval,
		srv:      srv,
		logger:   logger,
	}
}

// Start launches the background evaluation loop in a goroutine.
// Calling Start while already running is a no-op. A new lifecycle channel pair
// is allocated for each run so a stopped sync can be restarted safely.
func (s *SmartGroupSync) Start(ctx context.Context) {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	stopCh := make(chan struct{})
	doneCh := make(chan struct{})
	s.stopCh = stopCh
	s.doneCh = doneCh
	s.running = true
	s.mu.Unlock()

	s.logger.Info("smart group sync starting", zap.Duration("interval", s.interval))
	go s.runLoop(ctx, stopCh, doneCh)
}

// Stop signals the current background loop to exit and waits for that specific
// worker to finish. Calling Stop when not running is a no-op. The lifecycle
// mutex remains held through worker shutdown so Start and concurrent Stop calls
// cannot interleave with the channel close or reuse a stale lifecycle.
func (s *SmartGroupSync) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.running = false
	stopCh := s.stopCh
	doneCh := s.doneCh
	if stopCh != nil {
		close(stopCh)
	}
	if doneCh != nil {
		<-doneCh
	}
	s.mu.Unlock()

	s.logger.Info("smart group sync stopped")
}

func (s *SmartGroupSync) runLoop(ctx context.Context, stopCh <-chan struct{}, doneCh chan<- struct{}) {
	defer close(doneCh)
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			s.logger.Info("smart group sync: context done, exiting")
			return
		case <-stopCh:
			s.logger.Info("smart group sync: stop signal received, exiting")
			return
		case <-ticker.C:
			s.evaluateSmartGroups(ctx)
		}
	}
}

// evaluateSmartGroups is the public wrapper used by tests.
func (s *SmartGroupSync) evaluateSmartGroups(ctx context.Context) {
	if s.srv == nil {
		return
	}
	s.srv.evaluateAllSmartGroups(ctx)
}

// Running reports whether the sync loop is currently active.
func (s *SmartGroupSync) Running() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

// NewSmartGroupSyncTestHelper creates a sync with a custom interval for testing.
func NewSmartGroupSyncTestHelper(interval time.Duration, srv *APIServer, logger *zap.Logger) *SmartGroupSync {
	return NewSmartGroupSync(interval, srv, logger)
}
