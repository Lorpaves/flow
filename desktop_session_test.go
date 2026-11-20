package main

import (
	"encoding/json"
	"flow/internal/core"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func TestLegacyTabsAndActiveSessionRestart(t *testing.T) {
	a := testApp()
	a.dataDir = t.TempDir()
	a.restoreDesktopSession(desktopSession{}, false, nil)
	tt := ui.NewTester(a.view, 1360, 880)
	target := a.filtered()[0]
	a.openIssue(target.ID)
	a.query = "原生会话"
	tt.Frame()
	a.newTab()
	a.navigate("settings", "")
	tt.Frame()
	a.pinTab(a.navigation.Active)
	if !a.navigation.Tabs[0].Pinned || a.navigation.Active != 0 || a.page != "settings" {
		t.Fatal("pin did not preserve active route")
	}
	a.closeTab(a.navigation.Active)
	if len(a.navigation.Tabs) != 2 || len(a.navigation.Closed) != 0 {
		t.Fatal("close shortcut closed a pinned tab")
	}
	a.newTab()
	a.navigate("projects", "")
	tt.Frame()
	a.newTab()
	a.navigate("trash", "")
	tt.Frame()
	a.closeOtherTabs(a.navigation.Active)
	if len(a.navigation.Tabs) != 2 || a.page != "trash" || len(a.navigation.Closed) != 2 || !a.navigation.Tabs[0].Pinned {
		t.Fatal("close others lost pinned or active tabs")
	}
	a.reopenTab()
	if a.page != "projects" || len(a.navigation.Tabs) != 3 {
		t.Fatal("closed tab did not reopen")
	}
	a.sidebarWidth = 318
	a.folderCollapsed = map[string]bool{"folder": true}
	tt.Frame()
	click(t, tt, "Toggle sidebar")
	a.selected[target.ID] = true
	tt.Advance(time.Second)
	tt.Frame()
	if a.desktopDirty() || a.err != "" {
		t.Fatal("desktop did not save", a.err)
	}
	raw, e := os.ReadFile(filepath.Join(a.dataDir, "desktop-session.json"))
	if e != nil {
		t.Fatal(e)
	}
	if strings.Contains(string(raw), "Selected") || strings.Contains(string(raw), "List") {
		t.Fatal("persisted runtime or bulk selection state")
	}
	state, exists, e := readDesktopSession(a.dataDir)
	if e != nil || !exists {
		t.Fatal("read session", e)
	}
	b := newApp(a.data)
	b.dataDir = a.dataDir
	b.restoreDesktopSession(state, exists, e)
	if b.page != "projects" || b.sidebarWidth != 318 || !b.navigation.SidebarCollapsed || !b.folderCollapsed["folder"] || !b.navigation.Tabs[0].Pinned || len(b.selected) != 0 {
		t.Fatal("restart lost desktop state or restored bulk selection")
	}
	b.reopenTab()
	if b.issueID != target.ID || b.query != "原生会话" {
		t.Fatal("restart lost closed tab route", b.issueID, b.query)
	}
	b.historyStep(-1)
	if b.issueID != "" {
		t.Fatal("reopened tab lost navigation history")
	}
	b.navigation.SidebarCollapsed = false
	view := ui.NewTester(b.view, 1000, 680)
	capture(t, view, "28-restored-page")
	view.Frame()
	view.Frame()
}

func TestHeaderNavigationWithoutTopTabs(t *testing.T) {
	a := testApp()
	a.newTab() // A restored legacy session may contain several tabs.
	a.navigate("projects", "")
	tt := ui.NewTester(a.view, 1000, 680)
	tt.SetTitleBar(ui.TitleBar{Height: 38, Left: 80})
	tt.Frame()
	for _, label := range []string{"New tab", "Switch tab 1", "Switch tab 2", "Tab actions"} {
		if _, ok := tt.Find(label); ok {
			t.Fatal("removed tab control remains", label)
		}
	}
	for _, key := range []struct {
		mods ui.Modifiers
		key  ui.Key
	}{{ui.Cmd, ui.KeyT}, {ui.Cmd | ui.Shift, ui.KeyT}, {ui.Cmd, ui.Key1}, {ui.Cmd | ui.Shift, ui.KeyA}} {
		tt.Key(key.mods, key.key)
	}
	if len(a.navigation.Tabs) != 2 || a.navigation.Active != 1 || a.paletteOpen {
		t.Fatal("hidden tab actions are still enabled")
	}
	click(t, tt, "Go back")
	if a.page != "issues" {
		t.Fatal("header back failed")
	}
	click(t, tt, "Go forward")
	if a.page != "projects" {
		t.Fatal("header forward failed")
	}
	capture(t, tt, "28-no-top-tabs")
	click(t, tt, "Toggle sidebar")
	r, ok := tt.Find("Toggle sidebar")
	if !ok || r.Y > 60 || r.X < 80 {
		t.Fatalf("header overlaps titlebar controls: %+v", r)
	}
	capture(t, tt, "28-no-top-tabs-collapsed")
	a.openPalette("all")
	for _, cmd := range a.commandResults() {
		if strings.Contains(cmd.id, "tab") {
			t.Fatal("tab command still exposed", cmd.id)
		}
	}
}

func TestNativeMenuWithoutTabsRespectsModalEditors(t *testing.T) {
	a := testApp()
	var create, help, closeWindow *mygo.MenuItem
	for _, top := range a.desktopMenuItems() {
		for _, item := range top.Submenu {
			if strings.Contains(item.ID, "tab") {
				t.Fatal("native menu exposes removed tabs", item.ID)
			}
			switch item.ID {
			case "new-issue":
				create = item
			case "shortcut-help":
				help = item
			}
			if item.Role == mygo.RoleClose {
				closeWindow = item
			}
		}
	}
	if create == nil || help == nil || closeWindow == nil || closeWindow.Accelerator != "CmdOrCtrl+W" {
		t.Fatal("native menu missing actions")
	}
	help.Click(nil, nil)
	if !a.helpOpen {
		t.Fatal("native help did not open")
	}
	create.Click(nil, nil)
	if a.newOpen {
		t.Fatal("native create bypassed modal help")
	}
	a.helpOpen = false
	create.Click(nil, nil)
	if !a.newOpen {
		t.Fatal("native create did not open")
	}
}

func TestDesktopSessionQueuedSavePreservesLatestStateAndWorkspace(t *testing.T) {
	a := testApp()
	a.dataDir = t.TempDir()
	a.restoreDesktopSession(desktopSession{}, false, nil)
	a.window = &mygo.Window{}
	a.query = "first"
	a.persistDesktopSession()
	first := <-a.jobs
	a.query = "changed while saving"
	remote := a.data.Clone()
	remote.Preferences.Locale = "zh-Hans"
	a.data = remote
	_, err := first.run(nil)
	first.done(core.Empty(), err)
	if a.data.Preferences.Locale != "zh-Hans" || !a.desktopDirty() || a.pending != 0 {
		t.Fatal("session save overwrote workspace or dropped a later change")
	}
	a.persistDesktopSession()
	second := <-a.jobs
	_, err = second.run(nil)
	second.done(core.Empty(), err)
	state, _, err := readDesktopSession(a.dataDir)
	if err != nil || state.Tabs[state.Active].Route.Query != "changed while saving" || a.desktopDirty() {
		t.Fatal("latest session missing", err)
	}
	// A disk error keeps state dirty, cancels quitting, and leaves the last
	// committed file readable rather than reporting the save as successful.
	a.query = "must survive failure"
	validDir := a.dataDir
	a.dataDir = filepath.Join(validDir, "desktop-session.json")
	a.quitRequested = true
	a.persistDesktopSession()
	failing := <-a.jobs
	_, err = failing.run(nil)
	failing.done(core.Empty(), err)
	if err == nil || a.quitRequested || !a.desktopDirty() || a.pending != 0 {
		t.Fatal("save failure lost pending session")
	}
	state, _, err = readDesktopSession(validDir)
	if err != nil || state.Tabs[state.Active].Route.Query != "changed while saving" {
		t.Fatal("failed write damaged previous session")
	}
}

func TestDesktopSessionDeletedTargetsAndInvalidFiles(t *testing.T) {
	a := testApp()
	a.dataDir = t.TempDir()
	a.restoreDesktopSession(desktopSession{}, false, nil)
	issue := a.filtered()[0]
	a.navigate("project", issue.ProjectID)
	a.openIssue(issue.ID)
	a.persistDesktopSession()
	state, exists, err := readDesktopSession(a.dataDir)
	if err != nil || !exists {
		t.Fatal(err)
	}
	data := a.data.Clone()
	delete(data.Projects, issue.ProjectID)
	delete(data.Issues, issue.ID)
	b := newApp(data)
	b.restoreDesktopSession(state, exists, nil)
	if b.issueID != "" || b.projectID != "" || b.page != "projects" {
		t.Fatal("restored deleted entity", b.page, b.issueID)
	}
	for _, raw := range []string{"{broken", `{"version":99,"tabs":[{}]}`, `{"version":1,"tabs":[{"ID":"1"},{"ID":"1"}]}`} {
		file := filepath.Join(a.dataDir, "desktop-session.json")
		if err := os.WriteFile(file, []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
		state, exists, err := readDesktopSession(a.dataDir)
		if err == nil || exists {
			t.Fatal("accepted unsupported or corrupt session")
		}
		b := testApp()
		b.dataDir = a.dataDir
		b.restoreDesktopSession(state, exists, err)
		b.newTab()
		b.persistDesktopSession()
		got, _ := os.ReadFile(file)
		if string(got) != raw || !b.desktopDisabled {
			t.Fatal("overwrote invalid session")
		}
	}
	state = desktopSession{Version: 1, SidebarWidth: 9999, Active: 999, Tabs: []navigationTab{{ID: "5", Route: route{Page: "issues", Config: core.DefaultView()}}}}
	raw, _ := json.Marshal(state)
	if err := writeDesktopSession(a.dataDir, raw); err != nil {
		t.Fatal(err)
	}
	state, _, err = readDesktopSession(a.dataDir)
	if err != nil || state.SidebarWidth != 380 || state.Active != 0 || state.Next != 5 {
		t.Fatal("invalid geometry/index was not normalized", err)
	}
}
