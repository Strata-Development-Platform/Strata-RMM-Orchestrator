package patch

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

type Executor struct {
	Platform string
}

func NewExecutor() *Executor {
	return &Executor{Platform: runtime.GOOS}
}

type ExecResult struct {
	Status    PatchStatus
	Output    string
	Error     string
	RebootReq bool
}

func (e *Executor) Scan(ctx context.Context) ([]*Patch, []*Patch, error) {
	switch e.Platform {
	case "windows":
		return e.scanWindows(ctx)
	case "linux":
		return e.scanLinux(ctx)
	default:
		return nil, nil, fmt.Errorf("unsupported platform: %s", e.Platform)
	}
}

func (e *Executor) Install(ctx context.Context, patchIDs []string) (*ExecResult, error) {
	switch e.Platform {
	case "windows":
		return e.installWindows(ctx, patchIDs)
	case "linux":
		return e.installLinux(ctx, patchIDs)
	default:
		return nil, fmt.Errorf("unsupported platform: %s", e.Platform)
	}
}

func (e *Executor) scanWindows(ctx context.Context) ([]*Patch, []*Patch, error) {
	ps := `$Session = New-Object -ComObject Microsoft.Update.Session
$Searcher = $Session.CreateUpdateSearcher()
$SearchResult = $Searcher.Search("IsInstalled=0 AND IsHidden=0")
$Updates = $SearchResult.Updates | Select-Object Title, KBArticleIDs, MsrcSeverity, Description, Categories, LastDeploymentChangeTime
$Updates | ConvertTo-Json -Compress`

	output, err := e.runPowerShell(ctx, ps)
	if err != nil {
		return nil, nil, fmt.Errorf("scan patches: %w", err)
	}

	_ = output
	return nil, nil, nil
}

func (e *Executor) installWindows(ctx context.Context, patchIDs []string) (*ExecResult, error) {
	kbFilter := strings.Join(patchIDs, ",")
	ps := fmt.Sprintf(`$Session = New-Object -ComObject Microsoft.Update.Session
$Searcher = $Session.CreateUpdateSearcher()
$SearchResult = $Searcher.Search("IsInstalled=0 AND IsHidden=0")
$Updates = $SearchResult.Updates | Where-Object { $_.KBArticleIDs -join ',' -match '%s' }
$Downloader = $Session.CreateUpdateDownloader()
$Downloader.Updates = $Updates
$Downloader.Download()
$Installer = New-Object -ComObject Microsoft.Update.Installer
$Installer.Updates = $Updates
$Result = $Installer.Install()
$Result.ResultCode`, kbFilter)

	output, err := e.runPowerShell(ctx, ps)
	result := &ExecResult{Output: output}

	if err != nil {
		result.Error = err.Error()
		result.Status = StatusFailed
	} else if strings.Contains(output, "2") { // ResultCode 2 = reboot required
		result.Status = StatusRebootReq
		result.RebootReq = true
	} else {
		result.Status = StatusInstalled
	}

	return result, nil
}

func (e *Executor) scanLinux(ctx context.Context) ([]*Patch, []*Patch, error) {
	// Detect package manager
	pm := detectPackageManager()

	var cmd *exec.Cmd
	switch pm {
	case "apt":
		cmd = exec.CommandContext(ctx, "apt", "list", "--upgradable")
	case "dnf", "yum":
		cmd = exec.CommandContext(ctx, pm, "check-update")
	case "zypper":
		cmd = exec.CommandContext(ctx, "zypper", "list-patches")
	default:
		return nil, nil, fmt.Errorf("unknown package manager: %s", pm)
	}

	output, err := cmd.Output()
	if err != nil {
		return nil, nil, fmt.Errorf("scan linux patches: %w", err)
	}

	_ = output
	return nil, nil, nil
}

func (e *Executor) installLinux(ctx context.Context, packages []string) (*ExecResult, error) {
	pm := detectPackageManager()
	pkgList := strings.Join(packages, " ")

	var cmd *exec.Cmd
	switch pm {
	case "apt":
		cmd = exec.CommandContext(ctx, "apt", "install", "-y", pkgList)
	case "dnf", "yum":
		cmd = exec.CommandContext(ctx, pm, "install", "-y", pkgList)
	case "zypper":
		cmd = exec.CommandContext(ctx, "zypper", "install", "-y", pkgList)
	default:
		return nil, fmt.Errorf("unknown package manager: %s", pm)
	}

	output, err := cmd.Output()
	result := &ExecResult{Output: string(output)}

	if err != nil {
		result.Error = err.Error()
		result.Status = StatusFailed
	} else {
		result.Status = StatusInstalled
	}

	return result, nil
}

func (e *Executor) runPowerShell(ctx context.Context, script string) (string, error) {
	cmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", script)
	output, err := cmd.Output()
	if err != nil {
		return string(output), fmt.Errorf("powershell error: %w", err)
	}
	return string(output), nil
}

func detectPackageManager() string {
	// Ordered by preference
	for _, pm := range []string{"apt", "dnf", "yum", "zypper", "pacman"} {
		if _, err := exec.LookPath(pm); err == nil {
			return pm
		}
	}
	return ""
}

func init() {
	_ = time.Second
}
