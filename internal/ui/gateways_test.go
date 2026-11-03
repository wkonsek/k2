package ui

import (
	"errors"
	"strings"
	"testing"

	"k2/internal/kube"
	"k2/internal/shell"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/x/ansi"
)

const gatewayAPIs = `pods                               po           v1                             true   Pod
gateways                           gw           networking.istio.io/v1         true   Gateway
virtualservices                    vs           networking.istio.io/v1         true   VirtualService
gatewayclasses                     gc           gateway.networking.k8s.io/v1   false  GatewayClass
gateways                           gtw          gateway.networking.k8s.io/v1   true   Gateway
httproutes                                      gateway.networking.k8s.io/v1   true   HTTPRoute
`

const istioGateways = `{"items":[{"apiVersion":"networking.istio.io/v1","kind":"Gateway",
	"metadata":{"name":"ingress","namespace":"edge","creationTimestamp":"2026-01-01T00:00:00Z"},
	"spec":{"selector":{"istio":"ingressgateway"},"servers":[{"hosts":["shop.example.com"]}]}}]}`

const apiGateways = `{"items":[{"apiVersion":"gateway.networking.k8s.io/v1","kind":"Gateway",
	"metadata":{"name":"main-gw","namespace":"edge","creationTimestamp":"2026-01-01T00:00:00Z"},
	"spec":{"gatewayClassName":"openshift-default"},
	"status":{"addresses":[{"value":"10.0.40.12"}],"conditions":[{"type":"Programmed","status":"True"}]}}]}`

func gatewayCluster(byKind map[string]shell.Result) *shell.Fake {
	return &shell.Fake{Handler: func(r shell.Request) shell.Result {
		if len(r.Args) > 0 && r.Args[0] == "api-resources" {
			return shell.OK(gatewayAPIs)
		}
		if len(r.Args) > 1 && r.Args[0] == "get" {
			if res, ok := byKind[r.Args[1]]; ok {
				return res
			}
			return shell.OK(`{"items":[]}`)
		}
		return shell.Fail(1, "not in this test")
	}}
}

func gatewayModel(t *testing.T, f *shell.Fake) model {
	t.Helper()
	if f == nil {
		f = gatewayCluster(nil)
	}
	m := newTestModel(t, testEnv(t), f)
	m.kubeconfig, m.context, m.namespace = "/kc", "kind-dev", "edge"
	m.apis = kube.ParseAPIList(gatewayAPIs)
	m.resources = kube.ResourceNames(m.apis)
	m.cmdOn = true
	m.cmd.SetValue("")
	m.cmd.Focus()
	return m
}

func TestGatewaysPickTheGroupThatHasObjects(t *testing.T) {
	f := gatewayCluster(map[string]shell.Result{"gateways.networking.istio.io": shell.OK(istioGateways)})
	m := open(t, gatewayModel(t, f), "gateways")
	if m.stage != "res" || m.kind != "gateways.networking.istio.io" {
		t.Fatalf("kind %q stage %q status %q", m.kind, m.stage, m.status)
	}
	if !strings.Contains(m.status, "also :gateways.gateway.networking.k8s.io") {
		t.Fatalf("status %q", m.status)
	}
	it := m.content.Items()[0].(item)
	if strings.Join(it.heads, " ") != "NAME SELECTOR HOSTS AGE" || it.cells[1] != "istio=ingressgateway" {
		t.Fatalf("row %v %v", it.heads, it.cells)
	}
	gets := calls(f, "get")
	if len(gets) != 2 || !strings.HasPrefix(gets[0], "get gateways.gateway.networking.k8s.io ") || !strings.HasPrefix(gets[1], "get gateways.networking.istio.io ") {
		t.Fatalf("gets %v", gets)
	}
}

func TestGatewaysPreferGatewayAPI(t *testing.T) {
	f := gatewayCluster(map[string]shell.Result{
		"gateways.gateway.networking.k8s.io": shell.OK(apiGateways),
		"gateways.networking.istio.io":       shell.OK(istioGateways),
	})
	m := open(t, gatewayModel(t, f), "gateways")
	if m.kind != "gateways.gateway.networking.k8s.io" || len(calls(f, "get")) != 1 {
		t.Fatalf("kind %q gets %v", m.kind, calls(f, "get"))
	}
	it := m.content.Items()[0].(item)
	if strings.Join(it.heads, " ") != "NAME CLASS ADDRESS PROGRAMMED AGE" || strings.Join(it.cells[:4], " ") != "main-gw openshift-default 10.0.40.12 True" {
		t.Fatalf("row %v %v", it.heads, it.cells)
	}
}

func TestQualifiedResourceForms(t *testing.T) {
	for in, want := range map[string]string{
		"gateways.networking.istio.io":                "gateways.networking.istio.io",
		"gateway.networking.k8s.io/v1/gatewayclasses": "gatewayclasses.v1.gateway.networking.k8s.io",
		"httproutes": "httproutes",
	} {
		f := gatewayCluster(nil)
		m := open(t, gatewayModel(t, f), in)
		gets := calls(f, "get")
		if m.kind != want || len(gets) != 1 || !strings.HasPrefix(gets[0], "get "+want+" ") {
			t.Errorf("%s: kind %q gets %v status %q", in, m.kind, gets, m.status)
		}
	}
	m := open(t, gatewayModel(t, nil), "widgets.example.com")
	if m.stage == "res" || !strings.Contains(m.status, "widgets.example.com is not installed") {
		t.Fatalf("missing group: %q", m.status)
	}
}

