package software

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"go.etcd.io/bbolt"
	"go.uber.org/zap"

	jsmsg "github.com/strata-rmm/strata-rmm-orchestrator/internal/messaging/jetstream"
)

const (
	defaultSoftwareTimeout = 30 * time.Minute
	maxSoftwareTimeout     = 2 * time.Hour
	maxSoftwareErrorBytes  = 4096
)

type SoftwareCommand struct {
	Type          string `json:"type"`
	DeploymentID  string `json:"deployment_id"`
	Action        string `json:"action"`
	SourceURL     string `json:"source_url"`
	Checksum      string `json:"checksum"`
	InstallArgs   string `json:"install_args"`
	UninstallArgs string `json:"uninstall_args"`
	PackageType   string `json:"package_type"`
	Timeout       int    `json:"timeout"`
}

type SoftwareResult struct {
	Type         string `json:"type"`
	DeploymentID string `json:"deployment_id"`
	Action       string `json:"action"`
	Status       string `json:"status"`
	ErrorMessage string `json:"error_message,omitempty"`
	DurationMs   int64  `json:"duration_ms"`
}

type Installer struct {
	nc         *nats.Conn
	js         nats.JetStreamContext
	logger     *zap.Logger
	tenantID   string
	agentID    string
	sub        *nats.Subscription
	httpClient *http.Client
	db         *bbolt.DB
	ledger     *softwareReceiptLedger
}

// NewInstaller accepts the agent's durable bbolt database as an optional final
// argument so package-local execution tests can construct the installer without
// storage. Start fails closed without durable storage; production always passes
// the enrolled agent database.
func NewInstaller(nc *nats.Conn, logger *zap.Logger, tenantID, agentID string, databases ...*bbolt.DB) *Installer {
	if logger == nil {
		logger = zap.NewNop()
	}
	var db *bbolt.DB
	if len(databases) > 0 {
		db = databases[0]
	}
	return &Installer{
		nc:         nc,
		logger:     logger,
		tenantID:   tenantID,
		agentID:    agentID,
		httpClient: &http.Client{Timeout: 30 * time.Minute},
		db:         db,
	}
}

func (inst *Installer) Start(ctx context.Context) error {
	if ctx == nil {
		return errors.New("software installer context is required")
	}
	if inst.nc == nil {
		return errors.New("software installer NATS connection is required")
	}
	if inst.tenantID == "" || inst.agentID == "" {
		return errors.New("software installer tenant and agent identity are required")
	}
	if inst.db == nil {
		return errors.New("software installer durable database is required")
	}
	ledger, err := newSoftwareReceiptLedger(inst.db)
	if err != nil {
		return err
	}
	if err := ledger.resumeInterrupted(); err != nil {
		return fmt.Errorf("resume interrupted software commands: %w", err)
	}
	inst.ledger = ledger

	js, err := inst.nc.JetStream()
	if err != nil {
		return fmt.Errorf("software installer JetStream: %w", err)
	}
	inst.js = js

	subject := fmt.Sprintf("tenant.%s.cmd.%s", inst.tenantID, inst.agentID)
	durable := softwareDurableName(inst.tenantID, inst.agentID)
	inst.sub, err = js.Subscribe(subject, func(msg *nats.Msg) {
		go inst.handleCommand(ctx, msg)
	},
		nats.Durable(durable),
		nats.ManualAck(),
		nats.AckExplicit(),
		nats.DeliverAll(),
		nats.BindStream(jsmsg.StreamCommands),
	)
	if err != nil {
		return fmt.Errorf("subscribe durable software commands: %w", err)
	}

	go func() {
		<-ctx.Done()
		_ = inst.sub.Unsubscribe()
	}()
	return nil
}

func (inst *Installer) Stop() {
	if inst.sub != nil {
		_ = inst.sub.Unsubscribe()
	}
}

func softwareDurableName(tenantID, agentID string) string {
	sum := sha256.Sum256([]byte(tenantID + "\x00" + agentID + "\x00software"))
	return "software_" + hex.EncodeToString(sum[:12])
}

