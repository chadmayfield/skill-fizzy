//go:build unix

package config

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

// ownGroup starts cmd in a new process group and makes its cancel kill the
// whole group, so the command's children die with it.
func ownGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
}
