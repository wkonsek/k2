package main

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"sort"
	"strings"
	"time"
)

type cl struct {
	w   *world
	ctx string
	now time.Time

	born time.Time

	warmup time.Duration
}

func newCl(w *world, ctx string) *cl {
	return &cl{w: w, ctx: ctx, now: time.Now(), warmup: 6 * time.Second}
}

func (c *cl) objs() []obj { return c.w.Contexts[c.ctx] }

func (c *cl) add(o obj) { c.w.Contexts[c.ctx] = append(c.w.Contexts[c.ctx], o) }

func (c *cl) find(kind, ns, nm string) obj {
	k, ok := lookupKind(kind)
	if !ok {
		return nil
	}
	for _, o := range c.objs() {
		if str(o, "kind") == k.Kind && objName(o) == nm && (!k.Namespaced || nsOf(o) == ns) && !c.gone(o) {
			return o
		}
	}
	return nil
}

func (c *cl) remove(o obj) {
	objs := c.objs()
	for i, x := range objs {
		if sameRef(x, o) {
			c.w.Contexts[c.ctx] = append(objs[:i:i], objs[i+1:]...)
			return
		}
	}
}

func sameRef(a, b obj) bool {
	return str(a, "kind") == str(b, "kind") && nsOf(a) == nsOf(b) && objName(a) == objName(b)
}

func (c *cl) birth() time.Time {
	if c.born.IsZero() {
		return c.now
	}
	return c.born
}

func (c *cl) gone(o obj) bool {
	g := str(o, "_demo", "goneAt")
	return g != "" && !parseTS(g).After(c.now)
}

func (c *cl) purge() {
	var keep []obj
	for _, o := range c.objs() {
		if !c.gone(o) {
			keep = append(keep, o)
		}
	}
	c.w.Contexts[c.ctx] = keep
}

func (c *cl) terminate(o obj, grace time.Duration) {
	if str(o, "metadata", "deletionTimestamp") != "" {
		return
	}
	sub(o, "metadata")["deletionTimestamp"] = ts(c.now)
	sub(o, "_demo")["goneAt"] = c.now.Add(grace).UTC().Format(time.RFC3339Nano)
}

func terminating(o obj) bool { return str(o, "metadata", "deletionTimestamp") != "" }

const safeChars = "bcdfghjklmnpqrstvwxz2456789"

func suffix(n int, parts ...string) string {
	h := fnv.New64a()
	for _, p := range parts {
		h.Write([]byte(p))
		h.Write([]byte{0})
	}
	v := h.Sum64()
	out := make([]byte, n)
	for i := range out {
		out[i] = safeChars[v%uint64(len(safeChars))]
		v /= uint64(len(safeChars))
		if v == 0 {
			v = h.Sum64() ^ uint64(i*7919+1)
		}
	}
	return string(out)
}

func hashN(n int, parts ...string) int {
	h := fnv.New32a()
	for _, p := range parts {
		h.Write([]byte(p))
		h.Write([]byte{0})
	}
	return int(h.Sum32() % uint32(n))
}

func uid(parts ...string) string {
	s := suffix(32, parts...)
	hex := make([]byte, 32)
	for i := range s {
		hex[i] = "0123456789abcdef"[int(s[i])%16]
	}
	h := string(hex)
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:32]
}

func templateHash(tmpl any) string {
	b, _ := json.Marshal(tmpl)
	return suffix(8, string(b))
}

func (c *cl) ownedBy(kind string, owner obj) []obj {
	var out []obj
	for _, o := range c.objs() {
		if str(o, "kind") != kind || nsOf(o) != nsOf(owner) || c.gone(o) {
			continue
		}
		for _, r := range list(o, "metadata", "ownerReferences") {
			if str(r, "kind") == str(owner, "kind") && str(r, "name") == objName(owner) && get(r, "controller") == true {
				out = append(out, o)
			}
		}
	}
	return out
}

func ownerRef(o obj) []any {
	return []any{map[string]any{
		"apiVersion": str(o, "apiVersion"), "kind": str(o, "kind"), "name": objName(o),
		"uid": str(o, "metadata", "uid"), "controller": true, "blockOwnerDeletion": true,
	}}
}

