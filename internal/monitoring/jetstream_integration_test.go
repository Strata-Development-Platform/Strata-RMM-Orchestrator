//go:build integration

package monitoring

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
)

// TestJetStreamPersistentConsumerNoMessageLoss verifies that durable consumers
// retain unacknowledged messages across consumer restarts — the core requirement
// for Phase 1.1 (JetStream persistent consumers).
func TestJetStreamPersistentConsumerNoMessageLoss(t *testing.T) {
	url := os.Getenv("TEST_NATS_URL")
	if url == "" {
		t.Skip("TEST_NATS_URL is required for JetStream integration tests")
	}

	nc, err := nats.Connect(url, nats.Timeout(5*time.Second))
	require.NoError(t, err)
	t.Cleanup(nc.Close)

	js, err := nc.JetStream(nats.MaxWait(5 * time.Second))
	require.NoError(t, err)

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	stream := "PERSISTENT_TEST_" + suffix
	consumer := "PERSISTENT_CONSUMER_" + suffix
	subject := "tenant.test.agent." + suffix + ".metrics"

	// Create stream
	_, err = js.AddStream(&nats.StreamConfig{
		Name:      stream,
		Subjects:  []string{subject},
		Retention: nats.LimitsPolicy,
		Storage:   nats.MemoryStorage,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = js.DeleteStream(stream) })

	// Create durable consumer (simulates production consumer)
	_, err = js.AddConsumer(stream, &nats.ConsumerConfig{
		Durable:       consumer,
		AckPolicy:     nats.AckExplicitPolicy,
		AckWait:       30 * time.Second,
		MaxDeliver:    10,
		FilterSubject: subject,
	})
	require.NoError(t, err)

	// Publish messages
	numMessages := 10
	for i := 0; i < numMessages; i++ {
		payload := fmt.Sprintf(`{"tenant_id":"test-tenant-%d","agent_id":"agent-%d","metric":"cpu","value":%.2f}`, i, i, float64(i)*10.5)
		msg := nats.NewMsg(subject)
		msg.Data = []byte(payload)
		_, err = js.PublishMsg(msg)
		require.NoError(t, err)
	}

	// Create first consumer and acknowledge first 5 messages
	sub1, err := js.PullSubscribe(subject, consumer, nats.BindStream(stream))
	require.NoError(t, err)

	fetch1, err := sub1.Fetch(5, nats.MaxWait(5*time.Second))
	require.NoError(t, err)
	require.Len(t, fetch1, 5)

	for _, msg := range fetch1 {
		err = msg.Ack()
		require.NoError(t, err)
	}

	// Simulate consumer restart: close subscription and create new one
	sub1.Drain()

	// Wait for consumer to be considered "dead" (messages become available again)
	time.Sleep(2 * time.Second)

	// Create new consumer (simulates restart)
	nc2, err := nats.Connect(url, nats.Timeout(5*time.Second))
	require.NoError(t, err)
	t.Cleanup(nc2.Close)

	js2, err := nc2.JetStream(nats.MaxWait(5 * time.Second))
	require.NoError(t, err)

	sub2, err := js2.PullSubscribe(subject, consumer, nats.BindStream(stream))
	require.NoError(t, err)
	t.Cleanup(func() { _ = sub2.Drain() })

	// Fetch remaining messages (should include the 5 unacked from first consumer)
	fetch2, err := sub2.Fetch(numMessages, nats.MaxWait(10*time.Second))
	require.NoError(t, err)

	// Verify we received all 10 messages (5 acked + 5 unacked)
	ackedCount := 0
	unackedCount := 0
	for _, msg := range fetch2 {
		// Try to ack — if already acked, it will fail
		err = msg.AckSync()
		if err == nil {
			ackedCount++
		} else {
			unackedCount++
		}
	}

	// We should have received all 10 messages
	// 5 were acked by first consumer (AckSync will fail for these)
	// 5 were not acked (AckSync will succeed for these)
	require.Equal(t, numMessages, ackedCount+unackedCount, "should have received all %d messages", numMessages)

	// At least the 5 unacked messages should be delivered
	require.GreaterOrEqual(t, ackedCount, 5, "should have acked at least 5 messages (the unacked ones from first consumer)")
}
