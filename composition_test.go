package main

import (
	"flow/internal/core"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func TestCreationDraftSurvivesCloseRestartAndTemplate(t *testing.T) {
	a := testApp()
	tt := ui.NewTester(a.view, 1000, 680)
	click(t, tt, "New issue")
	click(t, tt, "New issue title")
	tt.Type("恢复草稿")
	click(t, tt, "New issue description")
	tt.Type("正文不会丢失")
	click(t, tt, "Expand / collapse")
	tt.Frame()
	tt.Frame()
	click(t, tt, "Close create dialog")
	id := a.composeID
	if a.hasDirty() || a.data.Drafts[id].Form.Plain != "正文不会丢失" {
		t.Fatalf("draft not saved: %v / %s", a.hasDirty(), a.err)
	}
	path := filepath.Join(t.TempDir(), "flow.db")
	s, e := core.Open(path)
	if e != nil {
		t.Fatal(e)
	}
	_, e = s.Update(func(d *core.Data) error { *d = a.data.Clone(); return nil })
	if e != nil {
		t.Fatal(e)
	}
	s.Close()
	s, e = core.Open(path)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	b := newApp(s.Snapshot())
	tb := ui.NewTester(b.view, 1000, 680)
	click(t, tb, "New issue")
	if b.newIssue.Title != "恢复草稿" || b.newDoc.Text().PlainText() != "正文不会丢失" {
		t.Fatal("restart recovery lost text")
	}
	b.startTemplate("", b.compositionForm())
	tb.Frame()
	click(t, tb, "Template name")
	tb.Command("selectAll")
	tb.Type("常用模版")
	click(t, tb, "Save template")
	var templateID string
	for key := range b.data.Templates {
		templateID = key
	}
	if templateID == "" {
		t.Fatalf("template missing: %s", b.err)
	}
	b.useTemplate(templateID)
	tb.Frame()
	b.closeComposition()
	tb.Frame()
	if len(b.data.Drafts) != 2 {
		t.Fatalf("template replaced old draft: %d", len(b.data.Drafts))
	}
	b.resumeDraft(id, false)
	tb.Frame()
	click(t, tb, "Create issue")
	if _, ok := b.data.Drafts[id]; ok {
		t.Fatal("submitted draft remains")
	}
	if b.data.Issues[b.createdIssue].Plain != "正文不会丢失" {
		t.Fatal("submit lost body")
	}
	capture(t, tb, "13-drafts-compact")
}
func TestDraftAutosaveDiscardAndDeepChildCreation(t *testing.T) {
	a := testApp()
	tt := ui.NewTester(a.view, 1360, 880)
	a.newTask("", "backlog")
	tt.Frame()
	click(t, tt, "New issue title")
	tt.Type("Autosave")
	tt.Advance(time.Second)
	id := a.composeID
	if a.data.Drafts[id].ID == "" {
		t.Fatal("debounced draft not saved")
	}
	click(t, tt, "Discard draft")
	tt.Advance(time.Second)
	if len(a.data.Drafts) != 0 || a.newIssue.Title != "" {
		t.Fatal("discard resurrected draft")
	}
	a.closeComposition()
	a.newTask(a.filtered()[0].ID, "backlog")
	a.newIssue.Title = "Child"
	a.createIssue()
	child := a.createdIssue
	a.newTask(child, "todo")
	a.newIssue.Title = "Grandchild"
	a.createIssue()
	if a.data.Issues[a.createdIssue].ParentID != child {
		t.Fatal("deep child missing")
	}
}
func TestDeletedIssueEditsRecoverWithoutResurrecting(t *testing.T) {
	a := testApp()
	id := a.filtered()[0].ID
	a.openIssue(id)
	d := a.drafts[id]
	d.doc.InsertText("未保存的本地内容")
	a.touch(d, time.Now())
	external := a.data.Clone()
	external.TrashIssue(id)
	a.acceptExternal(external)
	a.saveDraft(id)
	if a.data.Issues[id].DeletedAt == 0 || len(a.data.Drafts) != 1 || a.hasDirty() {
		t.Fatalf("recovery failed: %s", a.err)
	}
	if a.data.Drafts[d.recoveryID].Form.Plain != d.doc.Text().PlainText() {
		t.Fatal("recovery text missing")
	}
	a.resumeDraft(d.recoveryID, true)
	a.newIssue.Title = "Recovered copy"
	a.createIssue()
	if a.data.Issues[a.createdIssue].Plain != d.doc.Text().PlainText() {
		t.Fatal("copy did not recover text")
	}
}

func TestCreationSubmissionPreservesSubsequentEdits(t *testing.T) {
	for _, mode := range []string{"same draft", "switch draft", "detach then switch"} {
		t.Run(mode, func(t *testing.T) {
			a := testApp()
			s, e := core.Open(filepath.Join(t.TempDir(), "submit.db"))
			if e != nil {
				t.Fatal(e)
			}
			defer s.Close()
			_, e = s.Update(func(d *core.Data) error { *d = a.data.Clone(); return nil })
			if e != nil {
				t.Fatal(e)
			}
			a.window = &mygo.Window{}
			source := a.filtered()[0].ID
			a.newRelatedTask(source, "blocking_issue")
			a.newIssue.Title = "submitted"
			id := a.composeID
			a.createIssue()
			a.createIssue() // Duplicate key events while saving create only one issue.
			a.newIssue.Title = "newer text"
			a.newDoc.InsertText("继续输入的正文")
			if mode == "detach then switch" {
				a.detachCreationRelation()
			}
			if mode != "same draft" {
				a.newTask("", "todo")
			} else {
				a.detachCreationRelation()
				a.closeComposition()
			}
			for n := 0; a.pending > 0 && n < 10; n++ {
				j := <-a.jobs
				data, e := j.run(s)
				j.done(data, e)
			}
			if a.pending != 0 || a.err != "" {
				t.Fatal("write did not settle", a.err)
			}
			data := s.Snapshot()
			if len(data.Issues) != len(testApp().data.Issues)+1 || data.Issues[a.createdIssue].Title != "submitted" {
				t.Fatal("submission changed or repeated")
			}
			v := data.Drafts[id]
			if v.Form.Title != "newer text" || v.Form.Plain != "继续输入的正文" {
				t.Fatal("newer edits lost", v)
			}
			if mode == "switch draft" && (v.Relation == nil || v.Relation.Kind != "blocking_issue") || mode != "switch draft" && v.Relation != nil {
				t.Fatal("newer relation lost")
			}
			if mode == "same draft" && a.newOpen {
				t.Fatal("submit reopened closed dialog")
			}
		})
	}
}

func TestCreationDraftQueuedSaveKeepsLastEditAfterSwitch(t *testing.T) {
	a := testApp()
	s, e := core.Open(filepath.Join(t.TempDir(), "queue.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	_, e = s.Update(func(d *core.Data) error { *d = a.data.Clone(); return nil })
	if e != nil {
		t.Fatal(e)
	}
	a.window = &mygo.Window{}
	a.newTask("", "backlog")
	id := a.composeID
	a.newIssue.Title = "first"
	a.persistComposition()
	a.newIssue.Title = "last edit before switching"
	a.newTask("", "todo")
	for n := 0; a.pending > 0 && n < 10; n++ {
		j := <-a.jobs
		data, e := j.run(s)
		j.done(data, e)
	}
	if a.pending != 0 || a.hasDirty() || s.Snapshot().Drafts[id].Form.Title != "last edit before switching" {
		t.Fatalf("queued save lost latest edit: %s", a.err)
	}
}