func (c *cl) reconcile() {
	c.purge()
	for _, d := range c.objs() {
		if str(d, "kind") == "Deployment" && !terminating(d) {
			c.reconcileDeployment(d)
		}
	}
	for _, o := range c.objs() {
		switch str(o, "kind") {
		case "ReplicaSet":
			if !terminating(o) {
				c.reconcileReplicaSet(o)
			}
		case "StatefulSet":
			if !terminating(o) {
				c.reconcileStatefulSet(o)
			}
		case "DaemonSet":
			if !terminating(o) {
				c.reconcileDaemonSet(o)
			}
		}
	}
}

func (c *cl) reconcileDeployment(d obj) {
	tmpl := deepCopy(sub(d, "spec", "template"))
	hash := templateHash(tmpl)
	rsName := objName(d) + "-" + hash
	replicas := num(d, "spec", "replicas")
	var current obj
	maxRev := 0
	for _, rs := range c.ownedBy("ReplicaSet", d) {
		var rev int
		fmt.Sscan(str(rs, "metadata", "annotations", "deployment.kubernetes.io/revision"), &rev)
		if rev > maxRev {
			maxRev = rev
		}
		if objName(rs) == rsName {
			current = rs
		} else {
			sub(rs, "spec")["replicas"] = 0
		}
	}
	if current == nil {
		lbl := sub(tmpl, "metadata", "labels")
		lbl["pod-template-hash"] = hash
		rev := maxRev + 1
		current = obj{
			"apiVersion": "apps/v1", "kind": "ReplicaSet",
			"metadata": obj{
				"name": rsName, "namespace": nsOf(d), "uid": uid(c.ctx, "rs", nsOf(d), rsName),
				"creationTimestamp": ts(c.birth()), "labels": deepCopy(lbl),
				"annotations":     obj{"deployment.kubernetes.io/revision": fmt.Sprint(rev), "deployment.kubernetes.io/desired-replicas": fmt.Sprint(replicas)},
				"ownerReferences": ownerRef(d), "generation": 1,
			},
			"spec": obj{"replicas": replicas, "selector": obj{"matchLabels": obj{"app": str(lbl, "app"), "pod-template-hash": hash}}, "template": tmpl},
		}
		c.add(current)
		sub(d, "metadata", "annotations")["deployment.kubernetes.io/revision"] = fmt.Sprint(rev)
	}
	sub(current, "spec")["replicas"] = replicas
}

func (c *cl) reconcileReplicaSet(rs obj) {
	pods := c.livePods(rs)
	want := num(rs, "spec", "replicas")
	for len(pods) < want {
		c.w.Seq++
		pn := objName(rs) + "-" + suffix(5, c.ctx, objName(rs), fmt.Sprint(c.w.Seq))
		pods = append(pods, c.newPod(rs, pn))
	}

	sort.SliceStable(pods, func(i, j int) bool {
		return str(pods[i], "metadata", "creationTimestamp") < str(pods[j], "metadata", "creationTimestamp")
	})
	for i := want; i < len(pods); i++ {
		c.terminate(pods[len(pods)-1-(i-want)], 3*time.Second)
	}
}

func (c *cl) reconcileStatefulSet(s obj) {
	want := num(s, "spec", "replicas")
	have := map[string]bool{}
	for _, p := range c.livePods(s) {
		var i int
		if _, err := fmt.Sscanf(strings.TrimPrefix(objName(p), objName(s)+"-"), "%d", &i); err == nil && i >= want {
			c.terminate(p, 3*time.Second)
			continue
		}
		have[objName(p)] = true
	}
	for i := 0; i < want; i++ {
		pn := fmt.Sprintf("%s-%d", objName(s), i)
		if !have[pn] && c.find("pod", nsOf(s), pn) == nil {
			c.newPod(s, pn)
		}
	}
}

func (c *cl) reconcileDaemonSet(ds obj) {
	nodes := c.nodeNames()
	onNode := map[string]bool{}
	for _, p := range c.livePods(ds) {
		onNode[str(p, "spec", "nodeName")] = true
	}
	for _, n := range nodes {
		if !onNode[n] {
			c.w.Seq++
			p := c.newPod(ds, objName(ds)+"-"+suffix(5, c.ctx, objName(ds), n, fmt.Sprint(c.w.Seq)))
			sub(p, "spec")["nodeName"] = n
		}
	}
}

func (c *cl) livePods(owner obj) []obj {
	var out []obj
	for _, p := range c.ownedBy("Pod", owner) {
		if !terminating(p) {
			out = append(out, p)
		}
	}
	return out
}

func (c *cl) nodeNames() []string {
	var out []string
	for _, o := range c.objs() {
		if str(o, "kind") == "Node" {
			out = append(out, objName(o))
		}
	}
	sort.Strings(out)
	return out
}

