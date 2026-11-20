package main

import (
	"flow/internal/core"
	"fmt"
	"strconv"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
	"github.com/lobehub/lobe-mygo/pkg/action"
	"github.com/lobehub/lobe-mygo/pkg/field"
	"github.com/lobehub/lobe-mygo/pkg/lucide"
	"github.com/lobehub/lobe-mygo/pkg/theme"
)

type issueDrag struct {
	ID  string
	IDs []string
}
type issueRow struct {
	parentGroup issueGroup
	title, key  string
	depth       int
	group       issueGroup
	header      string
	count       int
	issue       core.Issue
}

func (a *app) filtering() bool {
	return a.query != "" || a.statusFilter != "" || a.priorityFilter >= 0 || a.labelFilter != ""
}
func (a *app) advancedFiltering() bool {
	return len(a.viewConfig.Filter.Rules)+len(a.viewConfig.Filter.Groups) > 0
}

// filterMenu is the one place a list is narrowed from: each property in a
// submenu, then the rule editor and saving the result as a view.
func (a *app) filterMenu(m *ui.Menu) {
	m.Submenu(a.tr("状态", "Status"), func(m *ui.Menu) {
		for _, status := range core.Statuses {
			if m.Item(a.status(status)).Radio(status == a.statusFilter).Chosen() {
				a.statusFilter = status
			}
		}
	})
	m.Submenu(a.tr("优先级", "Priority"), func(m *ui.Menu) {
		for p := 1; p <= 5; p++ {
			if m.Item(a.priority(p % 5)).Radio(p%5 == a.priorityFilter).Chosen() {
				a.priorityFilter = p % 5
			}
		}
	})
	if labels := a.data.LabelList(); len(labels) > 0 {
		m.Submenu(a.tr("标签", "Label"), func(m *ui.Menu) {
			for _, l := range labels {
				if m.Item(l.Name).Radio(l.ID == a.labelFilter).Chosen() {
					a.labelFilter = l.ID
				}
			}
		})
	}
	m.Separator()
	if m.Item(a.tr("高级筛选…", "Advanced filter…")).Chosen() {
		a.viewDraft = a.viewConfig.Clone()
		a.formError = ""
		a.filterOpen = true
	}
	if m.Item(a.tr("保存为视图…", "Save as view…")).Chosen() {
		a.savedViewName = a.data.Views[a.savedViewID].Name
		a.formError = ""
		a.saveViewOpen = true
	}
	if a.filtering() || a.advancedFiltering() {
		m.Separator()
		if m.Item(a.tr("清除筛选", "Clear filters")).Chosen() {
			a.clearFilters()
		}
	}
}
func (a *app) clearFilters() {
	a.query, a.statusFilter, a.priorityFilter, a.labelFilter = "", "", -1, ""
	if a.advancedFiltering() {
		a.viewConfig.Filter = core.FilterGroup{}
		a.persistViewConfig()
	}
}

// filterChip shows one active filter and removes it when closed.
func (a *app) filterChip(c *ui.Context, key, text string, mark func(), remove func()) {
	a.lift(ui.Row(c.Key("filter-"+key)).Height(28).Padding(0, 5, 0, 10).Gap(6).Radius(999).Background(a.tk.BgContainer).Shrink(0)).Children(func() {
		if mark != nil {
			mark()
		}
		ui.Text(c, text).FontSize(12).SingleLine().TextColor(a.tk.TextSecondary)
		x := ui.ButtonBase(c).Label(a.tr("移除筛选：", "Remove filter: ")+text).Size(18, 18).Center().Radius(9).TextColor(a.tk.TextTertiary).Cursor(ui.CursorPointer)
		if x.Hovered() {
			x.Background(a.tk.FillSecondary).TextColor(a.tk.Text)
		}
		x.Children(func() { ui.Icon(c, lucide.X()).FontSize(12) }).OnClick(remove)
	})
}
func (a *app) issuesView(c *ui.Context) {
	if a.projectID != "" {
		a.projectNavigation(c)
	}
	items := a.filtered()
	switch {
	case len(a.selected) > 0 && a.page == "trash" && a.trashBar(c):
	case len(a.selected) > 0 && a.page != "trash" && a.batchBar(c):
		// The selection takes the toolbar's place: what it offers acts on the
		// chosen issues, and the list below does not move.
	default:
		a.listToolbar(c)
	}
	if a.page == "trash" {
		projects := a.data.ProjectList(false, true)
		if len(projects) > 0 {
			a.card(ui.Column(c.Key("trash-projects"))).Margin(0, 12, 10, 12).Clip().Dividers(1, a.hairline()).Children(func() {
				for _, p := range projects {
					ui.Row(c.Key("trash-project-"+p.ID)).Height(44).Padding(0, 8, 0, 18).Gap(10).Children(func() {
						ui.Box(c).Size(8, 8).Radius(4).Background(ui.Hex(p.Color))
						ui.Text(c, p.Name).Grow(1).SingleLine()
						textButton(c, a.tr("恢复", "Restore project")).OnClick(func() { a.apply(func(d *core.Data) error { d.RestoreProject(p.ID); return nil }, nil) })
						iconButton(c, "trash", a.tr("永久删除项目 ", "Delete project forever: ")+p.Name).OnClick(func() { a.askErase(eraseRequest{project: p.ID}) })
					})
				}
			})
		}
	}
	// Empty board columns still take drops and new issues; an empty list is
	// only a stack of headers, so it says what to do instead.
	if len(items) == 0 && (!a.viewConfig.EmptyGroups || !a.board || a.page == "trash") {
		switch {
		case a.page == "trash":
			a.empty(c, "trash", a.tr("回收站是空的", "Trash is empty"), "", nil)
		case a.filtering() || a.advancedFiltering():
			a.empty(c, "search", a.tr("没有匹配的任务", "No matching issues"), a.tr("清除筛选", "Clear filters"), a.clearFilters)
		default:
			a.empty(c, "issues", a.tr("还没有任务", "No issues yet"), a.tr("新建任务", "Create an issue"), func() { a.newTask("", "backlog") })
		}
		return
	}
	if a.board && a.page != "trash" {
		a.boardView(c, items)
	} else {
		a.listView(c, items)
	}
}

