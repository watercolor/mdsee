//go:build !linux && !darwin && !windows

package app

import "os/exec"

func configureMermaidCommand(cmd *exec.Cmd) {}
