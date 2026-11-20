package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/richtext"
)

func TestSlashBlockKeyboardFocusAndUndo(t *testing.T) {
	a := testApp()
	a.newTask("", "backlog")
	a.newIssue.Title = "块编辑验收"
	tt := ui.NewTester(a.view, 1000, 680)
	click(t, tt, "New issue description")
	tt.Type("/")
	click(t, tt, "Search block types")
	tt.Type("Heading 2")
	tt.Key(0, ui.KeyEnter)
	if a.newDoc.Text().PlainText() != "" || a.newDoc.Text().ParagraphStyleAt(0).Heading != 2 {
		t.Fatalf("slash command failed: %s", a.newDoc.Text().Markdown())
	}
	tt.Type("中文标题")
	if a.newDoc.Text().PlainText() != "中文标题" {
		t.Fatal("focus did not return to editor")
	}
	tt.Command("undo")
	tt.Command("undo")
	if a.newDoc.Text().PlainText() != "/" || a.newDoc.Text().ParagraphStyleAt(0).Heading != 0 {
		t.Fatalf("slash should undo in one step: %s", a.newDoc.Text().Markdown())
	}
	if _, visible := tt.Find("Search block types"); visible {
		t.Fatal("undo reopened slash menu and stole redo focus")
	}
	// Escape only dismisses the block chooser and leaves both form and text intact.
	click(t, tt, "Block format")
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	tt.Frame()
	if !a.newOpen || a.newDoc.Text().PlainText() != "/" {
		t.Fatal("escape dismissed outer form")
	}
	a.newDoc.SetText(richtext.NewText(""))
	tt.Frame()
	click(t, tt, "New issue description")
	tt.Type("/")
	click(t, tt, "Search block types")
	tt.Type("Numbered")
	tt.Key(0, ui.KeyEnter)
	tt.Type("第一项")
	tt.Key(0, ui.KeyEnter)
	tt.Type("第二项")
	if !strings.Contains(a.newDoc.Text().Markdown(), "1. 第一项") {
		t.Fatalf("numbered list formatting: %s", a.newDoc.Text().Markdown())
	}
	tt.Advance(time.Second)
	click(t, tt, "Create issue")
	i := a.data.Issues[a.createdIssue]
	var saved richtext.Text
	if e := json.Unmarshal(i.Body, &saved); e != nil {
		t.Fatal(e)
	}
	if saved.ParagraphStyleAt(0).List != richtext.Numbers {
		t.Fatal("block styles not persisted")
	}
}
func TestParagraphMoveDuplicateDeletePreservesFormattingAndUndo(t *testing.T) {
	doc := richtext.NewDocument(richtext.ParseMarkdown("# 标题\n\n**重点**\n\n- [x] 事项"))
	original := doc.Text().Markdown()
	start, _ := doc.Text().ParagraphRange(3)
	doc.SetSelection(start, start)
	// Select the second paragraph by its actual rune offset, retaining rich runs.
	n := 0
	for offset := range doc.Text().Paragraphs() {
		if n == 1 {
			doc.SetSelection(offset, offset)
			break
		}
		n++
	}
	moveEditorBlock(doc, "up")
	if !strings.HasPrefix(doc.Text().Markdown(), "**重点**") {
		t.Fatal(doc.Text().Markdown())
	}
	doc.Undo()
	if doc.Text().Markdown() != original {
		t.Fatal("move undo lost styles")
	}
	moveEditorBlock(doc, "duplicate")
	if strings.Count(doc.Text().Markdown(), "**重点**") != 2 {
		t.Fatal("duplicate lost formatting")
	}
	moveEditorBlock(doc, "delete")
	if doc.Text().Markdown() != original {
		t.Fatal("delete left wrong paragraph")
	}
	doc.Undo()
	doc.Undo()
	if doc.Text().Markdown() != original {
		t.Fatal("compound undo failed")
	}
	empty := richtext.NewDocument(richtext.NewText("only"))
	moveEditorBlock(empty, "delete")
	if empty.Text().PlainText() != "" {
		t.Fatal("last paragraph not removed")
	}
	empty.Undo()
	if empty.Text().PlainText() != "only" {
		t.Fatal("last paragraph undo failed")
	}
}
func TestEditorLinkSelectionAndNestedDialogFocus(t *testing.T) {
	a := testApp()
	a.newTask("", "backlog")
	a.newIssue.Title = "Links"
	tt := ui.NewTester(a.view, 1000, 680)
	click(t, tt, "New issue description")
	tt.Type("参考文档")
	tt.Command("selectAll")
	click(t, tt, "Link")
	click(t, tt, "Text link URL")
	tt.Type("https://example.com/docs")
	click(t, tt, "Save link")
	if !strings.Contains(a.newDoc.Text().Markdown(), "[参考文档](https://example.com/docs)") {
		t.Fatalf("selection lost on link: %s", a.newDoc.Text().Markdown())
	}
	tt.Key(0, ui.KeyRight)
	tt.Type(" after")
	if !strings.Contains(a.newDoc.Text().PlainText(), "after") || !a.newOpen {
		t.Fatal("link dialog focus was not returned")
	}
	capture(t, tt, "17-editor-links-compact")
}
