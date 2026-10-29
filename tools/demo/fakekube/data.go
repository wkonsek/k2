package main

import (
	"encoding/base64"
	"fmt"
	"os"
	"strings"
	"time"
)

type contextSpec struct {
	name  string
	nodes []string
	scale int

	trouble bool
}

var demoContexts = []contextSpec{
	{name: "kind-dev", nodes: []string{"dev-control-plane"}, scale: 1, trouble: true},
	{name: "staging-eu-1", nodes: []string{"staging-a-7x2kq", "staging-b-m4c9t", "staging-c-q8w2z"}, scale: 1},
	{name: "prod-eu-1", nodes: []string{"prod-a-k2v9m", "prod-a-x7r4d", "prod-b-c5n8p", "prod-b-w2j6s", "prod-c-h9t3f"}, scale: 2},
}

func initWorld() error {
	if demoDir() == "" {
		return fmt.Errorf("K2_DEMO_DIR is not set")
	}
	if err := os.MkdirAll(demoDir(), 0o755); err != nil {
		return err
	}
	w := &world{Contexts: map[string][]obj{}, Applied: map[string]bool{}}
	now := time.Now()
	for _, spec := range demoContexts {
		buildContext(w, spec, now)
	}
	if err := writeDemoKubeconfig(now); err != nil {
		return err
	}
	unlock := lockWorld()
	defer unlock()
	return w.save()
}

type ctr struct {
	name, image string
	port        int
	portName    string
	cpuReq      string
	cpuLim      string
	memReq      string
	memLim      string
	envFromCM   string
	envFromSec  string
	args        []string
}

func (k ctr) json() obj {
	c := obj{"name": k.name, "image": k.image, "imagePullPolicy": "IfNotPresent",
		"resources": obj{
			"requests": obj{"cpu": k.cpuReq, "memory": k.memReq},
			"limits":   obj{"cpu": k.cpuLim, "memory": k.memLim},
		},
		"terminationMessagePath": "/dev/termination-log", "terminationMessagePolicy": "File"}
	if k.port != 0 {
		pn := k.portName
		if pn == "" {
			pn = "http"
		}
		c["ports"] = []any{obj{"containerPort": k.port, "name": pn, "protocol": "TCP"}}
		c["readinessProbe"] = obj{"tcpSocket": obj{"port": k.port}, "periodSeconds": 10}
	}
	var from []any
	if k.envFromCM != "" {
		from = append(from, obj{"configMapRef": obj{"name": k.envFromCM}})
	}
	if k.envFromSec != "" {
		from = append(from, obj{"secretRef": obj{"name": k.envFromSec}})
	}
	if from != nil {
		c["envFrom"] = from
	}
	if len(k.args) > 0 {
		var a []any
		for _, s := range k.args {
			a = append(a, s)
		}
		c["args"] = a
	}
	return c
}

type app struct {
	ns, name string
	kind     string
	replicas int
	ctrs     []ctr
	inits    []ctr
	age      time.Duration
	rsAge    time.Duration
	cm       string
	secret   string
	pvc      string
	tier     string
	oldImage string
}

type builder struct {
	c    *cl
	spec contextSpec
	now  time.Time
}

