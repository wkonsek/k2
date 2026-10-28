package shell

import (
	"bytes"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/creack/pty"
)

type Request struct {
	Name string

	Args []string

	Env []string

	Stdin string

	Combined bool

	// TTY gives the command a terminal for stdout. Diff tools such as dyff
	// only colorize when stdout is a terminal.
	TTY bool

	OnLine func(line string)
}

type Result struct {
	Stdout []byte

	Stderr []byte

	Code int

	Err error
}

type Runner interface {
	Run(Request) Result

	LookPath(name string) (string, error)
}

type Exec struct{}

func (Exec) Run(req Request) Result {
	cmd := exec.Command(req.Name, req.Args...)
	if len(req.Env) > 0 || req.TTY {
		cmd.Env = append(os.Environ(), req.Env...)
		if req.TTY {
			cmd.Env = append(cmd.Env, "TERM=xterm-256color")
		}
	}
	if req.Stdin != "" {
		cmd.Stdin = strings.NewReader(req.Stdin)
	}
	if req.TTY {
		if res, ok := runTTY(cmd, req); ok {
			return res
		}
	}
	var stdout, stderr bytes.Buffer
	var out io.Writer = &stdout
	lines := &lineWriter{fn: req.OnLine}
	if req.OnLine != nil {
		out = io.MultiWriter(&stdout, lines)
	}
	cmd.Stdout = out
	errLines := &lineWriter{fn: req.OnLine}
	switch {
	case req.Combined:
		cmd.Stderr = out
	case req.OnLine != nil:

		var mu sync.Mutex
		lines.mu, errLines.mu = &mu, &mu
		cmd.Stderr = io.MultiWriter(&stderr, errLines)
	default:
		cmd.Stderr = &stderr
	}
	err := cmd.Run()
	lines.flush()
	errLines.flush()
	res := Result{Stdout: stdout.Bytes(), Stderr: stderr.Bytes(), Err: err}
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			res.Code = ee.ExitCode()
		} else {
			res.Code = -1
		}
	}
	return res
}

func runTTY(cmd *exec.Cmd, req Request) (Result, bool) {
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: 240, Rows: 60})
	if err != nil {
		return Result{}, false
	}
	defer ptmx.Close()
	var buf bytes.Buffer
	out := io.Writer(&buf)
	var lines *lineWriter
	if req.OnLine != nil {
		lines = &lineWriter{fn: req.OnLine}
		out = io.MultiWriter(&buf, lines)
	}
	_, _ = io.Copy(out, ptmx)
	if lines != nil {
		lines.flush()
	}
	err = cmd.Wait()
	res := Result{Stdout: buf.Bytes(), Err: err}
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			res.Code = ee.ExitCode()
		} else {
			res.Code = -1
		}
	}
	return res, true
}

type lineWriter struct {
	fn   func(string)
	part []byte
	mu   *sync.Mutex
}

func (w *lineWriter) Write(b []byte) (int, error) {
	if w.mu != nil {
		w.mu.Lock()
		defer w.mu.Unlock()
	}
	w.part = append(w.part, b...)
	for {
		i := bytes.IndexByte(w.part, '\n')
		if i < 0 {
			return len(b), nil
		}
		w.fn(strings.TrimRight(string(w.part[:i]), "\r"))
		w.part = w.part[i+1:]
	}
}

func (w *lineWriter) flush() {
	if w.fn != nil && len(w.part) > 0 {
		w.fn(string(w.part))
		w.part = nil
	}
}

func (Exec) LookPath(name string) (string, error) { return exec.LookPath(name) }

func Output(r Runner, name string, args ...string) ([]byte, error) {
	res := r.Run(Request{Name: name, Args: args})
	return res.Stdout, res.Err
}

func Combined(r Runner, name string, args ...string) ([]byte, error) {
	res := r.Run(Request{Name: name, Args: args, Combined: true})
	return res.Stdout, res.Err
}

func OneLine(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if s == "" {
		return "request failed"
	}
	r := []rune(s)
	if len(r) > 140 {
		return string(r[:139]) + "…"
	}
	return s
}

// FailText is the full command output for an error. The status line shortens
// it later; the last-error popup and the error log keep this text.
func FailText(out []byte, err error) string {
	text := strings.TrimSpace(string(out))
	if text == "" && err != nil {
		text = strings.TrimSpace(err.Error())
	}
	if text == "" {
		return "request failed"
	}
	const max = 256 * 1024
	if len(text) > max {
		return text[:max] + "\n… truncated"
	}
	return text
}
