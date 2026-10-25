package layout

type Dir int

const (
	Leaf Dir = iota
	Row
	Col
)

type Node struct {
	Dir  Dir
	Pane string
	Kids []*Node
}

func New(id string) *Node { return &Node{Dir: Leaf, Pane: id} }

func (n *Node) Leaves() []string {
	if n == nil {
		return nil
	}
	if n.Dir == Leaf {
		return []string{n.Pane}
	}
	var out []string
	for _, k := range n.Kids {
		out = append(out, k.Leaves()...)
	}
	return out
}

func (n *Node) Count() int { return len(n.Leaves()) }

func (n *Node) Has(id string) bool {
	_, _, ok := n.find(id)
	return ok
}

func (n *Node) find(id string) (parent *Node, idx int, ok bool) {
	if n == nil {
		return nil, 0, false
	}
	if n.Dir == Leaf {
		return nil, 0, n.Pane == id
	}
	for i, k := range n.Kids {
		if k.Dir == Leaf && k.Pane == id {
			return n, i, true
		}
		if p, j, ok := k.find(id); ok && p != nil {
			return p, j, true
		}
	}
	return nil, 0, false
}

func (n *Node) Clone() *Node {
	if n == nil {
		return nil
	}
	c := &Node{Dir: n.Dir, Pane: n.Pane}
	for _, k := range n.Kids {
		c.Kids = append(c.Kids, k.Clone())
	}
	return c
}

func (n *Node) SplitDown(at, id string) *Node {
	parent, _, ok := n.find(at)
	switch {
	case !ok:
		return n
	case parent != nil && parent.Dir == Col:
		parent.Kids = append(parent.Kids, New(id))
		return n
	}
	return n.wrap(at, id, Col)
}

func (n *Node) SplitRight(at, id string) *Node {
	parent, i, ok := n.find(at)
	switch {
	case !ok:
		return n
	case parent != nil && parent.Dir == Row:
		parent.Kids = append(parent.Kids[:i+1], append([]*Node{New(id)}, parent.Kids[i+1:]...)...)
		return n
	}
	return n.wrap(at, id, Row)
}

func (n *Node) wrap(at, id string, dir Dir) *Node {
	box := &Node{Dir: dir, Kids: []*Node{New(at), New(id)}}
	parent, i, _ := n.find(at)
	if parent == nil {
		return box
	}
	parent.Kids[i] = box
	return n
}

func (n *Node) Remove(id string) *Node {
	parent, i, ok := n.find(id)
	if !ok {
		return n
	}
	if parent == nil {
		return nil
	}
	parent.Kids = append(parent.Kids[:i], parent.Kids[i+1:]...)
	return n.normalize()
}

func (n *Node) normalize() *Node {
	if n == nil || n.Dir == Leaf {
		return n
	}
	var kids []*Node
	for _, k := range n.Kids {
		k = k.normalize()
		if k == nil {
			continue
		}
		if k.Dir == n.Dir {
			kids = append(kids, k.Kids...)
			continue
		}
		kids = append(kids, k)
	}
	switch len(kids) {
	case 0:
		return nil
	case 1:
		return kids[0]
	}
	n.Kids = kids
	return n
}

func (n *Node) Valid() bool {
	seen := map[string]bool{}
	var walk func(*Node) bool
	walk = func(x *Node) bool {
		if x == nil {
			return false
		}
		switch x.Dir {
		case Leaf:
			if x.Pane == "" || seen[x.Pane] || len(x.Kids) > 0 {
				return false
			}
			seen[x.Pane] = true
			return true
		case Row, Col:
			if len(x.Kids) < 2 {
				return false
			}
			for _, k := range x.Kids {
				if !walk(k) {
					return false
				}
			}
			return true
		}
		return false
	}
	return walk(n)
}

type Rect struct{ X, Y, W, H int }

func (n *Node) Rects(r Rect) map[string]Rect {
	out := map[string]Rect{}
	n.place(r, out)
	return out
}

func (n *Node) place(r Rect, out map[string]Rect) {
	if n == nil {
		return
	}
	if n.Dir == Leaf {
		out[n.Pane] = r
		return
	}
	total := r.H
	if n.Dir == Row {
		total = r.W
	}
	at := 0
	for i, size := range Shares(total, len(n.Kids)) {
		k := n.Kids[i]
		c := r
		if n.Dir == Row {
			c.X, c.W = r.X+at, size
		} else {
			c.Y, c.H = r.Y+at, size
		}
		k.place(c, out)
		at += size
	}
}

func Shares(total, n int) []int {
	if n < 1 {
		return nil
	}
	out := make([]int, n)
	for i := range out {
		out[i] = total / n
		if i < total%n {
			out[i]++
		}
	}
	return out
}

type Move int

const (
	Left Move = iota
	Right
	Up
	Down
)

func Neighbor(rects map[string]Rect, order []string, from string, mv Move) string {
	f, ok := rects[from]
	if !ok {
		return ""
	}
	best, bestKey := "", [3]int{}
	for _, id := range order {
		c, ok := rects[id]
		if !ok || id == from {
			continue
		}
		var gap, overlap, center int
		switch mv {
		case Left:
			gap = f.X - (c.X + c.W)
			overlap = min(f.Y+f.H, c.Y+c.H) - max(f.Y, c.Y)
			center = abs((2*c.Y + c.H) - (2*f.Y + f.H))
		case Right:
			gap = c.X - (f.X + f.W)
			overlap = min(f.Y+f.H, c.Y+c.H) - max(f.Y, c.Y)
			center = abs((2*c.Y + c.H) - (2*f.Y + f.H))
		case Up:
			gap = f.Y - (c.Y + c.H)
			overlap = min(f.X+f.W, c.X+c.W) - max(f.X, c.X)
			center = abs((2*c.X + c.W) - (2*f.X + f.W))
		case Down:
			gap = c.Y - (f.Y + f.H)
			overlap = min(f.X+f.W, c.X+c.W) - max(f.X, c.X)
			center = abs((2*c.X + c.W) - (2*f.X + f.W))
		}
		if gap < 0 {
			continue
		}
		miss := 0
		if overlap <= 0 {
			miss = 1
		}
		key := [3]int{miss, gap, center}
		if best == "" || less(key, bestKey) {
			best, bestKey = id, key
		}
	}
	return best
}

func less(a, b [3]int) bool {
	for i := range a {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return false
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
