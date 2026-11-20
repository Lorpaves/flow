package main

import (
	"flow/internal/core"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestBatchDateProjectAndSelectedDrag(t *testing.T) {
	a := testApp()
	ids := a.visibleIssueIDs()[:2]
	for _, id := range ids {
		a.selected[id] = true
	}
	tt := ui.NewTester(a.view, 1360, 880)
	click(t, tt, "Set due date")
	click(t, tt, "Tomorrow")
	click(t, tt, "Save date")
	date, _ := core.ParseDate("tomorrow", time.Now())
	for _, id := range ids {
		if a.data.Issues[id].Due != date {
			t.Fatal("batch date missed issue")
		}
	}
	projects := a.data.ProjectList(false, false)
	pid := projects[1].ID
	click(t, tt, "Set project")
	tt.Type(projects[1].Name)
	tt.Key(0, ui.KeyEnter)
	for _, id := range ids {
		if a.data.Issues[id].ProjectID != pid {
			t.Fatal("batch project missed issue")
		}
	}
	a.setLayout("board")
	tt.Frame()
	i := a.data.Issues[ids[0]]
	from, ok := tt.Find(i.Code() + " " + i.Title)
	if !ok {
		t.Fatal("source card absent")
	}
	to, ok := tt.Find("Board column Todo")
	if !ok {
		t.Fatal("target column absent")
	}
	tt.Press(from.X+from.W/2, from.Y+from.H/2)
	tt.Move(from.X+from.W/2+12, from.Y+from.H/2)
	tt.Move(to.X+to.W/2, to.Y+to.H-30)
	tt.Release(to.X+to.W/2, to.Y+to.H-30)
	for _, id := range ids {
		if a.data.Issues[id].Status != "todo" {
			t.Fatal("drag did not move full selection")
		}
	}
}
func TestClipboardAndDeepLinkRouting(t *testing.T) {
	a := testApp()
	i := a.data.Issues[a.visibleIssueIDs()[0]]
	a.openIssue(i.ID)
	tt := ui.NewTester(a.view, 1360, 880)
	a.copyIssue(i, "url")
	if tt.Clipboard() != issueURL(i.ID) {
		t.Fatal("wrong copied URL")
	}
	a.copyIssue(i, "prompt")
	if !strings.Contains(tt.Clipboard(), i.Title) || !strings.Contains(tt.Clipboard(), i.Plain[:12]) {
		t.Fatal("prompt missing content")
	}
	a.navigate("settings", "")
	if e := a.openLink(issueURL(i.ID)); e != nil || a.issueID != i.ID {
		t.Fatalf("deep link failed %v", e)
	}
	a.ready = false
	a.pendingLink = ""
	if e := a.openLink(issueURL(i.ID)); e != nil || a.pendingLink == "" {
		t.Fatal("cold-start link not deferred")
	}
	if e := a.openLink("https://example.com/issue/1"); e == nil {
		t.Fatal("accepted wrong scheme")
	}
}
