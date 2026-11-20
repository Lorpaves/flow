package richtext

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
)

func TestLineBreakKeepsListItemAndCaretGeometry(t *testing.T) {
	doc := NewDocument(TextOf(Paragraph{Style: ParagraphStyle{List: Tasks, Checked: true, ID: "item"}, Runs: []Run{{Text: "第一行😀", Attributes: Attributes{Weight: 700}}}}))
	tt, _ := richTester(t, doc)
	tt.Key(ui.Shift, ui.KeyEnter)
	tt.Type("第二行")
	text := doc.Text()
	if text.ParagraphCount() != 1 || text.String() != "第一行😀\u2028第二行" || text.ParagraphStyleAt(0).ID != "item" || !text.ParagraphStyleAt(0).Checked {
		t.Fatalf("break changed list structure: %s", dump(text))
	}
	if !text.AttributesAt(text.Len() - 1).Bold() {
		t.Fatal("line break lost inline formatting")
	}
	v := doc.shownIn.Value()
	_, y1, _ := v.caretAt(0, 0, ui.TextDownstream)
	_, y2, _ := v.caretAt(text.Len(), 0, ui.TextDownstream)
	if y2 <= y1 {
		t.Fatal("soft line break did not lay out on another line")
	}
	tt.Key(ui.Cmd, ui.KeyLeft)
	_, caret := doc.Selection()
	if caret != 5 { // three characters, one emoji and one break
		t.Fatalf("Home on second visual line: %d", caret)
	}
	tt.Key(0, ui.KeyBackspace)
	if doc.Text().String() != "第一行😀第二行" {
		t.Fatalf("backspace removed more than the break: %q", doc.Text().String())
	}
	tt.Command("undo")
	if !doc.Text().Equal(text) {
		t.Fatal("undo did not restore break and styling")
	}
	doc.SetSelection(text.Len(), text.Len())
	tt.Frame()
	tt.Key(0, ui.KeyEnter)
	tt.Type("下一项")
	if doc.Text().ParagraphCount() != 2 || doc.Text().ParagraphStyleAt(doc.Text().Len()).Checked || doc.Text().ParagraphStyleAt(doc.Text().Len()).List != Tasks {
		t.Fatal("normal Enter did not create an unchecked next item")
	}
}

func TestLineBreakAndDividerRoundTrips(t *testing.T) {
	for _, source := range []string{"a  \nb", "- [x] **a**<br>*b*", "# 标题<br>第二行", "> a<br>b", "<br>", "a<br><br>", "a\n\n---\n\nb", "---", "> ---"} {
		t.Run(source, func(t *testing.T) {
			want := ParseMarkdown(source)
			encoded, err := json.Marshal(want)
			if err != nil {
				t.Fatal(err)
			}
			var stored Text
			if err := json.Unmarshal(encoded, &stored); err != nil {
				t.Fatal(err)
			}
			for format, got := range map[string]Text{"json": stored, "html": ParseHTML(want.HTML()), "markdown": ParseMarkdown(want.Markdown())} {
				if !got.Equal(want) {
					t.Errorf("%s changed structure\nwant %s\ngot %s\nhtml %s\nmd %s", format, dump(want), dump(got), want.HTML(), want.Markdown())
				}
			}
		})
	}
	if got := ParseRTF([]byte(`{\rtf1 one\line two\par next}`)); got.ParagraphCount() != 2 || got.String() != "one\u2028two\nnext" {
		t.Fatalf("RTF line vs paragraph lost: %s", dump(got))
	}
	if got := ParseHTML("<p><br></p><p>x</p>"); got.String() != "\nx" {
		t.Fatalf("empty HTML paragraph became line break: %q", got.String())
	}
}

