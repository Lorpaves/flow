package main

import (
	"flow/internal/core"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/field"
)

// Sequences belong to navigation, never to a text editor. Invalid second keys,
// Escape, a dialog or a focus change cancel the prefix instead of leaking it.
func (a *app) extraShortcuts(c *ui.Context) bool {
	if c.FocusTakesText() {
		a.keySequence = ""
	}
	if a.keySequence != "" && !c.Now().Before(a.keySequenceUntil) {
		a.keySequence = ""
	}
	c.OnShortcut(ui.Cmd, ui.KeySlash, func() { a.keySequence = ""; a.helpQuery = ""; a.helpOpen = true })
	c.OnShortcut(ui.Cmd, ui.KeyK, func() { a.openPalette("all") })
	if a.keySequence != "" {
		c.After(a.keySequenceUntil.Sub(c.Now()))
		for key := ui.KeyA; key <= ui.KeyZ; key++ {
			c.OnShortcut(0, key, func() { prefix := a.keySequence; a.keySequence = ""; a.runSequence(prefix, key) })
		}
		c.OnShortcut(0, ui.KeyEscape, func() { a.keySequence = "" })
		return true
	}
	if c.FocusTakesText() {
		return false
	}
	begin := func(prefix string) { a.keySequence = prefix; a.keySequenceUntil = c.Now().Add(1500 * time.Millisecond) }
	for _, v := range []struct {
		key    ui.Key
		prefix string
	}{{ui.KeyG, "G"}, {ui.KeyO, "O"}, {ui.KeyN, "N"}} {
		c.OnShortcut(0, v.key, func() { begin(v.prefix) })
	}
	c.OnShortcut(0, ui.KeyBracketLeft, func() { a.navigation.SidebarCollapsed = !a.navigation.SidebarCollapsed })
	c.OnShortcut(ui.Shift, ui.KeyV, func() { a.viewDraft = a.viewConfig.Clone(); a.displayOpen = true })
	c.OnShortcut(0, ui.KeyV, func() { a.newTask("", "backlog"); a.composeExpanded = true })
	c.OnShortcut(ui.Cmd|ui.Alt, ui.KeyX, func() { a.notice = ""; a.createdIssue = "" })
	c.OnShortcut(ui.Cmd|ui.Alt, ui.KeyO, func() {
		if a.createdIssue != "" {
			a.openIssue(a.createdIssue)
		}
	})
	if i, ok := a.currentIssueAction(); ok {
		if i.DeletedAt == 0 {
			c.OnShortcut(0, ui.KeyM, func() { begin("M") })
			c.OnShortcut(ui.Shift, ui.KeyP, func() { a.openPalette("issue-project") })
			c.OnShortcut(ui.Shift, ui.KeyM, func() { a.openPalette("issue-milestone") })
			c.OnShortcut(ui.Cmd, ui.KeyBackspace, func() { a.trashIssue(i.ID) })
			c.OnShortcut(ui.Ctrl, ui.KeyL, func() {
				a.attachmentOwnerKind, a.attachmentOwnerID = "issue", i.ID
				a.attachmentName, a.attachmentURL = "", ""
				a.attachmentOpen = true
			})
		} else {
			c.OnShortcut(ui.Shift, ui.Key3, func() { a.apply(func(d *core.Data) error { return d.RestoreIssue(i.ID) }, nil) })
		}
	}
	if a.page == "project" && a.issueID == "" {
		if a.projectTab != "issues" {
			c.OnShortcut(0, ui.KeyP, func() { begin("P") })
		}
		c.OnShortcut(ui.Cmd, ui.KeyU, func() { a.projectTab = "updates" })
		c.OnShortcut(ui.Cmd|ui.Shift, ui.KeyU, func() { a.editProjectUpdate(a.projectID, "") })
	}
	return false
}

