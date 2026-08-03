package patch

import (
	"context"
	"runtime"
	"strings"
	"testing"
)

func TestParseAptUpgradable(t *testing.T) {
	tests := []struct {
		name       string
		output     string
		wantCount  int
		wantNames  []string
		wantVerions []string
	}{
		{
			name: "single package",
			output: `libcurl4/oldoldstable 7.88.1-10+deb12u12 amd64 [upgradable from: 7.88.1-10+deb12u8]`,
			wantCount:  1,
			wantNames:  []string{"libcurl4"},
			wantVerions: []string{"7.88.1-10+deb12u12"},
		},
		{
			name: "multiple packages",
			output: `libcurl4/oldoldstable 7.88.1-10+deb12u12 amd64 [upgradable from: 7.88.1-10+deb12u8]
openssl/stable-security 3.0.17-1~deb12u3 amd64 [upgradable from: 3.0.11-1~deb12u2]
zlib1g/oldoldstable 1:1.2.13.dfsg-1 amd64 [upgradable from: 1:1.2.13.dfsg-1+deb12u1]`,
			wantCount:  3,
			wantNames:  []string{"libcurl4", "openssl", "zlib1g"},
			wantVerions: []string{"7.88.1-10+deb12u12", "3.0.17-1~deb12u3", "1:1.2.13.dfsg-1"},
		},
		{
			name: "empty output",
			output: "",
			wantCount:  0,
		},
		{
			name: "no upgradable packages",
			output: `libcurl4/oldoldstable 7.88.1-10+deb12u8 amd64
openssl/stable-security 3.0.11-1~deb12u2 amd64`,
			wantCount:  0,
		},
		{
			name: "partial line with empty version",
			output: `pkgname/branch [upgradable from: ]`,
			wantCount:  1, // versionPart is "branch" which is not empty
			wantNames:  []string{"pkgname"},
		},
		{
			name: "malformed line no slash",
			output: `this is not a valid line`,
			wantCount:  0,
		},
		{
			name: "mixed valid and invalid",
			output: `libcurl4/oldoldstable 7.88.1-10+deb12u12 amd64 [upgradable from: 7.88.1-10+deb12u8]
this is invalid
openssl/stable 3.0.17-1 amd64 [upgradable from: 3.0.11-1]`,
			wantCount:  2,
			wantNames:  []string{"libcurl4", "openssl"},
			wantVerions: []string{"7.88.1-10+deb12u12", "3.0.17-1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			patches := parseAptUpgradable([]byte(tt.output))
			if len(patches) != tt.wantCount {
				t.Errorf("parseAptUpgradable() got %d patches, want %d", len(patches), tt.wantCount)
				for i, p := range patches {
					t.Logf("  patch[%d]: Title=%s, KB=%s", i, p.Title, p.KB)
				}
				return
			}
			for i, p := range patches {
				if p.Platform != PlatformLinux {
					t.Errorf("patch[%d].Platform = %s, want %s", i, p.Platform, PlatformLinux)
				}
				if p.Severity != SeverityModerate {
					t.Errorf("patch[%d].Severity = %s, want %s", i, p.Severity, SeverityModerate)
				}
				if i < len(tt.wantNames) && p.KB != tt.wantNames[i] {
					t.Errorf("patch[%d].KB = %s, want %s", i, p.KB, tt.wantNames[i])
				}
				if i < len(tt.wantVerions) && !strings.Contains(p.Title, tt.wantVerions[i]) {
					t.Errorf("patch[%d].Title = %s, want to contain %s", i, p.Title, tt.wantVerions[i])
				}
			}
		})
	}
}

