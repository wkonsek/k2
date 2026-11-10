package kube

import (
	"strings"
	"testing"

	"k2/internal/shell"
)

func TestParsePrinterColumns(t *testing.T) {
	js := `{
  "spec": {
    "versions": [
      {
        "name": "v1",
        "storage": true,
        "additionalPrinterColumns": [
          {"name": "Ready", "type": "string", "jsonPath": ".status.conditions[?(@.type==\"Ready\")].status"},
          {"name": "Issuer", "type": "string", "jsonPath": ".spec.issuerRef.name"},
          {"name": "Status", "type": "string", "jsonPath": ".status.conditions[?(@.type==\"Ready\")].message"},
          {"name": "Age", "type": "date", "jsonPath": ".metadata.creationTimestamp"},
          {"name": "Name", "type": "string", "jsonPath": ".metadata.name"}
        ]
      }
    ]
  }
}`
	cols := ParsePrinterColumns([]byte(js))
	if len(cols) != 3 || cols[0].Name != "READY" || cols[1].Name != "ISSUER" || cols[2].Name != "STATUS" {
		t.Fatalf("%+v", cols)
	}
}

func TestEvalJSONPathReady(t *testing.T) {
	o := objs(t, `{"items":[{"metadata":{"name":"web","creationTimestamp":"2024-01-01T00:00:00Z"},
		"spec":{"issuerRef":{"name":"letsencrypt"}},
		"status":{"conditions":[{"type":"Ready","status":"True","message":"ok"}]}}]}`)[0]
	if got := EvalJSONPath(o, `.status.conditions[?(@.type=="Ready")].status`); got != "True" {
		t.Fatalf("ready %q", got)
	}
	if got := EvalJSONPath(o, ".spec.issuerRef.name"); got != "letsencrypt" {
		t.Fatalf("issuer %q", got)
	}
}

func TestParseKubectlTable(t *testing.T) {
	out := []byte(`Warning: something deprecated
NAME              READY   ISSUER        AGE
example-com-tls   True    letsencrypt   5d
api-tls           False   letsencrypt   12h
`)
	headers, rows, ok := ParseKubectlTable(out)
	if !ok || strings.Join(headers, " ") != "NAME READY ISSUER AGE" {
		t.Fatalf("headers %v ok %v", headers, ok)
	}
	if len(rows) != 2 || rows[0][1] != "True" || rows[1][0] != "api-tls" {
		t.Fatalf("rows %v", rows)
	}
	if _, _, ok := ParseKubectlTable([]byte("No resources found in shop namespace.\n")); ok {
		t.Fatal("empty list must not claim a table")
	}
}

func TestParseKubectlTableAllNamespaces(t *testing.T) {
	out := []byte(`NAMESPACE   NAME              READY   AGE
shop        example-com-tls   True    5d
kube-system root-ca           True    100d
`)
	headers, rows, ok := ParseKubectlTable(out)
	if !ok || headers[0] != "NAMESPACE" || rows[0][0] != "shop" || rows[1][1] != "root-ca" {
		t.Fatalf("%v %v", headers, rows)
	}
}

func TestTableFromPrintedMapsIdentity(t *testing.T) {
	js := `{"items":[
		{"metadata":{"name":"example-com-tls","namespace":"shop","creationTimestamp":"2024-01-01T00:00:00Z"}},
		{"metadata":{"name":"api-tls","namespace":"shop","creationTimestamp":"2024-01-02T00:00:00Z","deletionTimestamp":"2024-01-03T00:00:00Z"}}
	]}`
	headers := []string{"NAMESPACE", "NAME", "READY", "AGE"}
	rows := [][]string{
		{"shop", "example-com-tls", "True", "5d"},
		{"shop", "api-tls", "False", "12h"},
	}
	tab := TableFromPrinted(objs(t, js), headers, rows)
	if strings.Join(tab.Headers, " ") != "NAMESPACE NAME READY AGE" {
		t.Fatalf("headers %v", tab.Headers)
	}
	if tab.Refs[0] != [2]string{"shop", "example-com-tls"} || tab.Refs[1][1] != "api-tls" {
		t.Fatalf("refs %v", tab.Refs)
	}
	if !strings.Contains(tab.Rows[1][1], "terminating") {
		t.Fatalf("terminating %v", tab.Rows[1])
	}
	if tab.ColorCol != 2 || tab.Born[0].IsZero() {
		t.Fatalf("color %d born %v", tab.ColorCol, tab.Born)
	}
}

