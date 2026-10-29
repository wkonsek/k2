package main

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type revision struct {
	chartVersion string
	appVersion   string
	values       map[string]any
	ago          time.Duration
	desc         string
}

type release struct {
	ns, name, chart string
	defaults        map[string]any
	revs            []revision
}

func demoReleases(env string) []release {
	h := time.Hour
	d := 24 * time.Hour
	return []release{
		{ns: "shop", name: "web", chart: "web",
			defaults: map[string]any{"replicaCount": 1, "image": map[string]any{"repository": "nginx", "tag": "1.25-alpine", "pullPolicy": "IfNotPresent"},
				"service": map[string]any{"type": "ClusterIP", "port": 80}, "ingress": map[string]any{"enabled": false},
				"resources": map[string]any{"limits": map[string]any{"cpu": "200m", "memory": "128Mi"}}},
			revs: []revision{
				{"1.2.0", "1.25", map[string]any{"replicaCount": 2, "image": map[string]any{"tag": "1.25-alpine"}, "ingress": map[string]any{"enabled": true, "host": "shop." + env + ".example.com"}}, 41 * d, "Install complete"},
				{"1.3.0", "1.26", map[string]any{"replicaCount": 2, "image": map[string]any{"tag": "1.26-alpine"}, "ingress": map[string]any{"enabled": true, "host": "shop." + env + ".example.com"}}, 20 * d, "Upgrade complete"},
				{"1.3.1", "1.26", map[string]any{"replicaCount": 2, "image": map[string]any{"tag": "1.26-alpine"}, "ingress": map[string]any{"enabled": true, "host": "shop." + env + ".example.com", "tls": true}}, 6 * d, "Upgrade complete"},
				{"1.4.0", "1.27", map[string]any{"replicaCount": 2, "image": map[string]any{"tag": "1.27-alpine"}, "ingress": map[string]any{"enabled": true, "host": "shop." + env + ".example.com", "tls": true}, "featureFlags": map[string]any{"newCheckout": true}}, 13 * h, "Upgrade complete"},
			}},
		{ns: "shop", name: "redis", chart: "redis",
			defaults: map[string]any{"architecture": "replication", "auth": map[string]any{"enabled": true}, "master": map[string]any{"persistence": map[string]any{"size": "8Gi"}}},
			revs: []revision{
				{"20.1.3", "7.4.0", map[string]any{"architecture": "standalone", "master": map[string]any{"persistence": map[string]any{"size": "20Gi"}}}, 41 * d, "Install complete"},
				{"20.1.7", "7.4.1", map[string]any{"architecture": "standalone", "master": map[string]any{"persistence": map[string]any{"size": "20Gi"}}}, 9 * d, "Upgrade complete"},
			}},
		{ns: "monitoring", name: "kube-prometheus-stack", chart: "kube-prometheus-stack",
			defaults: map[string]any{"grafana": map[string]any{"enabled": true}, "alertmanager": map[string]any{"enabled": true}, "prometheus": map[string]any{"prometheusSpec": map[string]any{"retention": "10d"}}},
			revs: []revision{
				{"62.7.0", "v0.76.1", map[string]any{"prometheus": map[string]any{"prometheusSpec": map[string]any{"retention": "15d"}}}, 120 * d, "Install complete"},
				{"65.1.0", "v0.77.1", map[string]any{"prometheus": map[string]any{"prometheusSpec": map[string]any{"retention": "15d"}}, "alertmanager": map[string]any{"enabled": false}}, 8 * d, "Upgrade complete"},
			}},
		{ns: "argocd", name: "argo-cd", chart: "argo-cd",
			defaults: map[string]any{"server": map[string]any{"replicas": 1}, "dex": map[string]any{"enabled": true}},
			revs: []revision{
				{"7.6.8", "v2.12.4", map[string]any{"server": map[string]any{"replicas": 2}, "dex": map[string]any{"enabled": false}}, 17 * d, "Install complete"},
			}},
	}
}

