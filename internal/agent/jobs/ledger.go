package jobs

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
	"go.etcd.io/bbolt"
	"go.uber.org/zap"
)

var receiptsBucket = []byte("command_receipts")

const (
	StateReceived  = "received"
	StateRunning   = "running"
	StateSucceeded = "succeeded"
	StateFailed    = "failed"
	StateCancelled = "cancelled"
	StateExpired   = "expired"
)

// CommandReceipt is the agent's durable source of truth. ResultEnvelope contains
// the exact bytes published to NATS so a restart never has to reconstruct data.
type CommandReceipt struct {
	EventID        string          `json:"event_id"`
	JobID          string          `json:"job_id"`
	TargetID       string          `json:"target_id"`
	MSPID          string          `json:"msp_id"`
	ClientID       string          `json:"client_id,omitempty"`
	SiteID         string          `json:"site_id,omitempty"`
	DeviceID       string          `json:"device_id"`
	AgentID        string          `json:"agent_id"`
	CorrelationID  string          `json:"correlation_id"`
	Attempt        int             `json:"attempt"`
	CommandType    string          `json:"command_type"`
	ReceivedAt     string          `json:"received_at"`
	State          string          `json:"state"`
	ResultMsgID    string          `json:"result_msg_id,omitempty"`
	ResultEnvelope json.RawMessage `json:"result_envelope,omitempty"`
	ResultAcked    bool            `json:"result_acked"`
}

type ReceiptLedger struct {
	db     *bbolt.DB
	logger *zap.Logger
}

func NewReceiptLedger(db *bbolt.DB, logger *zap.Logger) (*ReceiptLedger, error) {
	if db == nil {
		return nil, errors.New("receipt ledger database is nil")
	}
	l := &ReceiptLedger{db: db, logger: logger}
	if err := l.db.Update(func(tx *bbolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(receiptsBucket)
		return err
	}); err != nil {
		return nil, fmt.Errorf("initialize receipt ledger: %w", err)
	}
	return l, nil
}

func (l *ReceiptLedger) RecordReceipt(receipt *CommandReceipt) error {
	if receipt == nil || receipt.EventID == "" {
		return errors.New("receipt event_id is required")
	}
	return l.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(receiptsBucket)
		if b.Get([]byte(receipt.EventID)) != nil {
			return bbolt.ErrBucketExists // caller treats this as a duplicate
		}
		data, err := json.Marshal(receipt)
		if err != nil {
			return fmt.Errorf("marshal receipt: %w", err)
		}
		return b.Put([]byte(receipt.EventID), data)
	})
}

func (l *ReceiptLedger) GetReceipt(eventID string) (*CommandReceipt, error) {
	var receipt CommandReceipt
	err := l.db.View(func(tx *bbolt.Tx) error {
		data := tx.Bucket(receiptsBucket).Get([]byte(eventID))
		if data == nil {
			return fmt.Errorf("receipt not found: %s", eventID)
		}
		return json.Unmarshal(append([]byte(nil), data...), &receipt)
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
	return l.update(eventID, func(receipt *CommandReceipt) {
		receipt.State = StateRunning
	})
}

func (l *ReceiptLedger) MarkComplete(eventID, status, resultMsgID string, envelope []byte) error {
	return l.update(eventID, func(receipt *CommandReceipt) {
		receipt.State = status
		receipt.ResultMsgID = resultMsgID
		receipt.ResultEnvelope = append(receipt.ResultEnvelope[:0], envelope...)
		receipt.ResultAcked = false
	})
}

func (l *ReceiptLedger) MarkResultAcknowledged(messageID string) error {
	if messageID == "" {
		return errors.New("result message_id is required")
	}
	found := false
	err := l.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(receiptsBucket)
		return b.ForEach(func(k, v []byte) error {
			var receipt CommandReceipt
			if err := json.Unmarshal(v, &receipt); err != nil {
				return fmt.Errorf("decode receipt %q: %w", k, err)
			}
			if receipt.ResultMsgID != messageID {
				return nil
			}
			receipt.ResultAcked = true
			data, err := json.Marshal(&receipt)
			if err != nil {
				return err
			}
			found = true
			return b.Put(k, data)
		})
	})
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("result receipt not found: %s", messageID)
	}
	return nil
}

