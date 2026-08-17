package jobs

import "fmt"

// ResumeInterruptedExecution atomically changes a receipt left in running state
// back to received so a redelivery after process restart can execute it again.
// It never changes terminal states or a normally received command.
func (l *ReceiptLedger) ResumeInterruptedExecution(eventID string) (bool, error) {
	if l == nil || l.db == nil {
		return false, fmt.Errorf("receipt ledger database is required")
	}
	resumed := false
	err := l.update(eventID, func(receipt *CommandReceipt) {
		if receipt.State != StateRunning {
			return
		}
		receipt.State = StateReceived
		resumed = true
	})
	return resumed, err
}
