package main

import (
	"encoding/json"
	"errors"
	"flow/internal/core"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func managedTestDesktop(t *testing.T) (*desktop, *app, *app, []*core.Store) {
	t.Helper()
	d := &desktop{dataDir: t.TempDir(), loaded: true, appearance: "dark", next: 1}
	a := testApp()
	d.attach(a, "main", true)
	b := newApp(a.data.Clone())
	d.attach(b, "1", true)
	stores := make([]*core.Store, 2)
	for n := range stores {
		s, err := core.Open(filepath.Join(d.dataDir, "flow.db"))
		if err != nil {
			t.Fatal(err)
		}
		stores[n] = s
		t.Cleanup(func() { s.Close() })
	}
	if _, err := stores[0].Update(func(data *core.Data) error { *data = a.data.Clone(); return nil }); err != nil {
		t.Fatal(err)
	}
	for _, a := range d.apps {
		a.restoreDesktopSession(desktopSession{}, false, nil)
	}
	d.saved = d.fingerprint()
	return d, a, b, stores
}

func drainDesktop(t *testing.T, d *desktop, stores []*core.Store) {
	t.Helper()
	for count := 0; count < 1000; count++ {
		worked := false
		for n, a := range d.apps {
			select {
			case j := <-a.jobs:
				data, err := j.run(stores[n])
				j.done(data, err)
				if err != nil {
					t.Fatal(err)
				}
				worked = true
			default:
			}
		}
		if !worked {
			return
		}
	}
	t.Fatal("desktop queues did not drain")
}

func TestMultipleWindowsKeepIndependentEditorsAndRestartDrafts(t *testing.T) {
	d, a, b, stores := managedTestDesktop(t)
	project := a.data.ProjectList(false, false)[0].ID
	for _, a := range d.apps {
		a.navigate("project", project)
		a.newTask("", "backlog")
	}
	ta, tb := ui.NewTester(a.view, 1100, 780), ui.NewTester(b.view, 1100, 780)
	click(t, ta, "New issue title")
	ta.Type("第一窗口草稿")
	click(t, tb, "New issue title")
	tb.Type("第二窗口草稿")
	click(t, ta, "New issue description")
	ta.Type("first body")
	click(t, tb, "New issue description")
	tb.Type("second body")
	if a.composeID == b.composeID || a.composeKey == b.composeKey || a.newDoc == b.newDoc || !strings.HasPrefix(b.composeKey, "window/1/") {
		t.Fatal("windows share creation identity or editor")
	}
	if a.newIssue.Title != "第一窗口草稿" || b.newIssue.Title != "第二窗口草稿" {
		t.Fatal("focus crossed windows")
	}
	// Separate standalone renders from the interleaved focus/typing checks.
	capture(t, ui.NewTester(a.view, 1100, 780), "35-window-one-create")
	capture(t, ui.NewTester(b.view, 1100, 780), "35-window-two-create")
	// The real desktop sends these saves to independent SQLite connections.
	a.window, b.window = &mygo.Window{}, &mygo.Window{}
	a.flushDrafts()
	b.flushDrafts()
	drainDesktop(t, d, stores)
	fresh, _, err := stores[0].Refresh()
	if err != nil {
		t.Fatal(err)
	}
	if fresh.Drafts[a.composeID].Form.Title != a.newIssue.Title || fresh.Drafts[b.composeID].Form.Title != b.newIssue.Title {
		t.Fatal("one window overwrote the other draft")
	}
	// Routes and appearance use device files, independently of workspace data.
	a.window, b.window = nil, nil
	a.closeComposition()
	b.closeComposition()
	a.navigate("settings", "")
	b.navigate("project", project)
	b.projectTab = "overview"
	a.sidebarWidth, b.sidebarWidth = 201, 370
	d.setAppearance("light")
	a.persistDesktopSession()
	b.persistDesktopSession()
	d.hide(a)
	d.focus(b)
	state, exists, err := readDesktopWindows(d.dataDir)
	if err != nil || !exists || state.Active != "1" || state.Windows[0].Visible || state.Appearance != "light" {
		t.Fatal("window index lost identity/visibility/theme", state, err)
	}
	for n, old := range []*app{a, b} {
		restarted := newApp(fresh.Clone())
		restarted.windowID, restarted.dataDir = old.windowID, d.dataDir
		session, exists, err := readDesktopSession(restarted.desktopDirectory())
		restarted.restoreDesktopSession(session, exists, err)
		if err != nil || !exists || restarted.sidebarWidth != old.sidebarWidth || restarted.page != old.page || restarted.projectTab != old.projectTab || restarted.appearanceMode() != "light" {
			t.Fatal("per-window session mixed", n, err)
		}
		restarted.navigate("project", project)
		restarted.newTask("", "backlog")
		if restarted.composeID != old.composeID || restarted.newIssue.Title != old.newIssue.Title {
			t.Fatal("window resumed another window's draft", n)
		}
	}
}

func TestMultipleWindowIssueEditsMergeWithRemoteStore(t *testing.T) {
	d, a, b, stores := managedTestDesktop(t)
	id := a.filtered()[0].ID
	for _, a := range d.apps {
		a.openIssue(id)
		a.window = &mygo.Window{}
	}
	a.drafts[id].issue.Title = "title from window one"
	a.touch(a.drafts[id], time.Now())
	b.drafts[id].doc.InsertText("body from window two")
	b.touch(b.drafts[id], time.Now())
	remote, err := core.Open(filepath.Join(d.dataDir, "flow.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer remote.Close()
	if _, err = remote.Update(func(data *core.Data) error {
		i := data.Issues[id]
		i.Priority = 1
		i.Favorite = true
		return data.PutIssue(i)
	}); err != nil {
		t.Fatal(err)
	}
	a.saveDraft(id)
	b.saveDraft(id)
	jobs := []job{<-a.jobs, <-b.jobs}
	results := make([]core.Data, 2)
	errs := make([]error, 2)
	var wg sync.WaitGroup
	for n, j := range jobs {
		wg.Go(func() { results[n], errs[n] = j.run(stores[n]) })
	}
	wg.Wait()
	for n, j := range jobs {
		j.done(results[n], errs[n])
		if errs[n] != nil {
			t.Fatal(errs[n])
		}
	}
	for n, a := range d.apps {
		data, _, err := stores[n].Refresh()
		if err != nil {
			t.Fatal(err)
		}
		a.acceptExternal(data)
		i := a.data.Issues[id]
		if i.Title != "title from window one" || !strings.Contains(i.Plain, "body from window two") || i.Priority != 1 || !i.Favorite {
			t.Fatal("lost another editor or remote field", i)
		}
		if a.drafts[id].issue.Title != i.Title || a.drafts[id].doc.Text().PlainText() != i.Plain {
			t.Fatal("clean editor did not refresh")
		}
	}
}

func TestDesktopQuitWaitsForEveryWindowAndCancelsOnOneFailure(t *testing.T) {
	d, a, b, stores := managedTestDesktop(t)
	quit := 0
	d.quitAction = func() { quit++ }
	for n, a := range d.apps {
		a.newTask("", "backlog")
		a.newIssue.Title = []string{"first", "second"}[n]
		a.newDoc.InsertText("saved content")
		a.window = &mygo.Window{}
	}
	if d.requestQuit() || quit != 0 {
		t.Fatal("quit before writes completed")
	}
	finishDiscussionJob(t, a, stores[0])
	if quit != 0 || !d.quitRequested {
		t.Fatal("one clean window terminated the others")
	}
	failing := <-b.jobs
	failing.done(core.Data{}, errors.New("test disk failure"))
	if d.quitRequested || a.quitRequested || b.quitRequested || quit != 0 || !b.hasDirty() {
		t.Fatal("failure did not cancel whole quit")
	}
	b.newDoc.InsertText(" later typing")
	if d.requestQuit() {
		t.Fatal("retry quit bypassed dirty second window")
	}
	b.newDoc.InsertText(" during pending quit")
	drainDesktop(t, d, stores)
	if quit != 1 || !d.readyToQuit() {
		t.Fatal("all-window retry did not drain", quit)
	}
	data, _, err := stores[1].Refresh()
	if err != nil || !strings.HasSuffix(data.Drafts[b.composeID].Form.Plain, "later typing during pending quit") {
		t.Fatal("quit dropped last edit", err)
	}
	// An edit arriving after Quit was scheduled must allow another drain cycle.
	b.newDoc.InsertText(" after scheduling")
	if d.requestQuit() {
		t.Fatal("scheduled quit ignored new dirty data")
	}
	drainDesktop(t, d, stores)
	if quit != 2 {
		t.Fatal("quit was never rescheduled after a late edit")
	}
}

func TestWindowMenuDispatchAndNewWindowDuringEditing(t *testing.T) {
	d, a, b, _ := managedTestDesktop(t)
	d.createWindow = func(id string, hidden bool, r *route) *app {
		c := newApp(a.data.Clone())
		d.attach(c, id, !hidden)
		c.restoreDesktopSession(desktopSession{}, false, nil)
		if r != nil {
			c.restoreRoute(*r)
		}
		return c
	}
	var create, window *mygo.MenuItem
	for _, top := range a.desktopMenuItems() {
		for _, item := range top.Submenu {
			if item.ID == "new-issue" {
				create = item
			}
			if item.ID == "new-window" {
				window = item
			}
		}
	}
	a.helpOpen = true
	d.active = b // An already-created menu callback must use the focused app.
	create.Click(nil, nil)
	if !b.newOpen || a.newOpen || !a.helpOpen {
		t.Fatal("menu used a stale window")
	}
	window.Click(nil, nil)
	if len(d.apps) != 3 || !b.newOpen {
		t.Fatal("new window destroyed the active editor")
	}
	c := d.apps[2]
	id := a.filtered()[0].ID
	c.openIssue(id)
	c.selected[id] = true
	c.openPageInWindow()
	copy := d.apps[3]
	if copy.issueID != id || len(copy.selected) != 0 || copy.drafts[id].doc == c.drafts[id].doc {
		t.Fatal("page copy shared runtime/editor state")
	}
	d.restoring = true
	if a.navigationActionsEnabled() || b.navigationActionsEnabled() || d.canOpenWindow() {
		t.Fatal("restore is not exclusive across windows")
	}
	d.restoring = false
	b.newIssue.Title = "unsaved in other window"
	if !a.workspaceBusy() {
		t.Fatal("restore ignored another window's dirty state")
	}
}

func TestWindowIndexValidationAndFailedWritePreservesPreviousFile(t *testing.T) {
	d, a, _, _ := managedTestDesktop(t)
	d.persist() // unchanged; explicitly establish an index below
	d.saved = ""
	d.persist()
	file := filepath.Join(d.dataDir, "desktop-windows.json")
	valid, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	d.dataDir = file // Force a write failure without changing the valid file.
	d.appearance = "light"
	d.quitRequested = true
	d.persist()
	got, _ := os.ReadFile(file)
	if string(got) != string(valid) || d.quitRequested || !d.dirty() || a.err == "" {
		t.Fatal("index failure lost old file or quit anyway")
	}
	d.dataDir = filepath.Dir(file)
	invalid := []string{"{broken", `{"version":99,"windows":[]}`, `{"version":1,"active":"main","appearance":"dark","windows":[{"id":"main"},{"id":"../escape"}]}`, `{"version":1,"active":"main","appearance":"dark","windows":[{"id":"main"},{"id":"main"}]}`}
	for _, raw := range invalid {
		if err := os.WriteFile(file, []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
		s, exists, err := readDesktopWindows(d.dataDir)
		if err == nil || exists {
			t.Fatal("invalid index accepted")
		}
		fresh := &desktop{dataDir: d.dataDir}
		fresh.attach(testApp(), "main", true)
		fresh.bootstrap(s, exists, err)
		fresh.persist()
		got, _ := os.ReadFile(file)
		if string(got) != raw || !fresh.disabled {
			t.Fatal("invalid index overwritten")
		}
	}
	var state desktopWindowsState
	if err := json.Unmarshal(valid, &state); err != nil {
		t.Fatal(err)
	}
	state.Windows[1].ID, state.Active, state.Next = "9", "9", 1
	raw, _ := json.Marshal(state)
	if err := writeDesktopWindows(d.dataDir, raw); err != nil {
		t.Fatal(err)
	}
	state, _, err = readDesktopWindows(d.dataDir)
	if err != nil || state.Next != 9 {
		t.Fatal("window counter could reuse restored identity", err)
	}
}

func TestDesktopFinalQuitIgnoresNativeFocusAndActivation(t *testing.T) {
	d, a, b, _ := managedTestDesktop(t)
	if !d.requestQuit() || !d.committingQuit {
		t.Fatal("clean desktop did not allow final quit")
	}
	before := d.fingerprint()
	d.focus(b)
	b.windowVisible = false
	d.show(b)
	if d.active != a || b.windowVisible || d.busy {
		t.Fatal("native shutdown activation queued a save to a closing window")
	}
	b.windowVisible = true
	if d.fingerprint() != before {
		t.Fatal("shutdown focus changed the persisted active window")
	}
}
