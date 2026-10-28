package ui

import (
	"errors"
	"strings"
	"testing"

	"k2/internal/shell"

	"github.com/charmbracelet/x/ansi"
)

const (
	kWarn = "# Warning: 'vars' is deprecated. Please use 'replacements' instead. [EXPERIMENTAL] Run 'kustomize edit fix' to update your Kustomization automatically."
	kErr  = `# error: accumulating components: accumulateDirectory: "recursed accumulation of path '/home/u/git/audi/kustomize-base/components/ksql': accumulating resources: accumulation err='accumulating resources from 'client': evalsymlink failure on '/home/u/git/audi/kustomize-base/components/ksql/client' : lstat /home/u/git/audi/kustomize-base/components/ksql/client: no such file or directory': must build at directory: not a valid directory THE-END"`
)

var kLong = "plain output line that is long " + strings.Repeat("filler ", 30) + "and goes on and on and on and on and on and on and on and on and on and on and on and on and on and on and on and on and on TAIL"

func failingDiff() *shell.Fake {
	return &shell.Fake{Handler: func(r shell.Request) shell.Result {
		if len(r.Args) > 0 && r.Args[0] == "diff" {
			return shell.Result{Stdout: []byte(kWarn + "\n" + kLong + "\n"), Stderr: []byte(kErr + "\n"), Code: 2, Err: errors.New("exit status 2")}
		}
		return shell.Fail(1, "?")
	}}
}

func diffResult(t *testing.T, w, h int) model {
	t.Helper()
	m := kustModel(t, failingDiff(), "diff")
	m = resize(t, m, w, h)
	m = settle(t, m, m.kustomizeCmd("diff", "/k/test"))
	if m.stage != "kout" {
		t.Fatalf("stage %q", m.stage)
	}
	return m
}

func rowsWidth(t *testing.T, view string, w int) {
	t.Helper()
	for i, l := range strings.Split(view, "\n") {
		if n := ansi.StringWidth(l); n != w {
			t.Fatalf("row %d is %d wide, want %d: %q", i, n, w, ansi.Strip(l))
		}
	}
}

