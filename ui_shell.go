package main

import (
	"flow/internal/core"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
	"github.com/lobehub/lobe-mygo/pkg/choice"
	"github.com/lobehub/lobe-mygo/pkg/feedback"
	"github.com/lobehub/lobe-mygo/pkg/layout"
	"github.com/lobehub/lobe-mygo/pkg/lucide"
	"github.com/lobehub/lobe-mygo/pkg/overlay"
	"github.com/lobehub/lobe-mygo/pkg/theme"
)

func (a *app) view(c *ui.Context) {
	a.services = c.Services()
	a.propertyPickers = map[ui.Key]*pickerState{}
	a.projectLabels = nil
	a.peekRendered = false
	a.peekShortcuts(c)
	a.themeFrame(c)
	a.forgetGone()
	a.observeUpdateCheck()
	a.trackFocus(c)
	c.SetMenuPresentation(ui.PopoverMenus)
	a.refreshDesktopMenu()
	if a.desktop != nil && a.desktop.canOpenWindow() {
		c.OnShortcut(ui.Cmd|ui.Shift, ui.KeyN, func() { a.desktop.newWindow(nil) })
	}
	if a.isRestoring() {
		ui.Column(c).Fill().Center().Background(a.theme.Background).Children(func() {
			ui.Text(c, a.tr("正在恢复工作区…", "Restoring your workspace…"))
		})
		return
	}
	if !a.navigationActionsEnabled() || c.FocusTakesText() {
		a.keySequence = ""
	}
	if a.navigationActionsEnabled() && !a.extraShortcuts(c) {
		a.propertyShortcuts(c)
		a.issueActionShortcuts(c)
		a.projectActionShortcuts(c)
		a.timelineShortcuts(c)
		if !c.FocusTakesText() {
			c.OnShortcut(0, ui.KeyC, func() { a.newTask("", "backlog") })
			c.OnShortcut(0, ui.KeySlash, func() { a.backToIssues(); a.search.Focus() })
			if a.issueID != "" {
				c.OnShortcut(0, ui.KeyJ, func() { a.stepIssue(1) })
				c.OnShortcut(0, ui.KeyK, func() { a.stepIssue(-1) })
			} else if !a.timelineVisible() && (a.page == "issues" || a.page == "project" || a.page == "favorites") {
				for _, key := range []ui.Key{ui.KeyJ, ui.KeyDown} {
					c.OnShortcut(0, key, func() { a.stepList(1) })
				}
				for _, key := range []ui.Key{ui.KeyK, ui.KeyUp} {
					c.OnShortcut(0, key, func() { a.stepList(-1) })
				}
				c.OnShortcut(0, ui.KeyEnter, func() {
					if a.focusedIssue != "" {
						a.openIssue(a.focusedIssue)
					}
				})
				c.OnShortcut(0, ui.KeyX, func() { a.toggleSelected(a.focusedIssue) })
				c.OnShortcut(0, ui.KeySpace, func() { a.openPeek(a.focusedIssue) })
			}
		}
		c.OnShortcut(ui.Alt, ui.KeyLeft, func() { a.historyStep(-1) })
		c.OnShortcut(ui.Alt, ui.KeyRight, func() { a.historyStep(1) })
		c.OnShortcut(ui.Cmd, ui.KeyN, func() { a.newTask("", "backlog") })
		c.OnShortcut(0, ui.KeyEscape, func() {
			if a.issueID != "" {
				if d := a.drafts[a.issueID]; d != nil && d.editing {
					d.editing = false
					a.saveDraft(a.issueID)
				} else {
					a.backToIssues()
				}
			} else if a.timelineVisible() {
				a.timelineSelected = nil
				a.timelineLinkTo = ""
				if strings.HasPrefix(a.timelineFocus, "milestone/") {
					m := a.data.Milestones[strings.TrimPrefix(a.timelineFocus, "milestone/")]
					a.timelineFocus = "project/" + m.ProjectID
				}
			} else {
				a.selected = map[string]bool{}
				a.query = ""
				a.list.Handle.Focus()
			}
		})
	}

	ui.Column(c).Fill().Background(a.tk.BgLayout).Children(func() {
		if !a.ready {
			ui.Column(c).Grow(1).Center().Gap(16).Children(func() {
				a.brand(c, 48)
				if a.err != "" {
					ui.Text(c, a.errorText(a.err)).TextColor(a.tk.Error)
				} else {
					feedback.Spin(c, feedback.SpinProps{}, nil)
				}
			})
			return
		}
		if !a.data.Preferences.Initialized {
			a.welcome(c)
			return
		}
		a.titleBar(c)
		content := func() {
			left := float32(0)
			if a.navigation.SidebarCollapsed {
				left = 8
			}
			// The page's content is one large tile; what it holds is laid
			// on it as white cards.
			ui.Column(c.Key("workspace-content")).Grow(1).MinWidth(0).Margin(0, 10, 10, left).Radius(22).Background(a.tile()).Clip().Children(func() {
				if a.err != "" {
					ui.Row(c).Padding(6, 8, 6, 16).Gap(8).Background(a.tk.ErrorBg).Children(func() {
						ui.Icon(c, lucide.CircleX()).FontSize(14).TextColor(a.tk.Error)
						ui.Text(c, a.errorText(a.err)).Grow(1).MinWidth(0).SingleLine()
						textButton(c, a.tr("重试", "Retry saves")).OnClick(func() {
							a.err = ""
							a.flushDrafts()
							if a.desktop != nil {
								a.desktop.persist()
							}
						})
						iconButton(c, "close", a.tr("关闭提示", "Dismiss")).OnClick(func() { a.err = "" })
					})
				}
				a.updateBanner(c)
				switch {
				case a.issueID != "":
					if a.viewConfig.Layout == "split" && a.page != "trash" {
						ui.Row(c).Grow(1).MinHeight(0).Children(func() {
							ui.Column(c.Key("split-list")).Width(400).MinWidth(320).FillHeight().Children(func() { a.listView(c, a.filtered()) })
							ui.Column(c.Key("split-detail")).Grow(1).MinWidth(300).FillHeight().Children(func() { a.issueView(c) })
						})
					} else {
						a.issueView(c)
					}
				case a.page == "reminders":
					a.remindersView(c)
				case a.page == "drafts" || a.page == "templates":
					a.libraryView(c)
				case a.page == "settings":
					a.settingsView(c)
				case a.page == "projects" || a.page == "archive":
					a.projectsView(c)
				case a.page == "project" && a.projectTab == "overview":
					a.overview(c)
				case a.page == "project" && a.projectTab != "issues":
					a.projectRecordsView(c)
				default:
					a.issuesView(c)
				}
				a.keyHint(c)
			})
		}
		if a.navigation.SidebarCollapsed {
			content()
		} else {
			pane := func(build func()) func() {
				return func() { ui.Themed(c, a.theme).Fill().Background(ui.Transparent).Children(build) }
			}
			ui.Themed(c.Key("workspace-seamless"), a.seamless).Grow(1).MinHeight(0).AlignItems(ui.Stretch).Background(ui.Transparent).Children(func() {
				ui.Split(c.Key("workspace-split"), &a.sidebarWidth, pane(func() { a.sidebar(c) }), pane(content)).Grow(1).MinHeight(0).SplitReset(224).OnChange(func() { a.sidebarWidth = max(200, min(380, a.sidebarWidth)) })
			})
		}
		feedback.ToastHost(c, feedback.ToastHostProps{})
	})
	if a.peekOpen && !a.peekRendered {
		a.peekOpen = false
	}
	a.createDialog(c)
	a.projectDialog(c)
	a.palette(c)
	a.shortcutHelp(c)
	a.datePickerDialog(c)
	a.viewDialogs(c)
	a.templateDialog(c)
	a.relationshipDialog(c)
	a.childDialog(c)
	a.historyDialog(c)
	a.attachmentDialog(c)
	a.scheduleDialogs(c)
	a.projectRecordDialogs(c)
	a.organizationDialogs(c)
	a.eraseDialog(c)
	a.updateDialog(c)
	a.mcpDialog(c)
	a.observeComposition(c)
	a.observeDesktopSession(c)
	a.focusSeen = a.focusedIssue
	for id, d := range a.drafts {
		if d.dirty && !d.busy && a.err == "" {
			left := 600*time.Millisecond - c.Now().Sub(d.changed)
			if left <= 0 {
				a.saveDraft(id)
			} else {
				c.After(left)
			}
		}
	}
}