func buildContext(w *world, spec contextSpec, now time.Time) {
	c := newCl(w, spec.name)
	c.now = now
	b := &builder{c: c, spec: spec, now: now}
	for i, n := range spec.nodes {
		b.node(n, i)
	}
	for _, ns := range []string{"default", "kube-system", "shop", "payments", "kafka", "monitoring", "argocd"} {
		b.namespace(ns)
	}
	s := spec.scale
	hours := time.Hour
	days := 24 * time.Hour
	apps := []app{

		{ns: "shop", name: "web", replicas: 2 * s, tier: "frontend", age: 41 * days, rsAge: 13 * hours, cm: "web-config", secret: "web-tls", oldImage: "nginx:1.26-alpine",
			ctrs: []ctr{{name: "nginx", image: "nginx:1.27-alpine", port: 8080, cpuReq: "50m", cpuLim: "200m", memReq: "64Mi", memLim: "128Mi"}}},
		{ns: "shop", name: "cart", replicas: 2 * s, tier: "backend", age: 41 * days, rsAge: 2*days + 3*hours, cm: "cart-config",
			ctrs: []ctr{{name: "app", image: "ghcr.io/acme/cart:1.8.2", port: 8080, cpuReq: "100m", cpuLim: "500m", memReq: "128Mi", memLim: "256Mi", envFromSec: "cart-redis"}}},
		{ns: "shop", name: "checkout", replicas: 2 * s, tier: "backend", age: 41 * days, rsAge: 13 * hours, cm: "checkout-config", oldImage: "ghcr.io/acme/checkout:2.2.9",
			inits: []ctr{{name: "migrate", image: "ghcr.io/acme/checkout:2.3.0", cpuReq: "50m", cpuLim: "200m", memReq: "64Mi", memLim: "128Mi", envFromSec: "checkout-db", args: []string{"migrate", "up"}}},
			ctrs: []ctr{
				{name: "app", image: "ghcr.io/acme/checkout:2.3.0", port: 8080, cpuReq: "100m", cpuLim: "500m", memReq: "128Mi", memLim: "256Mi", envFromSec: "checkout-db"},
				{name: "envoy", image: "envoyproxy/envoy:v1.31.2", port: 9901, portName: "admin", cpuReq: "50m", cpuLim: "200m", memReq: "64Mi", memLim: "128Mi"},
			}},
		{ns: "shop", name: "redis", kind: "StatefulSet", replicas: 1, tier: "cache", age: 41 * days, rsAge: 9 * days, pvc: "data",
			ctrs: []ctr{{name: "redis", image: "redis:7.4-alpine", port: 6379, portName: "redis", cpuReq: "100m", cpuLim: "500m", memReq: "128Mi", memLim: "512Mi"}}},

		{ns: "payments", name: "api", replicas: 2 * s, tier: "backend", age: 63 * days, rsAge: 4*hours + 12*time.Minute, cm: "api-config", secret: "api-tls", oldImage: "ghcr.io/acme/payments-api:3.1.3",
			ctrs: []ctr{{name: "api", image: "ghcr.io/acme/payments-api:3.1.4", port: 8443, portName: "https", cpuReq: "200m", cpuLim: "1", memReq: "256Mi", memLim: "512Mi", envFromSec: "postgres-credentials"}}},
		{ns: "payments", name: "worker", replicas: 2 * s, tier: "backend", age: 63 * days, rsAge: 4*hours + 12*time.Minute,
			ctrs: []ctr{{name: "worker", image: "ghcr.io/acme/payments-worker:3.1.4", cpuReq: "100m", cpuLim: "500m", memReq: "128Mi", memLim: "256Mi", envFromSec: "postgres-credentials"}}},
		{ns: "payments", name: "postgres", kind: "StatefulSet", replicas: 1, tier: "database", age: 63 * days, rsAge: 21 * days, pvc: "pgdata",
			ctrs: []ctr{{name: "postgres", image: "postgres:16.4", port: 5432, portName: "postgres", cpuReq: "250m", cpuLim: "1", memReq: "512Mi", memLim: "1Gi", envFromSec: "postgres-credentials"}}},

		{ns: "kafka", name: "kafka", kind: "StatefulSet", replicas: 3, tier: "broker", age: 88 * days, rsAge: 6 * days, pvc: "data",
			ctrs: []ctr{{name: "kafka", image: "apache/kafka:3.8.0", port: 9092, portName: "kafka", cpuReq: "500m", cpuLim: "2", memReq: "1Gi", memLim: "2Gi"}}},
		{ns: "kafka", name: "kafka-connect", replicas: 1, tier: "connect", age: 88 * days, rsAge: 1*days + 5*hours, cm: "kafka-connect-config",
			ctrs: []ctr{{name: "connect", image: "confluentinc/cp-kafka-connect:7.7.1", port: 8083, portName: "rest", cpuReq: "250m", cpuLim: "1", memReq: "512Mi", memLim: "1Gi"}}},
		{ns: "kafka", name: "schema-registry", replicas: 1, tier: "registry", age: 88 * days, rsAge: 12 * days,
			ctrs: []ctr{{name: "registry", image: "confluentinc/cp-schema-registry:7.7.1", port: 8081, cpuReq: "100m", cpuLim: "500m", memReq: "256Mi", memLim: "512Mi"}}},

		{ns: "monitoring", name: "prometheus", kind: "StatefulSet", replicas: 1, tier: "metrics", age: 120 * days, rsAge: 8 * days, pvc: "storage",
			ctrs: []ctr{{name: "prometheus", image: "quay.io/prometheus/prometheus:v2.54.1", port: 9090, portName: "web", cpuReq: "250m", cpuLim: "1", memReq: "1Gi", memLim: "2Gi"}}},
		{ns: "monitoring", name: "grafana", replicas: 1, tier: "dashboards", age: 120 * days, rsAge: 8 * days, cm: "grafana-dashboards",
			ctrs: []ctr{{name: "grafana", image: "grafana/grafana:11.2.2", port: 3000, cpuReq: "100m", cpuLim: "500m", memReq: "128Mi", memLim: "256Mi"}}},
		{ns: "monitoring", name: "kube-state-metrics", replicas: 1, tier: "metrics", age: 120 * days, rsAge: 8 * days,
			ctrs: []ctr{{name: "kube-state-metrics", image: "registry.k8s.io/kube-state-metrics/kube-state-metrics:v2.13.0", port: 8080, cpuReq: "10m", cpuLim: "100m", memReq: "32Mi", memLim: "128Mi"}}},
		{ns: "monitoring", name: "node-exporter", kind: "DaemonSet", tier: "metrics", age: 120 * days, rsAge: 8 * days,
			ctrs: []ctr{{name: "node-exporter", image: "quay.io/prometheus/node-exporter:v1.8.2", port: 9100, portName: "metrics", cpuReq: "10m", cpuLim: "100m", memReq: "16Mi", memLim: "64Mi"}}},

		{ns: "argocd", name: "argocd-server", replicas: 1 * s, tier: "server", age: 150 * days, rsAge: 17 * days,
			ctrs: []ctr{{name: "server", image: "quay.io/argoproj/argocd:v2.12.4", port: 8080, cpuReq: "50m", cpuLim: "500m", memReq: "64Mi", memLim: "256Mi"}}},
		{ns: "argocd", name: "argocd-repo-server", replicas: 1 * s, tier: "repo", age: 150 * days, rsAge: 17 * days,
			ctrs: []ctr{{name: "repo-server", image: "quay.io/argoproj/argocd:v2.12.4", port: 8081, cpuReq: "50m", cpuLim: "500m", memReq: "128Mi", memLim: "512Mi"}}},
		{ns: "argocd", name: "argocd-application-controller", kind: "StatefulSet", replicas: 1, tier: "controller", age: 150 * days, rsAge: 17 * days,
			ctrs: []ctr{{name: "application-controller", image: "quay.io/argoproj/argocd:v2.12.4", port: 8082, portName: "metrics", cpuReq: "250m", cpuLim: "1", memReq: "256Mi", memLim: "1Gi"}}},

		{ns: "kube-system", name: "coredns", replicas: 2, tier: "dns", age: 150 * days, rsAge: 150 * days,
			ctrs: []ctr{{name: "coredns", image: "registry.k8s.io/coredns/coredns:v1.11.3", port: 53, portName: "dns", cpuReq: "100m", cpuLim: "200m", memReq: "70Mi", memLim: "170Mi"}}},
		{ns: "kube-system", name: "metrics-server", replicas: 1, tier: "metrics", age: 150 * days, rsAge: 150 * days,
			ctrs: []ctr{{name: "metrics-server", image: "registry.k8s.io/metrics-server/metrics-server:v0.7.2", port: 10250, portName: "https", cpuReq: "100m", cpuLim: "200m", memReq: "200Mi", memLim: "400Mi"}}},
		{ns: "kube-system", name: "kube-proxy", kind: "DaemonSet", tier: "network", age: 150 * days, rsAge: 150 * days,
			ctrs: []ctr{{name: "kube-proxy", image: "registry.k8s.io/kube-proxy:v1.31.2", cpuReq: "10m", cpuLim: "100m", memReq: "32Mi", memLim: "64Mi"}}},
	}
	for _, a := range apps {
		b.workload(a)
	}
	b.services()
	b.configs()
	b.jobs()
	b.argoApps()
	b.helmReleases()
	if spec.trouble {
		b.trouble()
	}
	b.events()

	sortObjects(w.Contexts[spec.name])
}

