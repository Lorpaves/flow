package main

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestPointerFocusesRowUntilKeyboardMovesFocus(t *testing.T) {
	a := testApp()
	tt := ui.NewTester(a.view, 1360, 880)
	ids := a.visibleIssueIDs()
	third := a.data.Issues[ids[2]]
	r, ok := tt.Find(third.Code() + " " + third.Title)
	if !ok {
		t.Fatal("row not found")
	}
	x, y := r.X+r.W/2, r.Y+r.H/2
	route := a.desktopFingerprint()
	tt.Move(x, y)
	if a.focusedIssue != third.ID {
		t.Fatal("pointer did not focus the row under it")
	}
	if a.desktopFingerprint() != route {
		t.Fatal("passing over a row changed the saved session")
	}
	tt.Key(0, ui.KeyS)
	if a.activePicker == nil || !a.activePicker.open {
		t.Fatal("S did not open the status menu of the hovered row")
	}
	tt.Type("5")
	if a.data.Issues[third.ID].Status != "done" {
		t.Fatal("shortcut edited another row")
	}
	a.navigate("issues", "")
	tt.Frame()
	tt.Key(0, ui.KeyJ)
	tt.Key(0, ui.KeyJ)
	chosen := a.focusedIssue
	tt.Frame()
	tt.Frame()
	if chosen == "" || a.focusedIssue != chosen {
		t.Fatal("resting pointer took the focus from the keyboard")
	}
	under := ""
	for _, id := range a.visibleIssueIDs() {
		i := a.data.Issues[id]
		if row, ok := tt.Find(i.Code() + " " + i.Title); ok && y >= row.Y && y < row.Y+row.H {
			under = id
		}
	}
	if under == "" || under == chosen {
		t.Fatalf("fixture needs the pointer over another row: %q", under)
	}
	tt.Move(x+6, y)
	if a.focusedIssue != under {
		t.Fatal("moving the pointer did not take the focus back")
	}
}

func TestNoticeFloatsThenExpires(t *testing.T) {
	a := testApp()
	tt := ui.NewTester(a.view, 1360, 880)
	a.notice = "Template saved"
	tt.Frame()
	if a.notice != "" {
		t.Fatal("notice was not handed to the toasts")
	}
	tt.Advance(time.Second)
	if _, ok := tt.Find("Template saved"); !ok {
		t.Fatal("notice not shown")
	}
	toolbar, ok := tt.Find("Display options")
	if !ok || toolbar.Y > 100 {
		t.Fatalf("notice pushed the page down: %+v", toolbar)
	}
	tt.Advance(8 * time.Second)
	tt.Frame()
	if _, ok := tt.Find("Template saved"); ok {
		t.Fatal("expired notice still shown")
	}
}

func TestBreadcrumbRetracesIssuesOutsideTheList(t *testing.T) {
	a := testApp()
	a.statusFilter = "todo"
	tt := ui.NewTester(a.view, 1360, 880)
	listed := a.visibleIssueIDs()[0]
	outside := ""
	for id, i := range a.data.Issues {
		if i.Status == "done" {
			outside = id
		}
	}
	a.openIssue(listed)
	a.openIssue(outside)
	tt.Frame()
	if len(a.returnIssues) != 1 {
		t.Fatalf("fixture: return chain %v", a.returnIssues)
	}
	click(t, tt, "Back")
	if a.issueID != listed {
		t.Fatal("trail did not return to the issue passed through")
	}
	click(t, tt, "Back to issues")
	if a.issueID != "" || a.statusFilter != "todo" {
		t.Fatal("trail root did not return to the list")
	}
}

func TestDetailPropertiesColumnNeedsRoom(t *testing.T) {
	a := testApp()
	id := a.visibleIssueIDs()[0]
	a.openIssue(id)
	tt := ui.NewTester(a.view, 1360, 880)
	if _, ok := tt.Find("Issue properties"); !ok {
		t.Fatal("wide detail has no properties column")
	}
	click(t, tt, "Due date")
	if !a.date.open {
		t.Fatal("column property did not open its editor")
	}
	tt.Key(0, ui.KeyEscape)
	tt.SetSize(900, 700)
	if _, ok := tt.Find("Issue properties"); ok {
		t.Fatal("narrow detail kept the properties column")
	}
	click(t, tt, "Due date")
	if !a.date.open {
		t.Fatal("inline property did not open its editor")
	}
}

func TestPanelsStayUsableWhileArriving(t *testing.T) {
	a := testApp()
	a.motion = true // the native window eases menus and dialogs in
	tt := ui.NewTester(a.view, 1360, 880)
	i := a.data.Issues[a.visibleIssueIDs()[0]]
	click(t, tt, "Status "+i.Code())
	tt.Type("5") // typed before the menu has finished arriving
	if a.data.Issues[i.ID].Status != "done" {
		t.Fatal("menu ignored input while arriving")
	}
	tt.Move(4, 4)
	tt.Advance(2 * time.Second)
	if tt.FramePending() {
		t.Fatal("menu transition never settles")
	}
	tt.Key(ui.Cmd, ui.KeyK)
	tt.Type("Settings")
	tt.Key(0, ui.KeyEnter)
	if a.paletteOpen || a.page != "settings" {
		t.Fatal("dialog ignored input while arriving")
	}
	tt.Advance(2 * time.Second)
	if tt.FramePending() {
		t.Fatal("dialog transition never settles")
	}
}
