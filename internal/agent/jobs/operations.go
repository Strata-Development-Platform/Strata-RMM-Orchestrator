package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"runtime"
	"time"
)

type DeviceOperation struct {
	Action    string `json:"action"`
	Reason    string `json:"reason,omitempty"`
	Param     string `json:"param,omitempty"`
	Service   string `json:"service,omitempty"`
	ProcessID int    `json:"process_id,omitempty"`
}

type OperationResult struct {
	Action    string `json:"action"`
	Succeeded bool   `json:"succeeded"`
	Message   string `json:"message,omitempty"`
	Output    string `json:"output,omitempty"`
}

func RegisterDeviceOperations(registry *HandlerRegistry) {
	registry.Register("device.refresh", handleRefreshInventory)
	registry.Register("device.reboot", handleReboot)
	registry.Register("device.shutdown", handleShutdown)
	registry.Register("device.service_start", handleServiceStart)
	registry.Register("device.service_stop", handleServiceStop)
	registry.Register("device.service_restart", handleServiceRestart)
	registry.Register("device.process_kill", handleProcessKill)
	registry.Register("bulk.refresh", handleRefreshInventory)
	registry.Register("bulk.reboot", handleReboot)
	registry.Register("bulk.shutdown", handleShutdown)
}

func parseOperation(payload json.RawMessage) (*DeviceOperation, error) {
	var op DeviceOperation
	if err := json.Unmarshal(payload, &op); err != nil {
		return nil, err
	}
	return &op, nil
}

func handleRefreshInventory(ctx context.Context, cmd *CommandEnvelope) (string, int, []byte, error) {
	_, err := parseOperation(cmd.Payload)
	if err != nil {
		return "failed", 1, nil, fmt.Errorf("invalid payload: %w", err)
	}
	res := OperationResult{Action: "refresh", Succeeded: true, Message: "inventory refresh triggered"}
	data, _ := json.Marshal(res)
	return "succeeded", 0, data, nil
}

func handleReboot(ctx context.Context, cmd *CommandEnvelope) (string, int, []byte, error) {
	op, err := parseOperation(cmd.Payload)
	if err != nil {
		return "failed", 1, nil, fmt.Errorf("invalid payload: %w", err)
	}
	if runtime.GOOS == "linux" || runtime.GOOS == "darwin" {
		out, err := exec.CommandContext(ctx, "shutdown", "-r", "+1", fmt.Sprintf("reboot requested: %s", op.Reason)).CombinedOutput()
		if err != nil {
			res := OperationResult{Action: "reboot", Succeeded: false, Message: err.Error(), Output: string(out)}
			data, _ := json.Marshal(res)
			return "failed", 1, data, nil
		}
		res := OperationResult{Action: "reboot", Succeeded: true, Message: "reboot scheduled in 1 minute"}
		data, _ := json.Marshal(res)
		return "succeeded", 0, data, nil
	}
	res := OperationResult{Action: "reboot", Succeeded: false, Message: "unsupported on " + runtime.GOOS}
	data, _ := json.Marshal(res)
	return "failed", 1, data, nil
}

func handleShutdown(ctx context.Context, cmd *CommandEnvelope) (string, int, []byte, error) {
	op, err := parseOperation(cmd.Payload)
	if err != nil {
		return "failed", 1, nil, fmt.Errorf("invalid payload: %w", err)
	}
	if runtime.GOOS == "linux" || runtime.GOOS == "darwin" {
		out, err := exec.CommandContext(ctx, "shutdown", "-h", "+1", fmt.Sprintf("shutdown requested: %s", op.Reason)).CombinedOutput()
		if err != nil {
			res := OperationResult{Action: "shutdown", Succeeded: false, Message: err.Error(), Output: string(out)}
			data, _ := json.Marshal(res)
			return "failed", 1, data, nil
		}
		res := OperationResult{Action: "shutdown", Succeeded: true, Message: "shutdown scheduled"}
		data, _ := json.Marshal(res)
		return "succeeded", 0, data, nil
	}
	res := OperationResult{Action: "shutdown", Succeeded: false, Message: "unsupported on " + runtime.GOOS}
	data, _ := json.Marshal(res)
	return "failed", 1, data, nil
}

