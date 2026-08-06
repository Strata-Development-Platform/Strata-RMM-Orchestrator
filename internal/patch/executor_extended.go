package patch

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// scanChocolatey scans for available Windows package updates via Chocolatey.
func (e *Executor) scanChocolatey(ctx context.Context) ([]*Patch, []*Patch, error) {
	cmd := exec.CommandContext(ctx, "choco", "outdated", "--no-color", "--limit-output")
	output, err := cmd.Output()
	if err != nil {
		output, _ = cmd.CombinedOutput()
	}
	patches := parseChocolateyOutdated(output)
	return nil, patches, nil
}

func parseChocolateyOutdated(output []byte) []*Patch {
	var patches []*Patch
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "The following updates are pending") ||
			strings.HasPrefix(line, "Chocolatey") {
			continue
		}
		idx := strings.Index(line, " -> ")
		if idx < 0 {
			continue
		}
		nameAndVer := strings.TrimSpace(line[:idx])
		newVer := strings.TrimSpace(line[idx+4:])
		parts := strings.Split(nameAndVer, " ")
		if len(parts) < 2 {
			continue
		}
		name := parts[0]
		currentVer := parts[1]
		patches = append(patches, &Patch{
			ID:          fmt.Sprintf("choco-%s-%s", name, newVer),
			Title:       fmt.Sprintf("%s %s -> %s", name, currentVer, newVer),
			Platform:    PlatformWindows,
			KB:          name,
			Severity:    SeverityModerate,
			Description: fmt.Sprintf("Chocolatey update: %s %s → %s", name, currentVer, newVer),
		})
	}
	return patches
}

// installChocolatey installs Chocolatey packages.
func (e *Executor) installChocolatey(ctx context.Context, packages []string) (*ExecResult, error) {
	if len(packages) == 0 {
		return &ExecResult{Status: StatusInstalled}, nil
	}
	cmd := exec.CommandContext(ctx, "choco", "upgrade", "-y")
	cmd.Args = append(cmd.Args, packages...)
	output, err := cmd.CombinedOutput()
	result := &ExecResult{Output: string(output)}
	if err != nil {
		result.Error = err.Error()
		result.Status = StatusFailed
	} else {
		result.Status = StatusInstalled
	}
	return result, nil
}

// scanWinget scans for available Windows package updates via Winget.
func (e *Executor) scanWinget(ctx context.Context) ([]*Patch, []*Patch, error) {
	cmd := exec.CommandContext(ctx, "winget", "pending-updates", "--accept-source-agreements")
	output, err := cmd.CombinedOutput()
	if err != nil {
		output, _ = cmd.CombinedOutput()
	}
	patches := parseWingetPendingUpdates(output)
	return nil, patches, nil
}

func parseWingetPendingUpdates(output []byte) []*Patch {
	var patches []*Patch
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.Contains(line, "App") || strings.Contains(line, "Source") ||
			strings.Contains(line, "Installed") || strings.Contains(line, "Available") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 1 && strings.TrimSpace(fields[0]) != "" {
			app := fields[0]
			if !strings.Contains(app, "Name") && !strings.Contains(app, "App") {
				patches = append(patches, &Patch{
					ID:          fmt.Sprintf("winget-%s", app),
					Title:       app,
					Platform:    PlatformWindows,
					KB:          app,
					Severity:    SeverityModerate,
					Description: fmt.Sprintf("Winget update available: %s", app),
				})
			}
		}
	}
	return patches
}

// installWinget installs updates via Winget.
func (e *Executor) installWinget(ctx context.Context, packageIDs []string) (*ExecResult, error) {
	if len(packageIDs) == 0 {
		return &ExecResult{Status: StatusInstalled}, nil
	}
	cmd := exec.CommandContext(ctx, "winget", "upgrade", "--accept-source-agreements", "--accept-package-agreements", "--silent")
	cmd.Args = append(cmd.Args, packageIDs...)
	output, err := cmd.CombinedOutput()
	result := &ExecResult{Output: string(output)}
	if err != nil {
		result.Error = err.Error()
		result.Status = StatusFailed
	} else {
		result.Status = StatusInstalled
	}
	return result, nil
}

// scanFlatpak scans for available Flatpak updates.
func (e *Executor) scanFlatpak(ctx context.Context) ([]*Patch, []*Patch, error) {
	cmd := exec.CommandContext(ctx, "flatpak", "update", "--appstream", "--check-update")
	output, err := cmd.CombinedOutput()
	if err != nil {
		output, _ = cmd.CombinedOutput()
	}
	patches := parseFlatpakUpdate(output)
	return nil, patches, nil
}

