package richtext

import (
	"strings"

	"github.com/egoist/mygo/ui"
)

func sectionEndOf(paras []Paragraph, start int) int {
	depth := paras[start].Style.SectionDepth
	end := start + 1
	for ; end < len(paras); end++ {
		s := paras[end].Style
		if s.SectionDepth < depth || s.SectionTitle && s.SectionDepth <= depth {
			break
		}
	}
	return end
}

func (t Text) sectionEnd(start int) int {
	depth := seqAt(t.seq(), start).style.SectionDepth
	end := start + 1
	for ; end < t.ParagraphCount(); end++ {
		s := seqAt(t.seq(), end).style
		if s.SectionDepth < depth || s.SectionTitle && s.SectionDepth <= depth {
			break
		}
	}
	return end
}

func (t Text) sectionAt(i int) (int, bool) {
	p := t.paraAt(i)
	st := seqAt(t.seq(), p).style
	if st.SectionDepth == 0 {
		return 0, false
	}
	for q := p; q >= 0; q-- {
		s := seqAt(t.seq(), q).style
		if s.SectionDepth == 0 {
			break
		}
		if s.SectionTitle && s.SectionDepth <= st.SectionDepth && t.sectionEnd(q) > p {
			return q, true
		}
	}
	return 0, false
}

// SectionRange returns the surrounding section, including its title and all
// nested content. Offsets exclude the final newline, as ParagraphRange does.
func (t Text) SectionRange(i int) (start, end int, ok bool) {
	p, ok := t.sectionAt(i)
	if !ok {
		return 0, 0, false
	}
	z := t.sectionEnd(p)
	end = t.Len()
	if z < t.ParagraphCount() {
		end = t.paraStart(z) - 1
	}
	return t.paraStart(p), end, true
}

// MakeCollapsible turns the selected paragraphs into a section. The first is
// its editable title; other selected paragraphs are its body. Enter after the
// title adds body text, and Enter on an empty body paragraph leaves the section.
func (d *Document) MakeCollapsible() { d.Edit(func(e *Edit) { e.MakeCollapsible() }) }

// MakeCollapsible makes a section in this edit's undo transaction.
func (e *Edit) MakeCollapsible() {
	a, z := e.Selection()
	a, z = min(a, z), max(a, z)
	pa, pz := e.Text().paragraphSpan(a, z)
	start := e.Text().paraStart(pa)
	if e.Text().ParagraphStyleAt(start).SectionTitle {
		return
	}
	depth := min(maxIndent, e.Text().ParagraphStyleAt(start).SectionDepth+1)
	for p := pa; p <= pz; p++ {
		at := e.Text().paraStart(p)
		e.FormatParagraphs(at, at, func(s *ParagraphStyle) {
			s.SectionDepth = max(depth, min(maxIndent, s.SectionDepth+1))
			if p == pa {
				s.SectionTitle, s.Collapsed = true, false
				s.Heading, s.Indent, s.Quote, s.Align = 0, 0, false, ui.Start
			}
		})
	}
	e.Select(a, a)
}

// RemoveSection unwraps the section at the caret while retaining its body
// and nested sections. It can share a transaction with a block conversion.
func (e *Edit) RemoveSection() {
	_, c := e.Selection()
	t := e.Text()
	p, ok := t.sectionAt(c)
	if !ok {
		return
	}
	for q, end := p, t.sectionEnd(p); q < end; q++ {
		at := t.paraStart(q)
		e.FormatParagraphs(at, at, func(s *ParagraphStyle) {
			s.SectionDepth--
			if q == p {
				s.SectionTitle, s.Collapsed = false, false
			}
		})
	}
}

// ToggleSection expands or collapses the section surrounding the caret.
// Collapsing moves a selection in its body to the title, so typing never edits
// content hidden by that action. The change is undoable and serializable.
func (d *Document) ToggleSection() {
	d.Edit(func(e *Edit) {
		a, c := e.Selection()
		t := e.Text()
		p, ok := t.sectionAt(c)
		if !ok {
			return
		}
		start := t.paraStart(p)
		closed := !t.ParagraphStyleAt(start).Collapsed
		e.FormatParagraphs(start, start, func(s *ParagraphStyle) { s.Collapsed = closed })
		if closed {
			end := t.Len() + 1
			if z := t.sectionEnd(p); z < t.ParagraphCount() {
				end = t.paraStart(z)
			}
			titleEnd := start + seqAt(t.seq(), p).n
			if a > titleEnd && a < end {
				a = titleEnd
			}
			if c > titleEnd && c < end {
				c = titleEnd
			}
			e.Select(a, c)
		}
	})
}

