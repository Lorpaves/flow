package main

import (
	"flow/internal/core"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestCommandSearchPaginationBeyondThirty(t *testing.T) {
	a := testApp()
	for n := 0; n < 125; n++ {
		if _, e := a.data.AddIssue(fmt.Sprintf("Palette item %03d", n), "", "", "todo"); e != nil {
			t.Fatal(e)
		}
	}
	a.openPalette("issues")
	tt := ui.NewTester(a.view, 1000, 680)
	tt.Type("Palette item")
	results := a.commandResults()
	if len(results) != 125 {
		t.Fatal("search truncated", len(results))
	}
	for n := 0; n < 60; n++ {
		tt.Key(0, ui.KeyDown)
	}
	if a.paletteIndex != 60 || a.paletteLimit != 100 {
		t.Fatal("keyboard did not load next page", a.paletteIndex, a.paletteLimit)
	}
	capture(t, tt, "27-search-pagination")
	want := strings.TrimPrefix(results[60].id, "issue/")
	tt.Key(0, ui.KeyEnter)
	if a.issueID != want || a.query != "palette item" || a.paletteOpen {
		t.Fatal("result opened wrong context")
	}
	a.openPalette("issues")
	tt.Frame()
	tt.Type("Palette item")
	click(t, tt, "Load more results")
	if a.paletteLimit != 100 {
		t.Fatal("explicit pagination did not advance")
	}
}

func TestNavigationSequencesCancelAndRespectText(t *testing.T) {
	a := testApp()
	tt := ui.NewTester(a.view, 1000, 680)
	tt.Key(0, ui.KeyG)
	tt.Key(0, ui.KeyP)
	if a.page != "projects" || a.keySequence != "" {
		t.Fatal("G P failed")
	}
	tt.Key(0, ui.KeyG)
	tt.Key(0, ui.KeyD)
	if a.page != "projects" {
		t.Fatal("removed draft shortcut navigated")
	}
	tt.Key(0, ui.KeyG)
	tt.Key(0, ui.KeyQ)
	if a.page != "projects" || a.keySequence != "" {
		t.Fatal("invalid sequence leaked")
	}
	tt.Key(0, ui.KeyG)
	tt.Advance(2 * time.Second)
	tt.Key(0, ui.KeyP)
	if a.page != "projects" || a.keySequence != "" {
		t.Fatal("prefix did not expire")
	}
	tt.Key(0, ui.KeyG)
	tt.Key(0, ui.KeyE)
	click(t, tt, "Search issues")
	tt.TypeKey(0, ui.KeyG, "g")
	tt.TypeKey(0, ui.KeyP, "p")
	if a.query != "gp" || a.page != "issues" || a.keySequence != "" {
		t.Fatal("typing invoked navigation")
	}
	tt.Key(0, ui.KeyEscape)
	tt.Key(0, ui.KeyO)
	tt.Key(0, ui.KeyI)
	if !a.paletteOpen || a.paletteScope != "issues" {
		t.Fatal("O I failed")
	}
	tt.TypeKey(0, ui.KeyG, "g")
	tt.Key(0, ui.KeyEscape)
	if a.keySequence != "" {
		t.Fatal("palette retained navigation prefix")
	}
	tt.Key(0, ui.KeyG)
	tt.Key(0, ui.KeyA)
	for _, i := range a.filtered() {
		if i.Status != "todo" && i.Status != "progress" && i.Status != "review" {
			t.Fatal("active navigation includes completed", i.Status)
		}
	}
}

func TestContextCommandsPropertiesAndProjectSequence(t *testing.T) {
	a := testApp()
	i := a.filtered()[0]
	a.openIssue(i.ID)
	child, _ := a.data.AddIssue("Child", i.ProjectID, i.ID, "todo")
	var target core.Project
	for id, p := range a.data.Projects {
		if id != i.ProjectID {
			target = p
			break
		}
	}
	m, e := a.data.SaveMilestone(core.Milestone{ProjectID: target.ID, Name: "Keyboard milestone"})
	if e != nil {
		t.Fatal(e)
	}
	tt := ui.NewTester(a.view, 1360, 880)
	tt.Key(ui.Shift, ui.KeyP)
	tt.Type(target.Name)
	tt.Key(0, ui.KeyEnter)
	if a.data.Issues[i.ID].ProjectID != target.ID || a.data.Issues[child.ID].ProjectID != target.ID {
		t.Fatal("keyboard project move lost hierarchy", a.err)
	}
	tt.Key(ui.Shift, ui.KeyM)
	tt.Type(m.Name)
	tt.Key(0, ui.KeyEnter)
	if a.data.Issues[i.ID].MilestoneID != m.ID {
		t.Fatal("milestone command failed", a.err)
	}
	tt.Key(0, ui.KeyM)
	tt.Key(0, ui.KeyB)
	if !a.relationOpen || a.relationKind != "blocked_by" {
		t.Fatal("relationship sequence failed")
	}
	a.relationOpen = false
	a.navigate("project", target.ID)
	a.projectTab = "overview"
	tt.Frame()
	tt.Key(0, ui.KeyP)
	tt.Key(0, ui.KeyS)
	if !a.paletteOpen || a.paletteScope != "project-status" {
		t.Fatal("project sequence collided with priority")
	}
	tt.Type("completed")
	tt.Key(0, ui.KeyEnter)
	if a.data.Projects[target.ID].Status != "completed" {
		t.Fatal("project status not persisted", a.err)
	}
	tt.Key(ui.Cmd|ui.Shift, ui.KeyU)
	if !a.updateOpen || a.updateForm.ProjectID != target.ID {
		t.Fatal("project update shortcut failed")
	}
	a.updateOpen = false
	tt.Frame()
	a.openIssue(i.ID)
	a.openPalette("all")
	tt.Frame()
	tt.Type("Copy issue title")
	tt.Key(0, ui.KeyEnter)
	if tt.Clipboard() != i.Title {
		t.Fatal("context command not bound to active issue")
	}
}

func TestShortcutHelpAndDuplicateSearchLabels(t *testing.T) {
	a := testApp()
	tt := ui.NewTester(a.view, 1000, 680)
	tt.Key(ui.Cmd, ui.KeySlash)
	tt.Type("milestone")
	tt.Frame()
	if !a.helpOpen {
		t.Fatal("help shortcut absent")
	}
	capture(t, tt, "27-shortcut-help")
	tt.Key(0, ui.KeyEscape)
	for id, p := range a.data.Projects {
		p.Name = "Same label"
		a.data.Projects[id] = p
	}
	a.openPalette("projects")
	tt.Frame()
	tt.Frame()
	results := a.commandResults()
	if len(results) < 2 || results[0].id == results[1].id {
		t.Fatal("search identity derived from label")
	}
	tt.Key(0, ui.KeyDown)
	tt.Key(0, ui.KeyEnter)
	if a.projectID != strings.TrimPrefix(results[1].id, "project/") {
		t.Fatal("duplicate label selected wrong project")
	}
}
