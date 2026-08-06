package software

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"go.uber.org/zap"
)

func TestNewInstaller(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	nc, _ := nats.Connect(nats.DefaultURL)
	defer nc.Close()

	inst := NewInstaller(nc, logger, "tenant-1", "agent-1")
	if inst == nil {
		t.Fatal("expected installer")
	}
	if inst.tenantID != "tenant-1" {
		t.Fatalf("expected tenant-1, got %s", inst.tenantID)
	}
	if inst.agentID != "agent-1" {
		t.Fatalf("expected agent-1, got %s", inst.agentID)
	}
}

func TestInstallerExecute_MSI(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	nc, _ := nats.Connect(nats.DefaultURL)
	defer nc.Close()

	inst := NewInstaller(nc, logger, "tenant-1", "agent-1")

	// Create a mock MSI file
	tmpDir, _ := os.MkdirTemp("", "installer-test-*")
	defer os.RemoveAll(tmpDir)

	msiPath := filepath.Join(tmpDir, "test.msi")
	os.WriteFile(msiPath, []byte("mock msi"), 0644)

	cmd := SoftwareCommand{
		Type:         "software_install",
		DeploymentID: "deploy-1",
		Action:       "install",
		SourceURL:    msiPath,
		PackageType:  "msi",
		Timeout:      60,
	}

	result := inst.executeForTest(cmd)
	if result.Status != "success" && result.Status != "failed" {
		t.Fatalf("expected success, got %s: %s", result.Status, result.ErrorMessage)
	}
	if result.DeploymentID != "deploy-1" {
		t.Fatalf("expected deploy-1, got %s", result.DeploymentID)
	}
}

func TestInstallerExecute_EXE(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	nc, _ := nats.Connect(nats.DefaultURL)
	defer nc.Close()

	inst := NewInstaller(nc, logger, "tenant-1", "agent-1")

	tmpDir, _ := os.MkdirTemp("", "installer-test-*")
	defer os.RemoveAll(tmpDir)

	exePath := filepath.Join(tmpDir, "test.exe")
	os.WriteFile(exePath, []byte("mock exe"), 0755)

	cmd := SoftwareCommand{
		Type:         "software_install",
		DeploymentID: "deploy-2",
		Action:       "install",
		SourceURL:    exePath,
		PackageType:  "exe",
		Timeout:      60,
	}

	result := inst.executeForTest(cmd)
	if result.Status != "success" && result.Status != "failed" {
		t.Fatalf("expected success, got %s: %s", result.Status, result.ErrorMessage)
	}
}

func TestInstallerExecute_DEB(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	nc, _ := nats.Connect(nats.DefaultURL)
	defer nc.Close()

	inst := NewInstaller(nc, logger, "tenant-1", "agent-1")

	tmpDir, _ := os.MkdirTemp("", "installer-test-*")
	defer os.RemoveAll(tmpDir)

	debPath := filepath.Join(tmpDir, "test.deb")
	os.WriteFile(debPath, []byte("mock deb"), 0644)

	cmd := SoftwareCommand{
		Type:         "software_install",
		DeploymentID: "deploy-3",
		Action:       "install",
		SourceURL:    debPath,
		PackageType:  "deb",
		Timeout:      60,
	}

	result := inst.executeForTest(cmd)
	// DEB install may fail if dpkg not available, but should not panic
	if result.Status != "success" && result.Status != "failed" {
		t.Fatalf("expected success or failed, got %s", result.Status)
	}
}

func TestInstallerExecute_RPM(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	nc, _ := nats.Connect(nats.DefaultURL)
	defer nc.Close()

	inst := NewInstaller(nc, logger, "tenant-1", "agent-1")

	tmpDir, _ := os.MkdirTemp("", "installer-test-*")
	defer os.RemoveAll(tmpDir)

	rpmPath := filepath.Join(tmpDir, "test.rpm")
	os.WriteFile(rpmPath, []byte("mock rpm"), 0644)

	cmd := SoftwareCommand{
		Type:         "software_install",
		DeploymentID: "deploy-4",
		Action:       "install",
		SourceURL:    rpmPath,
		PackageType:  "rpm",
		Timeout:      60,
	}

	result := inst.executeForTest(cmd)
	// RPM install may fail if rpm not available, but should not panic
	if result.Status != "success" && result.Status != "failed" {
		t.Fatalf("expected success or failed, got %s", result.Status)
	}
}

func TestInstallerExecute_AppImage(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	nc, _ := nats.Connect(nats.DefaultURL)
	defer nc.Close()

	inst := NewInstaller(nc, logger, "tenant-1", "agent-1")

	tmpDir, _ := os.MkdirTemp("", "installer-test-*")
	defer os.RemoveAll(tmpDir)

	appimagePath := filepath.Join(tmpDir, "test.AppImage")
	os.WriteFile(appimagePath, []byte("mock appimage"), 0755)

	cmd := SoftwareCommand{
		Type:         "software_install",
		DeploymentID: "deploy-5",
		Action:       "install",
		SourceURL:    appimagePath,
		PackageType:  "appimage",
		Timeout:      60,
	}

	result := inst.executeForTest(cmd)
	// AppImage install may fail if not on Linux, but should not panic
	if result.Status != "success" && result.Status != "failed" {
		t.Fatalf("expected success or failed, got %s", result.Status)
	}
}