// brand is Flow's mark: three bars stepping forward, as work moves along a
// timeline. The app icon is the same drawing (cmd/icon).
func (a *app) brand(c *ui.Context, size float32) {
	u := size / 24
	ui.Box(c).Size(size, size).Radius(size*.225).Gradient(ui.Hex("#24242e"), ui.Hex("#0b0b0f"), 180).Shrink(0).Children(func() {
		for n, bar := range [][3]float32{{5.1, 6.8, 9.1}, {8, 10.6, 11}, {10.9, 14.4, 6.8}} {
			tone := [][2]string{{"#ddd6fe", "#a78bfa"}, {"#c7d2fe", "#818cf8"}, {"#bae6fd", "#38bdf8"}}[n]
			ui.Box(c.Key(n)).Absolute().Left(bar[0]*u).Top(bar[1]*u).Size(bar[2]*u, 2.8*u).Radius(1.4*u).Gradient(ui.Hex(tone[0]), ui.Hex(tone[1]), 90)
		}
	})
}
func (a *app) welcome(c *ui.Context) {
	ui.Column(c).Grow(1).Center().Gap(12).Children(func() {
		a.brand(c, 64)
		ui.Text(c, "Flow").FontSize(a.tk.FontSizeHeading3).FontWeight(600).Margin(8, 0, 0, 0)
		ui.Text(c, a.tr("离线的个人项目管理", "Personal project management, offline")).TextColor(a.tk.TextTertiary)
		ui.Row(c).Gap(8).Margin(16, 0, 0, 0).Children(func() {
			btn(c, a.tr("空白工作区", "Blank workspace")).Disabled(a.pending > 0).OnClick(func() { a.apply(func(d *core.Data) error { d.Preferences.Initialized = true; return nil }, nil) })
			primary(c, a.tr("演示工作区", "Demo workspace")).Disabled(a.pending > 0).OnClick(func() { a.apply(func(d *core.Data) error { d.Seed(); return nil }, nil) })
		})
		quietButton(c, a.tr("English", "简体中文")).OnClick(func() {
			locale := "en"
			if a.data.Preferences.Locale == "en" {
				locale = "zh-Hans"
			}
			a.apply(func(d *core.Data) error { d.Preferences.Locale = locale; return nil }, func() {
				if a.window != nil {
					mygo.App.SetLocale(locale)
				}
			})
		})
		if a.err != "" {
			ui.Text(c, a.errorText(a.err)).TextColor(a.tk.Error)
		}
	})
}

