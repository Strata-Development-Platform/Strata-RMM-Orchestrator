package scripts

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"go.uber.org/zap"
)

type ScriptCommand struct {
	Type        string            `json:"type"`
	ExecutionID string            `json:"execution_id"`
	Language    string            `json:"language"`
	Content     string            `json:"content"`
	Parameters  map[string]string `json:"parameters"`
	Timeout     int               `json:"timeout"`
}

type ScriptResult struct {
	Type        string `json:"type"`
	ExecutionID string `json:"execution_id"`
	Status      string `json:"status"`
	Stdout      string `json:"stdout"`
	Stderr      string `json:"stderr"`
	ExitCode    int    `json:"exit_code"`
	DurationMs  int64  `json:"duration_ms"`
}

// RunJob executes a durable job payload without publishing on the legacy
// script-result subject. The durable jobs dispatcher owns acknowledgement,
// result persistence and delivery.
func (e *Executor) RunJob(ctx context.Context, payload json.RawMessage) (string, int, []byte, error) {
	var cmd ScriptCommand
	if err := json.Unmarshal(payload, &cmd); err != nil {
		return "failed", -1, nil, fmt.Errorf("decode script payload: %w", err)
	}
	if cmd.Timeout <= 0 {
		cmd.Timeout = 300
	}
	content, err := interpolateParams(cmd.Content, cmd.Parameters)
	if err != nil {
		return "failed", -1, nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(cmd.Timeout)*time.Second)
	defer cancel()
	var stdout, stderr bytes.Buffer
	exitCode := -1
	switch cmd.Language {
	case "powershell":
		exitCode = e.runPowerShell(ctx, content, &stdout, &stderr)
	case "bash":
		exitCode = e.runBash(ctx, content, &stdout, &stderr)
	case "python":
		exitCode = e.runPython(ctx, content, &stdout, &stderr)
	case "batch":
		exitCode = e.runBatch(ctx, content, &stdout, &stderr)
	default:
		return "failed", -1, nil, fmt.Errorf("unsupported language: %s", cmd.Language)
	}
	result, err := json.Marshal(map[string]interface{}{
		"stdout": truncateOutput(stdout.String()), "stderr": truncateOutput(stderr.String()),
	})
	if err != nil {
		return "failed", exitCode, nil, err
	}
	if ctx.Err() != nil {
		return "failed", exitCode, result, ctx.Err()
	}
	if exitCode != 0 {
		return "failed", exitCode, result, fmt.Errorf("script exited with code %d", exitCode)
	}
	return "succeeded", exitCode, result, nil
}

type Executor struct {
	nc       *nats.Conn
	logger   *zap.Logger
	tenantID string
	agentID  string
	sub      *nats.Subscription
}

func NewExecutor(nc *nats.Conn, logger *zap.Logger, tenantID, agentID string) *Executor {
	return &Executor{
		nc:       nc,
		logger:   logger,
		tenantID: tenantID,
		agentID:  agentID,
	}
}

func (e *Executor) Start(ctx context.Context) error {
	subject := fmt.Sprintf("tenant.%s.cmd.%s", e.tenantID, e.agentID)
	var err error
	e.sub, err = e.nc.Subscribe(subject, func(msg *nats.Msg) {
		var cmd ScriptCommand
		if err := json.Unmarshal(msg.Data, &cmd); err != nil {
			return
		}
		if cmd.Type != "script_exec" {
			return
		}
		go e.execute(cmd)
	})
	if err != nil {
		return fmt.Errorf("subscribe script commands: %w", err)
	}

	go func() {
		<-ctx.Done()
		e.sub.Unsubscribe()
	}()
	return nil
}

func (e *Executor) Stop() {
	if e.sub != nil {
		e.sub.Unsubscribe()
	}
}

