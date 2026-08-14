package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
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
	registry.Register("patch_scan", handlePatchScan)
	registry.Register("patch_install", handlePatchInstall)
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
	return runOperationCommandWithTimeout(ctx, operationTimeout, name, args...)
}

func runOperationCommandWithTimeout(ctx context.Context, timeout time.Duration, name string, args ...string) (string, error) {
	if timeout <= 0 {
		timeout = operationTimeout
	}
	commandCtx, cancel := context.WithTimeout(ctx, timeout)
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

type patchPayload struct {
	PatchIDs     []string `json:"patch_ids"`
	DeploymentID string   `json:"deployment_id"`
	PolicyID     string   `json:"policy_id"`
}

func handlePatchScan(ctx context.Context, cmd *CommandEnvelope) (string, int, []byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	output, err := runPatchScan(ctx)
	if err != nil {
		return "failed", 1, marshalOperationResult(OperationResult{Action: "patch_scan", Message: err.Error()}), nil
	}

	var patches []map[string]interface{}
	if err := json.Unmarshal([]byte(output), &patches); err != nil {
		patches = []map[string]interface{}{
			{"raw_output": truncate(output, 4096)},
		}
	}

	result := OperationResult{
		Action:    "patch_scan",
		Succeeded: true,
		Message:   fmt.Sprintf("scanned %d available patches", len(patches)),
		Data:      patches,
	}
	return "succeeded", 0, marshalOperationResult(result), nil
}

func handlePatchInstall(ctx context.Context, cmd *CommandEnvelope) (string, int, []byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	var payload patchPayload
	if err := json.Unmarshal(cmd.Payload, &payload); err != nil {
		return "failed", 1, marshalOperationResult(OperationResult{Action: "patch_install", Message: "invalid payload"}), nil
	}

	if len(payload.PatchIDs) == 0 {
		return "failed", 1, marshalOperationResult(OperationResult{Action: "patch_install", Message: "no patch IDs provided"}), nil
	}

	output, rebootReq, err := runPatchInstall(ctx, payload.PatchIDs)
	result := OperationResult{
		Action: "patch_install",
		Output: truncate(output, 4096),
		Data:   map[string]bool{"reboot_required": rebootReq},
	}
	if err != nil {
		result.Succeeded = false
		result.Message = err.Error()
		return "failed", 1, marshalOperationResult(result), nil
	}

	result.Succeeded = true
	result.Message = fmt.Sprintf("installed %d patches", len(payload.PatchIDs))
	return "succeeded", 0, marshalOperationResult(result), nil
}

func runPatchScan(ctx context.Context) (string, error) {
	switch runtime.GOOS {
	case "windows":
		return runWindowsPatchScan(ctx)
	case "linux":
		return runLinuxPatchScan(ctx)
	default:
		return "", fmt.Errorf("patch scanning not supported on %s", runtime.GOOS)
	}
}

func runWindowsPatchScan(ctx context.Context) (string, error) {
	script := `$Session = New-Object -ComObject Microsoft.Update.Session
$Searcher = $Session.CreateUpdateSearcher()
$SearchResult = $Searcher.Search("IsInstalled=0 AND IsHidden=0")
$Updates = @()
foreach ($Update in $SearchResult.Updates) {
    $kb = @()
    foreach ($id in $Update.KBArticleIDs) { $kb += $id }
    $updates += @{
        Title = $Update.Title
        KB = ($kb -join ',')
        Severity = if ($Update.MsrcSeverity) { $Update.MsrcSeverity } else { 'unknown' }
        Description = $Update.Description
    }
}
$Updates | ConvertTo-Json -Compress`

	return runOperationCommandWithTimeout(ctx, 5*time.Minute, "powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", script)
}

func runLinuxPatchScan(ctx context.Context) (string, error) {
	pm := detectPackageManager()
	if pm == "" {
		return "", fmt.Errorf("no package manager found")
	}
	var cmdName string
	var cmdArgs []string
	switch pm {
	case "apt":
		cmdName = "apt"
		cmdArgs = []string{"list", "--upgradable"}
	case "dnf":
		cmdName = "dnf"
		cmdArgs = []string{"check-update"}
	case "yum":
		cmdName = "yum"
		cmdArgs = []string{"check-update"}
	case "zypper":
		cmdName = "zypper"
		cmdArgs = []string{"list-patches"}
	default:
		return "", fmt.Errorf("unsupported package manager: %s", pm)
	}
	output, err := runOperationCommandWithTimeout(ctx, 5*time.Minute, cmdName, cmdArgs...)
	if err != nil && (pm == "dnf" || pm == "yum") {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 100 {
			return output, nil
		}
	}
	return output, err
}

func runPatchInstall(ctx context.Context, patchIDs []string) (string, bool, error) {
	switch runtime.GOOS {
	case "windows":
		return runWindowsPatchInstall(ctx, patchIDs)
	case "linux":
		return runLinuxPatchInstall(ctx, patchIDs)
	default:
		return "", false, fmt.Errorf("patch installation not supported on %s", runtime.GOOS)
	}
}

func windowsPatchPattern(patchIDs []string) (string, error) {
	if len(patchIDs) == 0 {
		return "", fmt.Errorf("no patch IDs provided")
	}
	patterns := make([]string, 0, len(patchIDs))
	for _, raw := range patchIDs {
		id := strings.TrimSpace(raw)
		if id == "" || len(id) > 128 {
			return "", fmt.Errorf("invalid Windows patch identifier")
		}
		for _, r := range id {
			if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == ':' || r == '-') {
				return "", fmt.Errorf("invalid Windows patch identifier")
			}
		}
		patterns = append(patterns, regexp.QuoteMeta(id))
	}
	return "^(?:" + strings.Join(patterns, "|") + ")$", nil
}

