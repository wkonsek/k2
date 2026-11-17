package kube

import (
	"strings"
	"testing"
)

func TestParseHelmSearch(t *testing.T) {
	out := `NAME                   	CHART VERSION	APP VERSION	DESCRIPTION
kafbat-ui/kafka-ui     	1.4.0        	v1.2.0     	A UI for Kafka
kafbat-ui/kafka-ui     	1.3.1        	v1.1.0     	A UI for Kafka
bitnami/nginx          	18.2.0       	1.27.0     	nginx
`
	cs := ParseHelmSearch(out)
	if len(cs) != 3 || cs[0].Name != "kafbat-ui/kafka-ui" || cs[0].Version != "1.4.0" || cs[2].Name != "bitnami/nginx" {
		t.Fatalf("%+v", cs)
	}
	names := ChartNames(cs)
	if strings.Join(names, " ") != "bitnami/nginx kafbat-ui/kafka-ui" {
		t.Fatalf("%v", names)
	}
	vers := ChartVersions(cs, "kafbat-ui/kafka-ui")
	if strings.Join(vers, " ") != "1.4.0 1.3.1" {
		t.Fatalf("%v", vers)
	}
}

func TestManifestKindCounts(t *testing.T) {
	man := `---
kind: Service
metadata:
  name: ui
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: ui
---
kind: Deployment
metadata:
  name: worker
`
	if got := ManifestKindCounts(man); got != "Service ×1 · Deployment ×2" {
		t.Fatalf("%q", got)
	}
	if ManifestKindCounts("") != "no resources" {
		t.Fatal("empty")
	}
}

func TestHelmApplyArgs(t *testing.T) {
	got := strings.Join(HelmApplyArgs(false, "ui", "kafbat-ui/kafka-ui", "1.4.0", "kafka", true, "values.yaml", "/kc", "dev"), " ")
	want := "install ui kafbat-ui/kafka-ui -n kafka --kubeconfig /kc --kube-context dev --version 1.4.0 -f values.yaml --create-namespace"
	if got != want {
		t.Fatalf("%q", got)
	}
	up := strings.Join(HelmApplyArgs(true, "ui", "kafbat-ui/kafka-ui", "", "kafka", true, "", "/kc", "dev"), " ")
	if !strings.HasPrefix(up, "upgrade ui ") || strings.Contains(up, "--create-namespace") || strings.Contains(up, "--version") || strings.Contains(up, "-f ") {
		t.Fatalf("upgrade %q", up)
	}
	tpl := strings.Join(HelmTemplateArgs("ui", "kafbat-ui/kafka-ui", "1.4.0", "kafka", "v.yaml", "/kc", "dev"), " ")
	if !strings.HasPrefix(tpl, "template ui ") || !strings.Contains(tpl, "--version 1.4.0") {
		t.Fatalf("template %q", tpl)
	}
}

func TestUpgradeDiffUsesManifest(t *testing.T) {
	cur := HelmRelease{Name: "ui", Namespace: "kafka", Revision: 1, Chart: "kafka-ui-1.3.1", Manifest: "---\nkind: Service\nmetadata:\n  name: ui\n"}
	d := UpgradeDiff(cur, "---\nkind: Service\nmetadata:\n  name: ui\n---\nkind: Deployment\nmetadata:\n  name: ui\n", "kafbat-ui/kafka-ui", "1.4.0")
	if !strings.Contains(d, "Deployment/ui") || !strings.Contains(d, "added") {
		t.Fatalf("%s", d)
	}
}
