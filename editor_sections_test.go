package main

import (
	"encoding/json"
	"flow/internal/core"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/richtext"
)

func TestCollapsibleCreateRecoverAndReadWithoutEditing(t *testing.T) {
	a := testApp()
	a.newTask("", "backlog")
	a.newIssue.Title = "折叠内容验收"
	tt := ui.NewTester(a.view, 1120, 880)
	click(t, tt, "New issue description")
	tt.Type("/")
	click(t, tt, "Search block types")
	tt.Type("collapsible")
	tt.Key(0, ui.KeyEnter)
	if !a.newDoc.Text().ParagraphStyleAt(0).SectionTitle {
		t.Fatal("slash did not create section")
	}
	tt.Command("undo")
	if a.newDoc.Text().String() != "/" {
		t.Fatal("slash did not undo atomically")
	}
	tt.Command("redo")
	tt.Type("验收清单")
	tt.Key(0, ui.KeyEnter)
	tt.Type("保留的正文😀")
	tt.Key(ui.Shift, ui.KeyEnter)
	tt.Type("同一段第二行")
	click(t, tt, "Toggle collapsible section")
	want := a.newDoc.Text()
	if !want.ParagraphStyleAt(0).Collapsed {
		t.Fatal("toolbar did not collapse body")
	}
	tt.Advance(time.Second)
	if a.err != "" {
		t.Fatal(a.err)
	}
	path := filepath.Join(t.TempDir(), "flow.db")
	s, err := core.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Update(func(d *core.Data) error { *d = a.data.Clone(); return nil }); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = core.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	b := newApp(s.Snapshot())
	b.newTask("", "backlog")
	if !b.newDoc.Text().Equal(want) {
		t.Fatal("SQLite draft recovery lost closed section")
	}
	view := ui.NewTester(b.view, 1120, 880)
	capture(t, view, "37-collapsible-editor-closed")
	click(t, view, "Create issue")
	view.Advance(time.Second) // the toast slides in before its action can be pressed
	click(t, view, "Open issue")
	i := b.data.Issues[b.issueID]
	var saved richtext.Text
	if err := json.Unmarshal(i.Body, &saved); err != nil {
		t.Fatal(err)
	}
	if !saved.Equal(want) || !strings.Contains(i.Plain, "保留的正文😀") {
		t.Fatal("submit lost hidden or searchable content")
	}
	if view.HasText("保留的正文😀") {
		t.Fatal("reading view starts open despite saved collapse")
	}
	click(t, view, "验收清单")
	view.Advance(time.Second)
	if b.drafts[b.issueID].editing {
		t.Fatal("disclosure click entered edit mode")
	}
	if !view.HasText("保留的正文😀") {
		t.Fatal("reading view did not reveal body", view.Texts())
	}
	if string(b.data.Issues[b.issueID].Body) != string(i.Body) {
		t.Fatal("reading disclosure changed stored document")
	}
	capture(t, view, "37-collapsible-reading-open")
	view.Key(0, ui.KeySpace)
	view.Advance(time.Second)
	if view.HasText("保留的正文😀") {
		t.Fatal("keyboard did not collapse reading view")
	}
}

func TestCollapsibleBlockActionsKeepWholeNestedGroup(t *testing.T) {
	text := richtext.ParseHTML(`<p>before</p><details><summary>outer</summary><p>body</p><details><summary>nested</summary><p>nested body</p></details></details><p>after</p>`)
	doc := richtext.NewDocument(text)
	doc.SetSelection(7, 7)
	moveEditorBlock(doc, "down")
	if !strings.HasPrefix(doc.Text().String(), "before\nafter\nouter\nbody\nnested\nnested body") {
		t.Fatal("move split section", doc.Text().String())
	}
	moveEditorBlock(doc, "up")
	if !doc.Text().Equal(text) {
		t.Fatal("moving back changed group")
	}
	moveEditorBlock(doc, "duplicate")
	if strings.Count(doc.Text().Markdown(), "<details>") != 4 {
		t.Fatal("duplicate omitted nested body")
	}
	moveEditorBlock(doc, "delete")
	if !doc.Text().Equal(text) {
		t.Fatal("delete did not remove complete copy", doc.Text().Markdown())
	}
	doc.Undo()
	if strings.Count(doc.Text().Markdown(), "<details>") != 4 {
		t.Fatal("delete undo lost nested body")
	}
	doc.SetText(text)
	doc.SetSelection(7, 7)
	if !convertSectionTitle(doc, "h1") || doc.Text().ParagraphStyleAt(7).Heading != 1 || doc.Text().String() != text.String() {
		t.Fatal("converting a section title lost content")
	}
	doc.Undo()
	if !doc.Text().Equal(text) {
		t.Fatal("conversion not one undo step")
	}
}
