package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/strata-rmm/strata-rmm-orchestrator/internal/agent/collectors"
)

const (
	operationTimeout = 30 * time.Second
	maxCommandOutput = 64 * 1024
)

type DeviceOperation struct {
	Action    string `json:"action"`
	Reason    string `json:"reason,omitempty"`
	Service   string `json:"service,omitempty"`
	ProcessID int    `json:"process_id,omitempty"`
}

type OperationResult struct {
	Action    string      `json:"action"`
	Succeeded bool        `json:"succeeded"`
	Message   string      `json:"message,omitempty"`
	Output    string      `json:"output,omitempty"`
	Data      interface{} `json:"data,omitempty"`
}

func RegisterDeviceOperations(registry *HandlerRegistry) {
	registry.Register("device.refresh", handleRefreshInventory)
	registry.Register("device.reboot", handleReboot)
	registry.Register("device.shutdown", handleShutdown)
	registry.Register("device.service_start", handleServiceStart)
	registry.Register("device.service_stop", handleServiceStop)
	registry.Register("device.service_restart", handleServiceRestart)
	registry.Register("device.process_kill", handleProcessKill)
}

func parseOperation(payload json.RawMessage) (*DeviceOperation, error) {
	var op DeviceOperation
	decoder := json.NewDecoder(strings.NewReader(string(payload)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&op); err != nil {
		return nil, err
	}
	return &op, nil
}

func marshalOperationResult(result OperationResult) []byte {
	data, err := json.Marshal(result)
	if err != nil {
		return []byte(`{"succeeded":false,"message":"result encoding failed"}`)
	}
	return data
}

func runOperationCommand(ctx context.Context, name string, args ...string) (string, error) {
	commandCtx, cancel := context.WithTimeout(ctx, operationTimeout)
	defer cancel()
	output, err := exec.CommandContext(commandCtx, name, args...).CombinedOutput()
	if len(output) > maxCommandOutput {
		output = output[:maxCommandOutput]
	}
	if commandCtx.Err() != nil {
		return string(output), fmt.Errorf("operation timed out or was cancelled: %w", commandCtx.Err())
	}
	return string(output), err
}

func rebootCommand(goos, reason string) (string, []string, error) {
	switch goos {
	case "linux":
		return "shutdown", []string{"-r", "+1", "reboot requested: " + reason}, nil
	case "darwin":
		return "shutdown", []string{"-r", "+1"}, nil
	case "windows":
		return "shutdown.exe", []string{"/r", "/t", "60", "/d", "p:4:1", "/c", "reboot requested: " + reason}, nil
	default:
		return "", nil, fmt.Errorf("reboot unsupported on %s", goos)
	}
}

func shutdownCommand(goos, reason string) (string, []string, error) {
	switch goos {
	case "linux":
		return "shutdown", []string{"-h", "+1", "shutdown requested: " + reason}, nil
	case "darwin":
		return "shutdown", []string{"-h", "+1"}, nil
	case "windows":
		return "shutdown.exe", []string{"/s", "/t", "60", "/d", "p:4:1", "/c", "shutdown requested: " + reason}, nil
	default:
		return "", nil, fmt.Errorf("shutdown unsupported on %s", goos)
	}
}

func serviceCommand(goos, action, service string) (string, []string, error) {
	if strings.TrimSpace(service) == "" || len(service) > 256 || strings.ContainsAny(service, "\r\n\x00") {
		return "", nil, fmt.Errorf("invalid service identifier")
	}
	switch goos {
	case "linux":
		return "systemctl", []string{action, service}, nil
	case "windows":
		switch action {
		case "start":
			return "sc.exe", []string{"start", service}, nil
		case "stop":
			return "sc.exe", []string{"stop", service}, nil
		default:
			return "", nil, fmt.Errorf("service action %s requires a controlled multi-step operation on windows", action)
		}
	case "darwin":
		return "", nil, fmt.Errorf("service control unsupported on darwin without an explicit launchd domain target")
	default:
		return "", nil, fmt.Errorf("service control unsupported on %s", goos)
	}
}

func processTerminateCommand(goos string, processID int) (string, []string, error) {
	if processID <= 1 {
		return "", nil, fmt.Errorf("refusing to terminate protected process id")
	}
	pid := strconv.Itoa(processID)
	switch goos {
	case "linux", "darwin":
		return "kill", []string{"-TERM", pid}, nil
	case "windows":
		return "taskkill.exe", []string{"/PID", pid, "/T"}, nil
	default:
		return "", nil, fmt.Errorf("process termination unsupported on %s", goos)
	}
}

func handleRefreshInventory(ctx context.Context, cmd *CommandEnvelope) (string, int, []byte, error) {
	if _, err := parseOperation(cmd.Payload); err != nil {
		return "failed", 1, nil, fmt.Errorf("invalid payload: %w", err)
	}
	samples, err := collectors.NewSystemCollector(0).Collect(ctx)
	if err != nil {
		result := OperationResult{Action: "refresh", Succeeded: false, Message: err.Error()}
		return "failed", 1, marshalOperationResult(result), nil
	}
	result := OperationResult{
		Action: "refresh", Succeeded: true,
		Message: "inventory collected", Data: samples,
	}
	return "succeeded", 0, marshalOperationResult(result), nil
}

func handleReboot(ctx context.Context, cmd *CommandEnvelope) (string, int, []byte, error) {
	op, err := parseOperation(cmd.Payload)
	if err != nil {
		return "failed", 1, nil, fmt.Errorf("invalid payload: %w", err)
	}
	name, args, err := rebootCommand(runtime.GOOS, op.Reason)
	if err != nil {
		return "failed", 1, marshalOperationResult(OperationResult{Action: "reboot", Message: err.Error()}), nil
	}
	output, err := runOperationCommand(ctx, name, args...)
	if err != nil {
		return "failed", 1, marshalOperationResult(OperationResult{Action: "reboot", Message: err.Error(), Output: output}), nil
	}
	return "succeeded", 0, marshalOperationResult(OperationResult{Action: "reboot", Succeeded: true, Message: "reboot accepted", Output: output}), nil
}

func handleShutdown(ctx context.Context, cmd *CommandEnvelope) (string, int, []byte, error) {
	op, err := parseOperation(cmd.Payload)
	if err != nil {
		return "failed", 1, nil, fmt.Errorf("invalid payload: %w", err)
	}
	name, args, err := shutdownCommand(runtime.GOOS, op.Reason)
	if err != nil {
		return "failed", 1, marshalOperationResult(OperationResult{Action: "shutdown", Message: err.Error()}), nil
	}
	output, err := runOperationCommand(ctx, name, args...)
	if err != nil {
		return "failed", 1, marshalOperationResult(OperationResult{Action: "shutdown", Message: err.Error(), Output: output}), nil
	}
	return "succeeded", 0, marshalOperationResult(OperationResult{Action: "shutdown", Succeeded: true, Message: "shutdown accepted", Output: output}), nil
}

func handleServiceStart(ctx context.Context, cmd *CommandEnvelope) (string, int, []byte, error) {
	return handleServiceAction(ctx, cmd, "start")
}

func handleServiceStop(ctx context.Context, cmd *CommandEnvelope) (string, int, []byte, error) {
	return handleServiceAction(ctx, cmd, "stop")
}

func handleServiceRestart(ctx context.Context, cmd *CommandEnvelope) (string, int, []byte, error) {
	op, err := parseOperation(cmd.Payload)
	if err != nil {
		return "failed", 1, nil, fmt.Errorf("invalid payload: %w", err)
	}
	if runtime.GOOS == "windows" {
		stopName, stopArgs, stopErr := serviceCommand(runtime.GOOS, "stop", op.Service)
		if stopErr != nil {
			return "failed", 1, marshalOperationResult(OperationResult{Action: "service_restart", Message: stopErr.Error()}), nil
		}
		if output, runErr := runOperationCommand(ctx, stopName, stopArgs...); runErr != nil {
			return "failed", 1, marshalOperationResult(OperationResult{Action: "service_restart", Message: runErr.Error(), Output: output}), nil
		}
		select {
		case <-ctx.Done():
			return "cancelled", 1, marshalOperationResult(OperationResult{Action: "service_restart", Message: ctx.Err().Error()}), nil
		case <-time.After(2 * time.Second):
		}
		startName, startArgs, startErr := serviceCommand(runtime.GOOS, "start", op.Service)
		if startErr != nil {
			return "failed", 1, marshalOperationResult(OperationResult{Action: "service_restart", Message: startErr.Error()}), nil
		}
		output, runErr := runOperationCommand(ctx, startName, startArgs...)
		if runErr != nil {
			return "failed", 1, marshalOperationResult(OperationResult{Action: "service_restart", Message: runErr.Error(), Output: output}), nil
		}
		return "succeeded", 0, marshalOperationResult(OperationResult{Action: "service_restart", Succeeded: true, Message: op.Service + " restarted", Output: output}), nil
	}
	return handleServiceAction(ctx, cmd, "restart")
}

func handleServiceAction(ctx context.Context, cmd *CommandEnvelope, action string) (string, int, []byte, error) {
	op, err := parseOperation(cmd.Payload)
	if err != nil {
		return "failed", 1, nil, fmt.Errorf("invalid payload: %w", err)
	}
	name, args, err := serviceCommand(runtime.GOOS, action, op.Service)
	resultAction := "service_" + action
	if err != nil {
		return "failed", 1, marshalOperationResult(OperationResult{Action: resultAction, Message: err.Error()}), nil
	}
	output, err := runOperationCommand(ctx, name, args...)
	if err != nil {
		return "failed", 1, marshalOperationResult(OperationResult{Action: resultAction, Message: err.Error(), Output: output}), nil
	}
	return "succeeded", 0, marshalOperationResult(OperationResult{Action: resultAction, Succeeded: true, Message: op.Service + " " + action + "ed", Output: output}), nil
}

func handleProcessKill(ctx context.Context, cmd *CommandEnvelope) (string, int, []byte, error) {
	op, err := parseOperation(cmd.Payload)
	if err != nil {
		return "failed", 1, nil, fmt.Errorf("invalid payload: %w", err)
	}
	name, args, err := processTerminateCommand(runtime.GOOS, op.ProcessID)
	if err != nil {
		return "failed", 1, marshalOperationResult(OperationResult{Action: "process_kill", Message: err.Error()}), nil
	}
	output, err := runOperationCommand(ctx, name, args...)
	if err != nil {
		return "failed", 1, marshalOperationResult(OperationResult{Action: "process_kill", Message: err.Error(), Output: output}), nil
	}
	return "succeeded", 0, marshalOperationResult(OperationResult{Action: "process_kill", Succeeded: true, Message: fmt.Sprintf("termination requested for process %d", op.ProcessID), Output: output}), nil
}
