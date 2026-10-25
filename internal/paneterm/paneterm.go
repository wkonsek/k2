package paneterm

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/vt"
	"github.com/creack/pty"
)

var Frame = 12 * time.Millisecond

type Term struct {
	ID  string
	emu *vt.Emulator

	ptmx *os.File
	cmd  *exec.Cmd

	dirty chan struct{}
	done  chan struct{}

	mu      sync.Mutex
	w, h    int
	code    int
	err     error
	closed  bool
	stopped atomic.Bool
}

type OutputMsg struct{ T *Term }

type ExitMsg struct {
	T    *Term
	Code int
	Err  error
}

func Start(id string, cmd *exec.Cmd, w, h int) (*Term, error) {
	w, h = max(w, 2), max(h, 1)
	if cmd.Env == nil {
		cmd.Env = os.Environ()
	}
	if !hasEnv(cmd.Env, "TERM") || envIs(cmd.Env, "TERM", "") || envIs(cmd.Env, "TERM", "dumb") {
		cmd.Env = append(cmd.Env, "TERM=xterm-256color")
	}
	cmd.Env = append(cmd.Env, "COLUMNS=", "LINES=")
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: uint16(w), Rows: uint16(h)})
	if err != nil {
		return nil, err
	}
	t := &Term{
		ID: id, emu: vt.NewEmulator(w, h), ptmx: ptmx, cmd: cmd, w: w, h: h,
		dirty: make(chan struct{}, 1), done: make(chan struct{}),
	}
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		buf := make([]byte, 32*1024)
		for {
			n, err := ptmx.Read(buf)
			if n > 0 {
				t.mu.Lock()
				_, _ = t.emu.Write(buf[:n])
				t.mu.Unlock()
				select {
				case t.dirty <- struct{}{}:
				default:
				}
			}
			if err != nil {
				return
			}
		}
	}()
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := t.emu.Read(buf)
			if t.stopped.Load() || err != nil {
				return
			}
			_, _ = ptmx.Write(buf[:n])
		}
	}()
	go func() {
		werr := cmd.Wait()
		select {
		case <-readDone:
		case <-time.After(200 * time.Millisecond):
		}
		t.mu.Lock()
		t.code = cmd.ProcessState.ExitCode()
		var ee *exec.ExitError
		if werr != nil && !errors.As(werr, &ee) {
			t.err = werr
		}
		t.mu.Unlock()
		t.shutdown()
		close(t.done)
	}()
	return t, nil
}

func (t *Term) shutdown() {
	_ = t.ptmx.Close()
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return
	}
	t.closed = true
	t.stopped.Store(true)
	_, _ = io.WriteString(t.emu.InputPipe(), "")
}

func (t *Term) Next() tea.Cmd {
	return func() tea.Msg {
		select {
		case <-t.done:
			return t.exitMsg()
		case <-t.dirty:
		}

		select {
		case <-t.done:
			return t.exitMsg()
		case <-time.After(Frame):
		}
		return OutputMsg{T: t}
	}
}

func (t *Term) exitMsg() tea.Msg {
	t.mu.Lock()
	defer t.mu.Unlock()
	return ExitMsg{T: t, Code: t.code, Err: t.err}
}

func (t *Term) Exited() bool {
	select {
	case <-t.done:
		return true
	default:
		return false
	}
}

func (t *Term) Kill() {
	if t.cmd.Process != nil {
		_ = t.cmd.Process.Signal(sighup)
		go func() {
			select {
			case <-t.done:
			case <-time.After(2 * time.Second):
				_ = t.cmd.Process.Kill()
			}
		}()
	}
}

func (t *Term) Size() (w, h int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.w, t.h
}

func (t *Term) Resize(w, h int) {
	w, h = max(w, 2), max(h, 1)
	t.mu.Lock()
	if (w == t.w && h == t.h) || t.closed {
		t.mu.Unlock()
		return
	}
	t.w, t.h = w, h
	t.emu.Resize(w, h)
	t.mu.Unlock()
	_ = pty.Setsize(t.ptmx, &pty.Winsize{Cols: uint16(w), Rows: uint16(h)})
}

func (t *Term) Key(k tea.KeyMsg) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return false
	}
	if k.Paste {
		t.emu.Paste(string(k.Runes))
		return true
	}
	if ev, ok := uvKey(k); ok {
		t.emu.SendKey(ev)
		return true
	}
	s, ok := keyBytes(k)
	if !ok {
		return false
	}
	t.emu.SendText(s)
	return true
}

func (t *Term) Text(s string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.closed {
		t.emu.SendText(s)
	}
}

func (t *Term) Plain() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	w, h := t.w, t.h
	lines := make([]string, h)
	for y := range h {
		var b strings.Builder
		for x := 0; x < w; x++ {
			c := t.emu.CellAt(x, y)
			if c == nil || c.IsZero() {
				continue
			}
			b.WriteString(c.String())
		}
		lines[y] = strings.TrimRight(b.String(), " ")
	}
	return strings.Join(lines, "\n")
}

func (t *Term) View(cursor bool) string {
	t.mu.Lock()
	defer t.mu.Unlock()
	w, h := t.w, t.h
	cur := t.emu.CursorPosition()
	showCur := cursor && !t.emu.CursorHidden()
	var out strings.Builder
	line := make(uv.Line, w)
	for y := range h {
		for x := range w {
			c := t.emu.CellAt(x, y)
			if c == nil {
				line[x] = uv.EmptyCell
				continue
			}
			line[x] = *c
		}
		if showCur && y == cur.Y && cur.X >= 0 && cur.X < w {
			c := &line[cur.X]
			if c.IsZero() && cur.X > 0 {
				c = &line[cur.X-1]
			}
			if c.Content == "" {
				c.Content, c.Width = " ", 1
			}
			c.Style.Attrs ^= uv.AttrReverse
		}
		s := line.Render()
		if pad := w - ansi.StringWidth(s); pad > 0 {
			s += strings.Repeat(" ", pad)
		}
		if y > 0 {
			out.WriteByte('\n')
		}
		out.WriteString(s)
	}
	return out.String()
}

func hasEnv(env []string, k string) bool {
	for _, e := range env {
		if strings.HasPrefix(e, k+"=") {
			return true
		}
	}
	return false
}

func envIs(env []string, k, v string) bool {
	val, found := "", false
	for _, e := range env {
		if strings.HasPrefix(e, k+"=") {
			val, found = strings.TrimPrefix(e, k+"="), true
		}
	}
	return found && val == v
}
