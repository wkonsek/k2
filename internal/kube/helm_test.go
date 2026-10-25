package kube

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func helmPayload(t *testing.T, rel map[string]any) string {
	t.Helper()
	js, _ := json.Marshal(rel)
	var gz bytes.Buffer
	w := gzip.NewWriter(&gz)
	w.Write(js)
	w.Close()
	stored := base64.StdEncoding.EncodeToString(gz.Bytes())
	return base64.StdEncoding.EncodeToString([]byte(stored))
}

func release(name, ns string, rev int, status, deployed string) map[string]any {
	return map[string]any{
		"name": name, "namespace": ns, "version": rev,
		"info": map[string]any{"status": status, "last_deployed": deployed, "description": "Upgrade complete", "notes": "visit http://" + name},
		"chart": map[string]any{
			"metadata": map[string]any{"name": "nginx", "version": fmt.Sprintf("1.%d.0", rev), "appVersion": "1.25"},
			"values":   map[string]any{"replicas": 1, "image": map[string]any{"repo": "nginx", "tag": "1.25"}, "debug": true},
		},
		"config":   map[string]any{"replicas": 3, "image": map[string]any{"tag": "1.26"}, "debug": nil},
		"manifest": "---\nkind: Deployment\nmetadata:\n  name: " + name + "\n",
	}
}

func helmSecretsJSON(t *testing.T, revs ...map[string]any) string {
	var items []string
	for _, r := range revs {
		items = append(items, fmt.Sprintf(`{"kind":"Secret","type":"helm.sh/release.v1","metadata":{"name":"sh.helm.release.v1.%s.v%d","namespace":%q,"labels":{"owner":"helm","name":%q,"version":"%d","status":%q}},"data":{"release":%q}}`,
			r["name"], r["version"], r["namespace"], r["name"], r["version"], r["info"].(map[string]any)["status"], helmPayload(t, r)))
	}
	return `{"kind":"List","items":[` + strings.Join(items, ",") + `]}`
}

func TestDecodeHelmRelease(t *testing.T) {
	r, err := DecodeHelmRelease(helmPayload(t, release("web", "shop", 4, "deployed", "2026-10-23T08:00:00.123Z")))
	if err != nil {
		t.Fatal(err)
	}
	if r.Name != "web" || r.Revision != 4 || r.Chart != "nginx-1.4.0" || r.AppVersion != "1.25" || r.Status != "deployed" || r.Updated.IsZero() || !strings.Contains(r.Manifest, "kind: Deployment") || r.Notes != "visit http://web" {
		t.Fatalf("%+v", r)
	}

	js, _ := json.Marshal(release("x", "ns", 1, "failed", ""))
	plain := base64.StdEncoding.EncodeToString([]byte(base64.StdEncoding.EncodeToString(js)))
	if r, err := DecodeHelmRelease(plain); err != nil || r.Name != "x" {
		t.Fatalf("plain: %v %+v", err, r)
	}
	for _, bad := range []string{"%%%", base64.StdEncoding.EncodeToString([]byte("bm90IGpzb24="))} {
		if _, err := DecodeHelmRelease(bad); err == nil {
			t.Errorf("%q decoded", bad)
		}
	}
}

func TestHelmLatestHistoryTable(t *testing.T) {
	objs, err := ParseObjects([]byte(helmSecretsJSON(t,
		release("web", "shop", 1, "superseded", "2026-10-20T08:00:00Z"),
		release("web", "shop", 2, "deployed", "2026-10-21T08:00:00Z"),
		release("api", "shop", 1, "failed", "2026-10-22T08:00:00Z"),
		release("web", "lab", 7, "deployed", "2026-10-22T08:00:00Z"),
	)))
	if err != nil {
		t.Fatal(err)
	}
	rs := HelmLatest(objs)
	if len(rs) != 3 || rs[0].Namespace != "lab" || rs[1].Name != "api" || rs[2].Name != "web" || rs[2].Revision != 2 {
		t.Fatalf("latest %+v", rs)
	}
	h := HelmHistory(objs, "shop", "web")
	if len(h) != 2 || h[0].Revision != 2 || h[1].Status != "superseded" {
		t.Fatalf("history %+v", h)
	}
	if txt := HistoryText(h); !strings.Contains(txt, "REVISION") || !strings.Contains(txt, "superseded") {
		t.Fatal(txt)
	}
	tab := HelmTable(rs, true)
	if strings.Join(tab.Headers, ",") != "NAMESPACE,NAME,REVISION,STATUS,CHART,APP VERSION,UPDATED" || tab.Rows[2][3] != "deployed" || tab.Rows[2][4] != "nginx-1.2.0" || tab.Refs[2] != [2]string{"shop", "web"} {
		t.Fatalf("table %+v", tab)
	}
}

func TestMergeValues(t *testing.T) {
	r := release("web", "shop", 1, "deployed", "")
	rel, _ := DecodeHelmRelease(helmPayload(t, r))
	user := ValuesYAML(rel.Config)
	if !strings.Contains(user, "replicas: 3") || !strings.Contains(user, "tag: \"1.26\"") {
		t.Fatalf("user values:\n%s", user)
	}
	all := ValuesYAML(MergeValues(rel.Defaults, rel.Config))
	for _, want := range []string{"replicas: 3", "repo: nginx", "tag: \"1.26\""} {
		if !strings.Contains(all, want) {
			t.Fatalf("computed lacks %q:\n%s", want, all)
		}
	}
	if strings.Contains(all, "debug") {
		t.Fatal("a user null must remove the key")
	}
	if ValuesYAML(nil) != "{}\n" {
		t.Fatal("empty values")
	}
}
