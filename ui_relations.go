package main

import (
	"encoding/json"
	"flow/internal/core"
	"github.com/egoist/mygo/ui"
	"slices"
	"strings"
)

func (a *app) relationLabel(kind string) string {
	for _, v := range [][3]string{{"parent", "父任务", "Parent issue"}, {"parent_of", "设为其父任务", "Parent of"}, {"related", "关联任务", "Related to"}, {"blocks", "阻塞", "Blocks"}, {"blocked_by", "被阻塞于", "Blocked by"}, {"duplicate", "重复于", "Duplicate of"}} {
		if v[0] == kind {
			return a.tr(v[1], v[2])
		}
	}
	return kind
}
func (a *app) chooseRelationship(id, kind string) {
	a.flushDrafts()
	a.relationIssue = id
	a.relationKind = kind
	a.relationTarget = ""
	a.relationOpen = true
}
func (a *app) extendedIssueMenu(m *ui.Menu, i core.Issue) {
	m.Submenu(a.tr("创建关联任务", "Create related"), func(m *ui.Menu) {
		for _, kind := range []string{"related", "sub_issue", "parent_issue", "blocked_issue", "blocking_issue"} {
			if m.Item(a.creationRelationLabel(kind)).Chosen() {
				a.newRelatedTask(i.ID, kind)
			}
		}
	})
	if m.Item(a.tr("添加文档", "Add document")).Chosen() {
		a.editDocument("issue", i.ID, "")
	}
	m.Submenu(a.tr("设置关系", "Set relationship"), func(m *ui.Menu) {
		for _, kind := range []string{"parent", "parent_of", "related", "blocked_by", "blocks", "duplicate"} {
			if m.Item(a.relationLabel(kind)).Chosen() {
				a.chooseRelationship(i.ID, kind)
			}
		}
		if i.ParentID != "" && m.Item(a.tr("移除父任务", "Remove parent")).Chosen() {
			a.flushDrafts()
			a.apply(func(d *core.Data) error { return d.ReparentIssue(i.ID, "") }, func() { a.openIssue(i.ID) })
		}
	})
	if m.Item(a.tr("制作副本", "Make a copy")).Chosen() {
		a.flushDrafts()
		var id string
		a.apply(func(d *core.Data) error { v, e := d.CopyIssue(i.ID, i.Title, true); id = v.ID; return e }, func() {
			a.createdIssue = id
			a.notice = a.tr("已复制任务及子任务", "Copied issue and sub-issues")
		})
	}
	if m.Item(a.tr("提醒我", "Remind me")).Chosen() {
		a.remindIssue(i.ID)
	}
	m.Submenu(a.tr("转换为", "Convert to"), func(m *ui.Menu) {
		if m.Item(a.tr("周期任务", "Recurring issue")).Chosen() {
			a.editRecurrence("", i)
		}
		if m.Item(a.tr("任务模板", "Issue template")).Chosen() {
			form := i
			if edit := a.drafts[i.ID]; edit != nil {
				form = edit.issue
				form.Body, _ = json.Marshal(edit.doc.Text())
				form.Plain = edit.doc.Text().PlainText()
			}
			a.startTemplate("", form)
		}
		if m.Item(a.tr("项目", "Project")).Chosen() {
			a.flushDrafts()
			var project string
			a.apply(func(d *core.Data) error { v, e := d.ConvertIssueToProject(i.ID); project = v.ID; return e }, func() { a.navigate("project", project); a.projectTab = "overview" })
		}
	})
}