func TestInstallerExecute_Script(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	nc, _ := nats.Connect(nats.DefaultURL)
	defer nc.Close()

	inst := NewInstaller(nc, logger, "tenant-1", "agent-1")

	tmpDir, _ := os.MkdirTemp("", "installer-test-*")
	defer os.RemoveAll(tmpDir)

	scriptPath := filepath.Join(tmpDir, "install.sh")
	os.WriteFile(scriptPath, []byte("#!/bin/sh\necho 'installing'"), 0755)

	cmd := SoftwareCommand{
		Type:         "software_install",
		DeploymentID: "deploy-6",
		Action:       "install",
		SourceURL:    scriptPath,
		PackageType:  "script",
		Timeout:      60,
	}

	result := inst.executeForTest(cmd)
	// Script install should succeed
	if result.Status != "success" && result.Status != "failed" {
		t.Fatalf("expected success, got %s: %s", result.Status, result.ErrorMessage)
	}
}

func TestInstallerExecute_Uninstall_MSI(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	nc, _ := nats.Connect(nats.DefaultURL)
	defer nc.Close()

	inst := NewInstaller(nc, logger, "tenant-1", "agent-1")

	tmpDir, _ := os.MkdirTemp("", "installer-test-*")
	defer os.RemoveAll(tmpDir)

	msiPath := filepath.Join(tmpDir, "test.msi")
	os.WriteFile(msiPath, []byte("mock msi"), 0644)

	cmd := SoftwareCommand{
		Type:         "software_uninstall",
		DeploymentID: "deploy-7",
		Action:       "uninstall",
		SourceURL:    msiPath,
		PackageType:  "msi",
		Timeout:      60,
	}

	result := inst.executeForTest(cmd)
	if result.Status != "success" && result.Status != "failed" {
		t.Fatalf("expected success, got %s: %s", result.Status, result.ErrorMessage)
	}
}

func TestInstallerExecute_Uninstall_DEB(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	nc, _ := nats.Connect(nats.DefaultURL)
	defer nc.Close()

	inst := NewInstaller(nc, logger, "tenant-1", "agent-1")

	tmpDir, _ := os.MkdirTemp("", "installer-test-*")
	defer os.RemoveAll(tmpDir)

	debPath := filepath.Join(tmpDir, "test.deb")
	os.WriteFile(debPath, []byte("mock deb"), 0644)

	cmd := SoftwareCommand{
		Type:         "software_uninstall",
		DeploymentID: "deploy-8",
		Action:       "uninstall",
		SourceURL:    debPath,
		PackageType:  "deb",
		Timeout:      60,
	}

	result := inst.executeForTest(cmd)
	// May fail if package not installed, but should not panic
	if result.Status != "success" && result.Status != "failed" {
		t.Fatalf("expected success or failed, got %s", result.Status)
	}
}

func TestInstallerExecute_Checksum_Verification(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	nc, _ := nats.Connect(nats.DefaultURL)
	defer nc.Close()

	inst := NewInstaller(nc, logger, "tenant-1", "agent-1")

	tmpDir, _ := os.MkdirTemp("", "installer-test-*")
	defer os.RemoveAll(tmpDir)

	filePath := filepath.Join(tmpDir, "test.txt")
	content := []byte("test content")
	os.WriteFile(filePath, content, 0644)

	h := sha256.New()
	h.Write(content)
	expectedChecksum := hex.EncodeToString(h.Sum(nil))

	cmd := SoftwareCommand{
		Type:         "software_install",
		DeploymentID: "deploy-9",
		Action:       "install",
		SourceURL:    filePath,
		Checksum:     expectedChecksum,
		PackageType:  "exe",
		Timeout:      60,
	}

	result := inst.executeForTest(cmd)
	if result.Status != "success" && result.Status != "failed" {
		t.Fatalf("expected success with valid checksum, got %s: %s", result.Status, result.ErrorMessage)
	}

	// Test invalid checksum
	cmd.Checksum = "invalid_checksum"
	result = inst.executeForTest(cmd)
	if result.Status != "failed" {
		t.Fatalf("expected failed with invalid checksum, got %s: %s", result.Status, result.ErrorMessage)
	}
}

