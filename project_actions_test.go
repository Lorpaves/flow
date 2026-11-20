package main

import (
	"flow/internal/core"
	"github.com/egoist/mygo/ui"
	"slices"
	"testing"
	"time"
)

func TestProjectActionsLabelsCopyDatesAndReminder(t *testing.T) {
	a := testApp()
	p := a.data.ProjectList(false, false)[0]
	a.navigate("project", p.ID)
	a.projectTab = "overview"
	// A previously focused task must not steal project actions on overview.
	a.focusedIssue = a.data.Query(core.Filter{ProjectID: p.ID})[0].ID
	tt := ui.NewTester(a.view, 1000, 680)
	tt.Key(0, ui.KeyP)
	tt.Key(0, ui.KeyL)
	tt.Type("项目验收标签")
	tt.Key(0, ui.KeyEnter)
	var id string
	for _, l := range a.data.LabelsForKind("project") {
		id = l.ID
	}
	if id == "" || !slices.Contains(a.data.Projects[p.ID].LabelIDs, id) {
		t.Fatal("inline project label not assigned", a.err)
	}
	tt.Frame()
	capture(t, tt, "29-project-label-picker")
	tt.Key(0, ui.KeyEscape)
	tt.Key(ui.Cmd|ui.Shift, ui.KeyComma)
	if tt.Clipboard() != projectURL(p.ID) {
		t.Fatal("project copy shortcut targeted issue")
	}
	tt.Key(ui.Cmd|ui.Shift, ui.KeyQuote)
	if tt.Clipboard() != p.Name {
		t.Fatal("project title not copied")
	}
	tt.Key(ui.Ctrl|ui.Alt, ui.KeyS)
	if !a.date.open || a.date.title != "Project start date" {
		t.Fatal("start date shortcut")
	}
	a.date.input = "2026-11-01"
	a.saveDate()
	tt.Frame()
	tt.Key(ui.Ctrl|ui.Alt, ui.KeyD)
	a.date.input = "2026-12-01"
	a.saveDate()
	tt.Frame()
	if a.data.Projects[p.ID].Start != "2026-11-01" || a.data.Projects[p.ID].Due != "2026-12-01" {
		t.Fatal("project dates not saved", a.err)
	}
	tt.Key(ui.Shift, ui.KeyH)
	if !a.reminderOpen || a.reminderProject != p.ID || a.reminderIssue != "" {
		t.Fatal("project reminder shortcut targeted task")
	}
	a.reminderInput = "1h"
	a.saveReminder()
	tt.Frame()
	for _, r := range a.data.Reminders {
		if r.ProjectID != p.ID || r.IssueID != "" {
			t.Fatal("wrong reminder owner")
		}
		a.openReminder(r)
	}
	if a.projectID != p.ID || a.projectTab != "overview" {
		t.Fatal("reminder did not open owner")
	}
	a.navigate("issues", "")
	if e := a.openLink(projectURL(p.ID)); e != nil || a.projectID != p.ID || a.projectTab != "overview" {
		t.Fatal("project link", e)
	}
	for _, raw := range []string{"flow://project/missing", "flow://project/", "flow://project/" + p.ID + "/more", "flow://project/" + p.ID + "?x=1"} {
		if e := a.openLink(raw); e == nil {
			t.Fatal("invalid project URL accepted", raw)
		}
	}
	tt.Frame()
	capture(t, tt, "29-project-overview")
}

func TestRemovedSidebarDestinationsAndDraftRecovery(t *testing.T) {
	a := testApp()
	tt := ui.NewTester(a.view, 1000, 680)
	for _, label := range []string{"Drafts", "Templates", "Reminders & recurring", "New tab"} {
		if _, ok := tt.Find(label); ok {
			t.Fatal("removed destination still shown", label)
		}
	}
	for _, cmd := range a.commandResults() {
		if slices.Contains([]string{"navigate/drafts", "navigate/templates", "navigate/reminders"}, cmd.id) {
			t.Fatal("removed navigation command", cmd.id)
		}
	}
	for _, page := range []string{"drafts", "templates", "reminders"} {
		r := a.sanitizeRoute(route{Page: page, Config: core.DefaultView()})
		if r.Page != "issues" {
			t.Fatal("old session reopens removed page", page)
		}
	}
	tt.Key(0, ui.KeyG)
	tt.Key(0, ui.KeyD)
	tt.Key(ui.Alt, ui.KeyC)
	if a.page != "issues" {
		t.Fatal("removed navigation shortcuts remain")
	}
	a.newTask("", "backlog")
	tt.Frame()
	a.newIssue.Title = "继续自动保存"
	tt.Advance(time.Second)
	a.closeComposition()
	tt.Frame()
	b := newApp(a.data)
	tb := ui.NewTester(b.view, 1000, 680)
	click(t, tb, "New issue")
	if b.newIssue.Title != "继续自动保存" {
		t.Fatal("removing Drafts broke automatic recovery")
	}
	b.closeComposition()
	tb.Frame()
	capture(t, tb, "29-simplified-sidebar")
}
