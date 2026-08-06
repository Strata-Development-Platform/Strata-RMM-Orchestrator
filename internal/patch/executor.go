package patch

import (
	"context"
	"encoding/json"
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

type windowsPatch struct {
	Title        string   `json:"Title"`
	KBArticleIDs []string `json:"KBArticleIDs"`
	MsrcSeverity string   `json:"MsrcSeverity"`
	Description  string   `json:"Description"`
	Categories   []string `json:"Categories"`
	DeployTime   string   `json:"LastDeploymentChangeTime"`
}

func (e *Executor) scanWindows(ctx context.Context) ([]*Patch, []*Patch, error) {
	ps := `$Session = New-Object -ComObject Microsoft.Update.Session
$Searcher = $Session.CreateUpdateSearcher()
$SearchResult = $Searcher.Search("IsInstalled=0 AND IsHidden=0")
$Updates = @()
foreach ($Update in $SearchResult.Updates) {
    $kb = @()
    if ($Update.KBArticleIDs) { foreach ($id in $Update.KBArticleIDs) { $kb += $id } }
    $updates += @{
        Title = if ($Update.Title) { $Update.Title } else { "Unknown" }
        KBArticleIDs = $kb
        MsrcSeverity = if ($Update.MsrcSeverity) { $Update.MsrcSeverity } else { "unknown" }
        Description = if ($Update.Description) { $Update.Description } else { "" }
    }
}
$Updates | ConvertTo-Json -Compress`

	output, err := e.runPowerShell(ctx, ps)
	if err != nil {
		return nil, nil, fmt.Errorf("scan patches: %w", err)
	}

	var rawPatches []windowsPatch
	if err := json.Unmarshal([]byte(output), &rawPatches); err != nil {
		if len(rawPatches) > 0 {
			return nil, nil, fmt.Errorf("scan patches: %w (partial data available)", err)
		}
		return nil, nil, fmt.Errorf("scan patches: decode JSON: %w", err)
	}

	missing := make([]*Patch, 0, len(rawPatches))
	for _, wp := range rawPatches {
		p := &Patch{
			Title:       wp.Title,
			Platform:    PlatformWindows,
			Severity:    severityFromMSRC(wp.MsrcSeverity),
			Description: wp.Description,
		}
		if len(wp.KBArticleIDs) > 0 {
			p.KB = wp.KBArticleIDs[0]
		}
		missing = append(missing, p)
	}

	return nil, missing, nil
}

func severityFromMSRC(s string) PatchSeverity {
	switch strings.ToLower(s) {
	case "critical":
		return SeverityCritical
	case "important":
		return SeverityImportant
	case "moderate":
		return SeverityModerate
	case "low":
		return SeverityLow
	default:
		return SeverityModerate
	}
}

func (e *Executor) installWindows(ctx context.Context, patchIDs []string) (*ExecResult, error) {
	kbFilter := strings.Join(patchIDs, ",")
	ps := fmt.Sprintf(`$Session = New-Object -ComObject Microsoft.Update.Session
$Searcher = $Session.CreateUpdateSearcher()
$SearchResult = $Searcher.Search("IsInstalled=0 AND IsHidden=0")
$Updates = @()
foreach ($Update in $SearchResult.Updates) {
    $kb = ($Update.KBArticleIDs | ForEach-Object { $_.ToString() }) -join ','
    if ($kb -match '%s') {
        $Updates += $Update
    }
}
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
	pm := detectPackageManager()
	if pm == "" {
		return nil, nil, fmt.Errorf("no package manager found")
	}

	var cmd *exec.Cmd
	var output []byte
	var err error

	switch pm {
	case "apt":
		cmd = exec.CommandContext(ctx, "apt", "list", "--upgradable")
		output, err = cmd.Output()
		missing := parseAptUpgradable(output)
		return nil, missing, err
	case "dnf":
		cmd = exec.CommandContext(ctx, "dnf", "check-update")
		output, err = cmd.Output()
		missing := parseDnfCheckUpdate(output)
		return nil, missing, err
	case "yum":
		cmd = exec.CommandContext(ctx, "yum", "check-update")
		output, err = cmd.Output()
		missing := parseYumCheckUpdate(output)
		return nil, missing, err
	case "zypper":
		cmd = exec.CommandContext(ctx, "zypper", "list-patches")
		output, err = cmd.Output()
		missing := parseZypperListPatches(output)
		return nil, missing, err
	default:
		return nil, nil, fmt.Errorf("unknown package manager: %s", pm)
	}
}

func parseAptUpgradable(output []byte) []*Patch {
	var patches []*Patch
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if !strings.Contains(line, "[upgradable from:") {
			continue
		}
		parts := strings.Split(line, "/")
		if len(parts) < 2 {
			continue
		}
		name := parts[0]
		versionPart := strings.TrimPrefix(parts[len(parts)-1], "[upgradable from:")
		versionPart = strings.TrimSpace(strings.TrimSuffix(versionPart, "]"))
		if name == "" || versionPart == "" {
			continue
		}
		patches = append(patches, &Patch{
			Title:       name + " (" + versionPart + ")",
			Platform:    PlatformLinux,
			KB:          name,
			Severity:    SeverityModerate,
			Description: "Package upgrade available for " + name,
		})
	}
	return patches
}

func parseDnfCheckUpdate(output []byte) []*Patch {
	var patches []*Patch
	lines := strings.Split(string(output), "\n")
	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" || strings.HasSuffix(line, "-") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			name := fields[0]
			version := fields[1]
			patches = append(patches, &Patch{
				Title:       name + "-" + version,
				Platform:    PlatformLinux,
				KB:          name,
				Severity:    SeverityModerate,
				Description: "Package upgrade available for " + name,
			})
		}
	}
	return patches
}

func parseYumCheckUpdate(output []byte) []*Patch {
	var patches []*Patch
	lines := strings.Split(string(output), "\n")
	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" || strings.HasSuffix(line, "-") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			name := fields[0]
			version := fields[1]
			patches = append(patches, &Patch{
				Title:       name + "-" + version,
				Platform:    PlatformLinux,
				KB:          name,
				Severity:    SeverityModerate,
				Description: "Package upgrade available for " + name,
			})
		}
	}
	return patches
}

func parseZypperListPatches(output []byte) []*Patch {
	var patches []*Patch
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) >= 4 && fields[0] == "i" || len(fields) >= 4 && fields[0] == "I" {
			name := fields[2]
			version := fields[3]
			patches = append(patches, &Patch{
				Title:       name + "-" + version,
				Platform:    PlatformLinux,
				KB:          name,
				Severity:    SeverityModerate,
				Description: "Patch available for " + name,
			})
		}
	}
	return patches
}

func (e *Executor) installLinux(ctx context.Context, packages []string) (*ExecResult, error) {
	pm := detectPackageManager()
	if pm == "" {
		return nil, fmt.Errorf("unknown package manager")
	}

	var cmd *exec.Cmd
	switch pm {
	case "apt":
		cmd = exec.CommandContext(ctx, "apt", "install", "-y")
	case "dnf":
		cmd = exec.CommandContext(ctx, pm, "install", "-y")
	case "yum":
		cmd = exec.CommandContext(ctx, pm, "install", "-y")
	case "zypper":
		cmd = exec.CommandContext(ctx, "zypper", "--non-interactive", "install")
	default:
		return nil, fmt.Errorf("unknown package manager: %s", pm)
	}

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

func (e *Executor) runPowerShell(ctx context.Context, script string) (string, error) {
	cmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", script)
	output, err := cmd.Output()
	if err != nil {
		return string(output), fmt.Errorf("powershell error: %w", err)
	}
	return string(output), nil
}

func detectPackageManager() string {
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