func (r release) manifest(v revision) string {
	vals := mergeValues(r.defaults, v.values)
	var b strings.Builder
	switch r.name {
	case "web":
		img := get(vals, "image")
		fmt.Fprintf(&b, "---\n# Source: web/templates/service.yaml\napiVersion: v1\nkind: Service\nmetadata:\n  name: web\n  labels:\n    app.kubernetes.io/name: web\n    helm.sh/chart: web-%s\nspec:\n  type: ClusterIP\n  ports:\n    - port: 80\n      targetPort: http\n  selector:\n    app: web\n", v.chartVersion)
		fmt.Fprintf(&b, "---\n# Source: web/templates/deployment.yaml\napiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: web\n  labels:\n    app.kubernetes.io/name: web\n    helm.sh/chart: web-%s\nspec:\n  replicas: %v\n  selector:\n    matchLabels:\n      app: web\n  template:\n    metadata:\n      labels:\n        app: web\n    spec:\n      containers:\n        - name: nginx\n          image: \"%s:%s\"\n          ports:\n            - name: http\n              containerPort: 8080\n          resources:\n            limits:\n              cpu: 200m\n              memory: 128Mi\n",
			v.chartVersion, get(vals, "replicaCount"), str(img, "repository"), str(img, "tag"))
		if get(vals, "ingress", "enabled") == true {
			fmt.Fprintf(&b, "---\n# Source: web/templates/ingress.yaml\napiVersion: networking.k8s.io/v1\nkind: Ingress\nmetadata:\n  name: web\nspec:\n  ingressClassName: nginx\n")
			if get(vals, "ingress", "tls") == true {
				fmt.Fprintf(&b, "  tls:\n    - hosts:\n        - %s\n      secretName: web-tls\n", str(vals, "ingress", "host"))
			}
			fmt.Fprintf(&b, "  rules:\n    - host: %s\n      http:\n        paths:\n          - path: /\n            pathType: Prefix\n            backend:\n              service:\n                name: web\n                port:\n                  number: 80\n", str(vals, "ingress", "host"))
		}
		if get(vals, "featureFlags", "newCheckout") == true {
			fmt.Fprintf(&b, "---\n# Source: web/templates/configmap.yaml\napiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: web-flags\ndata:\n  FEATURE_NEW_CHECKOUT: \"true\"\n")
		}
	default:
		fmt.Fprintf(&b, "---\n# Source: %s/templates/statefulset.yaml\napiVersion: apps/v1\nkind: StatefulSet\nmetadata:\n  name: %s\n  labels:\n    helm.sh/chart: %s-%s\n    app.kubernetes.io/version: %q\nspec:\n  replicas: 1\n", r.chart, r.name, r.chart, v.chartVersion, v.appVersion)
	}
	return b.String()
}

func mergeValues(base, over map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range base {
		out[k] = v
	}
	for k, v := range over {
		if bm, ok := out[k].(map[string]any); ok {
			if om, ok := v.(map[string]any); ok {
				out[k] = mergeValues(bm, om)
				continue
			}
		}
		out[k] = v
	}
	return out
}

func (r release) releaseJSON(rev int, v revision, status string, now time.Time) map[string]any {
	deployed := now.Add(-v.ago)
	notes := fmt.Sprintf("%s is deployed.\n\nGet the application URL by running:\n  kubectl -n %s port-forward svc/%s 8080:80\n  open http://127.0.0.1:8080\n", r.name, r.ns, r.name)
	return map[string]any{
		"name": r.name, "namespace": r.ns, "version": rev,
		"info": map[string]any{"first_deployed": ts(now.Add(-r.revs[0].ago)), "last_deployed": ts(deployed), "deleted": "",
			"description": v.desc, "status": status, "notes": notes},
		"chart": map[string]any{
			"metadata": map[string]any{"name": r.chart, "version": v.chartVersion, "appVersion": v.appVersion, "apiVersion": "v2",
				"description": "A Helm chart for " + r.name, "type": "application"},
			"values": r.defaults,
		},
		"config":   v.values,
		"manifest": r.manifest(v),
	}
}

func encodeRelease(rel map[string]any) string {
	js, _ := json.Marshal(rel)
	var gz bytes.Buffer
	zw := gzip.NewWriter(&gz)
	zw.Write(js)
	zw.Close()
	return base64.StdEncoding.EncodeToString(gz.Bytes())
}

func decodeRelease(s string) (map[string]any, error) {
	raw, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return nil, err
	}
	zr, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	js, err := io.ReadAll(zr)
	if err != nil {
		return nil, err
	}
	var rel map[string]any
	return rel, json.Unmarshal(js, &rel)
}

func releaseSecret(ctx string, rel map[string]any, created time.Time) obj {
	n, ns := str(rel, "name"), str(rel, "namespace")
	rev := num(rel, "version")
	status := str(rel, "info", "status")
	sn := fmt.Sprintf("sh.helm.release.v1.%s.v%d", n, rev)
	return obj{"apiVersion": "v1", "kind": "Secret", "type": "helm.sh/release.v1",
		"metadata": obj{"name": sn, "namespace": ns, "uid": uid(ctx, "helm", ns, sn), "creationTimestamp": ts(created),
			"labels": obj{"owner": "helm", "name": n, "status": status, "version": strconv.Itoa(rev), "modifiedAt": strconv.FormatInt(created.Unix(), 10)}},
		"data": obj{"release": b64(encodeRelease(rel))}}
}

func (b *builder) helmReleases() {
	for _, r := range demoReleases(b.env()) {
		for i, v := range r.revs {
			status := "superseded"
			if i == len(r.revs)-1 {
				status = "deployed"
			}
			rel := r.releaseJSON(i+1, v, status, b.now)
			b.c.add(releaseSecret(b.spec.name, rel, b.now.Add(-v.ago)))
		}
	}
}

type helmFlags struct {
	ns, kubeconfig, ctx string
	pos                 []string
}

