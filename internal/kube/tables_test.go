package kube

import (
	"strings"
	"testing"
	"time"
)

func objs(t *testing.T, js string) []Object {
	t.Helper()
	o, err := ParseObjects([]byte(js))
	if err != nil {
		t.Fatal(err)
	}
	return o
}

func TestBaseKind(t *testing.T) {
	cases := map[string]string{
		"deploy": "deployments", "Deployments.apps": "deployments", "dp": "deployments",
		"sts": "statefulsets", "cj": "cronjobs", "ev": "events", "events.events.k8s.io": "events",
		"subscriptions.operators.coreos.com":             "subscriptions",
		"subscriptions.messaging.knative.dev":            "subscriptions.messaging.knative.dev",
		"csv":                                            "clusterserviceversions",
		"routes.route.openshift.io":                      "routes",
		"widgets.example.com":                            "widgets.example.com",
		"customresourcedefinitions.apiextensions.k8s.io": "customresourcedefinitions",
	}
	for in, want := range cases {
		if got := BaseKind(in); got != want {
			t.Errorf("BaseKind(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRawHelpers(t *testing.T) {
	o := objs(t, `{"items":[{"metadata":{"name":"a"},"spec":{"replicas":3,"suspend":true,
		"containers":[{"name":"app"},{"name":"side"}],"initContainers":[{"name":"init"}]},
		"status":{"conditions":[{"type":"Complete","status":"True"}]}}]}`)[0]
	if n, ok := o.Int("spec", "replicas"); !ok || n != 3 || o.Str("spec", "suspend") != "true" || o.Str("nope", "x") != "" {
		t.Fatalf("helpers: %v %v %q", n, ok, o.Str("spec", "suspend"))
	}
	if o.Str("spec", "containers", "1", "name") != "side" || o.Get("spec", "containers", "9") != nil {
		t.Fatal("list index")
	}
	init, main := o.Containers()
	if strings.Join(init, ",") != "init" || strings.Join(main, ",") != "app,side" {
		t.Fatalf("containers %v %v", init, main)
	}
	if JobStatus(o) != "Complete" {
		t.Fatal(JobStatus(o))
	}
}

func TestWorkloadTables(t *testing.T) {
	js := `{"items":[{"metadata":{"name":"web","namespace":"shop","creationTimestamp":"2020-01-01T00:00:00Z"},
		"spec":{"replicas":3},"status":{"readyReplicas":2,"updatedReplicas":3,"availableReplicas":2}}]}`
	d := TableFor("deploy", objs(t, js), false)
	if strings.Join(d.Headers, " ") != "NAME READY UP-TO-DATE AVAILABLE AGE" || strings.Join(d.Rows[0][:4], " ") != "web 2/3 3 2" {
		t.Fatalf("deploy %v %v", d.Headers, d.Rows)
	}
	if d.Extra[0]["replicas"] != "3" || d.Refs[0] != [2]string{"shop", "web"} {
		t.Fatalf("extra %v", d.Extra)
	}
	s := TableFor("statefulsets.apps", objs(t, js), true)
	if strings.Join(s.Headers, " ") != "NAMESPACE NAME READY AGE" || s.Rows[0][2] != "2/3" {
		t.Fatalf("sts %v %v", s.Headers, s.Rows)
	}
	ds := TableFor("ds", objs(t, `{"items":[{"metadata":{"name":"n"},"status":{"desiredNumberScheduled":2,"numberReady":1}}]}`), false)
	if strings.Join(ds.Headers, " ") != "NAME DESIRED CURRENT READY UP-TO-DATE AVAILABLE AGE" || ds.Rows[0][3] != "1/2" {
		t.Fatalf("ds %v %v", ds.Headers, ds.Rows)
	}
	rs := TableFor("rs", objs(t, js), false)
	if strings.Join(rs.Rows[0][:4], " ") != "web 3 0 2/3" {
		t.Fatalf("rs %v", rs.Rows)
	}

	if g := TableFor("widgets.example.com", objs(t, js), false); strings.Join(g.Headers, " ") != "NAME AGE" {
		t.Fatalf("generic %v", g.Headers)
	}
}

func TestJobTables(t *testing.T) {
	js := `{"items":[
	 {"metadata":{"name":"done"},"spec":{"completions":1},"status":{"succeeded":1,"startTime":"2024-01-01T00:00:00Z","completionTime":"2024-01-01T00:01:30Z","conditions":[{"type":"Complete","status":"True"}]}},
	 {"metadata":{"name":"bad"},"status":{"failed":1,"conditions":[{"type":"Failed","status":"True"}]}},
	 {"metadata":{"name":"run"},"status":{"active":1,"startTime":"2024-01-01T00:00:00Z"}},
	 {"metadata":{"name":"wait"},"spec":{"suspend":true}}]}`
	tab := TableFor("jobs", objs(t, js), false)
	if strings.Join(tab.Headers, " ") != "NAME STATUS COMPLETIONS DURATION AGE" || tab.ColorCol != 1 {
		t.Fatalf("job heads %v %d", tab.Headers, tab.ColorCol)
	}
	got := map[string]string{}
	for _, r := range tab.Rows {
		got[r[0]] = r[1] + " " + r[2] + " " + r[3]
	}
	if got["done"] != "Complete 1/1 1m" || !strings.HasPrefix(got["bad"], "Failed 0/1 -") ||
		!strings.HasPrefix(got["run"], "Running 0/1 ") || got["run"] == "Running 0/1 -" || !strings.HasPrefix(got["wait"], "Suspended") {
		t.Fatalf("job rows %v", got)
	}
	cj := TableFor("cj", objs(t, `{"items":[{"metadata":{"name":"nightly"},"spec":{"schedule":"0 2 * * *","suspend":true},
		"status":{"active":[{"name":"x"}],"lastScheduleTime":"`+time.Now().Add(-90*time.Minute).UTC().Format(time.RFC3339)+`"}}]}`), false)
	if strings.Join(cj.Headers, " ") != "NAME SCHEDULE SUSPEND ACTIVE LAST SCHEDULE AGE" ||
		strings.Join(cj.Rows[0][:5], "|") != "nightly|0 2 * * *|Suspended|1|1h30m ago" || cj.ColorCol != 2 {
		t.Fatalf("cronjob %v %v", cj.Headers, cj.Rows)
	}
}

func TestEventTable(t *testing.T) {
	now := time.Now().UTC()
	at := func(d time.Duration) string { return now.Add(-d).Format(time.RFC3339) }
	js := `{"items":[
	 {"metadata":{"name":"e1","namespace":"shop"},"type":"Warning","reason":"BackOff","message":"Back-off\nrestarting",
	  "involvedObject":{"kind":"Pod","name":"web-1"},"lastTimestamp":"` + at(time.Minute) + `"},
	 {"metadata":{"name":"e0","namespace":"shop"},"type":"Normal","reason":"Pulled","message":"pulled",
	  "involvedObject":{"kind":"Pod","name":"web-1"},"lastTimestamp":"` + at(time.Hour) + `"},
	 {"metadata":{"name":"e2","namespace":"shop","creationTimestamp":"` + at(2*time.Hour) + `"},"type":"Normal","reason":"New",
	  "regarding":{"kind":"Node","name":"n1"},"note":"hello","eventTime":"` + now.Add(-30*time.Second).Format(time.RFC3339Nano) + `"}]}`
	tab := TableFor("ev", objs(t, js), false)
	if strings.Join(tab.Headers, " ") != "LAST SEEN TYPE REASON OBJECT MESSAGE" || tab.ColorCol != 1 {
		t.Fatalf("heads %v", tab.Headers)
	}

	if tab.Refs[0][1] != "e0" || tab.Refs[1][1] != "e1" || tab.Refs[2][1] != "e2" {
		t.Fatalf("order %v", tab.Refs)
	}
	if strings.Join(tab.Rows[1][1:], "|") != "Warning|BackOff|pod/web-1|Back-off restarting" || tab.Rows[1][0] != "1m" {
		t.Fatalf("row %v", tab.Rows[1])
	}
	if tab.Rows[2][3] != "node/n1" || tab.Rows[2][4] != "hello" {
		t.Fatalf("events.k8s.io row %v", tab.Rows[2])
	}
	all := TableFor("events", objs(t, js), true)
	if all.Headers[0] != "NAMESPACE" || all.ColorCol != 2 {
		t.Fatalf("all ns %v", all.Headers)
	}
}

func TestMissingAPI(t *testing.T) {
	msg := `error: the server doesn't have a resource type "subscriptions"`
	if got := MissingAPI("subscriptions.operators.coreos.com", msg); got != "subscriptions.operators.coreos.com is not installed on this cluster (no OLM)" {
		t.Fatal(got)
	}
	if got := MissingAPI("routes", msg); !strings.HasSuffix(got, "(not OpenShift)") {
		t.Fatal(got)
	}
	if MissingAPI("pods", "connection refused") != "" {
		t.Fatal("other errors pass through")
	}
}
