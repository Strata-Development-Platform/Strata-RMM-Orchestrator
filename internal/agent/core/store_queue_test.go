package core

import (
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestStoreQueueCapacityIncludesMetricsAndEvents(t *testing.T) {
	store, err := NewStoreWithLimit(filepath.Join(t.TempDir(), "agent.db"), 2)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Unix(1700000000, 0)
	if err := store.QueueMetric(StoredMetric{Name: "cpu", Timestamp: now}); err != nil {
		t.Fatal(err)
	}
	if err := store.QueueMetric(StoredMetric{Name: "cpu", Timestamp: now}); err != nil {
		t.Fatal(err)
	}
	if err := store.QueueEvent(StoredEvent{Type: "audit", Timestamp: now}); !errors.Is(err, ErrQueueFull) {
		t.Fatalf("QueueEvent error = %v, want ErrQueueFull", err)
	}
	if size, err := store.QueueSize(); err != nil || size != 2 {
		t.Fatalf("QueueSize = %d, %v; want 2, nil", size, err)
	}
}

func TestQueuedMetricsRemainUntilAcknowledged(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "agent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	metric := StoredMetric{Name: "cpu.percent", Value: 42, Timestamp: time.Now().UTC()}
	if err := store.QueueMetric(metric); err != nil {
		t.Fatal(err)
	}
	first, err := store.PeekMetrics(10)
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.PeekMetrics(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 1 || len(second) != 1 || first[0].Key != second[0].Key {
		t.Fatalf("peek changed unacknowledged queue: first=%#v second=%#v", first, second)
	}
	if err := store.AckMetrics([]string{first[0].Key}); err != nil {
		t.Fatal(err)
	}
	afterAck, err := store.PeekMetrics(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(afterAck) != 0 {
		t.Fatalf("queue after acknowledgment = %#v", afterAck)
	}
}

func TestQueuedEventsRemainUntilAcknowledged(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "agent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	event := StoredEvent{Type: "service.changed", Message: "stopped", Timestamp: time.Now().UTC()}
	if err := store.QueueEvent(event); err != nil {
		t.Fatal(err)
	}
	queued, err := store.PeekEvents(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(queued) != 1 {
		t.Fatalf("queued events = %#v", queued)
	}
	if err := store.AckEvents([]string{queued[0].Key}); err != nil {
		t.Fatal(err)
	}
	afterAck, err := store.PeekEvents(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(afterAck) != 0 {
		t.Fatalf("events after acknowledgment = %#v", afterAck)
	}
}
