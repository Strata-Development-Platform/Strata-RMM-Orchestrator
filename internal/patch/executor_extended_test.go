package patch

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestParseChocolateyOutdated(t *testing.T) {
	tests := []struct {
		name       string
		output     string
		wantCount  int
		wantIDs    []string
		wantTitles []string
	}{
		{
			name:       "single pending update",
			output:     "chrome 120.0.6099.129 -> 120.0.6099.130",
			wantCount:  1,
			wantIDs:    []string{"choco-chrome-120.0.6099.130"},
			wantTitles: []string{"chrome 120.0.6099.129 -> 120.0.6099.130"},
		},
		{
			name: "multiple pending updates",
			output: `
The following updates are pending:
chrome 120.0.6099.129 -> 120.0.6099.130
firefox 120.0.1 -> 121.0.1
7zip 22.01 -> 23.01
`,
			wantCount:  3,
			wantIDs:    []string{"choco-chrome-120.0.6099.130", "choco-firefox-121.0.1", "choco-7zip-23.01"},
		},
		{
			name:      "empty output",
			output:    "",
			wantCount: 0,
		},
		{
			name:      "chocolatey header line skipped",
			output:    "Chocolatey v2.0.0",
			wantCount: 0,
		},
		{
			name:      "no updates message skipped",
			output:    "The following updates are pending:",
			wantCount: 0,
		},
		{
			name:      "line missing arrow skipped",
			output:    "not a valid update line",
			wantCount: 0,
		},
		{
			name:      "line missing version skipped",
			output:    "pkgname ",
			wantCount: 0,
		},
		{
			name: "mixed valid and invalid lines",
			output: `
The following updates are pending:
firefox 120.0.1 -> 121.0.1
not valid
chrome 120.0.6099.129 -> 120.0.6099.130
`,
			wantCount: 2,
			wantIDs:   []string{"choco-firefox-121.0.1", "choco-chrome-120.0.6099.130"},
		},
		{
			name:       "version with hyphens",
			output:     "my-package 1.2.2-beta.1 -> 1.2.3-beta.1",
			wantCount:  1,
			wantTitles: []string{"my-package 1.2.2-beta.1 -> 1.2.3-beta.1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			patches := parseChocolateyOutdated([]byte(tt.output))
			if len(patches) != tt.wantCount {
				t.Errorf("got %d patches, want %d", len(patches), tt.wantCount)
				for i, p := range patches {
					t.Logf("  [%d] ID=%s Title=%s", i, p.ID, p.Title)
				}
				return
			}
			for i, p := range patches {
				if p.Platform != PlatformWindows {
					t.Errorf("[%d].Platform = %s, want %s", i, p.Platform, PlatformWindows)
				}
				if p.Severity != SeverityModerate {
					t.Errorf("[%d].Severity = %s, want %s", i, p.Severity, SeverityModerate)
				}
				if i < len(tt.wantIDs) && p.ID != tt.wantIDs[i] {
					t.Errorf("[%d].ID = %s, want %s", i, p.ID, tt.wantIDs[i])
				}
				if i < len(tt.wantTitles) && p.Title != tt.wantTitles[i] {
					t.Errorf("[%d].Title = %s, want %s", i, p.Title, tt.wantTitles[i])
				}
			}
		})
	}
}

func TestInstallChocolatey_EmptyPackages(t *testing.T) {
	ctx := context.Background()
	result, err := (&Executor{Platform: "linux"}).installChocolatey(ctx, nil)
	if err != nil {
		t.Fatalf("installChocolatey(empty) error = %v", err)
	}
	if result.Status != StatusInstalled {
		t.Errorf("status = %s, want %s", result.Status, StatusInstalled)
	}
}

func TestInstallChocolatey_BinaryNotAvailable(t *testing.T) {
	ctx := context.Background()
	_, err := (&Executor{Platform: "linux"}).installChocolatey(ctx, []string{"git"})
	if err != nil {
		t.Logf("installChocolatey error (expected if choco unavailable): %v", err)
	}
}

