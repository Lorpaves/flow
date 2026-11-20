package main

import (
	"flow/internal/core"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"
)

type pickerState struct {
	open          bool
	query         string
	index         int
	selectedIndex int
	search        ui.Handle
	list          ui.ListState
	creating      bool
}
type pickerItem struct {
	id, label, color, hint string
	// A row leads with one mark: a workflow state, a priority (stored one
	// above its value so that zero means none), a named icon, or a color dot.
	status, icon string
	priority     int
	checked      bool
}

func (v pickerItem) marked() bool {
	return v.status != "" || v.priority > 0 || v.icon != "" || v.color != ""
}

func (a *app) closePicker() {
	if a.activePicker != nil {
		a.activePicker.open = false
	}
	a.activePicker = nil
}

// chip is the value button of a property: an optional mark, then text, on
// Lobe's outlined surface.
func (a *app) chip(c *ui.Context, text string, mark func()) ui.Element {
	b := ui.ButtonBase(c).Height(30).Padding(0, 12, 0, 9).Gap(6).Radius(999).Border(1, a.tk.FillBorder()).Cursor(ui.CursorPointer)
	if b.Hovered() {
		b.Background(a.tk.FillTertiary)
	}
	return b.Children(func() {
		if mark != nil {
			ui.Row(c).Size(16, 16).Shrink(0).Center().Children(mark)
		}
		ui.Text(c, text).FontSize(13).SingleLine()
	})
}