func TestGenericTablePrefersServerOverCRD(t *testing.T) {
	printers.mu.Lock()
	printers.byKey = map[string][]PrinterColumn{}
	printers.mu.Unlock()
	f := &shell.Fake{Handler: func(r shell.Request) shell.Result {
		line := strings.Join(append([]string{r.Name}, r.Args...), " ")
		if strings.Contains(line, "get crd ") {
			return shell.Fail(1, `Error from server (Forbidden): customresourcedefinitions.apiextensions.k8s.io is forbidden`)
		}
		if strings.Contains(line, "get certificates.cert-manager.io") && !strings.Contains(line, "-o") {
			return shell.OK("NAME              READY   AGE\nexample-com-tls   True    5d\n")
		}
		return shell.Fail(1, "unexpected "+line)
	}}
	c := New(f)
	o := objs(t, `{"items":[{"metadata":{"name":"example-com-tls","namespace":"shop","creationTimestamp":"2024-01-01T00:00:00Z"}}]}`)
	tab := c.GenericTable("/kc", "dev", "certificates.cert-manager.io", "shop", o)
	if strings.Join(tab.Headers, " ") != "NAME READY AGE" || tab.Rows[0][1] != "True" {
		t.Fatalf("server table: %v %v", tab.Headers, tab.Rows)
	}
	for _, call := range f.Calls() {
		if strings.Contains(call.Line(), "get crd") {
			t.Fatal("must not need CRD when server table works")
		}
	}
}

func TestGenericTableNonCRDServerColumns(t *testing.T) {
	printers.mu.Lock()
	printers.byKey = map[string][]PrinterColumn{}
	printers.mu.Unlock()
	f := &shell.Fake{Handler: func(r shell.Request) shell.Result {
		line := r.Line()
		if strings.Contains(line, "get services") && !strings.Contains(line, "-o") {
			return shell.OK("NAME   TYPE        CLUSTER-IP     EXTERNAL-IP   PORT(S)   AGE\nweb    ClusterIP   10.96.0.10     <none>        80/TCP    3d\n")
		}
		if strings.Contains(line, "get crd") {
			return shell.Fail(1, "no")
		}
		return shell.Fail(1, "unexpected "+line)
	}}
	c := New(f)
	o := objs(t, `{"items":[{"metadata":{"name":"web","namespace":"shop","creationTimestamp":"2024-01-01T00:00:00Z"}}]}`)
	tab := c.GenericTable("/kc", "dev", "services", "shop", o)
	want := "NAME TYPE CLUSTER-IP EXTERNAL-IP PORT(S) AGE"
	if strings.Join(tab.Headers, " ") != want || tab.Rows[0][1] != "ClusterIP" || tab.Refs[0][1] != "web" {
		t.Fatalf("%v %v refs %v", tab.Headers, tab.Rows, tab.Refs)
	}
}

func TestGenericTableCRDFallbackWhenPrintFails(t *testing.T) {
	printers.mu.Lock()
	printers.byKey = map[string][]PrinterColumn{}
	printers.mu.Unlock()
	f := &shell.Fake{Handler: func(r shell.Request) shell.Result {
		line := r.Line()
		if strings.Contains(line, "get certificates") && !strings.Contains(line, "crd") && !strings.Contains(line, "-o") {
			return shell.Fail(1, "error: the server does not allow this method on the requested resource")
		}
		if strings.Contains(line, "get crd certificates.cert-manager.io") {
			return shell.OK(`{"spec":{"versions":[{"name":"v1","storage":true,"additionalPrinterColumns":[
				{"name":"Ready","type":"string","jsonPath":".status.conditions[?(@.type==\"Ready\")].status"},
				{"name":"Age","type":"date","jsonPath":".metadata.creationTimestamp"}]}]}}`)
		}
		return shell.Fail(1, "unexpected "+line)
	}}
	c := New(f)
	o := objs(t, `{"items":[{"metadata":{"name":"web","creationTimestamp":"2024-01-01T00:00:00Z"},
		"status":{"conditions":[{"type":"Ready","status":"True"}]}}]}`)
	tab := c.GenericTable("/kc", "dev", "certificates.cert-manager.io", "shop", o)
	if strings.Join(tab.Headers, " ") != "NAME READY AGE" || tab.Rows[0][1] != "True" {
		t.Fatalf("crd fallback: %v %v", tab.Headers, tab.Rows)
	}
}

