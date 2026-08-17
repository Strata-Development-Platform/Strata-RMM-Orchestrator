package software

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"go.etcd.io/bbolt"
)

var softwareReceiptsBucket = []byte("software_command_receipts")

const (
	softwareStateReceived = "received"
	softwareStateRunning  = "running"
	softwareStateTerminal = "terminal"
)

var errSoftwareCommandConflict = errors.New("software command identity conflicts with existing receipt")

type softwareReceipt struct {
	Key         string         `json:"key"`
	Fingerprint string         `json:"fingerprint"`
	State       string         `json:"state"`
	Result      SoftwareResult `json:"result,omitempty"`
	HasResult   bool           `json:"has_result,omitempty"`
}

type softwareBeginDisposition int

const (
	softwareBeginExecute softwareBeginDisposition = iota
	softwareBeginInFlight
	softwareBeginReplay
)

type softwareReceiptLedger struct {
	db *bbolt.DB
}

func newSoftwareReceiptLedger(db *bbolt.DB) (*softwareReceiptLedger, error) {
	if db == nil {
		return nil, errors.New("software receipt database is required")
	}
	ledger := &softwareReceiptLedger{db: db}
	if err := db.Update(func(tx *bbolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(softwareReceiptsBucket)
		return err
	}); err != nil {
		return nil, fmt.Errorf("initialize software receipt ledger: %w", err)
	}
	return ledger, nil
}

// begin atomically claims one logical deployment/action for execution. A
// terminal receipt is replayed without re-executing, while a concurrent running
// receipt is left untouched so transport redelivery can retry later.
func (l *softwareReceiptLedger) begin(key, fingerprint string) (softwareBeginDisposition, SoftwareResult, error) {
	if l == nil || l.db == nil {
		return softwareBeginInFlight, SoftwareResult{}, errors.New("software receipt ledger is required")
	}
	if key == "" || fingerprint == "" {
		return softwareBeginInFlight, SoftwareResult{}, errors.New("software receipt identity is required")
	}

	disposition := softwareBeginExecute
	var replay SoftwareResult
	err := l.db.Update(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(softwareReceiptsBucket)
		data := bucket.Get([]byte(key))
		if data == nil {
			receipt := softwareReceipt{Key: key, Fingerprint: fingerprint, State: softwareStateRunning}
			encoded, err := json.Marshal(receipt)
			if err != nil {
				return err
			}
			return bucket.Put([]byte(key), encoded)
		}

		var receipt softwareReceipt
		if err := json.Unmarshal(append([]byte(nil), data...), &receipt); err != nil {
			return fmt.Errorf("decode software receipt: %w", err)
		}
		if receipt.Fingerprint != fingerprint {
			return errSoftwareCommandConflict
		}
		switch receipt.State {
		case softwareStateReceived:
			receipt.State = softwareStateRunning
			encoded, err := json.Marshal(receipt)
			if err != nil {
				return err
			}
			return bucket.Put([]byte(key), encoded)
		case softwareStateRunning:
			disposition = softwareBeginInFlight
			return nil
		case softwareStateTerminal:
			if !receipt.HasResult {
				return errors.New("terminal software receipt is missing result")
			}
			disposition = softwareBeginReplay
			replay = receipt.Result
			return nil
		default:
			return fmt.Errorf("invalid software receipt state %q", receipt.State)
		}
	})
	return disposition, replay, err
}

func (l *softwareReceiptLedger) complete(key, fingerprint string, result SoftwareResult) error {
	if l == nil || l.db == nil {
		return errors.New("software receipt ledger is required")
	}
	return l.db.Update(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(softwareReceiptsBucket)
		data := bucket.Get([]byte(key))
		if data == nil {
			return errors.New("software receipt not found")
		}
		var receipt softwareReceipt
		if err := json.Unmarshal(data, &receipt); err != nil {
			return err
		}
		if receipt.Fingerprint != fingerprint {
			return errSoftwareCommandConflict
		}
		if receipt.State == softwareStateTerminal {
			if receipt.HasResult && receipt.Result == result {
				return nil
			}
			return errors.New("software receipt already has a different terminal result")
		}
		if receipt.State != softwareStateRunning {
			return fmt.Errorf("software receipt is not running: %s", receipt.State)
		}
		receipt.State = softwareStateTerminal
		receipt.Result = result
		receipt.HasResult = true
		encoded, err := json.Marshal(receipt)
		if err != nil {
			return err
		}
		return bucket.Put([]byte(key), encoded)
	})
}

// release returns a claimed command to retryable state when execution could not
// be durably completed. It never changes terminal receipts.
func (l *softwareReceiptLedger) release(key, fingerprint string) error {
	return l.db.Update(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(softwareReceiptsBucket)
		data := bucket.Get([]byte(key))
		if data == nil {
			return nil
		}
		var receipt softwareReceipt
		if err := json.Unmarshal(data, &receipt); err != nil {
			return err
		}
		if receipt.Fingerprint != fingerprint {
			return errSoftwareCommandConflict
		}
		if receipt.State != softwareStateRunning {
			return nil
		}
		receipt.State = softwareStateReceived
		encoded, err := json.Marshal(receipt)
		if err != nil {
			return err
		}
		return bucket.Put([]byte(key), encoded)
	})
}

// resumeInterrupted fails closed for commands that were running when the
// process stopped. The endpoint side effect may already have completed before
// the terminal receipt was persisted, so automatic re-execution is unsafe.
// Convert the ambiguous receipt into a durable terminal failure that can be
// replayed to the control plane for operator reconciliation.
func (l *softwareReceiptLedger) resumeInterrupted() error {
	if l == nil || l.db == nil {
		return errors.New("software receipt ledger is required")
	}
	return l.db.Update(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(softwareReceiptsBucket)
		return bucket.ForEach(func(key, value []byte) error {
			var receipt softwareReceipt
			if err := json.Unmarshal(value, &receipt); err != nil {
				return fmt.Errorf("decode software receipt %q: %w", key, err)
			}
			if receipt.State != softwareStateRunning {
				return nil
			}
			parts := strings.SplitN(receipt.Key, "\x00", 2)
			if len(parts) != 2 || parts[0] == "" || (parts[1] != "install" && parts[1] != "uninstall") {
				return fmt.Errorf("invalid interrupted software receipt key %q", receipt.Key)
			}
			receipt.State = softwareStateTerminal
			receipt.Result = SoftwareResult{
				Type:         "software_result",
				DeploymentID: parts[0],
				Action:       parts[1],
				Status:       "failed",
				ErrorMessage: "execution outcome unknown after agent restart; manual reconciliation required",
			}
			receipt.HasResult = true
			encoded, err := json.Marshal(receipt)
			if err != nil {
				return err
			}
			return bucket.Put(key, encoded)
		})
	})
}
