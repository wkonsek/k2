package kube

import (
	"reflect"
	"strings"
	"testing"
)

func TestGatewayAPITable(t *testing.T) {
	o := objs(t, `{"items":[{"apiVersion":"gateway.networking.k8s.io/v1","kind":"Gateway",
		"metadata":{"name":"main-gw","namespace":"edge"},
		"spec":{"gatewayClassName":"openshift-default"},
		"status":{"addresses":[{"value":"10.0.40.12"},{"value":"10.0.40.13"}],
			"conditions":[{"type":"Accepted","status":"True"},{"type":"Programmed","status":"True"}]}},
		{"apiVersion":"gateway.networking.k8s.io/v1","kind":"Gateway","metadata":{"name":"internal-gw","namespace":"edge"},
		"spec":{"gatewayClassName":"openshift-default"}}]}`)
	tb := TableFor("gateways", o, false)
	if !reflect.DeepEqual(tb.Headers, []string{"NAME", "CLASS", "ADDRESS", "PROGRAMMED", "AGE"}) {
		t.Fatalf("headers %v", tb.Headers)
	}
	if r := tb.Rows[1]; !reflect.DeepEqual(r[:4], []string{"main-gw", "openshift-default", "10.0.40.12,10.0.40.13", "True"}) {
		t.Fatalf("row %v", r)
	}
	if r := tb.Rows[0]; !reflect.DeepEqual(r[:4], []string{"internal-gw", "openshift-default", "-", "-"}) {
		t.Fatalf("row %v", r)
	}
	if all := TableFor("gateways.gateway.networking.k8s.io", o, true); all.Headers[0] != "NAMESPACE" || all.Headers[2] != "CLASS" {
		t.Fatalf("all namespaces %v", all.Headers)
	}
}

func TestIstioGatewayTable(t *testing.T) {
	o := objs(t, `{"items":[{"apiVersion":"networking.istio.io/v1","kind":"Gateway",
		"metadata":{"name":"ingress","namespace":"gateway-system"},
		"spec":{"selector":{"istio":"ingressgateway","app":"edge"},
			"servers":[{"hosts":["shop.example.com","api.example.com"]},{"hosts":["shop.example.com"]}]}}]}`)
	tb := TableFor("gateways", o, false)
	if !reflect.DeepEqual(tb.Headers, []string{"NAME", "SELECTOR", "HOSTS", "AGE"}) {
		t.Fatalf("headers %v", tb.Headers)
	}
	if r := tb.Rows[0]; !reflect.DeepEqual(r[:3], []string{"ingress", "app=edge,istio=ingressgateway", "shop.example.com,api.example.com"}) {
		t.Fatalf("row %v", r)
	}
}

func TestGroupTablesWhenEmptyOrUnknown(t *testing.T) {
	for kind, want := range map[string]string{
		"gateways.networking.istio.io":                "NAME SELECTOR HOSTS AGE",
		"gateways.v1.gateway.networking.k8s.io":       "NAME CLASS ADDRESS PROGRAMMED AGE",
		"gatewayclasses.v1.gateway.networking.k8s.io": "NAME CONTROLLER ACCEPTED AGE",
		"httproutes":          "NAME HOSTNAMES PARENTS AGE",
		"gateways":            "NAME AGE",
		"widgets.example.com": "NAME AGE",
	} {
		if got := strings.Join(TableFor(kind, nil, false).Headers, " "); got != want {
			t.Errorf("%s: %q, want %q", kind, got, want)
		}
	}
	o := objs(t, `{"items":[{"apiVersion":"example.com/v1","kind":"Gateway","metadata":{"name":"x"}}]}`)
	if got := strings.Join(TableFor("gateways", o, false).Headers, " "); got != "NAME AGE" {
		t.Errorf("other group gateway %q", got)
	}
}

func TestParseObjectsSkipsWarningsAndShowsErrors(t *testing.T) {
	o, err := ParseObjects([]byte("Warning: networking.istio.io/v1alpha3 Gateway is deprecated\n{\"items\":[{\"metadata\":{\"name\":\"a\"}}]}"))
	if err != nil || len(o) != 1 {
		t.Fatalf("warning prefix: %v %v", o, err)
	}
	if _, err := ParseObjects([]byte(`error: the server doesn't have a resource type "gateways"`)); err == nil || !strings.Contains(err.Error(), "doesn't have a resource type") {
		t.Fatalf("kubectl error hidden: %v", err)
	}
}
