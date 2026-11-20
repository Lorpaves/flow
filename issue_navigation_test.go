package main

import (
	"flow/internal/core"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestPeekPreservesRouteSelectionAndOpensWithEnter(t *testing.T) {
	for _, layout := range []string{"list", "board"} {
		t.Run(layout, func(t *testing.T) {
			a := testApp()
			a.setLayout(layout)
			tt := ui.NewTester(a.view, 1000, 680)
			tt.Key(0, ui.KeyJ)
			id := a.focusedIssue
			a.selected[id] = true
			before := a.currentRoute()
			history := len(a.navigation.Back)
			tt.Frame()
			tt.Key(0, ui.KeySpace)
			tt.Frame()
			if !a.peekOpen || a.peekID != id || a.issueID != "" || !reflect.DeepEqual(before.Selected, a.selected) || len(a.navigation.Back) != history {
				t.Fatal("peek changed navigation/selection", a.peekOpen, a.peekID, id)
			}
			if _, ok := tt.Find("Issue title"); ok {
				t.Fatal("peek exposed full editor")
			}
			if _, ok := tt.Find("Issue preview"); !ok {
				t.Fatal("peek card absent")
			}
			capture(t, tt, "26-peek-"+layout)
			tt.Key(0, ui.KeyEscape)
			tt.Frame()
			if a.peekOpen || a.query != before.Query || a.focusedIssue != id || !a.selected[id] {
				t.Fatal("peek dismiss lost list context")
			}
			tt.Key(0, ui.KeySpace)
			tt.Key(0, ui.KeyEnter)
			if a.issueID != id || a.peekOpen {
				t.Fatal("Enter did not open full issue")
			}
		})
	}
}

func TestRelatedTemplatePreservesParentProject(t *testing.T) {
	a := testApp()
	root := a.filtered()[0]
	var other string
	for id := range a.data.Projects {
		if id != root.ProjectID {
			other = id
			break
		}
	}
	if other == "" {
		t.Fatal("fixture needs two projects")
	}
	tmpl, e := a.data.SaveTemplate(core.IssueTemplate{Name: "Other project template", Form: core.Issue{Title: "New parent", ProjectID: other, Status: "todo"}})
	if e != nil {
		t.Fatal(e)
	}
	a.newRelatedTask(root.ID, "parent_issue")
	a.useTemplate(tmpl.ID)
	if a.newIssue.ProjectID != root.ProjectID || a.newRelation == nil {
		t.Fatal("template changed relation project")
	}
	a.createIssue()
	if a.err != "" || a.data.Issues[root.ID].ParentID != a.createdIssue {
		t.Fatal("parent template submission failed", a.err)
	}
}

func TestPeekClosesWhenExternalChangeRemovesAnchor(t *testing.T) {
	a := testApp()
	tt := ui.NewTester(a.view, 1000, 680)
	tt.Key(0, ui.KeyJ)
	id := a.focusedIssue
	tt.Key(0, ui.KeySpace)
	external := a.data.Clone()
	external.TrashIssue(id)
	a.acceptExternal(external)
	tt.Frame()
	tt.Frame()
	if a.peekOpen || !a.navigationActionsEnabled() {
		t.Fatal("missing anchor left modal navigation guard")
	}
	tt.Key(0, ui.KeyC)
	if !a.newOpen {
		t.Fatal("normal commands did not recover")
	}
}

func TestRelatedCreationDraftRecoveryAndDeletedTarget(t *testing.T) {
	a := testApp()
	id := a.filtered()[0].ID
	a.newRelatedTask(id, "blocking_issue")
	tt := ui.NewTester(a.view, 1000, 680)
	click(t, tt, "New issue title")
	tt.Type("关联创建草稿")
	click(t, tt, "New issue description")
	tt.Type("中文正文保留")
	click(t, tt, "Close create dialog")
	draftID := a.composeID
	if a.data.Drafts[draftID].Relation == nil {
		t.Fatal("related draft lost relation")
	}
	b := newApp(a.data.Clone())
	b.resumeDraft(draftID, false)
	tb := ui.NewTester(b.view, 1000, 680)
	if b.newRelation == nil || b.newRelation.Kind != "blocking_issue" {
		t.Fatal("restart did not recover link")
	}
	click(t, tb, "Create issue")
	created := b.createdIssue
	if created == "" || len(b.data.Relations) != 1 {
		t.Fatal("related submission failed", b.err)
	}
	for _, r := range b.data.Relations {
		if r.From != created || r.To != id {
			t.Fatal("wrong relation direction")
		}
	}
	b.newRelatedTask(id, "parent_issue")
	b.newIssue.Title = "不能丢失的父任务草稿"
	b.newDoc.InsertText("依赖目标已删除")
	b.data.TrashIssue(id)
	tb.Frame()
	click(t, tb, "Create issue")
	if b.err != "related_issue_unavailable" || b.newIssue.Title != "不能丢失的父任务草稿" {
		t.Fatal("deleted target discarded draft", b.err)
	}
	click(t, tb, "Remove creation relation")
	click(t, tb, "Create issue")
	if b.data.Issues[b.createdIssue].Title != "不能丢失的父任务草稿" || b.data.Issues[b.createdIssue].ParentID != "" {
		t.Fatal("could not recover as standalone", b.err)
	}
	capture(t, tb, "26-related-created")
}

func TestIssueHierarchyCopyAndDocumentActions(t *testing.T) {
	a := testApp()
	parent := a.filtered()[0]
	child, _ := a.data.AddIssue("子任务一", parent.ProjectID, parent.ID, "todo")
	other, _ := a.data.AddIssue("子任务二", parent.ProjectID, parent.ID, "todo")
	a.openIssue(child.ID)
	tt := ui.NewTester(a.view, 1360, 880)
	tt.Key(ui.Cmd|ui.Shift, ui.KeyUp)
	if a.issueID != parent.ID {
		t.Fatal("parent shortcut failed")
	}
	tt.Key(ui.Cmd|ui.Shift, ui.KeyDown)
	if !a.childOpen {
		t.Fatal("multiple children did not open chooser")
	}
	tt.Type(other.Title)
	tt.Key(0, ui.KeyEnter)
	if a.issueID != other.ID || a.childOpen {
		t.Fatal("child search navigation failed")
	}
	// Copy the live edit, including text not yet committed by autosave.
	d := a.drafts[other.ID]
	d.issue.Title = "本地未保存标题"
	d.doc.InsertText("正文")
	a.touch(d, time.Now())
	a.copyIssue(other, "title")
	if tt.Clipboard() != d.issue.Title {
		t.Fatal("copied stale title")
	}
	a.copyIssue(other, "markdown")
	if !strings.Contains(tt.Clipboard(), "# 本地未保存标题") || !strings.Contains(tt.Clipboard(), "正文") {
		t.Fatal("Markdown copy lost title/body")
	}
	a.editDocument("issue", other.ID, "")
	tt.Frame()
	// Shared issue document UI is the same persisted model used by MCP.
	a.documentForm.Name = "任务说明文档"
	a.documentText.InsertText("文档正文")
	a.saveDocument()
	tt.Frame()
	if len(a.data.Documents) != 1 {
		t.Fatal("issue document did not save", a.err)
	}
	for _, v := range a.data.Documents {
		if v.OwnerID != other.ID || v.OwnerKind != "issue" {
			t.Fatal("wrong document owner")
		}
	}
	if _, ok := tt.Find("Edit document: 任务说明文档"); !ok {
		t.Fatal("issue document not listed")
	}
}
