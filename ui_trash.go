package main

import (
	"fmt"
	"slices"

	"flow/internal/core"

	"github.com/egoist/mygo/ui"
)

// eraseRequest is what the user asked to delete for good, waiting for them
// to confirm: issues, a project, or the whole trash.
type eraseRequest struct {
	issues  []string
	project string
	all     bool
}

func (a *app) askErase(r eraseRequest) { a.erasing, a.eraseOpen = r, true }

// eraseDialog confirms deleting for good, which nothing undoes.
func (a *app) eraseDialog(c *ui.Context) {
	ui.DialogBase(c, &a.eraseOpen, func(backdrop, panel ui.Element) {
		a.dialogChrome(backdrop, panel)
		panel.Width(420).Padding(20).Gap(10)
		r := a.erasing
		title, detail := "", a.tr("连同子任务、评论、附件和历史一起删除，无法撤销。", "Sub-issues, comments, attachments and history go with it. This can't be undone.")
		switch {
		case r.all:
			issues, projects := a.trashCounts()
			title = a.tr("清空回收站？", "Empty the trash?")
			detail = fmt.Sprintf(a.tr("%d 个任务和 %d 个项目将被永久删除，无法撤销。", "%d issues and %d projects will be deleted forever. This can't be undone."), issues, projects)
		case r.project != "":
			p := a.data.Projects[r.project]
			title = fmt.Sprintf(a.tr("永久删除项目「%s」？", "Delete “%s” forever?"), p.Name)
			detail = a.tr("项目里的任务一起删除，无法撤销。", "Its issues go with it. This can't be undone.")
		case len(r.issues) == 1:
			i := a.data.Issues[r.issues[0]]
			title = fmt.Sprintf(a.tr("永久删除「%s」？", "Delete “%s” forever?"), i.Code()+" "+i.Title)
		default:
			title = fmt.Sprintf(a.tr("永久删除 %d 个任务？", "Delete %d issues forever?"), len(r.issues))
		}
		a.dialogTitle(c, title, nil)
		ui.Text(c, detail).FontSize(13).TextColor(a.tk.TextSecondary)
		a.dialogError(c)
		dialogFooter(c, func() {
			btn(c, a.tr("取消", "Cancel")).OnClick(func() { a.eraseOpen = false })
			dangerButton(c, a.tr("永久删除", "Delete forever")).Label(a.tr("确认永久删除", "Confirm delete forever")).Disabled(a.pending > 0).OnClick(func() { a.erase(r) })
		})
	})
}

func (a *app) erase(r eraseRequest) {
	a.apply(func(d *core.Data) error {
		switch {
		case r.all:
			d.EmptyTrash()
			return nil
		case r.project != "":
			return d.EraseProject(r.project)
		}
		return d.EraseIssues(r.issues)
	}, func() {
		a.eraseOpen = false
		a.selected = map[string]bool{}
		a.forgetGone()
	})
}

func (a *app) trashCounts() (issues, projects int) {
	for _, i := range a.data.Issues {
		if i.DeletedAt != 0 {
			issues++
		}
	}
	for _, p := range a.data.Projects {
		if p.DeletedAt != 0 {
			projects++
		}
	}
	return issues, projects
}

// forgetGone lets go of issues and projects that no longer exist, deleted
// for good here or on another Mac: an open one returns to its list, and an
// edit of one is dropped rather than saved into nothing.
func (a *app) forgetGone() {
	for id := range a.drafts {
		if _, ok := a.data.Issues[id]; !ok {
			delete(a.drafts, id)
		}
	}
	a.returnIssues = slices.DeleteFunc(a.returnIssues, func(id string) bool { _, ok := a.data.Issues[id]; return !ok })
	if _, ok := a.data.Issues[a.issueID]; a.issueID != "" && !ok {
		a.issueID = ""
	}
	if _, ok := a.data.Projects[a.projectID]; a.projectID != "" && !ok {
		a.navigate("projects", "")
	}
}

// trashBar is the selection's bar in the trash: put back, or delete for good.
func (a *app) trashBar(c *ui.Context) bool {
	ids := []string{}
	for _, i := range a.filtered() {
		if a.selected[i.ID] {
			ids = append(ids, i.ID)
		}
	}
	if len(ids) == 0 {
		return false
	}
	clear := func() { a.selected = map[string]bool{} }
	ui.Row(c).Height(56).Shrink(0).Padding(0, 14).Gap(4).Children(func() {
		ui.Text(c, fmt.Sprintf(a.tr("已选 %d 项", "%d selected"), len(ids))).FontWeight(500).Margin(0, 8, 0, 4)
		textButton(c, a.tr("恢复", "Restore")).Label(a.tr("恢复所选任务", "Restore selected issues")).OnClick(func() {
			a.apply(func(d *core.Data) error {
				for _, id := range ids {
					if i, ok := d.Issues[id]; ok && i.DeletedAt != 0 {
						if e := d.RestoreIssue(id); e != nil {
							return e
						}
					}
				}
				return nil
			}, clear)
		})
		textButton(c, a.tr("永久删除", "Delete forever")).TextColor(a.tk.ErrorText).Label(a.tr("永久删除所选任务", "Delete selected issues forever")).OnClick(func() { a.askErase(eraseRequest{issues: ids}) })
		ui.Spacer(c)
		hotIconButton(c, "close", a.tr("取消选择", "Clear selection"), "esc").OnClick(clear)
	})
	return true
}

// trashMenu is a trashed issue's menu.
func (a *app) trashMenu(m *ui.Menu, i core.Issue) {
	if m.Item(a.tr("恢复", "Restore")).Chosen() {
		a.apply(func(d *core.Data) error { return d.RestoreIssue(i.ID) }, nil)
	}
	if m.Item(a.tr("永久删除…", "Delete forever…")).Danger().Chosen() {
		a.askErase(eraseRequest{issues: []string{i.ID}})
	}
}
