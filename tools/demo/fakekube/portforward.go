package main

import (
	"fmt"
	"net"
	"os"
	"strings"
	"time"
)

func portForward(f flags) int {
	if len(f.pos) < 3 {
		return unknown("kubectl", f.raw)
	}
	_, ctx, ns := f.target()
	ref, ports := f.pos[1], f.pos[2]
	kn, nm, ok := strings.Cut(ref, "/")
	if !ok {
		kn, nm = "pod", ref
	}
	k, found := lookupKind(kn)
	if !found {
		fmt.Fprintf(os.Stderr, "error: the server doesn't have a resource type %q\n", kn)
		return 1
	}
	local, remote, _ := strings.Cut(ports, ":")
	pod := resolvePod(ctx, ns, k, nm)
	if pod == "" {
		if k.Kind == "Pod" {
			fmt.Fprintf(os.Stderr, "Error from server (NotFound): pods %q not found\n", nm)
		} else {
			fmt.Fprintf(os.Stderr, "error: unable to forward port because pod is not running. Current status=Pending\n")
		}
		return 1
	}
	host := map[string]string{"kind-dev": "https://127.0.0.1:41235", "staging-eu-1": "https://api.staging-eu-1.example.com", "prod-eu-1": "https://api.prod-eu-1.example.com"}[ctx]
	ln, err := net.Listen("tcp", "127.0.0.1:"+local)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to listen on port %s: Listeners failed to create with the following errors: [unable to create listener: Error listen tcp4 127.0.0.1:%s: bind: address already in use]\n", local, local)
		fmt.Fprintf(os.Stderr, "error: unable to listen on any of the requested ports: [{%s %s}]\n", local, remote)
		return 1
	}
	now := time.Now().Format("0102 15:04:05.000000")
	fmt.Fprintf(os.Stderr, "I%s   %d round_trippers.go:553] GET %s/api/v1/namespaces/%s/pods/%s 200 OK in 9 milliseconds\n", now, os.Getpid(), host, ns, pod)
	fmt.Fprintf(os.Stderr, "I%s   %d round_trippers.go:553] POST %s/api/v1/namespaces/%s/pods/%s/portforward 101 Switching Protocols in 21 milliseconds\n", now, os.Getpid(), host, ns, pod)
	fmt.Printf("Forwarding from 127.0.0.1:%s -> %s\n", local, remote)
	fmt.Printf("Forwarding from [::1]:%s -> %s\n", local, remote)
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			fmt.Printf("Handling connection for %s\n", local)
			go func(c net.Conn) {
				defer c.Close()
				c.SetDeadline(time.Now().Add(2 * time.Second))
				buf := make([]byte, 1024)
				c.Read(buf)
				body := fmt.Sprintf(`{"pod":%q,"namespace":%q,"status":"ok"}`+"\n", pod, ns)
				fmt.Fprintf(c, "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: %d\r\nConnection: close\r\n\r\n%s", len(body), body)
			}(conn)
		}
	}()
	for {
		time.Sleep(time.Second)
		if !podAlive(ctx, ns, pod) {
			ln.Close()
			fmt.Fprintf(os.Stderr, "E%s   %d portforward.go:413] an error occurred forwarding %s -> %s: error forwarding port %s to pod %s, uid : network namespace for sandbox is closed\n",
				time.Now().Format("0102 15:04:05.000000"), os.Getpid(), local, remote, remote, uid(pod))
			fmt.Fprintln(os.Stderr, "error: lost connection to pod")
			return 1
		}
	}
}

func resolvePod(ctx, ns string, k kindInfo, nm string) string {
	view := snapshot(ctx)
	ready := func(p obj) bool {
		if terminating(p) || str(p, "status", "phase") != "Running" {
			return false
		}
		for _, c := range list(p, "status", "conditions") {
			if str(c, "type") == "Ready" {
				return str(c, "status") == "True"
			}
		}
		return false
	}
	var sel map[string]string
	switch k.Kind {
	case "Pod":
		for _, p := range view {
			if str(p, "kind") == "Pod" && nsOf(p) == ns && objName(p) == nm && !terminating(p) {
				return nm
			}
		}
		return ""
	case "Service":
		for _, o := range view {
			if str(o, "kind") == "Service" && nsOf(o) == ns && objName(o) == nm {
				sel = labels(o, "spec", "selector")
			}
		}
	default:
		for _, o := range view {
			if str(o, "kind") == k.Kind && nsOf(o) == ns && objName(o) == nm {
				sel = labels(o, "spec", "selector", "matchLabels")
			}
		}
	}
	if len(sel) == 0 {
		return ""
	}
	var first string
	for _, p := range view {
		if str(p, "kind") != "Pod" || nsOf(p) != ns {
			continue
		}
		lbl := labels(p, "metadata", "labels")
		match := true
		for k, v := range sel {
			if lbl[k] != v {
				match = false
			}
		}
		if match && ready(p) && (first == "" || objName(p) < first) {
			first = objName(p)
		}
	}
	return first
}

func podAlive(ctx, ns, pod string) bool {
	w := readOnly()
	c := newCl(w, ctx)
	o := c.find("pod", ns, pod)
	return o != nil && !terminating(o)
}