// The state belongs to the stable anchor, never to a build-scoped Element.
func (a *app) picker(c *ui.Context, anchor ui.Element, title, hint string, shortcut ui.Key, items []pickerItem, multi bool, choose func(string), create ...func(string)) {
	state := ui.Local(anchor, "picker", func() pickerState { return pickerState{} })
	state.selectedIndex = 0
	for n, item := range items {
		if item.checked {
			state.selectedIndex = n
			break
		}
	}
	open := func() {
		wasOpen := state.open
		a.closePicker()
		state.open, state.query, state.index = !wasOpen, "", state.selectedIndex
		state.list.ScrollIntoView(state.index)
		if state.open {
			a.activePicker = state
			state.search.Focus()
		}
	}
	anchor.OnClick(open)
	if shortcut != 0 {
		a.propertyPickers[shortcut] = state
	}
	ui.PopoverBase(c, anchor, &state.open, func(panel ui.Element) {
		a.popoverChrome(panel)
		panel.Width(260).Gap(2)
		results := []pickerItem{}
		q := strings.ToLower(strings.TrimSpace(state.query))
		marks := false
		for _, item := range items {
			marks = marks || item.marked()
			if q == "" || strings.Contains(strings.ToLower(item.label+" "+item.id), q) {
				results = append(results, item)
			}
		}
		if len(create) > 0 && q != "" && !slices.ContainsFunc(items, func(v pickerItem) bool { return strings.EqualFold(v.label, strings.TrimSpace(state.query)) }) {
			results = append(results, pickerItem{id: "__create__", label: a.tr("创建标签：", "Create label: ") + strings.TrimSpace(state.query), icon: "plus"})
			marks = true
		}
		state.index = max(0, min(state.index, len(results)-1))
		selectItem := func(item pickerItem) {
			if state.creating {
				return
			}
			if item.id == "__create__" {
				create[0](strings.TrimSpace(state.query))
				return
			}
			choose(item.id)
			if !multi {
				state.open = false
				a.activePicker = nil
			}
		}
		key := func(k ui.TextKey) bool {
			if k.Mods != 0 {
				return false
			}
			switch k.Key {
			case ui.KeyPageDown:
				state.index = min(len(results)-1, state.index+7)
				state.list.ScrollIntoView(state.index)
				return true
			case ui.KeyPageUp:
				state.index = max(0, state.index-7)
				state.list.ScrollIntoView(state.index)
				return true
			case ui.KeyDown:
				if len(results) > 0 {
					state.index = (state.index + 1) % len(results)
					state.list.ScrollIntoView(state.index)
				}
				return true
			case ui.KeyUp:
				if len(results) > 0 {
					state.index = (state.index + len(results) - 1) % len(results)
					state.list.ScrollIntoView(state.index)
				}
				return true
			case ui.KeyEnter:
				if len(results) > 0 {
					selectItem(results[state.index])
				}
				return true
			}
			return false
		}
		ui.Row(c).Height(32).Padding(0, 8).Gap(6).Children(func() {
			ui.TextInput(c.Key("picker-search"), &state.query).Bind(&state.search).AutoFocus().Label(title).Placeholder(title).PlaceholderColor(a.tk.TextPlaceholder).Grow(1).MinWidth(0).Height(28).FontSize(13).Background(ui.Transparent).Border(0, ui.Transparent).Padding(0).FocusRing(false).OnTextKey(key).OnChange(func() {
				state.index = 0
				if len(state.query) == 1 {
					for _, item := range items {
						if item.hint != "" && state.query == item.hint {
							selectItem(item)
							state.query = ""
							break
						}
					}
				}
			})
			if len([]rune(hint)) <= 3 {
				a.kbd(c, hint)
			}
		})
		ui.Box(c).Height(1).Margin(0, -4, 2, -4).Background(a.tk.BorderSecondary)
		state.list.Key = func(n int) any { return results[n].id }
		ui.List(c, &state.list, len(results), func(n int) {
			item := results[n]
			row := ui.ButtonBase(c.Key(item.id)).Label(item.label).Disabled(state.creating).Height(32).Padding(0, 8).Gap(8).Radius(a.tk.BorderRadiusSM).Justify(ui.Start).Cursor(ui.CursorPointer)
			if n == state.index || row.Hovered() {
				row.Background(a.tk.FillTertiary)
			}
			row.Children(func() {
				if marks {
					ui.Row(c).Size(16, 16).Shrink(0).Center().Children(func() {
						switch {
						case item.status != "":
							statusMark(c, item.status)
						case item.priority > 0:
							priorityMark(c, item.priority-1)
						case item.icon != "":
							mark := ui.Icon(c, glyph(item.icon)).FontSize(14).TextColor(a.tk.TextTertiary)
							if item.color != "" {
								mark.TextColor(ui.Hex(item.color))
							}
						case item.color != "":
							ui.Box(c).Size(8, 8).Radius(4).Background(ui.Hex(item.color))
						}
					})
				}
				ui.Text(c, item.label).FontSize(13).Grow(1).MinWidth(0).SingleLine()
				if item.checked {
					ui.Icon(c, glyph("check")).FontSize(14).TextColor(a.tk.TextSecondary)
				}
				a.kbd(c, item.hint)
			}).OnClick(func() { selectItem(item) })
		}).Height(min(288, float32(len(results)*32)))
		if len(results) == 0 {
			ui.Text(c, a.tr("没有匹配项", "No matches")).FontSize(13).Padding(8).TextColor(a.tk.TextTertiary)
		}
	})
}

