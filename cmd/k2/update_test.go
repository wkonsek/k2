package main

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func testUpdater(t *testing.T, url string) (updater, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	var out, errb bytes.Buffer
	return updater{
		url:    url,
		client: &http.Client{Timeout: 5 * time.Second},
		bash:   func() (string, error) { return exec.LookPath("bash") },
		stdin:  strings.NewReader(""),
		stdout: &out,
		stderr: &errb,
	}, &out, &errb
}

func scriptServer(t *testing.T, body string, status int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func needBash(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("no bash")
	}
}

func TestIsUpdate(t *testing.T) {
	for _, a := range []string{"--update", "-U"} {
		if !isUpdate([]string{a}) {
			t.Errorf("%s must update", a)
		}
	}
	for _, args := range [][]string{nil, {"-u"}, {"update"}, {"--fresh", "--update"}, {"--updates"}, {"--version"}} {
		if isUpdate(args) {
			t.Errorf("%v must not update", args)
		}
	}
}

func TestUpdateRunsTheScript(t *testing.T) {
	needBash(t)
	t.Setenv("K2_VERSION", "v9.9.9")
	srv := scriptServer(t, "echo \"installing $K2_VERSION\"\n", http.StatusOK)
	u, out, errb := testUpdater(t, srv.URL)
	if code := u.run([]string{"-U"}); code != 0 {
		t.Fatalf("exit %d: %s", code, errb)
	}
	if out.String() != "installing v9.9.9\n" {
		t.Fatalf("stdout %q", out)
	}
}

func TestUpdateKeepsStdin(t *testing.T) {
	needBash(t)
	srv := scriptServer(t, "read -r answer\necho \"got $answer\"\n", http.StatusOK)
	u, out, _ := testUpdater(t, srv.URL)
	u.stdin = strings.NewReader("yes\n")
	if code := u.run([]string{"--update"}); code != 0 || out.String() != "got yes\n" {
		t.Fatalf("exit %d stdout %q", code, out)
	}
}

func TestUpdatePropagatesExitCode(t *testing.T) {
	needBash(t)
	srv := scriptServer(t, "echo boom >&2\nexit 7\n", http.StatusOK)
	u, _, errb := testUpdater(t, srv.URL)
	if code := u.run([]string{"-U"}); code != 7 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(errb.String(), "boom") {
		t.Fatalf("stderr %q", errb)
	}
}

func TestUpdateRemovesTheTempScript(t *testing.T) {
	needBash(t)
	srv := scriptServer(t, "echo \"$0\"\n", http.StatusOK)
	u, out, _ := testUpdater(t, srv.URL)
	if code := u.run([]string{"-U"}); code != 0 {
		t.Fatalf("exit %d", code)
	}
	path := strings.TrimSpace(out.String())
	if !strings.Contains(path, "k2-install-") {
		t.Fatalf("script path %q", path)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("temp script left behind: %v", err)
	}
}

func TestUpdateReportsDownloadErrors(t *testing.T) {
	needBash(t)
	srv := scriptServer(t, "not found", http.StatusNotFound)
	u, out, errb := testUpdater(t, srv.URL)
	if code := u.run([]string{"-U"}); code != 1 {
		t.Fatalf("exit %d", code)
	}
	if out.Len() != 0 || !strings.Contains(errb.String(), "cannot download") || !strings.Contains(errb.String(), "404") {
		t.Fatalf("stdout %q stderr %q", out, errb)
	}
	u, _, errb = testUpdater(t, "http://127.0.0.1:1/install.sh")
	if code := u.run([]string{"-U"}); code != 1 || !strings.Contains(errb.String(), "cannot download") {
		t.Fatalf("unreachable: exit %d stderr %q", code, errb)
	}
}

func TestUpdateNeedsBash(t *testing.T) {
	srv := scriptServer(t, "echo hi\n", http.StatusOK)
	u, out, errb := testUpdater(t, srv.URL)
	u.bash = func() (string, error) { return "", errors.New("not found") }
	if code := u.run([]string{"-U"}); code != 1 {
		t.Fatalf("exit %d", code)
	}
	if out.Len() != 0 || !strings.Contains(errb.String(), "bash is required") {
		t.Fatalf("stdout %q stderr %q", out, errb)
	}
}

func TestUpdateRejectsExtraArguments(t *testing.T) {
	u, _, errb := testUpdater(t, "http://127.0.0.1:1/never")
	if code := u.run([]string{"--update", "now"}); code != 2 || !strings.Contains(errb.String(), "takes no arguments") {
		t.Fatalf("exit %d stderr %q", code, errb)
	}
}

func TestHelpListsUpdate(t *testing.T) {
	out, _ := cliAction([]string{"--help"}, "v")
	if !strings.Contains(out, "k2 --update") || !strings.Contains(out, "-U") || !strings.Contains(out, "K2_VERSION") {
		t.Fatalf("help lacks the update flag:\n%s", out)
	}
}

func TestUpdateIsNotAUIAction(t *testing.T) {
	if out, exit := cliAction([]string{"--update"}, "v"); exit || out != "" {
		t.Fatalf("cliAction must leave --update to main: %q %v", out, exit)
	}
}