func (a *app) runSequence(prefix string, key ui.Key) {
	switch prefix {
	case "G":
		pages := map[ui.Key]string{ui.KeyE: "issues", ui.KeyX: "archive", ui.KeyP: "projects", ui.KeyS: "settings"}
		if page, ok := pages[key]; ok {
			a.navigate(page, "")
		}
		if key == ui.KeyA || key == ui.KeyB {
			a.navigate("issues", "")
			a.viewConfig = core.DefaultView()
			statuses := []string{"todo", "progress", "review"}
			if key == ui.KeyB {
				statuses = []string{"backlog"}
			}
			a.viewConfig.Filter = core.FilterGroup{Rules: []core.FilterRule{{Field: "status", Op: "is", Values: statuses}}}
		}
	case "O":
		if scope, ok := map[ui.Key]string{ui.KeyI: "issues", ui.KeyP: "projects", ui.KeyF: "favorites", ui.KeyV: "views", ui.KeyD: "documents"}[key]; ok {
			a.openPalette(scope)
		}
	case "N":
		if key == ui.KeyP {
			a.editProject("")
		}
	case "M":
		if i, ok := a.currentIssueAction(); ok && i.DeletedAt == 0 {
			if kind, ok := map[ui.Key]string{ui.KeyB: "blocked_by", ui.KeyX: "blocks", ui.KeyR: "related", ui.KeyM: "duplicate"}[key]; ok {
				a.chooseRelationship(i.ID, kind)
			}
		}
	case "P":
		if key == ui.KeyL {
			a.openProjectLabels()
		}
		if key == ui.KeyS && a.projectID != "" {
			a.openPalette("project-status")
		}
	}
}

type shortcutHelpRow struct{ group, zh, en, keys string }

var shortcutHelpRows = []shortcutHelpRow{
	{"navigation", "新建独立窗口 / 关闭窗口", "New window / Close window", "⌘ ⇧ N / ⌘ W"},
	{"navigation", "命令搜索", "Search commands", "⌘ K"}, {"navigation", "键盘快捷键", "Keyboard shortcuts", "⌘ /"}, {"navigation", "所有任务 / 进行中的任务 / 待整理", "All issues / Active / Backlog", "G E / G A / G B"},
	{"navigation", "项目 / 设置 / 归档项目", "Projects / Settings / Archived projects", "G P / G S / G X"}, {"navigation", "查找任务 / 项目 / 收藏", "Find issue / Project / Favorite", "O I / O P / O F"}, {"navigation", "查找视图 / 文档", "Find view / Document", "O V / O D"}, {"navigation", "切换侧栏", "Toggle sidebar", "["}, {"navigation", "前进 / 后退", "Forward / Back", "⌥ → / ⌥ ←"},
	{"issues", "新建 / 展开新建", "Create / Expanded create", "C / V"}, {"issues", "列表或详情前后切换", "Previous / Next issue", "K / J"}, {"issues", "预览 / 打开 / 切换选中", "Peek / Open / Toggle selection", "Space / Enter / X"}, {"issues", "状态 / 优先级 / 标签", "Status / Priority / Labels", "S / P / L"},
	{"issues", "截止日期 / 移除日期 / 提醒", "Due date / Remove date / Remind", "⇧ D / ⌘ ⇧ D / ⇧ H"}, {"issues", "所属项目 / 里程碑", "Project / Milestone", "⇧ P / ⇧ M"}, {"issues", "父任务 / 子任务", "Open parent / Child", "⌘ ⇧ ↑ / ⌘ ⇧ ↓"}, {"issues", "新建子任务 / 设置父任务", "Create child / Set parent", "⌘ ⇧ O / ⌘ ⇧ P"},
	{"issues", "被阻塞 / 阻塞 / 关联 / 重复", "Blocked / Blocking / Related / Duplicate", "M B / M X / M R / M M"}, {"issues", "重命名 / 编辑说明 / 聚焦评论", "Rename / Description / Comment", "⇧ R / Ctrl ⇧ I / Ctrl M"}, {"issues", "收藏 / 添加链接 / 移至回收站 / 恢复", "Favorite / Link / Trash / Restore", "⌥ F / Ctrl L / ⌘ ⌫ / #"},
	{"copy", "复制 ID / 分支", "Copy ID / Branch", "⌘ . / ⌘ ⇧ ."}, {"copy", "复制链接 / 标题", "Copy URL / Title", "⌘ ⇧ , / ⌘ ⇧ '"}, {"copy", "复制 Markdown / 提示词", "Copy Markdown / Prompt", "⌘ ⌥ C / ⌘ ⌥ P"},
	{"comments", "回复聚焦评论", "Reply to focused comment", "R"}, {"comments", "发送 / 换行", "Send / New line", "Enter or ⌘ Enter / ⇧ Enter"},
	{"projects", "新建项目 / 修改项目状态", "New project / Project status", "N P / P S"}, {"projects", "项目标签 / 项目提醒", "Project labels / Reminder", "P L / ⇧ H"}, {"projects", "项目开始 / 目标日期", "Project start / Target date", "Ctrl ⌥ S / Ctrl ⌥ D"}, {"projects", "复制项目 ID / 链接 / 标题", "Copy project ID / URL / Title", "⌘ . / ⌘ ⇧ , / ⌘ ⇧ '"},
	{"projects", "查看更新 / 撰写更新", "View updates / Write update", "⌘ U / ⌘ ⇧ U"},
	{"timeline", "周 / 月 / 季 / 年 / 今天", "Week / Month / Quarter / Year / Today", "W / M / Q / Y / T"},
	{"timeline", "缩放 / 重置到年", "Zoom / Reset to year", "− / = / 0 / ⌘ scroll"},
	{"timeline", "选择前后项 / 切换选中 / 打开", "Previous or next / Toggle selection / Open", "↑ ↓ ← → / X / Enter"},
	{"timeline", "联动依赖 / 保持关联日期", "Shift dependencies / Keep related dates", "Shift drag / ⌘ drag"},
	{"editor", "粗体 / 斜体 / 撤销 / 重做", "Bold / Italic / Undo / Redo", "⌘ B / ⌘ I / ⌘ Z / ⌘ ⇧ Z"}, {"editor", "块菜单", "Block menu", "/"},
}

