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

// TestJetStreamPersistentConsumerNoMessageLoss proves the delivery contract used
// by telemetry ingestion: acknowledged messages are not redelivered, while
// unacknowledged messages remain available to the durable consumer after a
// subscriber restart.
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

	_, err = js.AddStream(&nats.StreamConfig{
		Name:      stream,
		Subjects:  []string{subject},
		Retention: nats.LimitsPolicy,
		Storage:   nats.MemoryStorage,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = js.DeleteStream(stream) })

	_, err = js.AddConsumer(stream, &nats.ConsumerConfig{
		Durable:       consumer,
		AckPolicy:     nats.AckExplicitPolicy,
		AckWait:       2 * time.Second,
		MaxDeliver:    10,
		FilterSubject: subject,
	})
	require.NoError(t, err)

	const numMessages = 10
	for i := 0; i < numMessages; i++ {
		payload := fmt.Sprintf(`{"seq":%d}`, i)
		_, err = js.Publish(subject, []byte(payload))
		require.NoError(t, err)
	}

	sub1, err := js.PullSubscribe(subject, consumer, nats.BindStream(stream))
	require.NoError(t, err)

	firstBatch, err := sub1.Fetch(10, nats.MaxWait(5*time.Second))
	require.NoError(t, err)
	require.Len(t, firstBatch, numMessages)

	for i := 0; i < 5; i++ {
		require.NoError(t, firstBatch[i].AckSync())
	}
	// Intentionally leave messages 5..9 unacknowledged to model a process
	// failure before durable persistence/ACK.
	require.NoError(t, sub1.Drain())

	// Wait beyond AckWait so the durable consumer makes unACKed messages
	// eligible for redelivery.
	time.Sleep(2500 * time.Millisecond)

	nc2, err := nats.Connect(url, nats.Timeout(5*time.Second))
	require.NoError(t, err)
	t.Cleanup(nc2.Close)
	js2, err := nc2.JetStream(nats.MaxWait(5 * time.Second))
	require.NoError(t, err)
	sub2, err := js2.PullSubscribe(subject, consumer, nats.BindStream(stream))
	require.NoError(t, err)
	t.Cleanup(func() { _ = sub2.Drain() })

	redelivered, err := sub2.Fetch(5, nats.MaxWait(5*time.Second))
	require.NoError(t, err)
	require.Len(t, redelivered, 5, "only the five unacknowledged messages should be redelivered")

	seen := make(map[string]struct{}, 5)
	for _, msg := range redelivered {
		seen[string(msg.Data)] = struct{}{}
		require.NoError(t, msg.AckSync())
	}
	for i := 5; i < 10; i++ {
		_, ok := seen[fmt.Sprintf(`{"seq":%d}`, i)]
		require.True(t, ok, "unacknowledged message %d was not redelivered", i)
	}

	info, err := js2.ConsumerInfo(stream, consumer)
	require.NoError(t, err)
	require.Equal(t, uint64(0), info.NumAckPending, "all redelivered messages should be acknowledged")
	require.Equal(t, uint64(0), info.NumPending, "consumer should have no pending messages")
}
