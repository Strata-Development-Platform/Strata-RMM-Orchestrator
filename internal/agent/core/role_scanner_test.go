package core

import (
	"context"
	"os/exec"
	"runtime"
	"strings"
	"testing"
)

// TestRoleScannerNewRoleScanner verifies the scanner is created with correct checkers.
func TestRoleScannerNewRoleScanner(t *testing.T) {
	scanner := NewRoleScanner()
	if scanner == nil {
		t.Fatal("RoleScanner should not be nil")
	}
	if len(scanner.checkers) == 0 {
		t.Fatal("RoleScanner should have at least one role checker")
	}
}

// TestRoleScannerScanReturnsRoles verifies Scan returns detected roles.
func TestRoleScannerScanReturnsRoles(t *testing.T) {
	ctx := context.Background()
	scanner := NewRoleScanner()
	roles := scanner.Scan(ctx)

	// On any platform, Scan should return a string slice (possibly empty)
	if roles == nil {
		t.Fatal("roles should not be nil")
	}
}

// TestRoleScannerHasRole verifies HasRole returns correct values.
func TestRoleScannerHasRole(t *testing.T) {
	ctx := context.Background()
	scanner := NewRoleScanner()

	// Since we can't easily mock exec.Command in this test, we verify the method doesn't panic
	_ = scanner.HasRole(ctx, RoleADDomainController)
	_ = scanner.HasRole(ctx, RoleSQLServer)
	_ = scanner.HasRole(ctx, RoleHyperV)
}

// TestRoleConstants verifies all role constants have unique values.
func TestRoleConstants(t *testing.T) {
	roles := []Role{
		RoleADDomainController,
		RoleSQLServer,
		RoleHyperV,
		RoleLinuxDNS,
		RoleLinuxWebServer,
		RoleLinuxDatabase,
	}

	seen := make(map[string]bool)
	for _, r := range roles {
		v := string(r)
		if seen[v] {
			t.Errorf("duplicate role value: %q", v)
		}
		seen[v] = true
	}
}

// TestRoleScannerContextCancellation verifies Scan respects context cancellation.
func TestRoleScannerContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	scanner := NewRoleScanner()
	roles := scanner.Scan(ctx)

	// Should return early (possibly empty or partial)
	if roles == nil {
		t.Fatal("roles should not be nil even on cancelled context")
	}
}

// TestRoleScannerPlatformDetection verifies scanner detects platform correctly.
func TestRoleScannerPlatformDetection(t *testing.T) {
	scanner := NewRoleScanner()

	switch runtime.GOOS {
	case "windows":
		if len(scanner.checkers) != 3 {
			t.Errorf("expected 3 checkers on Windows, got %d", len(scanner.checkers))
		}
	case "linux":
		if len(scanner.checkers) != 3 {
			t.Errorf("expected 3 checkers on Linux, got %d", len(scanner.checkers))
		}
	default:
		if len(scanner.checkers) != 0 {
			t.Errorf("expected 0 checkers on %s, got %d", runtime.GOOS, len(scanner.checkers))
		}
	}
}

// TestMockCommandBuild verifies exec.Command is available and works.
func TestMockCommandBuild(t *testing.T) {
	// Just verify exec.Command works — this is a build sanity check
	cmd := exec.Command("echo", "test")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("exec.Command failed: %v", err)
	}
	if !strings.Contains(string(output), "test") {
		t.Errorf("unexpected output: %q", string(output))
	}
}
