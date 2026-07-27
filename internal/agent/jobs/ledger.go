package jobs

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"go.etcd.io/bbolt"
	"go.uber.org/zap"
)

type CommandReceipt struct {
	EventID      string `json:"event_id"`
	JobID        string `json:"job_id"`
	TargetID     string `json:"target_id"`
	CorrelationID string `json:"correlation_id"`
	Attempt      int    `json:"attempt"`
	CommandType  string `json:"command_type"`
	ReceivedAt   string `json:"received_at"`
	State        string `json:"state"`
	ResultMsgID  string `json:"result_msg_id,omitempty"`
}

const (
	StateReceived    = "received"
	StateRunning     = "running"
	StateSucceeded   = "succeeded"
	StateFailed      = "failed"
	StateCancelled   = "cancelled"
	StateExpired     = "expired"
)

type ReceiptLedger struct {
	db     *bbolt.DB
	logger *zap.Logger
	mu     sync.RWMutex
}

func NewReceiptLedger(db *bbolt.DB, logger *zap.Logger) *ReceiptLedger {
	l := &ReceiptLedger{db: db, logger: logger}
	l.init()
	return l
}

func (l *ReceiptLedger) init() {
	l.db.Update(func(tx *bbolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte("command_receipts"))
		return err
	})
}

func receiptKey(eventID string) string { return eventID }

func (l *ReceiptLedger) RecordReceipt(receipt *CommandReceipt) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte("command_receipts"))
		data, err := json.Marshal(receipt)
		if err != nil {
			return err
		}
		return b.Put([]byte(receiptKey(receipt.EventID)), data)
	})
}

func (l *ReceiptLedger) GetReceipt(eventID string) (*CommandReceipt, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	var receipt CommandReceipt
	err := l.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte("command_receipts"))
		data := b.Get([]byte(receiptKey(eventID)))
		if data == nil {
			return fmt.Errorf("receipt not found: %s", eventID)
		}
		return json.Unmarshal(data, &receipt)
	})
	if err != nil {
		return nil, err
	}
	return &receipt, nil
}

func (l *ReceiptLedger) IsDuplicate(eventID string) bool {
	_, err := l.GetReceipt(eventID)
	return err == nil
}

func (l *ReceiptLedger) MarkRunning(eventID string) error {
	return l.updateState(eventID, StateRunning)
}

func (l *ReceiptLedger) MarkComplete(eventID, status, resultMsgID string) error {
	return l.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte("command_receipts"))
		data := b.Get([]byte(receiptKey(eventID)))
		if data == nil {
			return nil
		}
		var receipt CommandReceipt
		json.Unmarshal(data, &receipt)
		receipt.State = status
		if resultMsgID != "" {
			receipt.ResultMsgID = resultMsgID
		}
		updated, _ := json.Marshal(receipt)
		return b.Put([]byte(receiptKey(eventID)), updated)
	})
}

func (l *ReceiptLedger) updateState(eventID, state string) error {
	return l.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte("command_receipts"))
		data := b.Get([]byte(receiptKey(eventID)))
		if data == nil {
			return nil
		}
		var receipt CommandReceipt
		json.Unmarshal(data, &receipt)
		receipt.State = state
		updated, _ := json.Marshal(receipt)
		return b.Put([]byte(receiptKey(eventID)), updated)
	})
}

func (l *ReceiptLedger) GetUnacknowledgedResults() []*CommandReceipt {
	var results []*CommandReceipt
	l.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte("command_receipts"))
		c := b.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			var receipt CommandReceipt
			if err := json.Unmarshal(v, &receipt); err != nil {
				continue
			}
			if (receipt.State == StateSucceeded || receipt.State == StateFailed) && receipt.ResultMsgID != "" {
				results = append(results, &receipt)
			}
		}
		return nil
	})
	return results
}

func (l *ReceiptLedger) Cleanup(before time.Time) {
	l.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte("command_receipts"))
		c := b.Cursor()
		toDelete := [][]byte{}
		for k, v := c.First(); k != nil; k, v = c.Next() {
			var receipt CommandReceipt
			if err := json.Unmarshal(v, &receipt); err != nil {
				continue
			}
			receivedAt, err := time.Parse(time.RFC3339, receipt.ReceivedAt)
			if err == nil && receivedAt.Before(before) {
				toDelete = append(toDelete, k)
			}
		}
		for _, k := range toDelete {
			b.Delete(k)
		}
		return nil
	})
}

func PublishAcknowledgement(nc *nats.Conn, subject string, eventID, jobID, targetID, mspID, deviceID, agentID, correlationID string, attempt int, status string) error {
	ack := map[string]interface{}{
		"schema_version": 1,
		"message_id":     fmt.Sprintf("ack-%s", eventID),
		"event_id":       eventID,
		"job_id":         jobID,
		"target_id":      targetID,
		"msp_id":         mspID,
		"device_id":      deviceID,
		"agent_id":       agentID,
		"correlation_id": correlationID,
		"attempt":        attempt,
		"status":         status,
		"timestamp":      time.Now().UTC().Format(time.RFC3339),
	}
	data, _ := json.Marshal(ack)
	return nc.Publish(subject, data)
}

func PublishResult(nc *nats.Conn, subject, msgID, eventID, jobID, targetID, mspID, deviceID, agentID, correlationID string, attempt int, status string, exitCode int, resultJSON []byte, errorStr string, startedAt, completedAt time.Time) (string, error) {
	if msgID == "" {
		msgID = fmt.Sprintf("res-%s-%d", eventID, attempt)
	}
	durationMs := completedAt.Sub(startedAt).Milliseconds()
	res := map[string]interface{}{
		"schema_version": 1,
		"message_id":     msgID,
		"event_id":       eventID,
		"job_id":         jobID,
		"target_id":      targetID,
		"msp_id":         mspID,
		"device_id":      deviceID,
		"agent_id":       agentID,
		"correlation_id": correlationID,
		"attempt":        attempt,
		"status":         status,
		"exit_code":      exitCode,
		"result":         resultJSON,
		"error":          errorStr,
		"started_at":     startedAt.UTC().Format(time.RFC3339),
		"completed_at":   completedAt.UTC().Format(time.RFC3339),
		"duration_ms":    durationMs,
	}
	data, _ := json.Marshal(res)
	if err := nc.Publish(subject, data); err != nil {
		return msgID, err
	}
	return msgID, nil
}
