package kube

import (
	"strings"
	"testing"
)

const pulseJSON = `{"kind":"List","items":[
{"kind":"Pod","metadata":{"name":"web-1","namespace":"shop"},"spec":{"containers":[{"name":"a","resources":{"limits":{"cpu":"500m","memory":"256Mi"}}}]},
 "status":{"phase":"Running","containerStatuses":[{"ready":true,"restartCount":2,"state":{"running":{}}}]}},
{"kind":"Pod","metadata":{"name":"web-2","namespace":"shop"},"spec":{"containers":[{"name":"a","resources":{"requests":{"cpu":"250m","memory":"128Mi"}}}]},
 "status":{"phase":"Running","containerStatuses":[{"ready":false,"restartCount":7,"state":{"waiting":{"reason":"CrashLoopBackOff"}}}]}},
{"kind":"Pod","metadata":{"name":"new","namespace":"shop"},"status":{"phase":"Pending"}},
{"kind":"Pod","metadata":{"name":"done","namespace":"shop"},"status":{"phase":"Succeeded"}},
{"kind":"Deployment","metadata":{"name":"web","namespace":"shop"},"spec":{"replicas":2},"status":{"readyReplicas":1}},
{"kind":"Deployment","metadata":{"name":"api","namespace":"shop"},"spec":{"replicas":1},"status":{"readyReplicas":1}},
{"kind":"StatefulSet","metadata":{"name":"db","namespace":"shop"},"spec":{"replicas":1},"status":{"readyReplicas":1}},
{"kind":"Job","metadata":{"name":"j1","namespace":"shop"},"status":{"conditions":[{"type":"Failed","status":"True"}]}},
{"kind":"Job","metadata":{"name":"j2","namespace":"shop"},"status":{"succeeded":1,"conditions":[{"type":"Complete","status":"True"}]}},
{"kind":"PersistentVolumeClaim","metadata":{"name":"data","namespace":"shop"},"status":{"phase":"Bound"}},
{"kind":"PersistentVolumeClaim","metadata":{"name":"wait","namespace":"shop"},"status":{"phase":"Pending"}}
]}`

const eventsJSON = `{"kind":"List","items":[
{"kind":"Event","metadata":{"name":"e1","namespace":"shop"},"type":"Warning","reason":"BackOff","message":"Back-off restarting","count":12,"lastTimestamp":"2026-10-23T10:00:00Z","involvedObject":{"kind":"Pod","name":"web-2"}},
{"kind":"Event","metadata":{"name":"e2","namespace":"shop"},"type":"Warning","reason":"FailedMount","message":"no volume","count":1,"lastTimestamp":"2026-10-23T11:00:00Z","involvedObject":{"kind":"Pod","name":"new"}}
]}`

func TestBuildPulse(t *testing.T) {
	objs, err := ParseObjects([]byte(pulseJSON))
	if err != nil {
		t.Fatal(err)
	}
	evs, _ := ParseObjects([]byte(eventsJSON))
	usage := map[string]Usage{"shop/web-1": {CPU: "250m", Mem: "128Mi"}, "shop/web-2": {CPU: "100m", Mem: "64Mi"}}
	p := BuildPulse(objs, evs, usage)
	if p.Pods != 4 || p.Running != 1 || p.Crashing != 1 || p.Failed != 1 || p.Pending != 1 || p.Succeeded != 1 || p.Restarts != 9 {
		t.Fatalf("pods %+v", p)
	}
	if p.Workloads[0] != (WorkloadPulse{"Deployments", 1, 2}) || p.Workloads[1] != (WorkloadPulse{"StatefulSets", 1, 1}) || p.Workloads[2].Total != 0 {
		t.Fatalf("workloads %+v", p.Workloads)
	}
	if p.JobsFailed != 1 || p.JobsDone != 1 || p.PVCBound != 1 || p.PVCPending != 1 {
		t.Fatalf("jobs/pvc %+v", p)
	}
	if len(p.Warnings) != 2 || p.Warnings[0].Reason != "FailedMount" || p.Warnings[1].Count != 12 || p.Warnings[1].Object != "pod/web-2" {
		t.Fatalf("warnings %+v", p.Warnings)
	}
	if !p.Metrics || p.CPUUsed != 350 || p.CPUOf != 750 || !p.CPUApprox || p.MemOf != 384*1024*1024 {
		t.Fatalf("metrics %+v", p)
	}
	joined := strings.Join(p.Problems, "|")
	for _, want := range []string{"pod web-2 CrashLoopBackOff", "deployment web Progressing", "job j1 failed"} {
		if !strings.Contains(joined, want) {
			t.Errorf("problems lack %q: %s", want, joined)
		}
	}
	if q := BuildPulse(objs, nil, nil); q.Metrics {
		t.Fatal("no usage, no metrics")
	}
}
