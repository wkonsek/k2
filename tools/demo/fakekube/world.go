package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
)

type obj = map[string]any

type world struct {
	Contexts map[string][]obj `json:"contexts"`

	Applied map[string]bool `json:"applied"`

	Seq int `json:"seq"`
}

func worldPath() string { return filepath.Join(demoDir(), "world.json") }

func lockWorld() func() {
	f, err := os.OpenFile(worldPath()+".lock", os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return func() {}
	}
	_ = syscall.Flock(int(f.Fd()), syscall.LOCK_EX)
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		f.Close()
	}
}

func loadWorld() *world {
	w := &world{Contexts: map[string][]obj{}, Applied: map[string]bool{}}
	b, err := os.ReadFile(worldPath())
	if err != nil {
		return w
	}
	_ = json.Unmarshal(b, w)
	if w.Applied == nil {
		w.Applied = map[string]bool{}
	}
	return w
}

func (w *world) save() error {
	b, err := json.Marshal(w)
	if err != nil {
		return err
	}
	tmp := worldPath() + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, worldPath())
}

func readOnly() *world {
	unlock := lockWorld()
	defer unlock()
	return loadWorld()
}

func mutate(fn func(w *world) int) int {
	unlock := lockWorld()
	defer unlock()
	w := loadWorld()
	code := fn(w)
	if err := w.save(); err != nil {
		return 1
	}
	return code
}

type kindInfo struct {
	Plural     string
	Kind       string
	APIVersion string
	Namespaced bool
	Short      []string
}

func (k kindInfo) group() string {
	if g, _, ok := strings.Cut(k.APIVersion, "/"); ok {
		return g
	}
	return ""
}

func (k kindInfo) nameRef() string {
	s := strings.ToLower(k.Kind)
	if g := k.group(); g != "" {
		s += "." + g
	}
	return s
}

var kinds = []kindInfo{
	{"configmaps", "ConfigMap", "v1", true, []string{"cm"}},
	{"endpoints", "Endpoints", "v1", true, []string{"ep"}},
	{"events", "Event", "v1", true, []string{"ev"}},
	{"namespaces", "Namespace", "v1", false, []string{"ns"}},
	{"nodes", "Node", "v1", false, []string{"no"}},
	{"persistentvolumeclaims", "PersistentVolumeClaim", "v1", true, []string{"pvc"}},
	{"persistentvolumes", "PersistentVolume", "v1", false, []string{"pv"}},
	{"pods", "Pod", "v1", true, []string{"po"}},
	{"secrets", "Secret", "v1", true, nil},
	{"serviceaccounts", "ServiceAccount", "v1", true, []string{"sa"}},
	{"services", "Service", "v1", true, []string{"svc"}},
	{"customresourcedefinitions", "CustomResourceDefinition", "apiextensions.k8s.io/v1", false, []string{"crd", "crds"}},
	{"daemonsets", "DaemonSet", "apps/v1", true, []string{"ds"}},
	{"deployments", "Deployment", "apps/v1", true, []string{"deploy"}},
	{"replicasets", "ReplicaSet", "apps/v1", true, []string{"rs"}},
	{"statefulsets", "StatefulSet", "apps/v1", true, []string{"sts"}},
	{"applications", "Application", "argoproj.io/v1alpha1", true, []string{"app", "apps"}},
	{"horizontalpodautoscalers", "HorizontalPodAutoscaler", "autoscaling/v2", true, []string{"hpa"}},
	{"cronjobs", "CronJob", "batch/v1", true, []string{"cj"}},
	{"jobs", "Job", "batch/v1", true, nil},
	{"ingresses", "Ingress", "networking.k8s.io/v1", true, []string{"ing"}},
	{"clusterroles", "ClusterRole", "rbac.authorization.k8s.io/v1", false, nil},
	{"storageclasses", "StorageClass", "storage.k8s.io/v1", false, []string{"sc"}},
}

func lookupKind(s string) (kindInfo, bool) {
	s = strings.ToLower(s)
	base, grp, _ := strings.Cut(s, ".")
	for _, k := range kinds {
		if grp != "" && !strings.HasPrefix(k.group(), grp) && k.group() != grp {
			continue
		}
		if base == k.Plural || base == strings.ToLower(k.Kind) {
			return k, true
		}
		for _, sh := range k.Short {
			if base == sh {
				return k, true
			}
		}
	}
	return kindInfo{}, false
}

func kindOf(o obj) kindInfo {
	k, _ := lookupKind(str(o, "kind"))
	return k
}

func get(o any, path ...string) any {
	cur := o
	for _, p := range path {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = m[p]
	}
	return cur
}

func str(o any, path ...string) string {
	s, _ := get(o, path...).(string)
	return s
}

func num(o any, path ...string) int {
	switch v := get(o, path...).(type) {
	case float64:
		return int(v)
	case int:
		return v
	}
	return 0
}

func list(o any, path ...string) []any {
	l, _ := get(o, path...).([]any)
	return l
}

func sub(o obj, path ...string) obj {
	cur := o
	for _, p := range path {
		next, ok := cur[p].(map[string]any)
		if !ok {
			next = obj{}
			cur[p] = next
		}
		cur = next
	}
	return cur
}

func objName(o obj) string { return str(o, "metadata", "name") }
func nsOf(o obj) string    { return str(o, "metadata", "namespace") }

func labels(o any, path ...string) map[string]string {
	out := map[string]string{}
	if mm, ok := get(o, path...).(map[string]any); ok {
		for k, v := range mm {
			if s, ok := v.(string); ok {
				out[k] = s
			}
		}
	}
	return out
}

func ts(t time.Time) string { return t.UTC().Format(time.RFC3339) }

func parseTS(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t, _ = time.Parse(time.RFC3339Nano, s)
	}
	return t
}

func deepCopy(o obj) obj {
	b, _ := json.Marshal(o)
	var c obj
	_ = json.Unmarshal(b, &c)
	return c
}

func sortObjects(objs []obj) {
	sort.SliceStable(objs, func(i, j int) bool {
		a, b := objs[i], objs[j]
		if nsOf(a) != nsOf(b) {
			return nsOf(a) < nsOf(b)
		}
		return objName(a) < objName(b)
	})
}
