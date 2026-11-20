package richtext

import (
	"strings"

	xhtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

func dividerParagraph() Paragraph {
	return Paragraph{Style: ParagraphStyle{Divider: true}, Runs: []Run{{Text: string(DividerRune)}}}
}

// aroundDivider gives typing, rich pastes and IME commits the same block
// boundary behavior. Replacing a selection still replaces the selected rule.
func (t Text) aroundDivider(a, z int, with Text) (Text, int) {
	caret := a + with.Len()
	if a != z || with.Len() == 0 || !t.ParagraphStyleAt(a).Divider || with.String() == "\n" {
		return t.aroundTable(a, z, with)
	}
	ps, _ := t.ParagraphRange(a)
	var parts []Paragraph
	if a > ps {
		parts = append(parts, Paragraph{})
		caret++
	}
	for _, p := range with.Paragraphs() {
		parts = append(parts, p)
	}
	if a == ps {
		parts = append(parts, Paragraph{})
	}
	return TextOf(parts...), caret
}

// InsertDivider replaces the selection with a horizontal divider on its own
// paragraph. Text on either side keeps its formatting; the caret moves to the
// paragraph after it. Insertion and splitting are one step of undo.
func (d *Document) InsertDivider() { d.Edit(func(e *Edit) { e.InsertDivider() }) }

// InsertDivider inserts a divider as part of this edit's undo transaction.
func (e *Edit) InsertDivider() {
	a, z := e.Selection()
	a, z = min(a, z), max(a, z)
	e.ReplaceString(a, z, "", Attributes{})
	t := e.Text()
	ps, pe := t.ParagraphRange(a)
	var parts []Paragraph
	if a > ps {
		for _, p := range t.Slice(ps, a).Paragraphs() {
			parts = append(parts, p)
		}
	}
	divider := dividerParagraph()
	divider.Style.SectionDepth = t.ParagraphStyleAt(a).SectionDepth
	parts = append(parts, divider)
	caret := ps
	for _, p := range parts {
		caret += TextOf(p).Len() + 1
	}
	for _, p := range t.Slice(a, pe).Paragraphs() {
		p.Style.ID, p.Style.Divider = "", false
		parts = append(parts, p)
	}
	e.Replace(ps, pe, TextOf(parts...))
	e.Select(caret, caret)
}

func placeholderBreak(n *xhtml.Node) bool {
	if _, explicit := hasAttr(n, "data-mygo-break"); explicit {
		return false
	}
	if n.Parent == nil || (n.Parent.DataAtom != atom.P && n.Parent.DataAtom != atom.Div) {
		return false
	}
	for c := n.Parent.FirstChild; c != nil; c = c.NextSibling {
		if c != n && !(c.Type == xhtml.TextNode && strings.TrimSpace(c.Data) == "") {
			return false
		}
	}
	return true
}
