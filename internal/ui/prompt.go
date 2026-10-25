package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type tokenRole int

const (
	roleText tokenRole = iota
	roleVerb
	roleResource
	roleFlag
	roleValue
)

func roleStyle(r tokenRole) lipgloss.Style {
	switch r {
	case roleVerb:
		return lipgloss.NewStyle().Foreground(th.Accent).Bold(true)
	case roleResource:
		return lipgloss.NewStyle().Foreground(th.Accent2)
	case roleFlag:
		return lipgloss.NewStyle().Foreground(th.Accent3)
	case roleValue:
		return dimStyle
	}
	return itemStyle
}

func kubectlRoles(line string) []tokenRole {
	runes := []rune(line)
	roles := make([]tokenRole, len(runes))
	words, takesValue := 0, false
	for i := 0; i < len(runes); {
		if runes[i] == ' ' {
			i++
			continue
		}
		j := i
		for j < len(runes) && runes[j] != ' ' {
			j++
		}
		word := string(runes[i:j])
		role := roleText
		switch {
		case takesValue:
			role, takesValue = roleValue, false
		case strings.HasPrefix(word, "-"):
			role = roleFlag
			name, _, hasValue := strings.Cut(word, "=")
			takesValue = !hasValue && (valueFlags[name] || name == "-o" || name == "--output")
			if hasValue {

				k := i + len([]rune(name))
				for x := i; x < j; x++ {
					roles[x] = roleFlag
					if x > k {
						roles[x] = roleValue
					}
				}
				i = j
				continue
			}
		case words == 0:
			role, words = roleVerb, 1
		case words == 1:
			role, words = roleResource, 2
		}
		for x := i; x < j; x++ {
			roles[x] = role
		}
		i = j
	}
	return roles
}

func (m model) targetChip() string {
	ns := or(m.namespace, "all namespaces")
	if m.dangerous() {
		return lipgloss.NewStyle().Foreground(th.Danger).Bold(true).Render(skull+" "+m.context) + dangerStyle().Render(" · "+ns)
	}
	return titleStyle.Render("◉ "+or(m.context, "no context")) + dimStyle.Render(" · ") + itemStyle.Render(ns)
}

func dangerStyle() lipgloss.Style { return lipgloss.NewStyle().Foreground(th.Danger) }

func (m model) kubectlPrompt(innerW int) string {
	mark := keyStyle
	if m.dangerous() {
		mark = dangerStyle().Bold(true)
	}
	chip := m.targetChip()
	prompt := mark.Render("❯ ") + dimStyle.Render("kubectl ")
	lead := chip + "  " + prompt
	if innerW-lipgloss.Width(lead) < 16 {
		return clipLine(chip, innerW) + "\n" + m.commandLine(prompt, innerW)
	}
	return m.commandLine(lead, innerW)
}

func (m model) commandLine(lead string, innerW int) string {
	value := []rune(m.sh.Value())
	pos := min(max(m.sh.Position(), 0), len(value))
	room := max(innerW-lipgloss.Width(lead), 4)
	focused := m.sh.Focused()
	if len(value) == 0 {
		cur := " "
		if ph := []rune(m.sh.Placeholder); len(ph) > 0 {
			cur = string(ph[0])
			rest := string(ph[1:])
			if !focused {
				return lead + dimStyle.Render(clipLine(string(ph), room))
			}
			return lead + dimStyle.Reverse(true).Render(cur) + dimStyle.Render(clipLine(rest, room-1))
		}
		if !focused {
			return lead
		}
		return lead + itemStyle.Reverse(true).Render(cur)
	}
	roles := kubectlRoles(string(value))

	from := 0
	if pos >= room {
		from = pos - room + 1
	}
	to := min(len(value), from+room)
	var b strings.Builder
	b.WriteString(lead)
	for i := from; i < to; {
		if focused && i == pos {
			b.WriteString(roleStyle(roles[i]).Reverse(true).Render(string(value[i])))
			i++
			continue
		}
		j := i + 1
		for j < to && roles[j] == roles[i] && !(focused && j == pos) {
			j++
		}
		b.WriteString(roleStyle(roles[i]).Render(string(value[i:j])))
		i = j
	}
	if focused && pos == len(value) && to-from < room {
		b.WriteString(itemStyle.Reverse(true).Render(" "))
	}
	return b.String()
}
