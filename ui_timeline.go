package main

import (
	"flow/internal/core"
	"fmt"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
	"github.com/lobehub/lobe-mygo/pkg/lucide"
	"math"
	"slices"
	"strings"
	"time"
)

type timelineEntry struct{ Kind, ID, Title, Start, End, Color string }
type timelineDrag struct{ Kind, ID, Date string }
type timelineLink struct {
	From, To string
	Points   []ui.Point
	Violated bool
}

// timelineAdjust is a bar the pointer drags: whole ("move"), or by its
// "start" or "end" edge, DX DIPs so far.
type timelineAdjust struct {
	Key, Edge string
	DX        float32
}

func (v timelineEntry) key() string      { return v.Kind + "/" + v.ID }
func calendarDay(raw string) time.Time   { d, _ := time.Parse("2006-01-02", raw); return d }
func dayDistance(from, to time.Time) int { return int((to.Unix() - from.Unix()) / 86400) }
func (a *app) timelineVisible() bool {
	return a.issueID == "" && ((a.page == "projects" && a.projectsTimeline) || (a.page == "project" && a.projectTab == "timeline"))
}
func (a *app) timelineEntries() []timelineEntry {
	projects := a.data.ProjectList(false, false)
	if a.page == "project" {
		projects = []core.Project{a.data.Projects[a.projectID]}
	}
	out := []timelineEntry{}
	for _, p := range projects {
		start, end := p.Start, p.Due
		if start == "" {
			start = end
		}
		if end == "" {
			end = start
		}
		out = append(out, timelineEntry{"project", p.ID, p.Name, start, end, p.Color})
		for _, m := range a.data.MilestoneList(p.ID) {
			out = append(out, timelineEntry{"milestone", m.ID, "◇ " + m.Name, m.Due, m.Due, p.Color})
		}
		if a.page == "project" {
			for _, i := range a.data.Query(core.Filter{ProjectID: p.ID, IncludeArchived: true}) {
				start := i.Start
				if start == "" {
					start = i.Due
				}
				out = append(out, timelineEntry{"issue", i.ID, i.Code() + " " + i.Title, start, i.Due, p.Color})
			}
		}
	}
	return out
}
func (a *app) timelineScaleName() string {
	if slices.Contains([]string{"week", "month", "quarter", "year"}, a.timelineScale) {
		return a.timelineScale
	}
	return "month"
}
func (a *app) timelineAnchor() time.Time {
	raw := a.timelineStart
	if raw == "" && a.page == "project" {
		raw = a.data.Projects[a.projectID].Start
	}
	if raw == "" {
		raw = time.Now().Format("2006-01-02")
	}
	d := calendarDay(raw)
	switch a.timelineScaleName() {
	case "week":
		return d.AddDate(0, 0, -(int(d.Weekday())+6)%7)
	case "quarter":
		return time.Date(d.Year(), time.Month((int(d.Month())-1)/3*3+1), 1, 0, 0, 0, 0, time.UTC)
	case "year":
		return time.Date(d.Year(), 1, 1, 0, 0, 0, 0, time.UTC)
	}
	return time.Date(d.Year(), d.Month(), 1, 0, 0, 0, 0, time.UTC)
}
func (a *app) timelineRange() (time.Time, int, float32) {
	start := a.timelineAnchor()
	days := 42
	width := float32(36)
	switch a.timelineScaleName() {
	case "week":
		days, width = 14, 56
	case "quarter":
		days, width = dayDistance(start, start.AddDate(0, 3, 0)), 9
	case "year":
		days, width = dayDistance(start, start.AddDate(1, 0, 0)), 2
	default:
		start = start.AddDate(0, 0, -(int(start.Weekday())+6)%7)
	}
	zoom := a.timelineZoom
	if zoom <= 0 {
		zoom = 1
	}
	return start, days, width * zoom
}
func (a *app) timelineMoveRange(dir int) {
	d := a.timelineAnchor()
	switch a.timelineScaleName() {
	case "week":
		d = d.AddDate(0, 0, 7*dir)
	case "quarter":
		d = d.AddDate(0, 3*dir, 0)
	case "year":
		d = d.AddDate(dir, 0, 0)
	default:
		d = d.AddDate(0, dir, 0)
	}
	a.timelineStart = d.Format("2006-01-02")
	a.timelineScroll.X = 0
}

// timelineGoToday shows the range around today, scrolled to a few days
// before it.
func (a *app) timelineGoToday() {
	a.timelineStart = time.Now().Format("2006-01-02")
	start, _, cell := a.timelineRange()
	a.timelineScroll.X = max(0, float32(dayDistance(start, calendarDay(a.timelineStart))-3)*cell)
}
func (a *app) setTimelineScale(scale string) {
	a.timelineScale = scale
	a.timelineZoom = 1
	a.timelineScroll.X = 0
}
func (a *app) zoomTimeline(factor float32) {
	if a.timelineZoom <= 0 {
		a.timelineZoom = 1
	}
	a.timelineZoom = max(.5, min(4, a.timelineZoom*factor))
}

