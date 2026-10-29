package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type logLine struct {
	at   time.Time
	text string
}

func period(pod, ctr string) time.Duration {
	return time.Duration(700+hashN(1800, pod, ctr)) * time.Millisecond
}

func flavor(image string) string {
	for _, k := range []string{"nginx", "kafka-connect", "kafka", "postgres", "redis", "grafana", "prometheus", "envoy", "argocd", "coredns", "schema-registry"} {
		if strings.Contains(image, k) {
			return k
		}
	}
	return "json"
}

func pick(i int, pod string, options ...string) string {
	return options[hashN(len(options), pod, strconv.Itoa(i))]
}

func lineText(fl, app, pod string, i int, at time.Time) string {
	h := func(n int) int { return hashN(n, pod, strconv.Itoa(i), "x") }
	paths := []string{"/", "/api/cart", "/api/products?page=" + strconv.Itoa(h(9)+1), "/checkout", "/static/app.js", "/healthz", "/api/orders"}
	switch fl {
	case "nginx":
		code := pick(i, pod, "200", "200", "200", "200", "304", "200", "201", "404")
		return fmt.Sprintf(`10.244.%d.%d - - [%s] "%s %s HTTP/1.1" %s %d "-" "Mozilla/5.0"`, h(3)+1, h(200)+10, at.Format("02/Jan/2006:15:04:05 +0000"),
			pick(i, pod, "GET", "GET", "GET", "POST"), paths[h(len(paths))], code, 180+h(9000))
	case "kafka":
		return fmt.Sprintf("[%s] INFO [%s] %s", at.Format("2006-01-02 15:04:05,000"),
			pick(i, pod, "ReplicaFetcher replicaId=1, leaderId=0, fetcherId=0", "Log partition=orders-"+strconv.Itoa(h(6)), "GroupCoordinator 0", "LogCleaner"),
			pick(i, pod, "Rolled new log segment at offset "+strconv.Itoa(120000+h(90000)), "Member checkout-consumer-"+strconv.Itoa(h(4))+" joined group checkout", "Incremental fetch session established", "Deleted log segment, size 1048576"))
	case "kafka-connect":
		return fmt.Sprintf("[%s] INFO [orders-sink|task-%d] %s", at.Format("2006-01-02 15:04:05,000"), h(3),
			pick(i, pod, "Committing offsets asynchronously", "WorkerSinkTask{id=orders-sink-0} Committing offsets", "Flushed 128 records to sink", "Finished commitOffsets successfully in 4 ms"))
	case "postgres":
		return at.Format("2006-01-02 15:04:05.000 UTC") + " [" + strconv.Itoa(40+h(900)) + "] " +
			pick(i, pod, "LOG:  checkpoint starting: time", "LOG:  checkpoint complete: wrote 412 buffers (2.5%)", "LOG:  connection authorized: user=payments database=payments", "LOG:  duration: 3.112 ms  statement: SELECT 1")
	case "redis":
		return "1:M " + at.Format("02 Jan 2006 15:04:05.000") + " * " + pick(i, pod, "100 changes in 300 seconds. Saving...", "Background saving started by pid 412", "DB saved on disk", "Background saving terminated with success")
	case "grafana":
		return fmt.Sprintf(`logger=context userId=1 orgId=1 uname=admin t=%s level=info msg="Request Completed" method=GET path=%s status=200 duration=%dms`,
			at.Format(time.RFC3339), pick(i, pod, "/api/dashboards/uid/shop", "/api/datasources/proxy/1/api/v1/query_range", "/api/search"), 2+h(80))
	case "prometheus":
		return fmt.Sprintf(`ts=%s caller=head.go:%d level=info component=tsdb msg="%s"`, at.Format(time.RFC3339Nano), 700+h(400),
			pick(i, pod, "Head GC completed", "Creating checkpoint", "WAL checkpoint complete", "Compaction completed"))
	case "envoy":
		return fmt.Sprintf("[%s][%d][warning][config] [source/common/config/grpc_stream.h:%d] StreamAggregatedResources gRPC config stream to xds closed: 14, upstream connect error", at.Format("2006-01-02 15:04:05.000"), 1, 190+h(20))
	}
	msgs := map[string][]string{
		"cart":     {"cart updated", "cart loaded", "item added", "price lookup", "cart expired"},
		"checkout": {"order placed", "payment authorized", "order validated", "stock reserved"},
		"api":      {"payment captured", "payment authorized", "refund issued", "webhook delivered"},
		"worker":   {"settlement batch done", "job picked up", "ledger synced"},
	}[app]
	if msgs == nil {
		msgs = []string{"request served", "cache hit", "sync done", "reconciled"}
	}
	lvl := pick(i, pod, "info", "info", "info", "info", "info", "debug", "warn")
	return fmt.Sprintf(`{"level":%q,"ts":%q,"msg":%q,"order":"A-%d","ms":%d}`, lvl, at.Format(time.RFC3339Nano), msgs[h(len(msgs))], 10200+h(9000), 3+h(140))
}