func (b *builder) node(n string, i int) {
	role := "worker"
	if strings.Contains(n, "control-plane") {
		role = "control-plane"
	}
	zone := []string{"eu-west-1a", "eu-west-1b", "eu-west-1c"}[i%3]
	b.c.add(obj{"apiVersion": "v1", "kind": "Node",
		"metadata": obj{"name": n, "uid": uid(b.spec.name, "node", n), "creationTimestamp": ts(b.now.Add(-150 * 24 * time.Hour)),
			"labels": obj{"kubernetes.io/hostname": n, "kubernetes.io/os": "linux", "kubernetes.io/arch": "amd64",
				"node-role.kubernetes.io/" + role: "", "topology.kubernetes.io/zone": zone}},
		"spec": obj{"podCIDR": fmt.Sprintf("10.244.%d.0/24", i)},
		"status": obj{
			"capacity":    obj{"cpu": "8", "memory": "32734204Ki", "pods": "110"},
			"allocatable": obj{"cpu": "7910m", "memory": "31583228Ki", "pods": "110"},
			"conditions": []any{
				obj{"type": "MemoryPressure", "status": "False", "reason": "KubeletHasSufficientMemory"},
				obj{"type": "DiskPressure", "status": "False", "reason": "KubeletHasNoDiskPressure"},
				obj{"type": "Ready", "status": "True", "reason": "KubeletReady", "message": "kubelet is posting ready status"},
			},
			"addresses": []any{obj{"type": "InternalIP", "address": fmt.Sprintf("172.18.0.%d", i+2)}, obj{"type": "Hostname", "address": n}},
			"nodeInfo": obj{"kubeletVersion": "v1.31.2", "osImage": "Ubuntu 24.04.1 LTS", "kernelVersion": "6.8.0-1016-aws",
				"containerRuntimeVersion": "containerd://1.7.22", "architecture": "amd64", "operatingSystem": "linux"},
		}})
}

