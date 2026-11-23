//go:build !unix

package shell

import "os/exec"

func armProcessGroup(cmd *exec.Cmd) {}
