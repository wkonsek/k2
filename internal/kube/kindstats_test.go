package kube

import "testing"

func TestParseDockerStatsAndSum(t *testing.T) {
	out := "idk2aaaaaaaaaa\tk2-control-plane\t4.20%\t1.400GiB / 8.000GiB\t17.50%\n" +
		"idlabcpbbbbbbbb\tlab-control-plane\t1.00%\t200MiB / 8.000GiB\t2.44%\n" +
		"idlabw1bbbbbbbb\tlab-worker\t3.50%\t300MiB / 8.000GiB\t3.66%\n"
	stats := ParseDockerStats(out)
	if len(stats) != 3 {
		t.Fatalf("rows %d", len(stats))
	}
	if stats[0].CPU < 4.19 || stats[0].CPU > 4.21 || stats[0].MemUsed < 1_300_000_000 || stats[0].MemUsed > 1_600_000_000 {
		t.Fatalf("k2 row %+v", stats[0])
	}
	nodes := map[string]KindNodes{
		"k2":  {Containers: []string{"idk2aaaaaaaaaa"}},
		"lab": {Containers: []string{"idlabcpbbbbbbbb", "idlabw1bbbbbbbb"}},
	}
	got := UsageByCluster(stats, nodes)
	k2, ok := got["k2"]
	if !ok || !k2.OK || k2.CPU < 4.19 || k2.CPU > 4.21 {
		t.Fatalf("k2 usage %+v ok=%v", k2, ok)
	}
	lab := got["lab"]
	if !lab.OK || lab.CPU < 4.49 || lab.CPU > 4.51 {
		t.Fatalf("lab cpu %+v", lab)
	}
	wantMem := int64(200<<20 + 300<<20)
	if lab.MemUsed < wantMem-1024 || lab.MemUsed > wantMem+1024 {
		t.Fatalf("lab mem %d want ~%d", lab.MemUsed, wantMem)
	}
	if lab.MemPct < 6.0 || lab.MemPct > 6.2 {
		t.Fatalf("lab mem pct %v", lab.MemPct)
	}
}

func TestParseDockerBytesAndFormat(t *testing.T) {
	cases := map[string]int64{
		"1.400GiB": 1503238554,
		"200MiB":   200 << 20,
		"512KiB":   512 << 10,
		"100B":     100,
		"2GB":      2_000_000_000,
	}
	for in, want := range cases {
		got, ok := ParseDockerBytes(in)
		if !ok {
			t.Errorf("ParseDockerBytes(%q) failed", in)
			continue
		}
		if in == "1.400GiB" {
			if got < 1_490_000_000 || got > 1_520_000_000 {
				t.Errorf("ParseDockerBytes(%q)=%d", in, got)
			}
			continue
		}
		if got != want {
			t.Errorf("ParseDockerBytes(%q)=%d want %d", in, got, want)
		}
	}
	if FormatBytes(1503238554) != "1.4 GiB" {
		t.Fatalf("FormatBytes GiB %q", FormatBytes(1503238554))
	}
	if FormatBytes(200<<20) != "200 MiB" {
		t.Fatalf("FormatBytes MiB %q", FormatBytes(200<<20))
	}
	if FormatCPU(4.2) != "4.2%" || FormatCPU(12.4) != "12%" {
		t.Fatalf("FormatCPU %q %q", FormatCPU(4.2), FormatCPU(12.4))
	}
}

func TestParseDockerMemUsage(t *testing.T) {
	used, limit := ParseDockerMemUsage("1.400GiB / 8.000GiB")
	if used < 1_300_000_000 || limit < 8_000_000_000 {
		t.Fatalf("usage %d / %d", used, limit)
	}
}

func TestDockerStatsArgs(t *testing.T) {
	args := DockerStatsArgs([]string{"abc", "def"})
	if len(args) < 5 || args[0] != "stats" || args[len(args)-1] != "def" {
		t.Fatalf("%v", args)
	}
}

func TestContainerIDMatch(t *testing.T) {
	full := "idk2aaaaaaaaaaabcdef"
	if !ContainerIDMatch(full, full[:12]) || !ContainerIDMatch(full[:12], full) || ContainerIDMatch("abc", "xyz") {
		t.Fatal("id match")
	}
}

func TestFormatCoresAndHostPct(t *testing.T) {
	cases := []struct {
		pct  float64
		cpus int
		want string
		host float64
	}{
		{139, 20, "1.4c (7%)", 6.95},
		{20, 20, "0.2c (1%)", 1},
		{100, 1, "1.0c (100%)", 100},
		{4.2, 20, "0.04c (0.2%)", 0.21},
		{0, 8, "0.0c (0%)", 0},
		{1800, 20, "18.0c (90%)", 90},
		{50, 0, "0.5c (50%)", 50},
	}
	for _, c := range cases {
		if got := FormatCores(c.pct, c.cpus); got != c.want {
			t.Fatalf("FormatCores(%v, %d) = %q, want %q", c.pct, c.cpus, got, c.want)
		}
		if got := HostCPUPct(c.pct, c.cpus); got < c.host-0.001 || got > c.host+0.001 {
			t.Fatalf("HostCPUPct(%v, %d) = %v, want %v", c.pct, c.cpus, got, c.host)
		}
	}
}
