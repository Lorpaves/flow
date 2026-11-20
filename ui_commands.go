package main

import (
	"flow/internal/core"
	"fmt"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/display"
)

type appCommand struct {
	id, label, hint string
	run             func()
	keys            bool // hint is a keyboard shortcut rather than context
}

func (a *app) openPalette(scope string) {
	a.closePicker()
	a.keySequence = ""
	a.paletteScope, a.paletteQuery, a.paletteIndex, a.paletteLimit = scope, "", 0, 50
	a.paletteList = ui.ListState{}
	a.paletteTarget = ""
	if i, ok := a.currentIssueAction(); ok {
		a.paletteTarget = i.ID
	}
	a.paletteOpen = true
}

func (a *app) commandResults() []appCommand {
	scope, query := a.paletteScope, strings.ToLower(strings.TrimSpace(a.paletteQuery))
	if scope == "" {
		scope = "all"
	}
	out := []appCommand{}
	add := func(id, zh, en, hint string, run func()) {
		text := zh + " " + en + " " + hint
		if query == "" || strings.Contains(strings.ToLower(text), query) {
			out = append(out, appCommand{id: id, label: a.tr(zh, en), hint: hint, run: run})
		}
	}
	key := func(id, zh, en, keys string, run func()) {
		before := len(out)
		add(id, zh, en, keys, run)
		if len(out) > before {
			out[before].keys = true
		}
	}
	if scope == "all" || scope == "actions" {
		if a.desktop != nil && a.desktop.canOpenWindow() {
			key("new-window", "新建窗口", "New window", "⌘ ⇧ N", func() { a.desktop.newWindow(nil) })
			add("open-page-window", "在新窗口打开当前页面", "Open page in new window", "", a.openPageInWindow)
		}
		key("new-issue", "新建任务", "New issue", "C", func() { a.newTask("", "backlog") })
		key("new-project", "新建项目", "New project", "N P", func() { a.editProject("") })
		for _, v := range []struct{ page, zh, en string }{{"issues", "所有任务", "All issues"}, {"projects", "项目", "Projects"}, {"favorites", "收藏", "Favorites"}, {"settings", "设置", "Settings"}} {
			add("navigate/"+v.page, v.zh, v.en, "", func() { a.navigate(v.page, "") })
		}
		key("help", "键盘快捷键", "Keyboard shortcuts", "⌘ /", func() { a.helpQuery = ""; a.helpOpen = true })
		if st, ok := a.currentUpdateStatus(); ok && st.Phase == updateReady {
			add("restart-update", "重启并更新到 v"+st.Version, "Restart to update to v"+st.Version, "", func() { a.desktop.updater.relaunch() })
		}
		add("check-updates", "检查更新", "Check for updates", "", func() { a.triggerUpdateCheck() })
		add("mcp-settings", "MCP 服务与配置", "MCP server & setup", "", func() { a.openMCPDialog() })
		if i, ok := a.currentIssueAction(); ok {
			hint := i.Code()
			for _, v := range []struct{ kind, zh, en string }{{"id", "复制任务 ID", "Copy issue ID"}, {"title", "复制任务标题", "Copy issue title"}, {"url", "复制任务链接", "Copy issue URL"}, {"markdown", "复制任务 Markdown", "Copy issue Markdown"}, {"branch", "复制分支名称", "Copy branch name"}, {"prompt", "复制任务提示词", "Copy issue prompt"}} {
				add("issue-copy/"+v.kind, v.zh, v.en, hint, func() { a.copyIssue(i, v.kind) })
			}
			if i.DeletedAt == 0 {
				for _, kind := range []string{"related", "sub_issue", "parent_issue", "blocked_issue", "blocking_issue"} {
					label := a.creationRelationLabel(kind)
					add("issue-create/"+kind, "创建"+label, "Create "+label, hint, func() { a.newRelatedTask(i.ID, kind) })
				}
				add("issue-reminder", "提醒此任务", "Remind about issue", hint, func() { a.remindIssue(i.ID) })
				add("issue-document", "添加任务文档", "Add issue document", hint, func() { a.editDocument("issue", i.ID, "") })
				add("issue-project", "移动任务到项目", "Move issue to project", hint, func() { a.openPalette("issue-project") })
				add("issue-milestone", "设置任务里程碑", "Set issue milestone", hint, func() { a.openPalette("issue-milestone") })
				add("issue-parent", "设置父任务", "Set parent issue", hint, func() { a.chooseRelationship(i.ID, "parent") })
				add("issue-recurring", "设置周期任务", "Make recurring", hint, func() { a.editRecurrence("", i) })
				add("issue-trash", "移至回收站", "Move issue to trash", hint, func() { a.trashIssue(i.ID) })
			} else {
				add("issue-restore", "恢复任务", "Restore issue", hint, func() { a.apply(func(d *core.Data) error { return d.RestoreIssue(i.ID) }, nil) })
			}
		}
		if p, ok := a.data.Projects[a.projectID]; ok && p.DeletedAt == 0 {
			for _, v := range []struct{ kind, zh, en string }{{"id", "复制项目 ID", "Copy project ID"}, {"url", "复制项目链接", "Copy project URL"}, {"title", "复制项目标题", "Copy project title"}} {
				add("project-copy/"+v.kind, v.zh, v.en, p.Name, func() { a.copyProject(p, v.kind) })
			}
			add("project-labels", "项目标签", "Project labels", p.Name, a.openProjectLabels)
			add("project-reminder", "提醒此项目", "Remind about project", p.Name, func() { a.remindProject(p.ID) })
			add("project-edit", "编辑当前项目", "Edit current project", p.Name, func() { a.editProject(p.ID) })
			add("project-update", "撰写项目更新", "Write project update", p.Name, func() { a.editProjectUpdate(p.ID, "") })
			add("project-document", "添加项目文档", "Add project document", p.Name, func() { a.editDocument("project", p.ID, "") })
			add("project-milestone", "新建里程碑", "New milestone", p.Name, func() { a.editMilestone(p.ID, "") })
		}
	}
	if scope == "issue-project" {
		i, ok := a.data.Issues[a.paletteTarget]
		if !ok || i.DeletedAt != 0 {
			return out
		}
		if i.ParentID != "" {
			add("project/parent", "子任务跟随父任务项目 · 打开父任务", "Sub-issues follow their parent project · Open parent", "", func() { a.openIssue(i.ParentID) })
			return out
		}
		set := func(id string) { a.batchUpdate([]string{i.ID}, func(v *core.Issue) { v.ProjectID = id }) }
		add("project/none", "无所属项目", "No project", "", func() { set("") })
		for _, p := range a.data.ProjectList(false, false) {
			add("project/"+p.ID, p.Name, p.Name, "", func() { set(p.ID) })
		}
		return out
	}
	if scope == "issue-milestone" {
		i, ok := a.data.Issues[a.paletteTarget]
		if !ok || i.DeletedAt != 0 {
			return out
		}
		set := func(id string) { a.batchUpdate([]string{i.ID}, func(v *core.Issue) { v.MilestoneID = id }) }
		add("milestone/none", "无里程碑", "No milestone", "", func() { set("") })
		for _, m := range a.data.MilestoneList(i.ProjectID) {
			add("milestone/"+m.ID, m.Name, m.Name, "", func() { set(m.ID) })
		}
		return out
	}
	if scope == "project-status" {
		id := a.projectID
		for _, status := range core.ProjectStatuses {
			add("status/"+status, a.projectStatus(status), status, "", func() {
				a.apply(func(d *core.Data) error {
					p, ok := d.Projects[id]
					if !ok || p.DeletedAt != 0 {
						return fmt.Errorf("not_found")
					}
					p.Status = status
					return d.PutProject(p)
				}, nil)
			})
		}
		return out
	}
	if scope == "all" || scope == "projects" || scope == "favorites" {
		for _, p := range a.data.ProjectList(false, false) {
			if scope == "favorites" && !p.Favorite {
				continue
			}
			add("project/"+p.ID, p.Name, p.Name, a.tr("项目", "Project"), func() { a.navigate("project", p.ID) })
		}
	}
	if scope == "all" || scope == "views" {
		for _, v := range a.data.ViewList() {
			add("view/"+v.ID, v.Name, v.Name, a.tr("视图", "View"), func() { a.openSavedView(v.ID) })
		}
	}
	if scope == "all" || scope == "documents" {
		docs := []core.Document{}
		for _, v := range a.data.Documents {
			docs = append(docs, v)
		}
		slices.SortFunc(docs, func(x, y core.Document) int { return strings.Compare(x.Name+x.ID, y.Name+y.ID) })
		for _, v := range docs {
			active := false
			if v.OwnerKind == "issue" {
				i, ok := a.data.Issues[v.OwnerID]
				active = ok && i.DeletedAt == 0
			} else {
				p, ok := a.data.Projects[v.OwnerID]
				active = ok && p.DeletedAt == 0
			}
			if !active {
				continue
			}
			add("document/"+v.ID, v.Name, v.Name, a.tr("文档", "Document"), func() { a.editDocument(v.OwnerKind, v.OwnerID, v.ID) })
		}
	}
	if scope == "all" || scope == "issues" || scope == "favorites" {
		for _, i := range a.data.Query(core.Filter{Query: query, Favorites: scope == "favorites"}) {
			out = append(out, appCommand{id: "issue/" + i.ID, label: i.Code() + "  " + i.Title, hint: a.tr("任务", "Issue"), run: func() { a.openSearchIssue(i.ID, query) }})
		}
	}
	return out
}

