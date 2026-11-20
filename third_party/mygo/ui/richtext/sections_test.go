package richtext

import (
	"bytes"
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
)

const nestedSectionHTML = `<details open><summary><strong>外层😀</strong></summary><p>正文<br data-mygo-break>继续</p><ul><li>one</li><li>two</li></ul><hr><pre><code>code

last</code></pre><details><summary>内层</summary><p><a href="https://example.com">link</a></p><p><br></p></details><p>外层尾段</p></details><p>外面</p>`

func TestSectionRoundTripsKeepNestedBodyAndFormats(t *testing.T) {
	for _, source := range []string{nestedSectionHTML, `<details><summary></summary><p><br></p></details>`, `<details open><summary>empty</summary></details>`, `<details><summary>image</summary><p><img src="flow-attachment://abc" alt="image" width="16" height="16"></p></details>`} {
		want := ParseHTML(source)
		data, err := json.Marshal(want)
		if err != nil {
			t.Fatal(err)
		}
		var stored Text
		if err := json.Unmarshal(data, &stored); err != nil {
			t.Fatal(err)
		}
		for format, got := range map[string]Text{"json": stored, "html": ParseHTML(want.HTML()), "markdown": ParseMarkdown(want.Markdown())} {
			wire, err := json.Marshal(got)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(wire, data) {
				t.Errorf("%s\nwant %s\ngot %s\nmarkdown %s", format, dump(want), dump(got), want.Markdown())
			}
		}
	}
}

func TestSectionMultiParagraphPasteStaysInsideAndRevealsTitle(t *testing.T) {
	for _, at := range []int{0, 3, 6} {
		doc := NewDocument(ParseHTML(`<details><summary>title</summary><p>body</p></details><p>outside</p>`))
		tt, _ := richTester(t, doc)
		if at == 6 {
			doc.SetSelection(0, 0)
			doc.ToggleSection()
		}
		before := doc.Text()
		doc.SetSelection(at, at)
		tt.Frame()
		tt.SetClipboardData(transfer.New(transfer.NewItem(transfer.Bytes(transfer.HTML, []byte(`<p>first</p><p><b>second</b></p>`)))))
		tt.Command("paste")
		got := doc.Text()
		if got.ParagraphStyleAt(0).Collapsed || !got.ParagraphStyleAt(0).SectionTitle {
			t.Fatalf("paste hid content or lost title at %d: %s", at, dump(got))
		}
		for offset, p := range got.Paragraphs() {
			if strings.Contains(plainOf(p), "second") && (p.Style.SectionTitle || p.Style.SectionDepth != 1) {
				t.Fatalf("paste split section at %d/%d: %s", at, offset, dump(got))
			}
		}
		tt.Command("undo")
		if !doc.Text().Equal(before) {
			t.Fatal("paste undo was not atomic")
		}
	}
}

func TestSectionCreationEnterExitAndUndo(t *testing.T) {
	doc := NewDocument(Text{})
	tt, _ := richTester(t, doc)
	typeRunes(tt, ">>> ")
	if !doc.Text().ParagraphStyleAt(0).SectionTitle {
		t.Fatal("shortcut did not make a section")
	}
	tt.Command("undo")
	if doc.Text().String() != ">>> " {
		t.Fatal("shortcut is not one undo step")
	}
	tt.Command("redo")
	tt.Type("标题😀")
	tt.Key(0, ui.KeyEnter)
	tt.Type("隐藏正文")
	tt.Key(0, ui.KeyEnter)
	tt.Key(0, ui.KeyEnter)
	tt.Type("外部正文")
	want := doc.Text()
	if want.ParagraphCount() != 3 || want.ParagraphStyleAt(want.Len()).SectionDepth != 0 || want.ParagraphStyleAt(4).SectionTitle {
		t.Fatalf("Enter structure: %s", dump(want))
	}
	doc.SetSelection(0, 0)
	doc.ToggleSection()
	tt.Frame()
	if boxes := doc.VisibleParagraphs(); len(boxes) != 2 || boxes[0].Index != 0 || boxes[1].Index != 2 {
		t.Fatalf("hidden body still takes space: %+v", boxes)
	}
	if !strings.Contains(doc.Text().PlainText(), "隐藏正文") {
		t.Fatal("collapse removed body")
	}
	doc.Undo()
	if !doc.Text().Equal(want) {
		t.Fatal("fold undo changed content")
	}
}

func TestSectionEnterAtStartKeepsTitleWithBody(t *testing.T) {
	for _, open := range []string{"", " open"} {
		doc := NewDocument(ParseHTML(`<details` + open + `><summary>title</summary><p>body</p></details>`))
		tt, _ := richTester(t, doc)
		before := doc.Text()
		doc.SetSelection(0, 0)
		tt.Frame()
		tt.Key(0, ui.KeyEnter)
		got := doc.Text()
		if got.ParagraphStyleAt(0).SectionDepth != 0 || !got.ParagraphStyleAt(1).SectionTitle || got.ParagraphStyleAt(1).Collapsed != (open == "") || got.String() != "\ntitle\nbody" {
			t.Fatalf("Enter detached title: %s", dump(got))
		}
		tt.Command("undo")
		if !doc.Text().Equal(before) {
			t.Fatal("Enter undo")
		}
	}
}

