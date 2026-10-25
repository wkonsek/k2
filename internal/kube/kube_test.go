package kube

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"k2/internal/shell"
)

func TestParseAPIResources(t *testing.T) {
	out := `bindings                                       v1                                true         Binding
configmaps                        cm           v1                                true         ConfigMap
pods                              po           v1                                true         Pod
deployments                       deploy       apps/v1                           true         Deployment
customresourcedefinitions         crd,crds     apiextensions.k8s.io/v1           false        CustomResourceDefinition
subscriptions                     sub,subs     operators.coreos.com/v1alpha1     true         Subscription
clusterserviceversions            csv,csvs     operators.coreos.com/v1alpha1     true         ClusterServiceVersion
routes                                         route.openshift.io/v1             true         Route
garbage
`
	got := ParseAPIResources(out)
	want := []string{"bindings", "clusterserviceversions", "cm", "configmaps", "crd", "crds", "csv", "csvs",
		"customresourcedefinitions", "deploy", "deployments", "po", "pods", "routes", "sub", "subs", "subscriptions"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v", got)
	}
}

const podsJSON = `{"items":[
 {"metadata":{"name":"web-1","namespace":"shop","creationTimestamp":"2026-10-22T10:00:00Z"},
  "spec":{"nodeName":"node-a"},
  "status":{"phase":"Running","podIP":"10.0.0.7","containerStatuses":[
    {"ready":true,"restartCount":2,"state":{}},
    {"ready":false,"restartCount":1,"state":{"waiting":{"reason":"CrashLoopBackOff"}}}]}},
 {"metadata":{"name":"db-0","namespace":"shop","creationTimestamp":"2026-10-22T10:00:00Z","deletionTimestamp":"2026-10-22T11:00:00Z"},
  "spec":{},"status":{"phase":"Running"}},
 {"metadata":{"name":"api","namespace":"core","creationTimestamp":"2026-10-22T10:00:00Z"},
  "spec":{"nodeName":"node-b"},
  "status":{"phase":"Succeeded","containerStatuses":[{"ready":false,"state":{"terminated":{"reason":"Completed"}}}]}}
]}`

func TestParseObjectsAndPodTable(t *testing.T) {
	objs, err := ParseObjects([]byte(podsJSON))
	if err != nil {
		t.Fatal(err)
	}

	if objs[0].Metadata.Name != "api" || objs[1].Metadata.Name != "db-0" || objs[2].Metadata.Name != "web-1" {
		t.Fatalf("order %v %v %v", objs[0].Metadata.Name, objs[1].Metadata.Name, objs[2].Metadata.Name)
	}
	tab := PodTable(objs, true)
	if !reflect.DeepEqual(tab.Headers, []string{"NAMESPACE", "NAME", "READY", "STATUS", "RESTARTS", "IP", "NODE", "AGE"}) || tab.ColorCol != 3 {
		t.Fatalf("headers %v col %d", tab.Headers, tab.ColorCol)
	}
	api, db, web := tab.Rows[0], tab.Rows[1], tab.Rows[2]
	if api[3] != "Completed" || api[2] != "0/1" || api[5] != "-" || api[6] != "node-b" {
		t.Errorf("api row %v", api)
	}
	if db[3] != "Terminating" || db[2] != "-" || db[6] != "-" {
		t.Errorf("db row %v", db)
	}
	if web[3] != "CrashLoopBackOff" || web[2] != "1/2" || web[4] != "3" || web[5] != "10.0.0.7" || web[6] != "node-a" {
		t.Errorf("web row %v", web)
	}
	if tab.Refs[2] != [2]string{"shop", "web-1"} || tab.Born[2].IsZero() {
		t.Errorf("refs/born %v %v", tab.Refs[2], tab.Born[2])
	}
	one := PodTable(objs[:1], false)
	if one.Headers[0] != "NAME" || one.ColorCol != 2 || len(one.Rows[0]) != 7 {
		t.Errorf("namespaced table %v", one)
	}
	if _, err := ParseObjects([]byte("Error from server")); err == nil {
		t.Fatal("expected error for non-json")
	}
}

