package shell

import (
	"errors"
	"fmt"
	"strings"
	"sync"
)

type Fake struct {
	Handler func(Request) Result

	Missing []string

	mu    sync.Mutex
	calls []Request
}

func (f *Fake) Run(req Request) Result {
	f.mu.Lock()
	f.calls = append(f.calls, req)
	f.mu.Unlock()
	res := Fail(1, "fake: no handler for "+req.Name)
	if f.Handler != nil {
		res = f.Handler(req)
	}
	if req.OnLine != nil {

		for _, out := range [][]byte{res.Stdout, res.Stderr} {
			if len(out) == 0 {
				continue
			}
			for _, line := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
				req.OnLine(line)
			}
		}
	}
	return res
}

func (f *Fake) LookPath(name string) (string, error) {
	for _, m := range f.Missing {
		if m == name {
			return "", errors.New("fake: " + name + " not found")
		}
	}
	return "/usr/bin/" + name, nil
}

func (f *Fake) Calls() []Request {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Request(nil), f.calls...)
}

func (r Request) Line() string {
	return strings.TrimSpace(r.Name + " " + strings.Join(r.Args, " "))
}

func OK(stdout string) Result { return Result{Stdout: []byte(stdout)} }

func Fail(code int, output string) Result {
	return Result{Stdout: []byte(output), Code: code, Err: fmt.Errorf("exit status %d", code)}
}