func (e *Executor) execute(cmd ScriptCommand) {
	start := time.Now()

	e.logger.Info("executing script",
		zap.String("execution_id", cmd.ExecutionID),
		zap.String("language", cmd.Language),
	)

	result := ScriptResult{
		Type:        "script_result",
		ExecutionID: cmd.ExecutionID,
	}

	content, err := interpolateParams(cmd.Content, cmd.Parameters)
	if err != nil {
		result.Status = "failed"
		result.Stderr = fmt.Sprintf("Parameter interpolation error: %v", err)
		result.DurationMs = time.Since(start).Milliseconds()
		e.publishResult(result)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cmd.Timeout)*time.Second)
	defer cancel()

	var stdout, stderr bytes.Buffer
	var exitCode int

	switch cmd.Language {
	case "powershell":
		exitCode = e.runPowerShell(ctx, content, &stdout, &stderr)
	case "bash":
		exitCode = e.runBash(ctx, content, &stdout, &stderr)
	case "python":
		exitCode = e.runPython(ctx, content, &stdout, &stderr)
	case "batch":
		exitCode = e.runBatch(ctx, content, &stdout, &stderr)
	default:
		result.Status = "failed"
		result.Stderr = fmt.Sprintf("unsupported language: %s", cmd.Language)
		result.DurationMs = time.Since(start).Milliseconds()
		e.publishResult(result)
		return
	}

	elapsed := time.Since(start).Milliseconds()

	result.Stdout = truncateOutput(stdout.String())
	result.Stderr = truncateOutput(stderr.String())
	result.ExitCode = exitCode
	result.DurationMs = elapsed

	switch {
	case exitCode == 0:
		result.Status = "success"
	case ctx.Err() == context.DeadlineExceeded:
		result.Status = "timeout"
	default:
		result.Status = "failed"
	}

	e.publishResult(result)
}

func (e *Executor) runPowerShell(ctx context.Context, content string, stdout, stderr *bytes.Buffer) int {
	script := fmt.Sprintf(`$ErrorActionPreference = "Stop"
%s`, content)

	cmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", script)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.Run()
	return cmd.ProcessState.ExitCode()
}

func (e *Executor) runBash(ctx context.Context, content string, stdout, stderr *bytes.Buffer) int {
	cmd := exec.CommandContext(ctx, "bash", "-c", content)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.Run()
	return cmd.ProcessState.ExitCode()
}

func (e *Executor) runPython(ctx context.Context, content string, stdout, stderr *bytes.Buffer) int {
	cmd := exec.CommandContext(ctx, "python3", "-c", content)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.Run()
	return cmd.ProcessState.ExitCode()
}

func (e *Executor) runBatch(ctx context.Context, content string, stdout, stderr *bytes.Buffer) int {
	tmpFile := filepath.Join(os.TempDir(), fmt.Sprintf("strata-script-%d.bat", time.Now().UnixNano()))
	os.WriteFile(tmpFile, []byte(content), 0700)
	defer os.Remove(tmpFile)

	cmd := exec.CommandContext(ctx, "cmd.exe", "/C", tmpFile)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.Run()
	return cmd.ProcessState.ExitCode()
}

func (e *Executor) publishResult(result ScriptResult) {
	subject := fmt.Sprintf("tenant.%s.agent.%s.script.result", e.tenantID, e.agentID)
	data, err := json.Marshal(result)
	if err != nil {
		e.logger.Error("marshal script result", zap.Error(err))
		return
	}
	if err := e.nc.Publish(subject, data); err != nil {
		e.logger.Error("publish script result", zap.Error(err))
		return
	}
	e.logger.Info("script result published",
		zap.String("execution_id", result.ExecutionID),
		zap.String("status", result.Status),
		zap.Int64("duration_ms", result.DurationMs),
	)
}

func interpolateParams(content string, params map[string]string) (string, error) {
	result := content
	for k, v := range params {
		result = strings.ReplaceAll(result, fmt.Sprintf("{{%s}}", k), v)
		result = strings.ReplaceAll(result, fmt.Sprintf("${{%s}}", k), v)
	}
	return result, nil
}

func truncateOutput(s string) string {
	if len(s) > 100000 {
		return s[:100000] + "\n... [output truncated at 100KB]"
	}
	return s
}

func SupportedLanguages() []string {
	return []string{"powershell", "bash", "python", "batch"}
}

func DetectPlatform() string {
	return runtime.GOOS
}
