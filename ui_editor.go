package main

import (
	"net/url"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/richtext"
	"github.com/lobehub/lobe-mygo/pkg/action"
	"github.com/lobehub/lobe-mygo/pkg/field"
)

type blockEditorState struct {
	open                 bool
	query                string
	index                int
	search               ui.Handle
	list                 ui.ListState
	slashStart, slashEnd int
	slashVersion         uint64
	linkOpen             bool
	linkURL, linkError   string
}
type blockCommand struct{ key, zh, en string }

var blockCommands = []blockCommand{{"paragraph", "正文", "Paragraph"}, {"h1", "一级标题", "Heading 1"}, {"h2", "二级标题", "Heading 2"}, {"h3", "三级标题", "Heading 3"}, {"h4", "四级标题", "Heading 4"}, {"file", "文件或图片", "File or image"}, {"bullets", "项目符号列表", "Bulleted list"}, {"numbers", "编号列表", "Numbered list"}, {"tasks", "待办列表", "Checklist"}, {"quote", "引用", "Quote"}, {"code", "代码块", "Code block"}, {"divider", "分隔线", "Divider"}, {"collapsible", "折叠内容", "Collapsible section"}}

func init() { blockCommands = append(blockCommands, blockCommand{"table", "表格", "Table"}) }

func applyBlockStyle(p *richtext.ParagraphStyle, kind string) {
	p.Divider = false
	p.Heading, p.List, p.Checked, p.Quote, p.CodeBlock = 0, richtext.NoList, false, false, false
	switch kind {
	case "h1":
		p.Heading = 1
	case "h2":
		p.Heading = 2
	case "h3":
		p.Heading = 3
	case "h4":
		p.Heading = 4
	case "bullets":
		p.List = richtext.Bullets
	case "numbers":
		p.List = richtext.Numbers
	case "tasks":
		p.List = richtext.Tasks
	case "quote":
		p.Quote = true
	case "code":
		p.CodeBlock = true
	}
}

// Converting a section title unwraps its body in the same undo transaction.
func convertSectionTitle(doc *richtext.Document, kind string) bool {
	_, caret := doc.Selection()
	if !doc.Text().ParagraphStyleAt(caret).SectionTitle {
		return false
	}
	doc.Edit(func(e *richtext.Edit) {
		e.RemoveSection()
		e.FormatParagraphs(caret, caret, func(p *richtext.ParagraphStyle) { applyBlockStyle(p, kind) })
	})
	return true
}
func moveEditorBlock(doc *richtext.Document, action string) {
	text := doc.Text()
	_, caret := doc.Selection()
	start, end := text.ParagraphRange(caret)
	if table, ok := text.TableAt(caret); ok {
		start, end = table.Start, table.End
	}
	parts := []richtext.Paragraph{}
	idx := 0
	for offset, p := range text.Paragraphs() {
		if offset == start {
			idx = len(parts)
		}
		parts = append(parts, p)
	}
	target := idx
	last := editorBlockEnd(parts, idx)
	level := editorBlockLevel(parts[idx])
	switch action {
	case "up":
		previous := idx - 1
		if previous >= 0 && parts[previous].Style.TableColumns > 0 {
			previous -= parts[previous].Style.TableCell
		}
		for previous >= 0 && editorBlockLevel(parts[previous]) > level {
			previous--
		}
		if previous < 0 || editorBlockLevel(parts[previous]) != level || editorBlockEnd(parts, previous) != idx {
			return
		}
		group := slices.Clone(parts[idx:last])
		copy(parts[previous+len(group):last], parts[previous:idx])
		copy(parts[previous:], group)
		target = previous
	case "down":
		if last >= len(parts) || editorBlockLevel(parts[last]) != level {
			return
		}
		next := editorBlockEnd(parts, last)
		group := slices.Clone(parts[idx:last])
		copy(parts[idx:], parts[last:next])
		target = idx + next - last
		copy(parts[target:], group)
	case "duplicate":
		group := slices.Clone(parts[idx:last])
		for i := range group {
			group[i].Style.ID = ""
		}
		parts = slices.Insert(parts, last, group...)
		target = last
	case "delete":
		parts = slices.Delete(parts, idx, last)
		target = min(idx, len(parts)-1)
	}
	replacement := richtext.TextOf(parts...)
	position := 0
	for n := 0; n < target; n++ {
		position += richtext.TextOf(parts[n]).Len() + 1
	}
	if action != "delete" {
		position += min(caret-start, end-start)
	}
	doc.Edit(func(e *richtext.Edit) { e.Replace(0, e.Text().Len(), replacement); e.Select(position, position) })
}

