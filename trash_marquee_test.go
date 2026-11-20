package main

import (
	"flow/internal/core"
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func issueCoded(a *app, code string) core.Issue {
	for _, i := range a.data.Issues {
		if i.Code() == code {
			return i
		}
	}
	return core.Issue{}
}

func selectedCodes(a *app) []string {
	var out []string
	for id, on := range a.selected {
		if on {
			out = append(out, a.data.Issues[id].Code())
		}
	}
	slices.Sort(out)
	return out
}

func middle(t *testing.T, tt *ui.Tester, text string) (float32, float32) {
	t.Helper()
	r, ok := tt.Find(text)
	if !ok {
		t.Fatalf("%q is not shown", text)
	}
	return r.X + r.W/2, r.Y + r.H/2
}

// A box dragged from blank space chooses the issues it crosses; a click there
// chooses nothing, unless with Cmd; a press on a row is that row's.
func TestDraggingABoxChoosesTheIssuesItCrosses(t *testing.T) {
	a := testApp()
	tt := ui.NewTester(a.view, 1360, 880)
	// Over the Todo group's title, right of it, where nothing takes a press.
	_, y0 := middle(t, tt, "Todo")
	x := float32(800)
	_, y7 := middle(t, tt, "FLW-7")
	tt.Move(x, y0)
	tt.Press(x, y0)
	tt.Move(x-20, y0+12)
	tt.Move(x-260, y7)
	capture(t, tt, "39-marquee")
	tt.Release(x-260, y7)
	if got := selectedCodes(a); !slices.Equal(got, []string{"FLW-3", "FLW-7"}) {
		t.Fatalf("chosen = %v", got)
	}
	if a.marquee != nil {
		t.Fatal("the box outlived the release")
	}
	if !tt.HasText("2 selected") {
		t.Fatal("the selection bar is not shown")
	}

	// A Cmd-click on blank space keeps the choice; the engine reads the
	// modifiers from the pointer event, as macOS sends them.
	_, yp := middle(t, tt, "In Progress")
	tt.ClickAtWith(ui.Cmd, x, yp)
	if got := selectedCodes(a); !slices.Equal(got, []string{"FLW-3", "FLW-7"}) {
		t.Fatalf("a Cmd-click on blank space changed the choice: %v", got)
	}

	// A click on blank space chooses nothing.
	tt.Move(x, yp)
	tt.Press(x, yp)
	tt.Release(x, yp)
	if len(selectedCodes(a)) != 0 {
		t.Fatal("a click on blank space kept the choice")
	}

	// A press on a row stays the row's: it opens it, and draws no box.
	rx, ry := middle(t, tt, "FLW-3")
	tt.Move(rx, ry)
	tt.Press(rx, ry)
	tt.Move(rx+2, ry+2)
	if a.marquee != nil {
		t.Fatal("a press on a row began a box")
	}
	tt.Release(rx+2, ry+2)
	if a.issueID != issueCoded(a, "FLW-3").ID {
		t.Fatal("the row did not open")
	}
}

// The trash deletes for good, one issue or all of it, after asking.
func TestTrashDeletesForGood(t *testing.T) {
	a := testApp()
	one, two := issueCoded(a, "FLW-1"), issueCoded(a, "FLW-2")
	a.apply(func(d *core.Data) error { d.TrashIssue(one.ID); d.TrashIssue(two.ID); return nil }, nil)
	a.navigate("trash", "")
	tt := ui.NewTester(a.view, 1360, 880)
	capture(t, tt, "39-trash")
	click(t, tt, "Delete forever: FLW-1")
	if !a.eraseOpen {
		t.Fatal("deleting for good did not ask first")
	}
	capture(t, tt, "39-erase-dialog")
	click(t, tt, "Cancel")
	if _, ok := a.data.Issues[one.ID]; !ok || a.eraseOpen {
		t.Fatal("canceling deleted the issue")
	}
	click(t, tt, "Delete forever: FLW-1")
	click(t, tt, "Confirm delete forever")
	if _, ok := a.data.Issues[one.ID]; ok {
		t.Fatal("the issue is still there")
	}
	if _, ok := a.data.Erased[one.ID]; !ok {
		t.Fatal("no erasure was kept for sync")
	}
	click(t, tt, "Empty trash")
	click(t, tt, "Confirm delete forever")
	if _, ok := a.data.Issues[two.ID]; ok {
		t.Fatal("emptying the trash left an issue")
	}
	if !tt.HasText("Trash is empty") {
		t.Fatal("the trash does not say it is empty")
	}
}

// An issue open when it is deleted for good, here or on another Mac, closes
// instead of showing nothing, and its edit is not saved into nothing.
func TestAnIssueGoneForGoodCloses(t *testing.T) {
	a := testApp()
	i := issueCoded(a, "FLW-1")
	a.apply(func(d *core.Data) error { d.TrashIssue(i.ID); return nil }, nil)
	a.navigate("trash", "")
	a.openIssue(i.ID)
	tt := ui.NewTester(a.view, 1360, 880)
	click(t, tt, "Delete issue forever")
	click(t, tt, "Confirm delete forever")
	if a.issueID != "" || a.drafts[i.ID] != nil {
		t.Fatal("the erased issue is still open")
	}
}

// Held past the list's bottom, a box scrolls the list and keeps choosing,
// rows that scrolled out of sight above included.
func TestABoxScrollsTheListAndKeepsWhatScrolledAway(t *testing.T) {
	a := testApp()
	tt := ui.NewTester(a.view, 1100, 420)
	_, y0 := middle(t, tt, "Backlog")
	x := float32(700)
	tt.Move(x, y0)
	tt.Press(x, y0)
	tt.Move(x, y0+12)
	tt.Move(x, 414)
	for n := 0; n < 60; n++ {
		tt.Advance(40 * time.Millisecond)
	}
	tt.Release(x, 414)
	got := selectedCodes(a)
	for _, code := range []string{"FLW-1", "FLW-9"} {
		if !slices.Contains(got, code) {
			t.Fatalf("%s not chosen: %v", code, got)
		}
	}
	if len(got) != len(a.filtered()) {
		var all []string
		for _, i := range a.filtered() {
			all = append(all, i.Code())
		}
		t.Fatalf("chose %v of %v", got, all)
	}
}
