package monitoring

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/timescale"
)

type batchBuffer struct {
	metrics []timescale.MetricRow
	events  []timescale.EventRow
	mu      sync.Mutex
	ticker  *time.Ticker
	done    chan struct{}
}

func NewBatchBuffer(tickerDuration time.Duration) *batchBuffer {
	return &batchBuffer{ticker: time.NewTicker(tickerDuration), done: make(chan struct{})}
}

func (b *batchBuffer) AddMetric(row timescale.MetricRow) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.metrics = append(b.metrics, row)
}

func (b *batchBuffer) AddEvent(row timescale.EventRow) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.events = append(b.events, row)
}

// Flush retains any batch that fails persistence. It is intentionally conservative:
// no queued rows are discarded until the corresponding Timescale/PostgreSQL write
// succeeds.
func (b *batchBuffer) Flush(ctx context.Context, tsdb *timescale.Client) error {
	b.mu.Lock()
	metrics := append([]timescale.MetricRow(nil), b.metrics...)
	events := append([]timescale.EventRow(nil), b.events...)
	b.mu.Unlock()

	var errs []string
	metricsOK := len(metrics) == 0
	eventsOK := len(events) == 0

	if len(metrics) > 0 {
		if err := tsdb.InsertMetrics(ctx, metrics); err != nil {
			errs = append(errs, fmt.Sprintf("insert metrics batch (%d rows): %v", len(metrics), err))
		} else {
			metricsOK = true
		}
	}
	if len(events) > 0 {
		if err := tsdb.InsertEvents(ctx, events); err != nil {
			errs = append(errs, fmt.Sprintf("insert events batch (%d rows): %v", len(events), err))
		} else {
			eventsOK = true
		}
	}

	b.mu.Lock()
	if metricsOK && len(metrics) > 0 && len(b.metrics) >= len(metrics) {
		b.metrics = append([]timescale.MetricRow(nil), b.metrics[len(metrics):]...)
	}
	if eventsOK && len(events) > 0 && len(b.events) >= len(events) {
		b.events = append([]timescale.EventRow(nil), b.events[len(events):]...)
	}
	b.mu.Unlock()

	if len(errs) > 0 {
		return fmt.Errorf("batch flush errors: %s", strings.Join(errs, "; "))
	}
	return nil
}

func (b *batchBuffer) StartLoop(ctx context.Context, tsdb *timescale.Client) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-b.done:
				return
			case <-b.ticker.C:
				_ = b.Flush(ctx, tsdb)
			}
		}
	}()
}

func (b *batchBuffer) Stop() {
	select {
	case <-b.done:
		return
	default:
		close(b.done)
		b.ticker.Stop()
	}
}