// listToolbar is the single row above a list: find, narrow, and how to show it.
func (a *app) listToolbar(c *ui.Context) {
	ui.Row(c).Height(56).Shrink(0).Padding(0, 14).Gap(8).Children(func() {
		a.searchPill(c, 220, func() ui.Element {
			return field.SearchBar(c.Key("issue-search"), &a.query, field.SearchBarProps{Placeholder: a.tr("搜索", "Search"), Variant: field.InputBorderless}).Bind(&a.search).Label(a.tr("搜索任务", "Search issues"))
		})
		filter := action.Button(c.Key("filter"), "", action.ButtonProps{Variant: action.Text}).Height(34).PaddingX(12).Radius(999).TextColor(a.tk.TextSecondary).Label(a.tr("筛选", "Filter"))
		filter.Children(func() {
			ui.Icon(c, lucide.ListFilter()).FontSize(16)
			ui.Text(c, a.tr("筛选", "Filter")).SingleLine()
		}).PopoverMenuButton(a.filterMenu)
		ui.Row(c).Gap(6).MinWidth(0).Clip().Children(func() {
			if a.statusFilter != "" {
				a.filterChip(c, "status", a.status(a.statusFilter), func() { statusMark(c, a.statusFilter) }, func() { a.statusFilter = "" })
			}
			if a.priorityFilter >= 0 {
				a.filterChip(c, "priority", a.priority(a.priorityFilter), func() { priorityMark(c, a.priorityFilter).Size(14, 14) }, func() { a.priorityFilter = -1 })
			}
			if l, ok := a.data.Labels[a.labelFilter]; ok {
				a.filterChip(c, "label", l.Name, func() { ui.Box(c).Size(7, 7).Radius(4).Background(ui.Hex(l.Color)) }, func() { a.labelFilter = "" })
			}
			if a.advancedFiltering() {
				a.filterChip(c, "advanced", a.tr("高级筛选", "Advanced"), nil, func() { a.viewConfig.Filter = core.FilterGroup{}; a.persistViewConfig() })
			}
		})
		ui.Spacer(c)
		if issues, projects := a.trashCounts(); a.page == "trash" && issues+projects > 0 {
			textButton(c, a.tr("清空回收站", "Empty trash")).TextColor(a.tk.ErrorText).OnClick(func() { a.askErase(eraseRequest{all: true}) })
		}
		iconButton(c, "settings", a.tr("显示设置", "Display options")).OnClick(func() { a.viewDraft = a.viewConfig.Clone(); a.displayOpen = true })
		if a.page != "trash" {
			ui.Row(c).Padding(3).Gap(2).Radius(999).Background(a.tk.FillSecondary).Children(func() {
				for _, v := range []struct {
					name, layout, zh, en string
					on                   bool
				}{{"list", "list", "列表视图", "List view", !a.board}, {"board", "board", "看板视图", "Board view", a.board}} {
					b := iconButton(c.Key(v.layout), v.name, a.tr(v.zh, v.en)).Size(34, 28)
					if v.on {
						a.lift(b.Background(a.tk.BgContainer).TextColor(a.tk.Text))
					}
					b.OnClick(func() { a.setLayout(v.layout) })
				}
			})
		}
	})
}

