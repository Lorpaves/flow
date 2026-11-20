package main

import (
	"errors"
	"flow/internal/core"
	"net/url"
	"slices"

	"github.com/egoist/mygo/ui"
)

func projectURL(id string) string { return "flow://project/" + url.PathEscape(id) }
func (a *app) copyProject(p core.Project, kind string) {
	text := p.ID
	if kind == "title" {
		text = p.Name
	}
	if kind == "url" {
		text = projectURL(p.ID)
	}
	a.services.WriteClipboard(text)
	a.notice = a.tr("已复制", "Copied")
}
func (a *app) changeProject(id string, change func(*core.Project)) {
	a.apply(func(d *core.Data) error {
		p, ok := d.Projects[id]
		if !ok || p.DeletedAt != 0 {
			return errors.New("project_not_found")
		}
		change(&p)
		return d.PutProject(p)
	}, nil)
}
func (a *app) chooseProjectDate(id string, start bool) {
	p, ok := a.data.Projects[id]
	if !ok || p.DeletedAt != 0 {
		return
	}
	value, label := p.Due, a.tr("项目目标日期", "Project target date")
	if start {
		value, label = p.Start, a.tr("项目开始日期", "Project start date")
	}
	a.chooseDate(label, value, func(date string) {
		a.changeProject(id, func(p *core.Project) {
			if start {
				p.Start = date
			} else {
				p.Due = date
			}
		})
	})
}
func toggleLabel(ids []string, id string) []string {
	next := slices.Clone(ids)
	if n := slices.Index(next, id); n >= 0 {
		return slices.Delete(next, n, n+1)
	}
	return append(next, id)
}
func (a *app) projectLabelControl(c *ui.Context, p core.Project, form bool) ui.Element {
	label := a.tr("项目标签", "Project labels")
	// The page's own control is an icon in the title bar; a form names it.
	b := iconButton(c, "label", label)
	if form {
		b = a.chip(c, a.tr("标签", "Labels"), func() { ui.Icon(c, glyph("label")).FontSize(14).TextColor(a.tk.TextTertiary) }).Label(label)
	}
	items := []pickerItem{}
	for _, l := range a.data.LabelsForKind("project") {
		name := l.Name
		if l.Group != "" {
			name = l.Group + " / " + name
		}
		items = append(items, pickerItem{id: l.ID, label: name, color: l.Color, checked: slices.Contains(p.LabelIDs, l.ID)})
	}
	state := ui.Local(b, "picker", func() pickerState { return pickerState{} })
	if !form {
		a.projectLabels = state
	}
	choose := func(id string) {
		if form {
			a.projectForm.LabelIDs = toggleLabel(a.projectForm.LabelIDs, id)
		} else {
			a.changeProject(p.ID, func(v *core.Project) { v.LabelIDs = toggleLabel(v.LabelIDs, id) })
		}
	}
	a.picker(c, b, a.tr("查找项目标签…", "Find project labels…"), "P L", 0, items, true, choose, func(name string) {
		state.creating = true
		var created core.Label
		a.applyWithFailure(func(d *core.Data) error {
			var e error
			created, e = d.SaveLabel(core.Label{Kind: "project", Name: name, Color: "#a799e8"})
			if e != nil {
				return e
			}
			if !form {
				current, ok := d.Projects[p.ID]
				if !ok || current.DeletedAt != 0 {
					return errors.New("project_not_found")
				}
				current.LabelIDs = append(slices.Clone(current.LabelIDs), created.ID)
				return d.PutProject(current)
			}
			return nil
		}, func() {
			state.creating = false
			if form && a.projectOpen && a.projectForm.ID == p.ID {
				a.projectForm.LabelIDs = append(slices.Clone(a.projectForm.LabelIDs), created.ID)
			}
			if a.activePicker == state && state.open {
				state.query = ""
			}
		}, func() { state.creating = false })
	})
	return b
}
func (a *app) openProjectLabels() {
	state := a.projectLabels
	if state == nil {
		return
	}
	a.closePicker()
	state.open, state.query, state.index = true, "", state.selectedIndex
	state.list.ScrollIntoView(state.index)
	a.activePicker = state
	state.search.Focus()
}
func (a *app) projectActionShortcuts(c *ui.Context) {
	if c.FocusTakesText() || a.page != "project" || a.issueID != "" {
		return
	}
	if _, focused := a.currentIssueAction(); focused && a.projectTab == "issues" {
		return
	}
	p, ok := a.data.Projects[a.projectID]
	if !ok || p.DeletedAt != 0 {
		return
	}
	for _, v := range []struct {
		mods ui.Modifiers
		key  ui.Key
		kind string
	}{{ui.Cmd, ui.KeyPeriod, "id"}, {ui.Cmd | ui.Shift, ui.KeyComma, "url"}, {ui.Cmd | ui.Shift, ui.KeyQuote, "title"}} {
		c.OnShortcut(v.mods, v.key, func() { a.copyProject(p, v.kind) })
	}
	c.OnShortcut(ui.Shift, ui.KeyH, func() { a.remindProject(p.ID) })
	c.OnShortcut(ui.Alt, ui.KeyF, func() { a.changeProject(p.ID, func(v *core.Project) { v.Favorite = !v.Favorite }) })
	c.OnShortcut(ui.Ctrl|ui.Alt, ui.KeyS, func() { a.chooseProjectDate(p.ID, true) })
	c.OnShortcut(ui.Ctrl|ui.Alt, ui.KeyD, func() { a.chooseProjectDate(p.ID, false) })
}