func TestSectionNavigationAndDestructiveBoundaries(t *testing.T) {
	doc := NewDocument(ParseHTML(`<details><summary>title</summary><p>secret</p><details><summary>nested</summary><p>nested secret</p></details></details><p>tail</p>`))
	tt, _ := richTester(t, doc)
	closed := doc.Text()
	tail := closed.Len() - 4
	doc.SetSelection(5, 5)
	tt.Frame()
	tt.Key(0, ui.KeyRight)
	if _, caret := doc.Selection(); caret != tail {
		t.Fatalf("Right entered hidden body: %d", caret)
	}
	tt.Key(0, ui.KeyLeft)
	if _, caret := doc.Selection(); caret != 5 {
		t.Fatalf("Left entered hidden body: %d", caret)
	}
	tt.Key(0, ui.KeyDown)
	if _, caret := doc.Selection(); caret < tail {
		t.Fatalf("Down entered hidden body: %d", caret)
	}
	doc.SetSelection(tail, tail)
	tt.Frame()
	tt.Key(0, ui.KeyBackspace)
	if doc.Text().String() != closed.String() || doc.Text().ParagraphStyleAt(0).Collapsed {
		t.Fatal("Backspace deleted hidden content instead of revealing it")
	}
	tt.Command("undo")
	doc.SetSelection(5, 5)
	tt.Frame()
	tt.Key(0, ui.KeyDelete)
	if doc.Text().String() != closed.String() || doc.Text().ParagraphStyleAt(0).Collapsed {
		t.Fatal("Delete deleted hidden content instead of revealing it")
	}
	tt.Command("undo")
	doc.SetSelection(8, 8)
	tt.Frame()
	tt.Type("!")
	if doc.Text().String() != "title!"+closed.String()[5:] {
		t.Fatal("external hidden selection edited unseen body", doc.Text().String())
	}
	tt.Command("undo")
	tt.Command("selectAll")
	tt.Command("copy")
	tt.Type("replacement")
	tt.Command("selectAll")
	tt.Command("paste")
	if !doc.Text().Equal(closed) {
		t.Fatalf("copy lost collapsed content: %s", dump(doc.Text()))
	}
	doc.SetText(ParseHTML(`<details><summary>end</summary><p>hidden</p></details>`))
	doc.SetSelection(3, 3)
	tt.Frame()
	tt.Key(0, ui.KeyRight)
	tt.Key(0, ui.KeyDown)
	if _, caret := doc.Selection(); caret != 3 {
		t.Fatal("EOF entered hidden body", caret)
	}
	tt.Key(0, ui.KeyEnter)
	tt.Type("new")
	if doc.Text().ParagraphStyleAt(0).Collapsed || doc.Text().String() != "end\nnew\nhidden" {
		t.Fatalf("Enter did not reveal and add body: %s", dump(doc.Text()))
	}
}

func TestNestedSectionVisibilityAndReadOnlyDisclosure(t *testing.T) {
	doc := NewDocument(ParseHTML(nestedSectionHTML))
	tt := ui.NewTester(func(c *ui.Context) { Editor(c, doc, EditorOptions{ReadOnly: true}).Fill() }, 600, 800)
	v := doc.shownIn.Value()
	before := doc.Text()
	var hidden []int
	for i := range v.paras {
		if v.hidden(i) {
			hidden = append(hidden, i)
		}
	}
	if !slices.Equal(hidden, []int{9, 10}) {
		t.Fatal("nested fold", hidden, dump(before))
	}
	left, _ := v.insets(v.para(0).style)
	_, top, h := v.caretAt(0, 0, ui.TextDownstream)
	x, y := v.padX+left-v.look.indent/2, v.padY+float32(top)+h/2
	tt.Press(x, y)
	tt.Release(x, y)
	if boxes := doc.VisibleParagraphs(); len(boxes) != 2 || boxes[1].Index != 12 {
		t.Fatalf("outer read-only fold: %+v", boxes)
	}
	tt.Advance(time.Second)
	tt.Press(x, y)
	tt.Release(x, y)
	if v.hiddenCount != 2 {
		t.Fatal("nested state was not retained", v.hiddenCount)
	}
	if !doc.Text().Equal(before) || doc.CanUndo() {
		t.Fatal("read-only disclosure wrote document")
	}
}

func TestSectionUnwrapAndOrdinaryTypingKeepsFoldMap(t *testing.T) {
	doc := NewDocument(ParseHTML(nestedSectionHTML))
	tt, _ := richTester(t, doc)
	v := doc.shownIn.Value()
	mapStart := &v.foldOwners[0]
	tt.Type("!")
	if &v.foldOwners[0] != mapStart {
		t.Fatal("one character rescanned section structure")
	}
	before := doc.Text()
	doc.SetSelection(0, 0)
	doc.Edit(func(e *Edit) { e.RemoveSection() })
	tt.Frame()
	if doc.Text().String() != before.String() || doc.Text().ParagraphStyleAt(0).SectionTitle {
		t.Fatal("unwrap changed content")
	}
	for _, p := range doc.Text().Paragraphs() {
		if p.Style.SectionTitle && p.Style.SectionDepth != 1 {
			t.Fatal("nested section did not outdent")
		}
	}
	doc.Undo()
	if !doc.Text().Equal(before) {
		t.Fatal("unwrap undo")
	}
}