// groupMark draws what a group stands for: its state, priority, label or project.
func (a *app) groupMark(c *ui.Context, g issueGroup) {
	switch g.Field {
	case "status":
		statusMark(c, g.Value)
	case "priority":
		p, _ := strconv.Atoi(g.Value)
		priorityMark(c, p)
	case "labels":
		if l, ok := a.data.Labels[g.Value]; ok {
			ui.Box(c).Size(8, 8).Radius(4).Margin(0, 3).Background(ui.Hex(l.Color))
		} else {
			ui.Icon(c, lucide.Tag()).FontSize(14).TextColor(a.tk.TextQuaternary)
		}
	case "project":
		if p, ok := a.data.Projects[g.Value]; ok {
			ui.Box(c).Size(8, 8).Radius(3).Margin(0, 3).Background(ui.Hex(p.Color))
		} else {
			ui.Icon(c, lucide.Box()).FontSize(14).TextColor(a.tk.TextQuaternary)
		}
	case "due":
		ui.Icon(c, lucide.Calendar()).FontSize(14).TextColor(a.tk.TextQuaternary)
	}
}
func (a *app) groupTitle(g issueGroup, title string) string {
	if g.Field == "due" && g.Value != "" {
		return a.shortDate(g.Value)
	}
	return title
}
func (a *app) listView(c *ui.Context, items []core.Issue) {
	rows := a.issueRows(items)
	if a.revealIssue {
		for n, row := range rows {
			if row.issue.ID == a.focusedIssue {
				a.list.ScrollIntoView(n)
				a.revealIssue = false
				break
			}
		}
	}
	a.list.Key = func(n int) any { return rows[n].key }
	a.list.Header = func(n int) bool { return rows[n].header != "" }
	a.forgetMarqueeRows(rows)
	a.marqueeFrame(c, rows)
	ui.List(c.Key("issues-list"), &a.list, len(rows), func(n int) {
		r := rows[n]
		if r.header != "" {
			closed := a.collapsed[r.header]
			// A group's title sits on the tile, over the white card of its rows.
			head := ui.Row(c).Height(40).Padding(4, 18, 0, 10+float32(r.depth)*18).Gap(8).Bind(a.marqueeRow(r.key))
			hovered := head.Hovered()
			head.Children(func() {
				// The whole title folds the group, a larger target than the caret alone.
				toggle := ui.ButtonBase(c).Label(a.tr("折叠或展开分组 ", "Toggle group ")+r.title).Height(28).Padding(0, 10, 0, 6).Gap(8).Radius(999).Cursor(ui.CursorPointer).Bind(a.marqueeControl("toggle:" + r.key))
				if toggle.Hovered() {
					toggle.Background(a.tk.FillTertiary)
				}
				toggle.Children(func() {
					turn := float32(90)
					if closed {
						turn = 0
					}
					ui.Icon(c, lucide.ChevronRight()).FontSize(14).Rotate(turn).TextColor(a.tk.TextQuaternary)
					a.groupMark(c, r.group)
					ui.Text(c, a.groupTitle(r.group, r.title)).FontWeight(600).FontSize(13).SingleLine()
					ui.Text(c, fmt.Sprint(r.count)).TextColor(a.tk.TextTertiary).FontSize(12)
				}).OnClick(func() { a.collapsed[r.header] = !a.collapsed[r.header] })
				ui.Spacer(c)
				if a.page != "trash" {
					reveal(iconButton(c, "plus", a.tr("添加任务到 ", "Add issue to ")+r.title).Size(24, 24), hovered).Bind(a.marqueeControl("add:" + r.key)).OnClick(func() { a.newGroupedTask(r.parentGroup, r.group) })
				}
			})
			return
		}
		// Consecutive rows make one white card: the first rounds its top, the
		// last its bottom, and hairlines part the ones between.
		first, last := n == 0 || rows[n-1].header != "", n == len(rows)-1 || rows[n+1].header != ""
		gap := float32(0)
		if last {
			gap = 10
		}
		ui.Column(c.Key(r.key)).Padding(0, 12, gap, 12+float32(r.depth)*18).Bind(a.marqueeRow(r.key)).Children(func() {
			top, bottom := float32(0), float32(0)
			if first {
				top = 16
			}
			if last {
				bottom = 16
			}
			card := ui.Column(c).Radius(top, top, bottom, bottom).Background(a.tk.BgContainer).Clip().Bind(a.marqueeControl("row:" + r.key))
			if !last {
				card.BorderWidth(0, 0, 1, 0).BorderColor(a.hairline())
			}
			card.Children(func() { a.issueRowView(c, r.issue, false) })
		})
	}).Grow(1).MinHeight(0).HandleInput(func(ev ui.InputEvent) bool { return a.marqueeInput(c, rows, ev) }).DrawOver(a.paintMarquee(c, rows))
}

