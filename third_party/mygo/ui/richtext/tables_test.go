package richtext

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
)

func TestTableRoundTripAndPlainClipboard(t *testing.T) {
	sources := []Text{
		ParseMarkdown("| 名称😀 | 状态 |\n| :---: | ---: |\n| **one**<br>two | `a` |\n| empty | |"),
		ParseHTML(`<table><tr><td></td><td>second</td></tr><tr><th>header column</th><td><a href="https://example.com">link</a></td></tr></table>`),
		ParseHTML(`<details><summary>table</summary><table><tr><th>a</th><th>b</th></tr><tr><td>data</td><td></td></tr></table></details>`),
		ParseHTML(`<table><tr><th>one</th></tr></table>`),
	}
	for _, want := range sources {
		wire, _ := json.Marshal(want)
		var stored Text
		if err := json.Unmarshal(wire, &stored); err != nil {
			t.Fatal(err)
		}
		for format, got := range map[string]Text{"json": stored, "html": ParseHTML(want.HTML()), "markdown": ParseMarkdown(want.Markdown())} {
			encoded, _ := json.Marshal(got)
			if !bytes.Equal(encoded, wire) {
				t.Errorf("%s round trip\nwant %s\ngot %s\nmd %s", format, dump(want), dump(got), want.Markdown())
			}
		}
	}
	table := ParseMarkdown("| a | b |\n|---|---|\n| 1 | 2 |")
	if table.ParagraphCount() != 4 || table.PlainText() != "a\tb\n1\t2" {
		t.Fatal("cell model or plain clipboard", dump(table), table.PlainText())
	}
}

func TestTableStructuralActionsUndoAndEmptyCells(t *testing.T) {
	doc := NewDocument(NewText("beforeafter"))
	doc.SetSelection(6, 6)
	doc.InsertTable(2, 3)
	text := doc.Text()
	info, ok := text.TableAt(7)
	if !ok || info.Rows != 2 || info.Columns != 3 || text.String() != "before\n\n\n\n\n\n\nafter" {
		t.Fatalf("insert %s %+v", dump(text), info)
	}
	doc.EditTable("column-after")
	_, caret := doc.Selection()
	info, ok = doc.Text().TableAt(caret)
	if !ok || info.Columns != 4 || info.Column != 1 || info.Rows != 2 {
		t.Fatalf("column %+v %s", info, dump(doc.Text()))
	}
	doc.EditTable("row-after")
	_, caret = doc.Selection()
	info, ok = doc.Text().TableAt(caret)
	if !ok || info.Rows != 3 || info.Row != 1 {
		t.Fatal("row", info)
	}
	doc.EditTable("delete-column")
	doc.EditTable("delete-row")
	if !doc.Text().Equal(text) {
		t.Fatal("add/delete failed to preserve rectangular empty cells", dump(doc.Text()))
	}
	doc.EditTable("delete")
	if strings.Contains(doc.Text().HTML(), "<table>") {
		t.Fatal("delete retained table")
	}
	doc.Undo()
	if !doc.Text().Equal(text) {
		t.Fatal("table delete undo")
	}
}

func TestTableGridTypingNavigationAndWrapping(t *testing.T) {
	doc := NewDocument(Text{})
	doc.InsertTable(2, 2)
	tt, _ := richTester(t, doc)
	doc.SetSelection(0, 0)
	tt.Frame()
	tt.Type("名称😀")
	tt.Key(0, ui.KeyTab)
	tt.Type("状态")
	tt.Key(0, ui.KeyTab)
	tt.Type("第一行")
	tt.Key(0, ui.KeyEnter)
	tt.Type("第二行")
	tt.Key(0, ui.KeyTab)
	tt.Type("完成")
	text := doc.Text()
	info, ok := text.TableAt(0)
	if !ok || info.Rows != 2 || text.ParagraphCount() != 5 {
		t.Fatalf("typing broke cells %s", dump(text))
	}
	v := doc.shownIn.Value()
	x0, y0, _ := v.caretAt(0, 0, ui.TextDownstream)
	second := text.paraStart(1)
	x1, y1, _ := v.caretAt(second, 0, ui.TextDownstream)
	_, y2, _ := v.caretAt(text.paraStart(2), 0, ui.TextDownstream)
	if x1 <= x0 || y0 != y1 || y2 <= y0 {
		t.Fatalf("not a grid: (%v,%v),(%v,%v), next %v", x0, y0, x1, y1, y2)
	}
	_, row2y, _ := v.caretAt(text.paraStart(3), 0, ui.TextDownstream)
	if row2y != y2 {
		t.Fatal("row cells misaligned")
	}
	if got, _ := v.positionAt(x1+2, y1+5); text.paraAt(got) != 1 {
		t.Fatal("hit test chose wrong cell", got)
	}
	doc.SetSelection(text.paraStart(3), text.paraStart(3))
	tt.Frame()
	tt.Key(0, ui.KeyBackspace)
	if !doc.Text().Equal(text) {
		t.Fatal("Backspace merged cells")
	}
	tt.Key(ui.Cmd, ui.KeyDown)
	doc.SetSelection(info.End, info.End)
	tt.Frame()
	tt.Key(0, ui.KeyTab)
	_, caret := doc.Selection()
	more, ok := doc.Text().TableAt(caret)
	if !ok || more.Rows != 3 || more.Row != 2 || more.Column != 0 {
		t.Fatal("last-cell Tab did not add row", more)
	}
	tt.Command("undo")
	if !doc.Text().Equal(text) {
		t.Fatal("row insertion undo")
	}
	doc.ExitTable()
	tt.Frame()
	tt.Type("outside")
	if doc.Text().ParagraphStyleAt(doc.Text().Len()).TableColumns != 0 {
		t.Fatal("could not leave table")
	}
}

