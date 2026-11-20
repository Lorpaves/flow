package main

import (
	"flow/internal/core"
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/choice"
	"github.com/lobehub/lobe-mygo/pkg/field"
)

func (a *app) viewKey() string { return a.page + "/" + a.projectID }
func (a *app) loadViewConfig() {
	a.viewConfig = core.DefaultView()
	if v, ok := a.data.Preferences.ViewStates[a.viewKey()]; ok {
		a.viewConfig = v.Clone()
	}
	a.board = a.viewConfig.Layout == "board"
}
func (a *app) persistViewConfig() {
	key, config := a.viewKey(), a.viewConfig.Clone()
	a.board = config.Layout == "board"
	a.apply(func(d *core.Data) error {
		if d.Preferences.ViewStates == nil {
			d.Preferences.ViewStates = map[string]core.ViewConfig{}
		}
		d.Preferences.ViewStates[key] = config
		return nil
	}, nil)
}
func (a *app) setLayout(layout string) { a.viewConfig.Layout = layout; a.persistViewConfig() }
func (a *app) openSavedView(id string) {
	v, ok := a.data.Views[id]
	if !ok {
		return
	}
	a.navigate("view", v.ProjectID)
	a.savedViewID = id
	a.viewConfig = v.Config.Clone()
	a.query = v.Query
	a.board = v.Config.Layout == "board"
}
func (a *app) saveCurrentView() {
	name := strings.TrimSpace(a.savedViewName)
	if name == "" {
		a.formError = "title_required"
		return
	}
	id := a.savedViewID
	if id == "" {
		id = core.ID()
	}
	config := a.viewConfig.Clone()
	for _, r := range []core.FilterRule{{Field: "status", Op: "is", Values: []string{a.statusFilter}}, {Field: "labels", Op: "is", Values: []string{a.labelFilter}}} {
		if r.Values[0] != "" {
			config.Filter = core.FilterGroup{Rules: []core.FilterRule{r}, Groups: []core.FilterGroup{config.Filter}}
		}
	}
	if a.priorityFilter >= 0 {
		config.Filter = core.FilterGroup{Rules: []core.FilterRule{{Field: "priority", Op: "is", Values: []string{strconv.Itoa(a.priorityFilter)}}}, Groups: []core.FilterGroup{config.Filter}}
	}
	v := core.SavedView{ID: id, Name: name, ProjectID: a.projectID, Query: a.query, Config: config}
	a.apply(func(d *core.Data) error {
		if d.Views == nil {
			d.Views = map[string]core.SavedView{}
		}
		d.Views[id] = v
		return nil
	}, func() { a.saveViewOpen = false; a.openSavedView(id) })
}
func (a *app) configSelect(c *ui.Context, label string, value string, options [][2]string, set func(string)) {
	ui.Row(c).MinHeight(36).Gap(12).Children(func() {
		ui.Text(c, label).Grow(1).TextColor(a.tk.TextSecondary)
		title := value
		items := []pickerItem{}
		for _, o := range options {
			if o[0] == value {
				title = o[1]
			}
			items = append(items, pickerItem{id: o[0], label: o[1], checked: o[0] == value})
		}
		b := a.chip(c, title, nil).Label(a.tr("更改", "Change ") + label).Width(200).Justify(ui.SpaceBetween)
		a.picker(c, b, label, "", 0, items, false, set)
	})
}
func (a *app) groupOptions() [][2]string {
	return [][2]string{{"none", a.tr("不分组", "No grouping")}, {"status", a.tr("状态", "Status")}, {"priority", a.tr("优先级", "Priority")}, {"project", a.tr("项目", "Project")}, {"labels", a.tr("标签", "Labels")}, {"due", a.tr("截止日期", "Due date")}}
}
func (a *app) viewDialogs(c *ui.Context) {
	ui.DialogBase(c, &a.displayOpen, func(backdrop, panel ui.Element) {
		a.dialogChrome(backdrop, panel)
		panel.Width(440).Padding(16).Gap(6)
		a.dialogTitle(c, a.tr("显示", "Display"), func() { a.displayOpen = false })
		a.configSelect(c, a.tr("布局", "Layout"), a.viewDraft.Layout, [][2]string{{"list", a.tr("列表", "List")}, {"board", a.tr("看板", "Board")}, {"split", a.tr("列表与详情", "List and detail")}}, func(v string) { a.viewDraft.Layout = v })
		a.configSelect(c, a.tr("分组", "Grouping"), a.viewDraft.GroupBy, a.groupOptions(), func(v string) { a.viewDraft.GroupBy = v })
		a.configSelect(c, a.tr("次级分组", "Sub-grouping"), a.viewDraft.Subgroup, a.groupOptions(), func(v string) { a.viewDraft.Subgroup = v })
		a.configSelect(c, a.tr("排序", "Ordering"), a.viewDraft.OrderBy, [][2]string{{"manual", a.tr("手动", "Manual")}, {"priority", a.tr("优先级", "Priority")}, {"title", a.tr("标题", "Title")}, {"due", a.tr("截止日期", "Due date")}, {"created", a.tr("创建时间", "Created")}, {"updated", a.tr("更新时间", "Updated")}}, func(v string) { a.viewDraft.OrderBy = v })
		a.configSelect(c, a.tr("已完成", "Completed issues"), a.viewDraft.Completed, [][2]string{{"all", a.tr("全部", "All")}, {"hide", a.tr("隐藏", "Hide")}, {"week", a.tr("最近一周", "Past week")}, {"month", a.tr("最近一个月", "Past month")}}, func(v string) { a.viewDraft.Completed = v })
		a.configSelect(c, a.tr("子任务", "Sub-issues"), a.viewDraft.SubIssues, [][2]string{{"all", a.tr("独立显示", "Show all")}, {"hide", a.tr("隐藏", "Hide")}, {"nested", a.tr("嵌套显示", "Nest matching issues")}}, func(v string) { a.viewDraft.SubIssues = v })
		ui.Row(c).MinHeight(36).Gap(20).Children(func() {
			choice.Checkbox(c, &a.viewDraft.Descending, a.tr("倒序", "Descending"), choice.CheckboxProps{})
			choice.Checkbox(c, &a.viewDraft.EmptyGroups, a.tr("看板显示空列", "Show empty columns"), choice.CheckboxProps{})
		})
		a.divider(c).Margin(4, 0)
		ui.Text(c, a.tr("显示字段", "Properties")).FontSize(12).TextColor(a.tk.TextTertiary).Margin(4, 0)
		ui.Row(c).Wrap().Gap(6).Children(func() {
			for _, p := range [][2]string{{"id", "ID"}, {"status", a.tr("状态", "Status")}, {"priority", a.tr("优先级", "Priority")}, {"project", a.tr("项目", "Project")}, {"labels", a.tr("标签", "Labels")}, {"due", a.tr("截止日期", "Due date")}, {"created", a.tr("创建时间", "Created")}, {"updated", a.tr("更新时间", "Updated")}} {
				on := slices.Contains(a.viewDraft.Properties, p[0])
				b := ui.ButtonBase(c.Key(p[0])).Label("Display "+p[0]).Height(26).Padding(0, 10).Radius(999).FontSize(12).Border(1, a.tk.FillBorder()).TextColor(a.tk.TextTertiary).Cursor(ui.CursorPointer)
				if on {
					b.Background(a.tk.FillSecondary).TextColor(a.tk.Text).Border(1, ui.Transparent)
				}
				b.Children(func() { ui.Text(c, p[1]) }).OnClick(func() {
					if on {
						a.viewDraft.Properties = slices.DeleteFunc(a.viewDraft.Properties, func(v string) bool { return v == p[0] })
					} else {
						a.viewDraft.Properties = append(a.viewDraft.Properties, p[0])
					}
				})
			}
		})
		dialogFooter(c, func() {
			btn(c, a.tr("取消", "Cancel")).OnClick(func() { a.displayOpen = false })
			primary(c, a.tr("应用", "Apply")).Label(a.tr("应用显示设置", "Apply display options")).OnClick(func() { a.viewConfig = a.viewDraft.Clone(); a.displayOpen = false; a.persistViewConfig() })
		})
	})
	ui.DialogBase(c, &a.filterOpen, func(backdrop, panel ui.Element) {
		a.dialogChrome(backdrop, panel)
		panel.Width(720).MaxHeight(660).Padding(16).Gap(12)
		a.dialogTitle(c, a.tr("高级筛选", "Advanced filter"), func() { a.filterOpen = false })
		ui.Scroll(c).MaxHeight(450).Gap(10).Children(func() { a.filterGroupEditor(c, &a.viewDraft.Filter, "root", 0) })
		if a.formError != "" {
			ui.Text(c, a.formError).FontSize(12).TextColor(a.tk.Error)
		}
		ui.Row(c).Gap(8).Children(func() {
			quietButton(c, a.tr("清除", "Clear conditions")).OnClick(func() { a.viewDraft.Filter = core.FilterGroup{} })
			ui.Spacer(c)
			btn(c, a.tr("取消", "Cancel")).OnClick(func() { a.filterOpen = false })
			primary(c, a.tr("应用", "Apply")).Label(a.tr("应用筛选", "Apply filters")).OnClick(func() {
				if e := a.viewDraft.Filter.Validate(0); e != nil {
					a.formError = e.Error()
					return
				}
				a.viewConfig.Filter = a.viewDraft.Filter.Clone()
				a.filterOpen = false
				a.persistViewConfig()
			})
		})
	})
	ui.DialogBase(c, &a.saveViewOpen, func(backdrop, panel ui.Element) {
		a.dialogChrome(backdrop, panel)
		panel.Width(400).Padding(16).Gap(12)
		a.dialogTitle(c, a.tr("保存视图", "Save view"), nil)
		field.Input(c, &a.savedViewName, field.InputProps{Placeholder: a.tr("名称", "Name")}).Label(a.tr("视图名称", "View name")).AutoFocus()
		if a.formError != "" {
			ui.Text(c, a.errorText(a.formError)).FontSize(12).TextColor(a.tk.Error)
		}
		dialogFooter(c, func() {
			btn(c, a.tr("取消", "Cancel")).OnClick(func() { a.saveViewOpen = false })
			primary(c, a.tr("保存", "Save")).Label(a.tr("确认保存视图", "Confirm save view")).OnClick(a.saveCurrentView)
		})
	})
}
func (a *app) filterGroupEditor(c *ui.Context, g *core.FilterGroup, key string, depth int) {
	ui.Column(c.Key(key)).Padding(8).Gap(6).Border(1, a.tk.BorderSecondary).Radius(a.tk.BorderRadius).Children(func() {
		choice.Checkbox(c, &g.Any, a.tr("满足任一条件", "Match any condition"), choice.CheckboxProps{})
		for n := range g.Rules {
			r := &g.Rules[n]
			if r.ID == "" {
				r.ID = core.ID()
			}
			ui.Row(c.Key(r.ID)).Gap(6).Children(func() {
				a.chip(c, a.filterFieldName(r.Field), nil).Label("Filter field " + key + strconv.Itoa(n)).Width(110).PopoverMenuButton(func(m *ui.Menu) {
					for _, f := range []string{"status", "priority", "labels", "project", "milestone", "title", "description", "due", "created", "updated", "parent", "favorite"} {
						if m.Item(a.filterFieldName(f)).Chosen() {
							r.Field = f
							r.Values = nil
						}
					}
				})
				a.chip(c, a.filterOperatorName(r.Op), nil).Label("Filter operator " + key + strconv.Itoa(n)).Width(110).PopoverMenuButton(func(m *ui.Menu) {
					for _, op := range []string{"is", "is_not", "contains", "not_contains", "all", "before", "after", "empty", "not_empty"} {
						if m.Item(a.filterOperatorName(op)).Chosen() {
							r.Op = op
						}
					}
				})
				a.filterValueControl(c, r, key+strconv.Itoa(n))
				iconButton(c, "close", a.tr("移除条件 ", "Remove condition ")+key+strconv.Itoa(n)).OnClick(func() { g.Rules = slices.Delete(g.Rules, n, n+1) })
			})
		}
		for n := range g.Groups {
			a.filterGroupEditor(c, &g.Groups[n], key+"/"+strconv.Itoa(n), depth+1)
			ui.Row(c.Key("remove-" + strconv.Itoa(n))).Children(func() {
				quietButton(c, a.tr("移除条件组", "Remove group")).OnClick(func() { g.Groups = slices.Delete(g.Groups, n, n+1) })
			})
		}
		ui.Row(c).Gap(4).Children(func() {
			actionButton(c, "plus", a.tr("条件", "Condition")).Label(a.tr("添加条件", "Add condition")).OnClick(func() {
				g.Rules = append(g.Rules, core.FilterRule{Field: "status", Op: "is", Values: []string{"todo"}})
			})
			if depth < 7 {
				actionButton(c, "plus", a.tr("条件组", "Group")).Label(a.tr("添加条件组", "Add group")).OnClick(func() { g.Groups = append(g.Groups, core.FilterGroup{}) })
			}
		})
	})
}
func (a *app) showsProperty(name string) bool { return slices.Contains(a.viewConfig.Properties, name) }
