//go:build windows

package software

import (
	"os"
	"os/exec"
	"strconv"
	"time"
)

func configureSoftwareProcessTree(cmd *exec.Cmd) {
	// MSI/EXE/PowerShell installers can create child processes. Kill the full
	// Windows process tree when the command context is cancelled, with a direct
	// process kill fallback if taskkill itself is unavailable or fails.
	cmd.WaitDelay = 2 * time.Second
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return os.ErrProcessDone
		}
		killTree := exec.Command("taskkill.exe", "/PID", strconv.Itoa(cmd.Process.Pid), "/T", "/F")
		if err := killTree.Run(); err == nil {
			return nil
		}
		if err := cmd.Process.Kill(); err != nil {
			return err
		}
		return nil
	}
}
