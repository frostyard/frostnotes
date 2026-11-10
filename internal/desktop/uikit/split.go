package uikit

import "github.com/egoist/mygo/ui"

// SplitNode is a node of a split tree: a leaf draws itself; a split lays
// out at least two children side by side (Row) or stacked, with sizes
// that sum to 1.
type SplitNode struct {
	ID       string
	Row      bool
	Children []*SplitNode
	Sizes    []float64
	Leaf     func(c *ui.Context)
}

// SplitView draws a split tree; resize reports a split's new sizes.
func SplitView(c *ui.Context, n *SplitNode, state *SplitState, resize func(id string, sizes []float64)) {
	if n.Leaf != nil {
		n.Leaf(c)
		return
	}
	splitFrom(c, n, 0, state, resize)
}

// SplitState keeps each divider's position while it is dragged.
type SplitState struct{ pos map[string]*float32 }

func (s *SplitState) at(key string, want float32) *float32 {
	if s.pos == nil {
		s.pos = map[string]*float32{}
	}
	p := s.pos[key]
	if p == nil {
		p = new(float32)
		s.pos[key] = p
	}
	*p = want
	return p
}

// splitFrom lays out children from index i: the first beside a split of
// the rest, as MyGo's splits hold two panes.
func splitFrom(c *ui.Context, n *SplitNode, i int, state *SplitState, resize func(string, []float64)) {
	rest := 0.0
	for _, s := range n.Sizes[i:] {
		rest += s
	}
	if rest <= 0 {
		rest = 1
	}
	key := n.ID + "/" + string(rune('0'+i))
	want := float32(n.Sizes[i] / rest * 100)
	pos := state.at(key, want)
	first := func() { SplitView(c.Key(n.Children[i].ID), n.Children[i], state, resize) }
	second := func() {
		if i+2 == len(n.Children) {
			SplitView(c.Key(n.Children[i+1].ID), n.Children[i+1], state, resize)
			return
		}
		splitFrom(c, n, i+1, state, resize)
	}
	var el ui.Element
	if n.Row {
		el = ui.SplitPercent(c.Key(key), pos, first, second)
	} else {
		el = ui.SplitVerticalPercent(c.Key(key), pos, first, second)
	}
	el.Grow(1)
	if el.Changed() && resize != nil {
		sizes := append([]float64(nil), n.Sizes...)
		sizes[i] = rest * float64(*pos) / 100
		left := rest - sizes[i]
		old := rest - n.Sizes[i]
		for j := i + 1; j < len(sizes); j++ {
			if old > 0 {
				sizes[j] = n.Sizes[j] / old * left
			}
		}
		resize(n.ID, sizes)
	}
}