// trackFocus tells a focus change made by the keyboard or a command from one
// made by the pointer. Rows follow the pointer (see hoverFocus) until the
// keyboard moves the focus; they then wait for the pointer to move again, so a
// resting pointer never steals the row the keyboard chose.
//
// Only a chosen focus is kept in the route. A row merely passed over is not
// worth a history entry or a session write, which would also briefly disable
// the controls that wait for pending writes.
//
// It also hands a pending notice to Lobe's toasts, which float over the page
// and leave on their own.
func (a *app) trackFocus(c *ui.Context) {
	if a.focusedIssue != a.focusSeen {
		a.hoverHold, a.hoverHeld = true, false
		a.selectAnchor, a.keptFocus = a.focusedIssue, a.focusedIssue
	}
	if a.notice == "" {
		return
	}
	toast := feedback.ToastProps{ID: "flow-notice", Title: a.notice}
	if id := a.createdIssue; id != "" {
		toast.Type, toast.Timeout = feedback.ToastSuccess, 8*time.Second
		toast.Action, toast.OnAction = a.tr("打开任务", "Open issue"), func() { a.openIssue(id) }
	}
	feedback.ShowToast(c, toast)
	a.notice = ""
}

// hoverFocus moves the list focus to the row under the pointer, so S, P, L, X
// and Space act on what the pointer is over, as in Linear.
func (a *app) hoverFocus(c *ui.Context, row ui.Element, id string) {
	if a.issueID != "" || a.peekOpen || a.activePicker != nil && a.activePicker.open || !row.Hovered() {
		return
	}
	if !a.hoverHold {
		if a.focusedIssue != id {
			a.focusedIssue, a.focusSeen = id, id
			c.Invalidate() // rows built before this one drew the old focus
		}
		return
	}
	x, y, _ := row.PointerPosition()
	x, y = row.LocalToWindow(x, y)
	if !a.hoverHeld {
		a.hoverX, a.hoverY, a.hoverHeld = x, y, true
		return
	}
	if dx, dy := x-a.hoverX, y-a.hoverY; dx*dx+dy*dy > 4 {
		a.hoverHold, a.hoverHeld = false, false
		a.focusedIssue, a.focusSeen = id, id
		c.Invalidate()
	}
}

