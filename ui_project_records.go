package main

import (
	"flow/internal/core"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
	"github.com/egoist/mygo/ui/richtext"
	"github.com/lobehub/lobe-mygo/pkg/choice"
	"github.com/lobehub/lobe-mygo/pkg/field"
	"github.com/lobehub/lobe-mygo/pkg/nav"
)

// projectNavigation is the project's tabs: Lobe's Tabs, underlined.
func (a *app) projectNavigation(c *ui.Context) {
	tabs := [][3]string{{"overview", "概览", "Overview"}, {"issues", "任务", "Issues"}, {"milestones", "里程碑", "Milestones"}, {"documents", "文档", "Documents"}, {"updates", "更新", "Updates"}, {"timeline", "时间线", "Timeline"}}
	items, current := []nav.TabItem{}, 0
	for n, v := range tabs {
		items = append(items, nav.TabItem{Label: a.tr(v[1], v[2])})
		if a.projectTab == v[0] {
			current = n
		}
	}
	index := a.boundIndex(c, "project-tab/"+a.projectID, current, func(n int) {
		a.rememberNavigation()
		a.projectTab = tabs[n][0]
		a.projectResourceQuery = ""
		a.timelineStart = ""
	})
	ui.Row(c).Shrink(0).Padding(4, 16, 0, 16).Children(func() {
		nav.Tabs(c.Key("project-tabs"), index, nav.TabsProps{Items: items, Variant: nav.TabsSquare}, nil)
	})
}
func (a *app) health(value string) string {
	for _, v := range [][3]string{{"", "未设置", "Not set"}, {"on_track", "进展正常", "On track"}, {"at_risk", "存在风险", "At risk"}, {"off_track", "进度落后", "Off track"}} {
		if v[0] == value {
			return a.tr(v[1], v[2])
		}
	}
	return value
}
func (a *app) healthPicker(c *ui.Context, value string, set func(string)) {
	items := []pickerItem{}
	for _, v := range []string{"", "on_track", "at_risk", "off_track"} {
		items = append(items, pickerItem{id: v, label: a.health(v), checked: value == v})
	}
	a.picker(c, a.chip(c, a.health(value), nil).Label(a.tr("项目健康状态", "Project health")), a.tr("选择健康状态", "Select health"), "", 0, items, false, set)
}
func (a *app) projectStatusPicker(c *ui.Context, value string, set func(string)) {
	items := []pickerItem{}
	for _, s := range core.ProjectStatuses {
		items = append(items, pickerItem{id: s, label: a.projectStatus(s), checked: s == value})
	}
	a.picker(c, a.chip(c, a.projectStatus(value), nil).Label(a.tr("项目状态", "Project status")), a.tr("选择项目状态", "Select project status"), "", 0, items, false, set)
}
func (a *app) projectRecordsView(c *ui.Context) {
	a.projectNavigation(c)
	if a.projectTab == "timeline" {
		a.projectTimeline(c)
		return
	}
	ui.Scroll(c.Key("project-records-"+a.projectTab)).Grow(1).MinHeight(0).Padding(24, 32).Gap(16).Children(func() {
		switch a.projectTab {
		case "milestones":
			a.milestonesSection(c, a.projectID)
		case "documents":
			a.documentsSection(c, "project", a.projectID)
		case "updates":
			a.updatesSection(c, a.projectID)
		}
	})
}
func (a *app) milestonePicker(c *ui.Context, project, current string, set func(string)) ui.Element {
	if project == "" {
		return ui.Element{}
	}
	label := a.tr("里程碑", "Milestone")
	if m, ok := a.data.Milestones[current]; ok {
		label = m.Name
	}
	items := []pickerItem{{id: "", label: a.tr("无里程碑", "No milestone"), icon: "milestone", checked: current == ""}}
	for _, v := range a.data.MilestoneList(project) {
		items = append(items, pickerItem{id: v.ID, label: v.Name, icon: "milestone", checked: current == v.ID})
	}
	b := a.chip(c, label, func() { icon(c, "milestone").Size(14, 14).TextColor(a.theme.TextMuted) }).Label(a.tr("任务里程碑", "Issue milestone"))
	a.picker(c, b, a.tr("选择里程碑", "Choose milestone"), "", 0, items, false, set)
	return b
}
func (a *app) editMilestone(project, id string) {
	v := a.data.Milestones[id]
	if id == "" {
		v = core.Milestone{ProjectID: project}
	}
	a.milestoneForm, a.milestoneBase = v, v
	a.milestoneOpen = true
}
func (a *app) saveMilestone() {
	form, base := a.milestoneForm, a.milestoneBase
	a.apply(func(d *core.Data) error {
		if form.ID != "" {
			if current, ok := d.Milestones[form.ID]; ok {
				form = core.MergeMilestoneEdits(base, form, current)
			}
		}
		_, e := d.SaveMilestone(form)
		return e
	}, func() { a.milestoneOpen = false })
}
func (a *app) milestonesSection(c *ui.Context, project string) {
	items := a.data.MilestoneList(project)
	count := ""
	if len(items) > 0 {
		count = fmt.Sprint(len(items))
	}
	a.sectionHeader(c, a.tr("里程碑", "Milestones"), count, func() {
		actionButton(c, "plus", a.tr("新建里程碑", "New milestone")).OnClick(func() { a.editMilestone(project, "") })
	})
	if len(items) == 0 {
		a.empty(c, "milestone", a.tr("还没有里程碑", "No milestones yet"), "", nil)
	}
	for n, v := range items {
		done, total := a.data.MilestoneProgress(v.ID)
		a.recordCard(ui.Column(c.Key(v.ID)), true).Padding(10, 10, 12, 14).Gap(4).Children(func() {
			ui.Row(c).Gap(6).Children(func() {
				check(c, v.Completed, "", 16, func() {
					a.apply(func(d *core.Data) error {
						m := d.Milestones[v.ID]
						m.Completed = !v.Completed
						_, e := d.SaveMilestone(m)
						return e
					}, nil)
				}).Label(a.tr("完成里程碑：", "Complete milestone: ") + v.Name)
				name := ui.ButtonBase(c).Grow(1).MinWidth(0).Height(28).Padding(0, 4).Justify(ui.Start)
				name.Children(func() { ui.Text(c, v.Name).FontWeight(600).SingleLine() }).OnClick(func() { a.editMilestone(project, v.ID) })
				ui.Text(c, fmt.Sprintf("%d / %d", done, total)).FontSize(12).Margin(0, 4).TextColor(a.theme.TextMuted)
				a.dateButton(c, v.Due, a.tr("里程碑日期：", "Milestone date: ")+v.Name, func(due string) {
					a.apply(func(d *core.Data) error { m := d.Milestones[v.ID]; m.Due = due; _, e := d.SaveMilestone(m); return e }, nil)
				}, a.tr("设置日期", "Set date"))
				iconButton(c, "up", a.tr("上移里程碑：", "Move milestone up: ")+v.Name).Disabled(n == 0).OnClick(func() { a.reorderMilestone(project, v.ID, -1) })
				iconButton(c, "down", a.tr("下移里程碑：", "Move milestone down: ")+v.Name).Disabled(n == len(items)-1).OnClick(func() { a.reorderMilestone(project, v.ID, 1) })
				iconButton(c, "trash", a.tr("删除里程碑：", "Delete milestone: ")+v.Name).OnClick(func() { a.apply(func(d *core.Data) error { d.RemoveMilestone(v.ID); return nil }, nil) })
			})
			if v.Description != "" {
				markdown.View(c, v.Description, a.mediaMarkdownOptions()).FontSize(13)
			}
			for _, i := range a.data.Query(core.Filter{ProjectID: project, IncludeArchived: true}) {
				if i.MilestoneID == v.ID {
					row := ui.Row(c.Key(i.ID)).Height(32).Padding(0, 4).Gap(8).Radius(6)
					if row.Hovered() {
						row.Background(a.rowHover())
					}
					row.Children(func() {
						ui.Box(c).Size(16, 16).Center().Children(func() { statusMark(c, i.Status) })
						open := ui.ButtonBase(c).Grow(1).MinWidth(0).Height(28).Gap(10).Justify(ui.Start)
						open.Children(func() {
							ui.Text(c, i.Code()).FontSize(12).TextColor(a.theme.TextMuted)
							ui.Text(c, i.Title).Grow(1).MinWidth(0).SingleLine()
						}).OnClick(func() { a.openIssue(i.ID) })
					})
				}
			}
			ui.Row(c).Children(func() {
				button := actionButton(c, "plus", a.tr("添加已有任务", "Add existing issue")).Label(a.tr("添加任务到里程碑：", "Add issue to milestone: ") + v.Name)
				choices := []pickerItem{}
				for _, i := range a.data.Query(core.Filter{ProjectID: project, IncludeArchived: true}) {
					if i.MilestoneID != v.ID {
						choices = append(choices, pickerItem{id: i.ID, label: i.Code() + " " + i.Title, status: i.Status})
					}
				}
				a.picker(c, button, a.tr("选择任务", "Choose issue"), "", 0, choices, false, func(id string) {
					a.apply(func(d *core.Data) error { i := d.Issues[id]; i.MilestoneID = v.ID; return d.PutIssue(i) }, nil)
				})
				ui.Spacer(c)
			})
		})
	}
}
func (a *app) reorderMilestone(project, id string, delta int) {
	a.apply(func(d *core.Data) error {
		items := d.MilestoneList(project)
		n := slices.IndexFunc(items, func(v core.Milestone) bool { return v.ID == id })
		if n < 0 || n+delta < 0 || n+delta >= len(items) {
			return nil
		}
		items[n], items[n+delta] = items[n+delta], items[n]
		for n, v := range items {
			v.Position = int64(n)
			d.Milestones[v.ID] = v
		}
		return nil
	}, nil)
}
func (a *app) editDocument(kind, owner, id string) {
	v := a.data.Documents[id]
	if id == "" {
		v = core.Document{OwnerKind: kind, OwnerID: owner}
	}
	a.documentForm, a.documentBase = v, v
	a.documentText = richtext.NewDocument(a.hydrateMediaText(richtext.ParseMarkdown(v.Body)))
	// Compare the editor's serialized baseline, so parsing Markdown alone is
	// not mistaken for a local edit when a newer MCP body arrives.
	a.documentBase.Body = a.documentText.Text().Markdown()
	a.documentOpen = true
}
func (a *app) saveDocument() {
	if a.pending > 0 {
		return
	}
	form, base := a.documentForm, a.documentBase
	doc, version := a.documentText, a.documentText.Version()
	form.Body = doc.Text().Markdown()
	submitted := form
	var saved core.Document
	a.apply(func(d *core.Data) error {
		if form.ID != "" {
			if current, ok := d.Documents[form.ID]; ok {
				form = core.MergeDocumentEdits(base, form, current)
			}
		}
		var e error
		saved, e = d.SaveDocument(form)
		return e
	}, func() {
		if a.documentText != doc {
			return // Another document was opened while this one was saving.
		}
		unchanged := doc.Version() == version && a.documentForm.Name == submitted.Name
		current := a.documentForm
		current.Body = doc.Text().Markdown()
		a.documentForm = core.MergeDocumentEdits(submitted, current, saved)
		a.documentBase = saved
		if doc.Version() == version {
			a.documentText = richtext.NewDocument(a.hydrateMediaText(richtext.ParseMarkdown(saved.Body)))
		}
		a.documentBase.Body = richtext.ParseMarkdown(saved.Body).Markdown()
		if unchanged {
			a.documentOpen = false
		}
	})
}
func (a *app) documentsSection(c *ui.Context, kind, id string) {
	items := []core.Document{}
	for _, v := range a.data.Documents {
		if v.OwnerKind == kind && v.OwnerID == id {
			items = append(items, v)
		}
	}
	slices.SortFunc(items, func(x, y core.Document) int { return strings.Compare(x.Name, y.Name) })
	if len(items) == 0 && kind == "issue" {
		return
	}
	ui.Column(c.Key("documents-" + kind)).Gap(6).Children(func() {
		a.sectionHeader(c, a.tr("文档", "Documents"), "", func() {
			if kind == "issue" {
				iconButton(c, "plus", a.tr("新建文档", "New document")).OnClick(func() { a.editDocument(kind, id, "") })
			} else {
				actionButton(c, "plus", a.tr("新建文档", "New document")).OnClick(func() { a.editDocument(kind, id, "") })
			}
		})
		if len(items) == 0 && kind == "project" {
			a.empty(c, "document", a.tr("还没有文档", "No documents yet"), "", nil)
		}
		for _, v := range items {
			card := a.recordCard(ui.Column(c.Key(v.ID)), kind == "project").Padding(8, 8, 12, 14).Gap(6)
			hovered := card.Hovered()
			card.Children(func() {
				ui.Row(c).Gap(4).Children(func() {
					ui.Icon(c, glyph("document")).FontSize(15).Margin(0, 6, 0, 0).TextColor(a.tk.TextTertiary)
					open := ui.ButtonBase(c).Grow(1).MinWidth(0).Height(28).Justify(ui.Start).Cursor(ui.CursorPointer)
					open.Children(func() { ui.Text(c, v.Name).FontWeight(500).SingleLine() }).OnClick(func() { a.editDocument(kind, id, v.ID) })
					ui.Text(c, a.shortTime(v.UpdatedAt)).FontSize(12).Margin(0, 6).TextColor(a.tk.TextQuaternary)
					reveal(iconButton(c, "pencil", a.tr("编辑文档：", "Edit document: ")+v.Name), hovered).OnClick(func() { a.editDocument(kind, id, v.ID) })
					reveal(iconButton(c, "trash", a.tr("删除文档：", "Delete document: ")+v.Name), hovered).OnClick(func() { a.apply(func(d *core.Data) error { delete(d.Documents, v.ID); return nil }, nil) })
				})
				if strings.TrimSpace(v.Body) != "" {
					markdown.View(c, v.Body, a.mediaMarkdownOptions()).FontSize(13)
				}
			})
		}
	})
}
func (a *app) editProjectUpdate(project, id string) {
	v := a.data.Updates[id]
	if id == "" {
		v = core.ProjectUpdate{ProjectID: project, Health: a.data.Projects[project].Health}
		if v.Health == "" {
			v.Health = "on_track"
		}
	}
	a.updateForm, a.updateBase = v, v
	a.updateText = richtext.NewDocument(a.hydrateMediaText(richtext.ParseMarkdown(v.Body)))
	a.updateBase.Body = a.updateText.Text().Markdown()
	a.updateOpen = true
}
func (a *app) saveProjectUpdate() {
	if a.pending > 0 {
		return
	}
	form, base := a.updateForm, a.updateBase
	doc, version := a.updateText, a.updateText.Version()
	form.Body = doc.Text().Markdown()
	submitted := form
	var saved core.ProjectUpdate
	a.apply(func(d *core.Data) error {
		if form.ID != "" {
			if current, ok := d.Updates[form.ID]; ok {
				form = core.MergeProjectUpdateEdits(base, form, current)
			}
		}
		var e error
		saved, e = d.SaveProjectUpdate(form)
		return e
	}, func() {
		if a.updateText != doc {
			return
		}
		unchanged := doc.Version() == version && a.updateForm.Health == submitted.Health
		current := a.updateForm
		current.Body = doc.Text().Markdown()
		a.updateForm = core.MergeProjectUpdateEdits(submitted, current, saved)
		a.updateBase = saved
		if doc.Version() == version {
			a.updateText = richtext.NewDocument(a.hydrateMediaText(richtext.ParseMarkdown(saved.Body)))
		}
		a.updateBase.Body = richtext.ParseMarkdown(saved.Body).Markdown()
		if unchanged {
			a.updateOpen = false
		}
	})
}
func (a *app) updatesSection(c *ui.Context, project string) {
	a.sectionHeader(c, a.tr("项目更新", "Project updates"), a.health(a.data.Projects[project].Health), func() {
		actionButton(c, "pencil", a.tr("撰写更新", "Write an update")).OnClick(func() { a.editProjectUpdate(project, "") })
	})
	items := []core.ProjectUpdate{}
	for _, v := range a.data.Updates {
		if v.ProjectID == project {
			items = append(items, v)
		}
	}
	slices.SortFunc(items, func(x, y core.ProjectUpdate) int {
		if x.CreatedAt > y.CreatedAt {
			return -1
		}
		if x.CreatedAt < y.CreatedAt {
			return 1
		}
		return strings.Compare(x.ID, y.ID)
	})
	if len(items) == 0 {
		a.empty(c, "comment", a.tr("还没有更新", "No updates yet"), "", nil)
	}
	for _, v := range items {
		a.recordCard(ui.Column(c.Key(v.ID)), true).Padding(14, 16).Gap(12).Children(func() {
			ui.Row(c).Gap(8).Children(func() {
				ui.Text(c, a.health(v.Health)).FontWeight(600)
				ui.Text(c, time.UnixMilli(v.CreatedAt).Format("2006-01-02 15:04")).FontSize(12).TextColor(a.theme.TextMuted)
				ui.Spacer(c)
				quietButton(c, a.tr("编辑", "Edit")).Label(a.tr("编辑更新：", "Edit update: ") + shortText(v.Body)).OnClick(func() { a.editProjectUpdate(project, v.ID) })
				iconButton(c, "trash", a.tr("删除更新：", "Delete update: ")+shortText(v.Body)).OnClick(func() { a.apply(func(d *core.Data) error { delete(d.Updates, v.ID); return nil }, nil) })
			})
			markdown.View(c, v.Body, a.mediaMarkdownOptions()).FontSize(14)
		})
	}
}
func (a *app) projectRecordDialogs(c *ui.Context) {
	ui.DialogBase(c, &a.milestoneOpen, func(backdrop, panel ui.Element) {
		a.dialogChrome(backdrop, panel)
		panel.Width(580).Padding(16).Gap(12)
		ui.Text(c, a.tr("里程碑", "Milestone")).FontWeight(600)
		field.Input(c.Key("milestone-name"), &a.milestoneForm.Name, field.InputProps{}).AutoFocus().Label(a.tr("里程碑名称", "Milestone name")).Placeholder(a.tr("阶段目标", "Stage goal"))
		field.TextArea(c.Key("milestone-description"), &a.milestoneForm.Description, field.TextAreaProps{MinRows: 4}).Label(a.tr("里程碑说明", "Milestone description"))
		a.dateButton(c, a.milestoneForm.Due, a.tr("里程碑目标日期", "Milestone target date"), func(v string) { a.milestoneForm.Due = v })
		choice.Checkbox(c, &a.milestoneForm.Completed, a.tr("已完成", "Completed"), choice.CheckboxProps{})
		a.dialogError(c)
		dialogFooter(c, func() {
			btn(c, a.tr("取消", "Cancel")).OnClick(func() { a.milestoneOpen = false })
			primary(c, a.tr("保存", "Save milestone")).Label(a.tr("保存里程碑", "Save milestone")).Disabled(a.pending > 0).OnClick(a.saveMilestone)
		})
	})
	ui.DialogBase(c, &a.documentOpen, func(backdrop, panel ui.Element) {
		a.dialogChrome(backdrop, panel)
		panel.Width(760).Padding(16).Gap(12)
		ui.Text(c, a.tr("编辑文档", "Edit document")).FontWeight(600)
		field.Input(c.Key("document-name"), &a.documentForm.Name, field.InputProps{}).AutoFocus().Label(a.tr("文档名称", "Document name")).Placeholder(a.tr("文档名称", "Document name"))
		doc := a.documentText
		media := editorMediaTarget{kind: a.documentForm.OwnerKind, id: a.documentForm.OwnerID, local: true}
		a.richEditor(c, "document-body", a.tr("文档正文", "Document body"), a.tr("记录背景与决策…", "Document context and decisions…"), doc, &a.documentEditor, 260, false, nil, func() { a.chooseEditorMedia(media, doc) }, media)
		a.dialogError(c)
		dialogFooter(c, func() {
			btn(c, a.tr("取消", "Cancel")).OnClick(func() { a.documentOpen = false })
			primary(c, a.tr("保存", "Save document")).Label(a.tr("保存文档", "Save document")).Disabled(a.pending > 0).OnClick(a.saveDocument)
		})
	})
	ui.DialogBase(c, &a.updateOpen, func(backdrop, panel ui.Element) {
		a.dialogChrome(backdrop, panel)
		panel.Width(760).Padding(16).Gap(12)
		ui.Text(c, a.tr("项目更新", "Project update")).FontWeight(600)
		a.healthPicker(c, a.updateForm.Health, func(v string) { a.updateForm.Health = v })
		doc := a.updateText
		media := editorMediaTarget{kind: "project", id: a.updateForm.ProjectID, local: true}
		a.richEditor(c, "update-body", a.tr("更新内容", "Update body"), a.tr("进展、风险与下一步…", "Progress, risks, and next steps…"), doc, &a.updateEditor, 260, true, nil, func() { a.chooseEditorMedia(media, doc) }, media)
		a.dialogError(c)
		dialogFooter(c, func() {
			btn(c, a.tr("取消", "Cancel")).OnClick(func() { a.updateOpen = false })
			primary(c, a.tr("保存", "Save update")).Label(a.tr("保存更新", "Save update")).Disabled(a.pending > 0).OnClick(a.saveProjectUpdate)
		})
	})
}
