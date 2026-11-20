package main

import (
	"flow/internal/core"
	"github.com/egoist/mygo/ui"
	"testing"
)

func TestSearchNavigationUsesGlobalContextAndHistory(t *testing.T) {
	a := testApp()
	projects := a.data.ProjectList(false, false)
	a.navigate("project", projects[0].ID)
	var target core.Issue
	for _, i := range a.data.Issues {
		if i.ProjectID == projects[1].ID {
			target = i
			break
		}
	}
	tt := ui.NewTester(a.view, 1360, 880)
	tt.Key(ui.Cmd, ui.KeyK)
	tt.Type(target.Code())
	tt.Key(0, ui.KeyEnter)
	if a.issueID != target.ID || a.projectID != "" || a.pageTitle() != projects[1].Name {
		t.Fatalf("incorrect global context: %s %s %s", a.issueID, a.projectID, a.pageTitle())
	}
	if n, count := a.detailPosition(); n != 0 || count != 1 {
		t.Fatalf("position %d/%d", n, count)
	}
	click(t, tt, "Go back")
	if a.issueID != "" || a.query != target.Code() && a.query != "flw-"+target.Code()[4:] {
		t.Fatal("back did not restore result list")
	}
	click(t, tt, "Go back")
	if a.projectID != projects[0].ID {
		t.Fatal("back did not restore originating project")
	}
	click(t, tt, "Go forward")
	click(t, tt, "Go forward")
	if a.issueID != target.ID {
		t.Fatal("forward lost issue")
	}
}
func TestFilteringPrunesHiddenSelectionAndTrashIsReadOnly(t *testing.T) {
	a := testApp()
	ids := a.visibleIssueIDs()
	target := ids[0]
	a.selected[target] = true
	a.query = "no such task"
	tt := ui.NewTester(a.view, 1360, 880)
	if len(a.selected) != 0 {
		t.Fatal("hidden selection survived filter")
	}
	a.query = ""
	a.trashIssue(target)
	a.navigate("trash", "")
	a.openIssue(target)
	tt.Frame()
	if _, ok := tt.Find("Issue title"); ok {
		t.Fatal("trash exposes title editor")
	}
	if _, ok := tt.Find("＋ Add sub-issue"); ok {
		t.Fatal("trash exposes child creation")
	}
	i := a.data.Issues[target]
	i.Title = "not allowed"
	if e := a.data.PutIssue(i); e == nil {
		t.Fatal("core allowed trash edit")
	}
	click(t, tt, "Restore issue")
	tt.Frame()
	if a.data.Issues[target].DeletedAt != 0 {
		t.Fatal("restore failed")
	}
	if _, ok := tt.Find("Issue title"); !ok {
		t.Fatal("restore did not enable editor")
	}
}
func TestLegacyTabRecordsKeepIndependentNavigationAndFilters(t *testing.T) {
	a := testApp()
	tt := ui.NewTester(a.view, 1360, 880)
	a.query = "中文"
	a.statusFilter = "todo"
	tt.Frame()
	a.newTab()
	if a.query != "" || a.statusFilter != "" {
		t.Fatal("new tab inherited filter")
	}
	a.navigate("settings", "")
	tt.Frame()
	a.selectTab(0)
	if a.query != "中文" || a.statusFilter != "todo" {
		t.Fatal("tab lost filter")
	}
	a.selectTab(1)
	if a.page != "settings" {
		t.Fatal("tab lost route")
	}
	a.closeTab(1)
	if len(a.navigation.Tabs) != 1 || a.query != "中文" {
		t.Fatal("closing active tab lost adjacent route")
	}
	tt.Frame()
	tt.Frame()
}