func editorBlockLevel(p richtext.Paragraph) int {
	if p.Style.SectionTitle {
		return p.Style.SectionDepth - 1
	}
	return p.Style.SectionDepth
}

func editorBlockEnd(parts []richtext.Paragraph, i int) int {
	end := i + 1
	if cols := parts[i].Style.TableColumns; cols > 0 && parts[i].Style.TableCell == 0 {
		for end < len(parts) && parts[end].Style.TableColumns == cols && parts[end].Style.TableCell == end-i && parts[end].Style.SectionDepth == parts[i].Style.SectionDepth {
			end++
		}
		if (end-i)%cols == 0 {
			return end
		}
		return i + 1
	}
	if !parts[i].Style.SectionTitle {
		return end
	}
	depth := parts[i].Style.SectionDepth
	for end < len(parts) {
		s := parts[end].Style
		if s.SectionDepth < depth || s.SectionTitle && s.SectionDepth <= depth {
			break
		}
		end++
	}
	return end
}

// editorOptions shape the editor's chrome.
type editorOptions struct {
	// trailing builds controls at the end of the toolbar's row. With it the
	// toolbar sits under the text and shows only while the editor is in use,
	// which makes a comment box: one line until it is written in.
	trailing func()
}

func (a *app) richEditor(c *ui.Context, key, label, placeholder string, doc *richtext.Document, handle *ui.Handle, height float32, autofocus bool, changed func(), attach func(), media ...editorMediaTarget) ui.Element {
	return a.richEditorWith(c, editorOptions{}, key, label, placeholder, doc, handle, height, autofocus, changed, attach, media...)
}

