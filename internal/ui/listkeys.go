package ui

import (
	"k2/internal/config"

	bkey "github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
)

func listKeyMap(k config.Keys) list.KeyMap {
	km := list.DefaultKeyMap()
	km.CursorUp = bkey.NewBinding(bkey.WithKeys(k.ScrollUp...))
	km.CursorDown = bkey.NewBinding(bkey.WithKeys(k.ScrollDown...))
	km.PrevPage = bkey.NewBinding(bkey.WithKeys(k.PageUp...))
	km.NextPage = bkey.NewBinding(bkey.WithKeys(k.PageDown...))
	km.GoToStart = bkey.NewBinding(bkey.WithKeys(append([]string{"home"}, k.Top...)...))
	km.GoToEnd = bkey.NewBinding(bkey.WithKeys(append([]string{"end"}, k.Bottom...)...))
	km.Quit = bkey.NewBinding(bkey.WithDisabled())
	km.ForceQuit = bkey.NewBinding(bkey.WithDisabled())
	return km
}