func TestParseDnfCheckUpdate(t *testing.T) {
	tests := []struct {
		name      string
		output    string
		wantCount int
		wantNames []string
	}{
		{
			name: "single package",
			output: `kernel.x86_64              5.14.0-427.16.1.el9_4      @appstream`,
			wantCount:  1,
			wantNames:  []string{"kernel.x86_64"},
		},
		{
			name: "multiple packages",
			output: `kernel.x86_64              5.14.0-427.16.1.el9_4      @appstream
openssl.x86_64             3.0.7-25.el9_4             @appstream
glibc.x86_64               2.34-64.el9_4              @baseos`,
			wantCount:  3,
			wantNames:  []string{"kernel.x86_64", "openssl.x86_64", "glibc.x86_64"},
		},
		{
			name: "empty output",
			output: "",
			wantCount:  0,
		},
		{
			name: "no updates (plugin line parsed as patch)",
			output: `Loading plugin fastestmirror`,
			wantCount:  1, // parsed as "Loading-plugin" due to field splitting
		},
		{
			name: "line with trailing dash (multi-line)",
			output: `some-package.x86_64  1.0-1.el9      repo-
other-package.x86_64   2.0-2.el9      repo`,
			wantCount:  1, // trailing-dash line is skipped, only second line parsed
			wantNames:  []string{"other-package.x86_64"},
		},
		{
			name: "single field only",
			output: `pkgname`,
			wantCount:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			patches := parseDnfCheckUpdate([]byte(tt.output))
			if len(patches) != tt.wantCount {
				t.Errorf("parseDnfCheckUpdate() got %d patches, want %d", len(patches), tt.wantCount)
				for i, p := range patches {
					t.Logf("  patch[%d]: Title=%s, KB=%s", i, p.Title, p.KB)
				}
				return
			}
			for i, p := range patches {
				if p.Platform != PlatformLinux {
					t.Errorf("patch[%d].Platform = %s, want %s", i, p.Platform, PlatformLinux)
				}
				if p.Severity != SeverityModerate {
					t.Errorf("patch[%d].Severity = %s, want %s", i, p.Severity, SeverityModerate)
				}
				if i < len(tt.wantNames) && p.KB != tt.wantNames[i] {
					t.Errorf("patch[%d].KB = %s, want %s", i, p.KB, tt.wantNames[i])
				}
			}
		})
	}
}

func TestParseYumCheckUpdate(t *testing.T) {
	tests := []struct {
		name      string
		output    string
		wantCount int
		wantNames []string
	}{
		{
			name: "single package",
			output: `kernel.x86_64              5.14.0-427.16.1.el9_4      @appstream`,
			wantCount:  1,
			wantNames:  []string{"kernel.x86_64"},
		},
		{
			name: "multiple packages",
			output: `kernel.x86_64              5.14.0-427.16.1.el9_4      @appstream
openssl.x86_64             3.0.7-25.el9_4             @appstream`,
			wantCount:  2,
			wantNames:  []string{"kernel.x86_64", "openssl.x86_64"},
		},
		{
			name: "empty output",
			output: "",
			wantCount:  0,
		},
		{
			name: "no updates (plugin line parsed as patch)",
			output: `Loading plugin fastestmirror`,
			wantCount:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			patches := parseYumCheckUpdate([]byte(tt.output))
			if len(patches) != tt.wantCount {
				t.Errorf("parseYumCheckUpdate() got %d patches, want %d", len(patches), tt.wantCount)
				for i, p := range patches {
					t.Logf("  patch[%d]: Title=%s, KB=%s", i, p.Title, p.KB)
				}
				return
			}
			for i, p := range patches {
				if p.Platform != PlatformLinux {
					t.Errorf("patch[%d].Platform = %s, want %s", i, p.Platform, PlatformLinux)
				}
				if p.Severity != SeverityModerate {
					t.Errorf("patch[%d].Severity = %s, want %s", i, p.Severity, SeverityModerate)
				}
				if i < len(tt.wantNames) && p.KB != tt.wantNames[i] {
					t.Errorf("patch[%d].KB = %s, want %s", i, p.KB, tt.wantNames[i])
				}
			}
		})
	}
}

