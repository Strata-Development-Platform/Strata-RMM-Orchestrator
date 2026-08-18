package update

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// TriggerRestartWithSchema delegates restart, post-restart health verification,
// database schema rollback, and binary rollback to the external systemd
// finalizer. sourceSchema is the live schema captured before staging and
// targetSchema is the signed candidate schema. Neither value is secret.
func (u *OrchestratorUpdater) TriggerRestartWithSchema(sourceSchema, targetSchema int) error {
	mode := u.DetectMode()
	if mode != "baremetal" {
		return fmt.Errorf("%s deployment requires the digest-pinned promoted release workflow", mode)
	}
	if sourceSchema < 0 || targetSchema < 0 || sourceSchema > targetSchema {
		return fmt.Errorf("invalid schema restart boundary %d -> %d", sourceSchema, targetSchema)
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
