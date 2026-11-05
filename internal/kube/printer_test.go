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
	if tab.ColorCol != 1 {
		t.Fatalf("color %d", tab.ColorCol)
	}
}

func TestGenericFallbackWithoutColumns(t *testing.T) {
	js := `{"items":[{"metadata":{"name":"x","creationTimestamp":"2024-01-01T00:00:00Z"}}]}`
	if g := TableForColumns("widgets.example.com", objs(t, js), false, nil); strings.Join(g.Headers, " ") != "NAME AGE" {
		t.Fatalf("%v", g.Headers)
	}
}

func TestClientPrinterColumns(t *testing.T) {
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

func TestAnotherCRDPrinterColumns(t *testing.T) {
	js := `{"items":[{"metadata":{"name":"gw","creationTimestamp":"2024-01-01T00:00:00Z"},
		"spec":{"address":"1.2.3.4"},"status":{"phase":"Ready"}}]}`
	cols := []PrinterColumn{
		{Name: "ADDRESS", JSONPath: ".spec.address"},
		{Name: "PHASE", JSONPath: ".status.phase"},
	}
	tab := TableForColumns("widgets.example.com", objs(t, js), false, cols)
	if strings.Join(tab.Headers, " ") != "NAME ADDRESS PHASE AGE" || tab.Rows[0][1] != "1.2.3.4" || tab.Rows[0][2] != "Ready" {
		t.Fatalf("%v %v", tab.Headers, tab.Rows)
	}
	if d := TableForColumns("deploy", objs(t, `{"items":[{"metadata":{"name":"web"},"status":{"readyReplicas":2,"replicas":3,"updatedReplicas":3,"availableReplicas":2}}]}`), false, cols); strings.Join(d.Headers, " ") != "NAME READY UP-TO-DATE AVAILABLE AGE" {
		t.Fatalf("custom specs must win: %v", d.Headers)
	}
}
