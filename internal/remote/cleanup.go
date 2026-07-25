package remote

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/storage"
)

type CleanupJob struct {
	recStore *RecordingStore
	backend  storage.Backend
	logger   *zap.Logger
	interval time.Duration
	mu       sync.Mutex
	running  bool
	stopCh   chan struct{}
}

func NewCleanupJob(recStore *RecordingStore, backend storage.Backend, logger *zap.Logger) *CleanupJob {
	return &CleanupJob{
		recStore: recStore,
		backend:  backend,
		logger:   logger,
		interval: 24 * time.Hour,
		stopCh:   make(chan struct{}),
	}
}

func (j *CleanupJob) WithInterval(d time.Duration) *CleanupJob {
	j.interval = d
	return j
}

func (j *CleanupJob) Start(ctx context.Context) {
	j.mu.Lock()
	if j.running {
		j.mu.Unlock()
		return
	}
	j.running = true
	j.mu.Unlock()

	go func() {
		j.logger.Info("recording cleanup job started", zap.Duration("interval", j.interval))

		j.runOnce(ctx)

		ticker := time.NewTicker(j.interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				j.runOnce(ctx)
			case <-ctx.Done():
				j.logger.Info("recording cleanup job stopped")
				j.mu.Lock()
				j.running = false
				j.mu.Unlock()
				return
			}
		}
	}()
}

func (j *CleanupJob) Stop() {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.running {
		close(j.stopCh)
		j.running = false
	}
}

func (j *CleanupJob) runOnce(ctx context.Context) {
	expiredIDs, err := j.recStore.DeleteExpired()
	if err != nil {
		j.logger.Error("fetch expired recordings", zap.Error(err))
		return
	}

	if len(expiredIDs) == 0 {
		return
	}

	j.logger.Info("cleaning up expired recordings", zap.Int("count", len(expiredIDs)))

	for _, id := range expiredIDs {
		rec, err := j.recStore.GetByID(id)
		if err != nil {
			continue
		}

		if err := j.backend.Delete(ctx, rec.StorageKey); err != nil {
			j.logger.Warn("delete expired recording from storage",
				zap.String("recording_id", id),
				zap.String("key", rec.StorageKey),
				zap.Error(err),
			)
		}

		if err := j.recStore.Delete(id); err != nil {
			j.logger.Error("delete expired recording from db",
				zap.String("recording_id", id),
				zap.Error(err),
			)
		}
	}
}
