package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

func refArg(pos []string, i int) (kindInfo, string, bool) {
	if len(pos) <= i {
		return kindInfo{}, "", false
	}
	kn, nm, ok := strings.Cut(pos[i], "/")
	if !ok {
		if len(pos) <= i+1 {
			return kindInfo{}, "", false
		}
		kn, nm = pos[i], pos[i+1]
	}
	k, found := lookupKind(kn)
	return k, nm, found
}

func deleteCmd(f flags) int {
	if f.kustomize != "" {
		return kustomizeRun(f, "delete")
	}
	k, nm, ok := refArg(f.pos, 1)
	if !ok {
		return unknown("kubectl", f.raw)
	}
	_, ctx, ns := f.target()
	code := mutate(func(w *world) int {
		c := newCl(w, ctx)
		o := c.find(k.Kind, ns, nm)
		if o == nil {
			notFound(resourceRef{k, nm})
			return 1
		}
		grace := 2500 * time.Millisecond
		c.terminate(o, grace)

		for _, kind := range []string{"ReplicaSet", "Pod"} {
			for _, x := range c.ownedBy(kind, o) {
				c.terminate(x, grace)
				for _, p := range c.ownedBy("Pod", x) {
					c.terminate(p, grace)
				}
			}
		}
		c.reconcile()
		return 0
	})
	if code != 0 {
		return code
	}
	time.Sleep(1200 * time.Millisecond)
	if k.Namespaced {
		fmt.Printf("%s %q deleted from %s namespace\n", k.nameRef(), nm, ns)
	} else {
		fmt.Printf("%s %q deleted\n", k.nameRef(), nm)
	}
	return 0
}

func scaleCmd(f flags) int {
	k, nm, ok := refArg(f.pos, 1)
	if !ok {
		return unknown("kubectl", f.raw)
	}
	var n int
	if _, err := fmt.Sscan(f.values["--replicas"], &n); err != nil {
		fmt.Fprintln(os.Stderr, "error: --replicas=COUNT is required")
		return 1
	}
	_, ctx, ns := f.target()
	return mutate(func(w *world) int {
		c := newCl(w, ctx)
		o := c.find(k.Kind, ns, nm)
		if o == nil {
			notFound(resourceRef{k, nm})
			return 1
		}
		sub(o, "spec")["replicas"] = n
		c.reconcile()
		fmt.Printf("%s/%s scaled\n", k.nameRef(), nm)
		return 0
	})
}

func rolloutCmd(f flags) int {
	if len(f.pos) < 2 || f.pos[1] != "restart" {
		return unknown("kubectl", f.raw)
	}
	k, nm, ok := refArg(f.pos, 2)
	if !ok {
		return unknown("kubectl", f.raw)
	}
	_, ctx, ns := f.target()
	return mutate(func(w *world) int {
		c := newCl(w, ctx)
		o := c.find(k.Kind, ns, nm)
		if o == nil {
			notFound(resourceRef{k, nm})
			return 1
		}
		sub(o, "spec", "template", "metadata", "annotations")["kubectl.kubernetes.io/restartedAt"] = time.Now().Format(time.RFC3339)
		if k.Kind != "Deployment" {

			for _, p := range c.livePods(o) {
				c.terminate(p, 2*time.Second)
			}
		}
		c.reconcile()
		fmt.Printf("%s/%s restarted\n", k.nameRef(), nm)
		return 0
	})
}

func labelCmd(f flags, verb string) int {
	k, nm, ok := refArg(f.pos, 1)
	if !ok {
		return unknown("kubectl", f.raw)
	}
	pairs := f.pos[2:]
	if !strings.Contains(f.pos[1], "/") {
		pairs = f.pos[3:]
	}
	field := map[string]string{"label": "labels", "annotate": "annotations"}[verb]
	_, ctx, ns := f.target()
	return mutate(func(w *world) int {
		c := newCl(w, ctx)
		o := c.find(k.Kind, ns, nm)
		if o == nil {
			notFound(resourceRef{k, nm})
			return 1
		}
		mp := sub(o, "metadata", field)
		for _, p := range pairs {
			if key, val, set := strings.Cut(p, "="); set {
				mp[key] = val
			} else {
				delete(mp, strings.TrimSuffix(p, "-"))
			}
		}
		fmt.Printf("%s/%s %s\n", k.nameRef(), nm, map[string]string{"label": "labeled", "annotate": "annotated"}[verb])
		return 0
	})
}

func setImageCmd(f flags) int {
	if len(f.pos) < 3 || f.pos[1] != "image" {
		return unknown("kubectl", f.raw)
	}
	k, nm, ok := refArg(f.pos, 2)
	if !ok {
		return unknown("kubectl", f.raw)
	}
	_, ctx, ns := f.target()
	return mutate(func(w *world) int {
		c := newCl(w, ctx)
		o := c.find(k.Kind, ns, nm)
		if o == nil {
			notFound(resourceRef{k, nm})
			return 1
		}
		for _, p := range f.pos[3:] {
			cn, img, _ := strings.Cut(p, "=")
			for _, field := range []string{"containers", "initContainers"} {
				for _, ct := range list(o, "spec", "template", "spec", field) {
					if str(ct, "name") == cn {
						ct.(obj)["image"] = img
					}
				}
			}
		}
		c.reconcile()
		fmt.Printf("%s/%s image updated\n", k.nameRef(), nm)
		return 0
	})
}

