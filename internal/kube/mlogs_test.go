package kube

import (
	"strings"
	"testing"
	"time"
)

func TestParseLogQuery(t *testing.T) {
	q, err := ParseLogQuery("app=web,tier!=db --since=5m -c app", true)
	if err != nil || q.Selector != "app=web,tier!=db" || q.Since != 5*time.Minute || q.Container != "app" || q.All {
		t.Fatalf("%+v %v", q, err)
	}
	q, err = ParseLogQuery("^web-.* --tail 20", true)
	if err != nil || q.Regex == nil || !q.Regex.MatchString("web-1") || q.Tail != 20 || !q.All {
		t.Fatalf("%+v %v", q, err)
	}
	if q, _ := ParseLogQuery("env in (a,b)", false); q.Selector != "env in (a,b)" || q.All {
		t.Fatalf("set selector %+v", q)
	}
	if q, _ := ParseLogQuery("web --all=false", true); q.All {
		t.Fatal("--all=false")
	}
	for _, bad := range []string{"", "--since=x web", "--tail=-1 web", "web(["} {
		if _, err := ParseLogQuery(bad, true); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestSelectorOf(t *testing.T) {
	o := Object{Raw: map[string]any{"kind": "Deployment", "spec": map[string]any{"selector": map[string]any{
		"matchLabels":      map[string]any{"b": "2", "a": "1"},
		"matchExpressions": []any{map[string]any{"key": "env", "operator": "In", "values": []any{"x", "y"}}, map[string]any{"key": "gone", "operator": "DoesNotExist"}},
	}}}}
	if got := SelectorOf(o); got != "a=1,b=2,env in (x,y),!gone" {
		t.Fatal(got)
	}
	svc := Object{Raw: map[string]any{"kind": "Service", "spec": map[string]any{"selector": map[string]any{"app": "web"}}}}
	if got := SelectorOf(svc); got != "app=web" {
		t.Fatal(got)
	}
}

func TestParseMultiLogsAndCursor(t *testing.T) {
	out := "[pod/web-1/app] 2026-10-23T10:00:01.5Z hello\n" +
		"[pod/web-1/side] 2026-10-23T10:00:02Z side line\n" +
		"[pod/web-1/app] 2026-10-23T10:00:03.000000001Z \n" +
		"stray text\n"
	lines := ParseMultiLogs(out, "web-1")
	if len(lines) != 4 || lines[0].Source() != "web-1/app" || lines[0].Text != "hello" || lines[1].Container != "side" || lines[2].Text != "" || lines[3].Container != "?" {
		t.Fatalf("%+v", lines)
	}
	fresh, cur := LogCursor(nil).Fresh(lines[:2])
	if len(fresh) != 2 || cur.Since("web-1").Second() != 1 {
		t.Fatalf("first %v %v", fresh, cur)
	}

	again, cur2 := cur.Fresh(ParseMultiLogs(out, "web-1")[:3])
	if len(again) != 1 || again[0].Text != "" {
		t.Fatalf("again %+v", again)
	}
	if len(cur) != 2 || cur["web-1/app"].Equal(cur2["web-1/app"]) {
		t.Fatal("cursor must be copied")
	}
	mixed := []LogLine{{Pod: "b", At: time.Unix(5, 0)}, {Pod: "a", At: time.Unix(3, 0)}, {Pod: "c", At: time.Unix(5, 0)}}
	SortLines(mixed)
	if mixed[0].Pod != "a" || mixed[1].Pod != "b" || mixed[2].Pod != "c" {
		t.Fatalf("sort %+v", mixed)
	}
}

func TestMultiLogsArgs(t *testing.T) {
	q := LogQuery{All: true}
	a := strings.Join(MultiLogsArgs("web-1", "shop", "/kc", "ctx", q, time.Time{}, 50), " ")
	if a != "logs web-1 -n shop --prefix --timestamps --kubeconfig /kc --context ctx --all-containers --tail=50" {
		t.Fatal(a)
	}
	since := time.Date(2026, 10, 23, 10, 0, 1, 500, time.UTC)
	a = strings.Join(MultiLogsArgs("web-1", "shop", "/kc", "ctx", LogQuery{Container: "app", Since: time.Minute}, since, 50), " ")
	if !strings.HasSuffix(a, "-c app --since-time=2026-10-23T10:00:01Z") {
		t.Fatal(a)
	}
	a = strings.Join(MultiLogsArgs("web-1", "shop", "/kc", "ctx", LogQuery{Since: time.Minute}, time.Time{}, 50), " ")
	if !strings.HasSuffix(a, "--since=1m0s") {
		t.Fatal(a)
	}
}
