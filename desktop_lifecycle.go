package main

import "github.com/egoist/mygo"

func (a *app) closing() bool { return a.quitRequested || a.closeRequested }

func (a *app) cancelClosing() {
	wasClosing := a.closing()
	a.quitRequested, a.closeRequested = false, false
	if a.desktop != nil {
		a.desktop.cancelQuit()
		if wasClosing && !a.windowVisible {
			a.desktop.show(a)
		}
	}
}

func (a *app) requestWindowClose() {
	if !a.quitRequested {
		a.closeRequested = true
	}
	a.flushDrafts()
	a.finishQuit()
}

func (a *app) finishQuit() {
	if a.desktop != nil && a.desktop.quitRequested {
		a.desktop.finishQuit()
		return
	}
	if !a.closing() || a.pending > 0 || a.hasDirty() {
		return
	}
	quit := a.quitRequested
	a.closeRequested = false
	if a.desktop != nil {
		a.desktop.hide(a)
		return
	}
	if a.closeAction != nil {
		a.closeAction(quit)
	} else if a.window != nil {
		if quit {
			mygo.App.Quit()
		} else {
			a.window.Hide()
		}
	}
}