// keyHint shows the first key of a two-key shortcut while the second is awaited.
func (a *app) keyHint(c *ui.Context) {
	if a.keySequence == "" {
		return
	}
	hint := ui.Row(c.Key("key-sequence")).Absolute().Bottom(16).Left(16).Height(32).Padding(0, 10).Gap(6).Radius(a.tk.BorderRadius).Background(a.tk.BgElevated)
	a.tk.Elevate(hint, theme.BoxShadowSecondary)
	hint.Children(func() {
		a.kbd(c, a.keySequence)
		ui.Text(c, "…").TextColor(a.tk.TextTertiary)
	})
}

// boundIndex lends a Lobe control (Segmented, Tabs) an index to own between
// frames while the value lives elsewhere. The control writes its choice into
// the index after a build; the next build sees the index differ from the
// value and hands it to set, once. A value changed elsewhere wins.
type boundIndex struct{ index, value, sent int }

func (a *app) boundIndex(c *ui.Context, key string, current int, set func(int)) *int {
	if a.segments == nil {
		a.segments = map[string]*boundIndex{}
	}
	st := a.segments[key]
	if st == nil {
		st = &boundIndex{index: current, value: current, sent: -1}
		a.segments[key] = st
	}
	switch {
	case st.value != current:
		st.index, st.value, st.sent = current, current, -1
	case st.index != current && st.index != st.sent:
		st.sent = st.index
		set(st.index)
		c.Invalidate()
	}
	return &st.index
}

// segmented is Lobe's Segmented over a value kept elsewhere.
func (a *app) segmented(c *ui.Context, key string, options []string, current int, small bool, set func(int)) ui.Element {
	p := action.SegmentedProps{}
	if small {
		p.Size = theme.Small
	}
	return action.Segmented(c.Key("segmented-"+key), a.boundIndex(c, key, current, set), options, p)
}

const (
	titleBarHeight  = 44
	trafficLightEnd = 80
)

// titleBar is the window's top row, as in Lobe's desktop apps: history on the
// left over the sidebar, then where the user is, then what can be done there.
// Pages below it hold content only.
func (a *app) titleBar(c *ui.Context) {
	bar := c.TitleBar()
	inset := float32(8)
	if bar.Left > 0 {
		inset = max(bar.Left+8, trafficLightEnd)
	}
	ui.Row(c).Height(max(titleBarHeight, bar.Height)).Shrink(0).Padding(0, max(12, bar.Right+8), 0, 0).Gap(8).DragWindow().Children(func() {
		nav := ui.Row(c).Shrink(0).Gap(2).Padding(0, 6, 0, inset)
		if !a.navigation.SidebarCollapsed {
			nav.Width(max(a.sidebarWidth, inset+128)).Justify(ui.SpaceBetween)
		}
		nav.Children(func() { a.navigationControls(c) })
		a.breadcrumb(c)
		ui.Spacer(c)
		a.pageActions(c)
	})
}

// breadcrumb says where the user is. In an issue it is the way back: the list,
// any issues passed through, then this one.
func (a *app) breadcrumb(c *ui.Context) {
	crumb := func(c *ui.Context) ui.Element {
		b := ui.ButtonBase(c).Height(28).Padding(0, 8).Gap(8).Radius(a.tk.BorderRadiusSM).TextColor(a.tk.TextSecondary).Cursor(ui.CursorPointer)
		if b.Hovered() {
			b.Background(a.tk.FillTertiary).TextColor(a.tk.Text)
		}
		return b
	}
	sep := func() { ui.Icon(c, lucide.ChevronRight()).FontSize(12).TextColor(a.tk.TextQuaternary) }
	ui.Row(c).Gap(2).MinWidth(0).Children(func() {
		if a.issueID == "" {
			ui.Row(c).Height(28).Padding(0, 8).Gap(8).Children(func() {
				ui.Icon(c, glyph(a.pageIcon())).FontSize(16).TextColor(a.tk.TextTertiary)
				ui.Text(c, a.pageTitle()).FontWeight(500).SingleLine()
			})
			return
		}
		back := a.tr("返回列表", "Back to issues")
		root := crumb(c.Key("crumb-root")).Label(back)
		overlay.Tooltip(c, root, back, overlay.TooltipProps{Hotkey: "esc"})
		root.Children(func() {
			ui.Icon(c, glyph(a.pageIcon())).FontSize(16).TextColor(a.tk.TextTertiary)
			ui.Text(c, a.pageTitle()).SingleLine().MaxWidth(220)
		}).OnClick(func() { a.returnIssues = nil; a.backToIssues() })
		for n, id := range a.returnIssues {
			sep()
			b := crumb(c.Key("crumb-" + id)).Children(func() { ui.Text(c, a.data.Issues[id].Code()).SingleLine() })
			if n == len(a.returnIssues)-1 {
				b.Label(a.tr("返回上一页", "Back"))
			}
			b.OnClick(func() { a.returnIssues = a.returnIssues[:n+1]; a.backToIssues() })
		}
		sep()
		ui.Text(c, a.data.Issues[a.issueID].Code()).FontWeight(500).Padding(0, 8)
	})
}

