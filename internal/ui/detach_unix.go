//go:build !windows

package ui

import (
	"os/exec"
	"syscall"
)

func detach(c *exec.Cmd) {
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}