// timelineDates are the stored dates of a row, unlike Start and End, which
// fill each other in for drawing.
func (a *app) timelineDates(v timelineEntry) (start, end string) {
	switch v.Kind {
	case "issue":
		i := a.data.Issues[v.ID]
		return i.Start, i.Due
	case "project":
		p := a.data.Projects[v.ID]
		return p.Start, p.Due
	}
	return "", a.data.Milestones[v.ID].Due
}

// setTimelineDates schedules a row: the days from start to end, or the one
// day end (a milestone is always one day).
func (a *app) setTimelineDates(v timelineEntry, start, end string) {
	a.flushDrafts()
	a.apply(func(d *core.Data) error {
		switch v.Kind {
		case "issue":
			i := d.Issues[v.ID]
			if end == "" {
				start, end = "", start
			}
			i.Start, i.Due = start, end
			return d.PutIssue(i)
		case "milestone":
			m := d.Milestones[v.ID]
			m.Due = end
			_, e := d.SaveMilestone(m)
			return e
		case "project":
			p := d.Projects[v.ID]
			p.Start, p.Due = start, end
			return d.PutProject(p)
		}
		return nil
	}, nil)
}

// timelineShown returns the days a row shows: its own, or where the pointer
// dragging it, or a row selected with it, has taken them so far.
func (a *app) timelineShown(v timelineEntry, cell float32) (string, string) {
	d := a.timelineAdjust
	days := int(math.Round(float64(d.DX / cell)))
	if v.Start == "" || days == 0 || d.Key != v.key() && (d.Edge != "move" || !a.timelineSelected[d.Key] || !a.timelineSelected[v.key()]) {
		return v.Start, v.End
	}
	s, e := calendarDay(v.Start), calendarDay(v.End)
	switch d.Edge {
	case "start":
		if s = s.AddDate(0, 0, days); s.After(e) {
			s = e
		}
	case "end":
		if e = e.AddDate(0, 0, days); e.Before(s) {
			e = s
		}
	default:
		s, e = s.AddDate(0, 0, days), e.AddDate(0, 0, days)
	}
	return s.Format("2006-01-02"), e.Format("2006-01-02")
}

// dragTimeline follows the pointer dragging handle, the bar of v or one of
// its edges, and saves the days it took them to once the pointer lets go.
// It reports a drag ending, which is no click.
func (a *app) dragTimeline(c *ui.Context, handle ui.Element, v timelineEntry, edge string, cell float32) bool {
	d := &a.timelineAdjust
	mine := d.Key == v.key() && d.Edge == edge
	dx, _, held := handle.Dragged()
	switch {
	case held:
		if dx != 0 {
			if !mine {
				*d = timelineAdjust{Key: v.key(), Edge: edge}
			}
			d.DX += dx
		}
		return false
	case handle.PressCanceled():
		if mine {
			*d = timelineAdjust{}
		}
		return false
	case !mine:
		return false
	}
	start, end := a.timelineShown(v, cell)
	moved := start != v.Start || end != v.End
	*d = timelineAdjust{}
	switch {
	case !moved:
	case edge == "move":
		a.shiftTimelineWithMode(timelineDrag{v.Kind, v.ID, v.Start}, start, timelineMode(c.Modifiers()))
	case start == end:
		a.setTimelineDates(v, "", end)
	default:
		a.setTimelineDates(v, start, end)
	}
	return true
}

// timelinePeriod names the time range the timeline shows.
func (a *app) timelinePeriod() string {
	d := a.timelineAnchor()
	switch a.timelineScaleName() {
	case "week":
		return a.shortRange(d.Format("2006-01-02"), d.AddDate(0, 0, 13).Format("2006-01-02"))
	case "quarter":
		q := (int(d.Month())-1)/3 + 1
		return a.tr(fmt.Sprintf("%d年第%d季度", d.Year(), q), fmt.Sprintf("Q%d %d", q, d.Year()))
	case "year":
		return a.tr(fmt.Sprintf("%d年", d.Year()), fmt.Sprint(d.Year()))
	}
	return a.tr(fmt.Sprintf("%d年%d月", d.Year(), d.Month()), d.Format("January 2006"))
}

// timelineMonth names a month over the days, w DIPs wide, that show of it,
// with its year when that is not this one, or the year begins.
func (a *app) timelineMonth(d time.Time, w float32) string {
	year := d.Year() != time.Now().Year() || d.Month() == time.January
	if a.data.Preferences.Locale == "en" {
		name := d.Format("January")
		if w < 90 {
			name = d.Format("Jan")
		}
		if year {
			name += d.Format(" 2006")
		}
		return name
	}
	if year {
		return fmt.Sprintf("%d年%d月", d.Year(), d.Month())
	}
	return fmt.Sprintf("%d月", d.Month())
}

