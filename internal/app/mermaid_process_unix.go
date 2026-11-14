//go:build linux || darwin

package app

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

// mmdc may launch Chromium, so timeout and exit also stop its child processes.
func configureMermaidCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
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