func (a *app) shortcutHelp(c *ui.Context) {
	ui.DialogBase(c, &a.helpOpen, func(backdrop, panel ui.Element) {
		a.dialogChrome(backdrop, panel)
		w, h := c.Size()
		panel.Width(min(640, w-36)).MaxHeight(h - 36).Padding(16).Gap(12)
		a.dialogTitle(c, a.tr("快捷键", "Keyboard shortcuts"), func() { a.helpOpen = false })
		field.SearchBar(c.Key("help-search"), &a.helpQuery, field.SearchBarProps{Placeholder: a.tr("搜索", "Search"), Variant: field.InputFilled}).AutoFocus().Label(a.tr("搜索快捷键", "Search shortcuts"))
		ui.Scroll(c).Height(min(520, h-180)).Children(func() {
			group := ""
			q := strings.ToLower(strings.TrimSpace(a.helpQuery))
			for _, v := range shortcutHelpRows {
				if q != "" && !strings.Contains(strings.ToLower(v.zh+v.en+v.keys), q) {
					continue
				}
				if v.group != group {
					group = v.group
					names := map[string][2]string{"navigation": {"导航", "Navigation"}, "issues": {"任务", "Issues"}, "copy": {"复制", "Copy"}, "comments": {"评论", "Comments"}, "projects": {"项目", "Projects"}, "timeline": {"时间线", "Timeline"}, "editor": {"编辑", "Editor"}}
					n := names[group]
					ui.Text(c, a.tr(n[0], n[1])).FontSize(12).FontWeight(500).TextColor(a.tk.TextTertiary).Margin(14, 0, 4, 0)
				}
				// Alternatives are separated by " / "; each one is a run of keycaps.
				ui.Row(c.Key(v.en)).MinHeight(32).Gap(6).Padding(0, 14, 0, 0).Children(func() {
					ui.Text(c, a.tr(v.zh, v.en)).Grow(1).MinWidth(0).TextColor(a.tk.TextSecondary)
					for n, keys := range strings.Split(v.keys, " / ") {
						if n > 0 {
							ui.Text(c.Key(n*2), "/").FontSize(11).TextColor(a.tk.TextQuaternary)
						}
						a.kbd(c.Key(n*2+1), keys)
					}
				})
			}
		})
	})
}
