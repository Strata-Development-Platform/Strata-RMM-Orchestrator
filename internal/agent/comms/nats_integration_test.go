package comms

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/strata-rmm/strata-rmm-orchestrator/internal/agent/core"
)

type integrationLogger struct{}

func (integrationLogger) Info(string, ...interface{})  {}
func (integrationLogger) Error(string, ...interface{}) {}
func (integrationLogger) Warn(string, ...interface{})  {}
func (integrationLogger) Debug(string, ...interface{}) {}

func TestReplayQueuedTelemetryAgainstNATS(t *testing.T) {
	natsURL := os.Getenv("NATS_INTEGRATION_URL")
	if natsURL == "" {
		t.Skip("NATS_INTEGRATION_URL is not set")
	}

	store, err := core.NewStore(filepath.Join(t.TempDir(), "agent.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("closing store: %v", err)
		}
	})

	identity := &core.Identity{TenantID: "tenant-integration", AgentID: "agent-integration"}
	cfg := &core.NATSConfig{
		URLs:          []string{natsURL},
		ReconnectWait: 100 * time.Millisecond,
		MaxReconnects: 20,
	}
	logger := integrationLogger{}

	offlineClient := NewClient(cfg, identity, logger)
	offlineHandler := NewCommsHandler(offlineClient, store, logger)
	sampleTime := time.Unix(1_700_000_000, 0).UTC()
	offlineHandler.PublishMetrics(context.Background(), []core.MetricSample{{
		Name:      "cpu.percent",
		Value:     42.5,
		Timestamp: sampleTime,
	}})

	if size, err := store.QueueSize(); err != nil || size != 1 {
		t.Fatalf("offline queue size = %d, %v; want 1, nil", size, err)
	}

	observer, err := nats.Connect(natsURL, nats.Timeout(5*time.Second))
	if err != nil {
		t.Fatalf("connecting observer: %v", err)
	}
	t.Cleanup(observer.Close)

	messages := make(chan *nats.Msg, 1)
	subject := "tenant.tenant-integration.agent.agent-integration.metrics"
	subscription, err := observer.ChanSubscribe(subject, messages)
	if err != nil {
		t.Fatalf("subscribing observer: %v", err)
	}
	t.Cleanup(func() {
		if err := subscription.Unsubscribe(); err != nil {
			t.Errorf("unsubscribing observer: %v", err)
		}
	})
	if err := observer.Flush(); err != nil {
		t.Fatalf("flushing observer subscription: %v", err)
	}

	client := NewClient(cfg, identity, logger)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	if err := client.Connect(ctx); err != nil {
		t.Fatalf("connecting agent client: %v", err)
	}
	t.Cleanup(client.Close)

	handler := NewCommsHandler(client, store, logger)
	if err := handler.Start(ctx); err != nil {
		t.Fatalf("starting communications handler: %v", err)
	}
	t.Cleanup(handler.Stop)

	select {
	case message := <-messages:
		var payload struct {
			Samples []struct {
				Name      string  `json:"name"`
				Value     float64 `json:"value"`
				Timestamp int64   `json:"timestamp"`
			} `json:"samples"`
		}
		if err := json.Unmarshal(message.Data, &payload); err != nil {
			t.Fatalf("decoding replayed metric: %v", err)
		}
		if len(payload.Samples) != 1 ||
			payload.Samples[0].Name != "cpu.percent" ||
			payload.Samples[0].Value != 42.5 ||
			payload.Samples[0].Timestamp != sampleTime.Unix() {
			t.Fatalf("replayed payload = %s", message.Data)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for replayed metric")
	}

	if size, err := store.QueueSize(); err != nil || size != 0 {
		t.Fatalf("queue size after replay = %d, %v; want 0, nil", size, err)
	}
}