func handleServiceStart(ctx context.Context, cmd *CommandEnvelope) (string, int, []byte, error) {
	op, err := parseOperation(cmd.Payload)
	if err != nil || op.Service == "" {
		return "failed", 1, nil, fmt.Errorf("service name required")
	}
	if runtime.GOOS == "linux" {
		out, err := exec.CommandContext(ctx, "systemctl", "start", op.Service).CombinedOutput()
		if err != nil {
			data, _ := json.Marshal(OperationResult{Action: "service_start", Succeeded: false, Message: err.Error(), Output: string(out)})
			return "failed", 1, data, nil
		}
		data, _ := json.Marshal(OperationResult{Action: "service_start", Succeeded: true, Message: op.Service + " started"})
		return "succeeded", 0, data, nil
	}
	data, _ := json.Marshal(OperationResult{Action: "service_start", Succeeded: false, Message: "unsupported"})
	return "failed", 1, data, nil
}

func handleServiceStop(ctx context.Context, cmd *CommandEnvelope) (string, int, []byte, error) {
	op, err := parseOperation(cmd.Payload)
	if err != nil || op.Service == "" {
		return "failed", 1, nil, fmt.Errorf("service name required")
	}
	if runtime.GOOS == "linux" {
		out, err := exec.CommandContext(ctx, "systemctl", "stop", op.Service).CombinedOutput()
		if err != nil {
			data, _ := json.Marshal(OperationResult{Action: "service_stop", Succeeded: false, Message: err.Error(), Output: string(out)})
			return "failed", 1, data, nil
		}
		data, _ := json.Marshal(OperationResult{Action: "service_stop", Succeeded: true, Message: op.Service + " stopped"})
		return "succeeded", 0, data, nil
	}
	res := OperationResult{Action: "service_stop", Succeeded: false, Message: "unsupported"}
	data, _ := json.Marshal(res)
	return "failed", 1, data, nil
}

func handleServiceRestart(ctx context.Context, cmd *CommandEnvelope) (string, int, []byte, error) {
	op, err := parseOperation(cmd.Payload)
	if err != nil || op.Service == "" {
		return "failed", 1, nil, fmt.Errorf("service name required")
	}
	if runtime.GOOS == "linux" {
		out, err := exec.CommandContext(ctx, "systemctl", "restart", op.Service).CombinedOutput()
		if err != nil {
			data, _ := json.Marshal(OperationResult{Action: "service_restart", Succeeded: false, Message: err.Error(), Output: string(out)})
			return "failed", 1, data, nil
		}
		data, _ := json.Marshal(OperationResult{Action: "service_restart", Succeeded: true, Message: op.Service + " restarted"})
		return "succeeded", 0, data, nil
	}
	data, _ := json.Marshal(OperationResult{Action: "service_restart", Succeeded: false, Message: "unsupported"})
	return "failed", 1, data, nil
}

func handleProcessKill(ctx context.Context, cmd *CommandEnvelope) (string, int, []byte, error) {
	op, err := parseOperation(cmd.Payload)
	if err != nil || op.ProcessID <= 0 {
		return "failed", 1, nil, fmt.Errorf("valid process_id required")
	}
	if runtime.GOOS == "linux" || runtime.GOOS == "darwin" {
		out, err := exec.CommandContext(ctx, "kill", "-9", fmt.Sprintf("%d", op.ProcessID)).CombinedOutput()
		if err != nil {
			data, _ := json.Marshal(OperationResult{Action: "process_kill", Succeeded: false, Message: err.Error(), Output: string(out)})
			return "failed", 1, data, nil
		}
		data, _ := json.Marshal(OperationResult{Action: "process_kill", Succeeded: true, Message: fmt.Sprintf("process %d terminated", op.ProcessID)})
		return "succeeded", 0, data, nil
	}
	data, _ := json.Marshal(OperationResult{Action: "process_kill", Succeeded: false, Message: "unsupported"})
	return "failed", 1, data, nil
}

func init() {
	_ = time.Now
}
