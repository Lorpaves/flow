package main

import (
	"encoding/json"
	"flow/internal/core"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
)

func (a *app) activityText(v core.Activity) string {
	noun := map[string][2]string{"issue": {"任务", "issue"}, "project": {"项目", "project"}, "comment": {"评论", "comment"}, "relation": {"关系", "relationship"}, "attachment": {"附件", "attachment"}, "document": {"文档", "document"}, "milestone": {"里程碑", "milestone"}, "project_update": {"项目更新", "project update"}}[v.Kind]
	action := map[string][2]string{"created": {"创建了", "Created"}, "updated": {"修改了", "Updated"}, "removed": {"移除了", "Removed"}, "trashed": {"删除了", "Deleted"}, "restored": {"恢复了", "Restored"}}[v.Action]
	label := a.tr(action[0]+noun[0], action[1]+" "+noun[1])
	if v.Action == "updated" {
		fields := []string{}
		names := map[string][2]string{"title": {"标题", "title"}, "body": {"说明", "description"}, "plain": {"说明", "description"}, "status": {"状态", "status"}, "priority": {"优先级", "priority"}, "projectId": {"项目", "project"}, "parentId": {"父任务", "parent"}, "milestoneId": {"里程碑", "milestone"}, "labelIds": {"标签", "labels"}, "due": {"截止日期", "due date"}, "favorite": {"收藏", "favorite"}, "description": {"说明", "description"}, "name": {"名称", "name"}, "health": {"健康状态", "health"}, "dependencies": {"依赖", "dependencies"}, "archived": {"归档", "archive"}, "start": {"开始日期", "start date"}, "completed": {"完成状态", "completion"}}
		for _, field := range v.Fields {
			title := field
			if n, ok := names[field]; ok {
				title = a.tr(n[0], n[1])
			}
			if !slices.Contains(fields, title) {
				fields = append(fields, title)
			}
		}
		label += " · " + strings.Join(fields, ", ")
	}
	return label
}

// activitySection is the conversation on an issue, then a short quiet log of
// what changed. A row's actions show under the pointer.
func (a *app) activitySection(c *ui.Context, kind, id string) {
	a.divider(c).Margin(8, 0, 0, 0)
	if kind == "issue" {
		a.commentsSection(c, id)
	}
	items := a.data.History(kind, id)
	if a.historyLimits == nil {
		a.historyLimits = map[string]int{}
	}
	key := kind + "/" + id
	limit := a.historyLimits[key]
	if limit < 5 {
		limit = 5
	}
	ui.Column(c.Key("activity-" + key)).Children(func() {
		for n, v := range items {
			if n >= limit {
				break
			}
			row := ui.Row(c.Key("activity-" + v.ID)).Height(28).Gap(8).FontSize(12).TextColor(a.tk.TextTertiary)
			hovered := row.Hovered()
			row.Children(func() {
				ui.Row(c).Size(16, 16).Shrink(0).Center().Children(func() { ui.Box(c).Size(5, 5).Radius(3).Background(a.tk.TextQuaternary) })
				ui.Text(c, a.activityText(v)).SingleLine().MinWidth(0)
				when := a.shortTime(v.At)
				if v.Source == "mcp" {
					when += " · MCP"
				} else if v.Source == "scheduler" {
					when += " · " + a.tr("计划", "Schedule")
				}
				if v.UndoneAt != 0 {
					when += " · " + a.tr("已撤销", "Undone")
				}
				ui.Text(c, when).SingleLine().TextColor(a.tk.TextQuaternary)
				ui.Spacer(c)
				reveal(quietButton(c, a.tr("详情", "Details")), hovered).Label(a.activityControlLabel("details", v)).OnClick(func() { a.historyRecord = v; a.historyOpen = true })
				if a.data.CanUndoActivity(v) {
					reveal(quietButton(c, a.tr("撤销", "Undo")), hovered).Label(a.activityControlLabel("undo", v)).Disabled(a.pending > 0).OnClick(func() {
						a.flushDrafts()
						a.apply(func(d *core.Data) error { return d.UndoActivity(v.ID) }, func() {
							if kind == "issue" && a.issueID == id {
								a.openIssue(id)
							}
						})
					})
				}
			})
		}
	})
	if len(items) > limit {
		ui.Row(c).Children(func() {
			quietButton(c, fmt.Sprintf(a.tr("更早的 %d 条", "%d earlier"), len(items)-limit)).Label(a.tr("显示更多活动", "Show more activity")).OnClick(func() { a.historyLimits[key] = limit + 30 })
			ui.Spacer(c)
		})
	}
}
func descriptionSnapshot(v core.Activity, before bool) string {
	raw := v.After
	if before {
		raw = v.Before
	}
	switch v.Kind {
	case "issue":
		var i core.Issue
		if json.Unmarshal(raw, &i) == nil {
			return documentFor(i).Text().Markdown()
		}
	case "project":
		var p core.Project
		if json.Unmarshal(raw, &p) == nil {
			return p.Description
		}
	case "document":
		var d core.Document
		if json.Unmarshal(raw, &d) == nil {
			return d.Body
		}
	}
	return ""
}
func (a *app) historyDialog(c *ui.Context) {
	ui.DialogBase(c, &a.historyOpen, func(backdrop, panel ui.Element) {
		a.dialogChrome(backdrop, panel)
		panel.Width(720).Padding(16).Gap(12)
		v := a.historyRecord
		ui.Row(c).MinHeight(32).Gap(8).Children(func() {
			ui.Text(c, a.activityText(v)).FontWeight(600).Grow(1)
			iconButton(c, "close", a.tr("关闭历史", "Close history")).OnClick(func() { a.historyOpen = false })
		})
		hasDescription := (v.Kind == "issue" && (slices.Contains(v.Fields, "body") || slices.Contains(v.Fields, "plain"))) || v.Kind == "project" && slices.Contains(v.Fields, "description") || v.Kind == "document" && slices.Contains(v.Fields, "body")
		ui.Scroll(c).Height(330).Gap(15).Children(func() {
			if hasDescription {
				ui.Text(c, a.tr("修改前", "Before")).FontSize(12).TextColor(a.tk.TextTertiary)
				markdown.View(c, descriptionSnapshot(v, true), a.mediaMarkdownOptions())
				a.divider(c)
				ui.Text(c, a.tr("修改后", "After")).FontSize(12).TextColor(a.tk.TextTertiary)
				markdown.View(c, descriptionSnapshot(v, false), a.mediaMarkdownOptions())
			} else {
				var before, after map[string]json.RawMessage
				_ = json.Unmarshal(v.Before, &before)
				_ = json.Unmarshal(v.After, &after)
				for _, key := range v.Fields {
					if slices.Contains([]string{"sha256", "data", "ownerId", "ownerKind", "updatedAt", "createdAt"}, key) {
						continue
					}
					ui.Text(c, a.historyFieldLabel(key)).FontSize(12).TextColor(a.tk.TextTertiary)
					ui.Text(c, a.historyValue(key, before[key])+" → "+a.historyValue(key, after[key])).Selectable()
				}
			}
		})
		a.dialogError(c)
		dialogFooter(c, func() {
			if hasDescription && len(v.Before) > 0 && a.data.Issues[v.OwnerID].DeletedAt == 0 && a.data.Projects[v.OwnerID].DeletedAt == 0 {
				btn(c, a.tr("恢复修改前", "Restore previous description")).Disabled(a.pending > 0).OnClick(func() {
					a.flushDrafts()
					a.apply(func(d *core.Data) error { return d.RestoreDescription(v.ID, true) }, func() {
						a.historyOpen = false
						if a.issueID != "" {
							a.openIssue(a.issueID)
						}
					})
				})
			}
		})
	})
}