// pageActions are the few things to do with the page shown; everything else
// lives in the page's menu.
func (a *app) pageActions(c *ui.Context) {
	ui.Row(c).Gap(4).Shrink(0).Children(func() {
		ui.Row(c.Key("page")).Gap(4).Children(func() {
			switch {
			case a.issueID != "":
				a.issueActions(c)
			case a.projectID != "":
				a.projectActions(c)
			}
		})
		hotIconButton(c.Key("palette"), "search", a.tr("搜索与命令", "Search and commands"), "mod+k").OnClick(func() { a.openPalette("") })
		if a.page == "settings" || a.page == "trash" {
			return
		}
		label, projects := a.tr("新建任务", "New issue"), a.issueID == "" && (a.page == "projects" || a.page == "archive")
		if projects {
			label = a.tr("新建项目", "New project")
		}
		create := accentButton(c, action.Button(c.Key("create"), "", action.ButtonProps{Variant: action.Primary, Size: theme.Small})).Height(30).PaddingX(14).Margin(0, 0, 0, 6).FontSize(13).Children(func() {
			ui.Icon(c, lucide.Plus()).FontSize(14)
			ui.Text(c, label).SingleLine()
		}).OnClick(func() {
			if projects {
				a.editProject("")
			} else {
				a.newTask("", "backlog")
			}
		})
		if !projects {
			overlay.Tooltip(c, create, label, overlay.TooltipProps{Hotkey: "c"})
		}
	})
}

// issueActions belong to the issue shown: its star, its menu and its place in
// the list it was opened from.
func (a *app) issueActions(c *ui.Context) {
	d := a.drafts[a.issueID]
	if d == nil || a.data.Issues[a.issueID].DeletedAt != 0 {
		return
	}
	switch {
	case a.err != "":
		ui.Text(c, a.tr("未保存", "Not saved")).FontSize(12).TextColor(a.tk.Error).Margin(0, 6, 0, 0)
	case d.dirty || d.busy:
		ui.Text(c, a.tr("保存中…", "Saving…")).FontSize(12).TextColor(a.tk.TextQuaternary).Margin(0, 6, 0, 0)
	}
	star := iconButton(c, "star", a.tr("收藏任务", "Favorite issue"))
	if d.issue.Favorite {
		star.TextColor(a.tk.Gold)
	}
	star.OnClick(func() { d.issue.Favorite = !d.issue.Favorite; a.touch(d, c.Now()) })
	iconButton(c, "more", a.tr("任务操作", "Issue options")).PopoverMenuButton(func(m *ui.Menu) { a.issueMenu(m, d.issue) })
	a.detailNavigation(c)
	layout.Divider(c, layout.DividerProps{Vertical: true}).Margin(0, 4)
}

// projectActions belong to the project shown.
func (a *app) projectActions(c *ui.Context) {
	p := a.data.Projects[a.projectID]
	a.projectLabelControl(c, p, false)
	star := iconButton(c, "star", a.tr("收藏项目", "Favorite project"))
	if p.Favorite {
		star.TextColor(a.tk.Gold)
	}
	star.OnClick(func() {
		a.apply(func(d *core.Data) error {
			v := d.Projects[p.ID]
			v.Favorite = !v.Favorite
			d.Projects[p.ID] = v
			return nil
		}, nil)
	})
	iconButton(c, "more", a.tr("项目操作", "Project actions")).PopoverMenuButton(func(m *ui.Menu) {
		if m.Item(a.tr("编辑项目", "Edit project")).Chosen() {
			a.editProject(p.ID)
		}
		if m.Item(a.tr("项目标签…", "Project labels…")).Chosen() {
			a.openProjectLabels()
		}
		if m.Item(a.tr("提醒我", "Remind me")).Shortcut(ui.Shift, ui.KeyH).Chosen() {
			a.remindProject(p.ID)
		}
		if m.Item(a.tr("设置开始日期…", "Set start date…")).Chosen() {
			a.chooseProjectDate(p.ID, true)
		}
		if m.Item(a.tr("设置目标日期…", "Set target date…")).Chosen() {
			a.chooseProjectDate(p.ID, false)
		}
		m.Separator()
		for _, v := range []struct{ kind, zh, en string }{{"id", "复制项目 ID", "Copy project ID"}, {"url", "复制项目链接", "Copy project URL"}, {"title", "复制项目标题", "Copy project title"}} {
			if m.Item(a.tr(v.zh, v.en)).Chosen() {
				a.copyProject(p, v.kind)
			}
		}
		m.Separator()
		label := a.tr("归档项目", "Archive project")
		if p.Archived {
			label = a.tr("取消归档", "Unarchive project")
		}
		if m.Item(label).Chosen() {
			a.apply(func(d *core.Data) error {
				v := d.Projects[p.ID]
				v.Archived = !v.Archived
				d.Projects[p.ID] = v
				return nil
			}, nil)
		}
		if m.Item(a.tr("移至回收站", "Move to trash")).Danger().Chosen() {
			a.apply(func(d *core.Data) error { d.TrashProject(p.ID); return nil }, func() { a.navigate("projects", "") })
		}
	})
	layout.Divider(c, layout.DividerProps{Vertical: true}).Margin(0, 4)
}

