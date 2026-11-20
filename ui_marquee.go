package main

import (
	"maps"
	"math"
	"time"

	"github.com/egoist/mygo/ui"
)

// A box dragged out from the list's blank space (beside the rows, over a
// group's title, below the last row) chooses the issues it touches, as
// Finder's lists do; with Cmd or Shift held as it starts, besides those
// chosen already. A press on a row still opens or drags that row, and a
// click on blank space chooses nothing.
type marquee struct {
	x0, y0, x, y float32 // window DIPs: where the press began, where the pointer is
	anchor       string  // the row the box's start sticks to as the list scrolls
	offset       float32 // how far below that row's top the box started
	base         map[string]bool
	moved        bool
}

// marqueeRow is the handle of a list row's box, by the row's key; a row
// whose handle has no box is not built, scrolled out of sight.
func (a *app) marqueeRow(key string) *ui.Handle { return marqueeHandle(&a.marqueeRows, key) }

// marqueeControl is the handle of something in the list a press acts on.
func (a *app) marqueeControl(key string) *ui.Handle { return marqueeHandle(&a.marqueeControls, key) }

func marqueeHandle(m *map[string]*ui.Handle, key string) *ui.Handle {
	if *m == nil {
		*m = map[string]*ui.Handle{}
	}
	h := (*m)[key]
	if h == nil {
		h = &ui.Handle{}
		(*m)[key] = h
	}
	return h
}

// forgetMarqueeRows lets go of the handles of rows the list no longer has.
func (a *app) forgetMarqueeRows(rows []issueRow) {
	live := map[string]bool{}
	for _, r := range rows {
		live[r.key] = true
	}
	for key := range a.marqueeRows {
		if !live[key] {
			delete(a.marqueeRows, key)
		}
	}
	for key := range a.marqueeControls {
		if k, ok := marqueeControlRow(key); ok && !live[k] {
			delete(a.marqueeControls, key)
		}
	}
}

func marqueeControlRow(key string) (string, bool) {
	for n := 0; n < len(key); n++ {
		if key[n] == ':' {
			return key[n+1:], true
		}
	}
	return "", false
}

// marqueeInput starts, follows and ends a box over the list.
func (a *app) marqueeInput(c *ui.Context, rows []issueRow, ev ui.InputEvent) bool {
	list := a.list.Bounds(c)
	x, y := list.X+ev.X, list.Y+ev.Y
	switch ev.Kind {
	case ui.InputPointerDown:
		if ev.Button != 0 || a.page == "trash" && len(rows) == 0 {
			return false
		}
		for _, h := range a.marqueeControls {
			if h.Bounds(c).Contains(x, y) {
				return false // the row or control's own press
			}
		}
		m := &marquee{x0: x, y0: y, x: x, y: y, base: map[string]bool{}}
		if ev.Mods&(ui.Cmd|ui.Shift) != 0 {
			maps.Copy(m.base, a.selected)
		}
		// The start sticks to the issue nearest above it, or else the first
		// one built, so that it stays put in the list as the list scrolls.
		// Not to a group's title: a title stays at the top while its rows
		// scroll under it.
		for _, r := range rows {
			box := a.marqueeRow(r.key).Bounds(c)
			if box.W == 0 || r.header != "" {
				continue
			}
			if m.anchor == "" || box.Y <= y {
				m.anchor, m.offset = r.key, y-box.Y
			}
		}
		a.marquee = m
		return true
	case ui.InputPointerMove:
		m := a.marquee
		if m == nil {
			return false
		}
		m.x, m.y = x, y
		if !m.moved && math.Hypot(float64(x-m.x0), float64(y-m.y0)) >= 4 {
			m.moved = true
		}
		if m.moved {
			a.marqueeSelect(c, rows)
		}
		return true
	case ui.InputPointerUp:
		m := a.marquee
		if m == nil {
			return false
		}
		a.marquee = nil
		if !m.moved && !ev.Canceled && ev.Mods&(ui.Cmd|ui.Shift) == 0 {
			a.selected = map[string]bool{}
		}
		return true
	}
	return false
}