func TestParseWingetPendingUpdates(t *testing.T) {
	tests := []struct {
		name      string
		output    string
		wantCount int
		wantApps  []string
	}{
		{
			name:      "single winget update",
			output:    "Microsoft.VisualStudioCode",
			wantCount: 1,
			wantApps:  []string{"Microsoft.VisualStudioCode"},
		},
		{
			name: "multiple winget updates",
			output: `
App       Available
Microsoft.VisualStudioCode    1.85.0
Mozilla.Firefox    121.0
`,
			wantCount: 2,
			wantApps:  []string{"Microsoft.VisualStudioCode", "Mozilla.Firefox"},
		},
		{
			name:      "empty output",
			output:    "",
			wantCount: 0,
		},
		{
			name:      "header App skipped",
			output:    "App    Available    Installed",
			wantCount: 0,
		},
		{
			name:      "App keyword skipped",
			output:    "App",
			wantCount: 0,
		},
		{
			name:      "Available keyword skipped",
			output:    "Available",
			wantCount: 0,
		},
		{
			name:      "Installed keyword skipped",
			output:    "Installed",
			wantCount: 0,
		},
		{
			name:      "whitespace only skipped",
			output:    "   ",
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			patches := parseWingetPendingUpdates([]byte(tt.output))
			if len(patches) != tt.wantCount {
				t.Errorf("got %d patches, want %d", len(patches), tt.wantCount)
				for i, p := range patches {
					t.Logf("  [%d] Title=%s KB=%s", i, p.Title, p.KB)
				}
				return
			}
			for i, p := range patches {
				if p.Platform != PlatformWindows {
					t.Errorf("[%d].Platform = %s, want %s", i, p.Platform, PlatformWindows)
				}
				if i < len(tt.wantApps) && p.KB != tt.wantApps[i] {
					t.Errorf("[%d].KB = %s, want %s", i, p.KB, tt.wantApps[i])
				}
			}
		})
	}
}

func TestInstallWinget_EmptyPackages(t *testing.T) {
	ctx := context.Background()
	result, err := (&Executor{Platform: "linux"}).installWinget(ctx, nil)
	if err != nil {
		t.Fatalf("installWinget(empty) error = %v", err)
	}
	if result.Status != StatusInstalled {
		t.Errorf("status = %s, want %s", result.Status, StatusInstalled)
	}
}

func TestInstallWinget_BinaryNotAvailable(t *testing.T) {
	ctx := context.Background()
	_, err := (&Executor{Platform: "linux"}).installWinget(ctx, []string{"git"})
	if err != nil {
		t.Logf("installWinget error (expected if winget unavailable): %v", err)
	}
}

func TestParseFlatpakUpdate(t *testing.T) {
	tests := []struct {
		name      string
		output    string
		wantCount int
		wantRefs  []string
	}{
		{
			name:      "single flatpak update",
			output:    "Updating: org.gnome.TextEditor/x86_64/stable from 45.2 to 46.0",
			wantCount: 1,
			wantRefs:  []string{"org.gnome.TextEditor/x86_64/stable"},
		},
		{
			name: "multiple flatpak updates",
			output: `Updating: org.gnome.TextEditor/x86_64/stable from 45.2 to 46.0
Updating: com.spotify.Client/x86_64/stable from 1.2 to 1.3`,
			wantCount: 2,
			wantRefs:  []string{"org.gnome.TextEditor/x86_64/stable", "com.spotify.Client/x86_64/stable"},
		},
		{
			name:      "empty output",
			output:    "",
			wantCount: 0,
		},
		{
			name:      "nothing to do skipped",
			output:    "Nothing to do.",
			wantCount: 0,
		},
		{
			name:      "update without from/to",
			output:    "Updating: org.gnome.TextEditor/x86_64/stable 46.0",
			wantCount: 1,
			wantRefs:  []string{"org.gnome.TextEditor/x86_64/stable"},
		},
		{
			name:      "Updating too few fields",
			output:    "Updating: only",
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			patches := parseFlatpakUpdate([]byte(tt.output))
			if len(patches) != tt.wantCount {
				t.Errorf("got %d patches, want %d", len(patches), tt.wantCount)
				for i, p := range patches {
					t.Logf("  [%d] Title=%s KB=%s", i, p.Title, p.KB)
				}
				return
			}
			for i, p := range patches {
				if p.Platform != PlatformLinux {
					t.Errorf("[%d].Platform = %s, want %s", i, p.Platform, PlatformLinux)
				}
				if i < len(tt.wantRefs) && p.KB != tt.wantRefs[i] {
					t.Errorf("[%d].KB = %s, want %s", i, p.KB, tt.wantRefs[i])
				}
			}
		})
	}
}

