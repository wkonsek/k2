//go:build !windows

package paneterm

import "syscall"

var sighup = syscall.SIGHUP
