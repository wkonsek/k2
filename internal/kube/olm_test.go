package kube

import (
	"strings"
	"testing"
)

const olmSubs = `{"items":[{"metadata":{"name":"etcd","namespace":"operators"},
 "spec":{"name":"etcd","channel":"stable","source":"community","sourceNamespace":"olm"},
 "status":{"installedCSV":"etcd.v0.9.4","currentCSV":"etcd.v0.9.5","state":"UpgradePending"}}]}`

const olmCSV = `{"items":[{"metadata":{"name":"etcd.v0.9.4","namespace":"operators"},
 "spec":{"displayName":"etcd","version":"0.9.4","replaces":"etcd.v0.9.2",
  "installModes":[{"type":"OwnNamespace","supported":true},{"type":"AllNamespaces","supported":false}],
  "customresourcedefinitions":{"owned":[{"name":"etcdclusters.etcd.database.coreos.com","kind":"EtcdCluster","version":"v1beta2"}],
   "required":[{"name":"backups.x.io","kind":"Backup","version":"v1"}]},
  "install":{"spec":{"deployments":[{"name":"etcd-operator"}]}}},
 "status":{"phase":"Succeeded","reason":"InstallSucceeded","conditions":[{"phase":"Succeeded","reason":"InstallSucceeded","message":"done","lastTransitionTime":"2026-10-22T10:00:00Z"}]}}]}`

func TestOLMTables(t *testing.T) {
	sub := TableFor("subscriptions.operators.coreos.com", objs(t, olmSubs), false)
	if strings.Join(sub.Headers, "|") != "NAME|PACKAGE|CHANNEL|SOURCE|INSTALLED CSV|CURRENT CSV|STATE|AGE" ||
		strings.Join(sub.Rows[0][:7], "|") != "etcd|etcd|stable|community|etcd.v0.9.4|etcd.v0.9.5|UpgradePending" ||
		sub.ColorCol != 6 || sub.Extra[0]["csv"] != "etcd.v0.9.4" {
		t.Fatalf("sub %v %v %d", sub.Headers, sub.Rows, sub.ColorCol)
	}
	csv := TableFor("csv", objs(t, olmCSV), false)
	if strings.Join(csv.Headers, "|") != "NAME|DISPLAY|VERSION|REPLACES|PHASE|AGE" ||
		strings.Join(csv.Rows[0][:5], "|") != "etcd.v0.9.4|etcd|0.9.4|etcd.v0.9.2|Succeeded" || csv.ColorCol != 4 {
		t.Fatalf("csv %v %v", csv.Headers, csv.Rows)
	}
	ip := TableFor("ip", objs(t, `{"items":[{"metadata":{"name":"install-abc","namespace":"operators"},
	 "spec":{"clusterServiceVersionNames":["etcd.v0.9.5","dep.v1"],"approval":"Manual","approved":false},
	 "status":{"phase":"RequiresApproval"}}]}`), false)
	if strings.Join(ip.Headers, "|") != "NAME|CSV|APPROVAL|APPROVED|PHASE|AGE" ||
		strings.Join(ip.Rows[0][:5], "|") != "install-abc|etcd.v0.9.5,dep.v1|Manual|false|RequiresApproval" ||
		ip.Extra[0]["approval"] != "Manual" || ip.Extra[0]["approved"] != "false" {
		t.Fatalf("ip %v %v", ip.Rows, ip.Extra)
	}
	cs := TableFor("catalogsources", objs(t, `{"items":[{"metadata":{"name":"community","namespace":"olm"},
	 "spec":{"displayName":"Community","sourceType":"grpc","publisher":"OperatorHub.io"},
	 "status":{"connectionState":{"lastObservedState":"TRANSIENT_FAILURE"}}}]}`), false)
	if strings.Join(cs.Rows[0][:5], "|") != "community|Community|grpc|OperatorHub.io|TRANSIENT_FAILURE" || cs.Headers[4] != "CONNECTION" || cs.ColorCol != 4 {
		t.Fatalf("catsrc %v %v", cs.Headers, cs.Rows)
	}
	og := TableFor("og", objs(t, `{"items":[{"metadata":{"name":"global"},"spec":{}},{"metadata":{"name":"own"},"spec":{"targetNamespaces":["a","b"]}}]}`), false)
	if og.Rows[0][1] != "(all)" || og.Rows[1][1] != "a,b" || og.Rows[0][2] != "Default" {
		t.Fatalf("og %v", og.Rows)
	}
	crd := TableFor("crd", objs(t, `{"items":[{"metadata":{"name":"widgets.example.com"},
	 "spec":{"group":"example.com","scope":"Namespaced","names":{"kind":"Widget","plural":"widgets"},
	  "versions":[{"name":"v1","served":true,"storage":true},{"name":"v1beta1","served":true},{"name":"v1alpha1","served":false}]}}]}`), false)
	if strings.Join(crd.Headers, "|") != "NAME|GROUP|KIND|VERSIONS|SCOPE|AGE" ||
		strings.Join(crd.Rows[0][:5], "|") != "widgets.example.com|example.com|Widget|v1*,v1beta1|Namespaced" ||
		crd.Extra[0]["resource"] != "widgets.example.com" {
		t.Fatalf("crd %v %v", crd.Rows, crd.Extra)
	}
	rt := TableFor("routes", objs(t, `{"items":[
	 {"metadata":{"name":"web"},"spec":{"host":"web.apps.example.com","path":"/api","to":{"name":"web"},"port":{"targetPort":"http"},
	  "tls":{"termination":"edge","insecureEdgeTerminationPolicy":"Redirect"}}},
	 {"metadata":{"name":"plain"},"spec":{"host":"plain.apps.example.com","to":{"name":"plain"},"port":{"targetPort":8080}}}]}`), false)
	if strings.Join(rt.Headers, "|") != "NAME|HOST|PATH|SERVICE|PORT|TLS|AGE" ||
		strings.Join(rt.Rows[1][:6], "|") != "web|https://web.apps.example.com/api|/api|web|http|edge/Redirect" ||
		strings.Join(rt.Rows[0][:6], "|") != "plain|http://plain.apps.example.com|-|plain|8080|-" {
		t.Fatalf("routes %v", rt.Rows)
	}
}

func TestCSVSummary(t *testing.T) {
	s := CSVSummary(objs(t, olmCSV)[0])
	for _, want := range []string{"phase: Succeeded", "reason: InstallSucceeded", "replaces: etcd.v0.9.2",
		"install modes: OwnNamespace", "owned CRDs (1)", "etcdclusters.etcd.database.coreos.com  (EtcdCluster v1beta2)",
		"required CRDs (1)", "deployments (1)", "  etcd-operator", "conditions (1)"} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in\n%s", want, s)
		}
	}
}