// marqueeBand is the box in window DIPs, its start following the row it
// sticks to. A start scrolled out of sight lies beyond the list's edge,
// above or below, as far as any row: the rows on its side of it are then
// the ones in the box.
func (a *app) marqueeBand(c *ui.Context, rows []issueRow) (band ui.Rect, above, below bool) {
	m := a.marquee
	y0 := m.y0
	if box := a.marqueeRow(m.anchor).Bounds(c); box.W > 0 {
		y0 = box.Y + m.offset
	} else {
		first, at := -1, -1
		for n, r := range rows {
			if first < 0 && a.marqueeRow(r.key).Bounds(c).W > 0 {
				first = n
			}
			if r.key == m.anchor {
				at = n
			}
		}
		if at >= 0 && first >= 0 && at < first {
			y0, above = -1e6, true
		} else {
			y0, below = 1e6, true
		}
	}
	top, bottom := min(y0, m.y), max(y0, m.y)
	left, right := min(m.x0, m.x), max(m.x0, m.x)
	return ui.Rect{X: left, Y: top, W: right - left, H: bottom - top}, above, below
}

// marqueeSelect chooses the issues the box touches: those built by their
// boxes, and those scrolled out of sight between its start and what shows.
func (a *app) marqueeSelect(c *ui.Context, rows []issueRow) {
	m := a.marquee
	band, above, below := a.marqueeBand(c, rows)
	list := a.list.Bounds(c)
	across := band.X < list.X+list.W-12 && band.X+band.W > list.X+12
	firstBuilt, lastBuilt, anchor := -1, -1, -1
	for n, r := range rows {
		if a.marqueeRow(r.key).Bounds(c).W > 0 {
			if firstBuilt < 0 {
				firstBuilt = n
			}
			lastBuilt = n
		}
		if r.key == m.anchor {
			anchor = n
		}
	}
	chosen := maps.Clone(m.base)
	for n, r := range rows {
		if r.header != "" {
			continue
		}
		box := a.marqueeControl("row:" + r.key).Bounds(c)
		var in bool
		switch {
		case box.W > 0:
			in = box.X < band.X+band.W && box.X+box.W > band.X && box.Y < band.Y+band.H && box.Y+box.H > band.Y
			in = in && !(above && n < anchor) && !(below && n > anchor)
		case n < firstBuilt:
			in = above && across && n >= anchor
		case n > lastBuilt:
			in = below && across && n <= anchor
		}
		if in {
			chosen[r.issue.ID] = true
		}
	}
	a.selected = chosen
}

// marqueeFrame keeps a box going while the list builds: it follows the
// list as it scrolls, and scrolls it while the pointer is held past an edge.
func (a *app) marqueeFrame(c *ui.Context, rows []issueRow) {
	m := a.marquee
	if m == nil || !m.moved {
		return
	}
	list := a.list.Bounds(c)
	first, last := a.list.Visible()
	switch {
	case m.y < list.Y+16 && first > 0:
		a.list.ScrollIntoView(first - 1)
	case m.y > list.Y+list.H-16 && last >= 0 && last < len(rows)-1:
		a.list.ScrollIntoView(last + 1)
	default:
		a.marqueeSelect(c, rows)
		return
	}
	a.marqueeSelect(c, rows)
	c.After(40 * time.Millisecond)
}

// paintMarquee draws the box over the rows.
func (a *app) paintMarquee(c *ui.Context, rows []issueRow) func(p *ui.Painter, r ui.Rect) {
	return func(p *ui.Painter, r ui.Rect) {
		if a.marquee == nil || !a.marquee.moved {
			return
		}
		band, _, _ := a.marqueeBand(c, rows)
		p.Clip(r, 0, func() {
			p.Fill(band, a.tk.Primary.Alpha(.1), 6)
			p.Stroke(band, a.tk.Primary.Alpha(.6), 6, 1)
		})
	}
}
