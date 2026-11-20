package kube

import (
	"strings"

	"k2/internal/shell"
)

type Failure int

const (
	FailNone Failure = iota
	FailAuth
	FailConn
	FailOther
)

var authMarkers = []string{
	"unauthorized",
	"you must be logged in",
	"invalid bearer",
	"token has expired",
	"token expired",
	"token is expired",
	"provide credentials",
	"getting credentials",
	"exec plugin",
	"failed to refresh token",
	"expired certificate",
	"certificate has expired",
	"(401)",
	"401 unauthorized",
}

var connMarkers = []string{
	"unable to connect to the server",
	"connection refused",
	"no such host",
	"i/o timeout",
	"context deadline exceeded",
	"tls handshake timeout",
	"no route to host",
	"network is unreachable",
	"connection reset",
	"dial tcp",
	"was refused",
	"client.timeout",
	"unexpected eof",
}

func ClassifyFailure(text string) Failure {
	s := strings.ToLower(text)
	for _, m := range authMarkers {
		if strings.Contains(s, m) {
			return FailAuth
		}
	}
	for _, m := range connMarkers {
		if strings.Contains(s, m) {
			return FailConn
		}
	}
	return FailOther
}

func FailureReason(f Failure, text string) string {
	s := strings.ToLower(text)
	has := func(subs ...string) bool {
		for _, sub := range subs {
			if strings.Contains(s, sub) {
				return true
			}
		}
		return false
	}
	switch f {
	case FailAuth:
		if has("getting credentials", "exec plugin", "failed to refresh token") {
			return "credentials failed"
		}
		if has("expired certificate", "certificate has expired") {
			return "certificate expired"
		}
		return "token expired or rejected"
	case FailConn:
		switch {
		case has("connection refused", "was refused"):
			return "connection refused"
		case has("no such host"):
			return "no such host"
		case has("no route to host"):
			return "no route to host"
		case has("network is unreachable"):
			return "network unreachable"
		case has("i/o timeout", "deadline exceeded", "handshake timeout", "client.timeout"):
			return "timed out"
		case has("connection reset", "unexpected eof"):
			return "connection reset"
		}
	}
	return shortFailure(text)
}

func shortFailure(text string) string {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		line = strings.TrimPrefix(line, "error: ")
		line = strings.TrimPrefix(line, "Unable to connect to the server: ")
		return shell.OneLine(line)
	}
	return "request failed"
}
