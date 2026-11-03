package kube

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"k2/internal/shell"
)

const gatewayDiscovery = `pods                               po           v1                             true   Pod
events                             ev           v1                             true   Event
events                             ev           events.k8s.io/v1               true   Event
deployments                        deploy       apps/v1                        true   Deployment
gateways                           gw           networking.istio.io/v1         true   Gateway
virtualservices                    vs           networking.istio.io/v1         true   VirtualService
gatewayclasses                     gc           gateway.networking.k8s.io/v1   false  GatewayClass
gateways                           gtw          gateway.networking.k8s.io/v1   true   Gateway
httproutes                                      gateway.networking.k8s.io/v1   true   HTTPRoute
`

func TestParseAPIList(t *testing.T) {
	list := ParseAPIList(gatewayDiscovery)
	if len(list) != 9 {
		t.Fatalf("parsed %d", len(list))
	}
	gc := list[6]
	if gc.Name != "gatewayclasses" || gc.Group != "gateway.networking.k8s.io" || gc.Version != "v1" || gc.Namespaced || gc.Kind != "GatewayClass" || !reflect.DeepEqual(gc.Short, []string{"gc"}) {
		t.Fatalf("gatewayclasses %+v", gc)
	}
	if hr := list[8]; hr.Name != "httproutes" || hr.Short != nil || !hr.Namespaced {
		t.Fatalf("httproutes %+v", hr)
	}
	if p := list[0]; p.Group != "" || p.Version != "v1" || p.GVR() != "v1/pods" || p.Qualified() != "pods" {
		t.Fatalf("pods %+v", p)
	}
}

func TestResourceNamesOfferGroupForms(t *testing.T) {
	names := strings.Join(ResourceNames(ParseAPIList(gatewayDiscovery)), " ")
	for _, want := range []string{"gateways", "gw", "gtw", "gateways.networking.istio.io", "gateways.gateway.networking.k8s.io",
		"gateway.networking.k8s.io/v1/gatewayclasses", "networking.istio.io/v1/gateways"} {
		if !strings.Contains(" "+names+" ", " "+want+" ") {
			t.Errorf("missing %q in %s", want, names)
		}
	}
	if strings.Contains(names, "v1/pods") {
		t.Errorf("core group form offered: %s", names)
	}
}

func TestResolve(t *testing.T) {
	list := ParseAPIList(gatewayDiscovery)
	cases := map[string][]string{
		"gateways":                                    {"gateways.gateway.networking.k8s.io", "gateways.networking.istio.io"},
		"gateway":                                     {"gateways.gateway.networking.k8s.io", "gateways.networking.istio.io"},
		"Gateways.networking.istio.io":                {"gateways.networking.istio.io"},
		"gateway.networking.istio.io":                 {"gateways.networking.istio.io"},
		"gateways.gateway.networking.k8s.io":          {"gateways.gateway.networking.k8s.io"},
		"gateway.networking.k8s.io/v1/gatewayclasses": {"gatewayclasses.v1.gateway.networking.k8s.io"},
		"gateway.networking.k8s.io/gatewayclasses":    {"gatewayclasses.gateway.networking.k8s.io"},
		"gatewayclasses.v1.gateway.networking.k8s.io": {"gatewayclasses.v1.gateway.networking.k8s.io"},
		"networking.istio.io/v1/gw":                   {"gateways.v1.networking.istio.io"},
		"gtw":                                         {"gtw"},
		"httproutes":                                  {"httproutes"},
		"events":                                      {"events"},
		"deploy.apps":                                 {"deployments.apps"},
		"v1/pods":                                     {"pods"},
		"gateway.networking.k8s.io/v1beta1/gateways":  {"gateways.v1beta1.gateway.networking.k8s.io"},
		"gateways.example.io":                         nil,
		"widgets":                                     nil,
		"a/b/c/d":                                     nil,
	}
	for in, want := range cases {
		if got := Resolve(in, list); !reflect.DeepEqual(got, want) {
			t.Errorf("Resolve(%q) = %v, want %v", in, got, want)
		}
	}
	offline := map[string]string{
		"gateways.networking.istio.io":                "gateways.networking.istio.io",
		"gateway.networking.k8s.io/v1/gatewayclasses": "gatewayclasses.v1.gateway.networking.k8s.io",
		"apps/deployments":                            "deployments.apps",
	}
	for in, want := range offline {
		if got := Resolve(in, nil); len(got) != 1 || got[0] != want {
			t.Errorf("offline Resolve(%q) = %v, want %s", in, got, want)
		}
	}
	if got := Resolve("gateways", nil); got != nil {
		t.Errorf("plain name without discovery = %v", got)
	}
}

func TestIsVersion(t *testing.T) {
	for in, want := range map[string]bool{"v1": true, "v1beta1": true, "v2alpha3": true, "v1beta": false, "vx": false, "apps": false, "v": false} {
		if IsVersion(in) != want {
			t.Errorf("IsVersion(%q) != %v", in, want)
		}
	}
}

func TestAPIListKeepsPartialDiscovery(t *testing.T) {
	c := Client{Run: &shell.Fake{Handler: func(shell.Request) shell.Result {
		return shell.Result{Stdout: []byte(gatewayDiscovery), Code: 1, Err: errors.New("exit status 1")}
	}}}
	list, err := c.APIList("kc", "ctx")
	if err != nil || len(list) != 9 {
		t.Fatalf("partial discovery dropped: %d %v", len(list), err)
	}
	c.Run = &shell.Fake{Handler: func(shell.Request) shell.Result { return shell.Fail(1, "forbidden") }}
	if _, err := c.APIList("kc", "ctx"); err == nil {
		t.Fatal("empty failing discovery should error")
	}
}
