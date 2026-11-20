package kube

import (
	"strings"
	"testing"
	"time"
)

const containerPod = `{"metadata":{"name":"shop-1","namespace":"shop","creationTimestamp":"2026-10-23T08:00:00Z"},
"spec":{
 "initContainers":[
  {"name":"migrate","image":"reg.example/shop:2.3.0","resources":{"requests":{"cpu":"50m","memory":"64Mi"},"limits":{"cpu":"200m","memory":"128Mi"}}},
  {"name":"shipper","image":"fluent/fluent-bit:3.1.9","restartPolicy":"Always","ports":[{"name":"metrics","containerPort":2020}],"livenessProbe":{"tcpSocket":{"port":2020}}}],
 "containers":[
  {"name":"app","image":"reg.example/shop:2.3.0","ports":[{"name":"http","containerPort":8080},{"containerPort":9090}],
   "readinessProbe":{"tcpSocket":{"port":8080}},"resources":{"requests":{"cpu":"100m","memory":"128Mi"},"limits":{"cpu":"500m","memory":"256Mi"}}},
  {"name":"proxy","image":"envoyproxy/envoy:v1.31.2","livenessProbe":{"tcpSocket":{"port":9901}},"readinessProbe":{"tcpSocket":{"port":9901}},
   "ports":[{"name":"admin","containerPort":9901}],"resources":{"requests":{"cpu":"1","memory":"1Gi"},"limits":{"cpu":"2","memory":"2Gi"}}}]},
"status":{
 "initContainerStatuses":[
  {"name":"migrate","ready":true,"restartCount":0,"state":{"terminated":{"reason":"Completed","startedAt":"2026-10-23T08:00:10Z","finishedAt":"2026-10-23T08:00:13Z"}}},
  {"name":"shipper","ready":true,"restartCount":0,"state":{"running":{"startedAt":"2026-10-23T08:00:14Z"}}}],
 "containerStatuses":[
  {"name":"app","ready":false,"restartCount":7,"state":{"waiting":{"reason":"CrashLoopBackOff"}},"lastState":{"terminated":{"reason":"Error","finishedAt":"2026-10-23T09:59:00Z"}}},
  {"name":"proxy","ready":true,"restartCount":0,"state":{"running":{"startedAt":"2026-10-23T08:00:20Z"}}}]}}`

func containerObject(t *testing.T) Object {
	t.Helper()
	objs, err := ParseObjects([]byte(`{"items":[` + containerPod + `]}`))
	if err != nil || len(objs) != 1 {
		t.Fatalf("parse: %v %d", err, len(objs))
	}
	return objs[0]
}

func rowOf(t *testing.T, tb Table, prefix string) map[string]string {
	t.Helper()
	for _, r := range tb.Rows {
		if strings.HasPrefix(r[0], prefix) {
			out := map[string]string{}
			for i, h := range tb.Headers {
				out[h] = r[i]
			}
			return out
		}
	}
	t.Fatalf("no row %q in %v", prefix, tb.Rows)
	return nil
}

