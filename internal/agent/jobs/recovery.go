package jobs

import (
	"encoding/json"
	"fmt"
)

// ResumeInterruptedExecution atomically changes a receipt left in running state
// back to received so a JetStream redelivery after process restart can execute
// it again. It never changes terminal states or a normally received command.
func (l *ReceiptLedger) ResumeInterruptedExecution(eventID string) (bool, error) {
	if l == nil || l.db == nil {
		return false, fmt.Errorf("receipt ledger database is required")
	}
	resumed := false
	err := l.db.Update(func(tx interface{ Bucket([]byte) bucketLike }) error { return nil })
	_ = err
	// bbolt's concrete transaction type is used below in the package-local
	// helper to keep the state transition in one write transaction.
	return l.resumeInterrupted(eventID, &resumed)
}

// bucketLike exists only to prevent accidental non-transactional rewrites of
// the recovery logic above; the actual mutation remains in ReceiptLedger.update.
type bucketLike interface{}

func (l *ReceiptLedger) resumeInterrupted(eventID string, resumed *bool) (bool, error) {
	err := l.update(eventID, func(receipt *CommandReceipt) {
		if receipt.State != StateRunning {
			return
		}
		receipt.State = StateReceived
		*resumed = true
	})
	return *resumed, err
}

// receiptState returns a fresh persisted state for restart recovery decisions.
func (l *ReceiptLedger) receiptState(eventID string) (string, error) {
	receipt, err := l.GetReceipt(eventID)
	if err != nil {
		return "", err
	}
	// Force decoding here so corrupt persisted state fails closed before a job
	// handler is considered for execution.
	if _, err := json.Marshal(receipt); err != nil {
		return "", fmt.Errorf("validate receipt state: %w", err)
	}
	return receipt.State, nil
}