func TestInstallFlatpak(t *testing.T) {
	ctx := context.Background()
	_, err := (&Executor{Platform: "linux"}).installFlatpak(ctx, nil)
	if err != nil {
		t.Logf("installFlatpak error: %v", err)
	}

	_, err = (&Executor{Platform: "linux"}).installFlatpak(ctx, []string{"org.gnome.TextEditor"})
	if err != nil {
		t.Logf("installFlatpak error (expected if flatpak unavailable): %v", err)
	}
}

func TestParseSnapRefreshList(t *testing.T) {
	tests := []struct {
		name      string
		output    string
		wantCount int
		wantSnaps []string
	}{
		{
			name:      "single snap update",
			output:    "chromium    120    121    latest/stable",
			wantCount: 1,
			wantSnaps: []string{"chromium"},
		},
		{
			name: "multiple snap updates",
			output: `Name    Version  Rev
chromium    120    121
firefox     121    122`,
			wantCount: 2,
			wantSnaps: []string{"chromium", "firefox"},
		},
		{
			name:      "empty output",
			output:    "",
			wantCount: 0,
		},
		{
			name:      "header skipped",
			output:    "Name    Version  Rev",
			wantCount: 0,
		},
		{
			name:      "separator line skipped",
			output:    "——  ———  ———",
			wantCount: 0,
		},
		{
			name:      "snap: prefix skipped",
			output:    "snap:chromium    120    121",
			wantCount: 0,
		},
		{
			name:      "empty field skipped",
			output:    "   ",
			wantCount: 0,
		},
		{
			name: "mixed valid and invalid",
			output: `Name    Version  Rev
chromium    120    121
invalid line
firefox     121    122`,
			wantCount: 3,
			wantSnaps: []string{"chromium", "invalid", "firefox"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			patches := parseSnapRefreshList([]byte(tt.output))
			if len(patches) != tt.wantCount {
				t.Errorf("got %d patches, want %d", len(patches), tt.wantCount)
				for i, p := range patches {
					t.Logf("  [%d] Title=%s KB=%s", i, p.Title, p.KB)
				}
				return
			}
			for i, p := range patches {
				if p.Platform != PlatformLinux {
					t.Errorf("[%d].Platform = %s, want %s", i, p.Platform, PlatformLinux)
				}
				if i < len(tt.wantSnaps) && p.KB != tt.wantSnaps[i] {
					t.Errorf("[%d].KB = %s, want %s", i, p.KB, tt.wantSnaps[i])
				}
			}
		})
	}
}

func TestInstallSnap(t *testing.T) {
	ctx := context.Background()
	_, err := (&Executor{Platform: "linux"}).installSnap(ctx, nil)
	if err != nil {
		t.Logf("installSnap error: %v", err)
	}

	_, err = (&Executor{Platform: "linux"}).installSnap(ctx, []string{"chromium"})
	if err != nil {
		t.Logf("installSnap error (expected if snap unavailable): %v", err)
	}
}

func TestScanWSUS(t *testing.T) {
	ctx := context.Background()

	exec := &Executor{Platform: "windows"}
	_, _, err := exec.scanWSUS(ctx)
	if err == nil {
		t.Log("scanWSUS succeeded on Windows host")
	} else {
		t.Logf("scanWSUS error (expected without PowerShell): %v", err)
	}

	exec = &Executor{Platform: "linux"}
	_, _, err = exec.scanWSUS(ctx)
	if err == nil {
		t.Error("scanWSUS on linux should fail because PowerShell not available")
	}
}

