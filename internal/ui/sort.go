package ui

import (
	"sort"
	"strconv"
	"strings"

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

func lessBy(a, b item, col int, age bool) bool {
	if age {
		return a.born.After(b.born)
	}
	ca, cb := "", ""
	if col < len(a.cells) {
		ca = a.cells[col]
	}
	if col < len(b.cells) {
		cb = b.cells[col]
	}
	na, sa, nok := sortValue(ca)
	nb, sb, mok := sortValue(cb)
	if col < len(a.heads) && isMetricHead(a.heads[col]) {

		parse := kube.ParseCPU
		if a.heads[col] == "MEM" {
			parse = kube.ParseMem
		}
		na, nok = parse(ca)
		nb, mok = parse(cb)
	}
	switch {
	case nok && mok:
		return na < nb
	case nok != mok:
		return nok
	}
	return sa < sb
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
	out := append([]list.Item(nil), items...)
	sort.SliceStable(out, func(i, j int) bool {
		a, aok := out[i].(item)
		b, bok := out[j].(item)
		if !aok || !bok {
			return false
		}
		if desc {
			return lessBy(b, a, idx, age)
		}
		return lessBy(a, b, idx, age)
	})
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