// propertyControl is the button of a status or a priority: its mark alone in a
// row, or its mark and name as a chip.
func (a *app) propertyControl(c *ui.Context, label, tip, keys, text string, compact bool, mark func()) ui.Element {
	b := ui.ButtonBase(c).Label(label).Height(28).Padding(0, 10, 0, 8).Gap(6).Radius(999).Cursor(ui.CursorPointer)
	if compact {
		b.Size(24, 24).Padding(0).Center()
	} else {
		b.Border(1, a.tk.FillBorder())
	}
	if b.Hovered() {
		b.Background(a.tk.FillTertiary)
	}
	overlayTip(c, b, tip, keys)
	return b.Children(func() {
		ui.Row(c).Size(16, 16).Shrink(0).Center().Children(mark)
		if !compact {
			ui.Text(c, text).FontSize(13).SingleLine()
		}
	})
}
func (a *app) statusControl(c *ui.Context, current string, compact bool, label string, shortcut ui.Key, set func(string)) ui.Element {
	b := a.propertyControl(c, label, a.tr("状态", "Status"), "s", a.status(current), compact, func() { statusMark(c, current) })
	items := []pickerItem{}
	for n, s := range core.Statuses {
		items = append(items, pickerItem{id: s, label: a.status(s), status: s, hint: strconv.Itoa(n + 1), checked: s == current})
	}
	a.picker(c, b, a.tr("状态…", "Change status…"), "S", shortcut, items, false, set)
	return b
}
func (a *app) statusPicker(c *ui.Context, current string, set func(string)) ui.Element {
	return a.statusControl(c, current, false, a.status(current), ui.KeyS, set)
}
func (a *app) priorityControl(c *ui.Context, current int, compact bool, label string, shortcut ui.Key, set func(int)) ui.Element {
	b := a.propertyControl(c, label, a.tr("优先级", "Priority"), "p", a.priority(current), compact, func() { priorityMark(c, current) })
	items := []pickerItem{}
	for n := 0; n <= 4; n++ {
		items = append(items, pickerItem{id: strconv.Itoa(n), label: a.priority(n), priority: n + 1, hint: strconv.Itoa(n), checked: n == current})
	}
	a.picker(c, b, a.tr("优先级…", "Set priority to…"), "P", shortcut, items, false, func(id string) { n, _ := strconv.Atoi(id); set(n) })
	return b
}
func (a *app) priorityPicker(c *ui.Context, current int, set func(int)) ui.Element {
	return a.priorityControl(c, current, false, a.priority(current), ui.KeyP, set)
}
func (a *app) projectPicker(c *ui.Context, current string, set func(string)) ui.Element {
	label := a.tr("项目", "Project")
	p, ok := a.data.Projects[current]
	if ok {
		label = p.Name
	}
	b := a.chip(c, label, func() {
		if ok {
			ui.Box(c).Size(8, 8).Radius(3).Background(ui.Hex(p.Color))
		} else {
			ui.Icon(c, glyph("project")).FontSize(14).TextColor(a.tk.TextTertiary)
		}
	}).Label(label)
	items := []pickerItem{{id: "", label: a.tr("无项目", "No project"), icon: "project", checked: current == ""}}
	for _, p := range a.data.ProjectList(false, false) {
		items = append(items, pickerItem{id: p.ID, label: p.Name, color: p.Color, checked: p.ID == current})
	}
	a.picker(c, b, a.tr("项目…", "Move to project…"), "", 0, items, false, set)
	return b
}
func (a *app) labelPicker(c *ui.Context, ids []string, set func([]string), project ...string) ui.Element {
	return a.labelControl(c, ids, ui.KeyL, set, project...)
}

// issueLabels resolves the labels an issue still has, in its own order.
func (a *app) issueLabels(ids []string) ([]core.Label, string) {
	labels, names := []core.Label{}, []string{}
	for _, id := range ids {
		if l, ok := a.data.Labels[id]; ok {
			labels = append(labels, l)
			names = append(names, l.Name)
		}
	}
	if len(names) == 0 {
		return labels, a.tr("标签", "Labels")
	}
	return labels, strings.Join(names, ", ")
}