func (b *builder) namespace(ns string) {
	age := 150 * 24 * time.Hour
	b.c.add(obj{"apiVersion": "v1", "kind": "Namespace",
		"metadata": obj{"name": ns, "uid": uid(b.spec.name, "ns", ns), "creationTimestamp": ts(b.now.Add(-age)),
			"labels": obj{"kubernetes.io/metadata.name": ns}},
		"spec":   obj{"finalizers": []any{"kubernetes"}},
		"status": obj{"phase": "Active"}})
}

func (b *builder) workload(a app) {
	kind := a.kind
	if kind == "" {
		kind = "Deployment"
	}
	lbl := obj{"app": a.name, "app.kubernetes.io/name": a.name, "app.kubernetes.io/part-of": a.ns}
	if a.tier != "" {
		lbl["tier"] = a.tier
	}
	var cs, is []any
	for _, k := range a.ctrs {
		c := k.json()
		if a.cm != "" {
			c["volumeMounts"] = []any{obj{"name": "config", "mountPath": "/etc/" + a.name, "readOnly": true}}
		}
		if a.secret != "" {
			c["volumeMounts"] = append(list(c, "volumeMounts"), obj{"name": "tls", "mountPath": "/etc/" + a.name + "/tls", "readOnly": true})
		}
		if a.pvc != "" {
			c["volumeMounts"] = append(list(c, "volumeMounts"), obj{"name": a.pvc, "mountPath": "/var/lib/" + a.name})
		}
		cs = append(cs, c)
	}
	for _, k := range a.inits {
		is = append(is, k.json())
	}
	podSpec := obj{"containers": cs, "serviceAccountName": a.name}
	if is != nil {
		podSpec["initContainers"] = is
	}
	var vols []any
	if a.cm != "" {
		vols = append(vols, obj{"name": "config", "configMap": obj{"name": a.cm}})
	}
	if a.secret != "" {
		vols = append(vols, obj{"name": "tls", "secret": obj{"secretName": a.secret}})
	}
	if vols != nil {
		podSpec["volumes"] = vols
	}
	tmpl := obj{"metadata": obj{"labels": deepCopy(lbl)}, "spec": podSpec}
	o := obj{"apiVersion": "apps/v1", "kind": kind,
		"metadata": obj{"name": a.name, "namespace": a.ns, "uid": uid(b.spec.name, kind, a.ns, a.name), "generation": 3,
			"creationTimestamp": ts(b.now.Add(-a.age)), "labels": deepCopy(lbl), "annotations": obj{}},
		"spec": obj{"selector": obj{"matchLabels": obj{"app": a.name}}, "template": tmpl}}
	switch kind {
	case "Deployment":
		o["spec"].(obj)["replicas"] = a.replicas
		o["spec"].(obj)["strategy"] = obj{"type": "RollingUpdate", "rollingUpdate": obj{"maxSurge": "25%", "maxUnavailable": "25%"}}
		o["spec"].(obj)["revisionHistoryLimit"] = 10
	case "StatefulSet":
		o["spec"].(obj)["replicas"] = a.replicas
		o["spec"].(obj)["serviceName"] = a.name
		o["spec"].(obj)["podManagementPolicy"] = "OrderedReady"
		if a.pvc != "" {
			o["spec"].(obj)["volumeClaimTemplates"] = []any{obj{"metadata": obj{"name": a.pvc},
				"spec": obj{"accessModes": []any{"ReadWriteOnce"}, "resources": obj{"requests": obj{"storage": "20Gi"}}}}}
		}
	}
	b.c.add(o)

	if kind == "Deployment" && a.oldImage != "" {
		old := deepCopy(tmpl)
		list(old, "spec", "containers")[0].(obj)["image"] = a.oldImage
		cur := sub(o, "spec")["template"]
		sub(o, "spec")["template"] = old
		b.c.born = b.now.Add(-a.age / 2)
		b.c.reconcileDeployment(o)
		sub(o, "spec")["template"] = cur
	}
	b.c.born = b.now.Add(-a.rsAge)
	b.c.warmup = 20 * time.Second
	switch kind {
	case "Deployment":
		b.c.reconcileDeployment(o)
		for _, rs := range b.c.ownedBy("ReplicaSet", o) {
			b.c.reconcileReplicaSet(rs)
		}
	case "StatefulSet":
		b.c.reconcileStatefulSet(o)
		for i := 0; i < a.replicas && a.pvc != ""; i++ {
			b.pvc(a.ns, fmt.Sprintf("%s-%s-%d", a.pvc, a.name, i), "20Gi", a.rsAge)
		}
	case "DaemonSet":
		b.c.reconcileDaemonSet(o)
	}
	b.c.born = time.Time{}
	b.c.warmup = 6 * time.Second
}

