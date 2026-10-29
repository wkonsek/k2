package main

import (
	"fmt"
	"os"
	"sort"
	"strings"
)

func describeCmd(f flags) int {
	k, nm, ok := refArg(f.pos, 1)
	if !ok {
		return unknown("kubectl", f.raw)
	}
	_, ctx, ns := f.target()
	view := snapshot(ctx)
	var o obj
	for _, x := range view {
		if str(x, "kind") == k.Kind && objName(x) == nm && (!k.Namespaced || nsOf(x) == ns) {
			o = x
		}
	}
	if o == nil {
		notFound(resourceRef{k, nm})
		return 1
	}
	var b strings.Builder
	kv := func(key, val string) { fmt.Fprintf(&b, "%-18s%s\n", key+":", val) }
	kv("Name", nm)
	if k.Namespaced {
		kv("Namespace", nsOf(o))
	}
	mapLines := func(key string, mp map[string]string) {
		keys := make([]string, 0, len(mp))
		for k := range mp {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		if len(keys) == 0 {
			kv(key, "<none>")
			return
		}
		for i, k := range keys {
			label := ""
			if i == 0 {
				label = key + ":"
			}
			fmt.Fprintf(&b, "%-18s%s=%s\n", label, k, mp[k])
		}
	}
	if k.Kind == "Pod" {
		kv("Priority", "0")
		kv("Service Account", str(o, "spec", "serviceAccountName"))
		kv("Node", str(o, "spec", "nodeName")+"/"+str(o, "status", "hostIP"))
		kv("Start Time", parseTS(str(o, "status", "startTime")).Format("Mon, 02 Jan 2006 15:04:05 -0700"))
		mapLines("Labels", labels(o, "metadata", "labels"))
		_, st, _ := podStatus(o)
		kv("Status", st)
		kv("IP", str(o, "status", "podIP"))
		for _, r := range list(o, "metadata", "ownerReferences") {
			kv("Controlled By", str(r, "kind")+"/"+str(r, "name"))
		}
		statuses := map[string]any{}
		for _, s := range list(o, "status", "containerStatuses") {
			statuses[str(s, "name")] = s
		}
		if ics := list(o, "spec", "initContainers"); len(ics) > 0 {
			b.WriteString("Init Containers:\n")
			for _, c := range ics {
				fmt.Fprintf(&b, "  %s:\n    Image:          %s\n    State:          Terminated\n      Reason:       Completed\n      Exit Code:    0\n    Ready:          True\n", str(c, "name"), str(c, "image"))
			}
		}
		b.WriteString("Containers:\n")
		for _, c := range list(o, "spec", "containers") {
			s := statuses[str(c, "name")]
			fmt.Fprintf(&b, "  %s:\n    Image:          %s\n", str(c, "name"), str(c, "image"))
			for _, p := range list(c, "ports") {
				fmt.Fprintf(&b, "    Port:           %d/TCP (%s)\n", num(p, "containerPort"), str(p, "name"))
			}
			switch {
			case get(s, "state", "running") != nil:
				fmt.Fprintf(&b, "    State:          Running\n      Started:      %s\n", parseTS(str(s, "state", "running", "startedAt")).Format("Mon, 02 Jan 2006 15:04:05 -0700"))
			case get(s, "state", "waiting") != nil:
				fmt.Fprintf(&b, "    State:          Waiting\n      Reason:       %s\n", str(s, "state", "waiting", "reason"))
				if get(s, "lastState", "terminated") != nil {
					fmt.Fprintf(&b, "    Last State:     Terminated\n      Reason:       Error\n      Exit Code:    1\n")
				}
			}
			fmt.Fprintf(&b, "    Ready:          %v\n    Restart Count:  %d\n", get(s, "ready") == true, num(s, "restartCount"))
			fmt.Fprintf(&b, "    Limits:\n      cpu:     %s\n      memory:  %s\n    Requests:\n      cpu:     %s\n      memory:  %s\n",
				str(c, "resources", "limits", "cpu"), str(c, "resources", "limits", "memory"), str(c, "resources", "requests", "cpu"), str(c, "resources", "requests", "memory"))
		}
		b.WriteString("Conditions:\n  Type              Status\n")
		for _, c := range list(o, "status", "conditions") {
			fmt.Fprintf(&b, "  %-17s %s\n", str(c, "type"), str(c, "status"))
		}
		b.WriteString("Volumes:\n")
		for _, v := range list(o, "spec", "volumes") {
			switch {
			case get(v, "configMap") != nil:
				fmt.Fprintf(&b, "  %s:\n    Type:      ConfigMap (a volume populated by a ConfigMap)\n    Name:      %s\n", str(v, "name"), str(v, "configMap", "name"))
			case get(v, "secret") != nil:
				fmt.Fprintf(&b, "  %s:\n    Type:        Secret (a volume populated by a Secret)\n    SecretName:  %s\n", str(v, "name"), str(v, "secret", "secretName"))
			}
		}
		kv("QoS Class", "Burstable")
	} else {
		mapLines("Labels", labels(o, "metadata", "labels"))
		mapLines("Annotations", labels(o, "metadata", "annotations"))
		kv("CreationTimestamp", parseTS(str(o, "metadata", "creationTimestamp")).Format("Mon, 02 Jan 2006 15:04:05 -0700"))
		if spec, ok := o["spec"].(obj); ok {
			b.WriteString("Spec:\n")
			for _, l := range strings.Split(strings.TrimRight(yamlText(spec), "\n"), "\n") {
				b.WriteString("  " + l + "\n")
			}
		}
		if st, ok := o["status"].(obj); ok && len(st) > 0 {
			b.WriteString("Status:\n")
			for _, l := range strings.Split(strings.TrimRight(yamlText(st), "\n"), "\n") {
				b.WriteString("  " + l + "\n")
			}
		}
	}

	var evs []obj
	for _, e := range view {
		if str(e, "kind") == "Event" && str(e, "involvedObject", "name") == nm && str(e, "involvedObject", "kind") == k.Kind {
			evs = append(evs, e)
		}
	}
	if len(evs) == 0 {
		b.WriteString("Events:            <none>\n")
	} else {
		b.WriteString("Events:\n  Type     Reason     Age   From               Message\n  ----     ------     ----  ----               -------\n")
		for _, e := range evs {
			fmt.Fprintf(&b, "  %-8s %-10s %-5s %-18s %s\n", str(e, "type"), str(e, "reason"), age(parseTS(str(e, "lastTimestamp"))), str(e, "source", "component"), str(e, "message"))
		}
	}
	os.Stdout.WriteString(b.String())
	return 0
}