func runWindowsPatchInstall(ctx context.Context, patchIDs []string) (string, bool, error) {
	kbPattern, err := windowsPatchPattern(patchIDs)
	if err != nil {
		return "", false, err
	}
	script := fmt.Sprintf(`$Session = New-Object -ComObject Microsoft.Update.Session
$Searcher = $Session.CreateUpdateSearcher()
$SearchResult = $Searcher.Search("IsInstalled=0 AND IsHidden=0")
$Updates = @()
foreach ($Update in $SearchResult.Updates) {
    $ids = @($Update.KBArticleIDs | ForEach-Object { $_.ToString() })
    if ($ids | Where-Object { $_ -match '%s' }) {
        $Updates += $Update
    }
}
if ($Updates.Count -eq 0) { throw 'No requested Windows updates were found' }
$Downloader = $Session.CreateUpdateDownloader()
$Downloader.Updates = $Updates
$DownloadResult = $Downloader.Download()
$Installer = New-Object -ComObject Microsoft.Update.Installer
$Installer.Updates = $Updates
$Result = $Installer.Install()
[PSCustomObject]@{ ResultCode = [int]$Result.ResultCode; RebootRequired = [bool]$Result.RebootRequired } | ConvertTo-Json -Compress`, kbPattern)

	output, runErr := runOperationCommandWithTimeout(ctx, 10*time.Minute, "powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", script)
	if runErr != nil {
		return output, false, runErr
	}
	var result struct {
		ResultCode     int  `json:"ResultCode"`
		RebootRequired bool `json:"RebootRequired"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &result); err != nil {
		return output, false, fmt.Errorf("decode Windows Update result: %w", err)
	}
	if result.ResultCode != 2 {
		return output, result.RebootRequired, fmt.Errorf("Windows Update installation returned result code %d", result.ResultCode)
	}
	return output, result.RebootRequired, nil
}

func runLinuxPatchInstall(ctx context.Context, patchIDs []string) (string, bool, error) {
	pm := detectPackageManager()
	if pm == "" {
		return "", false, fmt.Errorf("no package manager found")
	}
	args := append([]string{"install", "-y"}, patchIDs...)
	cmdName := pm
	if pm == "zypper" {
		args = append([]string{"--non-interactive", "update"}, patchIDs...)
		cmdName = "zypper"
	}
	output, err := runOperationCommandWithTimeout(ctx, 10*time.Minute, cmdName, args...)
	return output, false, err
}

func detectPackageManager() string {
	for _, pm := range []string{"apt", "dnf", "yum", "zypper"} {
		_, err := exec.LookPath(pm)
		if err == nil {
			return pm
		}
	}
	return ""
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}