func (b *builder) pvc(ns, n, size string, age time.Duration) {
	b.c.add(obj{"apiVersion": "v1", "kind": "PersistentVolumeClaim",
		"metadata": obj{"name": n, "namespace": ns, "uid": uid(b.spec.name, "pvc", ns, n), "creationTimestamp": ts(b.now.Add(-age))},
		"spec": obj{"accessModes": []any{"ReadWriteOnce"}, "resources": obj{"requests": obj{"storage": size}},
			"storageClassName": "standard", "volumeMode": "Filesystem", "volumeName": "pvc-" + uid(b.spec.name, "pv", ns, n)},
		"status": obj{"phase": "Bound", "accessModes": []any{"ReadWriteOnce"}, "capacity": obj{"storage": size}}})
}

func (b *builder) service(ns, n, app string, port, target int, portName string) {
	b.c.add(obj{"apiVersion": "v1", "kind": "Service",
		"metadata": obj{"name": n, "namespace": ns, "uid": uid(b.spec.name, "svc", ns, n), "creationTimestamp": ts(b.now.Add(-41 * 24 * time.Hour)),
			"labels": obj{"app": app}},
		"spec": obj{"type": "ClusterIP", "clusterIP": fmt.Sprintf("10.96.%d.%d", hashN(200, ns)+10, hashN(250, ns, n)+2),
			"selector": obj{"app": app}, "sessionAffinity": "None",
			"ports": []any{obj{"name": portName, "port": port, "targetPort": target, "protocol": "TCP"}}},
		"status": obj{"loadBalancer": obj{}}})
}