func TestDividerInsertionUndoAndTypingAroundBlock(t *testing.T) {
	doc := NewDocument(ParseMarkdown("**beforeafter**"))
	original := doc.Text()
	doc.SetSelection(6, 6)
	doc.InsertDivider()
	want := "**before**\n\n---\n\n**after**"
	if got := doc.Text().Markdown(); got != want {
		t.Fatalf("split: %s", got)
	}
	_, caret := doc.Selection()
	if caret != 9 {
		t.Fatalf("caret is not after divider: %d", caret)
	}
	doc.Undo()
	if !doc.Text().Equal(original) {
		t.Fatal("insertion was not one undo step")
	}
	doc.Redo()
	tt, _ := richTester(t, doc)
	doc.SetSelection(7, 7) // before the divider
	tt.Frame()
	tt.Type("插入")
	tt.Type("正文")
	if !strings.Contains(doc.Text().String(), "插入正文\n—") {
		t.Fatalf("typing before divider hid text or lost caret: %q", doc.Text().String())
	}
	doc.SetText(ParseMarkdown("before\n\n---\n\nafter"))
	doc.SetSelection(9, 9)
	tt.Frame()
	tt.Key(0, ui.KeyBackspace)
	if doc.Text().String() != "before\nafter" {
		t.Fatalf("backspace left rule character: %q", doc.Text().String())
	}
	tt.Command("undo")
	doc.SetSelection(6, 6)
	tt.Frame()
	tt.Key(0, ui.KeyDelete)
	if doc.Text().String() != "before\nafter" {
		t.Fatalf("delete left rule character: %q", doc.Text().String())
	}
	doc.SetText(ParseMarkdown("---"))
	doc.SetSelection(1, 1)
	tt.Frame()
	tt.Type("after")
	if got := doc.Text().Markdown(); got != "---\n\nafter" {
		t.Fatalf("typing after divider: %s", got)
	}
}

func TestDividerShortcutClipboardAndReadOnly(t *testing.T) {
	doc := NewDocument(Text{})
	tt, _ := richTester(t, doc)
	typeRunes(tt, "___ ")
	if doc.Text().String() != "—\n" || !doc.Text().ParagraphStyleAt(0).Divider {
		t.Fatalf("shortcut: %s", dump(doc.Text()))
	}
	tt.Command("undo")
	if doc.Text().String() != "___ " {
		t.Fatal("shortcut conversion must undo separately from typed prefix")
	}
	tt.Command("redo")
	tt.Type("tail")
	original := doc.Text()
	tt.Command("selectAll")
	tt.Command("copy")
	tt.Type("replace")
	tt.Command("selectAll")
	tt.Command("paste")
	if !doc.Text().Equal(original) {
		t.Fatal("clipboard lost divider")
	}
	doc.SetText(NewText("abc"))
	doc.SetSelection(3, 3)
	ro := ui.NewTester(func(c *ui.Context) { Editor(c, doc, EditorOptions{ReadOnly: true}).Fill() }, 500, 400)
	ro.Press(40, 20)
	ro.Release(40, 20)
	ro.Key(ui.Shift, ui.KeyEnter)
	if doc.Text().String() != "abc" {
		t.Fatal("read-only accepted a break")
	}
}

func TestDividerRichPasteAndIMECommitKeepBoundary(t *testing.T) {
	for _, at := range []int{0, 1} {
		doc := NewDocument(ParseMarkdown("---"))
		tt, _ := richTester(t, doc)
		doc.SetSelection(at, at)
		tt.Frame()
		tt.SetClipboardData(transfer.New(transfer.NewItem(transfer.Bytes(transfer.HTML, []byte("<p><b>paste</b></p>")))))
		tt.Command("paste")
		want := "**paste**\n\n---"
		if at == 1 {
			want = "---\n\n**paste**"
		}
		if doc.Text().Markdown() != want {
			t.Fatalf("paste at %d: %s", at, doc.Text().Markdown())
		}
		tt.Command("undo")
		if doc.Text().Markdown() != "---" {
			t.Fatal("rich paste did not undo atomically")
		}
		doc.SetSelection(at, at)
		tt.Frame()
		v := doc.shownIn.Value()
		v.SetMarkedText(nil, "中文", ui.TextInputRange{Start: 2, End: 2})
		v.ReplaceText(nil, "中文")
		tt.Frame()
		want = "中文\n\n---"
		if at == 1 {
			want = "---\n\n中文"
		}
		if doc.Text().Markdown() != want {
			t.Fatalf("IME commit at %d: %s", at, doc.Text().Markdown())
		}
	}
	doc := NewDocument(ParseMarkdown("---"))
	tt, _ := richTester(t, doc)
	tt.Key(0, ui.KeyEnter)
	if doc.Text().String() != "—\n" {
		t.Fatal("Enter created extra paragraphs around divider")
	}
}
