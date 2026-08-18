package update

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// TriggerRestartWithSchema delegates the binary swap, restart, post-restart
// health verification, database restoration, and binary rollback to the
// external systemd finalizer. The running process never replaces its own
// executable before the durable handoff succeeds, eliminating the crash window
// between an in-process swap and finalizer launch.
func (u *OrchestratorUpdater) TriggerRestartWithSchema(stagedBinary string, sourceSchema, targetSchema int) error {
	mode := u.DetectMode()
	if mode != "baremetal" {
		return fmt.Errorf("%s deployment requires the digest-pinned promoted release workflow", mode)
	}
	if sourceSchema < 0 || targetSchema < 0 || sourceSchema > targetSchema {
		return fmt.Errorf("invalid schema restart boundary %d -> %d", sourceSchema, targetSchema)
	}
	if stagedBinary == "" {
		return fmt.Errorf("staged update binary is required")
	}
	stagedAbs, err := filepath.Abs(stagedBinary)
	if err != nil {
		return fmt.Errorf("resolve staged update binary: %w", err)
	}
	updatesDir, err := filepath.Abs(filepath.Join(u.dataDir, "updates"))
	if err != nil {
		return fmt.Errorf("resolve update staging directory: %w", err)
	}
	if filepath.Dir(stagedAbs) != updatesDir {
		return fmt.Errorf("staged update binary must be inside the protected update directory")
	}
	if info, err := os.Stat(stagedAbs); err != nil || !info.Mode().IsRegular() || info.Mode()&0111 == 0 {
		return fmt.Errorf("staged update binary is unavailable or not executable")
	}

	const finalizer = "/usr/lib/strata-rmm/finalize-orchestrator-upgrade.sh"
	if info, err := os.Stat(finalizer); err != nil || info.Mode()&0111 == 0 {
		return fmt.Errorf("upgrade finalizer is unavailable or not executable: %s", finalizer)
	}

	unit := fmt.Sprintf("strata-rmm-upgrade-finalize-%d", os.Getpid())
	cmd := exec.Command(
		"systemd-run",
		"--unit="+unit,
		"--collect",
		"--property=Type=oneshot",
		finalizer,
		u.currentExe,
		stagedAbs,
		strconv.Itoa(sourceSchema),
		strconv.Itoa(targetSchema),
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("launch external upgrade finalizer: %w: %s", err, strings.TrimSpace(string(output)))
	}

	os.Exit(0)
	return nil
}