// commandMark leads a result with what it is: the issue's state, the project's
// color, or the kind of action.
func (a *app) commandMark(c *ui.Context, cmd appCommand) {
	kind, id, _ := strings.Cut(cmd.id, "/")
	name := map[string]string{
		"new-window": "expand", "open-page-window": "expand", "new-issue": "compose", "new-project": "project", "help": "help",
		"issue-copy": "link", "project-copy": "link", "issue-create": "plus", "issue-reminder": "clock", "project-reminder": "clock",
		"issue-document": "document", "project-document": "document", "document": "document", "issue-project": "project",
		"issue-milestone": "milestone", "project-milestone": "milestone", "milestone": "milestone", "issue-parent": "subissue",
		"issue-recurring": "repeat", "issue-trash": "trash", "issue-restore": "undo", "project-labels": "label",
		"project-edit": "pencil", "project-update": "comment", "view": "layers", "status": "issues",
	}[kind]
	ui.Row(c).Size(16, 16).Shrink(0).Center().TextColor(a.tk.TextTertiary).Children(func() {
		switch kind {
		case "issue":
			statusMark(c, a.data.Issues[id].Status)
		case "project":
			if p, ok := a.data.Projects[id]; ok {
				ui.Box(c).Size(8, 8).Radius(3).Background(ui.Hex(p.Color))
			} else {
				ui.Icon(c, glyph("project")).FontSize(16)
			}
		case "navigate":
			ui.Icon(c, glyph(map[string]string{"issues": "issues", "projects": "project", "favorites": "star", "settings": "gear"}[id])).FontSize(16)
		default:
			if name != "" {
				ui.Icon(c, glyph(name)).FontSize(16)
			}
		}
	})
}

