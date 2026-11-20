package richtext

import "github.com/egoist/mygo/ui"

type tableCellLayout struct{ row, column, columns int }

func (v *view) cell(p int) tableCellLayout {
	if p >= 0 && p < len(v.tableCells) {
		return v.tableCells[p]
	}
	return tableCellLayout{}
}

func sameTable(a, b ParagraphStyle) bool {
	return a.TableColumns == b.TableColumns && a.TableCell == b.TableCell && a.TableHeader == b.TableHeader
}

func (v *view) resetTables() {
	cells := make([]tableCellLayout, len(v.paras))
	count := 0
	for p := 0; p < len(v.paras); p++ {
		if s := v.para(p).style; s.TableColumns == 0 || s.TableCell != 0 {
			continue
		}
		first, end, cols, ok := v.shown.tableAt(v.shown.paraStart(p))
		if !ok {
			continue
		}
		for q := first; q < end; q++ {
			c := q - first
			cells[q] = tableCellLayout{row: q - c%cols, column: c % cols, columns: cols}
			count++
		}
		p = end - 1
	}
	for p, c := range cells {
		if c != v.cell(p) {
			if v.paras[p].layout != nil {
				v.laid--
			}
			v.paras[p] = paraView{}
		}
	}
	if count == 0 {
		cells = nil
	}
	v.tableCells = cells
	v.rowHeights = make([]float32, len(cells))
}

func (v *view) cellWidth(st ParagraphStyle, columns int) float32 {
	return max(1, (v.width-float32(st.SectionDepth)*v.look.indent)/float32(columns))
}

func (v *view) cellInsets(p int, st ParagraphStyle) (left, right float32) {
	c := v.cell(p)
	if c.columns == 0 {
		return v.insets(st)
	}
	w := v.cellWidth(st, c.columns)
	pad := min(float32(8), w/4)
	left = float32(st.SectionDepth)*v.look.indent + float32(c.column)*w + pad
	right = v.width - left - w + 2*pad
	return
}

func (v *view) paragraphTop(p int) float64 {
	if c := v.cell(p); c.columns > 0 {
		return v.hs.top(c.row)
	}
	return v.hs.top(p)
}

func (v *view) paragraphHeight(p int) float32 {
	if v.hidden(p) {
		return 0
	}
	if c := v.cell(p); c.columns > 0 {
		if c.column == 0 {
			return v.rowHeights[c.row]
		}
		return 0
	}
	return v.paras[p].h
}

func (v *view) invalidateHeight(p int) {
	if c := v.cell(p); c.columns > 0 {
		if h := v.rowHeights[c.row]; h > 0 {
			v.hs.add(c.row, -float64(h), 1)
			v.rowHeights[c.row] = 0
		}
	} else if h := v.paras[p].h; h > 0 {
		v.hs.add(p, -float64(h), 1)
	}
}

func (v *view) measureHeight(p int, before, after float32) {
	if v.cell(p).columns > 0 {
		return
	}
	if before > 0 {
		v.hs.add(p, float64(after-before), 0)
	} else {
		v.hs.add(p, float64(after), -1)
	}
}

func (v *view) paraLayout(p int) *paraView {
	if v.hidden(p) {
		return &v.paras[p]
	}
	c := v.cell(p)
	if c.columns == 0 {
		return v.layoutParagraph(p)
	}
	height := float32(0)
	for q := c.row; q < c.row+c.columns; q++ {
		height = max(height, v.layoutParagraph(q).h)
	}
	previous := v.rowHeights[c.row]
	if previous > 0 {
		v.hs.add(c.row, float64(height-previous), 0)
	} else {
		v.hs.add(c.row, float64(height), -1)
	}
	v.rowHeights[c.row] = height
	return &v.paras[p]
}

func (v *view) paintCell(painter *ui.Painter, p int, x, y float32) {
	c := v.cell(p)
	if c.columns == 0 {
		return
	}
	st := v.para(p).style
	w := v.cellWidth(st, c.columns)
	box := ui.Rect{X: x + float32(st.SectionDepth)*v.look.indent + float32(c.column)*w, Y: y, W: w, H: v.rowHeights[c.row]}
	if st.TableHeader {
		painter.Fill(box, v.look.CodeBlockBackground, 0)
	}
	painter.Stroke(box, v.look.border, 0, 1)
}