func containerLines(p obj, c any, from, to time.Time, tail int) []logLine {
	pod := objName(p)
	cn := str(c, "name")
	born := parseTS(str(p, "status", "startTime")).Add(time.Second)
	if born.IsZero() {
		return nil
	}
	per := period(pod, cn)
	end := to

	if str(p, "status", "phase") == "Succeeded" {
		end = born.Add(14 * time.Second)
	}
	if end.After(to) {
		end = to
	}
	last := int(end.Sub(born) / per)
	first := 0
	if !from.IsZero() {
		first = int(from.Sub(born)/per) + 1
		if first < 0 {
			first = 0
		}
	}
	if tail >= 0 && last-first+1 > tail {
		first = last - tail + 1
	}
	if last-first > 2000 {
		first = last - 2000
	}
	fl := flavor(str(c, "image"))
	app := labels(p, "metadata", "labels")["app"]
	var out []logLine
	for i := first; i <= last; i++ {
		at := born.Add(time.Duration(i) * per).Add(time.Duration(hashN(999, pod, strconv.Itoa(i))) * time.Microsecond)
		if !from.IsZero() && !at.After(from) {
			continue
		}
		out = append(out, logLine{at, lineText(fl, app, pod, i, at)})
	}
	return out
}

func logsCmd(f flags) int {
	if len(f.pos) < 2 {
		return unknown("kubectl", f.raw)
	}
	target := f.pos[1]
	target = strings.TrimPrefix(strings.TrimPrefix(target, "pod/"), "pods/")
	_, ctx, ns := f.target()
	var p obj
	for _, o := range snapshot(ctx) {
		if str(o, "kind") == "Pod" && nsOf(o) == ns && objName(o) == target {
			p = o
		}
	}
	if p == nil {
		fmt.Fprintf(os.Stderr, "Error from server (NotFound): pods %q not found\n", target)
		return 1
	}
	if str(p, "status", "phase") == "Pending" {
		fmt.Fprintf(os.Stderr, "Error from server (BadRequest): container %q in pod %q is waiting to start: ContainerCreating\n", str(list(p, "spec", "containers")[0], "name"), target)
		return 1
	}
	cs := list(p, "spec", "containers")
	if f.container != "" {
		var only []any
		for _, c := range append(list(p, "spec", "initContainers"), cs...) {
			if str(c, "name") == f.container {
				only = append(only, c)
			}
		}
		if only == nil {
			fmt.Fprintf(os.Stderr, "error: container %s is not valid for pod %s\n", f.container, target)
			return 1
		}
		cs = only
	} else if !f.bools["--all-containers"] && len(cs) > 1 {
		cs = cs[:1]
	}
	now := time.Now()
	var from time.Time
	tail := -1
	if v := f.values["--since-time"]; v != "" {
		from = parseTS(v)
	} else if v := f.values["--since"]; v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			from = now.Add(-d)
		}
	}
	if v := f.values["--tail"]; v != "" {
		tail, _ = strconv.Atoi(v)
	}
	if from.IsZero() && tail < 0 {
		tail = 400
	}
	var b strings.Builder
	for _, c := range cs {
		for _, l := range containerLines(p, c, from, now, tail) {
			if f.bools["--prefix"] {
				fmt.Fprintf(&b, "[pod/%s/%s] ", target, str(c, "name"))
			}
			if f.bools["--timestamps"] {
				b.WriteString(l.at.UTC().Format(time.RFC3339Nano) + " ")
			}
			b.WriteString(l.text + "\n")
		}
	}
	os.Stdout.WriteString(b.String())
	return 0
}