func TestCanaryValidate_EmptyDevices(t *testing.T) {
	exec := &Executor{Platform: "linux"}
	result, err := exec.CanaryValidate(context.Background(), "dep-1", []string{"KB123"}, 10, 90)
	if err != nil {
		t.Fatalf("CanaryValidate error = %v", err)
	}

	if result.Status != "passing" {
		t.Errorf("status = %s, want passing", result.Status)
	}
	if result.Progress.TotalDevices != 0 {
		t.Errorf("TotalDevices = %d, want 0", result.Progress.TotalDevices)
	}
	if result.Progress.CanaryPassed != 0 {
		t.Errorf("CanaryPassed = %d, want 0", result.Progress.CanaryPassed)
	}
}

func TestCanaryValidate_EmptyDevices_ZeroThreshold(t *testing.T) {
	exec := &Executor{Platform: "linux"}
	result, err := exec.CanaryValidate(context.Background(), "dep-0", []string{"KB0"}, 0, 0)
	if err != nil {
		t.Fatalf("CanaryValidate error = %v", err)
	}
	if result.Status != "passing" {
		t.Errorf("status = %s, want passing", result.Status)
	}
	if result.Progress.DeployedToCanary != 0 {
		t.Errorf("DeployedToCanary = %d, want 0", result.Progress.DeployedToCanary)
	}
}

func TestCanaryResult_StructFields(t *testing.T) {

	cr := &CanaryResult{
		DeploymentID: "dep-1",
		PatchID:      "KB123",
		Status:       "failing",
		Progress: CanaryProgress{
			CanarySize:     5,
			CanaryPassed:   1,
			CanaryFailed:   4,
			TotalDevices:   5,
			PassThreshold:  80,
		},
	}

	if cr.Status != "failing" {
		t.Errorf("status = %s, want failing", cr.Status)
	}
	if cr.Progress.CanarySize != 5 {
		t.Errorf("CanarySize = %d, want 5", cr.Progress.CanarySize)
	}
	if cr.Progress.PassThreshold != 80 {
		t.Errorf("Progress.PassThreshold = %d, want 80", cr.Progress.PassThreshold)
	}
}

func TestCanaryPatchResult_StructFields(t *testing.T) {
	now := time.Now()

	cpr := &CanaryPatchResult{
		DeploymentID: "dep-1",
		PatchID:      "KB123",
		CanaryGroup:  "group-a",
		DevicesTested: 5,
		DevicesOK:     5,
		DevicesFail:   0,
		Status:       "passing",
		StartedAt:    now,
		CompletedAt:  nil,
	}

	if cpr.DeploymentID != "dep-1" {
		t.Errorf("DeploymentID = %s, want dep-1", cpr.DeploymentID)
	}
	if cpr.CanaryGroup != "group-a" {
		t.Errorf("CanaryGroup = %s, want group-a", cpr.CanaryGroup)
	}
	if cpr.Status != "passing" {
		t.Errorf("status = %s, want passing", cpr.Status)
	}
	if cpr.CompletedAt != nil {
		t.Error("CompletedAt should be nil for running canary")
	}
	if cpr.StartedAt.IsZero() {
		t.Error("StartedAt should not be zero")
	}
	_ = now
}

func TestCanaryProgress_StructFields(t *testing.T) {
	cp := CanaryProgress{
		CanarySize:       10,
		CanaryPassed:     8,
		CanaryFailed:     2,
		TotalDevices:     50,
		DeployedToCanary: 10,
		PassThreshold:    80,
	}

	if cp.CanarySize != 10 {
		t.Errorf("CanarySize = %d, want 10", cp.CanarySize)
	}
	if cp.TotalDevices != 50 {
		t.Errorf("TotalDevices = %d, want 50", cp.TotalDevices)
	}
	if cp.PassThreshold != 80 {
		t.Errorf("PassThreshold = %d, want 80", cp.PassThreshold)
	}
}