func (b *builder) services() {
	for _, s := range []struct {
		ns, n, app   string
		port, target int
		pn           string
	}{
		{"shop", "web", "web", 80, 8080, "http"},
		{"shop", "cart", "cart", 8080, 8080, "http"},
		{"shop", "checkout", "checkout", 8080, 8080, "http"},
		{"shop", "redis", "redis", 6379, 6379, "redis"},
		{"payments", "api", "api", 443, 8443, "https"},
		{"payments", "postgres", "postgres", 5432, 5432, "postgres"},
		{"kafka", "kafka", "kafka", 9092, 9092, "kafka"},
		{"kafka", "kafka-connect", "kafka-connect", 8083, 8083, "rest"},
		{"kafka", "schema-registry", "schema-registry", 8081, 8081, "http"},
		{"monitoring", "prometheus", "prometheus", 9090, 9090, "web"},
		{"monitoring", "grafana", "grafana", 80, 3000, "http"},
		{"argocd", "argocd-server", "argocd-server", 443, 8080, "https"},
		{"kube-system", "kube-dns", "coredns", 53, 53, "dns"},
	} {
		b.service(s.ns, s.n, s.app, s.port, s.target, s.pn)
	}
	b.c.add(obj{"apiVersion": "v1", "kind": "Service",
		"metadata": obj{"name": "kubernetes", "namespace": "default", "uid": uid(b.spec.name, "svc", "kubernetes"), "creationTimestamp": ts(b.now.Add(-150 * 24 * time.Hour))},
		"spec":     obj{"type": "ClusterIP", "clusterIP": "10.96.0.1", "ports": []any{obj{"name": "https", "port": 443, "targetPort": 6443, "protocol": "TCP"}}}})
	host := map[string]string{"kind-dev": "shop.dev.localhost", "staging-eu-1": "shop.staging.example.com", "prod-eu-1": "shop.example.com"}[b.spec.name]
	b.c.add(obj{"apiVersion": "networking.k8s.io/v1", "kind": "Ingress",
		"metadata": obj{"name": "web", "namespace": "shop", "uid": uid(b.spec.name, "ing", "web"), "creationTimestamp": ts(b.now.Add(-41 * 24 * time.Hour))},
		"spec": obj{"ingressClassName": "nginx", "tls": []any{obj{"hosts": []any{host}, "secretName": "web-tls"}},
			"rules": []any{obj{"host": host, "http": obj{"paths": []any{
				obj{"path": "/", "pathType": "Prefix", "backend": obj{"service": obj{"name": "web", "port": obj{"number": 80}}}},
				obj{"path": "/checkout", "pathType": "Prefix", "backend": obj{"service": obj{"name": "checkout", "port": obj{"number": 8080}}}},
			}}}}},
		"status": obj{"loadBalancer": obj{"ingress": []any{obj{"ip": "172.18.0.240"}}}}})
}

