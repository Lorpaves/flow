package main

import (
	"flow/internal/core"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/field"
)

func (a *app) currentIssueAction() (core.Issue, bool) {
	id := a.issueID
	if a.peekOpen {
		id = a.peekID
	}
	if id == "" {
		if a.timelineVisible() || a.page == "project" && a.projectTab != "issues" {
			return core.Issue{}, false
		}
		id = a.focusedIssue
	}
	i, ok := a.data.Issues[id]
	return i, ok
}

func (a *app) openPeek(id string) {
	if _, ok := a.data.Issues[id]; !ok {
		return
	}
	a.peekID, a.peekOpen = id, true
	a.revealIssue = true
}

// Peek is a read-only summary anchored to the focused list/card. It does not
// create navigation history, focus an editor, or change the current selection.
func (a *app) peekCard(c *ui.Context, anchor ui.Element, issue core.Issue) {
	if !a.peekOpen || a.peekID != issue.ID || a.peekRendered {
		return
	}
	a.peekRendered = true
	if d := a.drafts[issue.ID]; d != nil {
		issue = d.issue
	}
	ui.PopoverBase(c, anchor, &a.peekOpen, func(panel ui.Element) {
		w, _ := c.Size()
		a.popoverSurface(panel)
		panel.Width(min(440, w-40)).Padding(14, 16).Gap(10).Radius(a.tk.BorderRadiusLG).Label(a.tr("任务预览", "Issue preview"))
		ui.Text(c, issue.Title).FontSize(a.tk.FontSizeLG).FontWeight(600).MaxLines(3)
		ui.Row(c).Wrap().Gap(12).FontSize(13).TextColor(a.tk.TextSecondary).Children(func() {
			ui.Text(c, issue.Code()).TextColor(a.tk.TextTertiary)
			ui.Row(c).Gap(6).Children(func() {
				statusMark(c, issue.Status)
				ui.Text(c, a.status(issue.Status))
			})
			if issue.Priority > 0 {
				ui.Row(c).Gap(6).Children(func() {
					priorityMark(c, issue.Priority)
					ui.Text(c, a.priority(issue.Priority))
				})
			}
			if p, ok := a.data.Projects[issue.ProjectID]; ok {
				ui.Row(c).Gap(6).Children(func() {
					ui.Box(c).Size(8, 8).Radius(3).Background(ui.Hex(p.Color))
					ui.Text(c, p.Name).SingleLine()
				})
			}
			if issue.Due != "" {
				ui.Text(c, a.shortDate(issue.Due)).TextColor(a.dueColor(issue.Due, issue.Status))
			}
		})
		if labels, _ := a.issueLabels(issue.LabelIDs); len(labels) > 0 {
			ui.Row(c).Wrap().Gap(6).Children(func() {
				for _, l := range labels {
					a.labelPill(c.Key(l.ID), l.Color, l.Name)
				}
			})
		}
		if text := strings.TrimSpace(issue.Plain); text != "" {
			ui.Text(c, text).FontSize(13).MaxLines(4).TextColor(a.tk.TextTertiary)
		}
	})
}

func (a *app) peekShortcuts(c *ui.Context) {
	if !a.peekOpen || c.FocusTakesText() {
		return
	}
	c.OnShortcut(0, ui.KeySpace, func() { a.peekOpen = false })
	c.OnShortcut(0, ui.KeyEnter, func() { a.openIssue(a.peekID) })
	for _, v := range []struct {
		key  ui.Key
		step int
	}{{ui.KeyJ, 1}, {ui.KeyDown, 1}, {ui.KeyK, -1}, {ui.KeyUp, -1}} {
		c.OnShortcut(0, v.key, func() { a.stepList(v.step); a.peekID = a.focusedIssue })
	}
}

func (a *app) openParentIssue(id string) {
	i := a.data.Issues[id]
	if p, ok := a.data.Issues[i.ParentID]; ok && p.DeletedAt == 0 {
		a.openIssue(p.ID)
	}
}

func (a *app) childIssues(parent string) []core.Issue {
	return slices.DeleteFunc(a.data.Query(core.Filter{IncludeArchived: true}), func(i core.Issue) bool { return i.ParentID != parent })
}

func (a *app) openChildIssue(id string) {
	children := a.childIssues(id)
	if len(children) == 1 {
		a.openIssue(children[0].ID)
	}
	if len(children) > 1 {
		a.childParent, a.childQuery, a.childOpen = id, "", true
	}
}

