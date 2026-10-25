package paneterm

import (
	"os/exec"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func waitExit(t *testing.T, term *Term) ExitMsg {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		ch := make(chan tea.Msg, 1)
		go func() { ch <- term.Next()() }()
		select {
		case msg := <-ch:
			if e, ok := msg.(ExitMsg); ok {
				return e
			}
		case <-deadline:
			t.Fatal("no exit")
		}
	}
}

func TestExitCodeAndLastScreen(t *testing.T) {
	term, err := Start("p1", exec.Command("sh", "-c", `printf 'error: bad yaml\r\n'; exit 3`), 40, 5)
	if err != nil {
		t.Fatal(err)
	}
	e := waitExit(t, term)
	if e.T != term || e.Code != 3 || e.Err != nil || !term.Exited() {
		t.Fatalf("exit %+v", e)
	}
	if !strings.Contains(term.Plain(), "error: bad yaml") {
		t.Fatalf("screen %q", term.Plain())
	}
	if lines := strings.Split(term.View(false), "\n"); len(lines) != 5 {
		t.Fatalf("view has %d lines", len(lines))
	}
}

func TestResizeReachesChild(t *testing.T) {
	term, err := Start("p1", exec.Command("sh", "-c", `read x; stty size; sleep 0.2`), 30, 6)
	if err != nil {
		t.Fatal(err)
	}
	term.Resize(50, 9)
	if w, h := term.Size(); w != 50 || h != 9 {
		t.Fatalf("size %dx%d", w, h)
	}
	term.Key(tea.KeyMsg{Type: tea.KeyEnter})
	waitExit(t, term)
	if !strings.Contains(term.Plain(), "9 50") {
		t.Fatalf("screen %q", term.Plain())
	}
}

func TestKill(t *testing.T) {
	term, err := Start("p1", exec.Command("sleep", "30"), 20, 3)
	if err != nil {
		t.Fatal(err)
	}
	term.Kill()
	waitExit(t, term)
}

func TestCtrlBAndCtrlFPassThrough(t *testing.T) {
	term, err := Start("p1", exec.Command("sh", "-c", `stty raw -echo; v=$(dd bs=1 count=2 2>/dev/null | od -An -tx1); printf 'got%s\r\n' "$v"; sleep 1`), 40, 5)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	if !term.Key(tea.KeyMsg{Type: tea.KeyCtrlB}) || !term.Key(tea.KeyMsg{Type: tea.KeyCtrlF}) {
		t.Fatal("keys not encoded")
	}
	go func() {
		for !term.Exited() {
			term.Next()()
		}
	}()
	deadline := time.Now().Add(3 * time.Second)
	for !strings.Contains(term.Plain(), "got 02 06") && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if !strings.Contains(term.Plain(), "got 02 06") {
		t.Fatalf("screen %q", term.Plain())
	}
}
