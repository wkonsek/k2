package kube

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"k2/internal/shell"
)

func livePIDs(path string) (all, live []int) {
	b, _ := os.ReadFile(path)
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		if line == "" {
			continue
		}
		n, err := strconv.Atoi(line)
		if err != nil || n <= 0 {
			continue
		}
		all = append(all, n)
		if syscall.Kill(n, 0) == nil {
			live = append(live, n)
		}
	}
	return all, live
}

func TestFollowCancelKillsKubectlAndChildren(t *testing.T) {
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "pids")
	stub := filepath.Join(dir, "kubectl")
	script := fmt.Sprintf("#!/bin/sh\necho $$ >> %q\nif printf '%%s' \"$*\" | grep -q -- '-f'; then\n  sleep 600 &\n  child=$!\n  echo $child >> %q\n  wait $child\n  exit 0\nfi\nexit 0\n", pidFile, pidFile)
	if err := os.WriteFile(stub, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	c := New(shell.Exec{})
	const n = 12
	for i := 0; i < n; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		fol := NewFollower()
		done := make(chan struct{})
		go func() {
			c.Follow(ctx, fol, FollowSpec{Args: func(time.Time) []string {
				return []string{"logs", "app-0", "-n", "shop", "-c", "server", "-f", "--timestamps"}
			}}, func(LogLine) {}, nil)
			close(done)
		}()
		deadline := time.Now().Add(2 * time.Second)
		for {
			all, live := livePIDs(pidFile)
			if len(live) >= 2 || time.Now().After(deadline) {
				if len(all) < 2*(i+1) {
					t.Fatalf("iteration %d: stub did not start, recorded %v", i, all)
				}
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		cancel()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Fatalf("follow did not return after cancel, iteration %d", i)
		}
	}
	time.Sleep(200 * time.Millisecond)
	all, live := livePIDs(pidFile)
	if len(live) != 0 {
		t.Fatalf("after %d cycles recorded %d leftover %d pids=%v live=%v", n, len(all), len(live), all, live)
	}
	if len(all) < 2*n {
		t.Fatalf("expected at least %d recorded pids, got %d", 2*n, len(all))
	}
}