func (a *app) childDialog(c *ui.Context) {
	ui.DialogBase(c, &a.childOpen, func(backdrop, panel ui.Element) {
		a.dialogChrome(backdrop, panel)
		panel.Width(480).Padding(16).Gap(12)
		a.dialogTitle(c, a.tr("子任务", "Sub-issues"), nil)
		search := field.Input(c, &a.childQuery, field.InputProps{Prefix: glyph("search"), Placeholder: a.tr("搜索", "Search")}).AutoFocus().Label(a.tr("搜索子任务", "Search sub-issues"))
		children := slices.DeleteFunc(a.childIssues(a.childParent), func(i core.Issue) bool {
			return !strings.Contains(strings.ToLower(i.Code()+" "+i.Title), strings.ToLower(a.childQuery))
		})
		open := func(id string) { a.childOpen = false; a.openIssue(id) }
		search.OnTextKey(func(k ui.TextKey) bool {
			if k.Key != ui.KeyEnter || k.Mods != 0 {
				return false
			}
			if len(children) > 0 {
				open(children[0].ID)
			}
			return true
		})
		ui.Scroll(c).Height(280).Children(func() {
			for _, child := range children {
				row := ui.ButtonBase(c.Key(child.ID)).Height(36).Padding(0, 8).Gap(8).Radius(a.tk.BorderRadiusSM).FillWidth().Justify(ui.Start).Cursor(ui.CursorPointer)
				if row.Hovered() {
					row.Background(a.rowHover())
				}
				row.Children(func() {
					ui.Row(c).Size(16, 16).Shrink(0).Center().Children(func() { statusMark(c, child.Status) })
					ui.Text(c, child.Code()).FontSize(12).TextColor(a.tk.TextTertiary)
					ui.Text(c, child.Title).Grow(1).MinWidth(0).SingleLine()
				}).OnClick(func() { open(child.ID) })
			}
			if len(children) == 0 {
				ui.Text(c, a.tr("没有匹配项", "No matches")).FontSize(13).Padding(8).TextColor(a.tk.TextTertiary)
			}
		})
		c.OnShortcut(0, ui.KeyEnter, func() {
			if len(children) > 0 {
				open(children[0].ID)
			}
		})
	})
}

func (a *app) issueActionShortcuts(c *ui.Context) {
	if c.FocusTakesText() {
		return
	}
	i, ok := a.currentIssueAction()
	if !ok {
		return
	}
	for _, v := range []struct {
		mods ui.Modifiers
		key  ui.Key
		kind string
	}{
		{ui.Cmd, ui.KeyPeriod, "id"}, {ui.Cmd | ui.Shift, ui.KeyPeriod, "branch"},
		{ui.Cmd | ui.Shift, ui.KeyComma, "url"}, {ui.Cmd | ui.Shift, ui.KeyQuote, "title"},
		{ui.Cmd | ui.Alt, ui.KeyC, "markdown"}, {ui.Cmd | ui.Alt, ui.KeyP, "prompt"},
	} {
		c.OnShortcut(v.mods, v.key, func() { a.copyIssue(i, v.kind) })
	}
	if i.DeletedAt != 0 {
		return
	}
	c.OnShortcut(ui.Cmd|ui.Shift, ui.KeyUp, func() { a.openParentIssue(i.ID) })
	c.OnShortcut(ui.Cmd|ui.Shift, ui.KeyDown, func() { a.openChildIssue(i.ID) })
	c.OnShortcut(ui.Cmd|ui.Shift, ui.KeyO, func() { a.newRelatedTask(i.ID, "sub_issue") })
	c.OnShortcut(ui.Cmd|ui.Shift, ui.KeyP, func() { a.chooseRelationship(i.ID, "parent") })
	c.OnShortcut(ui.Shift, ui.KeyH, func() { a.remindIssue(i.ID) })
	c.OnShortcut(ui.Ctrl, ui.KeyM, func() {
		if a.issueID == "" && !a.peekOpen {
			a.openIssue(i.ID)
		}
		a.selectCommentComposer(i.ID, "")
	})
	c.OnShortcut(ui.Shift, ui.KeyD, func() {
		a.chooseDate(a.tr("截止日期", "Due date"), i.Due, func(v string) { a.batchUpdate([]string{i.ID}, func(issue *core.Issue) { issue.Due = v }) })
	})
	c.OnShortcut(ui.Cmd|ui.Shift, ui.KeyD, func() { a.batchUpdate([]string{i.ID}, func(issue *core.Issue) { issue.Due = "" }) })
	c.OnShortcut(ui.Alt, ui.KeyF, func() { a.batchUpdate([]string{i.ID}, func(issue *core.Issue) { issue.Favorite = !issue.Favorite }) })
	c.OnShortcut(ui.Shift, ui.KeyR, func() {
		if !a.peekOpen {
			a.openIssue(i.ID)
		}
		a.drafts[i.ID].wantTitle = true
	})
	c.OnShortcut(ui.Ctrl|ui.Shift, ui.KeyI, func() {
		if !a.peekOpen {
			a.openIssue(i.ID)
		}
		d := a.drafts[i.ID]
		d.editing, d.wantDescription = true, true
	})
}
