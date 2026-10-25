package config

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"k2/internal/theme"

	"gopkg.in/yaml.v3"
)

type ContextTheme struct {
	Context    string
	Kubeconfig string
	Theme      string
}

func globRe(glob string) *regexp.Regexp {
	var b strings.Builder
	b.WriteString("^")
	for _, r := range glob {
		switch r {
		case '*':
			b.WriteString(".*")
		case '?':
			b.WriteString(".")
		default:
			b.WriteString(regexp.QuoteMeta(string(r)))
		}
	}
	b.WriteString("$")
	return regexp.MustCompile(b.String())
}

func Glob(pattern, s string) bool { return globRe(pattern).MatchString(s) }

func (e ContextTheme) Matches(kubeconfig, context string) bool {
	if e.Context != "" && !Glob(e.Context, context) {
		return false
	}
	if e.Kubeconfig != "" {
		if kubeconfig == "" {
			return false
		}
		return Glob(filepath.Clean(ExpandHome(e.Kubeconfig)), filepath.Clean(ExpandHome(kubeconfig)))
	}
	return true
}

func (c Config) ThemeFor(kubeconfig, context string) string {
	for _, e := range c.ContextThemes {
		if e.Matches(kubeconfig, context) {
			return e.Theme
		}
	}
	return c.Theme
}

func (p *parser) contextThemes(n *yaml.Node, dst *[]ContextTheme) {
	if n.Tag == "!!null" {
		return
	}
	if n.Kind != yaml.SequenceNode {
		p.warn("ui.context_themes: expected a list of {context, kubeconfig, theme} (line %d, ignored)", n.Line)
		return
	}
	for i, item := range n.Content {
		where := "ui.context_themes[" + strconv.Itoa(i+1) + "]"
		if item.Kind != yaml.MappingNode {
			p.warn("%s: expected {context: …, theme: …} (line %d, ignored)", where, item.Line)
			continue
		}
		var e ContextTheme
		ok := true
		for j := 0; j+1 < len(item.Content); j += 2 {
			k, v := item.Content[j].Value, item.Content[j+1]
			if v.Kind != yaml.ScalarNode {
				p.warn("%s.%s: expected a single value (line %d, entry ignored)", where, k, v.Line)
				ok = false
				continue
			}
			val := strings.TrimSpace(v.Value)
			switch k {
			case "context":
				e.Context = val
			case "kubeconfig":
				e.Kubeconfig = val
			case "theme":
				e.Theme = strings.ToLower(val)
			default:
				p.warn("%s: unknown field %q (have context, kubeconfig, theme; entry ignored)", where, k)
				ok = false
			}
		}
		switch {
		case !ok:
		case e.Theme == "":
			p.warn("%s: no theme (line %d, entry ignored)", where, item.Line)
		case e.Context == "" && e.Kubeconfig == "":
			p.warn("%s: needs a context or a kubeconfig pattern (line %d, entry ignored)", where, item.Line)
		default:
			if e.Theme != "custom" {
				if _, found := theme.Lookup(e.Theme); !found {
					p.warn("%s: unknown theme %q (entry ignored; have %s)", where, e.Theme, themeList())
					continue
				}
			}
			*dst = append(*dst, e)
		}
	}
}
