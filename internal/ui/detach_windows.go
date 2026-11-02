//go:build windows

package ui

import "os/exec"

func detach(c *exec.Cmd) {}
