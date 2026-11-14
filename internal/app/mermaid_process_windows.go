//go:build windows

package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

func configureMermaidCommand(cmd *exec.Cmd) {
	// npm installs mmdc.cmd on Windows. Invoke its JavaScript entry with Node
	// directly so generated paths are passed as arguments without shell parsing.
	extension := strings.ToLower(filepath.Ext(cmd.Path))
	if extension == ".cmd" || extension == ".bat" {
		if node, err := exec.LookPath("node"); err == nil {
			dir := filepath.Dir(cmd.Path)
			for _, entry := range []string{
				filepath.Join(dir, "node_modules", "@mermaid-js", "mermaid-cli", "src", "cli.js"),
				filepath.Join(dir, "..", "@mermaid-js", "mermaid-cli", "src", "cli.js"),
			} {
				if info, err := os.Stat(entry); err == nil && info.Mode().IsRegular() {
					cmd.Path = node
					cmd.Args = append([]string{node, entry}, cmd.Args[1:]...)
					break
				}
			}
		}
	}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return os.ErrProcessDone
		}
		kill := exec.Command("taskkill", "/PID", strconv.Itoa(cmd.Process.Pid), "/T", "/F")
		kill.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		if err := kill.Run(); err != nil {
			return cmd.Process.Kill()
		}
		return nil
	}
}