// sideItem is one row of the sidebar, as the nav rows of Lobe's desktop apps.
func (a *app) sideItem(c *ui.Context, active bool) ui.Element {
	b := ui.ButtonBase(c).Height(36).Padding(0, 8).Gap(6).Radius(12).FillWidth().Justify(ui.Start).Cursor(ui.CursorPointer).TextColor(a.tk.TextSecondary)
	// The page shown is a white pill lifted off the page, as the kit's
	// segmented controls mark their choice.
	switch hovered := b.Hovered(); {
	case active:
		a.lift(b.Background(a.tk.BgContainer).TextColor(a.tk.Text).FontWeight(500))
	case hovered:
		b.Background(a.tk.FillSecondary).TextColor(a.tk.Text)
	}
	return b
}

// sideMark is the icon cell every sidebar row starts with, so labels line up.
func (a *app) sideMark(c *ui.Context, mark func()) {
	ui.Row(c).Size(24, 24).Shrink(0).Center().Children(mark)
}

// sideSection heads a group of sidebar rows. Its add button shows under the
// pointer; with open it is also the way to the group's own page.
func (a *app) sideSection(c *ui.Context, title, addLabel string, active bool, open, add func()) {
	head := ui.Row(c).Height(30).Padding(0, 2, 0, 4).Margin(12, 0, 0, 0)
	hovered := head.Hovered()
	head.Children(func() {
		label := ui.ButtonBase(c).Height(24).Padding(0, 6).Gap(4).Radius(a.tk.BorderRadiusSM).FontSize(a.tk.FontSizeSM).FontWeight(500).TextColor(a.tk.TextDescription)
		if open != nil {
			label.Cursor(ui.CursorPointer).OnClick(open)
			if active {
				label.Background(a.tk.FillTertiary).TextColor(a.tk.Text)
			} else if label.Hovered() {
				label.Background(a.tk.FillTertiary).TextColor(a.tk.TextSecondary)
			}
		}
		label.Children(func() {
			ui.Text(c, title)
			if open != nil && hovered {
				ui.Icon(c, lucide.ChevronRight()).FontSize(12)
			}
		})
		ui.Spacer(c)
		reveal(iconButton(c, "plus", addLabel).Size(24, 24), hovered).OnClick(add)
	})
}
func (a *app) nav(c *ui.Context, name, label, page string) {
	active := a.page == page && a.projectID == "" && a.savedViewID == ""
	a.sideItem(c.Key("nav-"+page), active).Children(func() {
		a.sideMark(c, func() { ui.Icon(c, glyph(name)).FontSize(16) })
		ui.Text(c, label).Grow(1).SingleLine()
	}).OnClick(func() { a.navigate(page, "") })
}
func (a *app) sidebar(c *ui.Context) {
	ui.Column(c).Fill().Padding(0, 0, 8, 8).Children(func() {
		ui.Scroll(c).Grow(1).Basis(0).Gap(1).Children(func() {
			a.nav(c, "issues", a.tr("所有任务", "All issues"), "issues")
			a.nav(c, "star", a.tr("收藏", "Favorites"), "favorites")
			a.sidebarFavorites(c)
			if views := a.data.ViewList(); len(views) > 0 {
				ui.Text(c, a.tr("视图", "Views")).FontSize(a.tk.FontSizeSM).FontWeight(500).TextColor(a.tk.TextDescription).Padding(16, 10, 6, 10)
				for _, v := range views {
					a.sideItem(c.Key("saved-"+v.ID), a.savedViewID == v.ID).Label("View " + v.Name).Children(func() {
						a.sideMark(c, func() { ui.Icon(c, lucide.Layers()).FontSize(16) })
						ui.Text(c, v.Name).Grow(1).MinWidth(0).SingleLine()
					}).OnClick(func() { a.openSavedView(v.ID) }).ContextMenu(func(m *ui.Menu) {
						if m.Item(a.tr("删除视图", "Delete view")).Chosen() {
							a.apply(func(d *core.Data) error { d.RemoveSavedView(v.ID); return nil }, nil)
						}
					})
				}
			}
			a.sideSection(c, a.tr("项目", "Projects"), a.tr("新建项目", "New project"), a.page == "projects" && a.projectID == "", func() { a.navigate("projects", "") }, func() { a.editProject("") })
			for _, p := range a.data.ProjectList(false, false) {
				a.sideItem(c.Key(p.ID), a.projectID == p.ID && a.savedViewID == "").Children(func() {
					a.sideMark(c, func() { ui.Box(c).Size(8, 8).Radius(3).Background(ui.Hex(p.Color)) })
					ui.Text(c, p.Name).Grow(1).MinWidth(0).SingleLine()
				}).OnClick(func() { a.navigate("project", p.ID) })
			}
		})
		ui.Row(c).Shrink(0).Padding(4, 0, 0, 2).Gap(2).Children(func() {
			foot := func(name, label, page string) {
				b := iconButton(c.Key("foot-"+page), name, label)
				if a.page == page && a.projectID == "" {
					b.Background(a.tk.FillTertiary).TextColor(a.tk.Text)
				}
				b.OnClick(func() { a.navigate(page, "") })
			}
			ui.Box(c.Key("foot-settings-wrap")).Size(28, 28).Shrink(0).Children(func() {
				foot("gear", a.tr("设置", "Settings"), "settings")
				if a.updateHasBadge() {
					ui.Box(c.Key("update-dot")).Absolute().Top(3).Right(3).Size(6, 6).Radius(3).Background(a.tk.Primary)
				}
			})
			foot("archive", a.tr("归档项目", "Archive"), "archive")
			foot("trash", a.tr("回收站", "Trash"), "trash")
			ui.Spacer(c)
			if a.pending > 0 {
				feedback.Spin(c, feedback.SpinProps{Size: theme.Small}, nil).Margin(0, 6)
			}
			hotIconButton(c, "help", a.tr("快捷键", "Shortcut help"), "mod+/").OnClick(func() { a.helpQuery = ""; a.helpOpen = true })
		})
	})
}
func (a *app) pageTitle() string {
	if i, ok := a.data.Issues[a.issueID]; ok && i.ProjectID != "" {
		return a.data.Projects[i.ProjectID].Name
	}
	if v, ok := a.data.Views[a.savedViewID]; ok {
		return v.Name
	}
	if a.projectID != "" {
		return a.data.Projects[a.projectID].Name
	}
	switch a.page {
	case "reminders":
		return a.tr("提醒与周期", "Reminders & recurring")
	case "drafts":
		return a.tr("草稿", "Drafts")
	case "templates":
		return a.tr("模板", "Templates")
	case "projects":
		return a.tr("项目", "Projects")
	case "favorites":
		return a.tr("收藏", "Favorites")
	case "archive":
		return a.tr("归档项目", "Archive")
	case "trash":
		return a.tr("回收站", "Trash")
	case "settings":
		return a.tr("设置", "Settings")
	default:
		return a.tr("所有任务", "All issues")
	}
}
func (a *app) pageIcon() string {
	switch {
	case a.data.Issues[a.issueID].ProjectID != "":
		return "project"
	case a.savedViewID != "":
		return "layers"
	case a.projectID != "" || a.page == "projects":
		return "project"
	case a.page == "settings":
		return "gear"
	case a.page == "favorites":
		return "star"
	case a.page == "trash":
		return "trash"
	case a.page == "archive":
		return "archive"
	}
	return "issues"
}

