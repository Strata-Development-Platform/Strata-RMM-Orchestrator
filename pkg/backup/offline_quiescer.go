package backup

import (
	"context"
	"errors"
	"sync"
)

// OfflineTargetQuiescer is used only for restore into an explicitly separate,
// non-serving recovery environment. The caller must prove target separation
// before constructing it.
type OfflineTargetQuiescer struct {
	mu       sync.RWMutex
	targetID string
	closed   bool
}

func NewOfflineTargetQuiescer(targetID string) (*OfflineTargetQuiescer, error) {
	if targetID == "" {
		return nil, errors.New("offline recovery target identity is required")
	}
	return &OfflineTargetQuiescer{targetID: targetID}, nil
}

func (q *OfflineTargetQuiescer) Quiesce(context.Context) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.closed = true
	return nil
}

func (q *OfflineTargetQuiescer) Resume(context.Context) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.closed = false
	return nil
}

func (q *OfflineTargetQuiescer) Status(context.Context) (QuiesceStatus, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return QuiesceStatus{
		Quiesced:   q.closed,
		Components: []string{"offline-target:" + q.targetID},
	}, nil
}