func (a *app) richEditorWith(c *ui.Context, opts editorOptions, key, label, placeholder string, doc *richtext.Document, handle *ui.Handle, height float32, autofocus bool, changed func(), attach func(), media ...editorMediaTarget) ui.Element {
	var editor ui.Element
	frame := ui.Column(c.Key(key + "-editor-frame")).Gap(6)
	state := ui.Local(frame, "blocks", func() blockEditorState { return blockEditorState{slashStart: -1} })
	change := func() {
		if changed != nil {
			changed()
		}
	}
	openBlocks := func(start, end int) {
		state.open = true
		state.query = ""
		state.index = 0
		state.slashStart, state.slashEnd = start, end
		state.slashVersion = doc.Version()
		state.search.Focus()
	}
	link := func() { state.linkURL = doc.Formatting().Link; state.linkError = ""; state.linkOpen = true }
	compact := opts.trailing != nil
	// The toolbar holds what has no key of its own. Headings, lists and the
	// other blocks are in the block menu, which typing "/" also opens.
	tools := func() {
		_, currentCaret := doc.Selection()
		inTable := doc.Text().ParagraphStyleAt(currentCaret).TableColumns > 0
		tool := func(name, label, keys string) ui.Element {
			b := action.ActionIcon(c, glyph(name), action.ActionIconProps{BlockSize: 26, GlyphSize: 15}).Label(label)
			overlayTip(c, b, label, keys)
			return b
		}
		tool("bold", a.tr("粗体", "Bold"), "mod+b").KeepFocus().OnClick(func() { doc.ToggleBold(); change() })
		tool("italic", a.tr("斜体", "Italic"), "mod+i").KeepFocus().OnClick(func() { doc.ToggleItalic(); change() })
		tool("code", a.tr("代码", "Code"), "").KeepFocus().OnClick(func() { doc.ToggleCode(); change() })
		tool("link", a.tr("链接", "Link"), "mod+k").KeepFocus().OnClick(link)
		blockButton := tool("block", a.tr("块格式", "Block format"), "/").Disabled(inTable).KeepFocus().OnClick(func() { openBlocks(-1, -1) })
		if attach != nil {
			handle.OnShortcut(c, ui.Cmd|ui.Shift, ui.KeyU, attach)
			tool("paperclip", a.tr("附件", "Attach"), "mod+shift+u").OnClick(attach)
		}
		if inTable {
			textButton(c.Key("table-actions"), a.tr("表格操作", "Table actions")).KeepFocus().PopoverMenuButton(func(m *ui.Menu) {
				for _, action := range [][3]string{{"row-before", "在上方插入行", "Insert row above"}, {"row-after", "在下方插入行", "Insert row below"}, {"column-before", "在左侧插入列", "Insert column left"}, {"column-after", "在右侧插入列", "Insert column right"}, {"header-row", "切换标题行", "Toggle header row"}, {"header-column", "切换标题列", "Toggle header column"}, {"delete-row", "删除当前行", "Delete current row"}, {"delete-column", "删除当前列", "Delete current column"}, {"delete", "删除表格", "Delete table"}} {
					if m.Item(a.tr(action[1], action[2])).Chosen() {
						doc.EditTable(action[0])
						change()
						handle.Focus()
					}
				}
			})
			textButton(c.Key("table-exit"), a.tr("表格后继续", "Continue after table")).KeepFocus().OnClick(func() { doc.ExitTable(); handle.Focus(); change() })
		}
		_, sectionCaret := doc.Selection()
		if _, _, ok := doc.Text().SectionRange(sectionCaret); ok {
			textButton(c, a.tr("折叠/展开", "Toggle section")).Label(a.tr("折叠或展开内容", "Toggle collapsible section")).KeepFocus().OnClick(func() { doc.ToggleSection(); change() })
		}
		if !compact {
			action.ActionIcon(c, glyph("more"), action.ActionIconProps{BlockSize: 26, GlyphSize: 15, Title: a.tr("段落操作", "Paragraph actions")}).KeepFocus().PopoverMenuButton(func(m *ui.Menu) {
				for _, action := range [][3]string{{"up", "上移段落", "Move paragraph up"}, {"down", "下移段落", "Move paragraph down"}, {"duplicate", "复制段落", "Duplicate paragraph"}, {"delete", "删除段落", "Delete paragraph"}} {
					if m.Item(a.tr(action[1], action[2])).Chosen() {
						moveEditorBlock(doc, action[0])
						change()
						handle.Focus()
					}
				}
			})
		}
		ui.PopoverBase(c, blockButton, &state.open, func(panel ui.Element) {
			a.popoverChrome(panel)
			panel.Width(240).Gap(2)
			results := []blockCommand{}
			for _, cmd := range blockCommands {
				if cmd.key == "file" && attach == nil {
					continue
				}
				terms := cmd.zh + " " + cmd.en
				if cmd.key == "file" {
					terms += " insert upload"
				}
				if strings.Contains(strings.ToLower(terms), strings.ToLower(state.query)) {
					results = append(results, cmd)
				}
			}
			state.index = max(0, min(state.index, len(results)-1))
			choose := func(kind string) {
				removeSlash := state.slashStart >= 0 && state.slashVersion == doc.Version()
				doc.Edit(func(e *richtext.Edit) {
					if removeSlash {
						e.ReplaceString(state.slashStart, state.slashEnd, "", richtext.Attributes{})
						e.Select(state.slashStart, state.slashStart)
					}
					if kind == "file" {
						return
					}
					if kind == "collapsible" {
						e.MakeCollapsible()
						return
					}
					if kind == "table" {
						e.InsertTable(3, 3)
						return
					}
					_, caret := e.Selection()
					if e.Text().ParagraphStyleAt(caret).SectionTitle {
						e.RemoveSection()
					}
					if kind == "divider" {
						e.InsertDivider()
						return
					}
					from, to := e.Selection()
					e.FormatParagraphs(from, to, func(p *richtext.ParagraphStyle) { applyBlockStyle(p, kind) })
				})
				state.open = false
				state.slashStart = -1
				if kind == "file" && attach != nil {
					attach()
				}
				change()
				handle.Focus()
			}
			ui.TextInput(c.Key("block-search"), &state.query).Bind(&state.search).AutoFocus().Label(a.tr("搜索块类型", "Search block types")).Placeholder(a.tr("搜索块类型…", "Search block types…")).PlaceholderColor(a.tk.TextPlaceholder).Height(32).Margin(0, 8).Padding(0).FontSize(13).Background(ui.Transparent).Border(0, ui.Transparent).FocusRing(false).OnChange(func() { state.index = 0 }).OnTextKey(func(k ui.TextKey) bool {
				if k.Mods != 0 {
					return false
				}
				switch k.Key {
				case ui.KeyDown:
					if len(results) > 0 {
						state.index = (state.index + 1) % len(results)
					}
				case ui.KeyUp:
					if len(results) > 0 {
						state.index = (state.index + len(results) - 1) % len(results)
					}
				case ui.KeyEnter:
					if len(results) > 0 {
						choose(results[state.index].key)
					}
				default:
					return false
				}
				state.list.ScrollIntoView(state.index)
				return true
			})
			state.list.Key = func(n int) any { return results[n].key }
			ui.Box(c).Height(1).Margin(0, -4, 2, -4).Background(a.tk.BorderSecondary)
			ui.List(c, &state.list, len(results), func(n int) {
				cmd := results[n]
				button := ui.ButtonBase(c.Key(cmd.key)).Height(30).Padding(0, 8).Radius(a.tk.BorderRadiusSM).FillWidth().Justify(ui.Start).FontSize(13).Cursor(ui.CursorPointer)
				if n == state.index || button.Hovered() {
					button.Background(a.tk.FillTertiary)
				}
				button.Children(func() { ui.Text(c, a.tr(cmd.zh, cmd.en)).SingleLine() }).OnClick(func() { choose(cmd.key) })
			}).Height(min(float32(len(results)*30), 240))
			if len(results) == 0 {
				ui.Text(c, a.tr("没有匹配项", "No matches")).Padding(10, 9).TextColor(a.theme.TextMuted)
			}
		})
	}
	// A compact editor shows its tools once it is being written in. They stay
	// while a menu of theirs is open, which takes the focus from the text.
	inUse := handle.Focused(c) || state.open || state.linkOpen || doc.Text().Len() > 0
	frame.Children(func() {
		if !compact {
			ui.Row(c).Wrap().Gap(2).Margin(0, 0, 0, -5).Children(tools)
		}
		var onPaste func() bool
		if len(media) > 0 {
			target, services := media[0], c.Services()
			onPaste = func() bool { return a.pasteEditorMedia(target, doc, services) }
			a.editorMediaDrop(frame, target, doc)
		}
		editor = richtext.EditorBase(c.Key(key), doc, richtext.EditorOptions{Label: label, Placeholder: placeholder, OnLinkCommand: link, OnPaste: onPaste, OnLink: func(raw string) {
			if strings.HasPrefix(raw, attachmentURLPrefix) {
				if v, ok := a.data.Attachments[strings.TrimPrefix(raw, attachmentURLPrefix)]; ok {
					a.exportAttachment(v)
				}
			} else if strings.HasPrefix(raw, "flow://") {
				if e := a.openLink(raw); e != nil {
					a.err = e.Error()
				}
			} else {
				a.services.OpenURL(raw)
			}
		}}).Height(height).Bind(handle).FontSize(15).OnChange(func() {
			change()
			anchor, caret := doc.Selection()
			text := doc.Text()
			start, _ := text.ParagraphRange(caret)
			if !doc.CanRedo() && anchor == caret && caret == start+1 && text.Slice(start, caret).PlainText() == "/" && !text.ParagraphStyleAt(caret).CodeBlock && text.ParagraphStyleAt(caret).TableColumns == 0 {
				openBlocks(start, caret)
			}
		})
		if autofocus {
			editor.AutoFocus()
		}
		if compact {
			// The row keeps its place while idle: were it to appear with the
			// focus, a press below the editor would land on what moved there.
			ui.Row(c).Height(28).Gap(2).Children(func() {
				bar := ui.Row(c).Gap(2).Margin(0, 0, 0, -5)
				if !inUse {
					bar.Opacity(0).IgnoreInput(true)
				}
				bar.Children(tools)
				ui.Spacer(c)
				opts.trailing()
			})
		}
	})
	ui.DialogBase(c, &state.linkOpen, func(backdrop, panel ui.Element) {
		a.dialogChrome(backdrop, panel)
		panel.Width(440).Padding(16).Gap(12)
		a.dialogTitle(c, a.tr("链接", "Link"), nil)
		field.Input(c.Key("editor-link"), &state.linkURL, field.InputProps{Placeholder: "https://…", Prefix: glyph("link"), Invalid: state.linkError != ""}).AutoFocus().Label(a.tr("文字链接地址", "Text link URL"))
		if state.linkError != "" {
			ui.Text(c, state.linkError).FontSize(12).TextColor(a.tk.Error)
		}
		dialogFooter(c, func() {
			btn(c, a.tr("取消", "Cancel")).OnClick(func() { state.linkOpen = false; handle.Focus() })
			primary(c, a.tr("保存", "Save link")).Label(a.tr("保存链接", "Save link")).OnClick(func() {
				raw := strings.TrimSpace(state.linkURL)
				u, e := url.Parse(raw)
				if raw != "" && (e != nil || !slices.Contains([]string{"http", "https", "mailto", "flow"}, u.Scheme) || ((u.Scheme == "http" || u.Scheme == "https") && u.Host == "")) {
					state.linkError = a.tr("请输入完整的链接地址，或清空以移除链接。", "Enter a complete URL, or leave it empty to remove the link.")
					return
				}
				doc.SetLink(raw)
				change()
				state.linkOpen = false
				handle.Focus()
			})
		})
	})
	return editor
}
