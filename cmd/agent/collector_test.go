package agent

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/strata-rmm/strata-rmm-orchestrator/internal/agent/core"
)

func TestTenantIDFlagCannotBypassOrchestratorRegistration(t *testing.T) {
	cmd := NewCommand(context.Background(), zap.NewNop())
	cmd.SetArgs([]string{"--tenant-id", "tenant-a", "--enrollment-token", "untrusted-local-token"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "orchestrator registration is required") {
		t.Fatalf("Execute() error = %v, want orchestrator registration requirement", err)
	}
}

type testCollector struct {
	mu    sync.Mutex
	calls int
}

func (c *testCollector) Interval() time.Duration { return 20 * time.Millisecond }

func (c *testCollector) Collect(context.Context) ([]core.MetricSample, error) {
	c.mu.Lock()
	c.calls++
	c.mu.Unlock()
	return []core.MetricSample{{Name: "cpu.percent", Value: 42, Timestamp: time.Now()}}, nil
}

type testPublisher struct {
	published chan []core.MetricSample
}

func (p *testPublisher) PublishMetrics(_ context.Context, samples []core.MetricSample) {
	p.published <- samples
}

func TestCollectAndPublishRunsImmediately(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	collector := &testCollector{}
	publisher := &testPublisher{published: make(chan []core.MetricSample, 2)}

	done := make(chan struct{})
	go func() {
		collectAndPublish(ctx, collector, publisher, zap.NewNop())
		close(done)
	}()

	select {
	case samples := <-publisher.published:
		if len(samples) != 1 || samples[0].Name != "cpu.percent" {
			t.Fatalf("published samples = %#v", samples)
		}
	case <-time.After(time.Second):
		t.Fatal("collector did not publish immediately")
	}

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("collector loop did not stop after cancellation")
	}
}
