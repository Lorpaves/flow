package main

import (
	"cmp"
	"encoding/json"
	"flow/internal/core"
	"slices"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/field"
)

func (a *app) templateList() []core.IssueTemplate {
	out := []core.IssueTemplate{}
	for _, v := range a.data.Templates {
		out = append(out, v)
	}
	slices.SortFunc(out, func(x, y core.IssueTemplate) int {
		if n := cmp.Compare(x.Name, y.Name); n != 0 {
			return n
		}
		return cmp.Compare(x.ID, y.ID)
	})
	return out
}
func (a *app) libraryView(c *ui.Context) {
	ui.Scroll(c).Grow(1).MinHeight(0).Padding(22, 28).Gap(12).Children(func() {
		if a.page == "templates" {
			ui.Row(c).Children(func() {
				ui.Text(c, a.tr("把常用任务结构保存为模板。", "Reuse the structure of recurring work.")).Grow(1).TextColor(a.theme.TextMuted)
				btn(c, a.tr("新建模板", "New template")).OnClick(func() { a.startTemplate("", core.Issue{Status: "backlog"}) })
			})
			for _, v := range a.templateList() {
				a.recordCard(ui.Row(c.Key(v.ID)), true).Padding(14, 16).Gap(12).Children(func() {
					ui.Column(c).Grow(1).MinWidth(0).Gap(6).Children(func() { ui.Text(c, v.Name).Bold(); ui.Text(c, v.Form.Title).SingleLine().TextColor(a.theme.TextMuted) })
					textButton(c, a.tr("使用模板", "Use template")).Label(a.tr("使用模板：", "Use template: ") + v.Name).OnClick(func() { a.useTemplate(v.ID) })
					textButton(c, a.tr("编辑", "Edit")).Label(a.tr("编辑模板：", "Edit template: ") + v.Name).OnClick(func() { a.startTemplate(v.ID, v.Form) })
					iconButton(c, "trash", a.tr("删除模板 ", "Delete template ")+v.Name).OnClick(func() { a.apply(func(d *core.Data) error { d.RemoveTemplate(v.ID); return nil }, nil) })
				})
			}
			if len(a.data.Templates) == 0 {
				ui.Text(c, a.tr("还没有模板。也可以在任务菜单或创建窗口中保存模板。", "No templates yet. Save one from an issue or the create dialog.")).TextColor(a.theme.TextMuted)
			}
			return
		}
		ui.Text(c, a.tr("未提交的内容自动保存在此处，关闭应用后仍可继续。", "Unsubmitted work is saved here, including after closing the app.")).TextColor(a.theme.TextMuted)
		items := []core.IssueDraft{}
		for _, v := range a.data.Drafts {
			items = append(items, v)
		}
		slices.SortFunc(items, func(x, y core.IssueDraft) int {
			if n := cmp.Compare(y.UpdatedAt, x.UpdatedAt); n != 0 {
				return n
			}
			return cmp.Compare(x.ID, y.ID)
		})
		if len(items) == 0 {
			ui.Text(c, a.tr("没有未完成的草稿", "No unfinished drafts")).Padding(28, 0).TextColor(a.theme.TextMuted)
		}
		for _, v := range items {
			title := v.Form.Title
			if title == "" {
				title = a.tr("无标题草稿", "Untitled draft")
			}
			blocked := v.IssueID != "" && (a.data.Issues[v.IssueID].ID == "" || a.data.Issues[v.IssueID].DeletedAt != 0)
			a.recordCard(ui.Column(c.Key(v.ID)), true).Padding(14, 16).Gap(8).Children(func() {
				ui.Row(c).Gap(12).Children(func() {
					ui.Column(c).Grow(1).MinWidth(0).Gap(6).Children(func() {
						ui.Text(c, title).Bold().SingleLine()
						ui.Text(c, time.UnixMilli(v.UpdatedAt).Format("2006-01-02 15:04")).FontSize(11).TextColor(a.theme.TextMuted)
					})
					textButton(c, a.tr("继续编辑", "Resume draft")).Label(a.tr("继续编辑草稿：", "Resume draft: ") + title).Disabled(blocked).OnClick(func() { a.resumeDraft(v.ID, false) })
					if v.IssueID != "" {
						textButton(c, a.tr("作为新任务恢复", "Recover as new issue")).Label(a.tr("恢复草稿：", "Recover draft: ") + title).OnClick(func() { a.resumeDraft(v.ID, true) })
					}
					iconButton(c, "trash", a.tr("删除草稿 ", "Delete draft ")+title).OnClick(func() {
						a.apply(func(d *core.Data) error { d.RemoveDraft(v.ID); return nil }, func() {
							for key, stored := range a.compositions {
								if stored.id == v.ID {
									delete(a.compositions, key)
								}
							}
							if a.composeID == v.ID {
								a.composeID = ""
								a.newDoc = nil
								a.newOpen = false
							}
						})
					})
				})
				if v.Form.Plain != "" {
					ui.Text(c, v.Form.Plain).SingleLine().TextColor(a.theme.TextMuted)
				}
				if blocked {
					ui.Text(c, a.tr("原任务已删除；可恢复原任务，或把文字另存为新任务。", "The original issue was deleted. Restore it, or recover this text as a new issue.")).TextColor(a.theme.TextMuted)
				}
			})
		}
	})
}
func (a *app) templateDialog(c *ui.Context) {
	ui.DialogBase(c, &a.templateOpen, func(backdrop, panel ui.Element) {
		a.dialogChrome(backdrop, panel)
		panel.Width(660).Padding(16).Gap(12)
		ui.Text(c, a.tr("任务模板", "Issue template")).FontWeight(600)
		field.Input(c.Key("template-name"), &a.templateName, field.InputProps{}).AutoFocus().Label(a.tr("模板名称", "Template name")).Placeholder(a.tr("模板名称", "Template name"))
		field.Input(c.Key("template-title"), &a.templateForm.Title, field.InputProps{}).Label(a.tr("模板任务标题", "Template issue title")).Placeholder(a.tr("默认任务标题（可选）", "Default issue title (optional)"))
		a.richEditor(c, "template-body", a.tr("模板说明", "Template description"), a.tr("默认说明…", "Default description…"), a.templateDoc, &a.templateEditor, 170, false, nil, nil)
		ui.Row(c).Wrap().Gap(8).Children(func() {
			a.statusPicker(c, a.templateForm.Status, func(v string) { a.templateForm.Status = v })
			a.priorityPicker(c, a.templateForm.Priority, func(v int) { a.templateForm.Priority = v })
			a.projectPicker(c, a.templateForm.ProjectID, func(v string) { a.templateForm.ProjectID = v })
		})
		a.dialogError(c)
		dialogFooter(c, func() {
			btn(c, a.tr("取消", "Cancel")).OnClick(func() { a.templateOpen = false })
			primary(c, a.tr("保存", "Save template")).Label(a.tr("保存模板", "Save template")).Disabled(a.pending > 0).OnClick(func() {
				a.templateForm.Body, _ = json.Marshal(a.templateDoc.Text())
				a.templateForm.Plain = a.templateDoc.Text().PlainText()
				a.saveTemplate()
			})
		})
	})
}