// paletteKeys rewrites a shortcut as shown in the app ("⌘ ⇧ N", "N P") the way
// Lobe's Hotkey takes it ("mod+shift+n", "n+p").
func paletteKeys(keys string) string {
	parts := strings.Fields(keys)
	for n, part := range parts {
		switch part {
		case "⌘":
			parts[n] = "mod"
		case "⇧":
			parts[n] = "shift"
		case "⌥":
			parts[n] = "alt"
		default:
			parts[n] = strings.ToLower(part)
		}
	}
	return strings.Join(parts, "+")
}

func (a *app) palette(c *ui.Context) {
	ui.DialogBase(c, &a.paletteOpen, func(backdrop, panel ui.Element) {
		a.dialogChrome(backdrop, panel)
		w, h := c.Size()
		panel.Width(min(600, w-36)).MaxHeight(h - 36).Padding(0).Gap(0)
		items := a.commandResults()
		a.paletteIndex = max(0, min(a.paletteIndex, len(items)-1))
		if a.paletteLimit < 50 {
			a.paletteLimit = 50
		}
		shown := min(len(items), a.paletteLimit)
		run := func(n int) {
			if n >= 0 && n < len(items) {
				a.paletteOpen = false
				items[n].run()
			}
		}
		move := func(index int) {
			a.paletteIndex = max(0, min(index, len(items)-1))
			a.paletteLimit = max(a.paletteLimit, (a.paletteIndex/50+1)*50)
			a.paletteList.ScrollIntoView(a.paletteIndex)
		}
		placeholders := map[string][2]string{"issues": {"查找任务…", "Find an issue…"}, "projects": {"查找项目…", "Find a project…"}, "favorites": {"查找收藏…", "Find a favorite…"}, "views": {"查找视图…", "Find a view…"}, "documents": {"查找文档…", "Find a document…"}, "issue-project": {"选择项目…", "Choose a project…"}, "issue-milestone": {"选择里程碑…", "Choose a milestone…"}, "project-status": {"选择状态…", "Choose a project status…"}}
		placeholder := a.tr("搜索任务、项目或操作…", "Search issues, projects, or commands…")
		if text, ok := placeholders[a.paletteScope]; ok {
			placeholder = a.tr(text[0], text[1])
		}
		ui.Row(c).Height(48).Padding(0, 16).Gap(10).Children(func() {
			ui.Icon(c, glyph("search")).FontSize(16).TextColor(a.tk.TextQuaternary)
			ui.TextInput(c.Key("palette-search"), &a.paletteQuery).AutoFocus().Placeholder(placeholder).PlaceholderColor(a.tk.TextPlaceholder).Label(a.tr("命令搜索", "Command search")).Grow(1).MinWidth(0).Height(40).FontSize(15).Padding(0).Background(ui.Transparent).Border(0, ui.Transparent).FocusRing(false).OnChange(func() { a.paletteIndex = 0; a.paletteLimit = 50; a.paletteList.ScrollIntoView(0) }).OnTextKey(func(k ui.TextKey) bool {
				if k.Mods != 0 {
					return false
				}
				switch k.Key {
				case ui.KeyDown:
					move(a.paletteIndex + 1)
				case ui.KeyUp:
					move(a.paletteIndex - 1)
				case ui.KeyPageDown:
					move(a.paletteIndex + 8)
				case ui.KeyPageUp:
					move(a.paletteIndex - 8)
				case ui.KeyEnter:
					run(a.paletteIndex)
				default:
					return false
				}
				return true
			})
		})
		ui.Box(c).Height(1).Background(a.tk.BorderSecondary)
		a.paletteList.Key = func(n int) any { return items[n].id }
		ui.List(c, &a.paletteList, shown, func(n int) {
			cmd := items[n]
			b := ui.ButtonBase(c.Key(cmd.id)).Label(cmd.label).Height(36).Padding(0, 12).Radius(a.tk.BorderRadiusSM).Gap(10).Justify(ui.Start).Cursor(ui.CursorPointer)
			if n == a.paletteIndex || b.Hovered() {
				b.Background(a.tk.FillTertiary)
			}
			b.Children(func() {
				a.commandMark(c, cmd)
				ui.Text(c, cmd.label).Grow(1).MinWidth(0).SingleLine()
				if cmd.keys {
					display.Hotkey(c, paletteKeys(cmd.hint), display.HotkeyProps{Compact: true})
				} else {
					ui.Text(c, cmd.hint).FontSize(12).SingleLine().MaxWidth(200).TextColor(a.tk.TextQuaternary)
				}
			}).OnClick(func() { run(n) })
		}).Height(min(360, h-180)).Padding(4)
		if len(items) == 0 {
			ui.Text(c, a.tr("没有结果", "No results")).Padding(0, 16, 16, 16).TextColor(a.tk.TextTertiary)
		}
		if shown < len(items) {
			ui.Row(c).Height(40).Padding(0, 8).BorderWidth(1, 0, 0, 0).BorderColor(a.tk.BorderSecondary).Children(func() {
				quietButton(c, fmt.Sprintf(a.tr("再显示 %d 条", "Show %d more"), min(50, len(items)-shown))).Label(a.tr("加载更多结果", "Load more results")).OnClick(func() { a.paletteLimit += 50 })
			})
		}
	})
}