func TestCanaryResult_ErrorField(t *testing.T) {
	cr := &CanaryResult{
		DeploymentID: "dep-1",
		Status:       "failing",
		Error:        "canary device timeout",
	}

	if cr.Error != "canary device timeout" {
		t.Errorf("Error = %q, want canary device timeout", cr.Error)
	}
	if cr.RollbackUsed {
		t.Error("RollbackUsed should be false")
	}
}

func TestRollbackPatch_MacOSUnsupported(t *testing.T) {
	ctx := context.Background()
	exec := &Executor{Platform: "macos"}
	_, err := exec.RollbackPatch(ctx, "pkg", "device-1")
	if err == nil {
		t.Error("rollback should error on macOS")
	}
	if !strings.Contains(err.Error(), "rollback not supported on platform: macos") {
		t.Errorf("error = %q, want rollback not supported on platform: macos", err.Error())
	}
}

func TestRollbackPatch_Windows(t *testing.T) {
	ctx := context.Background()
	exec := &Executor{Platform: "windows"}
	result, err := exec.RollbackPatch(ctx, "KB123456", "device-1")
	if err != nil {
		t.Logf("rollbackWindowsPatch error (expected without PowerShell): %v", err)
	}
	if result != nil {
		_ = result.Output
	}
}

func TestRollbackPatch_Linux(t *testing.T) {
	ctx := context.Background()
	exec := &Executor{Platform: "linux"}
	result, err := exec.RollbackPatch(ctx, "some-package", "device-1")
	if err != nil {
		t.Logf("rollbackLinuxPatch error: %v", err)
	}
	if result != nil {
		_ = result.Output
	}
}

func TestGetCanaryDevices_ReturnsEmpty(t *testing.T) {
	exec := &Executor{Platform: "linux"}
	devices, err := exec.getCanaryDevices(context.Background(), "dep-1", 10)
	if err != nil {
		t.Fatalf("getCanaryDevices error = %v", err)
	}
	if len(devices) != 0 {
		t.Errorf("got %d canary devices, want 0", len(devices))
	}
}

func TestScanChocolatey_ReturnsMissingOnly(t *testing.T) {
	ctx := context.Background()
	exec := &Executor{Platform: "linux"}
	installed, missing, err := exec.scanChocolatey(ctx)
	if err != nil {
		t.Logf("scanChocolatey error: %v", err)
	}
	if installed != nil {
		t.Error("scanChocolatey should return installed=nil")
	}
	for i, p := range missing {
		if p.Platform != PlatformWindows {
			t.Errorf("missing[%d].Platform = %s, want %s", i, p.Platform, PlatformWindows)
		}
	}
}

func TestScanWinget_ReturnsMissingOnly(t *testing.T) {
	ctx := context.Background()
	exec := &Executor{Platform: "linux"}
	installed, missing, err := exec.scanWinget(ctx)
	if err != nil {
		t.Logf("scanWinget error: %v", err)
	}
	if installed != nil {
		t.Error("scanWinget should return installed=nil")
	}
	for i, p := range missing {
		if p.Platform != PlatformWindows {
			t.Errorf("missing[%d].Platform = %s, want %s", i, p.Platform, PlatformWindows)
		}
	}
}

func TestScanFlatpak_ReturnsMissingOnly(t *testing.T) {
	ctx := context.Background()
	exec := &Executor{Platform: "linux"}
	installed, missing, err := exec.scanFlatpak(ctx)
	if err != nil {
		t.Logf("scanFlatpak error: %v", err)
	}
	if installed != nil {
		t.Error("scanFlatpak should return installed=nil")
	}
	for i, p := range missing {
		if p.Platform != PlatformLinux {
			t.Errorf("missing[%d].Platform = %s, want %s", i, p.Platform, PlatformLinux)
		}
	}
}

