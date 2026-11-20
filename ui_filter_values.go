package main

import (
	"flow/internal/core"
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/field"
	"github.com/lobehub/lobe-mygo/pkg/theme"
)

func (a *app) filterFieldName(field string) string {
	words := map[string][2]string{"status": {"状态", "Status"}, "priority": {"优先级", "Priority"}, "labels": {"标签", "Labels"}, "project": {"项目", "Project"}, "milestone": {"里程碑", "Milestone"}, "title": {"标题", "Title"}, "description": {"正文", "Description"}, "due": {"截止日期", "Due date"}, "created": {"创建日期", "Created"}, "updated": {"修改日期", "Updated"}, "parent": {"父任务", "Parent issue"}, "favorite": {"收藏", "Favorite"}}
	v := words[field]
	return a.tr(v[0], v[1])
}
func (a *app) filterOperatorName(op string) string {
	words := map[string][2]string{"is": {"是其中之一", "Is any of"}, "is_not": {"不是", "Is not"}, "contains": {"包含", "Contains"}, "not_contains": {"不包含", "Does not contain"}, "all": {"包含全部", "Includes all"}, "before": {"早于", "Before"}, "after": {"晚于", "After"}, "empty": {"未设置", "Is empty"}, "not_empty": {"已设置", "Is set"}}
	v := words[op]
	return a.tr(v[0], v[1])
}
func (a *app) filterValueControl(c *ui.Context, r *core.FilterRule, key string) {
	if r.Op == "empty" || r.Op == "not_empty" {
		ui.Spacer(c)
		return
	}
	items := []pickerItem{}
	add := func(id, label string) {
		items = append(items, pickerItem{id: id, label: label, checked: slices.Contains(r.Values, id)})
	}
	entity := true
	switch r.Field {
	case "status":
		for _, s := range core.Statuses {
			add(s, a.status(s))
		}
	case "priority":
		for p := 0; p <= 4; p++ {
			add(strconv.Itoa(p), a.priority(p))
		}
	case "labels":
		for _, l := range a.data.LabelList() {
			add(l.ID, l.Name)
		}
	case "project":
		for _, p := range a.data.ProjectList(false, false) {
			add(p.ID, p.Name)
		}
	case "milestone":
		for _, p := range a.data.ProjectList(false, false) {
			for _, m := range a.data.MilestoneList(p.ID) {
				add(m.ID, p.Name+" / "+m.Name)
			}
		}
	case "parent":
		for _, i := range a.data.Query(core.Filter{}) {
			add(i.ID, i.Code()+" "+i.Title)
		}
	case "favorite":
		add("true", a.tr("已收藏", "Favorited"))
		add("false", a.tr("未收藏", "Not favorited"))
	case "due", "created", "updated":
		value := ""
		if len(r.Values) > 0 {
			value = r.Values[0]
		}
		a.dateButton(c, value, a.tr("筛选日期 ", "Filter date ")+key, func(value string) { r.Values = []string{value} }).Grow(1)
		return
	default:
		entity = false
	}
	if entity {
		names := []string{}
		for _, item := range items {
			if item.checked {
				names = append(names, item.label)
			}
		}
		label := strings.Join(names, ", ")
		if label == "" {
			label = a.tr("选择值…", "Choose values…")
		}
		b := a.chip(c, label, nil).Label("Filter value " + key).Grow(1).MinWidth(0).Justify(ui.Start)
		a.picker(c, b, a.tr("选择筛选值", "Choose filter values"), "", 0, items, true, func(id string) {
			if slices.Contains(r.Values, id) {
				r.Values = slices.DeleteFunc(r.Values, func(v string) bool { return v == id })
			} else {
				r.Values = append(r.Values, id)
			}
		})
		return
	}
	container := ui.Column(c.Key(r.Field)).Grow(1)
	value := ui.Local(container, "value", func() string { return strings.Join(r.Values, ", ") })
	container.Children(func() {
		field.Input(c, value, field.InputProps{Size: theme.Small}).Label("Filter value " + key).OnChange(func() { r.Values = []string{*value} })
	})
}