// childProgress counts each issue's sub-issues and how many are finished. It is
// derived once per data revision; rows only look their own entry up.
func (a *app) childProgress(id string) (done, total int) {
	if a.childStats == nil || a.childStatsRevision != a.dataRevision {
		a.childStats, a.childStatsRevision = map[string][2]int{}, a.dataRevision
		for _, i := range a.data.Issues {
			if i.ParentID == "" || i.DeletedAt != 0 {
				continue
			}
			v := a.childStats[i.ParentID]
			if i.Status == "done" || i.Status == "canceled" || i.Status == "duplicate" {
				v[0]++
			}
			v[1]++
			a.childStats[i.ParentID] = v
		}
	}
	v := a.childStats[id]
	return v[0], v[1]
}
func (a *app) issueRowView(c *ui.Context, i core.Issue, card bool) {
	trash := a.page == "trash"
	b := ui.ButtonBase(c.Key(i.ID)).Role(ui.RoleGroup).Label(i.Code() + " " + i.Title).FillWidth()
	a.hoverFocus(c, b, i.ID)
	focused, selected := a.focusedIssue == i.ID, a.selected[i.ID]
	hot := focused || b.Hovered() // row controls show for the row in use
	keyboard := focused && a.hoverHold && a.issueID == ""
	if card {
		cardHeight := float32(104)
		if a.showsProperty("project") || a.showsProperty("created") || a.showsProperty("updated") {
			cardHeight = 132
		}
		a.lift(b.Height(cardHeight).Padding(12, 14).Radius(16).Background(a.tk.BgContainer).Border(1, ui.Transparent))
		switch {
		case selected:
			b.Border(1, a.tk.Primary)
		case keyboard:
			b.Border(1, a.tk.Info)
		case hot:
			b.Border(1, a.tk.FillBorderStrong())
		}
	} else {
		b.Radius(0).Height(42).Padding(0, 12, 0, 10).Gap(8).BorderWidth(0, 0, 0, 2).BorderColor(ui.Transparent)
		switch {
		case selected:
			a.selectedFill(b)
		case hot:
			b.Background(a.rowHover())
		}
		if keyboard {
			b.BorderColor(a.tk.Info)
		}
	}
	if !trash {
		b.Drag(issueDrag{ID: i.ID, IDs: a.dragIDs(i.ID)})
	}
	mods := b.ClickModifiers()
	b.OnClick(func() { a.rowClick(i.ID, mods) })
	priorityKey, statusKey, labelKey := ui.Key(0), ui.Key(0), ui.Key(0)
	if focused {
		priorityKey, statusKey, labelKey = ui.KeyP, ui.KeyS, ui.KeyL
	}
	setPriority := func(p int) {
		a.apply(func(d *core.Data) error { v := d.Issues[i.ID]; v.Priority = p; return d.PutIssue(v) }, nil)
	}
	setLabels := func(ids []string) {
		a.apply(func(d *core.Data) error { v := d.Issues[i.ID]; v.LabelIDs = ids; return d.PutIssue(v) }, nil)
	}
	setDue := func(value string) {
		a.apply(func(d *core.Data) error { v := d.Issues[i.ID]; v.Due = value; return d.PutIssue(v) }, nil)
	}
	subIssues := func() {
		if done, total := a.childProgress(i.ID); total > 0 {
			ui.Row(c).Gap(4).Shrink(0).TextColor(a.tk.TextTertiary).Children(func() {
				ui.Icon(c, lucide.CornerDownRight()).FontSize(12)
				ui.Text(c, fmt.Sprintf("%d/%d", done, total)).FontSize(12)
			})
		}
	}
	b.Children(func() {
		if card {
			ui.Column(c).Fill().Gap(6).Children(func() {
				ui.Row(c).Height(18).Gap(6).Children(func() {
					if a.showsProperty("id") {
						ui.Text(c, i.Code()).FontSize(12).TextColor(a.tk.TextTertiary)
					}
					ui.Spacer(c)
					if i.Due != "" && a.showsProperty("due") {
						ui.Text(c, a.shortDate(i.Due)).FontSize(12).TextColor(a.dueColor(i.Due, i.Status))
					}
				})
				ui.Row(c).Grow(1).Gap(6).AlignItems(ui.Start).Children(func() {
					if a.showsProperty("status") {
						a.statusControl(c.Key("card-status"), i.Status, true, a.tr("状态 ", "Status ")+i.Code(), statusKey, func(v string) { a.setStatus(i.ID, v) }).Margin(-2, 0, 0, -4)
					}
					ui.Text(c, i.Title).MaxLines(2).Grow(1).MinWidth(0)
				})
				ui.Row(c).Height(24).Gap(4).Children(func() {
					if a.showsProperty("priority") {
						a.priorityControl(c.Key("card-priority"), i.Priority, true, a.tr("优先级 ", "Priority ")+i.Code(), priorityKey, setPriority).Margin(0, 0, 0, -4)
					}
					if a.showsProperty("labels") {
						a.labelStrip(c.Key("card-labels"), i.LabelIDs, labelKey, setLabels, i.ProjectID, 2).MaxWidth(180)
					}
					ui.Spacer(c)
					subIssues()
				})
				if a.showsProperty("project") || a.showsProperty("created") || a.showsProperty("updated") {
					ui.Row(c).Height(20).Gap(6).TextColor(a.tk.TextTertiary).Children(func() {
						if p, ok := a.data.Projects[i.ProjectID]; ok && a.showsProperty("project") {
							ui.Box(c).Size(7, 7).Radius(3).Background(ui.Hex(p.Color))
							ui.Text(c, p.Name).SingleLine().Grow(1).MinWidth(0).FontSize(12)
						} else {
							ui.Spacer(c)
						}
						if a.showsProperty("created") {
							ui.Text(c, a.shortTime(i.CreatedAt)).FontSize(12)
						}
						if a.showsProperty("updated") {
							ui.Text(c, a.shortTime(i.UpdatedAt)).FontSize(12)
						}
					})
				}
			})
			return
		}
		box := check(c, selected, "", 14, func() { a.toggleSelected(i.ID) }).Label(a.tr("选择 ", "Select ")+i.Code()).Size(20, 24).Center()
		reveal(box, hot || selected || len(a.selected) > 0)
		if a.showsProperty("priority") {
			a.priorityControl(c.Key("row-priority"), i.Priority, true, a.tr("优先级 ", "Priority ")+i.Code(), priorityKey, setPriority).Disabled(trash)
		}
		if a.showsProperty("id") {
			ui.Text(c, i.Code()).Width(60).FontSize(12).SingleLine().TextColor(a.tk.TextTertiary)
		}
		if a.showsProperty("status") {
			a.statusControl(c.Key("row-status"), i.Status, true, a.tr("状态 ", "Status ")+i.Code(), statusKey, func(v string) { a.setStatus(i.ID, v) }).Disabled(trash)
		}
		if i.ParentID != "" {
			ui.Icon(c, lucide.CornerDownRight()).FontSize(13).TextColor(a.tk.TextQuaternary)
		}
		ui.Text(c, i.Title).Grow(1).MinWidth(0).SingleLine()
		subIssues()
		wide := a.viewConfig.Layout != "split"
		if a.showsProperty("labels") && wide {
			a.labelStrip(c.Key("row-labels"), i.LabelIDs, labelKey, setLabels, i.ProjectID, 2).MaxWidth(260).Disabled(trash)
		}
		if a.showsProperty("project") {
			a.projectPicker(c.Key("row-project"), i.ProjectID, func(value string) {
				a.apply(func(d *core.Data) error { v := d.Issues[i.ID]; v.ProjectID = value; return d.PutIssue(v) }, nil)
			}).Height(24).MaxWidth(150).Disabled(trash)
		}
		if i.Due != "" && a.showsProperty("due") && wide {
			a.dueChip(c.Key("row-due"), i.Due, i.Status, a.tr("截止日期 ", "Due date ")+i.Code(), setDue).Disabled(trash)
		}
		if a.showsProperty("created") {
			ui.Text(c, a.shortTime(i.CreatedAt)).FontSize(12).TextColor(a.tk.TextTertiary)
		}
		if a.showsProperty("updated") {
			ui.Text(c, a.shortTime(i.UpdatedAt)).FontSize(12).TextColor(a.tk.TextTertiary)
		}
		if trash {
			textButton(c, a.tr("恢复", "Restore")).OnClick(func() { a.apply(func(d *core.Data) error { return d.RestoreIssue(i.ID) }, nil) })
			reveal(iconButton(c, "trash", a.tr("永久删除 ", "Delete forever: ")+i.Code()).Size(24, 24), hot).OnClick(func() { a.askErase(eraseRequest{issues: []string{i.ID}}) })
		} else {
			reveal(iconButton(c, "more", a.tr("任务操作 ", "Issue actions ")+i.Code()).Size(24, 24), hot).PopoverMenuButton(func(m *ui.Menu) { a.issueMenu(m, i) })
		}
	})
	if trash {
		b.ContextMenu(func(m *ui.Menu) { a.trashMenu(m, i) })
	} else {
		b.ContextMenu(func(m *ui.Menu) { a.issueMenu(m, i) })
		if dragged, ok := ui.Drop[issueDrag](b); ok && dragged.ID != i.ID {
			a.dropIssues(dragged.IDs, issueGroup{Field: "status", Value: i.Status}, i.ID)
		}
	}
	a.peekCard(c, b, i)
}
func (a *app) issueMenu(m *ui.Menu, i core.Issue) {
	if m.Item(a.tr("打开任务", "Open issue")).Chosen() {
		a.openIssue(i.ID)
	}
	m.Submenu(a.tr("状态", "Status"), func(m *ui.Menu) {
		for _, s := range core.Statuses {
			if m.Item(a.status(s)).Radio(s == i.Status).Chosen() {
				a.setStatus(i.ID, s)
			}
		}
	})
	m.Submenu(a.tr("优先级", "Priority"), func(m *ui.Menu) {
		for p := 0; p <= 4; p++ {
			if m.Item(a.priority(p)).Radio(i.Priority == p).Chosen() {
				a.apply(func(d *core.Data) error { v := d.Issues[i.ID]; v.Priority = p; return d.PutIssue(v) }, nil)
			}
		}
	})
	if m.Item(a.tr("收藏", "Favorite")).Checked(i.Favorite).Chosen() {
		a.apply(func(d *core.Data) error { v := d.Issues[i.ID]; v.Favorite = !v.Favorite; return d.PutIssue(v) }, nil)
	}
	m.Separator()
	if m.Item(a.tr("添加子任务", "Add sub-issue")).Chosen() {
		a.newTask(i.ID, "backlog")
	}
	if m.Item(a.tr("添加链接…", "Add link…")).Chosen() {
		a.addLink("issue", i.ID)
	}
	if m.Item(a.tr("添加文件…", "Attach files…")).Chosen() {
		a.chooseAttachments("issue", i.ID)
	}
	a.extendedIssueMenu(m, i)
	m.Submenu(a.tr("复制", "Copy"), func(m *ui.Menu) {
		for _, action := range [][2]string{{"title", a.tr("复制任务标题", "Copy issue title")}, {"markdown", a.tr("复制为 Markdown", "Copy as Markdown")}, {"id", a.tr("复制任务编号", "Copy issue ID")}, {"url", a.tr("复制任务链接", "Copy issue URL")}, {"branch", a.tr("复制分支名", "Copy branch name")}, {"prompt", a.tr("复制为提示词", "Copy as prompt")}} {
			if m.Item(action[1]).Chosen() {
				a.copyIssue(i, action[0])
			}
		}
	})
	m.Separator()
	if m.Item(a.tr("移至回收站", "Move to trash")).Danger().Chosen() {
		a.trashIssue(i.ID)
	}
}
func (a *app) boardView(c *ui.Context, items []core.Issue) {
	ui.ScrollHorizontal(c).Grow(1).MinHeight(0).Padding(0, 12, 12, 12).Children(func() {
		ui.Row(c).FillHeight().Gap(8).AlignItems(ui.Stretch).Children(func() {
			for _, g := range a.grouped(items, a.viewConfig.GroupBy, a.viewConfig.EmptyGroups) {
				if a.statusFilter != "" && g.Field == "status" && g.Value != a.statusFilter {
					continue
				}
				if !a.viewConfig.EmptyGroups && len(g.Items) == 0 {
					continue
				}
				col := ui.Column(c.Key(g.Key)).Width(280).FillHeight().Padding(4, 6, 8, 6).Gap(6).Radius(20).Background(theme.Mix(a.tile(), a.tk.Text, .025)).Border(1, ui.Transparent).Label(a.tr("看板列 ", "Board column ") + g.Title)
				if _, over := ui.DragOver[issueDrag](col); over {
					col.Border(1, a.tk.Info)
				}
				if drag, ok := ui.Drop[issueDrag](col); ok {
					a.dropIssues(drag.IDs, g, "")
				}
				hovered := col.Hovered()
				col.Children(func() {
					ui.Row(c).Height(36).Padding(0, 0, 0, 4).Gap(8).Children(func() {
						a.groupMark(c, g)
						ui.Text(c, a.groupTitle(g, g.Title)).FontWeight(500).FontSize(13).SingleLine()
						ui.Text(c, fmt.Sprint(len(g.Items))).TextColor(a.tk.TextQuaternary).FontSize(12)
						ui.Spacer(c)
						reveal(iconButton(c, "plus", a.tr("添加任务到 ", "Add issue to ")+g.Title).Size(24, 24), hovered).OnClick(func() {
							a.newGroupedTask(g)
						})
					})
					rows := []issueRow{}
					if a.viewConfig.Subgroup != "none" && a.viewConfig.Subgroup != "" {
						for _, sub := range a.grouped(g.Items, a.viewConfig.Subgroup, a.viewConfig.EmptyGroups) {
							rows = append(rows, issueRow{header: sub.Key, title: sub.Title, key: sub.Key, group: sub})
							for _, i := range sub.Items {
								rows = append(rows, issueRow{issue: i, key: sub.Key + "/" + i.ID})
							}
						}
					} else {
						for _, i := range g.Items {
							rows = append(rows, issueRow{issue: i, key: i.ID})
						}
					}
					state := a.boardStates[g.Key]
					if state == nil {
						state = &ui.ListState{}
						a.boardStates[g.Key] = state
					}
					state.Key = func(n int) any { return rows[n].key }
					if a.revealIssue {
						for n, r := range rows {
							if r.issue.ID == a.focusedIssue {
								state.ScrollIntoView(n)
								col.ScrollIntoView()
								a.revealIssue = false
								break
							}
						}
					}
					ui.List(c, state, len(rows), func(n int) {
						r := rows[n]
						if r.header != "" {
							ui.Row(c).Height(26).Padding(0, 4).Gap(6).Children(func() {
								a.groupMark(c, r.group)
								ui.Text(c, a.groupTitle(r.group, r.title)).FontSize(12).TextColor(a.tk.TextTertiary).SingleLine()
							})
						} else {
							a.issueRowView(c, r.issue, true)
						}
					}).Grow(1).Gap(6)
				})
			}
		})
	})
}