func shortText(text string) string {
	r := []rune(strings.Join(strings.Fields(text), " "))
	if len(r) > 45 {
		return string(r[:45]) + "…"
	}
	return string(r)
}
func (a *app) activityControlLabel(action string, v core.Activity) string {
	prefix := a.tr("查看记录：", "Activity details: ")
	if action == "undo" {
		prefix = a.tr("撤销记录：", "Undo activity: ")
	}
	return prefix + a.activityText(v) + " " + time.UnixMilli(v.At).Format("15:04:05")
}

func (a *app) historyFieldLabel(key string) string {
	names := map[string][2]string{"title": {"标题", "Title"}, "name": {"名称", "Name"}, "status": {"状态", "Status"}, "priority": {"优先级", "Priority"}, "projectId": {"项目", "Project"}, "parentId": {"父任务", "Parent"}, "milestoneId": {"里程碑", "Milestone"}, "labelIds": {"标签", "Labels"}, "due": {"截止日期", "Due date"}, "favorite": {"收藏", "Favorite"}, "archived": {"归档", "Archived"}, "deletedAt": {"删除状态", "Trash"}, "body": {"正文", "Body"}, "plain": {"正文", "Body"}, "description": {"说明", "Description"}, "health": {"健康状态", "Health"}, "start": {"开始日期", "Start date"}, "dependencies": {"依赖", "Dependencies"}, "kind": {"关系类型", "Relationship type"}, "from": {"来源任务", "From issue"}, "to": {"目标任务", "To issue"}, "url": {"链接", "URL"}, "mime": {"文件类型", "File type"}, "completed": {"完成", "Completed"}}
	if v, ok := names[key]; ok {
		return a.tr(v[0], v[1])
	}
	return a.tr("属性", "Property")
}
func (a *app) historyValue(key string, raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return "—"
	}
	var v any
	if json.Unmarshal(raw, &v) != nil {
		return "—"
	}
	switch value := v.(type) {
	case string:
		if value == "" {
			return "—"
		}
		switch key {
		case "status":
			if slices.Contains(core.Statuses, value) {
				return a.status(value)
			}
			return a.projectStatus(value)
		case "projectId":
			if p, ok := a.data.Projects[value]; ok {
				return p.Name
			}
		case "parentId", "from", "to":
			if i, ok := a.data.Issues[value]; ok {
				return i.Code() + " " + i.Title
			}
		case "milestoneId":
			if m, ok := a.data.Milestones[value]; ok {
				return m.Name
			}
		case "kind":
			return a.relationLabel(value)
		case "health":
			for _, h := range [][3]string{{"on_track", "进展正常", "On track"}, {"at_risk", "存在风险", "At risk"}, {"off_track", "进度落后", "Off track"}} {
				if value == h[0] {
					return a.tr(h[1], h[2])
				}
			}
		}
		return value
	case bool:
		if value {
			return a.tr("是", "Yes")
		}
		return a.tr("否", "No")
	case float64:
		if key == "priority" {
			return a.priority(int(value))
		}
		if key == "deletedAt" {
			if value == 0 {
				return a.tr("未删除", "Active")
			}
			return a.tr("已删除", "Deleted")
		}
	case []any:
		names := []string{}
		for _, id := range value {
			if key, ok := id.(string); ok {
				if l, ok := a.data.Labels[key]; ok {
					names = append(names, l.Name)
				} else if p, ok := a.data.Projects[key]; ok {
					names = append(names, p.Name)
				}
			}
		}
		if len(names) == 0 {
			return "—"
		}
		return strings.Join(names, ", ")
	}
	return string(raw)
}