func writeSectionHTML(b *strings.Builder, paras []Paragraph) {
	if paras[0].Style.Collapsed {
		b.WriteString("<details>")
	} else {
		b.WriteString("<details open>")
	}
	b.WriteString("<summary>")
	writeRuns(b, paras[0].Runs)
	b.WriteString("</summary>")
	var body strings.Builder
	writeBlocks(&body, paras[1:])
	// A blank physical line ends a CommonMark HTML block, even inside pre.
	// Character references retain code newlines without breaking the section.
	b.WriteString(strings.ReplaceAll(body.String(), "\n", "&#10;"))
	b.WriteString("</details>")
}

func sameSection(a, b ParagraphStyle) bool {
	return a.SectionDepth == b.SectionDepth && a.SectionTitle == b.SectionTitle && a.Collapsed == b.Collapsed
}

func (v *view) hidden(p int) bool { return p >= 0 && p < len(v.foldOwners) && v.foldOwners[p] != 0 }

func (v *view) sectionClosed(p int) bool {
	if closed, ok := v.foldOverrides[v.para(p).id]; ok && v.opts.ReadOnly {
		return closed
	}
	return v.para(p).style.Collapsed
}

// resetFolds runs when paragraph structure changes, never for an ordinary
// character edit. Each hidden paragraph points to its visible outer title.
func (v *view) resetFolds() {
	owners := make([]int, len(v.paras))
	v.hiddenCount, v.sectionCount = 0, 0
	owner, depth := -1, 0
	seqEach(v.shown.seq(), 0, func(p int, para *richPara) bool {
		s := para.style
		if owner >= 0 && (s.SectionDepth < depth || s.SectionTitle && s.SectionDepth <= depth) {
			owner = -1
		}
		if s.SectionTitle {
			v.sectionCount++
		}
		if owner >= 0 {
			owners[p] = owner + 1
			v.hiddenCount++
		} else if s.SectionTitle && v.sectionClosed(p) {
			owner, depth = p, s.SectionDepth
		}
		if owners[p] != 0 || v.hidden(p) {
			if v.paras[p].layout != nil {
				v.laid--
			}
			v.paras[p] = paraView{}
		}
		return true
	})
	if v.sectionCount == 0 {
		owners = nil
	}
	v.foldOwners = owners
}

func (v *view) visiblePosition(i int) int {
	p := v.shown.paraAt(i)
	if v.hidden(p) {
		p = v.foldOwners[p] - 1
		return v.shown.paraStart(p) + v.para(p).n
	}
	return i
}

func (v *view) visibleParagraph(p int) int {
	if v.hidden(p) {
		return v.foldOwners[p] - 1
	}
	if cell := v.cell(p); cell.columns > 0 {
		return cell.row
	}
	return p
}

// A collapsed title is a single navigation stop, including at EOF.
func (v *view) skipFold(i int, forward bool) int {
	p := v.shown.paraAt(i)
	if v.hidden(p) {
		owner := v.foldOwners[p] - 1
		if forward {
			end := v.shown.sectionEnd(owner)
			if end < len(v.paras) {
				return v.shown.paraStart(end)
			}
		}
		return v.shown.paraStart(owner) + v.para(owner).n
	}
	return i
}

// Destructive keys at a folded boundary first reveal what they would erase.
// Explicit range selections (including Select All) still include all content.
func (v *view) revealDeletion(a, z int) bool {
	if v.selAnchor != v.caret || v.hiddenCount == 0 {
		return false
	}
	for p := v.shown.paraAt(a); p <= v.shown.paraAt(z); p++ {
		if !v.hidden(p) {
			continue
		}
		owner := v.foldOwners[p] - 1
		start := v.shown.paraStart(owner)
		v.edit(func(e *Edit) { e.FormatParagraphs(start, start, func(s *ParagraphStyle) { s.Collapsed = false }) })
		return true
	}
	return false
}

func (v *view) foldBox(x float32, y float64) (int, bool) {
	if v.sectionCount == 0 {
		return 0, false
	}
	p := v.hs.at(y)
	if v.hidden(p) || !v.para(p).style.SectionTitle {
		return 0, false
	}
	left, _ := v.insets(v.para(p).style)
	_, top, h := v.caretAt(v.shown.paraStart(p), 0, ui.TextDownstream)
	return p, x >= left-v.look.indent && x < left-2 && y >= top && y < top+float64(h)
}

func (v *view) toggleFold(p int) {
	if v.opts.ReadOnly {
		if v.foldOverrides == nil {
			v.foldOverrides = map[uint64]bool{}
		}
		v.foldOverrides[v.para(p).id] = !v.sectionClosed(p)
		v.resetFolds()
		v.resetHeights()
		return
	}
	v.command(func(d *Document) {
		d.SetSelection(v.shown.paraStart(p), v.shown.paraStart(p))
		d.ToggleSection()
	})
}
