package paneterm

import (
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func BenchmarkView(b *testing.B) {
	t, err := Start("b", exec.Command("sleep", "30"), 200, 60)
	if err != nil {
		b.Fatal(err)
	}
	defer t.Kill()
	var s strings.Builder
	for y := 0; y < 60; y++ {
		fmt.Fprintf(&s, "\x1b[%d;1H\x1b[38;5;%dm%s\x1b[1;7m%s\x1b[m", y+1, y%255, strings.Repeat("x", 120), strings.Repeat("y", 80))
	}
	t.mu.Lock()
	t.emu.Write([]byte(s.String()))
	t.mu.Unlock()
	time.Sleep(10 * time.Millisecond)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = t.View(true)
	}
}
