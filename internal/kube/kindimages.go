package kube

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	KindNodeRepo       = "kindest/node"
	KindNodeTagsURL    = "https://hub.docker.com/v2/repositories/kindest/node/tags?page_size=50&ordering=last_updated"
	KindNodeTagLimit   = 5
	kindNodeTagTimeout = 4 * time.Second
	kindNodeTagMaxBody = 1 << 20
)

func KindNodeImage(tag string) string {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return ""
	}
	if strings.Contains(tag, "/") || strings.Contains(tag, ":") {
		return tag
	}
	return KindNodeRepo + ":" + tag
}

func ParseKindestNodeTag(name string) (major, minor, patch int, ok bool) {
	name = strings.TrimSpace(name)
	if strings.HasPrefix(name, KindNodeRepo+":") {
		name = strings.TrimPrefix(name, KindNodeRepo+":")
	}
	if !strings.HasPrefix(name, "v") {
		return 0, 0, 0, false
	}
	parts := strings.Split(name[1:], ".")
	if len(parts) != 3 {
		return 0, 0, 0, false
	}
	for _, p := range parts {
		if p == "" {
			return 0, 0, 0, false
		}
		for i := 0; i < len(p); i++ {
			if p[i] < '0' || p[i] > '9' {
				return 0, 0, 0, false
			}
		}
	}
	major, err1 := strconv.Atoi(parts[0])
	minor, err2 := strconv.Atoi(parts[1])
	patch, err3 := strconv.Atoi(parts[2])
	if err1 != nil || err2 != nil || err3 != nil {
		return 0, 0, 0, false
	}
	return major, minor, patch, true
}

func PickLatestKindNodeTags(names []string, n int) []string {
	type ver struct {
		maj, min, pat int
		name          string
	}
	best := map[[2]int]ver{}
	for _, name := range names {
		name = strings.TrimSpace(name)
		if strings.HasPrefix(name, KindNodeRepo+":") {
			name = strings.TrimPrefix(name, KindNodeRepo+":")
		}
		maj, min, pat, ok := ParseKindestNodeTag(name)
		if !ok {
			continue
		}
		key := [2]int{maj, min}
		if cur, hit := best[key]; !hit || pat > cur.pat {
			best[key] = ver{maj, min, pat, name}
		}
	}
	all := make([]ver, 0, len(best))
	for _, v := range best {
		all = append(all, v)
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].maj != all[j].maj {
			return all[i].maj > all[j].maj
		}
		if all[i].min != all[j].min {
			return all[i].min > all[j].min
		}
		return all[i].pat > all[j].pat
	})
	if n > 0 && len(all) > n {
		all = all[:n]
	}
	out := make([]string, len(all))
	for i, v := range all {
		out[i] = KindNodeImage(v.name)
	}
	return out
}

func ParseKindNodeImages(data []byte) []string {
	var parsed struct {
		Results []struct {
			Name string `json:"name"`
		} `json:"results"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil
	}
	names := make([]string, 0, len(parsed.Results))
	for _, r := range parsed.Results {
		names = append(names, r.Name)
	}
	return PickLatestKindNodeTags(names, KindNodeTagLimit)
}

func FetchKindNodeImages(client *http.Client, url string) ([]string, error) {
	if url == "" {
		url = KindNodeTagsURL
	}
	if client == nil {
		client = &http.Client{Timeout: kindNodeTagTimeout}
	}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, kindNodeTagMaxBody))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("kindest/node tags: %s", resp.Status)
	}
	return ParseKindNodeImages(data), nil
}