// settingRow is one preference: its name, then its control.
func (a *app) settingRow(c *ui.Context, title string, control func()) {
	ui.Row(c.Key(title)).MinHeight(52).Padding(8, 16).Gap(16).Children(func() {
		ui.Text(c, title).Grow(1).MinWidth(0)
		control()
	})
}

// settingDetail is a settingRow with a line under its name, as a status or
// a path; warn shows the line as a problem.
func (a *app) settingDetail(c *ui.Context, title, detail string, warn bool, control func()) {
	ui.Row(c.Key(title)).MinHeight(52).Padding(8, 16).Gap(16).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			ui.Text(c, title)
			if detail != "" {
				color := a.tk.TextTertiary
				if warn {
					color = a.tk.Error
				}
				ui.Text(c, detail).FontSize(12).TextColor(color).SingleLine()
			}
		})
		control()
	})
}

// dataSettings is where the workspace lives: synced through iCloud Drive,
// kept in which folder, and backed up.
func (a *app) dataSettings(c *ui.Context) {
	var status syncStatus
	running := a.desktop != nil && a.desktop.sync != nil
	if running {
		status = a.desktop.sync.status
	}
	detail, warn := a.syncStatusText(status)
	a.settingDetail(c, a.tr("iCloud 同步", "iCloud sync"), detail, warn, func() {
		if status.Enabled && status.Folder != "" {
			iconButton(c, "folder", a.tr("在访达中显示同步文件夹", "Show sync folder in Finder")).OnClick(func() { mygo.Shell.OpenPath(status.Folder) })
		}
		on := status.Enabled
		choice.Switch(c, &on, choice.SwitchProps{}).Label(a.tr("iCloud 同步", "iCloud sync")).Disabled(!running || !status.Loaded || status.Busy).OnClick(func() {
			if status.Enabled {
				a.disableSync()
			} else {
				a.enableSync("")
			}
		})
	})
	if a.dataDir != "" {
		a.settingDetail(c, a.tr("数据位置", "Data folder"), shortPath(a.dataDir), false, func() {
			iconButton(c, "folder", a.tr("在访达中显示", "Show in Finder")).OnClick(func() { mygo.Shell.ShowItemInFolder(filepath.Join(a.dataDir, "flow.db")) })
			if os.Getenv("FLOW_DATA_DIR") == "" {
				btn(c, a.tr("更改…", "Change…")).Label(a.tr("更改数据位置…", "Change data folder…")).Disabled(a.workspaceBusy() || a.isRestoring()).OnClick(a.changeDataLocation)
			}
		})
	}
	a.settingRow(c, a.tr("备份", "Backup"), func() {
		btn(c, a.tr("导出…", "Export…")).Label(a.tr("导出备份…", "Export backup…")).Disabled(a.workspaceBusy()).OnClick(a.backup)
		btn(c, a.tr("恢复…", "Restore…")).Label(a.tr("恢复备份…", "Restore backup…")).Disabled(a.workspaceBusy()).OnClick(a.restore)
	})
}