func (a *app) creationRelationLabel(kind string) string {
	for _, v := range [][3]string{{"related", "关联任务", "Related issue"}, {"sub_issue", "子任务", "Sub-issue"}, {"parent_issue", "父任务", "Parent issue"}, {"blocked_issue", "被此任务阻塞的任务", "Blocked issue"}, {"blocking_issue", "阻塞此任务的任务", "Blocking issue"}} {
		if v[0] == kind {
			return a.tr(v[1], v[2])
		}
	}
	return kind
}
func (a *app) newRelatedTask(id, kind string) {
	target, ok := a.data.Issues[id]
	relation := &core.CreationRelation{IssueID: id, Kind: kind}
	if !ok || target.DeletedAt != 0 || !relation.Valid() {
		a.err = "related_issue_unavailable"
		return
	}
	a.closePicker()
	a.stashComposition()
	form := core.Issue{ProjectID: target.ProjectID, Status: "backlog"}
	if kind == "sub_issue" {
		form.ParentID = id
	}
	if kind == "parent_issue" {
		form.ParentID = target.ParentID
	}
	a.loadCreationContext("related/"+id+"/"+kind, form, relation)
}
func (a *app) detachCreationRelation() {
	if a.newRelation == nil {
		return
	}
	delete(a.compositions, a.composeKey)
	if a.newRelation.Kind == "sub_issue" || a.newRelation.Kind == "parent_issue" {
		a.newIssue.ParentID = ""
	}
	a.newRelation = nil
	a.composeKey = "standalone/" + a.composeID
	a.err = ""
	a.persistComposition()
}
func (a *app) relationshipSection(c *ui.Context, id string) {
	items := []core.Relation{}
	for _, r := range a.data.Relations {
		if r.From == id || r.To == id {
			items = append(items, r)
		}
	}
	if len(items) == 0 {
		return
	}
	slices.SortFunc(items, func(x, y core.Relation) int { return strings.Compare(x.ID, y.ID) })
	ui.Column(c.Key("relationships")).Gap(2).Children(func() {
		a.sectionHeader(c, a.tr("关系", "Relationships"), "", func() {
			iconButton(c, "plus", a.tr("添加关系", "Add relationship")).OnClick(func() { a.chooseRelationship(id, "related") })
		})
		for _, r := range items {
			target, kind := r.To, r.Kind
			if r.To == id {
				target = r.From
				if kind == "blocks" {
					kind = "blocked_by"
				} else if kind == "duplicate" {
					kind = "duplicates"
				}
			}
			other := a.data.Issues[target]
			label := a.relationLabel(kind)
			if kind == "duplicates" {
				label = a.tr("其重复任务", "Duplicated by")
			}
			row := ui.Row(c.Key(r.ID)).Height(36).Padding(0, 4, 0, 8).Gap(8).Radius(a.tk.BorderRadiusSM)
			hovered := row.Hovered()
			if hovered {
				row.Background(a.rowHover())
			}
			row.Children(func() {
				ui.Text(c, label).Width(96).FontSize(12).TextColor(a.tk.TextTertiary).SingleLine()
				ui.Row(c).Size(16, 16).Shrink(0).Center().Children(func() { statusMark(c, other.Status) })
				open := ui.ButtonBase(c).Grow(1).MinWidth(0).Height(28).Gap(10).Justify(ui.Start).Cursor(ui.CursorPointer)
				open.Children(func() {
					ui.Text(c, other.Code()).FontSize(12).TextColor(a.tk.TextTertiary)
					ui.Text(c, other.Title).Grow(1).MinWidth(0).SingleLine()
					if other.DeletedAt != 0 {
						ui.Text(c, a.tr("已删除", "Deleted")).FontSize(12).TextColor(a.tk.TextQuaternary)
					}
				}).OnClick(func() {
					if other.DeletedAt != 0 {
						a.navigate("trash", "")
					}
					a.openIssue(target)
				})
				reveal(iconButton(c, "close", a.tr("移除关系 ", "Remove relation ")+other.Code()), hovered).OnClick(func() { a.apply(func(d *core.Data) error { delete(d.Relations, r.ID); return nil }, nil) })
			})
		}
	})
}
func (a *app) relationshipDialog(c *ui.Context) {
	ui.DialogBase(c, &a.relationOpen, func(backdrop, panel ui.Element) {
		a.dialogChrome(backdrop, panel)
		panel.Width(480).Padding(16).Gap(12)
		a.dialogTitle(c, a.data.Issues[a.relationIssue].Code()+"  "+a.data.Issues[a.relationIssue].Title, nil)
		a.chip(c, a.relationLabel(a.relationKind), nil).Label(a.tr("关系类型", "Relationship kind")).PopoverMenuButton(func(m *ui.Menu) {
			for _, kind := range []string{"parent", "parent_of", "related", "blocked_by", "blocks", "duplicate"} {
				if m.Item(a.relationLabel(kind)).Radio(a.relationKind == kind).Chosen() {
					a.relationKind = kind
					a.relationTarget = ""
				}
			}
		})
		items := []pickerItem{}
		for _, i := range a.data.Query(core.Filter{IncludeArchived: true}) {
			if i.ID == a.relationIssue {
				continue
			}
			if a.relationKind == "parent" && a.data.IsDescendant(i.ID, a.relationIssue) {
				continue
			}
			if a.relationKind == "parent_of" && a.data.IsDescendant(a.relationIssue, i.ID) {
				continue
			}
			items = append(items, pickerItem{id: i.ID, label: i.Code() + "  " + i.Title, status: i.Status})
		}
		label := a.tr("选择任务…", "Choose an issue…")
		if v, ok := a.data.Issues[a.relationTarget]; ok {
			label = v.Code() + "  " + v.Title
		}
		a.picker(c, a.chip(c, label, nil).Label(a.tr("目标任务", "Target issue")), a.tr("选择任务", "Choose issue"), "", 0, items, false, func(v string) { a.relationTarget = v })
		if a.relationKind == "parent" || a.relationKind == "parent_of" {
			ui.Text(c, a.tr("子任务跟随父任务的项目。", "Sub-issues follow their parent's project.")).FontSize(12).TextColor(a.tk.TextTertiary)
		}
		a.dialogError(c)
		dialogFooter(c, func() {
			btn(c, a.tr("取消", "Cancel")).OnClick(func() { a.relationOpen = false })
			primary(c, a.tr("保存", "Save relationship")).Label(a.tr("保存关系", "Save relationship")).Disabled(a.relationTarget == "" || a.pending > 0).OnClick(func() {
				id, target, kind := a.relationIssue, a.relationTarget, a.relationKind
				a.apply(func(d *core.Data) error {
					switch kind {
					case "parent":
						return d.ReparentIssue(id, target)
					case "parent_of":
						return d.ReparentIssue(target, id)
					case "blocked_by":
						_, e := d.AddRelation(target, id, "blocks")
						return e
					default:
						_, e := d.AddRelation(id, target, kind)
						return e
					}
				}, func() {
					a.relationOpen = false
					if a.issueID != "" {
						a.openIssue(a.issueID)
					}
				})
			})
		})
	})
}
