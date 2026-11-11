// Package uikit holds the desktop app's generic UI pieces that frostui
// does not have yet: tab strips, a split tree of tab groups and a side
// panel's icon pane tabs (plan 0009). It imports only the standard
// library, MyGo, purego and frostui, which a test enforces, so that a
// piece moves into frostui by changing its import path.
package uikit

import (
	"github.com/egoist/mygo/ui"
	"github.com/frostyard/frostui"
)

// Tab is a tab of a strip.
type Tab struct {
	ID, Title string
	Pinned    bool
}

// TabDrag is the value a dragged tab carries.
type TabDrag struct{ ID string }

// TabStrip is a row of closable, pinnable, draggable tabs over a group's
// view: Up and the arrows are its own, a middle click closes a tab, and
// the context menu key or a right click opens its menu.
type TabStrip struct {
	Label string
	// NewLabel and ListLabel name the New and List buttons, and CloseLabel a
	// tab's close button, given its title.
	NewLabel, ListLabel string
	CloseLabel          func(title string) string
	Tabs                []Tab
	Active              int
	// Activate chooses a tab, Close closes one, New adds one.
	Activate func(id string)
	Close    func(id string)
	New      func()
	// Move puts a dragged tab at index of this strip.
	Move func(id string, index int)
	// Menu builds a tab's context menu; List the menu of all the tabs.
	Menu func(m *ui.Menu, id string)
	List func(m *ui.Menu)
	// Dragging is set while a tab of the strip is dragged, so the groups
	// show where it can land.
	Dragging *bool
}

const stripHeight = 36

// View draws the strip.
func (s TabStrip) View(c *ui.Context) ui.Element {
	t := c.Theme()
	active := s.Active
	bar := ui.Row(c).Height(stripHeight).AlignItems(ui.Stretch).Background(frostui.Chrome(t))
	bar.Children(func() {
		tabs := ui.TabsBase(c, &active, len(s.Tabs))
		tabs.List.Grow(1).AlignItems(ui.Stretch).Label(s.Label).Children(func() {
			for i, tab := range s.Tabs {
				s.tab(c.Key(tab.ID), tabs.Tab(i), i, tab)
			}
			end := ui.Box(c).Grow(1)
			if d, ok := ui.Drop[TabDrag](end); ok && s.Move != nil {
				s.Move(d.ID, len(s.Tabs))
			}
		})
		ui.Row(c).Gap(frostui.SpaceXS).Padding(0, frostui.SpaceXS).AlignItems(ui.Center).Children(func() {
			if s.New != nil && ToolButton(c, "list-add", s.NewLabel).Clicked() {
				s.New()
			}
			if s.List != nil {
				ToolButton(c, "pan-down", s.ListLabel).Menu(s.List)
			}
		})
	})
	if active != s.Active && active >= 0 && active < len(s.Tabs) && s.Activate != nil {
		s.Activate(s.Tabs[active].ID)
	}
	return bar
}