// shortPath writes a path under the home folder from ~, and iCloud Drive's
// folder by name.
func shortPath(path string) string {
	if drive := iCloudDrive(); drive != "" && within(drive, path) {
		rel, _ := filepath.Rel(drive, path)
		return filepath.Join("iCloud Drive", rel)
	}
	if home, err := os.UserHomeDir(); err == nil && within(home, path) {
		rel, _ := filepath.Rel(home, path)
		return filepath.Join("~", rel)
	}
	return path
}

func (a *app) settingsCard(c *ui.Context, rows func()) {
	a.card(ui.Column(c)).Dividers(1, a.tk.BorderSecondary).Children(rows)
}
func (a *app) settingsView(c *ui.Context) {
	ui.Scroll(c).Grow(1).Padding(32, 40, 48, 40).Children(func() {
		ui.Column(c).FillWidth().MaxWidth(640).Margin(0, ui.Auto).Gap(20).Children(func() {
			a.settingsCard(c, func() {
				a.appearanceSettings(c)
				a.settingRow(c, a.tr("语言", "Language"), func() {
					tags := []string{"zh-Hans", "en"}
					current := 0
					if a.data.Preferences.Locale == "en" {
						current = 1
					}
					a.segmented(c, "locale", []string{"简体中文", "English"}, current, false, func(n int) {
						tag := tags[n]
						a.apply(func(d *core.Data) error { d.Preferences.Locale = tag; return nil }, func() {
							if a.window != nil {
								mygo.App.SetLocale(tag)
							}
						})
					})
				})
			})
			a.settingsCard(c, func() { a.dataSettings(c) })
			a.settingsCard(c, func() { a.mcpSettings(c) })
			a.settingsCard(c, func() { a.updateSettings(c) })
			a.labelsManager(c)
			ui.Text(c, "Flow "+a.currentAppVersion()).FontSize(12).TextColor(a.tk.TextQuaternary).Padding(0, 4)
		})
	})
}