func softwareCommandFingerprint(cmd SoftwareCommand) (string, error) {
	data, err := json.Marshal(cmd)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func decodeSoftwareCommand(data []byte) (SoftwareCommand, error) {
	var cmd SoftwareCommand
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cmd); err != nil {
		return SoftwareCommand{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return SoftwareCommand{}, errors.New("software command contains multiple JSON values")
		}
		return SoftwareCommand{}, err
	}
	return cmd, nil
}

func (inst *Installer) handleCommand(parent context.Context, msg *nats.Msg) {
	cmd, err := decodeSoftwareCommand(msg.Data)
	if err != nil {
		inst.logger.Warn("discard malformed software command", zap.Error(err))
		_ = msg.Term()
		return
	}
	if err := validateSoftwareCommand(cmd); err != nil {
		inst.logger.Warn("discard invalid software command", zap.Error(err), zap.String("deployment_id", cmd.DeploymentID))
		_ = msg.Term()
		return
	}
	if inst.ledger == nil {
		inst.logger.Error("software receipt ledger unavailable")
		_ = msg.Nak()
		return
	}

	key := softwareCommandKey(cmd)
	fingerprint, err := softwareCommandFingerprint(cmd)
	if err != nil {
		inst.logger.Error("fingerprint software command", zap.Error(err))
		_ = msg.Term()
		return
	}
	disposition, replay, err := inst.ledger.begin(key, fingerprint)
	if err != nil {
		if errors.Is(err, errSoftwareCommandConflict) {
			inst.logger.Warn("reject conflicting software command", zap.String("deployment_id", cmd.DeploymentID), zap.String("action", cmd.Action))
			_ = msg.Term()
			return
		}
		inst.logger.Error("claim software command", zap.Error(err))
		_ = msg.Nak()
		return
	}
	switch disposition {
	case softwareBeginReplay:
		if err := inst.publishResult(replay); err != nil {
			_ = msg.Nak()
			return
		}
		_ = msg.Ack()
		return
	case softwareBeginInFlight:
		_ = msg.Nak()
		return
	case softwareBeginExecute:
		// continue below
	default:
		inst.logger.Error("invalid software receipt disposition")
		_ = msg.Nak()
		return
	}

	result := inst.executeWithContext(parent, cmd)
	if err := inst.ledger.complete(key, fingerprint, result); err != nil {
		inst.logger.Error("persist software result before acknowledgement", zap.Error(err), zap.String("deployment_id", cmd.DeploymentID))
		if releaseErr := inst.ledger.release(key, fingerprint); releaseErr != nil {
			inst.logger.Error("release software command after persistence failure", zap.Error(releaseErr))
		}
		_ = msg.Nak()
		return
	}
	if err := inst.publishResult(result); err != nil {
		inst.logger.Warn("publish software result; command will redeliver", zap.Error(err), zap.String("deployment_id", cmd.DeploymentID))
		_ = msg.Nak()
		return
	}
	_ = msg.Ack()
}

func validateSoftwareCommand(cmd SoftwareCommand) error {
	if cmd.DeploymentID == "" {
		return errors.New("deployment_id is required")
	}
	if cmd.Action != "install" && cmd.Action != "uninstall" {
		return fmt.Errorf("unsupported software action %q", cmd.Action)
	}
	if cmd.Type != "software_"+cmd.Action {
		return errors.New("software command type/action mismatch")
	}
	if cmd.SourceURL == "" {
		return errors.New("source_url is required")
	}
	switch cmd.PackageType {
	case "msi", "exe", "deb", "rpm", "appimage", "script":
	default:
		return fmt.Errorf("unsupported package type %q", cmd.PackageType)
	}
	if cmd.Timeout < 0 || time.Duration(cmd.Timeout)*time.Second > maxSoftwareTimeout {
		return errors.New("software timeout is outside allowed bounds")
	}
	return nil
}

func softwareCommandKey(cmd SoftwareCommand) string {
	return cmd.DeploymentID + "\x00" + cmd.Action
}

