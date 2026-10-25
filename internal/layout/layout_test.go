package layout

import (
	"reflect"
	"strings"
	"testing"
)

func show(n *Node) string {
	if n == nil {
		return "nil"
	}
	if n.Dir == Leaf {
		return n.Pane
	}
	var parts []string
	for _, k := range n.Kids {
		parts = append(parts, show(k))
	}
	name := "row"
	if n.Dir == Col {
		name = "col"
	}
	return name + "(" + strings.Join(parts, ",") + ")"
}

func TestSplitDownStacksLikeBefore(t *testing.T) {
	n := New("a")
	n = n.SplitDown("a", "b")
	n = n.SplitDown("b", "c")

	n = n.SplitDown("a", "d")
	if show(n) != "col(a,b,c,d)" || !n.Valid() {
		t.Fatalf("got %s", show(n))
	}
	if got := n.Leaves(); !reflect.DeepEqual(got, []string{"a", "b", "c", "d"}) {
		t.Fatalf("leaves %v", got)
	}
}

func TestSplitRightAndNesting(t *testing.T) {
	n := New("a").SplitRight("a", "b")
	if show(n) != "row(a,b)" {
		t.Fatalf("got %s", show(n))
	}
	n = n.SplitRight("a", "c")
	if show(n) != "row(a,c,b)" {
		t.Fatalf("got %s", show(n))
	}
	n = n.SplitDown("c", "d")
	if show(n) != "row(a,col(c,d),b)" {
		t.Fatalf("got %s", show(n))
	}
	n = n.SplitRight("d", "e")
	if show(n) != "row(a,col(c,row(d,e)),b)" || !n.Valid() {
		t.Fatalf("got %s", show(n))
	}
	if n.SplitRight("zz", "x") != n || n.Count() != 5 || !n.Has("e") || n.Has("zz") {
		t.Fatal("unknown pane must leave the tree alone")
	}
}

func TestRemoveCollapses(t *testing.T) {
	n := New("a").SplitRight("a", "b").SplitDown("b", "c").SplitRight("c", "d")
	if show(n) != "row(a,col(b,row(c,d)))" {
		t.Fatalf("got %s", show(n))
	}

	n = n.Remove("b")
	if show(n) != "row(a,c,d)" || !n.Valid() {
		t.Fatalf("got %s", show(n))
	}
	n = n.Remove("c").Remove("a")
	if show(n) != "d" {
		t.Fatalf("got %s", show(n))
	}
	if n.Remove("nope") != n {
		t.Fatal("removing an unknown pane changed the tree")
	}
	if n.Remove("d") != nil {
		t.Fatal("removing the last pane must give nil")
	}
	col := New("a").SplitDown("a", "b").SplitDown("b", "c")
	if show(col.Remove("b")) != "col(a,c)" {
		t.Fatalf("got %s", show(col))
	}
}

func TestCloneIsDeep(t *testing.T) {
	n := New("a").SplitRight("a", "b")
	c := n.Clone()
	c.SplitDown("b", "x")
	if show(n) != "row(a,b)" || show(c) != "row(a,col(b,x))" {
		t.Fatalf("%s / %s", show(n), show(c))
	}
}

func TestValid(t *testing.T) {
	bad := []*Node{
		nil,
		{Dir: Row, Kids: []*Node{New("a")}},
		{Dir: Col, Kids: []*Node{New("a"), New("a")}},
		{Dir: Leaf},
		{Dir: Dir(7)},
	}
	for _, n := range bad {
		if n.Valid() {
			t.Errorf("%s should be invalid", show(n))
		}
	}
}

func TestRects(t *testing.T) {
	n := New("a").SplitDown("a", "b").SplitDown("b", "c")
	r := n.Rects(Rect{0, 0, 80, 40})
	if r["a"] != (Rect{0, 0, 80, 14}) || r["b"] != (Rect{0, 14, 80, 13}) || r["c"] != (Rect{0, 27, 80, 13}) {
		t.Fatalf("%v", r)
	}
	m := New("a").SplitRight("a", "b").SplitDown("b", "c")
	r = m.Rects(Rect{2, 1, 81, 30})
	want := map[string]Rect{"a": {2, 1, 41, 30}, "b": {43, 1, 40, 15}, "c": {43, 16, 40, 15}}
	if !reflect.DeepEqual(r, want) {
		t.Fatalf("%v", r)
	}
}

func TestNeighbor(t *testing.T) {
	n := New("a").SplitDown("a", "d")
	n = n.SplitRight("a", "b").SplitDown("b", "c")
	if show(n) != "col(row(a,col(b,c)),d)" {
		t.Fatalf("got %s", show(n))
	}
	r := n.Rects(Rect{0, 0, 80, 40})
	order := n.Leaves()
	cases := []struct {
		from string
		mv   Move
		want string
	}{
		{"a", Right, "b"},
		{"c", Left, "a"},
		{"b", Down, "c"},
		{"c", Down, "d"},
		{"d", Up, "a"},
		{"a", Down, "d"},
		{"a", Left, ""},
		{"d", Down, ""},
		{"b", Up, ""},
		{"b", Left, "a"},
	}
	for _, c := range cases {
		if got := Neighbor(r, order, c.from, c.mv); got != c.want {
			t.Errorf("%s %d: got %q, want %q", c.from, c.mv, got, c.want)
		}
	}
	if Neighbor(r, order, "zz", Left) != "" {
		t.Fatal("unknown pane")
	}
}
