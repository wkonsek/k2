package ui

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
)

type launches struct {
	dirs  []string
	argvs [][]string
	err   error
}

func (l *launches) fake(dir string, argv []string) error {
	l.dirs = append(l.dirs, dir)
	l.argvs = append(l.argvs, argv)
	return l.err
}

func TestOpenFolderInKustomize(t *testing.T) {
	root := kustTree(t)
	l := &launches{}
	m := browseModel(t, root)
	m.launch = l.fake
	selectTitle(t, &m, "deploy.yaml")
	m = press(t, m, ".")
	if !reflect.DeepEqual(l.dirs, []string{root}) || !reflect.DeepEqual(l.argvs, [][]string{{"code", root}}) {
		t.Fatalf("launched %v %v", l.dirs, l.argvs)
	}
	if m.stage != "kdir" || m.toast.text != "opened in code" || m.toast.tone != toneOK {
		t.Fatalf("stage %q toast %+v", m.stage, m.toast)
	}

	base := filepath.Join(root, "base")
	_ = m.showKList(root, []string{root, base})
	m.content.Select(1)
	m = press(t, m, ".")
	if len(l.dirs) != 2 || l.dirs[1] != base || l.argvs[1][1] != base {
		t.Fatalf("kust list launched %v", l.dirs)
	}

	m.cfg.Editor.OpenFolder = "code-insiders --new-window"
	m = press(t, m, ".")
	if got := l.argvs[2]; !reflect.DeepEqual(got, []string{"code-insiders", "--new-window", base}) || m.toast.text != "opened in code-insiders" {
		t.Fatalf("argv %v toast %q", got, m.toast.text)
	}
}

func TestOpenFolderNotFound(t *testing.T) {
	root := kustTree(t)
	l := &launches{err: &exec.Error{Name: "code", Err: exec.ErrNotFound}}
	m := browseModel(t, root)
	m.launch = l.fake
	m = press(t, m, ".")
	if len(l.dirs) != 1 || m.toast.tone != toneErr || !strings.HasPrefix(m.toast.text, "code not found in PATH") {
		t.Fatalf("toast %+v", m.toast)
	}

	if err := startDetached(root, []string{"k2-no-such-editor", root}); err == nil || !strings.Contains(err.Error(), "executable file not found") {
		t.Fatalf("missing binary: %v", err)
	}
}

func TestOpenFolderStartsDetached(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("needs sh")
	}
	root := kustTree(t)
	out := filepath.Join(t.TempDir(), "pwd")
	if err := startDetached(root, []string{"sh", "-c", "pwd > " + out}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if b, err := os.ReadFile(out); err == nil && len(b) > 0 {
			got, _ := filepath.EvalSymlinks(strings.TrimSpace(string(b)))
			want, _ := filepath.EvalSymlinks(root)
			if got != want {
				t.Fatalf("ran in %q, want %q", got, want)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("command did not run")
}

func TestOpenFolderOnlyInKustomize(t *testing.T) {
	l := &launches{}
	m := newTestModel(t, testEnv(t), nil)
	m.launch = l.fake
	m = press(t, m, ".")
	m.kubeconfig, m.context = "/kc", "kind-dev"
	m = press(t, m, "f", ".")
	if len(l.dirs) != 0 {
		t.Fatalf("launched outside kustomize: %v", l.dirs)
	}
}

func TestOpenFolderRebindAndHint(t *testing.T) {
	trueColor(t)
	root := kustTree(t)
	l := &launches{}
	m := browseModel(t, root)
	m.launch = l.fake
	m.width, m.height = 200, 40
	if !strings.Contains(ansi.Strip(m.View()), ". code") {
		t.Fatalf("hint bar lacks the key:\n%s", ansi.Strip(m.View()))
	}
	m.cfg.Keys.OpenFolder = []string{"O"}
	m = press(t, m, ".")
	if len(l.dirs) != 0 {
		t.Fatal(". still opens after rebinding")
	}
	m = press(t, m, "O")
	if len(l.dirs) != 1 || !strings.Contains(ansi.Strip(m.View()), "O code") {
		t.Fatalf("O launched %v", l.dirs)
	}
}