func (c *cl) newPod(owner obj, pn string) obj {
	tmpl := deepCopy(sub(owner, "spec", "template"))
	spec := sub(tmpl, "spec")
	nodes := c.nodeNames()
	if len(nodes) > 0 {
		spec["nodeName"] = nodes[hashN(len(nodes), c.ctx, pn)]
	}
	if spec["serviceAccountName"] == nil {
		spec["serviceAccountName"] = "default"
	}
	spec["restartPolicy"] = "Always"
	spec["dnsPolicy"] = "ClusterFirst"
	spec["terminationGracePeriodSeconds"] = 30
	if str(owner, "kind") == "StatefulSet" {
		spec["hostname"] = pn
		spec["subdomain"] = objName(owner)
	}
	lbl := sub(tmpl, "metadata", "labels")
	if str(owner, "kind") == "StatefulSet" {
		lbl["statefulset.kubernetes.io/pod-name"] = pn
	}
	b := c.birth()
	p := obj{
		"apiVersion": "v1", "kind": "Pod",
		"metadata": obj{
			"name": pn, "namespace": nsOf(owner), "uid": uid(c.ctx, "pod", nsOf(owner), pn, ts(b)),
			"creationTimestamp": ts(b), "labels": lbl, "ownerReferences": ownerRef(owner),
		},
		"spec": spec,
		"_demo": obj{
			"startAt": b.Add(time.Second).UTC().Format(time.RFC3339Nano),
			"readyAt": b.Add(c.warmup).UTC().Format(time.RFC3339Nano),
			"ip":      fmt.Sprintf("10.244.%d.%d", hashN(4, c.ctx, pn)+1, hashN(250, pn, c.ctx)+3),
		},
	}
	if a := get(tmpl, "metadata", "annotations"); a != nil {
		sub(p, "metadata")["annotations"] = a
	}
	c.add(p)
	return p
}

type podState struct {
	ready     bool
	phase     string
	succeeded bool
}

func (c *cl) view() []obj {
	var out []obj
	pods := map[string]podState{}
	for _, o := range c.objs() {
		if c.gone(o) || str(o, "kind") != "Pod" {
			continue
		}
		p := deepCopy(o)
		pods[nsOf(p)+"/"+objName(p)] = c.settlePod(p)
		delete(p, "_demo")
		out = append(out, p)
	}

	for _, o := range c.objs() {
		if c.gone(o) || str(o, "kind") == "Pod" {
			continue
		}
		x := deepCopy(o)
		delete(x, "_demo")
		switch str(x, "kind") {
		case "ReplicaSet", "StatefulSet", "DaemonSet":
			total, ready := c.countPods(o, pods)
			st := sub(x, "status")
			st["replicas"] = total
			st["readyReplicas"] = ready
			st["availableReplicas"] = ready
			st["currentReplicas"] = total
			st["updatedReplicas"] = total
			st["fullyLabeledReplicas"] = total
			st["observedGeneration"] = num(x, "metadata", "generation")
			if str(x, "kind") == "DaemonSet" {
				n := len(c.nodeNames())
				st["desiredNumberScheduled"] = n
				st["currentNumberScheduled"] = total
				st["numberReady"] = ready
				st["numberAvailable"] = ready
				st["updatedNumberScheduled"] = total
				st["numberMisscheduled"] = 0
			}
		case "Deployment":
			c.settleDeployment(o, x, pods)
		}
		out = append(out, x)
	}
	return out
}

func (c *cl) countPods(owner obj, pods map[string]podState) (total, ready int) {
	for _, p := range c.ownedBy("Pod", owner) {
		if terminating(p) {
			continue
		}
		total++
		if pods[nsOf(p)+"/"+objName(p)].ready {
			ready++
		}
	}
	return
}