func b64(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

func (b *builder) configs() {
	cm := func(ns, n string, data map[string]string) {
		d := obj{}
		for k, v := range data {
			d[k] = v
		}
		b.c.add(obj{"apiVersion": "v1", "kind": "ConfigMap",
			"metadata": obj{"name": n, "namespace": ns, "uid": uid(b.spec.name, "cm", ns, n), "creationTimestamp": ts(b.now.Add(-13 * time.Hour))},
			"data":     d})
	}
	sec := func(ns, n, typ string, data map[string]string) {
		d := obj{}
		for k, v := range data {
			d[k] = b64(v)
		}
		b.c.add(obj{"apiVersion": "v1", "kind": "Secret", "type": typ,
			"metadata": obj{"name": n, "namespace": ns, "uid": uid(b.spec.name, "sec", ns, n), "creationTimestamp": ts(b.now.Add(-41 * 24 * time.Hour))},
			"data":     d})
	}
	cm("shop", "web-config", map[string]string{"nginx.conf": "server {\n  listen 8080;\n  location / { root /usr/share/nginx/html; }\n  location /api/ { proxy_pass http://cart:8080/; }\n}\n", "FEATURE_NEW_CHECKOUT": "true"})
	cm("shop", "cart-config", map[string]string{"REDIS_ADDR": "redis:6379", "CART_TTL": "72h", "LOG_LEVEL": "info"})
	cm("shop", "checkout-config", map[string]string{"PAYMENTS_URL": "https://api.payments.svc:443", "CURRENCY": "EUR", "LOG_LEVEL": "info"})
	cm("payments", "api-config", map[string]string{"DB_HOST": "postgres", "DB_NAME": "payments", "MAX_CONNS": "40"})
	cm("kafka", "kafka-connect-config", map[string]string{"CONNECT_BOOTSTRAP_SERVERS": "kafka:9092", "CONNECT_GROUP_ID": "connect-cluster", "CONNECT_CONFIG_STORAGE_TOPIC": "_connect-configs"})
	cm("monitoring", "grafana-dashboards", map[string]string{"shop.json": `{"title":"Shop overview","panels":[]}`, "kafka.json": `{"title":"Kafka lag","panels":[]}`})
	sec("shop", "web-tls", "kubernetes.io/tls", map[string]string{"tls.crt": "-----BEGIN CERTIFICATE-----\nMIIB…demo…\n-----END CERTIFICATE-----\n", "tls.key": "-----BEGIN PRIVATE KEY-----\n…demo…\n-----END PRIVATE KEY-----\n"})
	sec("shop", "cart-redis", "Opaque", map[string]string{"REDIS_PASSWORD": "s3cr3t-demo-only"})
	sec("shop", "checkout-db", "Opaque", map[string]string{"DATABASE_URL": "postgres://checkout:demo@postgres.payments:5432/checkout"})
	sec("payments", "api-tls", "kubernetes.io/tls", map[string]string{"tls.crt": "-----BEGIN CERTIFICATE-----\n…demo…\n-----END CERTIFICATE-----\n", "tls.key": "…demo…"})
	sec("payments", "postgres-credentials", "Opaque", map[string]string{"POSTGRES_USER": "payments", "POSTGRES_PASSWORD": "not-a-real-password"})
}

func (b *builder) jobs() {
	type cj struct{ ns, n, schedule, image string }
	for _, j := range []cj{{"shop", "cleanup-carts", "*/30 * * * *", "ghcr.io/acme/cart:1.8.2"}, {"payments", "reconcile", "0 2 * * *", "ghcr.io/acme/payments-worker:3.1.4"}} {
		cron := obj{"apiVersion": "batch/v1", "kind": "CronJob",
			"metadata": obj{"name": j.n, "namespace": j.ns, "uid": uid(b.spec.name, "cj", j.ns, j.n), "creationTimestamp": ts(b.now.Add(-41 * 24 * time.Hour))},
			"spec": obj{"schedule": j.schedule, "concurrencyPolicy": "Forbid", "suspend": false, "successfulJobsHistoryLimit": 3,
				"jobTemplate": obj{"spec": obj{"template": obj{"spec": obj{"restartPolicy": "OnFailure",
					"containers": []any{obj{"name": j.n, "image": j.image, "args": []any{j.n}}}}}}}},
			"status": obj{"lastScheduleTime": ts(b.now.Add(-17 * time.Minute)), "lastSuccessfulTime": ts(b.now.Add(-16 * time.Minute))}}
		b.c.add(cron)
		for i, ago := range []time.Duration{47 * time.Minute, 17 * time.Minute} {
			jn := fmt.Sprintf("%s-%d", j.n, 29350000+hashN(9000, j.n)+i*30)
			start := b.now.Add(-ago)
			job := obj{"apiVersion": "batch/v1", "kind": "Job",
				"metadata": obj{"name": jn, "namespace": j.ns, "uid": uid(b.spec.name, "job", jn), "creationTimestamp": ts(start),
					"labels": obj{"job-name": jn}, "ownerReferences": ownerRef(cron)},
				"spec":   obj{"completions": 1, "parallelism": 1, "backoffLimit": 6, "template": deepCopy(sub(cron, "spec", "jobTemplate", "spec", "template"))},
				"status": obj{"succeeded": 1, "startTime": ts(start), "completionTime": ts(start.Add(14 * time.Second)), "conditions": []any{obj{"type": "Complete", "status": "True"}}}}
			sub(job, "spec", "template")["metadata"] = obj{"labels": obj{"job-name": jn}}
			b.c.add(job)
			b.c.born = start
			p := b.c.newPod(job, jn+"-"+suffix(5, b.spec.name, jn))
			sub(p, "_demo")["phase"] = "Succeeded"
			b.c.born = time.Time{}
		}
	}
}

func (b *builder) argoApps() {
	b.c.add(obj{"apiVersion": "apiextensions.k8s.io/v1", "kind": "CustomResourceDefinition",
		"metadata": obj{"name": "applications.argoproj.io", "uid": uid(b.spec.name, "crd", "app"), "creationTimestamp": ts(b.now.Add(-150 * 24 * time.Hour))},
		"spec": obj{"group": "argoproj.io", "scope": "Namespaced", "names": obj{"kind": "Application", "plural": "applications", "shortNames": []any{"app", "apps"}},
			"versions": []any{obj{"name": "v1alpha1", "served": true, "storage": true}}}})
	for _, a := range []struct{ n, path, sync, health string }{
		{"shop", "apps/shop/overlays/" + b.env(), "Synced", "Healthy"},
		{"payments", "apps/payments/overlays/" + b.env(), "Synced", "Healthy"},
		{"kafka", "platform/kafka", "OutOfSync", "Healthy"},
		{"monitoring", "platform/monitoring", "Synced", "Healthy"},
	} {
		b.c.add(obj{"apiVersion": "argoproj.io/v1alpha1", "kind": "Application",
			"metadata": obj{"name": a.n, "namespace": "argocd", "uid": uid(b.spec.name, "app", a.n), "creationTimestamp": ts(b.now.Add(-120 * 24 * time.Hour))},
			"spec": obj{"project": "default", "destination": obj{"namespace": a.n, "server": "https://kubernetes.default.svc"},
				"source": obj{"repoURL": "https://github.com/acme/platform.git", "path": a.path, "targetRevision": "main"}},
			"status": obj{"sync": obj{"status": a.sync, "revision": suffix(12, a.n, b.spec.name)}, "health": obj{"status": a.health}}})
	}
}

func (b *builder) env() string {
	switch b.spec.name {
	case "prod-eu-1":
		return "prod"
	case "staging-eu-1":
		return "staging"
	}
	return "dev"
}

func (b *builder) trouble() {
	for _, o := range b.c.objs() {
		if str(o, "kind") == "Pod" && strings.HasPrefix(objName(o), "checkout-") && nsOf(o) == "shop" && !terminating(o) {
			d := sub(o, "_demo")
			d["crash"] = "envoy"
			d["restarts"] = 7
			return
		}
	}
}

func (b *builder) events() {
	n := 0
	ev := func(ns, kind, obName, typ, reason, msg string, count int, last time.Duration, comp string) {
		n++
		b.c.add(obj{"apiVersion": "v1", "kind": "Event", "type": typ, "reason": reason, "message": msg, "count": count,
			"metadata":       obj{"name": fmt.Sprintf("%s.%x", obName, 0x17f0a3c2b1d0000+n), "namespace": ns, "creationTimestamp": ts(b.now.Add(-last))},
			"firstTimestamp": ts(b.now.Add(-last - time.Duration(count)*time.Minute)), "lastTimestamp": ts(b.now.Add(-last)),
			"involvedObject": obj{"kind": kind, "name": obName, "namespace": ns}, "source": obj{"component": comp}})
	}
	for _, o := range b.c.objs() {
		if str(o, "kind") != "Pod" || terminating(o) {
			continue
		}
		crash := str(o, "_demo", "crash")
		if crash != "" {
			ev(nsOf(o), "Pod", objName(o), "Warning", "BackOff", "Back-off restarting failed container "+crash+" in pod "+objName(o)+"_"+nsOf(o), 31, 40*time.Second, "kubelet")
			ev(nsOf(o), "Pod", objName(o), "Warning", "Unhealthy", "Readiness probe failed: dial tcp "+str(o, "_demo", "ip")+":9901: connect: connection refused", 12, 2*time.Minute, "kubelet")
			ev(nsOf(o), "Pod", objName(o), "Normal", "Pulled", `Container image "envoyproxy/envoy:v1.31.2" already present on machine`, 8, 2*time.Minute, "kubelet")
		}
	}
	ev("shop", "Deployment", "web", "Normal", "ScalingReplicaSet", "Scaled up replica set web-"+b.hashOf("shop", "web")+" to "+fmt.Sprint(2*b.spec.scale), 1, 13*time.Hour, "deployment-controller")
	ev("shop", "Deployment", "checkout", "Normal", "ScalingReplicaSet", "Scaled up replica set checkout-"+b.hashOf("shop", "checkout")+" to "+fmt.Sprint(2*b.spec.scale), 1, 13*time.Hour, "deployment-controller")
	ev("payments", "Deployment", "api", "Normal", "ScalingReplicaSet", "Scaled down replica set api-"+b.hashOf("payments", "api")+" to 0 from 2", 1, 4*time.Hour, "deployment-controller")
	ev("kafka", "StatefulSet", "kafka", "Normal", "SuccessfulCreate", "create Pod kafka-2 in StatefulSet kafka successful", 1, 6*24*time.Hour, "statefulset-controller")
	ev("shop", "CronJob", "cleanup-carts", "Normal", "SawCompletedJob", "Saw completed job: cleanup-carts", 1, 16*time.Minute, "cronjob-controller")
}

func (b *builder) hashOf(ns, n string) string {
	d := b.c.find("deploy", ns, n)
	if d == nil {
		return "0"
	}
	return templateHash(deepCopy(sub(d, "spec", "template")))
}