func TestKubectlErrorIsShownNotEmpty(t *testing.T) {
	f := gatewayCluster(map[string]shell.Result{
		"gateways.networking.istio.io": shell.Fail(1, `Error from server (Forbidden): gateways.networking.istio.io is forbidden: User "dev" cannot list resource "gateways"`),
	})
	m := open(t, gatewayModel(t, f), "gateways.networking.istio.io")
	if !strings.Contains(m.status, "Forbidden") || strings.Contains(m.status, "no gateways") {
		t.Fatalf("status %q", m.status)
	}
}

func TestFirstListing(t *testing.T) {
	one := []list.Item{item{id: "a"}}
	boom := errors.New("boom")
	get := func(res map[string]error, items map[string][]list.Item) func(string) (string, []list.Item, error) {
		return func(k string) (string, []list.Item, error) { return "h-" + k, items[k], res[k] }
	}
	kind, _, items, also, err := firstListing([]string{"a", "b", "c"}, get(map[string]error{"a": boom}, map[string][]list.Item{"c": one}))
	if kind != "c" || len(items) != 1 || err != nil || strings.Join(also, ",") != "a,b" {
		t.Fatalf("nonempty: %s %v %v %v", kind, items, also, err)
	}
	kind, _, _, _, err = firstListing([]string{"a", "b"}, get(map[string]error{"a": boom}, nil))
	if kind != "b" || err != nil {
		t.Fatalf("empty success: %s %v", kind, err)
	}
	kind, _, _, also, err = firstListing([]string{"a", "b"}, get(map[string]error{"a": boom, "b": boom}, nil))
	if kind != "a" || err != boom || strings.Join(also, ",") != "b" {
		t.Fatalf("errors: %s %v %v", kind, also, err)
	}
	kind, _, _, also, _ = firstListing([]string{"pods"}, get(nil, nil))
	if kind != "pods" || also != nil {
		t.Fatalf("single: %s %v", kind, also)
	}
}

func TestCommandGhostSuggestsGroupForms(t *testing.T) {
	m := typeText(t, gatewayModel(t, nil), "gateways")
	if got := ansi.Strip(m.cmd.View()); !strings.Contains(got, "gateways.gateway.networking.k8s.io") {
		t.Fatalf("ghost %q", got)
	}
	m = typeText(t, gatewayModel(t, nil), "gateways.n")
	if got := ansi.Strip(m.cmd.View()); !strings.Contains(got, "gateways.networking.istio.io") {
		t.Fatalf("istio ghost %q", got)
	}
	m = pressSettle(t, m, "right")
	if m.cmd.Value() != "gateways.networking.istio.io" {
		t.Fatalf("right accepted %q", m.cmd.Value())
	}
	m = typeText(t, gatewayModel(t, nil), "gateway")
	if got := ansi.Strip(m.cmd.View()); !strings.Contains(got, "gateway.networking.k8s.io/v1/gatewayclasses") {
		t.Fatalf("gvr ghost %q", got)
	}
	m = pressSettle(t, m, "right")
	if m.cmd.Value() != "gateway.networking.k8s.io/v1/gatewayclasses" {
		t.Fatalf("right accepted %q", m.cmd.Value())
	}
	m = pressSettle(t, m, "enter")
	if m.kind != "gatewayclasses.v1.gateway.networking.k8s.io" {
		t.Fatalf("opened %q (%s)", m.kind, m.status)
	}
	m = typeText(t, gatewayModel(t, nil), "zzz")
	if got := ansi.Strip(m.cmd.View()); strings.TrimSpace(strings.TrimPrefix(got, ":")) != "zzz" {
		t.Fatalf("no ghost expected %q", got)
	}
}

func TestCommandTabCyclesGroupForms(t *testing.T) {
	m := typeText(t, gatewayModel(t, nil), "gateways")
	m = pressSettle(t, m, "tab")
	if m.cmdHint != "gateways  gateways.gateway.networking.k8s.io  gateways.networking.istio.io" {
		t.Fatalf("hint %q", m.cmdHint)
	}
	m = pressSettle(t, m, "tab", "tab", "tab")
	if m.cmd.Value() != "gateways.networking.istio.io" {
		t.Fatalf("cycled %q", m.cmd.Value())
	}
}

func TestResourceListCarriesDiscovery(t *testing.T) {
	msg := fetchResources(kube.Client{Run: gatewayCluster(nil)}, "/kc", "kind-dev")().(resListMsg)
	if len(msg.apis) != 6 || !strings.Contains(strings.Join(msg.names, " "), "gateway.networking.k8s.io/v1/gatewayclasses") {
		t.Fatalf("apis %d names %v", len(msg.apis), msg.names)
	}
}
