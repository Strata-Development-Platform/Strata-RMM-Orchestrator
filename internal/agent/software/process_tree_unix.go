//go:build !windows

package software

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
	"time"
)

func configureSoftwareProcessTree(cmd *exec.Cmd) {
	// Package installers and scripts commonly spawn children. CommandContext's
	// default cancellation kills only the direct process, which can leave child
	// processes running and inherited stdout/stderr pipes open indefinitely.
	// Give every software command its own process group and cancel the full group.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.WaitDelay = 2 * time.Second
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return os.ErrProcessDone
		}
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
}