func createJobCmd(f flags) int {
	from := f.values["--from"]
	if len(f.pos) < 3 || f.pos[1] != "job" || !strings.HasPrefix(from, "cronjob/") {
		return unknown("kubectl", f.raw)
	}
	_, ctx, ns := f.target()
	jn := f.pos[2]
	return mutate(func(w *world) int {
		c := newCl(w, ctx)
		cj := c.find("cronjob", ns, strings.TrimPrefix(from, "cronjob/"))
		if cj == nil {
			fmt.Fprintf(os.Stderr, "error: cronjobs.batch %q not found\n", strings.TrimPrefix(from, "cronjob/"))
			return 1
		}
		job := obj{"apiVersion": "batch/v1", "kind": "Job",
			"metadata": obj{"name": jn, "namespace": ns, "creationTimestamp": ts(c.now), "labels": obj{"job-name": jn}, "uid": uid(ctx, "job", jn)},
			"spec":     obj{"completions": 1, "template": deepCopy(sub(cj, "spec", "jobTemplate", "spec", "template"))},
			"status":   obj{"active": 1, "startTime": ts(c.now)}}
		c.add(job)
		p := c.newPod(job, jn+"-"+suffix(5, ctx, jn))
		sub(p, "_demo")["phase"] = "Running"
		fmt.Printf("job.batch/%s created\n", jn)
		return 0
	})
}

func editCmd(f flags) int {
	k, nm, ok := refArg(f.pos, 1)
	if !ok {
		return unknown("kubectl", f.raw)
	}
	_, ctx, ns := f.target()
	var shown obj
	for _, o := range snapshot(ctx) {
		if str(o, "kind") == k.Kind && objName(o) == nm && (!k.Namespaced || nsOf(o) == ns) {
			shown = o
		}
	}
	if shown == nil {
		notFound(resourceRef{k, nm})
		return 1
	}
	tmp, err := os.CreateTemp("", "kubectl-edit-*.yaml")
	if err != nil {
		return 1
	}
	defer os.Remove(tmp.Name())
	header := "# Please edit the object below. Lines beginning with a '#' will be ignored,\n# and an empty file will abort the edit. If an error occurs while saving this file will be\n# reopened with the relevant failures.\n#\n"
	before := header + yamlText(shown)
	tmp.WriteString(before)
	tmp.Close()
	editor := os.Getenv("KUBE_EDITOR")
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}
	if editor == "" {
		editor = "vi"
	}
	cmd := exec.Command("sh", "-c", editor+` "$1"`, "sh", tmp.Name())
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "error: there was a problem with the editor")
		return 1
	}
	after, _ := os.ReadFile(tmp.Name())
	if string(after) == before {
		fmt.Println("Edit cancelled, no changes made.")
		return 0
	}
	var edited obj
	if err := yaml.Unmarshal(after, &edited); err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		return 1
	}
	return mutate(func(w *world) int {
		c := newCl(w, ctx)
		o := c.find(k.Kind, ns, nm)
		if o == nil {
			return 1
		}
		e := deepCopy(edited)
		if s, ok := e["spec"].(obj); ok && k.Kind != "Pod" {
			o["spec"] = s
		}
		for _, field := range []string{"labels", "annotations"} {
			if v, ok := get(e, "metadata", field).(obj); ok {
				sub(o, "metadata")[field] = v
			}
		}
		if d, ok := e["data"].(obj); ok {
			o["data"] = d
		}
		c.reconcile()
		fmt.Printf("%s/%s edited\n", k.nameRef(), nm)
		return 0
	})
}

func execCmd(f flags) int {
	if len(f.pos) < 2 {
		return unknown("kubectl", f.raw)
	}
	pod := f.pos[1]
	prompt := "root@" + pod + ":/app# "
	fmt.Print(prompt)
	buf := make([]byte, 1)
	var line []byte
	for {
		n, err := os.Stdin.Read(buf)
		if n == 0 || err != nil {
			return 0
		}
		switch buf[0] {
		case '\r', '\n':
			fmt.Print("\r\n")
			cmd := strings.TrimSpace(string(line))
			line = nil
			switch {
			case cmd == "exit" || cmd == "logout":
				return 0
			case cmd == "hostname":
				fmt.Print(pod + "\r\n")
			case cmd == "ls":
				fmt.Print("app  config  healthz\r\n")
			case cmd == "id":
				fmt.Print("uid=0(root) gid=0(root) groups=0(root)\r\n")
			case strings.HasPrefix(cmd, "env"):
				fmt.Print("HOSTNAME=" + pod + "\r\nPORT=8080\r\nLOG_LEVEL=info\r\n")
			case cmd != "":
				fmt.Print("sh: " + strings.Fields(cmd)[0] + ": not found\r\n")
			}
			fmt.Print(prompt)
		case 127, 8:
			if len(line) > 0 {
				line = line[:len(line)-1]
				fmt.Print("\b \b")
			}
		case 4:
			return 0
		default:
			line = append(line, buf[0])
			os.Stdout.Write(buf)
		}
	}
}