func TestNamespaceOfReadsContext(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config")
	body := `apiVersion: v1
kind: Config
current-context: kind-dev
contexts:
- name: shop/api-cluster:6443/kube:admin
  context: {cluster: api, namespace: payments, user: admin}
- name: proj/api-cluster:6443/dev
  context: {cluster: api, user: dev}
- name: kind-dev
  context: {cluster: kind, user: kind}
- name: a/b/c/d
  context: {cluster: c, user: u}
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	f := &shell.Fake{Handler: func(shell.Request) shell.Result { return shell.OK("from-kubectl") }}
	c := New(f)
	if got := c.NamespaceOf(path, "shop/api-cluster:6443/kube:admin"); got != "payments" {
		t.Fatalf("field wins: %q", got)
	}
	if got := c.NamespaceOf(path, "proj/api-cluster:6443/dev"); got != "proj" {
		t.Fatalf("oc name: %q", got)
	}
	if got := c.NamespaceOf(path, "kind-dev"); got != "" {
		t.Fatalf("empty field: %q", got)
	}
	if got := c.NamespaceOf(path, "a/b/c/d"); got != "" {
		t.Fatalf("four parts: %q", got)
	}
	if len(f.Calls()) != 0 {
		t.Fatal("kubectl was asked about a context that is in the file")
	}
	if got := c.NamespaceOf(path, "missing"); got != "from-kubectl" || len(f.Calls()) != 1 {
		t.Fatalf("fallback %q calls %d", got, len(f.Calls()))
	}
}

func TestResourceTable(t *testing.T) {
	objs, _ := ParseObjects([]byte(`{"items":[
	 {"metadata":{"name":"tls","namespace":"a","creationTimestamp":"2026-10-22T10:00:00Z"},"type":"kubernetes.io/tls","data":{"tls.crt":"eA==","tls.key":"eA=="}},
	 {"metadata":{"name":"old","namespace":"a","deletionTimestamp":"2026-10-22T10:00:00Z"}}]}`))
	sec := ResourceTable("secrets", objs, false)
	if !reflect.DeepEqual(sec.Headers, []string{"NAME", "TYPE", "DATA", "AGE"}) || sec.ColorCol != 1 {
		t.Fatalf("secret headers %v", sec.Headers)
	}
	if sec.Rows[0][0] != "old terminating" || sec.Rows[0][1] != "-" || sec.Rows[1][2] != "2" {
		t.Fatalf("rows %v", sec.Rows)
	}
	gen := ResourceTable("subscriptions", objs, true)
	if !reflect.DeepEqual(gen.Headers, []string{"NAMESPACE", "NAME", "AGE"}) || gen.ColorCol != -1 {
		t.Fatalf("generic %v", gen)
	}
	if !IsSecretKind("Secret") || IsSecretKind("secretstores") {
		t.Fatal("IsSecretKind")
	}
}

func TestAge(t *testing.T) {
	now := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		ago  time.Duration
		want string
	}{
		{0, "0s"}, {59 * time.Second, "59s"}, {time.Minute, "1m"}, {59 * time.Minute, "59m"},
		{time.Hour, "1h"}, {3*time.Hour + 5*time.Minute, "3h5m"}, {24 * time.Hour, "1d"}, {9*24*time.Hour + 5*time.Hour, "9d"},
	}
	for _, c := range cases {
		if got := AgeAt(now.Add(-c.ago), now); got != c.want {
			t.Errorf("AgeAt(-%v) = %q, want %q", c.ago, got, c.want)
		}
	}
	if Age(time.Time{}) != "" {
		t.Fatal("zero time")
	}
}

func b64(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

func TestDecodeSecret(t *testing.T) {
	raw := `{"data":{"user":"` + b64("admin") + `","multi":"` + b64("a\nb\n") + `","bin":"` + b64("\x00\x01\x02png") +
		`","empty":"","broken":"%%%"}}`
	got, err := DecodeSecret([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	want := "bin\n  [binary 6 bytes]\n\nbroken\n  [not base64]\n\nempty\n  [empty]\n\nmulti\n  a\n  b\n\nuser\n  admin"
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
	if s, _ := DecodeSecret([]byte(`{"data":{}}`)); s != "(no data)" {
		t.Fatal(s)
	}
	if _, err := DecodeSecret([]byte("nope")); err == nil {
		t.Fatal("want error")
	}
}

func TestIsBinary(t *testing.T) {
	if IsBinary([]byte("hello\nworld\t!")) || IsBinary(nil) {
		t.Fatal("text detected as binary")
	}
	if !IsBinary([]byte{'a', 0, 'b'}) || !IsBinary([]byte{1, 2, 3, 'a'}) {
		t.Fatal("binary not detected")
	}
}

func TestBuildArgs(t *testing.T) {
	cases := []struct {
		line, ns, format string
		json             bool
		want             string
	}{
		{"get pods", "shop", "", false, "get pods --kubeconfig /k --context c -n shop"},
		{"get pods", "", "", false, "get pods --kubeconfig /k --context c --all-namespaces"},
		{"get pods web-1 -o yaml", "", "wide", false, "get pods web-1 -o yaml --kubeconfig /k --context c"},
		{"logs web-1", "", "", false, "logs web-1 --kubeconfig /k --context c"},
		{"get pods -A", "shop", "wide", false, "get pods -A --kubeconfig /k --context c -o wide"},
		{"get csv --namespace=olm", "shop", "name", false, "get csv --namespace=olm --kubeconfig /k --context c -o name"},
		{"get pods --context=other", "shop", "", true, "get pods --context=other --kubeconfig /k -n shop -o json"},
	}
	for _, c := range cases {
		got := strings.Join(BuildArgs(c.line, "/k", "c", c.ns, c.format, c.json), " ")
		if got != c.want {
			t.Errorf("BuildArgs(%q)\n got %s\nwant %s", c.line, got, c.want)
		}
	}
}

func TestRunLineWithJQ(t *testing.T) {
	f := &shell.Fake{Handler: func(r shell.Request) shell.Result {
		switch r.Name {
		case "kubectl":
			return shell.Result{Stdout: []byte(`{"items":[]}`), Stderr: []byte("Warning: deprecated")}
		case "jq":
			if r.Stdin != `{"items":[]}` || r.Args[0] != ".items" {
				return shell.Fail(5, "bad jq call")
			}
			return shell.OK("[]\n")
		}
		return shell.Fail(1, "?")
	}}
	text, code := New(f).RunLine("get pods", "/k", "c", "ns", "", ".items")
	if code != 0 || text != "Warning: deprecated\n[]" {
		t.Fatalf("%d %q", code, text)
	}
	if args := f.Calls()[0].Args; args[len(args)-1] != "json" {
		t.Fatalf("jq must force -o json: %v", args)
	}
	f.Handler = func(r shell.Request) shell.Result {
		if r.Name == "jq" {
			return shell.Fail(3, "jq: error: syntax")
		}
		return shell.OK("{}")
	}
	text, code = New(f).RunLine("get pods", "/k", "c", "ns", "", ".[")
	if code != 1 || !strings.Contains(text, "jq: jq: error: syntax") {
		t.Fatalf("%d %q", code, text)
	}
}

func TestProjectsFallsBackWithoutOC(t *testing.T) {
	f := &shell.Fake{Missing: []string{"oc"}, Handler: func(r shell.Request) shell.Result {
		if strings.Contains(r.Line(), "get projects") {
			return shell.Fail(1, `error: the server doesn't have a resource type "projects"`)
		}
		return shell.OK("default\nkube-system\n")
	}}
	got, err := New(f).Projects("/k", "kind-dev")
	if err != nil || !reflect.DeepEqual(got, []string{"default", "kube-system"}) {
		t.Fatalf("%v %v", got, err)
	}
	for _, c := range f.Calls() {
		if c.Name == "oc" {
			t.Fatal("oc called although missing")
		}
	}
}

func TestLogsArgs(t *testing.T) {
	first := strings.Join(LogsArgs("web", "shop", "/k", "c", time.Time{}, 80, LogOpts{}), " ")
	if first != "logs web -n shop --kubeconfig /k --context c --tail=80" {
		t.Fatal(first)
	}
	since := time.Date(2026, 10, 22, 12, 0, 0, 0, time.FixedZone("x", 3600))
	next := LogsArgs("web", "shop", "/k", "c", since, 80, LogOpts{})
	if next[len(next)-1] != "--since-time=2026-10-22T11:00:00Z" {
		t.Fatal(next)
	}
}

func TestAppendAndTrimLogs(t *testing.T) {
	if got := AppendLogs("a\nb\nc", "b\nc\nd\ne"); got != "a\nb\nc\nd\ne" {
		t.Fatalf("overlap: %q", got)
	}
	if got := AppendLogs("a\nb", "b"); got != "a\nb" {
		t.Fatalf("suffix: %q", got)
	}
	if got := AppendLogs("", " x "); got != "x" {
		t.Fatal(got)
	}
	if got := TrimLog("1\n2\n3\n4", 2); got != "3\n4" {
		t.Fatal(got)
	}
}

func TestSplitRefAndEditArgs(t *testing.T) {
	if ns, n := SplitRef("shop/web", "x"); ns != "shop" || n != "web" {
		t.Fatal(ns, n)
	}
	if ns, n := SplitRef("web", "x"); ns != "x" || n != "web" {
		t.Fatal(ns, n)
	}
	if got := strings.Join(EditArgs("subscriptions", "etcd", "/k", "c", "olm"), " "); got != "edit subscriptions etcd --kubeconfig /k --context c -n olm" {
		t.Fatal(got)
	}
}

func TestPodTableLastRestart(t *testing.T) {
	ago := func(d time.Duration) string { return time.Now().Add(-d).UTC().Format(time.RFC3339) }
	js := `{"items":[{"metadata":{"name":"web","namespace":"shop","creationTimestamp":"` + ago(48*time.Hour) + `"},
  "status":{"phase":"Running","containerStatuses":[
    {"ready":true,"restartCount":2,"lastState":{"terminated":{"finishedAt":"` + ago(3*time.Hour) + `"}}},
    {"ready":true,"restartCount":1,"lastState":{"terminated":{"finishedAt":"` + ago(5*time.Minute+10*time.Second) + `"}}}]}},
 {"metadata":{"name":"old","namespace":"shop","creationTimestamp":"` + ago(time.Hour) + `"},
  "status":{"phase":"Running","containerStatuses":[{"ready":true,"restartCount":4}]}},
 {"metadata":{"name":"zero","namespace":"shop","creationTimestamp":"` + ago(time.Hour) + `"},
  "status":{"phase":"Running","containerStatuses":[{"ready":true,"restartCount":0,"lastState":{}}]}}]}`
	objs, err := ParseObjects([]byte(js))
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, r := range PodTable(objs, false).Rows {
		got[r[0]] = r[3]
	}
	if got["web"] != "3 (5m ago)" || got["old"] != "4" || got["zero"] != "0" {
		t.Fatalf("restarts %v", got)
	}
}

func TestDescribeArgs(t *testing.T) {
	got := DescribeArgs("pod", "web-1", "/kc", "kind-dev", "shop")
	want := []string{"describe", "pod", "web-1", "-n", "shop", "--kubeconfig", "/kc", "--context", "kind-dev"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%v", got)
	}

	got = DescribeArgs("nodes", "node-a", "/kc", "kind-dev", "")
	if !reflect.DeepEqual(got, []string{"describe", "nodes", "node-a", "--kubeconfig", "/kc", "--context", "kind-dev"}) {
		t.Fatalf("%v", got)
	}
	var seen []string
	c := New(&shell.Fake{Handler: func(r shell.Request) shell.Result {
		seen = append([]string{r.Name}, r.Args...)
		if r.Args[2] == "gone" {
			return shell.Fail(1, `Error from server (NotFound): pods "gone" not found`)
		}
		return shell.OK("Name: web-1\nEvents:  <none>\n")
	}})
	text, err := c.Describe("/kc", "kind-dev", "pod", "web-1", "shop")
	if err != nil || text != "Name: web-1\nEvents:  <none>" || seen[0] != "kubectl" || seen[1] != "describe" {
		t.Fatalf("%q %v %v", text, err, seen)
	}
	if _, err := c.Describe("/kc", "kind-dev", "pod", "gone", "shop"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("error %v", err)
	}
}

func TestForwardKind(t *testing.T) {
	for in, want := range map[string]string{"pods": "pod", "po": "pod", "services": "svc", "svc": "svc", "deployments.apps": "deploy",
		"deploy": "deploy", "statefulsets": "sts", "StatefulSet": "sts", "configmaps": "", "daemonsets": ""} {
		if got := ForwardKind(in); got != want {
			t.Errorf("ForwardKind(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTargetPortSuggestions(t *testing.T) {
	c := New(&shell.Fake{Handler: func(r shell.Request) shell.Result {
		line := strings.Join(r.Args, " ")
		switch {
		case strings.Contains(line, "get svc web -n shop"):
			return shell.OK(`{"spec":{"ports":[{"port":80,"targetPort":8080},{"port":443,"targetPort":"https"}]}}`)
		case strings.Contains(line, "get deployment api -n shop"):
			return shell.OK(`{"spec":{"template":{"spec":{"containers":[{"ports":[{"containerPort":9090},{"containerPort":9091}]},{"ports":[{"containerPort":9090}]}]}}}}`)
		}
		return shell.Fail(1, "no")
	}})
	got := c.TargetPortSuggestions("/kc", "dev", "svc", "shop", "web")
	if len(got) != 2 || got[0].Spec() != "80:80" || got[1].Spec() != "443:443" || got[0].Source != "svc/web" {
		t.Fatalf("svc %+v", got)
	}
	got = c.TargetPortSuggestions("/kc", "dev", "deploy", "shop", "api")
	if len(got) != 2 || got[0].Spec() != "9090:9090" || got[1].Spec() != "9091:9091" {
		t.Fatalf("deploy %+v", got)
	}
	if got := c.TargetPortSuggestions("/kc", "dev", "sts", "shop", "missing"); len(got) != 0 {
		t.Fatalf("missing %+v", got)
	}
}

func TestLogsArgsContainerPrevious(t *testing.T) {
	got := strings.Join(LogsArgs("web", "shop", "/k", "c", time.Time{}, 80, LogOpts{Container: "side", Previous: true}), " ")
	if got != "logs web -n shop --kubeconfig /k --context c -c side --previous --tail=80" {
		t.Fatal(got)
	}
}

func TestProjectsUseKubectlOnly(t *testing.T) {
	f := &shell.Fake{Handler: func(r shell.Request) shell.Result {
		if strings.Contains(r.Line(), "get projects") {
			return shell.OK("my-project\nopenshift-operators\n")
		}
		return shell.Fail(1, "unexpected")
	}}
	got, err := New(f).Projects("/k", "ocp")
	if err != nil || !reflect.DeepEqual(got, []string{"my-project", "openshift-operators"}) {
		t.Fatalf("%v %v", got, err)
	}
	for _, c := range f.Calls() {
		if c.Name != "kubectl" {
			t.Fatalf("ran %s", c.Name)
		}
	}
}