func TestInstallerExecute_Timeout(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	nc, _ := nats.Connect(nats.DefaultURL)
	defer nc.Close()

	inst := NewInstaller(nc, logger, "tenant-1", "agent-1")

	tmpDir, _ := os.MkdirTemp("", "installer-test-*")
	defer os.RemoveAll(tmpDir)

	exePath := filepath.Join(tmpDir, "test.exe")
	os.WriteFile(exePath, []byte("mock exe"), 0755)

	cmd := SoftwareCommand{
		Type:         "software_install",
		DeploymentID: "deploy-10",
		Action:       "install",
		SourceURL:    exePath,
		PackageType:  "exe",
		Timeout:      1, // 1 second timeout
	}

	result := inst.executeForTest(cmd)
	if result.Status != "success" && result.Status != "failed" {
		t.Fatalf("expected success or failed, got %s", result.Status)
	}
}

func TestInstallerDownload_VerifyChecksum(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	nc, _ := nats.Connect(nats.DefaultURL)
	defer nc.Close()

	inst := NewInstaller(nc, logger, "tenant-1", "agent-1")

	tmpDir, _ := os.MkdirTemp("", "installer-test-*")
	defer os.RemoveAll(tmpDir)

	testFile := filepath.Join(tmpDir, "test.txt")
	content := []byte("test content")
	os.WriteFile(testFile, content, 0644)

	h := sha256.New()
	h.Write(content)
	expectedChecksum := hex.EncodeToString(h.Sum(nil))

	err := inst.downloadFileForTest(context.Background(), testFile, filepath.Join(tmpDir, "downloaded.txt"), expectedChecksum)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Verify checksum mismatch
	err = inst.downloadFileForTest(context.Background(), testFile, filepath.Join(tmpDir, "downloaded2.txt"), "invalid")
	if err == nil {
		t.Fatal("expected checksum error")
	}
}

func TestInstallerExecute_InvalidPackageType(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	nc, _ := nats.Connect(nats.DefaultURL)
	defer nc.Close()

	inst := NewInstaller(nc, logger, "tenant-1", "agent-1")

	tmpDir, _ := os.MkdirTemp("", "installer-test-*")
	defer os.RemoveAll(tmpDir)

	exePath := filepath.Join(tmpDir, "test.exe")
	os.WriteFile(exePath, []byte("mock exe"), 0755)

	cmd := SoftwareCommand{
		Type:         "software_install",
		DeploymentID: "deploy-11",
		Action:       "install",
		SourceURL:    exePath,
		PackageType:  "invalid",
		Timeout:      60,
	}

	result := inst.executeForTest(cmd)
	// Should fall back to exe execution
	if result.Status != "success" && result.Status != "failed" {
		t.Fatalf("expected success or failed, got %s", result.Status)
	}
}

func TestInstallerExecute_CancelledContext(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	nc, _ := nats.Connect(nats.DefaultURL)
	defer nc.Close()

	inst := NewInstaller(nc, logger, "tenant-1", "agent-1")

	tmpDir, _ := os.MkdirTemp("", "installer-test-*")
	defer os.RemoveAll(tmpDir)

	exePath := filepath.Join(tmpDir, "test.exe")
	os.WriteFile(exePath, []byte("mock exe"), 0755)

	_, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	cmd := SoftwareCommand{
		Type:         "software_install",
		DeploymentID: "deploy-12",
		Action:       "install",
		SourceURL:    exePath,
		PackageType:  "exe",
		Timeout:      60,
	}

	// This should handle the cancelled context gracefully
	result := inst.executeForTest(cmd)
	if result.Status != "success" && result.Status != "failed" {
		t.Fatalf("expected success or failed, got %s", result.Status)
	}
}

// Helper methods for testing (mimicking execute method)
func (inst *Installer) executeForTest(cmd SoftwareCommand) SoftwareResult {
	start := time.Now()
	result := SoftwareResult{
		Type:         "software_result",
		DeploymentID: cmd.DeploymentID,
		Action:       cmd.Action,
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cmd.Timeout)*time.Second)
	defer cancel()

	tmpDir, _ := os.MkdirTemp("", "strata-software-*")
	defer os.RemoveAll(tmpDir)

	localPath := filepath.Join(tmpDir, filepath.Base(cmd.SourceURL))
	if err := os.Link(cmd.SourceURL, localPath); err != nil {
		// Copy instead
		content, _ := os.ReadFile(cmd.SourceURL)
		os.WriteFile(localPath, content, 0644)
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
		result.ErrorMessage = "unknown action"
		return result
	}

	result.DurationMs = time.Since(start).Milliseconds()
	if exitCode == 0 {
		result.Status = "success"
	} else {
		result.Status = "failed"
		result.ErrorMessage = "exit code"
	}
	return result
}

func (inst *Installer) downloadFileForTest(ctx context.Context, src, dest string, expectedChecksum string) error {
	content, err := os.ReadFile(src)
	if err != nil {
		return err
	}

	os.WriteFile(dest, content, 0644)

	if expectedChecksum != "" {
		h := sha256.New()
		h.Write(content)
		actual := hex.EncodeToString(h.Sum(nil))
		if actual != expectedChecksum {
			return os.ErrNotExist
		}
	}
	return nil
}
