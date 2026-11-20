package main

import (
	"errors"
	"flow/internal/core"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestAppearanceSettingsPersistAndFollowSystem(t *testing.T) {
	a := testApp()
	a.dataDir = t.TempDir()
	a.restoreDesktopSession(desktopSession{}, false, nil)
	a.navigate("settings", "")
	tt := ui.NewTester(a.view, 1120, 880)
	click(t, tt, "Light")
	if a.theme.Dark || a.appearanceMode() != "light" {
		t.Fatal("light appearance not applied")
	}
	capture(t, tt, "34-light-settings")
	a.navigate("issues", "")
	tt.Frame()
	capture(t, tt, "34-light-issues")
	a.newTask("", "backlog")
	tt.Frame()
	capture(t, tt, "34-light-create")
	a.closeComposition()
	a.navigate("settings", "")
	tt.Frame()
	tt.Advance(time.Second)
	session, exists, err := readDesktopSession(a.dataDir)
	if err != nil || !exists || session.Appearance != "light" {
		t.Fatal("appearance not persisted", session.Appearance, err)
	}
	b := newApp(a.data)
	b.dataDir = a.dataDir
	b.restoreDesktopSession(session, exists, err)
	tb := ui.NewTester(b.view, 1120, 880)
	if b.theme.Dark {
		t.Fatal("restart lost light theme")
	}
	click(t, tb, "System")
	tb.Advance(time.Second) // the segmented control slides its indicator to the choice
	tb.SetDark(true)
	if !b.theme.Dark {
		t.Fatal("system dark not followed")
	}
	tb.SetDark(false)
	if b.theme.Dark {
		t.Fatal("system light not followed")
	}
	click(t, tb, "Dark")
	tb.Advance(time.Second)
	tb.SetDark(false)
	if !b.theme.Dark {
		t.Fatal("forced dark followed system")
	}
	click(t, tb, "Light")
	tb.Advance(time.Second)
	tb.SetDark(true)
	if b.theme.Dark {
		t.Fatal("forced light followed system")
	}
	b.setAppearance("invalid")
	if b.appearanceMode() != "light" {
		t.Fatal("invalid appearance accepted")
	}
}

func TestWindowCloseDrainsLaterDraftEditsAndRetainsRoute(t *testing.T) {
	a := testApp()
	a.newTask("", "backlog")
	a.newIssue.Title = "close with draft"
	a.newDoc.InsertText("first")
	s := discussionAsyncStore(t, a)
	var closed []bool
	a.closeAction = func(quit bool) { closed = append(closed, quit) }
	a.requestWindowClose()
	if len(closed) != 0 || !a.closeRequested || a.pending == 0 {
		t.Fatal("closed before saving")
	}
	a.newDoc.InsertText(" later")
	finishDiscussionJob(t, a, s)
	if len(closed) != 0 || a.pending == 0 {
		t.Fatal("later edits not drained before close")
	}
	finishDiscussionJob(t, a, s)
	if len(closed) != 1 || closed[0] || a.closing() || a.hasDirty() || a.newDoc.Text().PlainText() != "first later" {
		t.Fatal("close discarded in-memory state or quit", closed, a.err)
	}
	if s.Snapshot().Drafts[a.composeID].Form.Plain != "first later" {
		t.Fatal("latest text not saved")
	}
	// A second close with no writes is immediate and still requests hiding.
	a.requestWindowClose()
	if len(closed) != 2 || closed[1] {
		t.Fatal("clean close did not hide")
	}
	a.quitRequested = true
	a.finishQuit()
	if len(closed) != 3 || !closed[2] {
		t.Fatal("explicit quit did not exit")
	}
}

func TestWindowCloseSaveFailureKeepsWindowAndDraft(t *testing.T) {
	a := testApp()
	a.newTask("", "backlog")
	a.newIssue.Title = "save failure"
	a.newDoc.InsertText("must survive")
	s := discussionAsyncStore(t, a)
	closed := 0
	a.closeAction = func(bool) { closed++ }
	a.requestWindowClose()
	work := <-a.jobs
	work.done(core.Data{}, errors.New("write failed"))
	if closed != 0 || a.closing() || a.newDoc.Text().PlainText() != "must survive" || !a.hasDirty() {
		t.Fatal("failed save hid window or discarded draft")
	}
	a.err = ""
	a.requestWindowClose()
	finishDiscussionJob(t, a, s)
	if closed != 1 || a.hasDirty() {
		t.Fatal("retry close failed", a.err)
	}
}
