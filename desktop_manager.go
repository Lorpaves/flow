package main

import (
	"encoding/json"
	"flow/internal/core"
	"path/filepath"
	"strconv"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// All desktop state is main-thread owned. Each app owns its editing objects,
// job queue, SQLite connection and navigation file; only workspace records are
// shared. Window identities survive restart, including their creation drafts.
type desktop struct {
	apps                                []*app
	active                              *app
	dataDir, appearance, saved, menuKey string
	next, loading                       int
	loaded, disabled, busy, restoring   bool
	quitRequested, flushing, quitSent   bool
	relaunchRequested                   bool
	committingQuit                      bool
	createWindow                        func(string, bool, *route) *app
	quitAction                          func()
	relaunchAction                      func()
	restoreActive                       string
	sync                                *syncService
	updater                             *updateService
	mcp                                 *mcpService
	// missingData names a data folder Flow was moved to that is not there
	// now; the default folder is open instead.
	missingData string
}

func (d *desktop) attach(a *app, id string, visible bool) {
	a.desktop, a.windowID, a.windowVisible, a.dataDir = d, id, visible, d.dataDir
	if validAppearance(d.appearance) {
		a.appearance = d.appearance
	}
	d.apps = append(d.apps, a)
	if d.active == nil {
		d.active = a
	}
}

func (d *desktop) find(id string) *app {
	for _, a := range d.apps {
		if a.windowID == id {
			return a
		}
	}
	return nil
}

func (a *app) desktopDirectory() string {
	if a.windowID == "" || a.windowID == "main" {
		return a.dataDir
	}
	return filepath.Join(a.dataDir, "windows", a.windowID)
}

func (a *app) creationContext(context string) string {
	if a.windowID == "" || a.windowID == "main" {
		return context // Existing single-window drafts retain their owner.
	}
	return "window/" + a.windowID + "/" + context
}

func (a *app) isRestoring() bool {
	return a.restoring || a.desktop != nil && a.desktop.restoring
}

func (a *app) workspaceBusy() bool {
	if a.desktop == nil {
		return a.pending > 0 || a.hasDirty()
	}
	for _, other := range a.desktop.apps {
		if !other.ready || other.pending > 0 || other.hasDirty() {
			return true
		}
	}
	return a.desktop.loading > 0
}

func (d *desktop) canOpenWindow() bool {
	return d.loaded && !d.disabled && !d.quitRequested && !d.restoring && d.loading == 0 && len(d.apps) < 1000 && d.next < 1_000_000_000
}

func (d *desktop) newWindow(r *route) {
	if !d.canOpenWindow() || d.createWindow == nil {
		return
	}
	d.next++
	a := d.createWindow(strconv.Itoa(d.next), false, r)
	d.active = a
	d.persist()
	d.invalidate()
}

// Duplicate only navigation data, never widget handles, selections or editors.
func (a *app) openPageInWindow() {
	if a.desktop == nil {
		return
	}
	r := a.currentRoute()
	r.List, r.Selected, r.Order, r.Return = ui.ListState{}, nil, nil, nil
	a.desktop.newWindow(&r)
}

func (d *desktop) focus(a *app) {
	if d.committingQuit {
		return
	}
	d.active = a
	d.menuKey = ""
	a.refreshDesktopMenu()
	d.persist()
}

func (d *desktop) show(a *app) {
	if a == nil || d.committingQuit {
		return
	}
	a.windowVisible = true
	if a.window != nil {
		a.window.Show()
	}
	d.focus(a)
}

func (d *desktop) hide(a *app) {
	a.windowVisible = false
	if a.window != nil {
		a.window.Hide()
	}
	d.persist()
	d.invalidate()
}

func (d *desktop) invalidate() {
	d.menuKey = ""
	for _, a := range d.apps {
		if a.window != nil {
			a.window.Invalidate()
		}
	}
}

func (d *desktop) setAppearance(mode string) {
	if !validAppearance(mode) {
		return
	}
	d.appearance = mode
	for _, a := range d.apps {
		a.appearance = mode
		a.applyNativeAppearance()
		a.persistDesktopSession()
	}
	d.persist()
	d.invalidate()
}

func (d *desktop) fingerprint() string {
	s := desktopWindowsState{Version: 1, Next: d.next, Appearance: d.appearance}
	if d.active != nil {
		s.Active = d.active.windowID
	}
	for _, a := range d.apps {
		s.Windows = append(s.Windows, desktopWindowState{ID: a.windowID, Visible: a.windowVisible})
	}
	b, _ := json.Marshal(s)
	return string(b)
}

func (d *desktop) dirty() bool {
	return d.loaded && !d.disabled && d.fingerprint() != d.saved
}

func (d *desktop) persist() {
	if d.committingQuit || d.busy || !d.dirty() || len(d.apps) == 0 {
		return
	}
	raw, dir := d.fingerprint(), d.dataDir
	d.busy = true
	done := func(_ core.Data, err error) {
		d.busy = false
		if err != nil {
			d.cancelQuit()
			d.active.err = err.Error()
			d.invalidate()
			return
		}
		d.saved = raw
		d.persist() // Coalesce focus/visibility changes made during the write.
		d.finishQuit()
	}
	if d.apps[0].window == nil {
		done(core.Data{}, writeDesktopWindows(dir, []byte(raw)))
		return
	}
	d.apps[0].jobs <- job{run: func(_ *core.Store) (core.Data, error) {
		return core.Data{}, writeDesktopWindows(dir, []byte(raw))
	}, done: done}
}

func (d *desktop) cancelQuit() {
	d.quitRequested, d.quitSent, d.committingQuit, d.relaunchRequested = false, false, false, false
	for _, a := range d.apps {
		a.quitRequested, a.closeRequested = false, false
	}
}

func (d *desktop) readyToQuit() bool {
	if d.loading > 0 || d.busy || d.dirty() || d.restoring {
		return false
	}
	for _, a := range d.apps {
		if a.pending > 0 || a.hasDirty() {
			return false
		}
	}
	return true
}

// Returns true only when every window is drained. One save failure cancels the
// whole quit, so a clean window can never terminate another window's write.
func (d *desktop) requestQuit() bool {
	d.quitSent = false
	d.quitRequested, d.flushing = true, true
	for _, a := range d.apps {
		a.closeRequested, a.quitRequested = false, true
		a.flushDrafts()
		if !d.quitRequested {
			break
		}
	}
	d.persist()
	d.flushing = false
	d.committingQuit = d.quitRequested && d.readyToQuit()
	return d.committingQuit
}

func (d *desktop) requestRelaunch() {
	d.relaunchRequested = true
	if d.relaunchAction != nil {
		d.relaunchAction()
	} else {
		mygo.App.Relaunch()
	}
}

func (d *desktop) finishQuit() {
	if !d.quitRequested || d.flushing || d.quitSent || !d.readyToQuit() {
		return
	}
	d.quitSent = true
	if d.relaunchRequested {
		if d.relaunchAction != nil {
			d.relaunchAction()
		} else {
			mygo.App.Relaunch()
		}
		return
	}
	if d.quitAction != nil {
		d.quitAction()
	} else {
		mygo.App.Quit()
	}
}

func (d *desktop) commandTarget(window *mygo.Window) *app {
	if window != nil {
		for _, a := range d.apps {
			if a.window == window {
				return a
			}
		}
	}
	return d.active
}
