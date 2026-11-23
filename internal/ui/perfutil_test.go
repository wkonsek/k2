package ui

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"

	"k2/internal/config"
	"k2/internal/kube"
	"k2/internal/shell"
)

type harness interface {
	Helper()
	Cleanup(func())
	TempDir() string
	Setenv(key, value string)
}

func envFor(t harness) config.Config {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("KUBECONFIG", "")
	cfg := config.Default()
	cfg.StateDir = t.TempDir()
	cfg.UI.PodMetrics = false
	return cfg
}

func modelFor(t harness, cfg config.Config, f *shell.Fake) model {
	t.Helper()
	if f == nil {
		f = &shell.Fake{Handler: func(shell.Request) shell.Result { return shell.OK("") }}
	}
	m := initialModel(Options{Runner: f, Config: &cfg})
	t.Cleanup(m.streams.stopAll)
	m.width, m.height = 200, 50
	m.kubeconfig, m.context, m.namespace = "/kc", "kind-dev", "shop"
	return m
}

func benchPod(i int) kube.Object {
	phase := "Running"
	waiting := ""
	ready := "true"
	if i%17 == 0 {
		phase = "Pending"
	}
	if i%41 == 0 {
		phase = "CrashLoopBackOff"
		waiting = `,"waiting":{"reason":"CrashLoopBackOff"}`
		ready = "false"
	}
	at := time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC).Add(time.Duration(i) * time.Minute).Format(time.RFC3339)
	js := fmt.Sprintf(`{"metadata":{"name":"web-frontend-production-deployment-7d9f8c6b5d-%05d","namespace":"shop","creationTimestamp":%q},`+
		`"spec":{"nodeName":"worker-node-availability-zone-a-%03d","containers":[{"name":"app"}]},`+
		`"status":{"phase":%q,"podIP":"10.%d.%d.%d","containerStatuses":[{"ready":%s,"restartCount":%d,"state":{%s}}]}}`,
		i, at, i%80, phase, 64+(i/65536)%16, (i/256)%256, i%256, ready, i%9, strings.TrimPrefix(waiting, ","))
	if waiting == "" {
		js = strings.Replace(js, `"state":{}`, `"state":{"running":{}}`, 1)
	}
	var o kube.Object
	if err := json.Unmarshal([]byte(js), &o); err != nil {
		panic(err)
	}
	return o
}

func manyPods(n int) []kube.Object {
	out := make([]kube.Object, n)
	for i := 0; i < n; i++ {
		out[i] = benchPod(i)
	}
	return out
}

func packPods(n, width int) (string, []list.Item) {
	header, items, err := packKube(kube.PodTable(manyPods(n), false), width, "shop")
	if err != nil {
		panic(err)
	}
	return header, items
}

func packSecrets(n, width int) (string, []list.Item) {
	headers := []string{"NAME", "TYPE", "DATA", "AGE"}
	rows := make([][]string, n)
	refs := make([][2]string, n)
	born := make([]time.Time, n)
	base := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < n; i++ {
		name := fmt.Sprintf("registry-pull-credentials-service-account-token-%04d", i)
		rows[i] = []string{name, "kubernetes.io/dockerconfigjson", "3", "12d"}
		refs[i] = [2]string{"shop", name}
		born[i] = base.Add(time.Duration(i) * time.Hour)
	}
	header, items, err := packTable(headers, rows, refs, born, 1, width, "shop")
	if err != nil {
		panic(err)
	}
	return header, items
}

func loadTable(m model, stage, kind, header string, items []list.Item) model {
	m.stage, m.kind = stage, kind
	m.content.SetShowTitle(false)
	setRows(&m.content, header, items, m.vs, "")
	m.fitContent(0, 0)
	return m
}