// timelineHeader names the columns: months over days, or over weeks once
// days get narrow, and years over months at the widest.
func (a *app) timelineHeader(c *ui.Context, start time.Time, days int, cell, labelWidth float32) {
	end := start.AddDate(0, 0, days)
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	byMonth, byWeek := cell < 6, cell < 18
	weekdays := []string{"周日", "周一", "周二", "周三", "周四", "周五", "周六"}
	size := float32(13)
	if cell < 26 {
		size = 11
	}
	ui.Row(c).Height(62).Shrink(0).BorderWidth(0, 0, 1, 0).BorderColor(a.hairline()).Children(func() {
		// The names stay at the left as the days scroll under them.
		ui.Row(c).Width(labelWidth).FillHeight().Padding(0, 16).Sticky().Left(0).ZIndex(2).Background(a.tk.BgContainer).BorderWidth(0, 1, 0, 0).BorderColor(a.hairline()).Children(func() {
			ui.Text(c, a.tr("项目与里程碑", "Projects and milestones")).TextColor(a.theme.TextMuted)
		})
		ui.Column(c).Width(float32(days) * cell).Children(func() {
			ui.Row(c).Height(28).Children(func() {
				for d := start; d.Before(end); {
					next := time.Date(d.Year(), d.Month()+1, 1, 0, 0, 0, 0, time.UTC)
					if byMonth {
						next = time.Date(d.Year()+1, 1, 1, 0, 0, 0, 0, time.UTC)
					}
					if next.After(end) {
						next = end
					}
					w := float32(dayDistance(d, next)) * cell
					text := a.timelineMonth(d, w)
					if byMonth {
						text = a.tr(fmt.Sprintf("%d年", d.Year()), fmt.Sprint(d.Year()))
					}
					if w < 36 {
						text = ""
					}
					month := ui.Row(c).Width(w).Height(28).Clip()
					if !d.Equal(start) {
						month.BorderWidth(0, 0, 0, 1).BorderColor(a.hairline())
					}
					// A month's name stays in sight beside the names until
					// the next month pushes it out.
					month.Children(func() {
						ui.Text(c, text).SingleLine().Padding(6, 8, 0, 8).FontSize(14).FontWeight(600).TextColor(a.tk.Text).Sticky().Left(labelWidth)
					})
					d = next
				}
			})
			ui.Row(c).Height(33).Children(func() {
				for d := start; d.Before(end); {
					next := d.AddDate(0, 0, 1)
					switch {
					case byMonth:
						next = time.Date(d.Year(), d.Month()+1, 1, 0, 0, 0, 0, time.UTC)
					case byWeek:
						next = d.AddDate(0, 0, 7-(int(d.Weekday())+6)%7)
					}
					if next.After(end) {
						next = end
					}
					w := float32(dayDistance(d, next)) * cell
					current := !today.Before(d) && today.Before(next)
					if byWeek {
						// A week or a month, named by its first day showing.
						text := fmt.Sprint(d.Day())
						if byMonth {
							text = a.tr(fmt.Sprintf("%d月", d.Month()), d.Format("Jan"))
						}
						if w < 16 {
							text = ""
						}
						t := ui.Text(c, text).SingleLine().Clip().Width(w).Padding(0, 4).FontSize(12).TextColor(a.tk.TextTertiary)
						if current {
							t.FontWeight(600).TextColor(a.tk.Primary)
						}
						d = next
						continue
					}
					weekend := d.Weekday() == time.Saturday || d.Weekday() == time.Sunday
					ui.Row(c).Width(w).Center().Gap(3).Children(func() {
						if cell >= 44 {
							ui.Text(c, a.tr(weekdays[d.Weekday()], d.Format("Mon"))).SingleLine().FontSize(12).TextColor(a.tk.TextQuaternary)
						}
						day := fmt.Sprint(d.Day())
						switch {
						case current:
							accentFill(c, ui.Row(c).Height(size+10).MinWidth(size+10).Padding(0, 4).Radius((size+10)/2).Center(), false, false).Children(func() {
								ui.Text(c, day).SingleLine().FontSize(size).FontWeight(600).TextColor(white)
							})
						case weekend:
							ui.Text(c, day).SingleLine().FontSize(size).TextColor(a.tk.TextQuaternary)
						default:
							ui.Text(c, day).SingleLine().FontSize(size).TextColor(a.tk.TextSecondary)
						}
					})
					d = next
				}
			})
		})
	})
}

