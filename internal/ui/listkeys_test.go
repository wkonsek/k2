package ui

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/charmbracelet/bubbles/list"

	"k2/internal/config"
)

func TestListKeysComeFromConfig(t *testing.T) {
	km := listKeyMap(config.DefaultKeys())
	if !reflect.DeepEqual(km.NextPage.Keys(), []string{"pgdown", "}"}) || !reflect.DeepEqual(km.PrevPage.Keys(), []string{"pgup", "{"}) {
		t.Fatalf("paging keys %v / %v", km.NextPage.Keys(), km.PrevPage.Keys())
	}
	if km.Quit.Enabled() || km.ForceQuit.Enabled() {
		t.Fatal("a list must not quit k2 by itself")
	}
	items := make([]list.Item, 100)
	for i := range items {
		items[i] = item{title: fmt.Sprint(i), id: fmt.Sprint(i)}
	}
	l := list.New(items, rowDelegate{width: 40}, 40, 10)
	l.KeyMap = km
	for _, k := range []string{"d", "u", "f", "b", "l", "h", "ctrl+f", "ctrl+b"} {
		l.Select(50)
		l, _ = l.Update(key(t, k))
		if l.Index() != 50 {
			t.Errorf("%s moved the list to %d", k, l.Index())
		}
	}
	l.Select(0)
	l, _ = l.Update(key(t, "pgdown"))
	if l.Index() == 0 {
		t.Error("pgdown did not page")
	}
	at := l.Index()
	if l, _ = l.Update(key(t, "}")); l.Index() <= at {
		t.Error("} did not page down")
	}
	at = l.Index()
	if l, _ = l.Update(key(t, "{")); l.Index() >= at {
		t.Error("{ did not page up")
	}
	l, _ = l.Update(key(t, "G"))
	if l.Index() != 99 {
		t.Errorf("G went to %d", l.Index())
	}
}