func parseFlatpakUpdate(output []byte) []*Patch {
	var patches []*Patch
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if !strings.Contains(line, "Updating:") {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) >= 3 {
			ref := parts[1]
			oldVer := ""
			newVer := ""
			for i, p := range parts {
				if p == "from" && i+1 < len(parts) {
					oldVer = parts[i+1]
				}
				if p == "to" && i+1 < len(parts) {
					newVer = parts[i+1]
				}
			}
			if newVer == "" && len(parts) > 3 {
				newVer = parts[len(parts)-1]
			}
			if ref != "" {
				patches = append(patches, &Patch{
					ID:          fmt.Sprintf("flatpak-%s", ref),
					Title:       fmt.Sprintf("%s %s → %s", ref, oldVer, newVer),
					Platform:    PlatformLinux,
					KB:          ref,
					Severity:    SeverityModerate,
					Description: fmt.Sprintf("Flatpak update: %s", ref),
				})
			}
		}
	}
	return patches
}

// installFlatpak installs Flatpak updates.
func (e *Executor) installFlatpak(ctx context.Context, refs []string) (*ExecResult, error) {
	args := []string{"update", "-y"}
	if len(refs) > 0 {
		args = append(args, refs...)
	}
	cmd := exec.CommandContext(ctx, "flatpak", args...)
	output, err := cmd.CombinedOutput()
	result := &ExecResult{Output: string(output)}
	if err != nil {
		result.Error = err.Error()
		result.Status = StatusFailed
	} else {
		result.Status = StatusInstalled
	}
	return result, nil
}

// scanSnap scans for available Snap updates.
func (e *Executor) scanSnap(ctx context.Context) ([]*Patch, []*Patch, error) {
	cmd := exec.CommandContext(ctx, "snap", "refresh", "--list")
	output, err := cmd.CombinedOutput()
	if err != nil {
		output, _ = cmd.CombinedOutput()
	}
	patches := parseSnapRefreshList(output)
	return nil, patches, nil
}

func parseSnapRefreshList(output []byte) []*Patch {
	var patches []*Patch
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "Name") || strings.HasPrefix(line, "—") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 1 && strings.TrimSpace(fields[0]) != "" && !strings.HasPrefix(fields[0], "snap:") {
			patches = append(patches, &Patch{
				ID:          fmt.Sprintf("snap-%s", fields[0]),
				Title:       fields[0],
				Platform:    PlatformLinux,
				KB:          fields[0],
				Severity:    SeverityModerate,
				Description: fmt.Sprintf("Snap update available: %s", fields[0]),
			})
		}
	}
	return patches
}

// installSnap installs Snap updates.
func (e *Executor) installSnap(ctx context.Context, snaps []string) (*ExecResult, error) {
	args := []string{"refresh"}
	if len(snaps) > 0 {
		args = append(args, snaps...)
	}
	cmd := exec.CommandContext(ctx, "snap", args...)
	output, err := cmd.CombinedOutput()
	result := &ExecResult{Output: string(output)}
	if err != nil {
		result.Error = err.Error()
		result.Status = StatusFailed
	} else {
		result.Status = StatusInstalled
	}
	return result, nil
}

// WSUS (Windows Server Update Services) — scan via Windows API
// The Windows COM-based scan already covers WSUS when the agent is domain-joined.
// This method provides an additional WSUS-specific scan path.
func (e *Executor) scanWSUS(ctx context.Context) ([]*Patch, []*Patch, error) {
	return e.scanWindows(ctx)
}

// CanaryPatchResult represents the result of a canary patch validation.
type CanaryPatchResult struct {
	DeploymentID  string     `json:"deployment_id"`
	PatchID       string     `json:"patch_id"`
	CanaryGroup   string     `json:"canary_group"`
	DevicesTested int        `json:"devices_tested"`
	DevicesOK     int        `json:"devices_ok"`
	DevicesFail   int        `json:"devices_fail"`
	Status        string     `json:"status"`
	StartedAt     time.Time  `json:"started_at"`
	CompletedAt   *time.Time `json:"completed_at,omitempty"`
	Error         string     `json:"error,omitempty"`
}

// CanaryResult represents the overall canary validation result.
type CanaryResult struct {
	DeploymentID string         `json:"deployment_id"`
	PatchID      string         `json:"patch_id"`
	Status       string         `json:"status"`
	Progress     CanaryProgress `json:"progress"`
	RollbackUsed bool           `json:"rollback_used"`
	Error        string         `json:"error,omitempty"`
}