// paintTimelineDays shades today and, while the header names each day, the
// weekends across a row.
func (a *app) paintTimelineDays(p *ui.Painter, r ui.Rect, start time.Time, days int, cell, label float32) {
	today := time.Now().Format("2006-01-02")
	for n := 0; n < days; n++ {
		d := start.AddDate(0, 0, n)
		box := ui.Rect{X: r.X + label + float32(n)*cell, Y: r.Y, W: cell, H: r.H - 1}
		switch {
		case d.Format("2006-01-02") == today:
			p.Fill(box, a.theme.Accent.Alpha(.08), 0)
		case cell >= 18 && (d.Weekday() == time.Saturday || d.Weekday() == time.Sunday):
			p.Fill(box, a.theme.Surface.Alpha(.5), 0)
		}
	}
}

// timelineBar draws the days from s to e of v as a bar the pointer moves
// whole or by either end, or a milestone as a diamond it moves.
func (a *app) timelineBar(c *ui.Context, v timelineEntry, s, e string, start time.Time, days int, cell, label float32, hovered bool) {
	if s == "" {
		return
	}
	from, to := dayDistance(start, calendarDay(s)), dayDistance(start, calendarDay(e))+1
	if to <= 0 || from >= days {
		return
	}
	color := ui.Hex(v.Color)
	active := a.timelineAdjust.Key == v.key()
	if v.Kind == "milestone" {
		mark := ui.ButtonBase(c.Key("milestone-marker")).Absolute().Left(label+float32(from)*cell+cell/2-9).Top(10).Size(18, 22).Center().Cursor(ui.CursorGrab).Label(a.tr("拖动里程碑：", "Drag milestone: ") + v.Title).Tooltip(v.Title + " · " + a.shortDate(e))
		mark.Children(func() { ui.Text(c, "◆").FontSize(16).TextColor(color) })
		if !a.dragTimeline(c, mark, v, "move", cell) && !active && mark.Clicked() {
			a.selectTimeline(v, mark.ClickModifiers())
		}
		return
	}
	openStart, openEnd := from < 0, to > days
	from, to = max(from, 0), min(to, days)
	x0, x1 := label+float32(from)*cell, label+float32(to)*cell
	fill := color.Mix(a.tk.BgContainer, .18)
	if hovered || active {
		fill = color
	}
	left, right := float32(5), float32(5)
	if openStart {
		left = 0
	}
	if openEnd {
		right = 0
	}
	bar := ui.Box(c.Key("bar")).Absolute().Left(x0+1).Top(8).Size(max(2, x1-x0-2), 26).Radius(left, right, right, left).Background(fill).Cursor(ui.CursorGrab).Label(a.tr("拖动日期：", "Drag dates: ") + v.Title).Tooltip(v.Title + " · " + a.shortRange(s, e))
	if active && a.timelineAdjust.Edge == "move" {
		bar.Cursor(ui.CursorGrabbing)
	}
	if !a.dragTimeline(c, bar, v, "move", cell) && !active && bar.Clicked() {
		a.selectTimeline(v, bar.ClickModifiers())
	}
	// Each end takes a few DIPs inside the bar and a few past it, leaving a
	// one-day bar room to be moved whole.
	reach := min(7, (x1-x0)/4)
	grip := ui.Hex("#ffffff").Alpha(.85)
	edge := func(name string, x float32, pad []float32, title string) {
		h := ui.Row(c.Key("bar-"+name)).Absolute().Left(x).Top(8).Size(reach+3, 26).Padding(pad...).Center().Cursor(ui.CursorResizeEW).Label(title + v.Title)
		if (hovered || active) && x1-x0 >= 20 {
			h.Children(func() { ui.Box(c).Size(2, 12).Radius(1).Background(grip) })
		}
		a.dragTimeline(c, h, v, name, cell)
	}
	if !openStart {
		edge("start", x0-2, []float32{0, 0, 0, 3}, a.tr("拖动开始日期：", "Drag start date: "))
	}
	if !openEnd {
		edge("end", x1-reach-1, []float32{0, 3, 0, 0}, a.tr("拖动结束日期：", "Drag end date: "))
	}
	if v.Kind == "project" && !openEnd {
		ui.ButtonBase(c.Key("dependency-handle")).Absolute().Left(x1+3).Top(15).Size(12, 12).Radius(6).Background(a.theme.Surface).Border(2, color).Drag(timelineDrag{"dependency", v.ID, ""}).Label(a.tr("连接依赖：", "Link dependency: ") + v.Title).Tooltip(a.tr("拖到后续项目以建立依赖", "Drag to a dependent project"))
	}
}