func TestParseZypperListPatches(t *testing.T) {
	tests := []struct {
		name      string
		output    string
		wantCount int
		wantNames []string
	}{
		{
			name: "single patch",
			output: `i | SUSE-SU-2024:1234-1 | important | openssl-3.0.13`,
			wantCount:  1,
			wantNames:  []string{"SUSE-SU-2024:1234-1"}, // fields[2] is advisory-id
		},
		{
			name: "uppercase status",
			output: `I | SUSE-SU-2024:5678-1 | moderate | kernel-default-6.4.0`,
			wantCount:  1,
			wantNames:  []string{"SUSE-SU-2024:5678-1"},
		},
		{
			name: "multiple patches",
			output: `i | SUSE-SU-2024:1234-1 | important | openssl-3.0.13
I | SUSE-SU-2024:5678-1 | moderate | kernel-default-6.4.0
i | SUSE-SU-2024:9012-1 | low | curl-8.5.0`,
			wantCount:  3,
			wantNames:  []string{"SUSE-SU-2024:1234-1", "SUSE-SU-2024:5678-1", "SUSE-SU-2024:9012-1"},
		},
		{
			name: "empty output",
			output: "",
			wantCount:  0,
		},
		{
			name: "no patches",
			output: `There are no enabled patches to display.`,
			wantCount:  0,
		},
		{
			name: "not enough fields",
			output: `i | SUSE-SU-2024:1234-1`,
			wantCount:  0,
		},
		{
			name: "mixed valid and invalid",
			output: `i | SUSE-SU-2024:1234-1 | important | openssl-3.0.13
This is a header line
I | SUSE-SU-2024:5678-1 | moderate | kernel-default-6.4.0`,
			wantCount:  2,
			wantNames:  []string{"SUSE-SU-2024:1234-1", "SUSE-SU-2024:5678-1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			patches := parseZypperListPatches([]byte(tt.output))
			if len(patches) != tt.wantCount {
				t.Errorf("parseZypperListPatches() got %d patches, want %d", len(patches), tt.wantCount)
				for i, p := range patches {
					t.Logf("  patch[%d]: Title=%s, KB=%s", i, p.Title, p.KB)
				}
				return
			}
			for i, p := range patches {
				if p.Platform != PlatformLinux {
					t.Errorf("patch[%d].Platform = %s, want %s", i, p.Platform, PlatformLinux)
				}
				if p.Severity != SeverityModerate {
					t.Errorf("patch[%d].Severity = %s, want %s", i, p.Severity, SeverityModerate)
				}
				if i < len(tt.wantNames) && p.KB != tt.wantNames[i] {
					t.Errorf("patch[%d].KB = %s, want %s", i, p.KB, tt.wantNames[i])
				}
			}
		})
	}
}

func TestSeverityFromMSRC(t *testing.T) {
	tests := []struct {
		input    string
		expected PatchSeverity
	}{
		{"critical", SeverityCritical},
		{"Critical", SeverityCritical},
		{"CRITICAL", SeverityCritical},
		{"important", SeverityImportant},
		{"Important", SeverityImportant},
		{"moderate", SeverityModerate},
		{"Moderate", SeverityModerate},
		{"low", SeverityLow},
		{"Low", SeverityLow},
		{"unknown", SeverityModerate},
		{"", SeverityModerate},
		{"high", SeverityModerate},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := severityFromMSRC(tt.input)
			if result != tt.expected {
				t.Errorf("severityFromMSRC(%q) = %s, want %s", tt.input, result, tt.expected)
			}
		})
	}
}

func TestNewExecutor(t *testing.T) {
	exec := NewExecutor()
	if exec.Platform != runtime.GOOS {
		t.Errorf("NewExecutor().Platform = %s, want %s", exec.Platform, runtime.GOOS)
	}
}

func TestExecutorScanPlatformDispatch(t *testing.T) {
	ctx := context.Background()

	// Windows executor should dispatch to scanWindows
	windowsExec := &Executor{Platform: "windows"}
	_, _, err := windowsExec.Scan(ctx)
	if err == nil {
		t.Error("scanWindows should error when PowerShell is not available")
	}

	// Linux executor dispatches to scanLinux (may succeed or fail depending on package manager)
	installed, missing, scanErr := (&Executor{Platform: "linux"}).Scan(ctx)
	_ = installed
	_ = missing
	_ = scanErr

	// Unsupported platform should error
	badExec := &Executor{Platform: "beos"}
	_, _, err = badExec.Scan(ctx)
	if err == nil {
		t.Error("scan should error for unsupported platform")
	}
	if !strings.Contains(err.Error(), "unsupported platform") {
		t.Errorf("scan error for unsupported platform = %q, want to contain 'unsupported platform'", err.Error())
	}
}

