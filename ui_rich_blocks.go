package main

import (
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
	"github.com/egoist/mygo/ui/richtext"
	"slices"
)

type richHTMLState struct {
	source string
	parts  []richtext.Paragraph
	open   map[int]*bool
}

// Markdown's HTML hook builds native disclosure controls, with no browser or
// executable HTML. Reading expands locally; only the editor changes saved state.
func (a *app) richHTMLView(c *ui.Context, source string) {
	root := ui.Column(c).Gap(8)
	state := ui.Local(root, "html-block", func() richHTMLState { return richHTMLState{} })
	if state.source != source || state.open == nil {
		*state = richHTMLState{source: source, open: map[int]*bool{}}
		for _, p := range richtext.ParseHTML(source).Paragraphs() {
			i := len(state.parts)
			state.parts = append(state.parts, p)
			if p.Style.SectionTitle {
				open := !p.Style.Collapsed
				state.open[i] = &open
			}
		}
	}
	var render func(start, end int)
	render = func(start, end int) {
		for i := start; i < end; {
			p := state.parts[i]
			if p.Style.TableColumns > 0 && p.Style.TableCell == 0 {
				last := editorBlockEnd(state.parts, i)
				cols := p.Style.TableColumns
				if (last-i)%cols == 0 {
					ui.Column(c.Key(i)).Gap(0).Children(func() {
						for row := i; row < last; row += cols {
							ui.Row(c.Key(row)).Gap(0).AlignItems(ui.Stretch).Children(func() {
								for col := 0; col < cols; col++ {
									cell := state.parts[row+col]
									box := ui.Column(c.Key(col)).Grow(1).Basis(0).MinWidth(0).Padding(8).Border(1, a.theme.Border).TextAlign(cell.Style.Align)
									if cell.Style.TableHeader {
										box.Background(a.theme.SurfaceHover)
										cell.Runs = slices.Clone(cell.Runs)
										for r := range cell.Runs {
											cell.Runs[r].Attributes.Weight = max(600, cell.Runs[r].Attributes.Weight)
										}
									}
									box.Children(func() {
										markdown.View(c, richtext.TextOf(richtext.Paragraph{Runs: cell.Runs}).Markdown(), a.mediaMarkdownOptions())
									})
								}
							})
						}
					})
					i = last
					continue
				}
			}
			if p.Style.SectionTitle {
				last := editorBlockEnd(state.parts, i)
				title := richtext.TextOf(richtext.Paragraph{Runs: p.Runs}).PlainText()
				if title == "" {
					title = a.tr("折叠内容", "Collapsible section")
				}
				bodyStart := i + 1
				ui.Collapsible(c.Key(i), title, state.open[i], func() { render(bodyStart, last) })
				i = last
				continue
			}
			last := i + 1
			for last < end && !state.parts[last].Style.SectionTitle && state.parts[last].Style.TableColumns == 0 {
				last++
			}
			markdown.View(c.Key(i), richtext.TextOf(state.parts[i:last]...).Markdown(), a.mediaMarkdownOptions())
			i = last
		}
	}
	root.Children(func() { render(0, len(state.parts)) })
}
