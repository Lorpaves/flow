package richtext

import (
	"encoding/csv"
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"
	xhtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

const maxTableColumns = 64
const maxTableRows = 1024

func flattenCell(t Text, style ParagraphStyle) Paragraph {
	p := Paragraph{Style: style}
	for start, cell := range t.Paragraphs() {
		if start > 0 {
			p.Runs = append(p.Runs, Run{Text: string(LineBreak)})
		}
		p.Runs = append(p.Runs, cell.Runs...)
	}
	return p
}

// aroundTable keeps a pasted grid separate from surrounding prose. A cell's
// own insertion path retains its paragraph; nested grids are pasted as cells.
func (t Text) aroundTable(a, z int, with Text) (Text, int) {
	caret := a + with.Len()
	if t.ParagraphStyleAt(a).TableColumns > 0 {
		return with, caret
	}
	first, begins := with.TableAt(0)
	last, ends := with.TableAt(with.Len())
	if !begins && !ends {
		return with, caret
	}
	var parts []Paragraph
	start, _ := t.ParagraphRange(a)
	if begins && first.Start == 0 && a > start {
		parts = append(parts, Paragraph{Style: t.ParagraphStyleAt(a)})
		caret++
	}
	for _, p := range with.Paragraphs() {
		parts = append(parts, p)
	}
	if ends && last.End == with.Len() && (a == z || z < t.Len()) {
		st := t.ParagraphStyleAt(z)
		st.ID, st.SectionTitle, st.Collapsed, st.Divider = "", false, false, false
		parts = append(parts, Paragraph{Style: st})
		caret++
	}
	return TextOf(parts...), caret
}

// parseTableTSV accepts spreadsheet clipboard text, including quoted multiline
// cells. A lone line of prose is never promoted to a table.
func parseTableTSV(s string) (Text, bool) {
	if !strings.Contains(s, "\t") {
		return Text{}, false
	}
	r := csv.NewReader(strings.NewReader(normalizeNewlines(s)))
	r.Comma, r.FieldsPerRecord, r.LazyQuotes = '\t', -1, true
	rows, err := r.ReadAll()
	if err != nil || len(rows) == 0 || len(rows) > maxTableRows {
		return Text{}, false
	}
	cols := 0
	for _, row := range rows {
		cols = max(cols, len(row))
	}
	if cols > maxTableColumns {
		return Text{}, false
	}
	var cells []Paragraph
	for _, row := range rows {
		for c := 0; c < cols; c++ {
			p := Paragraph{}
			if c < len(row) {
				p.Runs = []Run{{Text: strings.ReplaceAll(row[c], "\n", string(LineBreak))}}
			}
			cells = append(cells, p)
		}
	}
	indexCells(cells, cols)
	return TextOf(cells...), true
}

// pasteTable overlays a clipboard grid at the active cell, growing the target
// without discarding cells outside the pasted rectangle. One transaction keeps
// expansion, content and caret together for undo.
func (e *Edit) pasteTable(with Text) bool {
	a, z := e.Selection()
	a, z = min(a, z), max(a, z)
	t := e.Text()
	dst, ok := t.TableAt(a)
	src, sourceOK := with.TableAt(0)
	if !ok || !sourceOK || z > dst.End || src.End != with.Len() || a == dst.Start && z == dst.End && a != z {
		return false
	}
	rows, cols := max(dst.Rows, dst.Row+src.Rows), max(dst.Columns, dst.Column+src.Columns)
	if rows > maxTableRows || cols > maxTableColumns {
		return false
	}
	depth := t.ParagraphStyleAt(a).SectionDepth
	cells := make([]Paragraph, rows*cols)
	for i := range cells {
		cells[i].Style.SectionDepth = depth
	}
	i := 0
	for _, p := range t.Slice(dst.Start, dst.End).Paragraphs() {
		cells[i/dst.Columns*cols+i%dst.Columns] = p
		i++
	}
	i = 0
	for _, p := range with.Paragraphs() {
		at := (dst.Row+i/src.Columns)*cols + dst.Column + i%src.Columns
		cells[at].Runs = p.Runs
		i++
	}
	indexCells(cells, cols)
	e.Replace(dst.Start, dst.End, TextOf(cells...))
	caret := dst.Start
	last := (dst.Row+src.Rows-1)*cols + dst.Column + src.Columns - 1
	for i := 0; i < last; i++ {
		caret += TextOf(cells[i]).Len() + 1
	}
	caret += TextOf(cells[last]).Len()
	e.Select(caret, caret)
	return true
}

// TableInfo identifies the table and the current cell. Start and End are
// rune offsets, excluding the table's trailing newline; row/column are zero-based.
type TableInfo struct{ Start, End, Rows, Columns, Row, Column int }

func tableEndOf(paras []Paragraph, start int) int {
	st := paras[start].Style
	if st.TableColumns == 0 || st.TableCell != 0 {
		return start
	}
	end := start
	for end < len(paras) {
		s := paras[end].Style
		if s.TableColumns != st.TableColumns || s.TableCell != end-start || s.SectionDepth != st.SectionDepth {
			break
		}
		end++
	}
	if (end-start)%st.TableColumns != 0 {
		return start
	}
	return end
}

func (t Text) tableAt(i int) (first, end, cols int, ok bool) {
	p := t.paraAt(i)
	s := seqAt(t.seq(), p).style
	if s.TableColumns == 0 {
		return
	}
	first, cols = p-s.TableCell, s.TableColumns
	if first < 0 {
		return 0, 0, 0, false
	}
	end = first
	for end < t.ParagraphCount() {
		x := seqAt(t.seq(), end).style
		if x.TableColumns != cols || x.TableCell != end-first || x.SectionDepth != s.SectionDepth {
			break
		}
		end++
	}
	ok = end > p && (end-first)%cols == 0
	return
}

// TableAt returns the table surrounding a rune, including its cell coordinates.
func (t Text) TableAt(i int) (TableInfo, bool) {
	first, end, cols, ok := t.tableAt(i)
	if !ok {
		return TableInfo{}, false
	}
	z := t.Len()
	if end < t.ParagraphCount() {
		z = t.paraStart(end) - 1
	}
	cell := t.paraAt(i) - first
	return TableInfo{Start: t.paraStart(first), End: z, Rows: (end - first) / cols, Columns: cols, Row: cell / cols, Column: cell % cols}, true
}

func clearTable(s *ParagraphStyle) { s.TableColumns, s.TableCell, s.TableHeader = 0, 0, false }

func indexCells(parts []Paragraph, cols int) {
	for i := range parts {
		parts[i].Style.TableColumns, parts[i].Style.TableCell = cols, i
	}
}

// InsertTable inserts a rectangular table, with a header row, as one undo step.
func (d *Document) InsertTable(rows, columns int) {
	d.Edit(func(e *Edit) { e.InsertTable(rows, columns) })
}

// InsertTable includes table creation in the current edit transaction.
func (e *Edit) InsertTable(rows, columns int) {
	rows, columns = max(1, min(rows, maxTableRows)), max(1, min(columns, maxTableColumns))
	a, z := e.Selection()
	a, z = min(a, z), max(a, z)
	if _, ok := e.Text().TableAt(a); ok {
		return
	} // no nested tables
	e.ReplaceString(a, z, "", Attributes{})
	t := e.Text()
	ps, pe := t.ParagraphRange(a)
	st := t.ParagraphStyleAt(a)
	if st.SectionTitle {
		e.RemoveSection()
		t = e.Text()
		st = t.ParagraphStyleAt(a)
	}
	var parts []Paragraph
	if a > ps {
		for _, p := range t.Slice(ps, a).Paragraphs() {
			parts = append(parts, p)
		}
	}
	cellStart := ps
	for _, p := range parts {
		cellStart += TextOf(p).Len() + 1
	}
	for i := 0; i < rows*columns; i++ {
		parts = append(parts, Paragraph{Style: ParagraphStyle{TableColumns: columns, TableCell: i, TableHeader: i < columns, SectionDepth: st.SectionDepth}})
	}
	for _, p := range t.Slice(a, pe).Paragraphs() {
		p.Style.ID = ""
		clearTable(&p.Style)
		parts = append(parts, p)
	}
	e.Replace(ps, pe, TextOf(parts...))
	e.Select(cellStart, cellStart)
}

// EditTable changes the table at the caret. Supported actions are row-before,
// row-after, column-before, column-after, delete-row, delete-column, header-row,
// header-column and delete. Dimensions remain rectangular, including empty cells.
func (d *Document) EditTable(action string) { d.Edit(func(e *Edit) { e.EditTable(action) }) }

// ExitTable places the caret in the following paragraph, creating it if needed.
func (d *Document) ExitTable() {
	d.Edit(func(e *Edit) {
		_, caret := e.Selection()
		t := e.Text()
		info, ok := t.TableAt(caret)
		if !ok {
			return
		}
		_, end, _, _ := t.tableAt(caret)
		if end < t.ParagraphCount() && seqAt(t.seq(), end).style.TableColumns == 0 {
			pos := t.paraStart(end)
			e.Select(pos, pos)
			return
		}
		st := t.ParagraphStyleAt(info.End)
		e.Replace(info.End, info.End, TextOf(Paragraph{Style: st}, Paragraph{}))
		e.FormatParagraphs(info.End+1, info.End+1, func(s *ParagraphStyle) { *s = ParagraphStyle{SectionDepth: st.SectionDepth} })
		e.Select(info.End+1, info.End+1)
	})
}

func (v *view) tableTab(back bool) bool {
	first, end, cols, ok := v.shown.tableAt(v.caret)
	if !ok {
		return false
	}
	p := v.shown.paraAt(v.caret)
	if back {
		if p == first {
			return false
		}
		p--
	} else {
		p++
		if p == end {
			if (end-first)/cols >= maxTableRows {
				return false
			}
			v.command(func(d *Document) { d.EditTable("row-after") })
			_, end, _, _ = v.shown.tableAt(v.caret)
			p = end - cols
		}
	}
	a := v.shown.paraStart(p)
	z := a + v.para(p).n
	v.moveTo(z, ui.TextUpstream, false)
	v.selAnchor = a
	return true
}

func (e *Edit) EditTable(action string) {
	_, caret := e.Selection()
	t := e.Text()
	info, ok := t.TableAt(caret)
	if !ok {
		return
	}
	var cells []Paragraph
	for _, p := range t.Slice(info.Start, info.End).Paragraphs() {
		cells = append(cells, p)
	}
	cols, rows := info.Columns, info.Rows
	row, col := info.Row, info.Column
	depth := cells[0].Style.SectionDepth
	blank := func() Paragraph { return Paragraph{Style: ParagraphStyle{SectionDepth: depth}} }
	switch action {
	case "row-before", "row-after":
		if rows >= maxTableRows {
			return
		}
		at := row
		if action == "row-after" {
			at++
		}
		added := make([]Paragraph, cols)
		for i := range added {
			added[i] = blank()
		}
		cells = slices.Insert(cells, at*cols, added...)
		row = at
		rows++
	case "column-before", "column-after":
		if cols >= maxTableColumns {
			return
		}
		at := col
		if action == "column-after" {
			at++
		}
		for r := rows - 1; r >= 0; r-- {
			p := blank()
			p.Style.TableHeader = cells[r*cols+col].Style.TableHeader
			cells = slices.Insert(cells, r*cols+at, p)
		}
		col = at
		cols++
	case "delete-row":
		cells = slices.Delete(cells, row*cols, (row+1)*cols)
		rows--
		row = min(row, rows-1)
	case "delete-column":
		for r := rows - 1; r >= 0; r-- {
			cells = slices.Delete(cells, r*cols+col, r*cols+col+1)
		}
		cols--
		col = min(col, cols-1)
	case "header-row":
		on := !cells[row*cols+col].Style.TableHeader
		for c := 0; c < cols; c++ {
			cells[row*cols+c].Style.TableHeader = on
		}
	case "header-column":
		on := !cells[row*cols+col].Style.TableHeader
		for r := 0; r < rows; r++ {
			cells[r*cols+col].Style.TableHeader = on
		}
	case "delete":
		cells = nil
	default:
		return
	}
	if len(cells) == 0 || rows == 0 || cols == 0 {
		e.Replace(info.Start, info.End, TextOf(blank()))
		e.FormatParagraphs(info.Start, info.Start, clearTable)
		e.Select(info.Start, info.Start)
		return
	}
	indexCells(cells, cols)
	e.Replace(info.Start, info.End, TextOf(cells...))
	// Replace can inherit the old final paragraph's style for an empty cell.
	last := e.Text().paraAt(info.Start) + len(cells) - 1
	lastAt := e.Text().paraStart(last)
	e.FormatParagraphs(lastAt, lastAt, func(s *ParagraphStyle) { id := s.ID; *s = cells[len(cells)-1].Style; s.ID = id })
	pos := info.Start
	for i := 0; i < row*cols+col; i++ {
		pos += TextOf(cells[i]).Len() + 1
	}
	e.Select(pos, pos)
}

func writeTableHTML(b *strings.Builder, cells []Paragraph) {
	cols := cells[0].Style.TableColumns
	b.WriteString("<table>")
	for i, p := range cells {
		if i%cols == 0 {
			b.WriteString("<tr>")
		}
		tag := "td"
		if p.Style.TableHeader {
			tag = "th"
		}
		b.WriteString("<" + tag)
		switch p.Style.Align {
		case ui.Center:
			b.WriteString(` align="center"`)
		case ui.End:
			b.WriteString(` align="right"`)
		}
		b.WriteString(">")
		writeRuns(b, p.Runs)
		b.WriteString("</" + tag + ">")
		if (i+1)%cols == 0 {
			b.WriteString("</tr>")
		}
	}
	b.WriteString("</table>")
}

func writeTableMarkdown(b *strings.Builder, cells []Paragraph) {
	cols := cells[0].Style.TableColumns
	for i, p := range cells {
		if p.Style.TableHeader != (i < cols) || p.Style.Align != cells[i%cols].Style.Align {
			writeTableHTML(b, cells)
			return
		}
	}
	for i, p := range cells {
		if i%cols == 0 {
			b.WriteString("| ")
		}
		var cell strings.Builder
		writeMarkdownRuns(&cell, p.Runs)
		b.WriteString(strings.ReplaceAll(cell.String(), "|", `\|`))
		b.WriteString(" |")
		if (i+1)%cols != 0 {
			b.WriteByte(' ')
			continue
		}
		if i+1 < len(cells) || i < cols {
			b.WriteByte('\n')
		}
		if i == cols-1 {
			b.WriteByte('|')
			for _, head := range cells[:cols] {
				switch head.Style.Align {
				case ui.Center:
					b.WriteString(" :---: |")
				case ui.End:
					b.WriteString(" ---: |")
				default:
					b.WriteString(" --- |")
				}
			}
			if i+1 < len(cells) {
				b.WriteByte('\n')
			}
		}
	}
}

// table imports native HTML tables as cells, keeping rich inline content.
// Multiple block paragraphs inside one cell become intra-cell line breaks.
func (b *htmlBuilder) table(n *xhtml.Node, a Attributes) {
	b.flush()
	var rows [][]Paragraph
	var visit func(*xhtml.Node)
	visit = func(n *xhtml.Node) {
		if n.DataAtom == atom.Tr {
			var row []Paragraph
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				if c.DataAtom != atom.Td && c.DataAtom != atom.Th {
					continue
				}
				cell := htmlBuilder{space: true}
				for child := c.FirstChild; child != nil; child = child.NextSibling {
					cell.node(child, a)
				}
				cell.flush()
				p := Paragraph{Style: ParagraphStyle{TableHeader: c.DataAtom == atom.Th, SectionDepth: b.style.SectionDepth}}
				applyCSS(attr(c, "style"), Attributes{}, &p.Style, true)
				switch strings.ToLower(attr(c, "align")) {
				case "center":
					p.Style.Align = ui.Center
				case "right":
					p.Style.Align = ui.End
				}
				for i, part := range cell.paras {
					if i > 0 {
						p.Runs = append(p.Runs, Run{Text: string(LineBreak)})
					}
					p.Runs = append(p.Runs, part.Runs...)
				}
				row = append(row, p)
				// Preserve a spanning cell's text once, filling its extra columns.
				span, _ := strconv.Atoi(attr(c, "colspan"))
				for k := 1; k < min(span, maxTableColumns); k++ {
					row = append(row, Paragraph{Style: p.Style})
				}
			}
			if len(row) > 0 {
				rows = append(rows, row)
			}
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if c.DataAtom != atom.Table {
				visit(c)
			}
		}
	}
	visit(n)
	cols := 0
	for _, row := range rows {
		cols = max(cols, len(row))
	}
	var cells []Paragraph
	for _, row := range rows {
		cells = append(cells, row...)
		for k := len(row); k < cols; k++ {
			cells = append(cells, Paragraph{Style: ParagraphStyle{SectionDepth: b.style.SectionDepth}})
		}
	}
	if cols <= maxTableColumns {
		indexCells(cells, cols)
	}
	b.paras = append(b.paras, cells...)
}