func TestContainerTableColumnsAndValues(t *testing.T) {
	usage := map[string]Usage{"app": {CPU: "250m", Mem: "128Mi"}, "proxy": {CPU: "100m", Mem: "512Mi"}, "shipper": {CPU: "5m", Mem: "20Mi"}}
	tb := ContainerTable(containerObject(t), usage, true)
	want := []string{"NAME", "PF", "IMAGE", "READY", "STATE", "INIT", "RESTARTS", "PROBES(L:R)", "CPU", "MEM", "CPU/R:L", "MEM/R:L", "%CPU/R", "%CPU/L", "%MEM/R", "%MEM/L", "PORTS", "AGE"}
	if strings.Join(tb.Headers, "|") != strings.Join(want, "|") || tb.ColorCol != 4 {
		t.Fatalf("headers %v color %d", tb.Headers, tb.ColorCol)
	}
	if len(tb.Rows) != 4 || len(tb.Refs) != 4 || len(tb.Extra) != 4 || len(tb.Born) != 4 {
		t.Fatalf("rows %d", len(tb.Rows))
	}
	order := []string{"migrate (init)", "shipper (sidecar)", "app", "proxy"}
	for i, name := range order {
		if tb.Rows[i][0] != name {
			t.Fatalf("row %d is %q, want %q", i, tb.Rows[i][0], name)
		}
	}
	if tb.Refs[2] != [2]string{"shop", "app"} || tb.Extra[2]["container"] != "app" || tb.Extra[2]["ports"] != "8080,9090" {
		t.Fatalf("refs %v extra %v", tb.Refs[2], tb.Extra[2])
	}

	mig := rowOf(t, tb, "migrate")
	if mig["STATE"] != "Completed" || mig["INIT"] != "true" || mig["READY"] != "true" || mig["CPU"] != "-" || mig["PORTS"] != "" || mig["PROBES(L:R)"] != "off:off" {
		t.Fatalf("migrate %v", mig)
	}
	ship := rowOf(t, tb, "shipper")
	if ship["STATE"] != "Running" || ship["PORTS"] != "metrics:2020" || ship["PROBES(L:R)"] != "on:off" || ship["CPU/R:L"] != "0:0" {
		t.Fatalf("shipper %v", ship)
	}
	app := rowOf(t, tb, "app")
	wantApp := map[string]string{"READY": "false", "STATE": "CrashLoopBackOff", "INIT": "false", "PROBES(L:R)": "off:on", "CPU": "250m", "MEM": "128Mi",
		"CPU/R:L": "100:500", "MEM/R:L": "128:256", "%CPU/R": "250", "%CPU/L": "50", "%MEM/R": "100", "%MEM/L": "50", "PORTS": "http:8080,9090", "IMAGE": "reg.example/shop:2.3.0"}
	for k, v := range wantApp {
		if app[k] != v {
			t.Errorf("app %s = %q, want %q", k, app[k], v)
		}
	}
	if !strings.HasPrefix(app["RESTARTS"], "7 (") || !strings.HasSuffix(app["RESTARTS"], " ago)") {
		t.Fatalf("restarts %q", app["RESTARTS"])
	}
	proxy := rowOf(t, tb, "proxy")
	if proxy["CPU/R:L"] != "1000:2000" || proxy["MEM/R:L"] != "1024:2048" || proxy["PROBES(L:R)"] != "on:on" || proxy["%MEM/R"] != "50" || proxy["%CPU/L"] != "5" || proxy["PORTS"] != "admin:9901" {
		t.Fatalf("proxy %v", proxy)
	}
	if proxy["AGE"] != Age(time.Date(2026, 10, 23, 8, 0, 20, 0, time.UTC)) && proxy["AGE"] == "" {
		t.Fatalf("age %q", proxy["AGE"])
	}
}

func TestContainerTableWithoutMetrics(t *testing.T) {
	tb := ContainerTable(containerObject(t), nil, false)
	for _, name := range []string{"app", "proxy", "shipper"} {
		r := rowOf(t, tb, name)
		for _, h := range []string{"CPU", "MEM", "%CPU/R", "%CPU/L", "%MEM/R", "%MEM/L"} {
			if r[h] != "n/a" {
				t.Errorf("%s %s = %q", name, h, r[h])
			}
		}
	}
	if r := rowOf(t, tb, "app"); r["CPU/R:L"] != "100:500" {
		t.Fatalf("limits still show: %v", r)
	}
}

func TestContainerTableUnknownStatusAndAge(t *testing.T) {
	objs, _ := ParseObjects([]byte(`{"items":[{"metadata":{"name":"p","namespace":"x","creationTimestamp":"2026-10-23T08:00:00Z"},"spec":{"containers":[{"name":"c","image":"i"}]},"status":{}}]}`))
	tb := ContainerTable(objs[0], map[string]Usage{}, true)
	r := rowOf(t, tb, "c")
	if r["STATE"] != "Pending" || r["READY"] != "false" || r["CPU"] != "-" || r["AGE"] == "" || r["RESTARTS"] != "0" {
		t.Fatalf("%v", r)
	}
}

func TestParseTopContainers(t *testing.T) {
	got := ParseTopContainers("shop-1   app     250m   128Mi\nshop-1   proxy   2m     9Mi\nnonsense\nshop-1 x y z\n")
	if len(got) != 2 || got["app"] != (Usage{CPU: "250m", Mem: "128Mi"}) || got["proxy"].Mem != "9Mi" {
		t.Fatalf("%v", got)
	}
	args := strings.Join(TopContainersArgs("/kc", "ctx", "shop", "shop-1"), " ")
	for _, want := range []string{"top pod shop-1", "--containers", "--no-headers", "-n shop", "--kubeconfig /kc", "--context ctx"} {
		if !strings.Contains(args, want) {
			t.Fatalf("args %q lack %q", args, want)
		}
	}
}