func (c *cl) settleDeployment(d, x obj, pods map[string]podState) {
	hash := templateHash(deepCopy(sub(d, "spec", "template")))
	total, ready, updated := 0, 0, 0
	for _, rs := range c.ownedBy("ReplicaSet", d) {
		t, r := c.countPods(rs, pods)
		total += t
		ready += r
		if objName(rs) == objName(d)+"-"+hash {
			updated = t
		}
	}
	want := num(d, "spec", "replicas")
	st := sub(x, "status")
	st["observedGeneration"] = num(d, "metadata", "generation")
	st["replicas"] = total
	st["updatedReplicas"] = updated
	st["readyReplicas"] = ready
	st["availableReplicas"] = ready
	if want-ready > 0 {
		st["unavailableReplicas"] = want - ready
	}
	avail := "True"
	if ready < want {
		avail = "False"
	}
	created := str(d, "metadata", "creationTimestamp")
	st["conditions"] = []any{
		obj{"type": "Available", "status": avail, "reason": map[bool]string{true: "MinimumReplicasAvailable", false: "MinimumReplicasUnavailable"}[avail == "True"],
			"message": "Deployment has minimum availability.", "lastTransitionTime": created, "lastUpdateTime": created},
		obj{"type": "Progressing", "status": "True", "reason": "NewReplicaSetAvailable",
			"message": fmt.Sprintf("ReplicaSet %q has successfully progressed.", objName(d)+"-"+hash), "lastTransitionTime": created, "lastUpdateTime": created},
	}
}

func (c *cl) settlePod(p obj) podState {
	d := sub(p, "_demo")
	start := parseTS(str(d, "startAt"))
	readyAt := parseTS(str(d, "readyAt"))
	crash := str(d, "crash")
	restarts := num(d, "restarts")
	phase := "Running"
	if str(d, "phase") != "" {
		phase = str(d, "phase")
	}
	if c.now.Before(start) {
		phase = "Pending"
	}
	st := obj{"phase": phase, "qosClass": "Burstable", "hostIP": "172.18.0." + fmt.Sprint(2+hashN(6, str(p, "spec", "nodeName"))),
		"startTime": ts(parseTS(str(p, "metadata", "creationTimestamp")))}
	if phase != "Pending" {
		st["podIP"] = str(d, "ip")
		st["podIPs"] = []any{obj{"ip": str(d, "ip")}}
	}
	allReady := phase == "Running" && !c.now.Before(readyAt) && !terminating(p)
	var cs []any
	for _, ct := range list(p, "spec", "containers") {
		cn := str(ct, "name")
		s := obj{"name": cn, "image": str(ct, "image"), "imageID": "docker.io/library/" + cn + "@sha256:" + suffix(16, str(ct, "image")),
			"restartCount": 0, "started": true, "ready": allReady}
		switch {
		case phase == "Pending":
			s["state"] = obj{"waiting": obj{"reason": "ContainerCreating"}}
			s["started"] = false
		case phase == "Succeeded":
			s["state"] = obj{"terminated": obj{"reason": "Completed", "exitCode": 0, "startedAt": ts(start), "finishedAt": ts(start.Add(14 * time.Second))}}
			s["ready"] = false
			s["started"] = false
		case cn == crash:
			s["state"] = obj{"waiting": obj{"reason": "CrashLoopBackOff", "message": "back-off 5m0s restarting failed container=" + cn}}
			s["lastState"] = obj{"terminated": obj{"reason": "Error", "exitCode": 1, "startedAt": ts(c.now.Add(-100 * time.Second)), "finishedAt": ts(c.now.Add(-97 * time.Second))}}
			s["restartCount"] = restarts
			s["ready"] = false
			s["started"] = false
			allReady = false
		default:
			s["state"] = obj{"running": obj{"startedAt": ts(start)}}
		}
		cs = append(cs, s)
	}
	if crash != "" {
		for _, s := range cs {
			if str(s, "name") != crash {
				s.(obj)["ready"] = !terminating(p)
			}
		}
	}
	st["containerStatuses"] = cs
	var ics []any
	for _, ct := range list(p, "spec", "initContainers") {
		ics = append(ics, obj{"name": str(ct, "name"), "image": str(ct, "image"), "ready": true, "restartCount": 0, "started": false,
			"state": obj{"terminated": obj{"reason": "Completed", "exitCode": 0, "startedAt": ts(start), "finishedAt": ts(start.Add(3 * time.Second))}}})
	}
	if ics != nil {
		st["initContainerStatuses"] = ics
	}
	readyS := map[bool]string{true: "True", false: "False"}[allReady]
	st["conditions"] = []any{
		obj{"type": "PodScheduled", "status": "True", "lastTransitionTime": str(p, "metadata", "creationTimestamp")},
		obj{"type": "Initialized", "status": readyS, "lastTransitionTime": ts(start)},
		obj{"type": "ContainersReady", "status": readyS, "lastTransitionTime": ts(readyAt)},
		obj{"type": "Ready", "status": readyS, "lastTransitionTime": ts(readyAt)},
	}
	p["status"] = st
	return podState{ready: allReady, phase: phase, succeeded: phase == "Succeeded"}
}
