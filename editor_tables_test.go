package main

import (
	"encoding/json"
	"flow/internal/core"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/richtext"
)

func TestTableCreateControlsRecoverAndReading(t *testing.T) {
	a := testApp()
	a.newTask("", "backlog")
	a.newIssue.Title = "可编辑表格验收"
	tt := ui.NewTester(a.view, 1120, 880)
	click(t, tt, "Expand / collapse")
	click(t, tt, "New issue description")
	tt.Type("/")
	click(t, tt, "Search block types")
	tt.Type("table")
	tt.Key(0, ui.KeyEnter)
	if info, ok := a.newDoc.Text().TableAt(0); !ok || info.Rows != 3 || info.Columns != 3 {
		t.Fatal("slash table", a.newDoc.Text().Markdown())
	}
	tt.Command("undo")
	if a.newDoc.Text().String() != "/" {
		t.Fatal("slash table undo")
	}
	tt.Command("redo")
	tt.SetClipboardData(transfer.New(transfer.NewItem(transfer.Bytes(transfer.Text, []byte("名称\t状态\t备注\n编辑器\t完成\t支持中文😀\n本地 MCP\t可用\t离线存储")))))
	tt.Command("paste")
	click(t, tt, "Table actions")
	click(t, tt, "Insert column right")
	if info, _ := a.newDoc.Text().TableAt(0); info.Columns != 4 {
		t.Fatal("column menu")
	}
	click(t, tt, "Table actions")
	click(t, tt, "Delete current column")
	click(t, tt, "Table actions")
	click(t, tt, "Insert row below")
	if info, _ := a.newDoc.Text().TableAt(0); info.Rows != 4 {
		t.Fatal("row menu")
	}
	click(t, tt, "Table actions")
	click(t, tt, "Delete current row")
	click(t, tt, "Table actions")
	click(t, tt, "Toggle header column") // HTML fallback must render without recursion.
	click(t, tt, "Continue after table")
	tt.Type("表格之后")
	want := a.newDoc.Text()
	tt.Advance(time.Second)
	if a.err != "" {
		t.Fatal(a.err)
	}
	capture(t, tt, "38-table-editor")
	path := filepath.Join(t.TempDir(), "flow.db")
	s, err := core.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Update(func(d *core.Data) error { *d = a.data.Clone(); return nil }); err != nil {
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
		t.Fatal("SQLite recovery lost grid")
	}
	view := ui.NewTester(b.view, 1120, 880)
	click(t, view, "Create issue")
	view.Advance(time.Second) // the toast slides in before its action can be pressed
	click(t, view, "Open issue")
	var saved richtext.Text
	if err := json.Unmarshal(b.data.Issues[b.issueID].Body, &saved); err != nil {
		t.Fatal(err)
	}
	if !saved.Equal(want) || !view.HasText("支持中文😀") || !view.HasText("离线存储") {
		t.Fatal("reading lost table", view.Texts())
	}
	if b.drafts[b.issueID].editing {
		t.Fatal("reading entered edit mode")
	}
	capture(t, view, "38-table-reading")
}

func TestTableBlockActionsKeepWholeGrid(t *testing.T) {
	text := richtext.ParseHTML(`<p>before</p><table><tr><th>A</th><th>B</th></tr><tr><td>C</td><td>D</td></tr></table><p>after</p>`)
	doc := richtext.NewDocument(text)
	doc.SetSelection(13, 13) // last cell
	moveEditorBlock(doc, "down")
	if !strings.HasPrefix(doc.Text().String(), "before\nafter\nA\nB\nC\nD") {
		t.Fatal(doc.Text().Markdown())
	}
	doc.Undo()
	moveEditorBlock(doc, "duplicate")
	if strings.Count(doc.Text().HTML(), "<table>") != 2 {
		t.Fatal("duplicate split cells", doc.Text().HTML())
	}
	moveEditorBlock(doc, "delete")
	if !doc.Text().Equal(text) {
		t.Fatal("delete split grid")
	}
	doc.SetSelection(doc.Text().Len(), doc.Text().Len())
	moveEditorBlock(doc, "up")
	if !strings.HasPrefix(doc.Text().String(), "before\nafter\nA\nB\nC\nD") {
		t.Fatal("move across previous grid", doc.Text().HTML())
	}
}
