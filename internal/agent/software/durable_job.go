package software

import (
	"context"
	"encoding/json"
	"fmt"
)

// RunDurableJob adapts the software installer to the generic durable job
// protocol. The generic job receipt ledger owns command deduplication and result
// replay; this method only validates and executes the software payload.
func (inst *Installer) RunDurableJob(ctx context.Context, payload json.RawMessage, expectedAction string) (string, int, []byte, error) {
	cmd, err := decodeSoftwareCommand(payload)
	if err != nil {
		return "failed", 1, nil, fmt.Errorf("decode software command: %w", err)
	}
	if cmd.Action != expectedAction || cmd.Type != "software_"+expectedAction {
		return "failed", 1, nil, fmt.Errorf("software command does not match durable handler action %q", expectedAction)
	}
	if err := validateSoftwareCommand(cmd); err != nil {
		return "failed", 1, nil, err
	}

	result := inst.executeWithContext(ctx, cmd)
	encoded, err := json.Marshal(result)
	if err != nil {
		return "failed", 1, nil, fmt.Errorf("encode software result: %w", err)
	}
	if result.Status == "success" {
		return "succeeded", 0, encoded, nil
	}
	return "failed", 1, encoded, nil
}
