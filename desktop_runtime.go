package main

import (
	"flow/internal/core"
	"path/filepath"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func (d *desktop) createNativeWindow(id string, hidden bool, initial *route) *app {
	a := newApp(core.Empty())
	a.ready = false
	d.attach(a, id, !hidden)
	d.loading++
	title, stateKey := "Flow", "main"
	if id != "main" {
		title, stateKey = "Flow — "+id, "window-"+id
	}
	win := mygo.NewWindow(mygo.WindowOptions{Title: title, Width: 1360, Height: 880, MinWidth: 1000, MinHeight: 680, Hidden: true, TitleBarStyle: mygo.TitleBarHiddenInset, StateKey: stateKey, Content: ui.View(a.view)})
	a.window = win
	win.OnFocus(func() { d.focus(a) })
	win.OnClose(func(e *mygo.CloseEvent) {
		if d.committingQuit {
			return
		}
		e.PreventDefault()
		a.requestWindowClose()
	})
	d.startWorker(a, initial)
	return a
}

func (d *desktop) bootstrap(state desktopWindowsState, exists bool, err error) {
	main := d.apps[0]
	d.appearance = main.appearanceMode()
	if err != nil {
		d.disabled = true
		main.notice = main.tr("未能恢复窗口清单，原文件已保留：", "Could not restore windows; original file kept: ") + err.Error()
	} else if exists {
		d.next, d.appearance, d.restoreActive = state.Next, state.Appearance, state.Active
		for _, w := range state.Windows {
			if w.ID == "main" {
				main.windowVisible = w.Visible
			} else {
				d.createWindow(w.ID, !w.Visible, nil)
			}
		}
	}
	main.appearance = d.appearance
	main.applyNativeAppearance()
	d.loaded = true
}

func (d *desktop) windowLoaded(a *app) {
	d.loading--
	if a.windowVisible && a.window != nil {
		a.window.Show()
	}
	if d.loading == 0 {
		if target := d.find(d.restoreActive); target != nil {
			d.active = target
		}
		d.restoreActive = ""
		visible := false
		for _, other := range d.apps {
			visible = visible || other.windowVisible
		}
		if !visible || d.active.windowVisible {
			d.show(d.active)
		} else {
			for _, other := range d.apps {
				if other.windowVisible {
					d.show(other)
					break
				}
			}
		}
		d.persist()
		if d.missingData != "" && d.active != nil {
			a := d.active
			a.err = a.tr("找不到数据文件夹 ", "Data folder not found: ") + d.missingData + a.tr("，已打开默认位置。接上它所在的磁盘后重新打开 Flow。", ". Flow opened the default folder; connect the disk it is on and reopen Flow.")
			d.missingData = ""
		}
		d.startSync()
		d.startUpdater()
	}
	d.invalidate()
	d.finishQuit()
}

func (d *desktop) startWorker(a *app, initial *route) {
	win, jobs, dir, sessionDir, primary := a.window, a.jobs, d.dataDir, a.desktopDirectory(), a.windowID == "main"
	workerDone := make(chan struct{})
	mygo.App.OnQuit(func() { close(jobs); <-workerDone })
	go func() {
		defer close(workerDone)
		s, openErr := core.Open(filepath.Join(dir, "flow.db"))
		if openErr != nil {
			win.Update(func() {
				a.err = openErr.Error()
				if primary {
					d.loaded, d.disabled = true, true
				}
				d.windowLoaded(a)
			})
			// Keep draining so an inaccessible workspace cannot strand quit
			// on a queue whose worker already exited.
			for j := range jobs {
				win.Update(func() { j.done(core.Data{}, openErr) })
			}
			return
		}
		defer s.Close()
		session, sessionExists, sessionErr := readDesktopSession(sessionDir)
		var windows desktopWindowsState
		var windowsExist bool
		var windowsErr error
		if primary {
			windows, windowsExist, windowsErr = readDesktopWindows(dir)
		}
		data, fired, scheduleErr := s.Snapshot(), []string(nil), error(nil)
		if primary {
			data, fired, scheduleErr = runStoreSchedule(s, time.Now())
			if scheduleErr != nil {
				data = s.Snapshot()
			}
		}
		win.Update(func() {
			a.data = data
			a.loadViewConfig()
			a.restoreDesktopSession(session, sessionExists, sessionErr)
			a.ready = true
			if initial != nil {
				a.restoreRoute(*initial)
			}
			if primary {
				d.bootstrap(windows, windowsExist, windowsErr)
			}
			if scheduleErr != nil {
				a.err = scheduleErr.Error()
			} else {
				a.deliverReminders(fired)
			}
			if a.pendingLink != "" {
				if err := a.openLink(a.pendingLink); err != nil {
					a.err = err.Error()
				}
				a.pendingLink = ""
			}
			mygo.App.SetLocale(data.Preferences.Locale)
			d.windowLoaded(a)
		})
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case j, ok := <-jobs:
				if !ok {
					return
				}
				data, err := j.run(s)
				win.Update(func() { j.done(data, err) })
			case <-ticker.C:
				data, changed, err := s.Refresh()
				var fired []string
				if err == nil && primary && scheduledDue(data, time.Now()) {
					data, fired, err = runStoreSchedule(s, time.Now())
					changed = true
				}
				if err != nil {
					win.Update(func() { a.err = err.Error() })
				} else if changed {
					win.Update(func() { a.acceptExternal(data); a.deliverReminders(fired) })
				}
			}
		}
	}()
}
