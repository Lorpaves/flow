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

func TestStructuredBreaksSaveRecoverAndSubmit(t *testing.T) {
	a := testApp()
	a.newTask("", "backlog")
	a.newIssue.Title = "分隔线与段内换行验收"
	tt := ui.NewTester(a.view, 1120, 880)
	click(t, tt, "New issue description")
	tt.Type("第一行😀")
	tt.Key(ui.Shift, ui.KeyEnter)
	tt.Type("同一段第二行")
	tt.Key(0, ui.KeyEnter)
	tt.Type("/")
	click(t, tt, "Search block types")
	tt.Type("divider")
	tt.Key(0, ui.KeyEnter)
	if !strings.Contains(a.newDoc.Text().Markdown(), "---") {
		t.Fatal("slash did not insert divider")
	}
	tt.Command("undo")
	if !strings.HasSuffix(a.newDoc.Text().String(), "/") {
		t.Fatal("slash command was not one undo step")
	}
	tt.Command("redo")
	tt.Type("后续正文")
	want := a.newDoc.Text()
	if want.ParagraphCount() != 3 || want.String() != "第一行😀\u2028同一段第二行\n—\n后续正文" {
		t.Fatalf("structure: %s", want.Markdown())
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
		t.Fatal("SQLite/recovered draft lost structure")
	}
	view := ui.NewTester(b.view, 1120, 880)
	capture(t, view, "36-divider-line-break-draft")
	click(t, view, "Create issue")
	i := b.data.Issues[b.createdIssue]
	var saved richtext.Text
	if err := json.Unmarshal(i.Body, &saved); err != nil {
		t.Fatal(err)
	}
	if !saved.Equal(want) || i.Plain != "第一行😀\n同一段第二行\n—\n后续正文" {
		t.Fatal("submit changed body or searchable plain text")
	}
	if _, err := s.Update(func(d *core.Data) error { *d = b.data.Clone(); return nil }); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(saved.Markdown(), "第一行😀<br>同一段第二行") {
		t.Fatal("Markdown export lost line break")
	}
}