func bigCluster(t harness, pods, secrets int) (model, *shell.Fake) {
	t.Helper()
	podHeader, podItems := packPods(pods, 196)
	secHeader, secItems := packSecrets(secrets, 196)
	podJSON := podListJSON(pods)
	f := &shell.Fake{Handler: func(r shell.Request) shell.Result {
		line := strings.Join(r.Args, " ")
		switch {
		case strings.HasPrefix(line, "get pods") && strings.Contains(line, "-o json"):
			return shell.OK(podJSON)
		case strings.HasPrefix(line, "get secrets") && strings.Contains(line, "-o json"):
			return shell.OK(secretListJSON(secrets))
		case strings.HasPrefix(line, "describe"):
			return shell.OK("Name: " + lastArg(r.Args) + "\nStatus: Running\n")
		case strings.HasPrefix(line, "logs "):
			if containsArg(r.Args, "-f") {
				<-r.Ctx.Done()
				return shell.Fail(137, "killed")
			}
			return shell.OK("2026-11-23T10:00:00.000000000Z hello\n")
		case r.Name == "helm":
			return shell.OK("ok")
		default:
			return shell.OK("")
		}
	}}
	m := modelFor(t, envFor(t), f)
	m = loadTable(m, "workpods", "pods", podHeader, podItems)
	m.vs.pick = ""
	_ = secHeader
	_ = secItems
	_ = m.View()
	return m, f
}

func lastArg(args []string) string {
	if len(args) == 0 {
		return ""
	}
	return args[len(args)-1]
}

func containsArg(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}

func podListJSON(n int) string {
	var b strings.Builder
	b.Grow(n * 280)
	b.WriteString(`{"items":[`)
	for i := 0; i < n; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, `{"metadata":{"name":"web-frontend-production-deployment-7d9f8c6b5d-%05d","namespace":"shop","creationTimestamp":"2026-10-01T08:00:00Z"},`+
			`"spec":{"nodeName":"worker-node-availability-zone-a-000","containers":[{"name":"app"}]},`+
			`"status":{"phase":"Running","podIP":"10.64.0.1","containerStatuses":[{"ready":true,"restartCount":0,"state":{"running":{}}}]}}`, i)
	}
	b.WriteString(`]}`)
	return b.String()
}

func secretListJSON(n int) string {
	var b strings.Builder
	b.WriteString(`{"items":[`)
	for i := 0; i < n; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, `{"metadata":{"name":"registry-pull-credentials-service-account-token-%04d","namespace":"shop","creationTimestamp":"2026-09-01T00:00:00Z"},"type":"kubernetes.io/dockerconfigjson","data":{"t":"YQ=="}}`, i)
	}
	b.WriteString(`]}`)
	return b.String()
}

func fillLogs(m model, n int) model {
	var b strings.Builder
	b.Grow(n * 72)
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "2026-11-23T10:00:00.%06dZ line-%06d some log message payload for the stream\n", i, i)
	}
	m.stage = "logs"
	m.logPod = "shop/web-frontend-production-deployment-7d9f8c6b5d-00000"
	m.logText = b.String()
	m.logStick = true
	m.cfg.Logs.MaxLines = n
	m.vs.logPrev = false
	return m
}

func withSplits(m model, n int) model {
	for i := 0; i < n; i++ {
		next, cmd := m.split(i%2 == 0)
		m = next.(model)
		if cmd != nil {
			m = settleMsg(m, cmd)
		}
	}
	return m
}

func settleMsg(m model, cmd tea.Cmd) model {
	if cmd == nil {
		return m
	}
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	select {
	case msg := <-done:
		switch msg := msg.(type) {
		case tea.BatchMsg:
			for _, c := range msg {
				m = settleMsg(m, c)
			}
		case nil:
		default:
			next, more := m.Update(msg)
			m = next.(model)
			m = settleMsg(m, more)
		}
	case <-time.After(50 * time.Millisecond):
	}
	return m
}

func doUpdateView(m model, msg tea.Msg) model {
	next, _ := m.Update(msg)
	m = next.(model)
	_ = m.View()
	return m
}