func (inst *Installer) execute(cmd SoftwareCommand) {
	result := inst.executeWithContext(context.Background(), cmd)
	_ = inst.publishResult(result)
}

func (inst *Installer) executeWithContext(parent context.Context, cmd SoftwareCommand) SoftwareResult {
	start := time.Now()
	result := SoftwareResult{Type: "software_result", DeploymentID: cmd.DeploymentID, Action: cmd.Action}
	fail := func(err error) SoftwareResult {
		result.Status = "failed"
		result.ErrorMessage = boundedSoftwareError(err)
		result.DurationMs = time.Since(start).Milliseconds()
		return result
	}

	if err := validateSoftwareCommand(cmd); err != nil {
		return fail(err)
	}
	timeout := defaultSoftwareTimeout
	if cmd.Timeout > 0 {
		timeout = time.Duration(cmd.Timeout) * time.Second
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	tmpDir, err := os.MkdirTemp("", "strata-software-*")
	if err != nil {
		return fail(fmt.Errorf("temp dir: %w", err))
	}
	defer os.RemoveAll(tmpDir)

	base := filepath.Base(cmd.SourceURL)
	if base == "." || base == string(filepath.Separator) || base == "" {
		base = "package.bin"
	}
	localPath := filepath.Join(tmpDir, base)
	if err := inst.downloadFile(ctx, cmd.SourceURL, localPath, cmd.Checksum); err != nil {
		return fail(fmt.Errorf("download: %w", err))
	}
	if err := os.Chmod(localPath, 0700); err != nil {
		return fail(fmt.Errorf("prepare package: %w", err))
	}

	var exitCode int
	switch cmd.Action {
	case "install":
		exitCode = inst.runInstall(ctx, cmd, localPath)
	case "uninstall":
		exitCode = inst.runUninstall(ctx, cmd, localPath)
	}

	result.DurationMs = time.Since(start).Milliseconds()
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		result.Status = "failed"
		result.ErrorMessage = "timeout"
		return result
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		result.Status = "failed"
		result.ErrorMessage = "cancelled"
		return result
	}
	if exitCode == 0 {
		result.Status = "success"
	} else {
		result.Status = "failed"
		result.ErrorMessage = fmt.Sprintf("exit code: %d", exitCode)
	}
	return result
}