func TestGenericTableFallsBackToNameAge(t *testing.T) {
	printers.mu.Lock()
	printers.byKey = map[string][]PrinterColumn{}
	printers.mu.Unlock()
	f := &shell.Fake{Handler: func(r shell.Request) shell.Result {
		return shell.Fail(1, "boom")
	}}
	c := New(f)
	o := objs(t, `{"items":[{"metadata":{"name":"x","creationTimestamp":"2024-01-01T00:00:00Z"}}]}`)
	tab := c.GenericTable("/kc", "dev", "widgets.example.com", "shop", o)
	if strings.Join(tab.Headers, " ") != "NAME AGE" {
		t.Fatalf("%v", tab.Headers)
	}
}

func TestCustomSpecsStillWin(t *testing.T) {
	js := `{"items":[{"metadata":{"name":"web"},"status":{"readyReplicas":2,"replicas":3,"updatedReplicas":3,"availableReplicas":2}}]}`
	tab := TableForColumns("deploy", objs(t, js), false, []PrinterColumn{{Name: "X", JSONPath: ".x"}})
	if strings.Join(tab.Headers, " ") != "NAME READY UP-TO-DATE AVAILABLE AGE" {
		t.Fatalf("custom specs must win: %v", tab.Headers)
	}
}

func TestResourceTableUsesPrinterColumns(t *testing.T) {
	js := `{"items":[
		{"metadata":{"name":"web","namespace":"shop","creationTimestamp":"2024-01-01T00:00:00Z"},
			"spec":{"issuerRef":{"name":"le"}},
			"status":{"conditions":[{"type":"Ready","status":"True"}]}},
		{"metadata":{"name":"api","namespace":"shop","creationTimestamp":"2024-01-01T00:00:00Z"},
			"spec":{"issuerRef":{"name":"le"}},
			"status":{"conditions":[{"type":"Ready","status":"False"}]}}
	]}`
	cols := []PrinterColumn{
		{Name: "READY", Type: "string", JSONPath: `.status.conditions[?(@.type=="Ready")].status`},
		{Name: "ISSUER", Type: "string", JSONPath: ".spec.issuerRef.name"},
	}
	tab := TableForColumns("certificates.cert-manager.io", objs(t, js), false, cols)
	if strings.Join(tab.Headers, " ") != "NAME READY ISSUER AGE" {
		t.Fatalf("headers %v", tab.Headers)
	}
	by := map[string][]string{}
	for _, r := range tab.Rows {
		by[r[0]] = r
	}
	if by["web"][1] != "True" || by["api"][1] != "False" || by["web"][2] != "le" {
		t.Fatalf("rows %v", tab.Rows)
	}
}

func TestClientPrinterColumns(t *testing.T) {
	printers.mu.Lock()
	printers.byKey = map[string][]PrinterColumn{}
	printers.mu.Unlock()
	f := &shell.Fake{Handler: func(r shell.Request) shell.Result {
		line := strings.Join(append([]string{r.Name}, r.Args...), " ")
		if strings.Contains(line, "get crd certificates.cert-manager.io") {
			return shell.OK(`{"spec":{"versions":[{"name":"v1","storage":true,"additionalPrinterColumns":[
				{"name":"Ready","type":"string","jsonPath":".status.conditions[?(@.type==\"Ready\")].status"},
				{"name":"Age","type":"date","jsonPath":".metadata.creationTimestamp"}]}]}}`)
		}
		return shell.Fail(1, "no")
	}}
	c := New(f)
	cols := c.PrinterColumns("/kc", "dev", "certificates.cert-manager.io")
	if len(cols) != 1 || cols[0].Name != "READY" {
		t.Fatalf("%+v", cols)
	}
	again := c.PrinterColumns("/kc", "dev", "certificates.cert-manager.io")
	if len(again) != 1 || len(f.Calls()) != 1 {
		t.Fatalf("cache: cols=%v calls=%d", again, len(f.Calls()))
	}
}

func TestServerTableIgnoresWideOnlyNoise(t *testing.T) {
	out := []byte("NAME   DATA   AGE\ncfg    2      1d\n")
	headers, rows, ok := ParseKubectlTable(out)
	if !ok || strings.Join(headers, " ") != "NAME DATA AGE" || rows[0][1] != "2" {
		t.Fatalf("%v %v", headers, rows)
	}
}