// batchBar edits every selected issue at once. It reports whether it showed.
func (a *app) batchBar(c *ui.Context) bool {
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
	// As tall as the toolbar it replaces, so the list does not move under
	// a box being dragged.
	ui.Row(c).Height(56).Shrink(0).Padding(0, 14).Gap(4).Children(func() {
		ui.Text(c, fmt.Sprintf(a.tr("已选 %d 项", "%d selected"), len(ids))).FontWeight(500).Margin(0, 8, 0, 4)
		textButton(c, a.tr("状态", "Status")).Label(a.tr("修改状态", "Set status")).PopoverMenuButton(func(m *ui.Menu) {
			for _, s := range core.Statuses {
				if m.Item(a.status(s)).Chosen() {
					a.apply(func(d *core.Data) error {
						for _, id := range ids {
							i := d.Issues[id]
							i.Status = s
							if e := d.PutIssue(i); e != nil {
								return e
							}
						}
						return nil
					}, clear)
				}
			}
		})
		textButton(c, a.tr("优先级", "Priority")).Label(a.tr("修改优先级", "Set priority")).PopoverMenuButton(func(m *ui.Menu) {
			for p := 0; p <= 4; p++ {
				if m.Item(a.priority(p)).Chosen() {
					a.apply(func(d *core.Data) error {
						for _, id := range ids {
							i := d.Issues[id]
							i.Priority = p
							if e := d.PutIssue(i); e != nil {
								return e
							}
						}
						return nil
					}, clear)
				}
			}
		})
		a.batchPropertyControls(c, ids)
		ui.Spacer(c)
		iconButton(c, "trash", a.tr("移至回收站", "Move to trash")).OnClick(func() {
			a.apply(func(d *core.Data) error {
				for _, id := range ids {
					d.TrashIssue(id)
				}
				return nil
			}, clear)
		})
		hotIconButton(c, "close", a.tr("取消选择", "Clear selection"), "esc").OnClick(clear)
	})
	return true
}
func (a *app) issueView(c *ui.Context) {
	a.issueViewFor(c, a.issueID)
}