func (inst *Installer) downloadFile(ctx context.Context, url, destPath, expectedChecksum string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := inst.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	f, err := os.OpenFile(destPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()

	h := sha256.New()
	writer := io.Writer(f)
	if expectedChecksum != "" {
		writer = io.MultiWriter(f, h)
	}
	written, err := io.Copy(writer, resp.Body)
	if err != nil {
		return err
	}
	if expectedChecksum != "" {
		actual := hex.EncodeToString(h.Sum(nil))
		if !strings.EqualFold(actual, strings.TrimSpace(expectedChecksum)) {
			return errors.New("checksum mismatch")
		}
		inst.logger.Info("download verified", zap.Int64("bytes", written))
	} else {
		inst.logger.Info("download complete", zap.Int64("bytes", written))
	}
	return nil
}

func (inst *Installer) runInstall(ctx context.Context, cmd SoftwareCommand, localPath string) int {
	args := splitSoftwareArgs(cmd.InstallArgs)
	switch cmd.PackageType {
	case "msi":
		return inst.runProgram(ctx, "msiexec.exe", append([]string{"/i", localPath, "/quiet", "/norestart"}, args...)...)
	case "exe":
		return inst.runProgram(ctx, localPath, args...)
	case "deb":
		return inst.runProgram(ctx, "dpkg", append([]string{"-i", localPath}, args...)...)
	case "rpm":
		return inst.runProgram(ctx, "rpm", append([]string{"-ivh", localPath}, args...)...)
	case "appimage":
		return inst.runProgram(ctx, localPath, append([]string{"--appimage-extract-and-run"}, args...)...)
	case "script":
		return inst.runScript(ctx, localPath, args)
	default:
		return 127
	}
}

func (inst *Installer) runUninstall(ctx context.Context, cmd SoftwareCommand, localPath string) int {
	args := splitSoftwareArgs(cmd.UninstallArgs)
	switch cmd.PackageType {
	case "msi":
		return inst.runProgram(ctx, "msiexec.exe", append([]string{"/x", localPath, "/quiet", "/norestart"}, args...)...)
	case "exe":
		return inst.runProgram(ctx, localPath, args...)
	case "deb":
		return inst.runProgram(ctx, "dpkg", append([]string{"-r", localPath}, args...)...)
	case "rpm":
		return inst.runProgram(ctx, "rpm", append([]string{"-e", localPath}, args...)...)
	case "appimage", "script":
		return inst.runProgram(ctx, localPath, args...)
	default:
		return 127
	}
}

func (inst *Installer) runMSI(ctx context.Context, msiPath, args string) int {
	return inst.runProgram(ctx, "msiexec.exe", append([]string{"/i", msiPath, "/quiet", "/norestart"}, splitSoftwareArgs(args)...)...)
}

func (inst *Installer) runExec(ctx context.Context, exePath, args string) int {
	return inst.runProgram(ctx, exePath, splitSoftwareArgs(args)...)
}

// runShell is retained for compatibility with package-local tests, but it no
// longer invokes a command shell. Callers must supply a program and arguments;
// shell metacharacters are treated as ordinary argument text.
func (inst *Installer) runShell(ctx context.Context, command string) int {
	parts := splitSoftwareArgs(command)
	if len(parts) == 0 {
		return 127
	}
	return inst.runProgram(ctx, parts[0], parts[1:]...)
}

func (inst *Installer) runProgram(ctx context.Context, program string, args ...string) int {
	cmd := exec.CommandContext(ctx, program, args...)
	cmd.Stdout = &bytes.Buffer{}
	cmd.Stderr = &bytes.Buffer{}
	if err := cmd.Run(); err != nil {
		if cmd.ProcessState != nil {
			return cmd.ProcessState.ExitCode()
		}
		return 127
	}
	if cmd.ProcessState == nil {
		return 127
	}
	return cmd.ProcessState.ExitCode()
}

func (inst *Installer) runScript(ctx context.Context, scriptPath string, args []string) int {
	if runtime.GOOS == "windows" {
		return inst.runProgram(ctx, "powershell.exe", append([]string{"-NoProfile", "-NonInteractive", "-File", scriptPath}, args...)...)
	}
	return inst.runProgram(ctx, "/bin/sh", append([]string{scriptPath}, args...)...)
}

func splitSoftwareArgs(value string) []string {
	// This intentionally does not invoke a shell. Fields provides a conservative
	// argument boundary: shell operators such as ;, &&, | and $() are passed as
	// inert argv values to the selected installer program.
	return strings.Fields(value)
}

func boundedSoftwareError(err error) string {
	if err == nil {
		return ""
	}
	value := err.Error()
	if len(value) > maxSoftwareErrorBytes {
		return value[:maxSoftwareErrorBytes]
	}
	return value
}

func (inst *Installer) publishResult(result SoftwareResult) error {
	result.ErrorMessage = boundedSoftwareError(errors.New(result.ErrorMessage))
	if result.ErrorMessage == "<nil>" {
		result.ErrorMessage = ""
	}
	data, err := json.Marshal(result)
	if err != nil {
		return err
	}
	subject := fmt.Sprintf("tenant.%s.agent.%s.software.result", inst.tenantID, inst.agentID)
	if inst.js != nil {
		if _, err := inst.js.Publish(subject, data); err != nil {
			return err
		}
	} else {
		if inst.nc == nil {
			return errors.New("software result NATS connection is required")
		}
		if err := inst.nc.Publish(subject, data); err != nil {
			return err
		}
	}
	inst.logger.Info("software result published", zap.String("deployment_id", result.DeploymentID), zap.String("status", result.Status))
	return nil
}