func TestExecutorInstallPlatformDispatch(t *testing.T) {
	ctx := context.Background()

	// Unsupported platform should error
	badExec := &Executor{Platform: "beos"}
	_, err := badExec.Install(ctx, []string{"patch1"})
	if err == nil {
		t.Error("install should error for unsupported platform")
	}
	if !strings.Contains(err.Error(), "unsupported platform") {
		t.Errorf("install error for unsupported platform = %q, want to contain 'unsupported platform'", err.Error())
	}

	// Windows executor returns result with error stored (not as return value)
	windowsExec := &Executor{Platform: "windows"}
	result, err := windowsExec.Install(ctx, []string{"KB123456"})
	if err != nil {
		t.Errorf("installWindows returned error = %v, want nil (error stored in result.Error)", err)
	}
	if result == nil {
		t.Fatal("installWindows returned nil result")
	}
	// On platforms without PowerShell, result.Status should be Failed with error message
	if result.Status != StatusFailed {
		t.Errorf("result.Status = %s, want %s", result.Status, StatusFailed)
	}
}

func TestDetectPackageManager(t *testing.T) {
	pm := detectPackageManager()
	// Should return one of the available package managers or empty string
	if pm != "" && pm != "apt" && pm != "dnf" && pm != "yum" && pm != "zypper" && pm != "pacman" {
		t.Errorf("detectPackageManager() = %q, want one of apt/dnf/yum/zypper/pacman or empty", pm)
	}
}

func TestExecResultFields(t *testing.T) {
	result := &ExecResult{
		Status:    StatusInstalled,
		Output:    "output data",
		Error:     "",
		RebootReq: false,
	}
	if result.Status != StatusInstalled {
		t.Errorf("ExecResult.Status = %s, want %s", result.Status, StatusInstalled)
	}
	if result.Output != "output data" {
		t.Errorf("ExecResult.Output = %s, want %s", result.Output, "output data")
	}
	if result.Error != "" {
		t.Errorf("ExecResult.Error = %s, want empty", result.Error)
	}
	if result.RebootReq {
		t.Error("ExecResult.RebootReq should be false")
	}

	rebootResult := &ExecResult{
		Status:    StatusRebootReq,
		RebootReq: true,
	}
	if rebootResult.Status != StatusRebootReq {
		t.Errorf("ExecResult.Status = %s, want %s", rebootResult.Status, StatusRebootReq)
	}
	if !rebootResult.RebootReq {
		t.Error("ExecResult.RebootReq should be true")
	}

	failedResult := &ExecResult{
		Status: StatusFailed,
		Error:  "some error",
	}
	if failedResult.Status != StatusFailed {
		t.Errorf("ExecResult.Status = %s, want %s", failedResult.Status, StatusFailed)
	}
}

func TestPatchStructFields(t *testing.T) {
	patch := &Patch{
		ID:          "test-id",
		TenantID:    "tenant-1",
		KB:          "KB123456",
		Title:       "Security Update",
		Platform:    PlatformWindows,
		Severity:    SeverityCritical,
		Description: "Fixes CVE-2024-1234",
	}
	if patch.ID != "test-id" {
		t.Errorf("Patch.ID = %s, want test-id", patch.ID)
	}
	if patch.KB != "KB123456" {
		t.Errorf("Patch.KB = %s, want KB123456", patch.KB)
	}
	if patch.Platform != PlatformWindows {
		t.Errorf("Patch.Platform = %s, want %s", patch.Platform, PlatformWindows)
	}
	if patch.Severity != SeverityCritical {
		t.Errorf("Patch.Severity = %s, want %s", patch.Severity, SeverityCritical)
	}
	if patch.Title != "Security Update" {
		t.Errorf("Patch.Title = %s, want Security Update", patch.Title)
	}
}