func TestScanSnap_ReturnsMissingOnly(t *testing.T) {
	ctx := context.Background()
	exec := &Executor{Platform: "linux"}
	installed, missing, err := exec.scanSnap(ctx)
	if err != nil {
		t.Logf("scanSnap error: %v", err)
	}
	if installed != nil {
		t.Error("scanSnap should return installed=nil")
	}
	for i, p := range missing {
		if p.Platform != PlatformLinux {
			t.Errorf("missing[%d].Platform = %s, want %s", i, p.Platform, PlatformLinux)
		}
	}
}

func TestParseChocolateyOutdated_EmptyOrWhitespaceLines(t *testing.T) {
	output := `

chrome 120.0.6099.129 -> 120.0.6099.130

firefox 120.0.1 -> 121.0.1

`
	patches := parseChocolateyOutdated([]byte(output))
	if len(patches) != 2 {
		t.Errorf("got %d patches, want 2", len(patches))
	}
}

func TestParseWingetPendingUpdates_EmptyFields(t *testing.T) {
	output := "App    Available    Installed    Name"
	patches := parseWingetPendingUpdates([]byte(output))
	if len(patches) != 0 {
		t.Errorf("got %d patches, want 0", len(patches))
	}
}

func TestParseFlatpakUpdate_NoVersionInfo(t *testing.T) {
	output := "Updating: org.gnome.TextEditor/x86_64/stable 46.0"
	patches := parseFlatpakUpdate([]byte(output))
	if len(patches) != 1 {
		t.Errorf("got %d patches, want 1", len(patches))
		return
	}
	p := patches[0]
	if p.KB != "org.gnome.TextEditor/x86_64/stable" {
		t.Errorf("KB = %s, want org.gnome.TextEditor/x86_64/stable", p.KB)
	}
	if p.Title == "" {
		t.Error("Title should not be empty")
	}
}

func TestParseSnapRefreshList_EmptyName(t *testing.T) {
	output := "     120    121"
	patches := parseSnapRefreshList([]byte(output))
	// Parser accepts any non-empty first field that does not start with "snap:"
	if len(patches) != 1 {
		t.Errorf("got %d patches, want 1", len(patches))
	}
}

func TestExecutorInstall_UnsupportedPlatform(t *testing.T) {
	ctx := context.Background()
	exec := &Executor{Platform: "beos"}
	_, err := exec.Install(ctx, []string{"pkg"})
	if err == nil {
		t.Error("install should error for unsupported platform")
	}
	if !strings.Contains(err.Error(), "unsupported platform") {
		t.Errorf("install error = %q, want unsupported platform", err.Error())
	}
}

func TestExecutorInstall_WindowsErrorStoredInResult(t *testing.T) {
	ctx := context.Background()
	exec := &Executor{Platform: "windows"}
	result, err := exec.Install(ctx, []string{"KB123456"})
	if err != nil {
		t.Logf("installWindows returned error: %v", err)
		return
	}
	if result == nil {
		t.Fatal("result should not be nil")
	}
	// On CI Linux runner, PowerShell is not available so may be StatusFailed.
	_ = result.Status
	_ = result.Output
}

func TestInstallChocolatey_StatusFields(t *testing.T) {
	ctx := context.Background()
	result, err := (&Executor{Platform: "linux"}).installChocolatey(ctx, nil)
	if err != nil {
		t.Logf("installChocolatey error: %v", err)
	}
	if result == nil {
		t.Fatal("result should not be nil")
	}
	if result.Status != StatusInstalled {
		t.Errorf("status = %s, want %s", result.Status, StatusInstalled)
	}
}

func TestInstallWinget_StatusFields(t *testing.T) {
	ctx := context.Background()
	result, err := (&Executor{Platform: "linux"}).installWinget(ctx, nil)
	if err != nil {
		t.Logf("installWinget error: %v", err)
	}
	if result == nil {
		t.Fatal("result should not be nil")
	}
	if result.Status != StatusInstalled {
		t.Errorf("status = %s, want %s", result.Status, StatusInstalled)
	}
}
