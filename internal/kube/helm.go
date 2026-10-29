package kube

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type HelmRelease struct {
	Name, Namespace string
	Revision        int
	Status          string
	Chart           string
	AppVersion      string
	Updated         time.Time
	Description     string
	Notes           string
	Manifest        string
	Config          map[string]any
	Defaults        map[string]any
}

type helmRelease struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Version   int    `json:"version"`
	Info      struct {
		Status       string `json:"status"`
		LastDeployed string `json:"last_deployed"`
		Description  string `json:"description"`
		Notes        string `json:"notes"`
	} `json:"info"`
	Chart struct {
		Metadata struct {
			Name       string `json:"name"`
			Version    string `json:"version"`
			AppVersion string `json:"appVersion"`
		} `json:"metadata"`
		Values map[string]any `json:"values"`
	} `json:"chart"`
	Config   map[string]any `json:"config"`
	Manifest string         `json:"manifest"`
}

func DecodeHelmRelease(data string) (HelmRelease, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(data))
	if err != nil {
		return HelmRelease{}, fmt.Errorf("release: not base64: %w", err)
	}

	if len(raw) > 0 && raw[0] != '{' && !(len(raw) > 1 && raw[0] == 0x1f && raw[1] == 0x8b) {
		if inner, err := base64.StdEncoding.DecodeString(string(raw)); err == nil {
			raw = inner
		}
	}
	if len(raw) > 1 && raw[0] == 0x1f && raw[1] == 0x8b {
		zr, err := gzip.NewReader(bytes.NewReader(raw))
		if err != nil {
			return HelmRelease{}, fmt.Errorf("release: gzip: %w", err)
		}

		raw, err = io.ReadAll(io.LimitReader(zr, 64<<20))
		if err != nil {
			return HelmRelease{}, fmt.Errorf("release: gzip: %w", err)
		}
	}
	var r helmRelease
	if err := json.Unmarshal(raw, &r); err != nil {
		return HelmRelease{}, fmt.Errorf("release: json: %w", err)
	}
	out := HelmRelease{
		Name: r.Name, Namespace: r.Namespace, Revision: r.Version, Status: r.Info.Status,
		AppVersion: r.Chart.Metadata.AppVersion, Description: r.Info.Description, Notes: r.Info.Notes,
		Manifest: r.Manifest, Config: r.Config, Defaults: r.Chart.Values,
	}
	if r.Chart.Metadata.Name != "" {
		out.Chart = r.Chart.Metadata.Name + "-" + r.Chart.Metadata.Version
	}
	out.Updated, _ = time.Parse(time.RFC3339Nano, r.Info.LastDeployed)
	return out, nil
}

func (c Client) HelmSecrets(kubeconfig, context, namespace string) ([]Object, error) {
	return c.ObjectsWhere(kubeconfig, context, "secrets", namespace, "owner=helm")
}

func helmRev(o Object) (name string, rev int, status string) {
	l := labelsOf(o.Get("metadata", "labels"))
	rev, _ = strconv.Atoi(l["version"])
	return l["name"], rev, l["status"]
}

func HelmLatest(secrets []Object) []HelmRelease {
	newest := map[string]Object{}
	for _, s := range secrets {
		name, rev, _ := helmRev(s)
		if name == "" {
			continue
		}
		key := s.Metadata.Namespace + "/" + name
		if cur, ok := newest[key]; !ok {
			newest[key] = s
		} else if _, r, _ := helmRev(cur); rev > r {
			newest[key] = s
		}
	}
	var out []HelmRelease
	for _, s := range newest {
		r, err := DecodeHelmRelease(s.Str("data", "release"))
		name, rev, status := helmRev(s)
		if err != nil {

			r = HelmRelease{Name: name, Revision: rev, Status: status + " (unreadable)"}
		}
		if r.Namespace == "" {
			r.Namespace = s.Metadata.Namespace
		}
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Namespace != out[j].Namespace {
			return out[i].Namespace < out[j].Namespace
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func HelmHistory(secrets []Object, namespace, name string) []HelmRelease {
	var out []HelmRelease
	for _, s := range secrets {
		n, rev, status := helmRev(s)
		if n != name || s.Metadata.Namespace != namespace {
			continue
		}
		r, err := DecodeHelmRelease(s.Str("data", "release"))
		if err != nil {
			r = HelmRelease{Name: n, Namespace: namespace, Revision: rev, Status: status + " (unreadable)"}
		}
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Revision > out[j].Revision })
	return out
}

func HelmTable(rs []HelmRelease, showNS bool) Table {
	heads := []string{"NAME", "REVISION", "STATUS", "CHART", "APP VERSION", "UPDATED"}
	color := 2
	if showNS {
		heads = append([]string{"NAMESPACE"}, heads...)
		color = 3
	}
	t := Table{Headers: heads, ColorCol: color}
	for _, r := range rs {
		row := []string{r.Name, strconv.Itoa(r.Revision), orDash(r.Status), orDash(r.Chart), orDash(r.AppVersion), helmTime(r.Updated)}
		if showNS {
			row = append([]string{r.Namespace}, row...)
		}
		t.Rows = append(t.Rows, row)
		t.Refs = append(t.Refs, [2]string{r.Namespace, r.Name})
		t.Born = append(t.Born, time.Time{})
		t.Extra = append(t.Extra, map[string]string{"revision": strconv.Itoa(r.Revision), "status": r.Status})
	}
	return t
}

func helmTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Local().Format("2006-01-02 15:04")
}

func HistoryText(rs []HelmRelease) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%-9s %-20s %-12s %-28s %-12s %s\n", "REVISION", "UPDATED", "STATUS", "CHART", "APP VERSION", "DESCRIPTION")
	for _, r := range rs {
		fmt.Fprintf(&b, "%-9d %-20s %-12s %-28s %-12s %s\n", r.Revision, helmTime(r.Updated), orDash(r.Status), orDash(r.Chart), orDash(r.AppVersion), r.Description)
	}
	return b.String()
}

func MergeValues(defaults, user map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range defaults {
		out[k] = v
	}
	for k, v := range user {
		if v == nil {
			delete(out, k)
			continue
		}
		um, uok := v.(map[string]any)
		dm, dok := out[k].(map[string]any)
		if uok && dok {
			out[k] = MergeValues(dm, um)
			continue
		}
		out[k] = v
	}
	return out
}

func ValuesYAML(v map[string]any) string {
	if len(v) == 0 {
		return "{}\n"
	}
	out, err := yaml.Marshal(v)
	if err != nil {
		return fmt.Sprintf("# could not render values: %v\n", err)
	}
	return string(out)
}

func HelmArgs(kubeconfig, context, namespace string) []string {
	return []string{"-n", namespace, "--kubeconfig", kubeconfig, "--kube-context", context}
}
