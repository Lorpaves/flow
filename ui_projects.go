package main

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
	"github.com/lobehub/lobe-mygo/pkg/choice"
	"github.com/lobehub/lobe-mygo/pkg/display"
	"github.com/lobehub/lobe-mygo/pkg/feedback"
	"github.com/lobehub/lobe-mygo/pkg/field"
	"github.com/lobehub/lobe-mygo/pkg/theme"
)

func (a *app) projectsView(c *ui.Context) {
	projects := a.data.ProjectList(a.page == "archive", false)
	if a.page == "projects" {
		ui.Row(c).Height(56).Shrink(0).Padding(0, 14).Children(func() {
			current := 0
			if a.projectsTimeline {
				current = 1
			}
			a.segmented(c, "projects-layout", []string{a.tr("列表", "List"), a.tr("时间线", "Timeline")}, current, true, func(n int) { a.projectsTimeline = n == 1 })
		})
	}
	if a.page == "projects" && a.projectsTimeline {
		a.projectTimeline(c)
		return
	}
	if len(projects) == 0 {
		if a.page == "archive" {
			a.empty(c, "archive", a.tr("没有归档的项目", "No archived projects"), "", nil)
		} else {
			a.empty(c, "project", a.tr("还没有项目", "No projects yet"), a.tr("新建项目", "Create a project"), func() { a.editProject("") })
		}
		return
	}
	ui.List(c, nil, len(projects), func(n int) {
		p := projects[n]
		done, total := a.data.Progress(p.ID)
		// Each project is its own white card on the tile.
		b := a.lift(ui.ButtonBase(c.Key(p.ID)).Height(60).FillWidth().Padding(0, 16, 0, 18).Gap(12).Radius(18).Background(a.tk.BgContainer).Cursor(ui.CursorPointer))
		if b.Hovered() {
			b.Background(theme.Mix(a.tk.BgContainer, a.tk.Text, .025))
		}
		b.Children(func() {
			ui.Box(c).Size(10, 10).Radius(5).Background(ui.Hex(p.Color))
			ui.Text(c, p.Name).FontWeight(600).SingleLine().Shrink(0)
			ui.Text(c, p.Description).SingleLine().Grow(1).MinWidth(0).FontSize(13).TextColor(a.tk.TextTertiary)
			if p.Due != "" {
				ui.Text(c, a.shortDate(p.Due)).FontSize(12).SingleLine().TextColor(a.tk.TextTertiary)
			}
			display.Tag(c, a.projectStatus(p.Status), display.TagProps{Size: theme.Small})
			ui.Row(c).Width(132).Gap(8).Children(func() {
				ui.Column(c).Grow(1).AlignItems(ui.Stretch).Children(func() {
					percent := float32(0)
					if total > 0 {
						percent = float32(done) * 100 / float32(total)
					}
					feedback.Progress(c, percent, feedback.ProgressProps{Size: theme.Small, Color: ui.Hex(p.Color)})
				})
				ui.Text(c, fmt.Sprintf("%d/%d", done, total)).Width(36).FontSize(12).TextAlign(ui.End).TextColor(a.tk.TextTertiary)
			})
		})
		b.OnClick(func() { a.navigate("project", p.ID) })
	}).Grow(1).Padding(0, 14, 14, 14).Gap(8)
}
func (a *app) overview(c *ui.Context) {
	p := a.data.Projects[a.projectID]
	done, total := a.data.Progress(p.ID)
	percent := float32(0)
	if total > 0 {
		percent = float32(done) * 100 / float32(total)
	}
	a.projectNavigation(c)
	ui.Scroll(c).Grow(1).Padding(4, 14, 24, 14).Children(func() {
		a.card(ui.Column(c)).FillWidth().MaxWidth(840).Margin(0, ui.Auto).Padding(28, 40, 36, 40).Gap(16).Children(func() {
			ui.Row(c).Gap(12).Children(func() {
				ui.Box(c).Size(12, 12).Radius(6).Background(ui.Hex(p.Color))
				ui.Text(c, p.Name).FontSize(a.tk.FontSizeHeading3).FontWeight(600).Grow(1).MinWidth(0)
			})
			ui.Row(c).Wrap().Gap(6).Children(func() {
				display.Tag(c, a.projectStatus(p.Status), display.TagProps{})
				if p.Health != "" {
					display.Tag(c, a.health(p.Health), display.TagProps{Color: map[string]display.TagColor{"on_track": display.TagSuccess, "at_risk": display.TagWarning, "off_track": display.TagError}[p.Health]})
				}
				if p.Start != "" || p.Due != "" {
					span := a.shortDate(p.Due)
					if p.Start != "" && p.Due != "" {
						span = a.shortDate(p.Start) + " → " + a.shortDate(p.Due)
					} else if p.Due == "" {
						span = a.shortDate(p.Start) + " →"
					}
					display.Tag(c, span, display.TagProps{Icon: glyph("calendar")})
				}
				if p.Archived {
					display.Tag(c, a.tr("已归档", "Archived"), display.TagProps{Icon: glyph("archive")})
				}
				for _, id := range p.LabelIDs {
					if l, ok := a.data.Labels[id]; ok {
						a.labelPill(c.Key(l.ID), l.Color, l.Name)
					}
				}
			})
			ui.Row(c).Gap(12).Children(func() {
				ui.Column(c).Grow(1).AlignItems(ui.Stretch).Children(func() {
					feedback.Progress(c, percent, feedback.ProgressProps{Color: ui.Hex(p.Color)})
				})
				tip := ui.Text(c, fmt.Sprintf("%d / %d", done, total)).FontSize(13).TextColor(a.tk.TextSecondary)
				overlayTip(c, tip, a.tr("不含已取消和重复的任务", "Canceled and duplicate issues are not counted"), "")
			})
			if strings.TrimSpace(p.Description) != "" {
				markdown.View(c, p.Description, a.mediaMarkdownOptions()).FontSize(15)
			}
			a.projectDependencies(c, p.ID)
			a.attachmentSection(c, "project", p.ID)
			a.activitySection(c, "project", p.ID)
		})
	})
}
func (a *app) createDialog(c *ui.Context) {
	ui.DialogBase(c, &a.newOpen, func(backdrop, panel ui.Element) {
		a.propertyShortcuts(c)
		a.dialogChrome(backdrop, panel)
		height := float32(112)
		width := float32(680)
		if a.composeExpanded {
			height = 260
			width = 860
		}
		w, _ := c.Size()
		panel.Width(min(width, w-36)).Padding(0).Gap(0)
		ui.Row(c).Height(44).Padding(0, 8, 0, 16).Gap(4).Children(func() {
			title := a.tr("新建任务", "New issue")
			if a.newIssue.ParentID != "" {
				title = a.tr("新建子任务", "New sub-issue")
			}
			ui.Text(c, title).FontWeight(600).Grow(1)
			iconButton(c, "expand", a.tr("展开 / 收起", "Expand / collapse")).OnClick(func() { a.composeExpanded = !a.composeExpanded })
			hotIconButton(c, "close", a.tr("关闭创建窗口", "Close create dialog"), "esc").OnClick(a.closeComposition)
		})
		ui.Column(c).Padding(0, 16, 12, 16).Gap(8).Children(func() {
			if a.newRelation != nil {
				target, exists := a.data.Issues[a.newRelation.IssueID]
				label := a.creationRelationLabel(a.newRelation.Kind) + " · " + target.Code() + " " + target.Title
				if !exists || target.DeletedAt != 0 {
					label = a.tr("关联任务已删除", "Related issue was deleted")
				}
				ui.Row(c).Height(32).Padding(0, 4, 0, 10).Gap(8).Radius(a.tk.BorderRadiusSM).Background(a.tk.FillTertiary).Children(func() {
					ui.Icon(c, glyph("relation")).FontSize(14).TextColor(a.tk.TextTertiary)
					ui.Text(c, label).Grow(1).MinWidth(0).SingleLine().FontSize(13).TextColor(a.tk.TextSecondary)
					iconButton(c, "close", a.tr("解除关系", "Remove creation relation")).Size(24, 24).OnClick(a.detachCreationRelation)
				})
			}
			ui.TextInput(c.Key("new-title"), &a.newIssue.Title).Bind(&a.newTitle).AutoFocus().Placeholder(a.tr("标题", "Title")).PlaceholderColor(a.tk.TextPlaceholder).Label(a.tr("新任务标题", "New issue title")).FontSize(a.tk.FontSizeHeading4).FontWeight(600).Height(36).Padding(0).Background(ui.Transparent).Border(0, ui.Transparent).FocusRing(false)
			a.richEditor(c, "new-description", a.tr("新任务说明", "New issue description"), a.tr("添加说明…", "Add a description…"), a.newDoc, &a.newEditor, height, false, func() { a.newBody = a.newDoc.Text().PlainText() }, func() { a.chooseEditorMedia(editorMediaTarget{kind: "draft", id: a.composeID}, a.newDoc) }, editorMediaTarget{kind: "draft", id: a.composeID})
			if a.data.HasDraftAttachments(a.composeID) {
				ui.Scroll(c.Key("creation-attachments")).MaxHeight(100).Children(func() { a.attachmentItems(c, "draft", a.composeID) })
			}
			ui.Row(c).Wrap().Gap(6).Children(func() {
				a.statusPicker(c, a.newIssue.Status, func(v string) { a.newIssue.Status = v }).Label(a.tr("新任务状态", "New issue status"))
				a.priorityPicker(c, a.newIssue.Priority, func(v int) { a.newIssue.Priority = v }).Label(a.tr("新任务优先级", "New issue priority"))
				a.projectPicker(c, a.newIssue.ProjectID, func(v string) { a.newIssue.ProjectID = v }).Disabled(a.newIssue.ParentID != "" || a.newRelation != nil && a.newRelation.Kind == "parent_issue")
				a.labelPicker(c, a.newIssue.LabelIDs, func(v []string) { a.newIssue.LabelIDs = v }, a.newIssue.ProjectID).Label(a.tr("新任务标签", "New issue labels"))
				a.milestonePicker(c, a.newIssue.ProjectID, a.newIssue.MilestoneID, func(v string) { a.newIssue.MilestoneID = v })
				a.dateButton(c, a.newIssue.Due, a.tr("设置截止日期", "Set due date"), func(value string) { a.newIssue.Due = value; a.newDueOpen = value != "" }, a.tr("截止日期", "Due date"))
			})
			a.dialogError(c)
			if a.formError != "" {
				ui.Text(c, a.errorText(a.formError)).FontSize(12).TextColor(a.tk.Error)
			}
		})
		ui.Row(c).Height(52).Padding(0, 12, 0, 10).Gap(4).BorderWidth(1, 0, 0, 0).BorderColor(a.tk.BorderSecondary).Children(func() {
			iconButton(c, "template", a.tr("模板", "Templates")).PopoverMenuButton(func(m *ui.Menu) {
				if m.Item(a.tr("保存为模板", "Save as template")).Chosen() {
					a.startTemplate("", a.compositionForm())
				}
				m.Separator()
				for _, v := range a.templateList() {
					if m.Item(v.Name).Chosen() {
						a.useTemplate(v.ID)
					}
				}
			})
			iconButton(c, "trash", a.tr("清空草稿", "Discard draft")).Disabled(a.pending > 0).OnClick(a.discardComposition)
			ui.Spacer(c)
			choice.Checkbox(c, &a.createMore, a.tr("继续创建", "Create more"), choice.CheckboxProps{}).Margin(0, 8, 0, 0).TextColor(a.tk.TextSecondary)
			create := primary(c, a.tr("创建", "Create issue")).Label(a.tr("创建任务", "Create issue")).Disabled(a.pending > 0).OnClick(a.createIssue)
			overlayTip(c, create, a.tr("创建任务", "Create issue"), "mod+enter")
		})
		c.OnShortcut(ui.Cmd, ui.KeyEnter, func() {
			if a.pending == 0 {
				a.createIssue()
			}
		})
	})
}
func (a *app) projectDialog(c *ui.Context) {
	ui.DialogBase(c, &a.projectOpen, func(backdrop, panel ui.Element) {
		a.dialogChrome(backdrop, panel)
		panel.Width(600).Padding(0).Gap(0)
		ui.Row(c).Height(44).Padding(0, 8, 0, 16).Gap(4).Children(func() {
			title := a.tr("新建项目", "New project")
			if a.projectForm.ID != "" {
				title = a.tr("编辑项目", "Edit project")
			}
			ui.Text(c, title).FontWeight(600).Grow(1)
			iconButton(c, "close", a.tr("关闭项目窗口", "Close project dialog")).OnClick(func() { a.projectOpen = false })
		})
		ui.Column(c).Padding(0, 16, 12, 16).Gap(10).Children(func() {
			ui.TextInput(c.Key("project-name"), &a.projectForm.Name).AutoFocus().Placeholder(a.tr("名称", "Name")).PlaceholderColor(a.tk.TextPlaceholder).Label(a.tr("项目名称", "Project name")).Height(36).FontSize(a.tk.FontSizeHeading4).FontWeight(600).Padding(0).Background(ui.Transparent).Border(0, ui.Transparent).FocusRing(false)
			field.TextArea(c.Key("project-description"), &a.projectForm.Description, field.TextAreaProps{Placeholder: a.tr("说明", "Description"), Variant: field.InputFilled, MinRows: 3}).Label(a.tr("项目说明", "Project description"))
			ui.Row(c).Wrap().Gap(6).Children(func() {
				a.projectStatusPicker(c, a.projectForm.Status, func(v string) { a.projectForm.Status = v })
				a.priorityPicker(c, a.projectForm.Priority, func(v int) { a.projectForm.Priority = v }).Label(a.tr("项目优先级", "Project priority"))
				a.healthPicker(c, a.projectForm.Health, func(v string) { a.projectForm.Health = v })
				a.projectLabelControl(c, a.projectForm, true)
				a.dateButton(c, a.projectForm.Start, a.tr("项目开始日期", "Project start date"), func(v string) { a.projectForm.Start = v }, a.tr("开始", "Start"))
				a.dateButton(c, a.projectForm.Due, a.tr("项目目标日期", "Project target date"), func(v string) { a.projectForm.Due = v }, a.tr("目标", "Target"))
			})
			ui.Row(c).Gap(6).Children(func() {
				for _, color := range []string{"#a799e8", "#78b5a6", "#dbaa75", "#d5889e", "#7aa9d7"} {
					b := ui.ButtonBase(c.Key(color)).Label(color).Size(24, 24).Radius(12).Center().Border(2, ui.Transparent).Cursor(ui.CursorPointer)
					if a.projectForm.Color == color {
						b.Border(2, ui.Hex(color))
					}
					b.Children(func() { ui.Box(c).Size(12, 12).Radius(6).Background(ui.Hex(color)) }).OnClick(func() { a.projectForm.Color = color })
				}
			})
			a.dialogError(c)
			if a.formError != "" {
				ui.Text(c, a.errorText(a.formError)).FontSize(12).TextColor(a.tk.Error)
			}
		})
		ui.Row(c).Height(52).Padding(0, 12).Gap(8).Justify(ui.End).BorderWidth(1, 0, 0, 0).BorderColor(a.tk.BorderSecondary).Children(func() {
			btn(c, a.tr("取消", "Cancel")).OnClick(func() { a.projectOpen = false })
			primary(c, a.tr("保存", "Save project")).Label(a.tr("保存项目", "Save project")).Disabled(a.pending > 0).OnClick(a.saveProject)
		})
	})
}
