package kube

import (
	"sort"
	"strconv"
	"strings"
	"time"

	"k2/internal/shell"
)

type Pulse struct {
	Pods                     int
	Running, Pending, Failed int
	Succeeded, Other         int
	Restarts                 int
	Crashing                 int
	Workloads                []WorkloadPulse
	JobsActive, JobsFailed   int
	JobsDone                 int
	PVCBound, PVCPending     int
	PVCLost                  int
	Warnings                 []EventPulse

	CPUUsed, CPUOf float64
	MemUsed, MemOf float64
	CPUApprox      bool
	MemApprox      bool
	Metrics        bool
	Problems       []string
}

type WorkloadPulse struct {
	Kind         string
	Ready, Total int
}

type EventPulse struct {
	At      time.Time
	Object  string
	Reason  string
	Message string
	Count   int
}

const pulseList = "pods,deployments,statefulsets,daemonsets,jobs,persistentvolumeclaims"

func (c Client) PulseObjects(kubeconfig, context, namespace string) ([]Object, error) {
	return c.listMany(kubeconfig, context, namespace, pulseList, "")
}

func (c Client) WarningEvents(kubeconfig, context, namespace string) ([]Object, error) {
	return c.listMany(kubeconfig, context, namespace, "events", "type=Warning")
}

func (c Client) listMany(kubeconfig, context, namespace, kinds, fieldSel string) ([]Object, error) {
	args := []string{"get", kinds, "-o", "json", "--kubeconfig", kubeconfig, "--context", context}
	if fieldSel != "" {
		args = append(args, "--field-selector", fieldSel)
	}
	if namespace == "" {
		args = append(args, "-A")
	} else {
		args = append(args, "-n", namespace)
	}
	out, err := shell.Combined(c.Run, "kubectl", args...)
	if err != nil {
		return nil, cmdError(out, err)
	}
	return ParseObjects(out)
}

func BuildPulse(objs, events []Object, usage map[string]Usage) Pulse {
	var p Pulse
	ready := map[string]*WorkloadPulse{}
	order := []string{"Deployments", "StatefulSets", "DaemonSets"}
	for _, k := range order {
		ready[k] = &WorkloadPulse{Kind: k}
	}
	var cpuAll, memAll = true, true
	for _, o := range objs {
		switch o.Str("kind") {
		case "Pod":
			p.Pods++
			_, _, restarts, status := PodState(o)
			p.Restarts += restarts
			switch {
			case strings.Contains(status, "CrashLoop") || status == "Error" || strings.Contains(status, "ImagePull") || strings.Contains(status, "ErrImage"):
				p.Crashing++
				p.Failed++
				p.problem("pod " + o.Metadata.Name + " " + status)
			case o.Status.Phase == "Running":
				p.Running++
			case o.Status.Phase == "Pending":
				p.Pending++
				if status != "Pending" {
					p.problem("pod " + o.Metadata.Name + " " + status)
				}
			case o.Status.Phase == "Failed":
				p.Failed++
				p.problem("pod " + o.Metadata.Name + " " + status)
			case o.Status.Phase == "Succeeded":
				p.Succeeded++
			default:
				p.Other++
			}
			if usage != nil && o.Status.Phase == "Running" {
				u, ok := usage[o.Metadata.Namespace+"/"+o.Metadata.Name]
				if ok {
					p.Metrics = true
					cu, _ := ParseCPU(u.CPU)
					mu, _ := ParseMem(u.Mem)
					p.CPUUsed += cu
					p.MemUsed += mu
					cpuL, memL := podLimits(o)
					cpuR, memR := podRequests(o)

					if v, err := strconv.ParseFloat(cpuL, 64); err == nil {
						p.CPUOf += v
					} else if v, err := strconv.ParseFloat(cpuR, 64); err == nil {
						p.CPUOf += v
						cpuAll = false
					}
					if v, err := strconv.ParseFloat(memL, 64); err == nil {
						p.MemOf += v
					} else if v, err := strconv.ParseFloat(memR, 64); err == nil {
						p.MemOf += v
						memAll = false
					}
				}
			}
		case "Deployment", "StatefulSet":
			k := o.Str("kind") + "s"
			want := o.IntOr(1, "spec", "replicas")
			ready[k].Total++
			if o.IntOr(0, "status", "readyReplicas") >= want {
				ready[k].Ready++
			} else {
				p.problem(strings.ToLower(o.Str("kind")) + " " + o.Metadata.Name + " " + readyWord(o.IntOr(0, "status", "readyReplicas"), want))
			}
		case "DaemonSet":
			ready["DaemonSets"].Total++
			if o.IntOr(0, "status", "numberReady") >= o.IntOr(0, "status", "desiredNumberScheduled") {
				ready["DaemonSets"].Ready++
			} else {
				p.problem("daemonset " + o.Metadata.Name + " not ready")
			}
		case "Job":
			switch JobStatus(o) {
			case "Complete":
				p.JobsDone++
			case "Failed":
				p.JobsFailed++
				p.problem("job " + o.Metadata.Name + " failed")
			default:
				p.JobsActive++
			}
		case "PersistentVolumeClaim":
			switch o.Str("status", "phase") {
			case "Bound":
				p.PVCBound++
			case "Lost":
				p.PVCLost++
				p.problem("pvc " + o.Metadata.Name + " lost")
			default:
				p.PVCPending++
			}
		}
	}
	p.CPUApprox, p.MemApprox = !cpuAll, !memAll
	for _, k := range order {
		p.Workloads = append(p.Workloads, *ready[k])
	}
	for _, e := range events {
		p.Warnings = append(p.Warnings, EventPulse{
			At:      eventTime(e),
			Object:  strings.ToLower(e.Str("involvedObject", "kind")) + "/" + e.Str("involvedObject", "name"),
			Reason:  e.Str("reason"),
			Message: strings.TrimSpace(e.Str("message")),
			Count:   e.IntOr(1, "count"),
		})
	}
	sort.SliceStable(p.Warnings, func(i, j int) bool { return p.Warnings[i].At.After(p.Warnings[j].At) })
	return p
}

const maxProblems = 8

func (p *Pulse) problem(s string) {
	if len(p.Problems) < maxProblems {
		p.Problems = append(p.Problems, s)
	}
}
