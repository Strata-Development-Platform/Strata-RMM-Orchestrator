package software

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/nats-io/nats.go"
	"go.uber.org/zap"
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
	logger     *zap.Logger
	tenantID   string
	agentID    string
	sub        *nats.Subscription
	httpClient *http.Client
}

func NewInstaller(nc *nats.Conn, logger *zap.Logger, tenantID, agentID string) *Installer {
	return &Installer{
		nc:         nc,
		logger:     logger,
		tenantID:   tenantID,
		agentID:    agentID,
		httpClient: &http.Client{Timeout: 30 * time.Minute},
	}
}

func (inst *Installer) Start(ctx context.Context) error {
	subject := fmt.Sprintf("tenant.%s.cmd.%s", inst.tenantID, inst.agentID)
	var err error
	inst.sub, err = inst.nc.Subscribe(subject, func(msg *nats.Msg) {
		var cmd SoftwareCommand
		if err := json.Unmarshal(msg.Data, &cmd); err != nil {
			return
		}
		if cmd.Type != "software_install" && cmd.Type != "software_uninstall" {
			return
		}
		go inst.execute(cmd)
	})
	if err != nil {
		return fmt.Errorf("subscribe software commands: %w", err)
	}

	go func() {
		<-ctx.Done()
		inst.sub.Unsubscribe()
	}()
	return nil
}

func (inst *Installer) Stop() {
	if inst.sub != nil {
		inst.sub.Unsubscribe()
	}
}

func (inst *Installer) execute(cmd SoftwareCommand) {
	start := time.Now()
	result := SoftwareResult{
		Type:         "software_result",
		DeploymentID: cmd.DeploymentID,
		Action:       cmd.Action,
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cmd.Timeout)*time.Second)
	defer cancel()

	tmpDir, err := os.MkdirTemp("", "strata-software-*")
	if err != nil {
		result.Status = "failed"
		result.ErrorMessage = fmt.Sprintf("temp dir: %v", err)
		result.DurationMs = time.Since(start).Milliseconds()
		inst.publishResult(result)
		return
	}
	defer os.RemoveAll(tmpDir)

	localPath := filepath.Join(tmpDir, filepath.Base(cmd.SourceURL))
	if err := inst.downloadFile(ctx, cmd.SourceURL, localPath, cmd.Checksum); err != nil {
		result.Status = "failed"
		result.ErrorMessage = fmt.Sprintf("download: %v", err)
		result.DurationMs = time.Since(start).Milliseconds()
		inst.publishResult(result)
		return
	}

	os.Chmod(localPath, 0755)

	var exitCode int
	switch cmd.Action {
	case "install":
		exitCode = inst.runInstall(ctx, cmd, localPath)
	case "uninstall":
		exitCode = inst.runUninstall(ctx, cmd, localPath)
	default:
		result.Status = "failed"
		result.ErrorMessage = fmt.Sprintf("unknown action: %s", cmd.Action)
		result.DurationMs = time.Since(start).Milliseconds()
		inst.publishResult(result)
		return
	}

	result.DurationMs = time.Since(start).Milliseconds()
	if exitCode == 0 {
		result.Status = "success"
	} else {
		result.Status = "failed"
		result.ErrorMessage = fmt.Sprintf("exit code: %d", exitCode)
	}
	if ctx.Err() == context.DeadlineExceeded {
		result.Status = "failed"
		result.ErrorMessage = "timeout"
	}
	inst.publishResult(result)
}

func (inst *Installer) downloadFile(ctx context.Context, url, destPath, expectedChecksum string) error {
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	resp, err := inst.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	f, _ := os.Create(destPath)
	defer f.Close()

	if expectedChecksum != "" {
		h := sha256.New()
		written, _ := io.Copy(f, io.TeeReader(resp.Body, h))
		actual := hex.EncodeToString(h.Sum(nil))
		if actual != expectedChecksum {
			return fmt.Errorf("checksum mismatch: expected %s, got %s", expectedChecksum, actual)
		}
		inst.logger.Info("download verified", zap.Int64("bytes", written))
	} else {
		written, _ := io.Copy(f, resp.Body)
		inst.logger.Info("download complete", zap.Int64("bytes", written))
	}
	return nil
}

func (inst *Installer) runInstall(ctx context.Context, cmd SoftwareCommand, localPath string) int {
	switch cmd.PackageType {
	case "msi":
		return inst.runMSI(ctx, localPath, cmd.InstallArgs)
	case "exe":
		return inst.runExec(ctx, localPath, cmd.InstallArgs)
	case "deb":
		return inst.runShell(ctx, fmt.Sprintf("dpkg -i %s %s", localPath, cmd.InstallArgs))
	case "rpm":
		return inst.runShell(ctx, fmt.Sprintf("rpm -ivh %s %s", localPath, cmd.InstallArgs))
	default:
		return inst.runExec(ctx, localPath, cmd.InstallArgs)
	}
}

func (inst *Installer) runUninstall(ctx context.Context, cmd SoftwareCommand, localPath string) int {
	switch cmd.PackageType {
	case "msi":
		return inst.runMSI(ctx, localPath, cmd.UninstallArgs)
	case "deb":
		return inst.runShell(ctx, fmt.Sprintf("dpkg -r %s %s", localPath, cmd.UninstallArgs))
	case "rpm":
		return inst.runShell(ctx, fmt.Sprintf("rpm -e %s %s", localPath, cmd.UninstallArgs))
	default:
		return inst.runExec(ctx, localPath, cmd.UninstallArgs)
	}
}

func (inst *Installer) runMSI(ctx context.Context, msiPath, args string) int {
	cmd := exec.CommandContext(ctx, "msiexec.exe", "/i", msiPath, "/quiet", "/norestart")
	if args != "" {
		cmd.Args = append(cmd.Args, args)
	}
	cmd.Stdout = &bytes.Buffer{}
	cmd.Stderr = &bytes.Buffer{}
	cmd.Run()
	return cmd.ProcessState.ExitCode()
}

func (inst *Installer) runExec(ctx context.Context, exePath, args string) int {
	cmd := exec.CommandContext(ctx, exePath)
	if args != "" {
		cmd.Args = append(cmd.Args, args)
	}
	cmd.Stdout = &bytes.Buffer{}
	cmd.Stderr = &bytes.Buffer{}
	cmd.Run()
	return cmd.ProcessState.ExitCode()
}

func (inst *Installer) runShell(ctx context.Context, command string) int {
	shell := "/bin/sh"
	flag := "-c"
	if runtime.GOOS == "windows" {
		shell = "cmd.exe"
		flag = "/C"
	}
	cmd := exec.CommandContext(ctx, shell, flag, command)
	cmd.Stdout = &bytes.Buffer{}
	cmd.Stderr = &bytes.Buffer{}
	cmd.Run()
	return cmd.ProcessState.ExitCode()
}

func (inst *Installer) publishResult(result SoftwareResult) {
	subject := fmt.Sprintf("tenant.%s.agent.%s.software.result", inst.tenantID, inst.agentID)
	data, _ := json.Marshal(result)
	inst.nc.Publish(subject, data)
	inst.logger.Info("software result published",
		zap.String("deployment_id", result.DeploymentID),
		zap.String("status", result.Status),
	)
}