// labelControl is the label chip of forms and the issue detail.
func (a *app) labelControl(c *ui.Context, ids []string, shortcut ui.Key, set func([]string), project ...string) ui.Element {
	labels, name := a.issueLabels(ids)
	b := ui.ButtonBase(c).Label(name).Height(28).Padding(0, 10, 0, 8).Gap(6).Radius(999).MaxWidth(280).Border(1, a.tk.FillBorder()).Cursor(ui.CursorPointer)
	if b.Hovered() {
		b.Background(a.tk.FillTertiary)
	}
	b.Children(func() {
		if len(labels) == 0 {
			ui.Row(c).Size(16, 16).Shrink(0).Center().Children(func() { ui.Icon(c, glyph("label")).FontSize(14).TextColor(a.tk.TextTertiary) })
			ui.Text(c, name).FontSize(13).SingleLine()
			return
		}
		for n, l := range labels {
			if n == 3 {
				ui.Text(c, fmt.Sprintf("+%d", len(labels)-3)).FontSize(12).TextColor(a.tk.TextTertiary)
				break
			}
			ui.Row(c.Key(l.ID)).Size(16, 16).Shrink(0).Center().Children(func() { ui.Box(c).Size(8, 8).Radius(4).Background(ui.Hex(l.Color)) })
			ui.Text(c.Key("name-"+l.ID), l.Name).FontSize(13).SingleLine()
		}
	})
	a.labelChoices(c, b, ids, shortcut, set, project...)
	return b
}

// labelStrip shows an issue's labels as pills in a list row or a card. Without
// labels it takes no room and only anchors the menu that L opens.
func (a *app) labelStrip(c *ui.Context, ids []string, shortcut ui.Key, set func([]string), project string, limit int) ui.Element {
	labels, name := a.issueLabels(ids)
	b := ui.ButtonBase(c).Label(name).Height(24).Gap(4).Radius(999).Shrink(0).Cursor(ui.CursorPointer)
	if len(labels) == 0 {
		b.Width(1).Opacity(0).IgnoreInput(true)
	} else {
		b.Children(func() {
			for n, l := range labels {
				if n == limit {
					ui.Text(c, fmt.Sprintf("+%d", len(labels)-limit)).FontSize(12).Padding(0, 4).TextColor(a.tk.TextTertiary)
					break
				}
				a.labelPill(c.Key(l.ID), l.Color, l.Name)
			}
		})
	}
	a.labelChoices(c, b, ids, shortcut, set, project)
	return b
}

// labelChoices attaches the multi-select label menu, which can also create a label.
func (a *app) labelChoices(c *ui.Context, b ui.Element, ids []string, shortcut ui.Key, set func([]string), project ...string) {
	scope := ""
	if len(project) > 0 {
		scope = project[0]
	}
	items := []pickerItem{}
	for _, l := range a.data.AvailableLabels(scope) {
		name := l.Name
		if l.Group != "" {
			name = l.Group + " / " + name
		}
		items = append(items, pickerItem{id: l.ID, label: name, color: l.Color, checked: slices.Contains(ids, l.ID)})
	}
	choose := func(id string) {
		next := slices.Clone(ids)
		if n := slices.Index(next, id); n >= 0 {
			next = slices.Delete(next, n, n+1)
		} else {
			next = append(next, id)
		}
		set(next)
	}
	state := ui.Local(b, "picker", func() pickerState { return pickerState{} })
	composeID, issueID, creatingIssue := a.composeID, a.issueID, a.newOpen
	a.picker(c, b, a.tr("添加标签…", "Add labels…"), "L", shortcut, items, true, choose, func(name string) {
		state.creating = true
		var created core.Label
		a.applyWithFailure(func(d *core.Data) error {
			var e error
			created, e = d.SaveLabel(core.Label{Name: name, ProjectID: scope, Color: "#a799e8"})
			return e
		}, func() {
			state.creating = false
			if a.activePicker == state && state.open && a.composeID == composeID && a.issueID == issueID && a.newOpen == creatingIssue {
				choose(created.ID)
				state.query = ""
			}
		}, func() { state.creating = false })
	})
}

