package shell

import (
	"strings"
	"sync"
	"testing"
	"time"
)

func TestExecSeparatesAndCombinesStreams(t *testing.T) {
	r := Exec{}
	res := r.Run(Request{Name: "sh", Args: []string{"-c", "echo out; echo err >&2; exit 3"}})
	if string(res.Stdout) != "out\n" || string(res.Stderr) != "err\n" || res.Code != 3 || res.Err == nil {
		t.Fatalf("got %+v", res)
	}
	res = r.Run(Request{Name: "sh", Args: []string{"-c", "echo out; echo err >&2"}, Combined: true})
	if string(res.Stdout) != "out\nerr\n" || res.Code != 0 || res.Err != nil {
		t.Fatalf("combined %+v", res)
	}
	res = r.Run(Request{Name: "sh", Args: []string{"-c", "cat; echo $K2_X"}, Stdin: "in\n", Env: []string{"K2_X=1"}})
	if string(res.Stdout) != "in\n1\n" {
		t.Fatalf("stdin/env %q", res.Stdout)
	}
	res = r.Run(Request{Name: "k2-no-such-binary"})
	if res.Code != -1 || res.Err == nil {
		t.Fatalf("missing binary %+v", res)
	}
}

func TestFakeRecordsCalls(t *testing.T) {
	f := &Fake{Handler: func(r Request) Result { return OK(r.Line()) }, Missing: []string{"oc"}}
	out, err := Output(f, "kubectl", "get", "pods")
	if err != nil || string(out) != "kubectl get pods" {
		t.Fatalf("%q %v", out, err)
	}
	if _, err := f.LookPath("oc"); err == nil {
		t.Fatal("oc should be missing")
	}
	if len(f.Calls()) != 1 || f.Calls()[0].Line() != "kubectl get pods" {
		t.Fatal("calls not recorded")
	}
	if res := (&Fake{}).Run(Request{Name: "x"}); res.Err == nil {
		t.Fatal("nil handler should fail")
	}
}

func TestOneLine(t *testing.T) {
	if OneLine("  a\n\tb  ") != "a b" || OneLine("") != "request failed" {
		t.Fatal("OneLine basics")
	}
	long := OneLine(strings.Repeat("x", 300))
	if len([]rune(long)) != 140 || !strings.HasSuffix(long, "…") {
		t.Fatalf("len %d", len([]rune(long)))
	}
	full := FailText([]byte("Error from server (Forbidden): secrets is forbidden: User \"dev\" cannot list secrets in the namespace"), nil)
	if !strings.HasSuffix(full, "namespace") || strings.Contains(full, "…") {
		t.Fatalf("FailText cut the response: %q", full)
	}
}

func TestExecOnLine(t *testing.T) {
	var got []string
	res := Exec{}.Run(Request{Name: "sh", Args: []string{"-c", "printf 'a\\nb\\r\\nc'"}, OnLine: func(l string) { got = append(got, l) }})
	if res.Err != nil || strings.Join(got, "|") != "a|b|c" || string(res.Stdout) != "a\nb\r\nc" {
		t.Fatalf("lines %q stdout %q err %v", got, res.Stdout, res.Err)
	}
}

func TestFakeReplaysLines(t *testing.T) {
	var got []string
	f := &Fake{Handler: func(Request) Result { return OK("x\ny\n") }}
	f.Run(Request{Name: "kubectl", OnLine: func(l string) { got = append(got, l) }})
	if strings.Join(got, "|") != "x|y" {
		t.Fatalf("lines %q", got)
	}
}

func TestExecOnLineSeesStderr(t *testing.T) {
	var mu sync.Mutex
	var got []string
	var firstAt time.Time
	start := time.Now()
	res := Exec{}.Run(Request{Name: "sh", Args: []string{"-c", "echo out1; sleep 0.1; echo err1 >&2; sleep 0.3; printf err2 >&2"},
		OnLine: func(l string) {
			mu.Lock()
			defer mu.Unlock()
			if firstAt.IsZero() {
				firstAt = time.Now()
			}
			got = append(got, l)
		}})
	if res.Err != nil || strings.Join(got, "|") != "out1|err1|err2" {
		t.Fatalf("lines %q err %v", got, res.Err)
	}
	if string(res.Stdout) != "out1\n" || string(res.Stderr) != "err1\nerr2" {
		t.Fatalf("stdout %q stderr %q", res.Stdout, res.Stderr)
	}
	if firstAt.Sub(start) > 300*time.Millisecond {
		t.Fatalf("first line came after %v: not streamed", firstAt.Sub(start))
	}
}

func TestFakeReplaysStderr(t *testing.T) {
	var got []string
	f := &Fake{Handler: func(Request) Result { return Result{Stdout: []byte("x\n"), Stderr: []byte("e\n")} }}
	f.Run(Request{Name: "kubectl", OnLine: func(l string) { got = append(got, l) }})
	if strings.Join(got, "|") != "x|e" {
		t.Fatalf("lines %q", got)
	}
}
