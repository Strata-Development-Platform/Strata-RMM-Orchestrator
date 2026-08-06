package nats

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
)

// QueueKey is the NATS subject used for agent session replay.
const AgentSessionSubject = "strata.agent.session"

// SessionMessage represents a queued telemetry message for replay.
type SessionMessage struct {
	Type      string    `json:"type"` // "metric" or "event"
	AgentID   string    `json:"agent_id"`
	TenantID  string    `json:"tenant_id"`
	Payload   []byte    `json:"payload"`
	CreatedAt time.Time `json:"created_at"`
}

// SessionStore manages agent session replay messages in NATS JetStream.
type SessionStore struct {
	js nats.JetStreamContext
}

// NewSessionStore creates a new session store.
func NewSessionStore(js nats.JetStreamContext) *SessionStore {
	return &SessionStore{js: js}
}

// QueueMessage publishes a message to the agent session stream.
func (s *SessionStore) QueueMessage(ctx context.Context, msg SessionMessage) error {
	msg.CreatedAt = time.Now().UTC()
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal session message: %w", err)
	}

	subject := fmt.Sprintf("%s.%s", AgentSessionSubject, msg.AgentID)
	_, err = s.js.Publish(subject, data)
	return err
}

// QueueMessages publishes multiple messages to the agent session stream.
func (s *SessionStore) QueueMessages(ctx context.Context, msgs []SessionMessage) error {
	for _, msg := range msgs {
		if err := s.QueueMessage(ctx, msg); err != nil {
			return err
		}
	}
	return nil
}

// ReplayMessages replays messages from the agent session stream.
func (s *SessionStore) ReplayMessages(ctx context.Context, agentID string, maxMessages int) ([]SessionMessage, error) {
	subject := fmt.Sprintf("%s.%s", AgentSessionSubject, agentID)

	msgs, err := s.js.PullSubscribe(subject, "replay")
	if err != nil {
		return nil, fmt.Errorf("subscribe to session stream: %w", err)
	}
	defer func() { _ = msgs.Unsubscribe() }()

	var results []SessionMessage
	for i := 0; i < maxMessages; i++ {
		msgs, err := msgs.Fetch(1, nats.MaxWait(2*time.Second))
		if err != nil {
			break
		}

		for _, msg := range msgs {
			var sessionMsg SessionMessage
			if err := json.Unmarshal(msg.Data, &sessionMsg); err != nil {
				continue
			}
			results = append(results, sessionMsg)
			_ = msg.Ack()
		}

		if len(msgs) == 0 {
			break
		}
	}

	return results, nil
}