func (s TabStrip) tab(c *ui.Context, el ui.Element, index int, tab Tab) {
	t := c.Theme()
	el.Padding(0, frostui.SpaceS, 0, frostui.SpaceM).Gap(frostui.SpaceXS).AlignItems(ui.Center).MaxWidth(220).Shrink(1).Label(tab.Title).Drag(TabDrag{tab.ID})
	if index == s.Active {
		el.Background(t.Background)
	} else if el.Hovered() {
		el.Background(t.SurfaceHover)
	}
	if s.Dragging != nil && el.Dragging() {
		*s.Dragging = true
	}
	if d, ok := ui.Drop[TabDrag](el); ok && s.Move != nil {
		s.Move(d.ID, index)
	}
	if _, over := ui.DragOver[TabDrag](el); over {
		el.Background(frostui.ToneFill(t, frostui.Accent))
	}
	if s.Menu != nil {
		el.ContextMenu(func(m *ui.Menu) { s.Menu(m, tab.ID) })
	}
	el.HandleInput(func(ev ui.InputEvent) bool {
		if ev.Kind == ui.InputPointerDown && ev.Button == 2 && s.Close != nil && !tab.Pinned {
			s.Close(tab.ID)
			return true
		}
		if ev.Kind == ui.InputKeyDown && ev.Key == ui.KeyDelete && s.Close != nil && !tab.Pinned {
			s.Close(tab.ID)
			return true
		}
		return false
	})
	el.Children(func() {
		if tab.Pinned {
			frostui.Icon(c, "view-pin").TextColor(t.TextMuted)
		}
		ui.Text(c, tab.Title).FontSize(13).SingleLine().Grow(1).Shrink(1).AccessHidden(true)
		if !tab.Pinned && s.Close != nil && ToolButton(c, "window-close", s.CloseLabel(tab.Title)).Size(20, 20).Clicked() {
			s.Close(tab.ID)
		}
	})
}

// The edges of a group, where a dropped tab makes a new split.
const (
	EdgeLeft   = "left"
	EdgeRight  = "right"
	EdgeTop    = "top"
	EdgeBottom = "bottom"
)

// DropEdges draws, over a group's view while a tab is dragged, the four
// edges it can land on to make a new split, each a quarter of the group.
func DropEdges(c *ui.Context, drop func(id, edge string)) {
	t := c.Theme()
	zone := func(key, edge string, grow float32) {
		el := ui.Box(c.Key(key)).Grow(grow)
		if _, over := ui.DragOver[TabDrag](el); over {
			el.Background(frostui.ToneFill(t, frostui.Accent))
		}
		if d, ok := ui.Drop[TabDrag](el); ok && drop != nil && edge != "" {
			drop(d.ID, edge)
		}
	}
	ui.Row(c).Absolute().Left(0).Top(0).Right(0).Bottom(0).AlignItems(ui.Stretch).Children(func() {
		zone("l", EdgeLeft, 1)
		ui.Column(c).Grow(2).AlignItems(ui.Stretch).Children(func() {
			zone("t", EdgeTop, 1)
			ui.Box(c).Grow(2)
			zone("b", EdgeBottom, 1)
		})
		zone("r", EdgeRight, 1)
	})
}

// PaneTab is a pane in a side panel's tabs.
type PaneTab struct {
	ID, Title, Icon string
}

// PaneTabs is a row of icon tabs choosing a side panel's pane.
func PaneTabs(c *ui.Context, label string, panes []PaneTab, active int, choose func(id string)) ui.Element {
	selected := active
	tabs := ui.TabsBase(c, &selected, len(panes))
	el := tabs.List.Gap(frostui.SpaceXS).Padding(frostui.SpaceXS, frostui.SpaceS).AlignItems(ui.Center).Label(label)
	el.Children(func() {
		for i, p := range panes {
			tab := tabs.Tab(i).Size(28, 28).Center().Radius(6).Label(p.Title).Tooltip(p.Title)
			if i == active {
				tab.Background(c.Theme().Surface)
			}
			tab.Children(func() { frostui.Icon(c, p.Icon) })
		}
	})
	if selected != active && selected >= 0 && selected < len(panes) && choose != nil {
		choose(panes[selected].ID)
	}
	return el
}

// ToolButton is a flat icon button for the dense rows of a pane or a tab
// strip: frostui's IconButton keeps the padding of a form's.
func ToolButton(c *ui.Context, icon, label string) ui.Element {
	t := c.Theme()
	b := ui.ButtonBase(c).Size(26, 26).Center().Radius(frostui.RadiusChip).Cursor(ui.CursorPointer).Label(label).Tooltip(label)
	if b.Hovered() {
		b.Background(t.SurfaceHover)
	}
	return b.Children(func() { frostui.Icon(c, icon).TextColor(t.TextMuted) })
}