// detailWide reports whether the detail has room for a properties column
// beside the text, as Linear lays an issue out.
func (a *app) detailWide(c *ui.Context) bool {
	w, _ := c.Size()
	if !a.navigation.SidebarCollapsed {
		w -= a.sidebarWidth
	}
	if a.viewConfig.Layout == "split" && a.page != "trash" {
		w -= 400
	}
	return w >= 980
}

// issueProperties builds the editable properties of an issue: as chips in a
// wrapping row, or as full-width rows of the properties column.
func (a *app) issueProperties(c *ui.Context, d *draft, column bool) {
	i := &d.issue
	place := func(e ui.Element) {
		if column {
			e.Border(1, ui.Transparent).FillWidth().Height(36).Padding(0, 10).Radius(12).Justify(ui.Start)
		}
	}
	place(a.statusPicker(c, i.Status, func(v string) { i.Status = v; a.touch(d, c.Now()) }))
	place(a.priorityPicker(c, i.Priority, func(v int) { i.Priority = v; a.touch(d, c.Now()) }))
	place(a.projectPicker(c, i.ProjectID, func(v string) { i.ProjectID = v; a.touch(d, c.Now()) }).Disabled(i.ParentID != ""))
	place(a.labelPicker(c, i.LabelIDs, func(v []string) { i.LabelIDs = v; a.touch(d, c.Now()) }, i.ProjectID).MaxWidth(240))
	place(a.milestonePicker(c, i.ProjectID, i.MilestoneID, func(v string) { i.MilestoneID = v; a.touch(d, c.Now()) }))
	place(a.rangeButton(c, i.Start, i.Due, a.tr("截止日期", "Due date"), func(start, end string) {
		if end == "" {
			start, end = "", start
		}
		i.Start, i.Due = start, end
		a.touch(d, c.Now())
	}))
}
func (a *app) issueViewFor(c *ui.Context, issue string) {
	d := a.drafts[issue]
	if d == nil {
		return
	}
	i := &d.issue
	id := i.ID
	if a.data.Issues[id].DeletedAt != 0 {
		ui.Scroll(c.Key("trash-detail-"+id)).Grow(1).Padding(24, 40).Children(func() {
			ui.Column(c).FillWidth().MaxWidth(760).Margin(0, ui.Auto).Gap(16).Children(func() {
				ui.Row(c).Padding(8, 8, 8, 12).Gap(10).Radius(a.tk.BorderRadius).Background(a.tk.FillTertiary).Children(func() {
					ui.Icon(c, lucide.Trash2()).FontSize(16).TextColor(a.tk.TextTertiary)
					ui.Text(c, a.tr("已在回收站", "In Trash")).TextColor(a.tk.TextSecondary).Grow(1)
					btn(c, a.tr("恢复", "Restore issue")).OnClick(func() {
						a.apply(func(data *core.Data) error { return data.RestoreIssue(id) }, func() { a.openIssue(id) })
					})
					dangerButton(c, a.tr("永久删除", "Delete forever")).Label(a.tr("永久删除任务", "Delete issue forever")).OnClick(func() { a.askErase(eraseRequest{issues: []string{id}}) })
				})
				ui.Text(c, i.Title).FontSize(a.tk.FontSizeHeading3).FontWeight(600).Selectable()
				markdown.View(c, d.doc.Text().Markdown(), a.mediaMarkdownOptions()).FontSize(15)
			})
		})
		return
	}
	wide := a.detailWide(c)
	ui.Row(c.Key("detail-layout-" + id)).Grow(1).MinHeight(0).Children(func() {
		// The issue is a white card on the tile, and its properties a second
		// one beside it.
		ui.Column(c.Key("detail-main")).Grow(1).MinWidth(0).FillHeight().Children(func() {
			ui.Scroll(c.Key("detail-"+id)).Grow(1).MinHeight(0).Padding(4, 14, 24, 14).Children(func() {
				a.card(ui.Column(c.Key("detail-body"))).FillWidth().MaxWidth(840).Margin(0, ui.Auto).Padding(28, 40, 36, 40).Gap(12).Children(func() { a.issueBody(c, d, wide) })
			})
		})
		if wide {
			ui.Column(c).Width(272).Shrink(0).FillHeight().Padding(4, 14, 14, 0).Children(func() {
				a.card(ui.Column(c.Key("detail-properties"))).Label(a.tr("任务属性", "Issue properties")).FillWidth().Padding(10, 8).Gap(2).Children(func() {
					a.issueProperties(c, d, true)
				})
			})
		}
	})
}