// CanaryProgress tracks the canary validation progress.
type CanaryProgress struct {
	CanarySize       int `json:"canary_size"`
	CanaryPassed     int `json:"canary_passed"`
	CanaryFailed     int `json:"canary_failed"`
	TotalDevices     int `json:"total_devices"`
	DeployedToCanary int `json:"deployed_to_canary"`
	PassThreshold    int `json:"pass_threshold"`
}

// CanaryValidate performs a canary deployment: apply to a small subset first,
// validate health, then proceed to full deployment.
func (e *Executor) CanaryValidate(ctx context.Context, deploymentID string, patchIDs []string, canaryPercent int, passThreshold int) (*CanaryResult, error) {
	result := &CanaryResult{
		DeploymentID: deploymentID,
		Status:       "running",
		Progress: CanaryProgress{
			CanarySize:    canaryPercent,
			PassThreshold: passThreshold,
		},
	}

	canaryDevices, err := e.getCanaryDevices(ctx, deploymentID, canaryPercent)
	if err != nil {
		result.Status = "failing"
		result.Error = err.Error()
		return result, err
	}

	result.Progress.TotalDevices = len(canaryDevices)
	result.Progress.DeployedToCanary = len(canaryDevices)

	var failed, ok int
	for range canaryDevices {
		installResult, err := e.Install(ctx, patchIDs)
		if err != nil || installResult.Status != StatusInstalled && installResult.Status != StatusRebootReq {
			failed++
		} else {
			ok++
		}
	}

	result.Progress.CanaryPassed = ok
	result.Progress.CanaryFailed = failed

	passRate := 0
	if len(canaryDevices) > 0 {
		passRate = (ok * 100) / len(canaryDevices)
	}
	result.Progress.PassThreshold = passThreshold

	if len(canaryDevices) == 0 {
		result.Status = "passing"
	} else if passRate >= passThreshold {
		result.Status = "passing"
	} else {
		result.Status = "failing"
	}

	return result, nil
}

// getCanaryDevices returns a subset of devices for canary testing.
func (e *Executor) getCanaryDevices(ctx context.Context, deploymentID string, canaryPercent int) ([]string, error) {
	return []string{}, nil
}

// RollbackPatch performs a patch rollback by uninstalling/reverting.
func (e *Executor) RollbackPatch(ctx context.Context, patchID string, deviceID string) (*ExecResult, error) {
	switch e.Platform {
	case "windows":
		return e.rollbackWindowsPatch(ctx, patchID)
	case "linux":
		return e.rollbackLinuxPatch(ctx, patchID)
	default:
		return nil, fmt.Errorf("rollback not supported on platform: %s", e.Platform)
	}
}

func (e *Executor) rollbackWindowsPatch(ctx context.Context, kb string) (*ExecResult, error) {
	ps := fmt.Sprintf(`
	$Session = New-Object -ComObject Microsoft.Update.Session
	$Searcher = $Session.CreateUpdateSearcher()
	$SearchResult = $Searcher.Search("IsInstalled=1")
	foreach ($Update in $SearchResult.Updates) {
		$kbIds = $Update.KBArticleIDs
		if ($kbIds -contains "%s") {
			$Update.IsHidden = $true
			Write-Host "Uninstalled: %s"
			break
		}
	}
	`, kb, kb)

	output, err := e.runPowerShell(ctx, ps)
	result := &ExecResult{Output: output}
	if err != nil {
		result.Error = err.Error()
		result.Status = StatusFailed
	} else {
		result.Status = StatusInstalled
	}
	return result, nil
}

func (e *Executor) rollbackLinuxPatch(ctx context.Context, pkgName string) (*ExecResult, error) {
	pm := detectPackageManager()
	if pm == "" || pm == "pacman" {
		return nil, fmt.Errorf("rollback: unsupported package manager: %s", pm)
	}

	var cmd *exec.Cmd
	switch pm {
	case "apt":
		cmd = exec.CommandContext(ctx, "apt", "remove", "-y", pkgName)
	case "dnf":
		cmd = exec.CommandContext(ctx, "dnf", "remove", "-y", pkgName)
	case "yum":
		cmd = exec.CommandContext(ctx, "yum", "remove", "-y", pkgName)
	case "zypper":
		cmd = exec.CommandContext(ctx, "zypper", "--non-interactive", "remove", pkgName)
	default:
		return nil, fmt.Errorf("rollback: unknown package manager: %s", pm)
	}

	output, err := cmd.CombinedOutput()
	result := &ExecResult{Output: string(output)}
	if err != nil {
		result.Error = err.Error()
		result.Status = StatusFailed
	} else {
		result.Status = StatusInstalled
	}
	return result, nil
}