func (l *ReceiptLedger) update(eventID string, mutate func(*CommandReceipt)) error {
	return l.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(receiptsBucket)
		data := b.Get([]byte(eventID))
		if data == nil {
			return fmt.Errorf("receipt not found: %s", eventID)
		}
		var receipt CommandReceipt
		if err := json.Unmarshal(data, &receipt); err != nil {
			return err
		}
		mutate(&receipt)
		updated, err := json.Marshal(&receipt)
		if err != nil {
			return err
		}
		return b.Put([]byte(eventID), updated)
	})
}

func (l *ReceiptLedger) GetUnacknowledgedResults() ([]*CommandReceipt, error) {
	results := make([]*CommandReceipt, 0)
	err := l.db.View(func(tx *bbolt.Tx) error {
		return tx.Bucket(receiptsBucket).ForEach(func(_, value []byte) error {
			var receipt CommandReceipt
			if err := json.Unmarshal(value, &receipt); err != nil {
				return err
			}
			if !receipt.ResultAcked && len(receipt.ResultEnvelope) > 0 {
				copy := receipt
				copy.ResultEnvelope = append(json.RawMessage(nil), receipt.ResultEnvelope...)
				results = append(results, &copy)
			}
			return nil
		})
	})
	return results, err
}

func (l *ReceiptLedger) Cleanup(before time.Time) error {
	return l.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(receiptsBucket)
		var keys [][]byte
		if err := b.ForEach(func(k, v []byte) error {
			var receipt CommandReceipt
			if err := json.Unmarshal(v, &receipt); err != nil {
				return err
			}
			receivedAt, err := time.Parse(time.RFC3339, receipt.ReceivedAt)
			if err != nil {
				return err
			}
			if receipt.ResultAcked && receivedAt.Before(before) {
				keys = append(keys, append([]byte(nil), k...))
			}
			return nil
		}); err != nil {
			return err
		}
		for _, key := range keys {
			if err := b.Delete(key); err != nil {
				return err
			}
		}
		return nil
	})
}

func PublishAcknowledgement(nc *nats.Conn, subject string, eventID, jobID, targetID, mspID, deviceID, agentID, correlationID string, attempt int, status string) error {
	ack := map[string]interface{}{
		"schema_version": 1, "message_id": fmt.Sprintf("ack-%s-%d-%s", eventID, attempt, status),
		"event_id": eventID, "job_id": jobID, "target_id": targetID, "msp_id": mspID,
		"device_id": deviceID, "agent_id": agentID, "correlation_id": correlationID,
		"attempt": attempt, "status": status, "timestamp": time.Now().UTC().Format(time.RFC3339),
	}
	data, err := json.Marshal(ack)
	if err != nil {
		return err
	}
	return nc.Publish(subject, data)
}

func MarshalResult(msgID, eventID, jobID, targetID, mspID, clientID, siteID, deviceID, agentID, correlationID string, attempt int, status string, exitCode int, resultJSON []byte, errorStr string, startedAt, completedAt time.Time) (string, []byte, error) {
	if msgID == "" {
		msgID = fmt.Sprintf("res-%s-%d", eventID, attempt)
	}
	res := map[string]interface{}{
		"schema_version": 1, "message_id": msgID, "event_id": eventID, "job_id": jobID,
		"target_id": targetID, "msp_id": mspID, "client_id": clientID, "site_id": siteID,
		"device_id": deviceID, "agent_id": agentID, "correlation_id": correlationID,
		"attempt": attempt, "status": status, "exit_code": exitCode,
		"result": json.RawMessage(resultJSON), "error": errorStr,
		"started_at": startedAt.UTC().Format(time.RFC3339),
		"completed_at": completedAt.UTC().Format(time.RFC3339),
		"duration_ms": completedAt.Sub(startedAt).Milliseconds(),
	}
	data, err := json.Marshal(res)
	return msgID, data, err
}

func PublishResult(nc *nats.Conn, subject, msgID, eventID, jobID, targetID, mspID, deviceID, agentID, correlationID string, attempt int, status string, exitCode int, resultJSON []byte, errorStr string, startedAt, completedAt time.Time) (string, error) {
	msgID, data, err := MarshalResult(msgID, eventID, jobID, targetID, mspID, "", "", deviceID, agentID, correlationID, attempt, status, exitCode, resultJSON, errorStr, startedAt, completedAt)
	if err != nil {
		return msgID, err
	}
	return msgID, nc.Publish(subject, data)
}
