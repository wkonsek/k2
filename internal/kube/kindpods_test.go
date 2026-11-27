package kube

import "testing"

func TestParsePodCounts(t *testing.T) {
	out := "kube-system   coredns-aaa           1/1     Running   0     2h\n" +
		"kube-system   coredns-bbb           1/1     Running   0     2h\n" +
		"kube-system   etcd-k2               1/1     Running   0     2h\n" +
		"argocd        argocd-server-ccc     0/1     Running   2     10m\n" +
		"default       job-ddd               0/1     Completed 0     1m\n"
	p := ParsePodCounts(out)
	if !p.OK || p.Total != 5 || p.Running != 3 {
		t.Fatalf("got %+v", p)
	}
	if p.Label() != "3/5" {
		t.Fatalf("label %q", p.Label())
	}
	all := ParsePodCounts("ns  a  1/1  Running  0  1h\nns  b  2/2  Running  0  1h\n")
	if !all.OK || all.Total != 2 || all.Running != 2 || all.Label() != "2" {
		t.Fatalf("all ready %+v %q", all, all.Label())
	}
	empty := ParsePodCounts("")
	if !empty.OK || empty.Total != 0 || empty.Label() != "0" {
		t.Fatalf("empty %+v %q", empty, empty.Label())
	}
	noNS := ParsePodCounts("coredns  1/1  Running  0  2h\n")
	if !noNS.OK || noNS.Total != 1 || noNS.Running != 1 {
		t.Fatalf("no namespace %+v", noNS)
	}
	if (KindPods{}).Label() != "—" {
		t.Fatalf("zero value should be a dash")
	}
}
