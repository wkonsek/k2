//go:build windows

package paneterm

import "os"

var sighup = os.Kill
