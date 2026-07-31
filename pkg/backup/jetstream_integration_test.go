//go:build integration

package backup

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
)

// TestJetStreamRecovery_RoundTrip exercises the production recovery component
// against an actual JetStream server. It deliberately deletes the source stream
// before restore so a serializer-only implementation cannot pass.
func TestJetStreamRecovery_RoundTrip(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	url := firstNonEmpty(os.Getenv("TEST_NATS_URL"), os.Getenv("NATS_URL"))
	if url == "" {
		t.Skip("TEST_NATS_URL is required for JetStream integration tests")
	}

	nc, err := nats.Connect(url, nats.Timeout(5*time.Second))
	require.NoError(t, err)
	t.Cleanup(nc.Close)

	js, err := nc.JetStream(nats.MaxWait(5 * time.Second))
	require.NoError(t, err)

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	stream := "RECOVERY_" + suffix
	durable := "RECOVERY_CONSUMER_" + suffix
	subject := "recovery." + suffix + ".>"

	_, err = js.AddStream(&nats.StreamConfig{
		Name:      stream,
		Subjects:  []string{subject},
		Retention: nats.LimitsPolicy,
		Storage:   nats.MemoryStorage,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = js.DeleteStream(stream) })

	_, err = js.AddConsumer(stream, &nats.ConsumerConfig{
		Durable:       durable,
		AckPolicy:     nats.AckExplicitPolicy,
		FilterSubject: subject,
	})
	require.NoError(t, err)

	want := []struct {
		subject string
		data    []byte
		header  string
	}{
		{"recovery." + suffix + ".one", []byte("first"), "tenant-a"},
		{"recovery." + suffix + ".two", []byte{0, 1, 2, 255}, "tenant-b"},
		{"recovery." + suffix + ".three", []byte("third"), "tenant-a"},
	}
	for _, item := range want {
		msg := nats.NewMsg(item.subject)
		msg.Data = item.data
		msg.Header.Set("X-Tenant-ID", item.header)
		_, err = js.PublishMsg(msg)
		require.NoError(t, err)
	}

	var artifact bytes.Buffer
	recovery, err := NewJetStreamRecovery(nc)
	require.NoError(t, err)
	manifest, err := recovery.Backup(ctx, &artifact)
	require.NoError(t, err)
	require.NotEmpty(t, artifact.Bytes(), "backup must contain the stream data")

	require.NoError(t, js.DeleteStream(stream))
	_, err = js.StreamInfo(stream)
	require.Error(t, err, "source stream must be absent before restore")

	require.NoError(t, recovery.Restore(ctx, bytes.NewReader(artifact.Bytes())))
	require.NoError(t, recovery.Verify(ctx, manifest))

	info, err := js.StreamInfo(stream)
	require.NoError(t, err)
	require.Equal(t, uint64(len(want)), info.State.Msgs)

	consumer, err := js.ConsumerInfo(stream, durable)
	require.NoError(t, err)
	require.Equal(t, durable, consumer.Config.Durable)
	require.Equal(t, nats.AckExplicitPolicy, consumer.Config.AckPolicy)

	sub, err := js.PullSubscribe(subject, durable, nats.Bind(stream, durable))
	require.NoError(t, err)
	messages, err := sub.Fetch(len(want), nats.MaxWait(5*time.Second))
	require.NoError(t, err)
	require.Len(t, messages, len(want))
	for i, msg := range messages {
		require.Equal(t, want[i].subject, msg.Subject)
		require.Equal(t, want[i].data, msg.Data)
		require.Equal(t, want[i].header, msg.Header.Get("X-Tenant-ID"))
		require.NoError(t, msg.Ack())
	}

	// Restoration must be idempotent: replaying the same artifact cannot append
	// a second copy of each message.
	require.NoError(t, recovery.Restore(ctx, bytes.NewReader(artifact.Bytes())))
	info, err = js.StreamInfo(stream)
	require.NoError(t, err)
	require.Equal(t, uint64(len(want)), info.State.Msgs)
	require.NoError(t, recovery.Verify(ctx, manifest))
}

func TestJetStreamRecovery_RequiresJetStream(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	url := os.Getenv("TEST_NATS_NO_JS_URL")
	if url == "" {
		t.Skip("TEST_NATS_NO_JS_URL is required for the JetStream-disabled negative test")
	}

	nc, err := nats.Connect(url, nats.Timeout(5*time.Second))
	require.NoError(t, err)
	t.Cleanup(nc.Close)

	var artifact bytes.Buffer
	recovery, err := NewJetStreamRecovery(nc)
	require.Error(t, err)
	require.Nil(t, recovery)
	if recovery != nil {
		_, err = recovery.Backup(ctx, &artifact)
	}
	require.Error(t, err)
	require.Empty(t, artifact.Bytes())
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