func parseHelm(args []string) helmFlags {
	var f helmFlags
	for i := 0; i < len(args); i++ {
		a := args[i]
		val := func() string {
			if _, v, ok := strings.Cut(a, "="); ok {
				return v
			}
			if i+1 < len(args) {
				i++
				return args[i]
			}
			return ""
		}
		switch {
		case a == "-n" || a == "--namespace" || strings.HasPrefix(a, "--namespace="):
			f.ns = val()
		case a == "--kubeconfig" || strings.HasPrefix(a, "--kubeconfig="):
			f.kubeconfig = val()
		case a == "--kube-context" || strings.HasPrefix(a, "--kube-context="):
			f.ctx = val()
		case strings.HasPrefix(a, "-"):
		default:
			f.pos = append(f.pos, a)
		}
	}
	return f
}

func helmMain(args []string) int {
	if len(args) == 0 {
		return unknown("helm", args)
	}
	f := parseHelm(args[1:])
	if f.ctx == "" {
		f.ctx = currentContext(kubeconfigPath(f.kubeconfig))
	}
	if f.ns == "" {
		f.ns = "default"
	}
	switch args[0] {
	case "version":
		fmt.Println(`version.BuildInfo{Version:"v3.16.2", GitCommit:"13654a52f7c70a143b1dd51416d633e1071faffb", GitTreeState:"clean", GoVersion:"go1.22.7"}`)
		return 0
	case "rollback":
		if len(f.pos) == 0 {
			fmt.Fprintln(os.Stderr, "Error: \"helm rollback\" requires at least 1 argument")
			return 1
		}
		time.Sleep(900 * time.Millisecond)
		return mutate(func(w *world) int { return helmRollback(newCl(w, f.ctx), f) })
	case "uninstall":
		if len(f.pos) == 0 {
			fmt.Fprintln(os.Stderr, "Error: \"helm uninstall\" requires at least 1 argument")
			return 1
		}
		time.Sleep(700 * time.Millisecond)
		return mutate(func(w *world) int {
			c := newCl(w, f.ctx)
			found := false
			for _, s := range helmSecrets(c, f.ns, f.pos[0]) {
				c.remove(s)
				found = true
			}
			if !found {
				fmt.Fprintf(os.Stderr, "Error: uninstall: Release not loaded: %s: release: not found\n", f.pos[0])
				return 1
			}
			fmt.Printf("release \"%s\" uninstalled\n", f.pos[0])
			return 0
		})
	}
	return unknown("helm", args)
}

func helmSecrets(c *cl, ns, rel string) []obj {
	var out []obj
	for _, o := range c.objs() {
		if str(o, "kind") == "Secret" && nsOf(o) == ns && str(o, "metadata", "labels", "owner") == "helm" && str(o, "metadata", "labels", "name") == rel {
			out = append(out, o)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		a, _ := strconv.Atoi(str(out[i], "metadata", "labels", "version"))
		b, _ := strconv.Atoi(str(out[j], "metadata", "labels", "version"))
		return a < b
	})
	return out
}

func secretRelease(s obj) map[string]any {
	inner, _ := base64.StdEncoding.DecodeString(str(s, "data", "release"))
	rel, _ := decodeRelease(string(inner))
	return rel
}

func helmRollback(c *cl, f helmFlags) int {
	name := f.pos[0]
	secs := helmSecrets(c, f.ns, name)
	if len(secs) == 0 {
		fmt.Fprintf(os.Stderr, "Error: release: not found\n")
		return 1
	}
	last := secretRelease(secs[len(secs)-1])
	target := num(last, "version") - 1
	if len(f.pos) > 1 {
		target, _ = strconv.Atoi(f.pos[1])
	}
	var src map[string]any
	for _, s := range secs {
		if r := secretRelease(s); num(r, "version") == target {
			src = r
		}
	}
	if src == nil || target < 1 {
		fmt.Fprintf(os.Stderr, "Error: release has no %d version\n", target)
		return 1
	}
	for _, s := range secs {
		if str(s, "metadata", "labels", "status") == "deployed" {
			r := secretRelease(s)
			sub(r, "info")["status"] = "superseded"
			sub(s, "metadata", "labels")["status"] = "superseded"
			sub(s, "data")["release"] = b64(encodeRelease(r))
		}
	}
	next := num(last, "version") + 1
	src["version"] = next
	info := sub(src, "info")
	info["status"] = "deployed"
	info["description"] = fmt.Sprintf("Rollback to %d", target)
	info["last_deployed"] = ts(c.now)
	c.add(releaseSecret(c.ctx, src, c.now))

	if d := c.find("deploy", f.ns, name); d != nil {
		if tag := str(src, "config", "image", "tag"); tag != "" {
			if cs := list(d, "spec", "template", "spec", "containers"); len(cs) > 0 {
				img := str(cs[0], "image")
				if i := strings.LastIndex(img, ":"); i > 0 {
					cs[0].(obj)["image"] = img[:i+1] + tag
				}
			}
		}
		if rc := num(src, "config", "replicaCount"); rc > 0 {
			sub(d, "spec")["replicas"] = rc
		}
		c.reconcile()
	}
	fmt.Println("Rollback was a success! Happy Helming!")
	return 0
}

func yamlText(v any) string {
	var b bytes.Buffer
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(2)
	_ = enc.Encode(v)
	return b.String()
}
