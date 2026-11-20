package kube

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"k2/internal/shell"
)

var followBase = time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)

func stampedLine(i int) string {
	at := followBase.Add(time.Duration(i) * 100 * time.Microsecond)
	return fmt.Sprintf("%s seq-%06d", at.Format(time.RFC3339Nano), i)
}

func argValue(args []string, prefix string) string {
	for _, a := range args {
		if v, ok := strings.CutPrefix(a, prefix); ok {
			return v
		}
	}
	return ""
}

func TestParseStamped(t *testing.T) {
	l, ok := ParseStamped("2026-11-20T10:00:00.000100000Z hello world", false)
	if !ok || l.Text != "hello world" || !l.At.Equal(followBase.Add(100*time.Microsecond)) {
		t.Fatalf("%+v %v", l, ok)
	}
	l, ok = ParseStamped("[pod/web-1/app] 2026-11-20T10:00:00Z", true)
	if !ok || l.Pod != "web-1" || l.Container != "app" || l.Text != "" {
		t.Fatalf("%+v %v", l, ok)
	}
	for _, bad := range []string{"Error from server (NotFound): pods not found", "", "no stamp here"} {
		if _, ok := ParseStamped(bad, false); ok {
			t.Fatalf("%q parsed", bad)
		}
	}
	if _, ok := ParseStamped("2026-11-20T10:00:00Z x", true); ok {
		t.Fatal("prefix required")
	}
}

func TestFollowArgs(t *testing.T) {
	got := strings.Join(FollowArgs("web-1", "shop", "/kc", "dev", LogOpts{Container: "app"}, 80, time.Time{}), " ")
	for _, want := range []string{"logs web-1 -n shop", "--kubeconfig /kc", "--context dev", "-c app", "--tail=80", "-f", "--timestamps"} {
		if !strings.Contains(got, want) {
			t.Fatalf("%q lacks %q", got, want)
		}
	}
	since := followBase.Add(1500 * time.Millisecond)
	got = strings.Join(FollowArgs("web-1", "shop", "/kc", "dev", LogOpts{}, 80, since), " ")
	if !strings.Contains(got, "--since-time=2026-11-20T10:00:01Z") || strings.Contains(got, "--tail") {
		t.Fatalf("resume args %q", got)
	}
}

func TestFollowHighRateReconnectsWithoutGapsOrDuplicates(t *testing.T) {
	FollowRetry = time.Millisecond
	defer func() { FollowRetry = 300 * time.Millisecond }()

	const total = 30000
	var mu sync.Mutex
	next := 0
	conns := 0
	var sinceArgs []string
	fake := &shell.Fake{Handler: func(r shell.Request) shell.Result {
		mu.Lock()
		conns++
		from := next
		limit := min(total, next+9000)
		if v := argValue(r.Args, "--since-time="); v != "" {
			sinceArgs = append(sinceArgs, v)
			at, _ := time.Parse(time.RFC3339, v)
			from = int(at.Sub(followBase) / (100 * time.Microsecond))
		} else {
			from = 0
		}
		mu.Unlock()
		for i := from; i < limit; i++ {
			r.OnLine(stampedLine(i))
			mu.Lock()
			if i+1 > next {
				next = i + 1
			}
			mu.Unlock()
		}
		if limit >= total {
			<-r.Ctx.Done()
		}
		return shell.Fail(1, "stream closed")
	}}
	ctx, cancel := context.WithCancel(context.Background())
	var got []string
	done := make(chan struct{})
	go func() {
		defer close(done)
		New(fake).Follow(ctx, NewFollower(), FollowSpec{Args: func(since time.Time) []string {
			a := []string{"logs", "web-1"}
			if !since.IsZero() {
				a = append(a, "--since-time="+since.UTC().Format(time.RFC3339))
			}
			return a
		}}, func(l LogLine) {
			mu.Lock()
			got = append(got, l.Text)
			mu.Unlock()
		}, nil)
	}()
	deadline := time.After(20 * time.Second)
	for {
		mu.Lock()
		n := len(got)
		mu.Unlock()
		if n >= total {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("stalled at %d of %d after %d connections", n, total, conns)
		case <-time.After(5 * time.Millisecond):
		}
	}
	cancel()
	<-done
	mu.Lock()
	defer mu.Unlock()
	if conns < 3 || len(sinceArgs) < 2 {
		t.Fatalf("expected reconnects, connections %d since %v", conns, sinceArgs)
	}
	if len(got) != total {
		t.Fatalf("%d lines, want %d", len(got), total)
	}
	for i, text := range got {
		if text != fmt.Sprintf("seq-%06d", i) {
			t.Fatalf("line %d is %q", i, text)
		}
	}
}

func TestFollowStopsWhenCancelled(t *testing.T) {
	var running atomic.Int32
	fake := &shell.Fake{Handler: func(r shell.Request) shell.Result {
		running.Add(1)
		defer running.Add(-1)
		r.OnLine(stampedLine(0))
		<-r.Ctx.Done()
		return shell.Fail(137, "killed")
	}}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		New(fake).Follow(ctx, NewFollower(), FollowSpec{Args: func(time.Time) []string { return []string{"logs", "x"} }}, func(LogLine) {}, nil)
	}()
	for running.Load() == 0 {
		time.Sleep(time.Millisecond)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Follow did not return after cancel")
	}
	if running.Load() != 0 {
		t.Fatal("process still running")
	}
}

func TestFollowGivesUpWhenIdleAndReportsErrors(t *testing.T) {
	FollowRetry = time.Millisecond
	defer func() { FollowRetry = 300 * time.Millisecond }()
	var runs atomic.Int32
	fake := &shell.Fake{Handler: func(r shell.Request) shell.Result {
		runs.Add(1)
		r.OnLine(`Error from server (NotFound): pods "gone" not found`)
		return shell.Fail(1, "")
	}}
	var notes []string
	done := make(chan struct{})
	go func() {
		defer close(done)
		New(fake).Follow(context.Background(), NewFollower(), FollowSpec{MaxIdle: 3, Args: func(time.Time) []string { return []string{"logs", "gone"} }}, func(LogLine) { t.Error("emitted a line") }, func(n string) { notes = append(notes, n) })
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("did not give up")
	}
	if runs.Load() != 3 || len(notes) != 3 || !strings.Contains(notes[0], "NotFound") {
		t.Fatalf("runs %d notes %v", runs.Load(), notes)
	}
}

func TestFollowerDedupesBoundaryLines(t *testing.T) {
	f := NewFollower()
	at := followBase
	a := LogLine{At: at, Text: "a"}
	if !f.accept(a, "t a") || f.accept(a, "t a") {
		t.Fatal("same line accepted twice")
	}
	if !f.accept(LogLine{At: at, Text: "b"}, "t b") {
		t.Fatal("different line with the same stamp dropped")
	}
	if f.accept(LogLine{At: at.Add(-time.Millisecond), Text: "old"}, "old") {
		t.Fatal("older line accepted")
	}
	if !f.accept(LogLine{At: at.Add(time.Millisecond), Text: "c"}, "t c") {
		t.Fatal("newer line dropped")
	}
	if got := f.Since(); !got.Equal(followBase) {
		t.Fatalf("since %v", got)
	}
}