// issueBody is the reading column of an issue: its title and description, then
// only the records it actually has, then the conversation.
func (a *app) issueBody(c *ui.Context, d *draft, wide bool) {
	i := &d.issue
	id := i.ID
	title := ui.TextInput(c.Key("issue-title"), &i.Title).Bind(&d.titleEditor).Label(a.tr("任务标题", "Issue title")).Placeholder(a.tr("标题", "Title")).FontSize(a.tk.FontSizeHeading3).FontWeight(600).Height(38).Background(ui.Transparent).Border(0, ui.Transparent).Padding(0).FocusRing(false).OnChange(func() { a.touch(d, c.Now()) })
	if d.wantTitle {
		d.titleEditor.Focus(c)
		title.ScrollIntoView()
		d.wantTitle = false
	}
	if !wide {
		ui.Row(c).Wrap().Gap(6).Children(func() { a.issueProperties(c, d, false) })
	}
	if parent, ok := a.data.Issues[i.ParentID]; ok {
		ui.Row(c).Children(func() {
			actionButton(c, "subissue", parent.Code()+"  "+parent.Title).Label(a.tr("打开父任务 ", "Open parent ") + parent.Code()).MaxWidth(420).OnClick(func() { a.openIssue(parent.ID) })
			ui.Spacer(c)
		})
	}
	edit := func() { d.editing = true; d.editor.Focus() }
	if d.editing {
		if d.wantDescription {
			d.editor.Focus(c)
			d.wantDescription = false
		}
		a.richEditor(c, "issue-body", a.tr("任务说明", "Issue description"), a.tr("添加说明…", "Add a description…"), d.doc, &d.editor, 240, true, func() { a.touch(d, c.Now()) }, func() { a.chooseEditorMedia(editorMediaTarget{kind: "issue", id: id}, d.doc) }, editorMediaTarget{kind: "issue", id: id})
		ui.Row(c).Justify(ui.End).Children(func() {
			done := action.Button(c, a.tr("完成", "Done editing"), action.ButtonProps{Size: theme.Small}).Radius(999).PaddingX(12).Label(a.tr("完成编辑", "Done editing")).OnClick(func() { d.editing = false; a.saveDraft(id) })
			overlayTip(c, done, a.tr("完成编辑", "Done editing"), "esc")
		})
	} else {
		block := ui.Row(c).Gap(8).AlignItems(ui.Start)
		hovered := block.Hovered()
		block.Children(func() {
			ui.Column(c).Grow(1).MinWidth(0).MinHeight(28).Children(func() {
				if d.doc.Text().PlainText() == "" {
					ui.ButtonBase(c).Height(28).Justify(ui.Start).Cursor(ui.CursorText).TextColor(a.tk.TextPlaceholder).Children(func() {
						ui.Text(c, a.tr("添加说明…", "Add a description…"))
					}).OnClick(edit)
				} else {
					markdown.View(c.Key("description-preview"), d.doc.Text().Markdown(), a.mediaMarkdownOptions()).FontSize(15).OnClick(edit)
				}
			})
			reveal(iconButton(c, "pencil", a.tr("编辑说明", "Edit description")), hovered).OnClick(edit)
		})
	}

	ui.Row(c).Gap(2).Margin(0, 0, 0, -8).Children(func() {
		actionButton(c, "plus", a.tr("子任务", "Sub-issue")).Label(a.tr("＋ 添加子任务", "＋ Add sub-issue")).OnClick(func() { a.newTask(id, "backlog") })
		iconButton(c, "paperclip", a.tr("添加附件", "Attach")).Size(24, 24).PopoverMenuButton(func(m *ui.Menu) {
			if m.Item(a.tr("链接…", "Link…")).Chosen() {
				a.addLink("issue", id)
			}
			if m.Item(a.tr("文件…", "Files…")).Chosen() {
				a.chooseAttachments("issue", id)
			}
		})
	})
	if children := a.childIssues(id); len(children) > 0 {
		finished := 0
		for _, child := range children {
			if child.Status == "done" || child.Status == "canceled" || child.Status == "duplicate" {
				finished++
			}
		}
		ui.Column(c.Key("sub-issues")).Gap(2).Children(func() {
			a.sectionHeader(c, a.tr("子任务", "Sub-issues"), fmt.Sprintf("%d/%d", finished, len(children)), nil)
			for _, ch := range children {
				row := ui.Row(c.Key(ch.ID)).Height(36).Padding(0, 8, 0, 2).Gap(6).Radius(a.tk.BorderRadiusSM)
				if row.Hovered() {
					row.Background(a.rowHover())
				}
				row.Children(func() {
					a.statusControl(c.Key("status"), ch.Status, true, a.tr("状态 ", "Status ")+ch.Code(), 0, func(v string) { a.setStatus(ch.ID, v) })
					open := ui.ButtonBase(c.Key("open")).Grow(1).MinWidth(0).Height(28).Gap(10).Justify(ui.Start).Cursor(ui.CursorPointer)
					open.Children(func() {
						ui.Text(c, ch.Code()).FontSize(12).TextColor(a.tk.TextTertiary)
						ui.Text(c, ch.Title).Grow(1).MinWidth(0).SingleLine()
					}).OnClick(func() { a.openIssue(ch.ID) })
					priorityMark(c, ch.Priority)
				})
			}
		})
	}
	a.attachmentSection(c, "issue", id)
	a.documentsSection(c, "issue", id)
	a.relationshipSection(c, id)
	a.activitySection(c, "issue", id)
}
