package main

import (
	"fmt"

	"github.com/egoist/mygo"
)

func (a *app) navigationActionsEnabled() bool {
	return a.ready && !a.isRestoring() && !a.helpOpen && !a.peekOpen && !a.childOpen && a.data.Preferences.Initialized && !a.newOpen && !a.projectOpen && !a.paletteOpen && !a.date.open && !a.displayOpen && !a.filterOpen && !a.saveViewOpen && !a.templateOpen && !a.relationOpen && !a.historyOpen && !a.attachmentOpen && !a.reminderOpen && !a.scheduleOpen && !a.labelOpen && !a.folderOpen && !a.milestoneOpen && !a.documentOpen && !a.updateOpen && !a.eraseOpen && !a.appUpdateOpen && (a.activePicker == nil || !a.activePicker.open)
}

func (a *app) desktopMenuItems() []*mygo.MenuItem {
	item := func(id, label, shortcut string, navigation bool, action func(*app)) *mygo.MenuItem {
		enabled := a.navigationActionsEnabled()
		if !navigation {
			enabled = a.desktop != nil && a.desktop.canOpenWindow()
		}
		return &mygo.MenuItem{ID: id, Label: label, Accelerator: shortcut, Disabled: !enabled, Click: func(_ *mygo.MenuItem, window *mygo.Window) {
			target := a
			if a.desktop != nil {
				target = a.desktop.commandTarget(window)
			}
			if target == nil {
				return
			}
			run := func() {
				if navigation && !target.navigationActionsEnabled() || !navigation && (target.desktop == nil || !target.desktop.canOpenWindow()) {
					return
				}
				action(target)
				if navigation && target.window != nil {
					target.window.Show()
				}
			}
			if target.window != nil {
				target.window.Update(run)
			} else {
				run()
			}
		}}
	}
	file := []*mygo.MenuItem{
		item("new-issue", a.tr("新建任务", "New issue"), "CmdOrCtrl+N", true, func(target *app) { target.newTask("", "backlog") }),
	}
	if a.desktop != nil {
		file = append(file,
			item("new-window", a.tr("新建窗口", "New window"), "CmdOrCtrl+Shift+N", false, func(target *app) { target.desktop.newWindow(nil) }),
			item("open-page-window", a.tr("在新窗口打开当前页面", "Open page in new window"), "", false, func(target *app) { target.openPageInWindow() }))
	}
	file = append(file, &mygo.MenuItem{Role: mygo.RoleClose, Label: a.tr("关闭窗口", "Close window"), Accelerator: "CmdOrCtrl+W"})
	checkLabel := a.tr("检查更新…", "Check for Updates…")
	updateReadyNow := false
	if st, ok := a.currentUpdateStatus(); ok && st.Phase == updateReady {
		checkLabel = fmt.Sprintf(a.tr("重启以更新到 v%s", "Restart to Update to v%s"), st.Version)
		updateReadyNow = true
	}
	checkItem := &mygo.MenuItem{
		ID:       "check-updates",
		Label:    checkLabel,
		Disabled: !a.ready || a.isRestoring(),
		Click: func(_ *mygo.MenuItem, window *mygo.Window) {
			target := a
			if a.desktop != nil {
				target = a.desktop.commandTarget(window)
			}
			if target == nil {
				return
			}
			run := func() {
				if !target.ready || target.isRestoring() {
					return
				}
				if updateReadyNow && target.desktop != nil && target.desktop.updater != nil && target.desktop.updater.snapshot().Phase == updateReady {
					target.desktop.updater.relaunch()
					return
				}
				target.triggerUpdateCheck()
				if target.window != nil {
					target.window.Show()
				}
			}
			if target.window != nil {
				target.window.Update(run)
			} else {
				run()
			}
		},
	}
	appMenu := &mygo.MenuItem{Label: "Flow", Submenu: []*mygo.MenuItem{
		{Role: mygo.RoleAbout},
		checkItem,
		mygo.Separator(),
		{Role: mygo.RoleServices},
		mygo.Separator(),
		{Role: mygo.RoleHide},
		{Role: mygo.RoleHideOthers},
		{Role: mygo.RoleUnhide},
		mygo.Separator(),
		{Role: mygo.RoleQuit},
	}}
	out := []*mygo.MenuItem{
		appMenu,
		{Label: a.tr("文件", "File"), Submenu: file},
		{Role: mygo.RoleEditMenu}, {Role: mygo.RoleViewMenu}, {Role: mygo.RoleWindowMenu},
	}
	if a.desktop != nil {
		var windows []*mygo.MenuItem
		for _, target := range a.desktop.apps {
			label := "Flow"
			if target.windowID != "main" {
				label += " — " + target.windowID
			}
			if !target.windowVisible {
				label += a.tr("（已关闭）", " (closed)")
			}
			windows = append(windows, &mygo.MenuItem{ID: "show-window/" + target.windowID, Label: label, Click: func(_ *mygo.MenuItem, _ *mygo.Window) { a.desktop.show(target) }})
		}
		out = append(out, &mygo.MenuItem{Label: a.tr("打开的窗口", "Open windows"), Submenu: windows})
	}
	return append(out, &mygo.MenuItem{Label: a.tr("帮助", "Help"), Submenu: []*mygo.MenuItem{
		item("shortcut-help", a.tr("键盘快捷键", "Keyboard shortcuts"), "CmdOrCtrl+/", true, func(target *app) { target.helpQuery = ""; target.helpOpen = true }),
	}})
}

func (a *app) refreshDesktopMenu() {
	if a.window == nil {
		return
	}
	updateKey := ""
	if st, ok := a.currentUpdateStatus(); ok {
		updateKey = fmt.Sprintf("/%d/%s", st.Phase, st.Version)
	}
	key := fmt.Sprintf("%s/%t%s", a.data.Preferences.Locale, a.navigationActionsEnabled(), updateKey)
	if a.desktop != nil {
		if a.desktop.active != a {
			return // A background redraw must not replace the focused menu.
		}
		key += "/" + a.windowID + "/" + a.desktop.fingerprint() + fmt.Sprint(a.desktop.canOpenWindow())
		if a.desktop.menuKey == key {
			return
		}
		a.desktop.menuKey = key
	} else {
		if a.desktopMenuKey == key {
			return
		}
		a.desktopMenuKey = key
	}
	mygo.App.SetMenu(mygo.NewMenu(a.desktopMenuItems()))
}
