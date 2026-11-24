package ui

import (
	"sort"
	"strconv"
	"strings"
	"time"

	"k2/internal/kube"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/x/ansi"
)

func sortHeads(lst list.Model) []string {
	for _, raw := range lst.Items() {
		if it, ok := raw.(item); ok && len(it.heads) > 0 {
			return it.heads
		}
	}
	return nil
}

func cycleSort(heads []string, cur string, step int) string {
	opts := append([]string{""}, heads...)
	at := 0
	for i, h := range opts {
		if h == cur {
			at = i
		}
	}
	return opts[(at+step+len(opts))%len(opts)]
}

func sortValue(cell string) (float64, string, bool) {
	t := strings.TrimSpace(cell)
	head := strings.FieldsFunc(t, func(r rune) bool { return r == ' ' || r == '/' })
	if len(head) > 0 {
		if n, err := strconv.ParseFloat(head[0], 64); err == nil {

			frac := 0.0
			if i := strings.Index(t, "/"); i > 0 {
				if d, err := strconv.ParseFloat(strings.Fields(t[i+1:] + " ")[0], 64); err == nil {
					frac = d / 1e6
				}
			}
			return n + frac, "", true
		}
	}
	return 0, strings.ToLower(t), false
}

func sortItems(items []list.Item, col string, desc bool) []list.Item {
	if len(items) == 0 {
		return items
	}
	if col == "" {
		out := append([]list.Item(nil), items...)
		sort.SliceStable(out, func(i, j int) bool {
			a, aok := out[i].(item)
			b, bok := out[j].(item)
			return aok && bok && a.ord < b.ord
		})
		return out
	}
	first, ok := items[0].(item)
	if !ok {
		return items
	}
	idx := -1
	for i, h := range first.heads {
		if h == col {
			idx = i
		}
	}
	if idx < 0 {
		return items
	}
	age := isAgeHead(col)
	metric := isMetricHead(col)
	parse := kube.ParseCPU
	if metric && col == "MEM" {
		parse = kube.ParseMem
	}
	type key struct {
		num    float64
		str    string
		ok     bool
		okItem bool
		born   time.Time
		it     list.Item
	}
	keys := make([]key, len(items))
	for i, raw := range items {
		it, ok := raw.(item)
		k := key{it: raw, okItem: ok}
		if !ok {
			keys[i] = k
			continue
		}
		if age {
			k.born = it.born
		} else {
			cell := ""
			if idx < len(it.cells) {
				cell = it.cells[idx]
			}
			if metric {
				k.num, k.ok = parse(cell)
				if !k.ok {
					k.str = strings.ToLower(strings.TrimSpace(cell))
				}
			} else {
				k.num, k.str, k.ok = sortValue(cell)
			}
		}
		keys[i] = k
	}
	less := func(a, b key) bool {
		if !a.okItem || !b.okItem {
			return false
		}
		if age {
			return a.born.After(b.born)
		}
		switch {
		case a.ok && b.ok:
			return a.num < b.num
		case a.ok != b.ok:
			return a.ok
		}
		return a.str < b.str
	}
	sort.SliceStable(keys, func(i, j int) bool {
		if desc {
			return less(keys[j], keys[i])
		}
		return less(keys[i], keys[j])
	})
	out := make([]list.Item, len(keys))
	for i, k := range keys {
		out[i] = k.it
	}
	return out
}

func sortHeader(heads []string, widths []int, col string, desc bool) string {
	mark := "▲"
	if desc {
		mark = "▼"
	}
	parts := make([]string, len(heads))
	for i, h := range heads {
		w := 0
		if i < len(widths) {
			w = widths[i]
		}
		text := h
		if h == col && col != "" {
			if ansi.StringWidth(h)+1 <= w {
				text = h + mark
			} else if w > 1 {
				text = ansi.Truncate(h, w-1, "") + mark
			}
		}
		parts[i] = fitCell(text, w)
	}
	return joinCols(parts, widths)
}

func tableHeader(lst list.Model, vs viewState) string {
	if vs.sortCol == "" {
		return lst.Title
	}
	for _, raw := range lst.Items() {
		if it, ok := raw.(item); ok && len(it.heads) > 0 && len(it.widths) == len(it.heads) {
			return sortHeader(it.heads, it.widths, vs.sortCol, vs.sortDesc)
		}
	}
	return lst.Title
}

func setRows(lst *list.Model, header string, items []list.Item, vs viewState, pick string) {
	keep := pick
	if keep == "" {
		if cur := lst.SelectedItem(); cur != nil {
			if it, ok := cur.(item); ok {
				keep = it.id
			}
		}
	}
	lst.Title = header
	if cmd := lst.SetItems(sortItems(items, vs.sortCol, vs.sortDesc)); cmd != nil {
		*lst, _ = lst.Update(cmd())
	}
	if keep == "" {
		return
	}
	for i, raw := range lst.VisibleItems() {
		if it, ok := raw.(item); ok && it.id == keep {
			lst.Select(i)
			return
		}
	}
}

func (m *model) resort() {
	if m.content.FilterState() != list.Unfiltered {
		m.content.ResetFilter()
	}
	sel := ""
	if cur := m.content.SelectedItem(); cur != nil {
		if it, ok := cur.(item); ok {
			sel = it.id
		}
	}
	setRows(&m.content, m.content.Title, m.content.Items(), m.vs, sel)
	switch {
	case m.vs.sortCol == "":
		m.status = "kubectl order"
	case m.vs.sortDesc:
		m.status = "sorted by " + m.vs.sortCol + " ▼"
	default:
		m.status = "sorted by " + m.vs.sortCol + " ▲"
	}
}