func TestTableRichPasteAndCrossCellSelectionKeepGrid(t *testing.T) {
	doc := NewDocument(ParseMarkdown("| a | b |\n|---|---|\n| c | d |"))
	tt, _ := richTester(t, doc)
	doc.SetSelection(4, 4)
	tt.Frame()
	tt.SetClipboardData(transfer.New(transfer.NewItem(transfer.Bytes(transfer.HTML, []byte(`<p><b>X</b></p><p>Y</p>`)))))
	tt.Command("paste")
	if doc.Text().ParagraphCount() != 4 || !strings.Contains(doc.Text().String(), "X\u2028Y") {
		t.Fatal("paste split cells", dump(doc.Text()))
	}
	doc.Undo()
	doc.SetSelection(1, 5)
	tt.Frame()
	tt.Type("replace")
	if info, ok := doc.Text().TableAt(0); !ok || info.Rows != 2 || info.Columns != 2 {
		t.Fatal("selection replacement broke grid", dump(doc.Text()))
	}
	tt.Command("selectAll")
	tt.Command("copy")
	want := doc.Text()
	tt.Type("outside")
	tt.Command("selectAll")
	tt.Command("paste")
	if !doc.Text().Equal(want) {
		t.Fatal("table clipboard round trip", dump(doc.Text()))
	}
}

func TestTablePasteSplitsProseAndSpreadsheetPasteGrowsGrid(t *testing.T) {
	grid := ParseHTML(`<table><tr><th>a</th><th>b</th></tr><tr><td>c</td><td>d</td></tr></table>`)
	doc := NewDocument(NewText("beforeafter"))
	doc.SetSelection(6, 6)
	doc.Insert(grid)
	if got, ok := doc.Text().TableAt(7); !ok || got.Rows != 2 || got.Columns != 2 || doc.Text().String() != "before\na\nb\nc\nd\nafter" {
		t.Fatal("paste merged grid with prose", dump(doc.Text()))
	}
	doc.Undo()
	if doc.Text().String() != "beforeafter" {
		t.Fatal("paste undo")
	}
	doc.SetText(grid)
	doc.SetSelection(6, 6) // bottom-right cell
	tt, _ := richTester(t, doc)
	tt.SetClipboardData(transfer.New(transfer.NewItem(transfer.Bytes(transfer.Text, []byte("X\tY\n\"one\ntwo\"\tZ\n")))))
	tt.Command("paste")
	info, ok := doc.Text().TableAt(0)
	if !ok || info.Rows != 3 || info.Columns != 3 || doc.Text().PlainText() != "a\tb\t\nc\tX\tY\n\tone\ntwo\tZ" {
		t.Fatal("spreadsheet paste did not fill/grow cells", dump(doc.Text()))
	}
	tt.Command("undo")
	if !doc.Text().Equal(grid) {
		t.Fatal("spreadsheet undo did not restore dimensions and contents")
	}
	tt.Compose("ni", 2)
	tt.Compose("你", 1)
	tt.Compose("", 0)
	tt.Type("你")
	if got, ok := doc.Text().TableAt(0); !ok || got.Rows != 2 || got.Columns != 2 {
		t.Fatal("composition broke table", dump(doc.Text()))
	}
}