func timelineMode(mods ui.Modifiers) string {
	if mods&(ui.Cmd|ui.Ctrl) != 0 {
		return "single"
	}
	if mods&ui.Shift != 0 {
		return "all"
	}
	return "auto"
}
func (a *app) shiftTimeline(drag timelineDrag, target string) {
	a.shiftTimelineWithMode(drag, target, "auto")
}
func (a *app) shiftTimelineWithMode(drag timelineDrag, target, mode string) {
	days := dayDistance(calendarDay(drag.Date), calendarDay(target))
	if days == 0 {
		return
	}
	targets := []core.ScheduleTarget{{Kind: drag.Kind, ID: drag.ID}}
	if a.timelineSelected[drag.Kind+"/"+drag.ID] {
		targets = nil
		for _, v := range a.timelineEntries() {
			if a.timelineSelected[v.key()] {
				targets = append(targets, core.ScheduleTarget{Kind: v.Kind, ID: v.ID})
			}
		}
	}
	a.flushDrafts()
	a.apply(func(d *core.Data) error { _, e := d.ShiftSchedule(targets, days, mode); return e }, nil)
}
func (a *app) selectTimeline(v timelineEntry, mods ui.Modifiers) {
	a.timelineFocus = v.key()
	if a.timelineSelected == nil {
		a.timelineSelected = map[string]bool{}
	}
	if mods&(ui.Cmd|ui.Shift) != 0 {
		a.timelineSelected[v.key()] = !a.timelineSelected[v.key()]
	} else {
		a.timelineSelected = map[string]bool{v.key(): true}
	}
	a.timelineList.Handle.Focus()
}
func (a *app) openTimelineEntry(v timelineEntry) {
	switch v.Kind {
	case "issue":
		a.openIssue(v.ID)
	case "project":
		a.navigate("project", v.ID)
		a.projectTab = "overview"
	case "milestone":
		m := a.data.Milestones[v.ID]
		a.navigate("project", m.ProjectID)
		a.projectTab = "issues"
		a.viewConfig = core.DefaultView()
		a.viewConfig.Filter = core.FilterGroup{Rules: []core.FilterRule{{Field: "milestone", Op: "is", Values: []string{m.ID}}}}
	}
}
func (a *app) timelineShortcuts(c *ui.Context) {
	if !a.timelineVisible() || c.FocusTakesText() {
		return
	}
	for _, v := range []struct {
		k ui.Key
		s string
	}{{ui.KeyW, "week"}, {ui.KeyM, "month"}, {ui.KeyQ, "quarter"}, {ui.KeyY, "year"}, {ui.Key0, "year"}} {
		c.OnShortcut(0, v.k, func() { a.setTimelineScale(v.s) })
	}
	c.OnShortcut(0, ui.KeyT, a.timelineGoToday)
	c.OnShortcut(0, ui.KeyEqual, func() { a.zoomTimeline(1.25) })
	c.OnShortcut(ui.Shift, ui.KeyEqual, func() { a.zoomTimeline(1.25) })
	c.OnShortcut(0, ui.KeyMinus, func() { a.zoomTimeline(.8) })
	entries := a.timelineEntries()
	step := func(delta int, projectsOnly bool) {
		if len(entries) == 0 {
			return
		}
		current := slices.IndexFunc(entries, func(v timelineEntry) bool { return v.key() == a.timelineFocus })
		project := ""
		if current >= 0 {
			v := entries[current]
			switch v.Kind {
			case "project":
				project = v.ID
			case "milestone":
				project = a.data.Milestones[v.ID].ProjectID
			case "issue":
				project = a.data.Issues[v.ID].ProjectID
			}
		}
		choices := []int{}
		for n, v := range entries {
			if projectsOnly && v.Kind == "project" || !projectsOnly && (v.Kind == "project" && v.ID == project || v.Kind == "milestone" && a.data.Milestones[v.ID].ProjectID == project) {
				choices = append(choices, n)
			}
		}
		if len(choices) == 0 {
			return
		}
		pos := slices.Index(choices, current)
		if pos < 0 {
			for n, row := range choices {
				if entries[row].Kind == "project" && entries[row].ID == project {
					pos = n
					break
				}
			}
		}
		if pos < 0 {
			pos = 0
		} else {
			pos = max(0, min(len(choices)-1, pos+delta))
		}
		n := choices[pos]
		a.timelineFocus = entries[n].key()
		a.timelineList.ScrollIntoView(n)
		a.timelineList.Handle.Focus()
	}
	for _, k := range []ui.Key{ui.KeyUp, ui.KeyK} {
		c.OnShortcut(0, k, func() { step(-1, true) })
	}
	for _, k := range []ui.Key{ui.KeyDown, ui.KeyJ} {
		c.OnShortcut(0, k, func() { step(1, true) })
	}
	c.OnShortcut(0, ui.KeyLeft, func() { step(-1, false) })
	c.OnShortcut(0, ui.KeyRight, func() { step(1, false) })
	c.OnShortcut(0, ui.KeyX, func() {
		for _, v := range entries {
			if v.key() == a.timelineFocus {
				a.selectTimeline(v, ui.Cmd)
				return
			}
		}
	})
	c.OnShortcut(0, ui.KeyEnter, func() {
		for _, v := range entries {
			if v.key() == a.timelineFocus {
				a.openTimelineEntry(v)
				return
			}
		}
	})
}
func (a *app) timelineGeometry(c *ui.Context, entries []timelineEntry, start time.Time, cell, label float32) []timelineLink {
	base := float32(0)
	x := float32(0)
	found := false
	index := map[string]int{}
	for n, v := range entries {
		if v.Kind == "project" {
			index[v.ID] = n
		}
		if h := a.timelineRows[v.key()]; !found && h != nil {
			r := h.Bounds(c)
			if r.W > 0 && r.H > 0 {
				base = r.Y - float32(n)*43
				x = r.X + label
				found = true
			}
		}
	}
	if !found {
		return nil
	}
	out := []timelineLink{}
	for _, v := range entries {
		if v.Kind != "project" || v.Start == "" {
			continue
		}
		p := a.data.Projects[v.ID]
		for _, dep := range p.Dependencies {
			from, ok := index[dep]
			if !ok {
				continue
			}
			other := a.data.Projects[dep]
			if other.Due == "" {
				continue
			}
			x1 := x + float32(dayDistance(start, calendarDay(other.Due))+1)*cell
			x2 := x + float32(dayDistance(start, calendarDay(v.Start)))*cell
			y1, y2 := base+float32(from)*43+21, base+float32(index[p.ID])*43+21
			bend := max(x1+8, (x1+x2)/2)
			out = append(out, timelineLink{dep, p.ID, []ui.Point{{X: x1, Y: y1}, {X: bend, Y: y1}, {X: bend, Y: y2}, {X: x2, Y: y2}}, other.Due > v.Start})
		}
	}
	return out
}
func (a *app) projectTimeline(c *ui.Context) {
	start, days, cellWidth := a.timelineRange()
	labelWidth := float32(310)
	width := labelWidth + float32(days)*cellWidth
	entries := a.timelineEntries()
	if a.timelineRows == nil {
		a.timelineRows = map[string]*ui.Handle{}
	}
	live := map[string]bool{}
	for _, v := range entries {
		live[v.key()] = true
		if a.timelineRows[v.key()] == nil {
			a.timelineRows[v.key()] = &ui.Handle{}
		}
	}
	for key := range a.timelineRows {
		if !live[key] {
			delete(a.timelineRows, key)
			delete(a.timelineSelected, key)
		}
	}
	ui.Row(c).Height(56).Shrink(0).Padding(0, 14).Gap(4).Children(func() {
		iconButton(c, "back", a.tr("上个时间范围", "Previous time range")).OnClick(func() { a.timelineMoveRange(-1) })
		today := textButton(c, a.tr("今天", "Today")).OnClick(a.timelineGoToday)
		overlayTip(c, today, a.tr("回到今天", "Go to today"), "t")
		iconButton(c, "arrow", a.tr("下个时间范围", "Next time range")).OnClick(func() { a.timelineMoveRange(1) })
		ui.Text(c, a.timelinePeriod()).FontSize(13).FontWeight(500).Margin(0, 8).TextColor(a.tk.TextSecondary)
		ui.Spacer(c)
		scales := []string{"week", "month", "quarter", "year"}
		current := max(0, slices.Index(scales, a.timelineScaleName()))
		a.segmented(c, "timeline-scale", []string{a.tr("周", "Week"), a.tr("月", "Month"), a.tr("季", "Quarter"), a.tr("年", "Year")}, current, true, func(n int) { a.setTimelineScale(scales[n]) })
		zoom := a.timelineZoom
		if zoom <= 0 {
			zoom = 1
		}
		action.ActionIcon(c, lucide.Minus(), action.ActionIconProps{BlockSize: 28, GlyphSize: 16, Title: a.tr("缩小时间线", "Zoom timeline out")}).Radius(999).Margin(0, 0, 0, 8).OnClick(func() { a.zoomTimeline(.8) })
		ui.Text(c, fmt.Sprintf("%.0f%%", zoom*100)).Width(40).FontSize(12).TextAlign(ui.Center).TextColor(a.tk.TextTertiary)
		action.ActionIcon(c, lucide.Plus(), action.ActionIconProps{BlockSize: 28, GlyphSize: 16, Title: a.tr("放大时间线", "Zoom timeline in")}).Radius(999).OnClick(func() { a.zoomTimeline(1.25) })
		// How dragging behaves is worth knowing once, not reading on every visit.
		iconButton(c, "help", a.tr("拖动条形改日期，拖两端改开始或结束；按住 Shift 联动依赖和后续里程碑，按住 ⌘ 保持关联日期；⌘ 滚动缩放", "Drag a bar to move it, or its ends to change the start or end. Hold Shift to move dependencies and later milestones, ⌘ to keep related dates. ⌘ scroll zooms."))
	})
	if a.timelineLinkTo != "" {
		ui.Row(c).Padding(0, 12, 8, 20).Gap(8).Children(func() {
			ui.Text(c, a.data.Projects[a.timelineLinkFrom].Name+" → "+a.data.Projects[a.timelineLinkTo].Name).Grow(1)
			textButton(c, a.tr("打开项目", "Open project")).OnClick(func() { id := a.timelineLinkTo; a.timelineLinkTo = ""; a.navigate("project", id) })
			textButton(c, a.tr("移除依赖", "Remove dependency")).OnClick(func() {
				from, to := a.timelineLinkFrom, a.timelineLinkTo
				a.timelineLinkTo = ""
				a.apply(func(d *core.Data) error {
					p := d.Projects[to]
					p.Dependencies = slices.DeleteFunc(slices.Clone(p.Dependencies), func(id string) bool { return id == from })
					return d.PutProject(p)
				}, nil)
			})
			iconButton(c, "close", a.tr("关闭依赖详情", "Close dependency details")).OnClick(func() { a.timelineLinkTo = "" })
		})
	}
	zoomInput := func(ev ui.InputEvent) bool {
		if ev.Kind == ui.InputScroll && ev.Mods&(ui.Cmd|ui.Ctrl) != 0 {
			a.zoomTimeline(float32(math.Exp(-float64(ev.DY) * .01)))
			return true
		}
		if ev.Kind == ui.InputZoom {
			a.zoomTimeline(ev.Scale)
			return true
		}
		return false
	}
	// The chart is one white card on the tile.
	a.card(ui.Column(c.Key("timeline-card"))).Grow(1).MinHeight(0).Margin(0, 14, 14, 14).AlignItems(ui.Stretch).Clip().Children(func() {
		ui.ScrollHorizontal(c).TrackScroll(&a.timelineScroll).Grow(1).MinHeight(0).HandleInput(zoomInput).Children(func() {
			ui.Column(c).Width(width).FillHeight().Children(func() {
				a.timelineHeader(c, start, days, cellWidth, labelWidth)
				a.timelineList.Key = func(n int) any { return entries[n].key() }
				a.timelineList.Label = func(n int) string { return entries[n].Title }
				list := ui.List(c, &a.timelineList, len(entries), func(n int) {
					entry := entries[n]
					row := ui.Row(c.Key(entry.key())).Bind(a.timelineRows[entry.key()]).Width(width).Height(43).Gap(0).BorderWidth(0, 0, 1, 0).BorderColor(a.hairline())
					if a.timelineSelected[entry.key()] {
						row.Background(a.theme.Accent.Alpha(.11))
					}
					if a.timelineFocus == entry.key() {
						row.Border(1, a.theme.Accent.Alpha(.6))
					}
					if entry.Kind == "project" {
						// A project takes the dependency dragged from another.
						if dragged, over := ui.DragOver[timelineDrag](row); over && dragged.Kind == "dependency" && dragged.ID != entry.ID {
							row.Border(1, a.theme.Accent)
						}
						if dragged, drop := ui.Drop[timelineDrag](row); drop && dragged.Kind == "dependency" && dragged.ID != entry.ID {
							a.apply(func(d *core.Data) error {
								p := d.Projects[entry.ID]
								if !slices.Contains(p.Dependencies, dragged.ID) {
									p.Dependencies = append(slices.Clone(p.Dependencies), dragged.ID)
								}
								return d.PutProject(p)
							}, nil)
						}
					}
					row.Draw(func(p *ui.Painter, r ui.Rect) { a.paintTimelineDays(p, r, start, days, cellWidth, labelWidth) })
					hovered := row.Hovered()
					shownStart, shownEnd := a.timelineShown(entry, cellWidth)
					names := a.tk.BgContainer
					if a.timelineSelected[entry.key()] {
						names = a.theme.Accent.Alpha(.11).Over(names)
					}
					row.Children(func() {
						ui.Row(c).Width(labelWidth).FillHeight().Padding(0, 10).Gap(5).Sticky().Left(0).ZIndex(2).Background(names).BorderWidth(0, 1, 0, 0).BorderColor(a.hairline()).Children(func() {
							button := textButton(c, entry.Title).Label(a.tr("时间线项目：", "Timeline item: ") + entry.Title).Grow(1).MinWidth(0)
							if button.Clicked() {
								a.selectTimeline(entry, button.ClickModifiers())
							}
							if button.DoubleClicked() {
								a.openTimelineEntry(entry)
							}
							label := a.tr("时间线日期：", "Timeline date: ") + entry.Title
							if entry.Kind == "milestone" {
								a.dateButton(c, shownEnd, label, func(value string) { a.setTimelineDates(entry, "", value) }, a.tr("安排日期", "Schedule")).MinWidth(114).Shrink(0)
							} else {
								// While a bar is dragged, its dates follow it.
								start, end := a.timelineDates(entry)
								if shownStart != entry.Start || shownEnd != entry.End {
									start, end = shownStart, shownEnd
								}
								a.rangeButton(c, start, end, label, func(start, end string) { a.setTimelineDates(entry, start, end) }, a.tr("安排日期", "Schedule")).MinWidth(114).Shrink(0)
							}
						})
						a.timelineBar(c, entry, shownStart, shownEnd, start, days, cellWidth, labelWidth, hovered)
					})
				}).Grow(1).MinHeight(0).HandleInput(zoomInput)
				list.DrawOver(func(p *ui.Painter, r ui.Rect) {
					// Lines pass under the names, which stay at the left.
					left := labelWidth + a.timelineScroll.X
					p.Clip(ui.Rect{X: r.X + left, Y: r.Y, W: max(0, r.W-left), H: r.H}, 0, func() {
						for _, link := range a.timelineGeometry(c, entries, start, cellWidth, labelWidth) {
							color := a.tk.Primary
							if link.Violated {
								color = ui.Hex("#ef7777")
							}
							for n := 1; n < len(link.Points); n++ {
								f, t := link.Points[n-1], link.Points[n]
								p.Line(f.X, f.Y, t.X, t.Y, 1.5, color)
							}
							end := link.Points[len(link.Points)-1]
							p.Line(end.X, end.Y, end.X-5, end.Y-4, 1.5, color)
							p.Line(end.X, end.Y, end.X-5, end.Y+4, 1.5, color)
						}
					})
				})
				list.ObserveInput(func(ev ui.InputEvent) {
					if ev.Kind != ui.InputPointerUp || ev.Canceled {
						return
					}
					bounds := a.timelineList.Bounds(c)
					x, y := bounds.X+ev.X, bounds.Y+ev.Y
					if x < bounds.X+labelWidth+a.timelineScroll.X {
						return
					}
					for _, link := range a.timelineGeometry(c, entries, start, cellWidth, labelWidth) {
						for n := 1; n < len(link.Points); n++ {
							f, t := link.Points[n-1], link.Points[n]
							if x >= min(f.X, t.X)-3 && x <= max(f.X, t.X)+3 && y >= min(f.Y, t.Y)-3 && y <= max(f.Y, t.Y)+3 {
								a.timelineLinkFrom, a.timelineLinkTo = link.From, link.To
								return
							}
						}
					}
				})
			})
		})
	})
}
func (a *app) projectDependencies(c *ui.Context, id string) {
	p := a.data.Projects[id]
	ui.Row(c).Height(28).Children(func() {
		ui.Text(c, a.tr("依赖", "Dependencies")).FontSize(13).FontWeight(500).TextColor(a.tk.TextSecondary).Grow(1)
		choices := []pickerItem{}
		for _, v := range a.data.ProjectList(false, false) {
			if v.ID != id {
				choices = append(choices, pickerItem{id: v.ID, label: v.Name, checked: slices.Contains(p.Dependencies, v.ID)})
			}
		}
		a.picker(c, iconButton(c, "plus", a.tr("设置依赖", "Set dependencies")), a.tr("前置项目…", "Choose prerequisite projects"), "", 0, choices, true, func(target string) {
			a.apply(func(d *core.Data) error {
				p := d.Projects[id]
				if slices.Contains(p.Dependencies, target) {
					p.Dependencies = slices.DeleteFunc(slices.Clone(p.Dependencies), func(v string) bool { return v == target })
				} else {
					p.Dependencies = append(slices.Clone(p.Dependencies), target)
				}
				return d.PutProject(p)
			}, nil)
		})
	})
	for _, target := range p.Dependencies {
		other := a.data.Projects[target]
		ui.Row(c).Gap(10).Children(func() {
			ui.Text(c, a.tr("依赖于", "Depends on")).Width(90).TextColor(a.theme.TextMuted)
			textButton(c, other.Name).Grow(1).OnClick(func() { a.navigate("project", target); a.projectTab = "overview" })
			iconButton(c, "close", a.tr("移除依赖：", "Remove dependency: ")+other.Name).OnClick(func() {
				a.apply(func(d *core.Data) error {
					p := d.Projects[id]
					p.Dependencies = slices.DeleteFunc(slices.Clone(p.Dependencies), func(v string) bool { return v == target })
					return d.PutProject(p)
				}, nil)
			})
		})
	}
	incoming := []core.Project{}
	for _, other := range a.data.Projects {
		if other.DeletedAt == 0 && slices.Contains(other.Dependencies, id) {
			incoming = append(incoming, other)
		}
	}
	slices.SortFunc(incoming, func(x, y core.Project) int { return strings.Compare(x.Name, y.Name) })
	for _, other := range incoming {
		ui.Row(c).Gap(10).Children(func() {
			ui.Text(c, a.tr("后续项目", "Required by")).Width(90).TextColor(a.theme.TextMuted)
			textButton(c, other.Name).OnClick(func() { a.navigate("project", other.ID); a.projectTab = "overview" })
		})
	}
}