func (a *app) visibleIssueIDs() []string {
	out := []string{}
	seen := map[string]bool{}
	for _, row := range a.issueRows(a.filtered()) {
		if row.issue.ID != "" && !seen[row.issue.ID] {
			out = append(out, row.issue.ID)
			seen[row.issue.ID] = true
		}
	}

	return out
}
func (a *app) backToIssues() {
	a.flushDrafts()
	a.closePicker()
	if len(a.returnIssues) > 0 {
		id := a.returnIssues[len(a.returnIssues)-1]
		a.returnIssues = a.returnIssues[:len(a.returnIssues)-1]
		a.issueID = id
		a.openIssue(id)
		return
	}
	a.issueID = ""
	a.revealIssue = true
	a.list.Handle.Focus()
}
func (a *app) detailPosition() (int, int) {
	ids := []string{}
	for _, id := range a.issueOrder {
		if i, ok := a.data.Issues[id]; ok && i.DeletedAt == 0 {
			ids = append(ids, id)
		}
	}
	a.issueOrder = ids
	return slices.Index(ids, a.issueID), len(ids)
}
func (a *app) stepIssue(delta int) {
	n, count := a.detailPosition()
	if n >= 0 && n+delta >= 0 && n+delta < count {
		a.openIssue(a.issueOrder[n+delta])
	}
}
func (a *app) stepList(delta int) {
	ids := a.visibleIssueIDs()
	if len(ids) == 0 {
		return
	}
	n := slices.Index(ids, a.focusedIssue)
	if n < 0 {
		n = 0
	} else {
		n = max(0, min(len(ids)-1, n+delta))
	}
	a.focusedIssue, a.keptFocus = ids[n], ids[n]
	a.list.Handle.Focus()
	a.revealIssue = true
}
func (a *app) toggleSelected(id string) {
	if id == "" {
		return
	}
	if a.selected[id] {
		delete(a.selected, id)
	} else {
		a.selected[id] = true
	}
	a.focusedIssue, a.keptFocus, a.selectAnchor = id, id, id
}
func (a *app) rowClick(id string, mods ui.Modifiers) {
	if mods&ui.Shift != 0 {
		// The pointer focuses the row it is over, so a range starts from the
		// row last chosen rather than from the focus.
		anchor := a.selectAnchor
		if anchor == "" {
			anchor = a.focusedIssue
		}
		ids := a.visibleIssueIDs()
		first, last := slices.Index(ids, anchor), slices.Index(ids, id)
		if first >= 0 && last >= 0 {
			for n := min(first, last); n <= max(first, last); n++ {
				a.selected[ids[n]] = true
			}
			return
		}
	}
	if mods&ui.Cmd != 0 {
		a.toggleSelected(id)
		return
	}
	a.openIssue(id)
}
func (a *app) detailNavigation(c *ui.Context) {
	n, count := a.detailPosition()
	if n < 0 {
		return
	}
	ui.Text(c, fmt.Sprintf("%d / %d", n+1, count)).FontSize(12).Margin(0, 4, 0, 8).TextColor(a.tk.TextQuaternary)
	hotIconButton(c, "up", a.tr("上一条任务", "Previous issue"), "k").Disabled(n == 0).OnClick(func() { a.stepIssue(-1) })
	hotIconButton(c, "down", a.tr("下一条任务", "Next issue"), "j").Disabled(n == count-1).OnClick(func() { a.stepIssue(1) })
}

func (a *app) openProperty(key ui.Key) {
	state := a.propertyPickers[key]
	if state == nil {
		return
	}
	wasOpen := state.open
	a.closePicker()
	state.open = !wasOpen
	state.query = ""
	state.index = state.selectedIndex
	state.list.ScrollIntoView(state.index)
	if state.open {
		a.activePicker = state
		state.search.Focus()
	}
}
func (a *app) propertyShortcuts(c *ui.Context) {
	if !c.FocusTakesText() {
		for _, key := range []ui.Key{ui.KeyS, ui.KeyP, ui.KeyL} {
			if key == ui.KeyP && a.page == "project" && a.issueID == "" && a.projectTab != "issues" {
				continue
			}
			c.OnShortcut(0, key, func() { a.openProperty(key) })
		}
	}
}