func joined(view string) string {
	var b strings.Builder
	for _, l := range strings.Split(ansi.Strip(view), "\n") {
		if i := strings.LastIndex(l, "│││"); i >= 0 {
			l = l[i+len("│││"):]
		}
		l = strings.Trim(l, "│ ")
		b.WriteString(l + " ")
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

func TestOutputErrorsAlwaysWrapped(t *testing.T) {
	for _, size := range [][2]int{{80, 50}, {120, 40}, {200, 40}} {
		m := diffResult(t, size[0], size[1])
		if m.wrapping() {
			t.Fatal("wrap should be off by default")
		}
		view := m.View()
		rowsWidth(t, view, size[0])

		all := joined(view) + " " + joined(press(t, m, "g").View())
		for _, w := range []string{"THE-END", "update your Kustomization automatically.", "no such file or directory"} {
			if !strings.Contains(all, w) {
				t.Fatalf("%v: error/warning cut, %q missing:\n%s", size, w, ansi.Strip(view))
			}
		}

		if strings.Contains(all, "TAIL") {
			t.Fatalf("%v: normal line wrapped with wrap off", size)
		}

		m = press(t, m, "w", "g")
		if !m.wrapping() || !strings.Contains(joined(m.View()), "TAIL") {
			t.Fatalf("%v: w did not wrap the output:\n%s", size, ansi.Strip(m.View()))
		}
		rowsWidth(t, m.View(), size[0])
	}
}

func TestAlertRowsIndentAndFit(t *testing.T) {
	m := kustModel(t, nil, "")
	m.logText = "  error: " + strings.Repeat("word ", 40) + "END"
	m.ran = runInfo{}
	vis := m.visualLines(30)
	if len(vis) < 5 {
		t.Fatalf("not wrapped: %d rows", len(vis))
	}
	for i, v := range vis {
		p := ansi.Strip(v.text)
		if ansi.StringWidth(p) > 30 {
			t.Fatalf("row %d too wide: %q", i, p)
		}
		if i > 0 && !strings.HasPrefix(p, "  ") {
			t.Fatalf("row %d not indented: %q", i, p)
		}
	}
	if !strings.HasSuffix(ansi.Strip(vis[len(vis)-1].text), "END") {
		t.Fatal("tail lost")
	}

	m.stage = "logs"
	if len(m.visualLines(30)) != 1 {
		t.Fatal("log lines must follow the wrap toggle")
	}
	for line, want := range map[string]bool{
		"error: x": true, "Error from server (NotFound): pods \"x\" not found": true, "# Warning: y": true,
		"Warning: z": true, "✗ error: a": true, "E1023 08:01:02.123456 1 reflector.go: boom": true,
		"W1023 08:01:02.1 x": true, "deployment.apps/web created": false, "  Warning  FailedMount  pod": false,
		"plain": false,
	} {
		if isAlert(line) != want {
			t.Errorf("isAlert(%q) = %v", line, !want)
		}
	}
}

func TestFailedDiffSummary(t *testing.T) {
	trueColor(t)
	m := diffResult(t, 160, 45)
	plain := ansi.Strip(m.View())
	for _, w := range []string{"✗ diff failed — see error above", "! 1 warning", "✗ 1 error ", "── exit 2 · failed ──"} {
		if !strings.Contains(plain, w) {
			t.Fatalf("summary lacks %q:\n%s", w, plain)
		}
	}
	for _, w := range []string{"no differences", "1 errors"} {
		if strings.Contains(plain, w) {
			t.Fatalf("summary says %q:\n%s", w, plain)
		}
	}
	if colorBefore(m.View(), "✗ diff failed — see error above") != sgr(th.Danger) {
		t.Fatal("failure line not red")
	}
}

func TestFailedApplyDeleteSummary(t *testing.T) {
	for _, verb := range []string{"apply", "delete"} {
		f := &shell.Fake{Handler: func(r shell.Request) shell.Result {
			if len(r.Args) > 0 && r.Args[0] == verb {
				return shell.Result{Stderr: []byte(kErr), Code: 1, Err: errExit1}
			}
			return shell.Fail(1, "build failed")
		}}
		m := kustModel(t, f, verb)
		m = resize(t, m, 140, 45)
		m = settle(t, m, m.kustomizeCmd(verb, "/k/test"))
		plain := ansi.Strip(m.View())
		done := map[string]string{"apply": "nothing applied", "delete": "nothing deleted"}[verb]
		for _, w := range []string{"✗ " + verb + " failed — see error above", done, "✗ 1 error "} {
			if !strings.Contains(plain, w) {
				t.Fatalf("%s: lacks %q:\n%s", verb, w, plain)
			}
		}
		if strings.Contains(plain, "0 objects") || strings.Contains(joined(m.View()), "THE-END") == false {
			t.Fatalf("%s: wording or cut error:\n%s", verb, plain)
		}
		rowsWidth(t, m.View(), 140)
	}
}

func TestCopyModeInKustomizeResult(t *testing.T) {
	got := copyClip(t)
	m := diffResult(t, 100, 40)
	m = press(t, m, "y")
	if m.copy == nil {
		t.Fatal("y did not open copy mode in the kustomize result")
	}
	if !strings.Contains(joined(m.View()), "THE-END") {
		t.Fatalf("copy mode cuts the error:\n%s", ansi.Strip(m.View()))
	}

	for i, l := range strings.Split(m.View(), "\n") {
		if ansi.StringWidth(l) > 100 {
			t.Fatalf("copy row %d wider than the terminal", i)
		}
	}
	rows := m.copyRows(m.width)
	for i, r := range rows {
		if strings.Contains(r.text, "accumulating components") {
			m.copy.cur = i
			break
		}
	}
	next, cmd := m.Update(key(t, "y"))
	m = runCmd(next.(model), cmd)
	if len(*got) != 1 || (*got)[0] != kErr {
		t.Fatalf("yank %q", *got)
	}
}

func TestLastErrorPopup(t *testing.T) {
	m := actionModel(t, actionCluster())
	m = press(t, m, "!")
	if m.dlg != nil || m.status != "no error so far" {
		t.Fatalf("no error yet: %+v %q", m.dlg, m.status)
	}
	long := "Error from server (Forbidden): " + strings.Repeat("pods is forbidden: User \"dev\" cannot list resource ", 6) + "THE-END"
	cmd := m.showRows(openedMsg{kind: "pods", err: errors.New(long)})
	_ = cmd
	if len([]rune(m.status)) > 140 || strings.Contains(m.status, "THE-END") {
		t.Fatalf("status not cut, nothing to test: %q", m.status)
	}
	if !strings.HasSuffix(m.toast.text, "! details") {
		t.Fatalf("toast %q does not point at !", m.toast.text)
	}
	m = press(t, m, "!")
	if m.dlg == nil || m.dlg.act != "error" {
		t.Fatal("! did not open the popup")
	}
	view := m.View()
	rowsWidth(t, view, m.width)
	if !strings.Contains(ansi.Strip(view), "THE-END") || !strings.Contains(ansi.Strip(view), "✗ last error") {
		t.Fatalf("popup cuts the error:\n%s", ansi.Strip(view))
	}
	got := copyClip(t)
	next, c := m.Update(key(t, "y"))
	m = runCmd(next.(model), c)
	if m.dlg != nil || len(*got) != 1 || (*got)[0] != long || !strings.Contains(m.status, "copied the error") {
		t.Fatalf("y: dlg %v got %q status %q", m.dlg, *got, m.status)
	}
	m = press(t, m, "!", "esc")
	if m.dlg != nil {
		t.Fatal("esc did not close it")
	}

	if !m.globalKey("!") {
		t.Fatal("! is not global")
	}

	m = press(t, m, "/")
	m = typeText(t, m, "!")
	if m.dlg != nil {
		t.Fatal("! in the filter opened the popup")
	}
}

func TestActionAndViewerErrorsKeptWhole(t *testing.T) {
	long := "error: " + strings.Repeat("deployments.apps \"web\" is forbidden ", 6) + "THE-END"
	m := actionModel(t, actionCluster())
	next, _ := m.Update(editDoneMsg{what: "restarted deploy/web", err: errors.New(long)})
	m = next.(model)
	if m.lastErr.text != long || !strings.HasSuffix(m.toast.text, "! details") {
		t.Fatalf("lastErr %q toast %q", m.lastErr.text, m.toast.text)
	}
	m.applyView(viewMsg{id: m.logID, err: errors.New(strings.TrimPrefix(long, "error: "))})
	m = resize(t, m, 90, 30)
	if m.stage != "view" || !strings.Contains(joined(m.View()), "THE-END") {
		t.Fatalf("viewer cuts the error:\n%s", ansi.Strip(m.View()))
	}
	rowsWidth(t, m.View(), 90)

	m.stage, m.status = "nav", strings.Repeat("abc ", 40)+"STATUS-END"
	if !strings.Contains(joined(m.View()), "STATUS-END") {
		t.Fatalf("home status cut:\n%s", ansi.Strip(m.View()))
	}
}

func TestKubectlOutputErrorsWrapped(t *testing.T) {
	m := kustModel(t, nil, "")
	m = resize(t, m, 100, 30)
	m.logText = ""
	m.showOutput(kOutMsg{id: m.logID, text: "Error from server (NotFound): " + strings.Repeat("the server could not find the requested resource ", 4) + "THE-END", code: 1, line: "kubectl get foo"})
	if m.stage != "kout" || !strings.Contains(joined(m.View()), "THE-END") {
		t.Fatalf("kubectl output cuts the error:\n%s", ansi.Strip(m.View()))
	}
	rowsWidth(t, m.View(), 100)
}

func TestErrorLogOpensFullResponses(t *testing.T) {
	m := actionModel(t, actionCluster())
	first := "FIRST " + strings.Repeat("alpha ", 30) + "END-A"
	second := "SECOND " + strings.Repeat("beta ", 30) + "END-B"
	m.noteErr(first)
	m.context = "other"
	m.noteErr(second)
	m = press(t, m, "!")
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "END-B") || !strings.Contains(view, "2/2") {
		t.Fatalf("popup cut the error:\n%s", view)
	}
	m = press(t, m, "[")
	if !strings.Contains(ansi.Strip(m.View()), "END-A") || !strings.Contains(ansi.Strip(m.View()), "1/2") {
		t.Fatal("older error missing")
	}
	m = press(t, m, "enter")
	if m.stage != "view" || !strings.Contains(m.logText, "END-A") || strings.Contains(m.logText, "END-B") {
		t.Fatalf("one log stage %s text %q", m.stage, m.logText)
	}
	m.noteErr(first)
	m.noteErr(second)
	m = press(t, m, "!", "a")
	if m.stage != "view" || !strings.Contains(m.logText, "END-A") || !strings.Contains(m.logText, "END-B") {
		t.Fatalf("all log stage %s", m.stage)
	}
}
