package main

import (
	"flow/internal/core"
	"fmt"
	"github.com/egoist/mygo/ui"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func testApp() *app {
	d := core.Empty()
	d.Seed()
	d.Preferences.Locale = "en"
	a := newApp(d)
	// FLOW_CAPTURE_LIGHT=1 takes the screenshots in the light appearance.
	if os.Getenv("FLOW_CAPTURE_LIGHT") != "" {
		a.appearance = "light"
	}
	return a
}
func click(t *testing.T, tt *ui.Tester, label string) {
	t.Helper()
	if err := tt.Click(label); err != nil {
		t.Fatal(err)
	}
}
func capture(t *testing.T, tt *ui.Tester, name string) {
	t.Helper()
	dir := os.Getenv("FLOW_CAPTURE_DIR")
	if dir == "" {
		return
	}
	if e := os.MkdirAll(dir, 0755); e != nil {
		t.Fatal(e)
	}
	// Lobe's components ease their colors and overlays in; a capture waits for
	// them to land. Plain test runs skip this and keep their own timing.
	tt.Advance(350 * time.Millisecond)
	f, e := os.Create(filepath.Join(dir, name+".png"))
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	if e = png.Encode(f, tt.Image()); e != nil {
		t.Fatal(e)
	}
}
func TestCreateEditSearchAndShortcuts(t *testing.T) {
	a := testApp()
	tt := ui.NewTester(a.view, 1360, 880)
	capture(t, tt, "01-issues")
	click(t, tt, "New issue")
	click(t, tt, "New issue title")
	tt.Type("中文测试 c / k")
	tt.Key(0, ui.KeyC)
	if !a.newOpen {
		t.Fatal("typing dismissed create dialog")
	}
	a.newIssue.Title = "中文验收任务"
	tt.Frame()
	click(t, tt, "Create issue")
	if a.issueID != "" || a.createdIssue == "" {
		t.Fatal("create should stay on list with an open action")
	}
	tt.Advance(time.Second) // the toast slides in before its action can be pressed
	click(t, tt, "Open issue")
	id := a.issueID
	click(t, tt, "Edit description")
	click(t, tt, "Issue description")
	tt.Type("Offline editing works. 中文输入")
	tt.Advance(time.Second)
	tt.Frame()
	if a.data.Issues[id].Plain != "Offline editing works. 中文输入" {
		t.Fatalf("description not persisted: %q", a.data.Issues[id].Plain)
	}
	capture(t, tt, "02-detail")
	click(t, tt, "Done editing")
	capture(t, tt, "12-detail-reading")
	click(t, tt, "Back to issues")
	click(t, tt, "Search issues")
	tt.Type("中文验收")
	if len(a.filtered()) != 1 {
		t.Fatalf("filter found %d", len(a.filtered()))
	}
	if a.newOpen {
		t.Fatal("single-letter shortcut fired while typing")
	}
	tt.Key(0, ui.KeyEscape)
	tt.Key(ui.Cmd, ui.KeyK)
	if !a.paletteOpen {
		t.Fatal("command palette did not open")
	}
	capture(t, tt, "03-palette")
	tt.Key(0, ui.KeyEscape)
}

func TestDraftSavePreservesConcurrentListChanges(t *testing.T) {
	a := testApp()
	i := a.filtered()[0]
	a.openIssue(i.ID)
	d := a.drafts[i.ID]
	a.setStatus(i.ID, "done")
	a.moveIssue(i.ID, "done", "")
	position := a.data.Issues[i.ID].Position
	d.issue.Title = "Edited while another action finishes"
	a.touch(d, time.Now())
	a.saveDraft(i.ID)
	saved := a.data.Issues[i.ID]
	if saved.Status != "done" || saved.Position != position || saved.Title != d.issue.Title {
		t.Fatalf("draft overwrote another action: %+v", saved)
	}
}

func TestChineseCompositionAndRichTextUndo(t *testing.T) {
	a := testApp()
	a.newTask("", "todo")
	tt := ui.NewTester(a.view, 1360, 880)
	click(t, tt, "New issue title")
	tt.Compose("zhongwen", 8)
	if a.newIssue.Title != "" {
		t.Fatal("uncommitted composition entered title")
	}
	tt.Type("中文输入验收")
	click(t, tt, "Create issue")
	tt.Advance(time.Second)
	click(t, tt, "Open issue")
	click(t, tt, "Edit description")
	click(t, tt, "Issue description")
	tt.Compose("zhengwen", 8)
	tt.Type("正文验收")
	tt.Command("selectAll")
	tt.Command("bold")
	tt.Command("undo")
	tt.Advance(time.Second)
	if got := a.data.Issues[a.issueID].Plain; got != "正文验收" {
		t.Fatalf("composition/format undo: %q", got)
	}
	if a.newOpen || a.paletteOpen {
		t.Fatal("typing triggered a global shortcut")
	}
}

func TestExternalRefreshKeepsDraftAndUpdatesVisibleState(t *testing.T) {
	a := testApp()
	i := a.filtered()[0]
	a.openIssue(i.ID)
	draft := a.drafts[i.ID]
	draft.issue.Title = "Local unsaved title"
	a.touch(draft, time.Now())
	external := a.data.Clone()
	i.Status = "done"
	external.Issues[i.ID] = i
	a.acceptExternal(external)
	if draft.issue.Title != "Local unsaved title" {
		t.Fatal("external refresh lost local draft")
	}
	a.saveDraft(i.ID)
	if a.data.Issues[i.ID].Title != "Local unsaved title" || a.data.Issues[i.ID].Status != "done" {
		t.Fatal("field merge lost one writer")
	}
	external = a.data.Clone()
	i = external.Issues[i.ID]
	i.Title = "MCP title"
	external.Issues[i.ID] = i
	a.acceptExternal(external)
	if a.drafts[i.ID].issue.Title != "MCP title" {
		t.Fatal("clean detail did not refresh")
	}
}
func TestViewsAndLanguage(t *testing.T) {
	a := testApp()
	tt := ui.NewTester(a.view, 1360, 880)
	click(t, tt, "Board view")
	if !a.board {
		t.Fatal("board toggle failed")
	}
	capture(t, tt, "04-board")
	click(t, tt, "Projects")
	capture(t, tt, "05-projects")
	click(t, tt, "New project")
	click(t, tt, "Project name")
	tt.Type("New project test")
	click(t, tt, "Save project")
	if a.projectID == "" {
		t.Fatal("project not opened")
	}
	click(t, tt, "Overview")
	capture(t, tt, "06-overview")
	click(t, tt, "Settings")
	click(t, tt, "简体中文")
	if a.data.Preferences.Locale != "zh-Hans" {
		t.Fatal("locale change failed")
	}
	tt.Frame()
	if !tt.HasText("外观") {
		t.Fatal("Chinese labels missing")
	}
	capture(t, tt, "07-settings-zh")
	a.navigate("issues", "")
	a.board = false
	tt.SetSize(1000, 680)
	capture(t, tt, "08-compact-zh")
}
func TestNativeBoardDragAndTrash(t *testing.T) {
	a := testApp()
	a.board = true
	tt := ui.NewTester(a.view, 1360, 880)
	var first core.Issue
	for _, i := range a.filtered() {
		if i.Status == "backlog" {
			first = i
			break
		}
	}
	from, ok := tt.Find(first.Code() + " " + first.Title)
	if !ok {
		t.Fatal("card not found")
	}
	to, ok := tt.Find("Board column Todo")
	if !ok {
		t.Fatal("target column not found")
	}
	tt.Press(from.X+from.W/2, from.Y+from.H/2)
	tt.Move(from.X+from.W/2+12, from.Y+from.H/2)
	tt.Move(to.X+to.W/2, to.Y+to.H-35)
	tt.Release(to.X+to.W/2, to.Y+to.H-35)
	if a.data.Issues[first.ID].Status != "todo" {
		t.Fatal("cross-column drag did not persist status")
	}
	a.trashIssue(first.ID)
	a.navigate("trash", "")
	tt.Frame()
	if a.data.Issues[first.ID].DeletedAt == 0 {
		t.Fatal("trash failed")
	}
	click(t, tt, "Restore")
	if a.data.Issues[first.ID].DeletedAt != 0 {
		t.Fatal("restore failed")
	}
}

func TestInlinePickersAndNestedEscape(t *testing.T) {
	a := testApp()
	tt := ui.NewTester(a.view, 1360, 880)
	i := a.data.Issues[a.visibleIssueIDs()[0]]
	click(t, tt, "Status "+i.Code())
	if a.issueID != "" {
		t.Fatal("inline status opened detail")
	}
	capture(t, tt, "09-status-picker")
	tt.Type("3")
	if a.data.Issues[i.ID].Status != "progress" {
		t.Fatal("numeric status choice did not persist")
	}
	click(t, tt, "Priority "+i.Code())
	tt.Type("high")
	tt.Key(0, ui.KeyEnter)
	if a.data.Issues[i.ID].Priority != 2 || a.issueID != "" {
		t.Fatal("searchable inline priority failed")
	}
	a.openIssue(i.ID)
	tt.Frame()
	click(t, tt, "In Progress")
	tt.Key(0, ui.KeyEscape)
	if a.issueID != i.ID || (a.activePicker != nil && a.activePicker.open) {
		t.Fatal("Escape must close only the picker")
	}
	labels := a.data.LabelList()
	a.drafts[i.ID].issue.LabelIDs = nil
	tt.Frame()
	click(t, tt, "Labels")
	click(t, tt, labels[0].Name)
	click(t, tt, labels[1].Name)
	if len(a.drafts[i.ID].issue.LabelIDs) != 2 || !a.activePicker.open {
		t.Fatal("multi-label picker should stay open")
	}
	capture(t, tt, "10-label-picker")
	tt.Key(0, ui.KeyEscape)
	tt.Key(0, ui.KeyEscape)
	if a.issueID != "" {
		t.Fatal("second Escape should return to list")
	}
	tt.Frame()
	tt.Frame()
}

func TestNavigationAndCommandKeyboard(t *testing.T) {
	a := testApp()
	tt := ui.NewTester(a.view, 1360, 880)
	tt.Key(0, ui.KeyJ)
	tt.Key(0, ui.KeyS)
	if a.activePicker == nil || !a.activePicker.open {
		t.Fatal("list S did not open status picker")
	}
	tt.Key(0, ui.KeyEscape)
	first := a.focusedIssue
	tt.Key(0, ui.KeyJ)
	second := a.focusedIssue
	if first == "" || first == second {
		t.Fatal("list keyboard navigation did not advance")
	}
	tt.Key(0, ui.KeyX)
	if !a.selected[second] {
		t.Fatal("X did not select focused row")
	}
	tt.Key(0, ui.KeyEnter)
	if a.issueID != second {
		t.Fatal("Enter did not open focused issue")
	}
	click(t, tt, "Next issue")
	third := a.issueID
	if third == second {
		t.Fatal("next issue did not advance")
	}
	click(t, tt, "Previous issue")
	if a.issueID != second {
		t.Fatal("previous issue did not return")
	}
	click(t, tt, "Back to issues")
	if a.focusedIssue != second || !a.selected[second] {
		t.Fatal("return lost list context")
	}
	tt.Key(ui.Cmd, ui.KeyK)
	tt.Type("Settings")
	tt.Key(0, ui.KeyEnter)
	if a.paletteOpen || a.page != "settings" {
		t.Fatal("command Enter did not execute result")
	}
}

func TestCreateMoreDraftResumeAndModalEscape(t *testing.T) {
	a := testApp()
	a.openIssue(a.visibleIssueIDs()[0])
	original := a.issueID
	tt := ui.NewTester(a.view, 1360, 880)
	tt.Key(ui.Cmd, ui.KeyN)
	tt.Type("First continuous issue")
	click(t, tt, "New issue priority")
	tt.Key(0, ui.KeyEscape)
	if !a.newOpen || a.issueID != original {
		t.Fatal("picker Escape closed parent modal")
	}
	click(t, tt, "Create more")
	tt.Key(ui.Cmd, ui.KeyEnter)
	if !a.newOpen || a.issueID != original || a.newIssue.Title != "" {
		t.Fatal("continuous creation lost context")
	}
	tt.Type("Preserved draft")
	capture(t, tt, "11-create-more")
	tt.Key(0, ui.KeyEscape)
	if a.newOpen || a.issueID != original {
		t.Fatal("modal Escape navigated underlying detail")
	}
	tt.Key(ui.Cmd, ui.KeyN)
	if a.newIssue.Title != "Preserved draft" {
		t.Fatal("reopening lost draft")
	}
	click(t, tt, "Create more")
	click(t, tt, "Create issue")
	if a.newOpen || a.issueID != original {
		t.Fatal("creation should retain underlying detail")
	}
	found := 0
	for _, i := range a.data.Issues {
		if i.Title == "First continuous issue" || i.Title == "Preserved draft" {
			found++
		}
	}
	if found != 2 {
		t.Fatalf("created %d issues", found)
	}
}

func TestModifierSelectionAndOffscreenNavigation(t *testing.T) {
	a := testApp()
	tt := ui.NewTester(a.view, 1360, 880)
	ids := a.visibleIssueIDs()
	for _, id := range []string{ids[0], ids[1]} {
		i := a.data.Issues[id]
		if e := tt.ClickWith(ui.Cmd, i.Code()+" "+i.Title); e != nil {
			t.Fatal(e)
		}
	}
	if a.issueID != "" || len(a.selected) != 2 {
		t.Fatal("Cmd-click should select without opening")
	}
	i := a.data.Issues[ids[4]]
	if e := tt.ClickWith(ui.Shift, i.Code()+" "+i.Title); e != nil {
		t.Fatal(e)
	}
	if len(a.selected) != 5 {
		t.Fatalf("Shift range: selected %d", len(a.selected))
	}
	for n := 0; n < 80; n++ {
		a.data.AddIssue(fmt.Sprintf("Scroll regression %d", n), "", "", "todo")
	}
	a.dataRevision++
	a.navigate("issues", "")
	a.statusFilter = "todo"
	tt.Frame()
	for n := 0; n < 55; n++ {
		tt.Key(0, ui.KeyJ)
	}
	i = a.data.Issues[a.focusedIssue]
	rect, ok := tt.Find(i.Code() + " " + i.Title)
	if !ok || rect.Y < 100 || rect.Y > 850 {
		t.Fatalf("focused row not scrolled into view: %+v %v", rect, ok)
	}
	tt.Key(0, ui.KeyEnter)
	click(t, tt, "Back to issues")
	if a.statusFilter != "todo" || a.focusedIssue != i.ID {
		t.Fatalf("navigation lost filter or focus: filter=%s focus=%s want=%s", a.statusFilter, a.focusedIssue, i.ID)
	}
	if _, ok := tt.Find(i.Code() + " " + i.Title); !ok {
		t.Fatal("return lost visible position")
	}
	// Scrolling after a keyboard move must not be forced back every frame.
	tt.Wheel(800, 500, 0, -3000)
	tt.Advance(time.Second)
	tt.Frame()
	first, _ := a.list.Visible()
	if first > 10 {
		t.Fatal("focused row prevented manual scrolling")
	}
}
