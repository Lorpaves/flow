package main

import (
	"flow/internal/core"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
	"github.com/egoist/mygo/ui/richtext"
	"github.com/lobehub/lobe-mygo/pkg/action"
	"github.com/lobehub/lobe-mygo/pkg/lucide"
	"github.com/lobehub/lobe-mygo/pkg/theme"
)

type commentForm struct {
	ID, ParentID, Body string
	Editor             ui.Handle
	Doc                *richtext.Document
	wantFocus          bool
	composing          bool
}

func (a *app) commentFormFor(issue, target string) *commentForm {
	if a.commentForms == nil {
		a.commentForms = map[string]*commentForm{}
	}
	key := issue
	if target != "" {
		key += "/" + target
	}
	f := a.commentForms[key]
	if f == nil {
		f = &commentForm{Doc: richtext.NewDocument(richtext.Text{})}
		a.commentForms[key] = f
	}
	return f
}

func (a *app) selectCommentComposer(issue, target string) *commentForm {
	if a.commentTargets == nil {
		a.commentTargets = map[string]string{}
	}
	a.commentTargets[issue] = target
	f := a.commentFormFor(issue, target)
	f.wantFocus = true
	return f
}

func (a *app) replyToComment(v core.Comment) {
	f := a.selectCommentComposer(v.IssueID, "reply/"+v.ID)
	f.ParentID = v.ID
}

func (a *app) editComment(v core.Comment) {
	f := a.selectCommentComposer(v.IssueID, "edit/"+v.ID)
	if f.ID == "" {
		f.ID, f.ParentID, f.Body = v.ID, v.ParentID, v.Body
		f.Doc = richtext.NewDocument(a.hydrateMediaText(richtext.ParseMarkdown(v.Body)))
	}
}

func (f *commentForm) syncBody() { f.Body = f.Doc.Text().Markdown() }

// commentEditorHeight keeps an empty comment to one line and lets it grow with
// what is written.
func commentEditorHeight(doc *richtext.Document) float32 {
	height := float32(8)
	for _, p := range doc.Text().Paragraphs() {
		line := float32(24)
		for _, r := range p.Runs {
			if at := r.Attributes.Attachment; at != nil {
				line = max(line, at.Height+12)
			}
		}
		height += line
	}
	return min(float32(320), max(float32(32), height))
}

func (a *app) submitComment(issue, target string, f *commentForm) {
	f.syncBody()
	if a.pending > 0 || strings.TrimSpace(f.Body) == "" {
		return
	}
	if f.ID == "" && f.ParentID != "" {
		parent, ok := a.data.Comments[f.ParentID]
		if !ok || parent.DeletedAt != 0 {
			a.err = "invalid_comment_parent"
			return
		}
	}
	body, edit, parent := f.Body, f.ID, f.ParentID
	doc, version := f.Doc, f.Doc.Version()
	a.apply(func(d *core.Data) error {
		_, e := d.SaveComment(core.Comment{ID: edit, IssueID: issue, ParentID: parent, Body: body})
		return e
	}, func() {
		// Typing or changing thread during the write must not lose that draft.
		if f.ID == edit && f.Doc == doc && doc.Version() == version && f.ParentID == parent {
			f.ID, f.Body = "", ""
			f.Doc = richtext.NewDocument(richtext.Text{})
			if a.commentTargets[issue] == target {
				a.selectCommentComposer(issue, "")
			}
		}
	})
}

func (a *app) commentsSection(c *ui.Context, issue string) {
	for _, entry := range a.data.CommentThread(issue) {
		v := entry.Comment
		row := ui.Column(c.Key("comment-"+v.ID)).Margin(0, 0, 0, float32(min(entry.Depth, 4))*20).Padding(8, 8, 12, 14).Gap(2).Radius(14).Background(a.tk.FillQuaternary).Border(1, a.tk.BorderSecondary)
		focus := ui.Local(row, "focus", func() ui.Handle { return ui.Handle{} })
		row.Bind(focus).Focusable().Label(a.tr("评论：", "Comment: ") + shortText(v.Body))
		if v.DeletedAt != 0 {
			row.Label(a.tr("已删除的评论", "Deleted comment")).Padding(8, 12).Children(func() {
				ui.Text(c, a.tr("已删除", "Deleted")).FontSize(12).TextColor(a.tk.TextQuaternary)
			})
			continue
		}
		focus.OnShortcut(c, 0, ui.KeyR, func() { a.replyToComment(v) })
		hovered := row.Hovered()
		row.Children(func() {
			ui.Row(c).Height(26).Gap(6).Children(func() {
				when := a.shortTime(v.CreatedAt) + " " + time.UnixMilli(v.CreatedAt).Format("15:04")
				if v.UpdatedAt > v.CreatedAt {
					when += " · " + a.tr("已编辑", "Edited")
				}
				ui.Text(c, when).FontSize(12).TextColor(a.tk.TextTertiary)
				ui.Spacer(c)
				reply := reveal(iconButton(c, "comment", a.tr("回复评论：", "Reply to comment: ")+shortText(v.Body)).Size(24, 24), hovered)
				reply.OnClick(func() { a.replyToComment(v) })
				reveal(iconButton(c, "pencil", a.tr("编辑评论：", "Edit comment: ")+shortText(v.Body)).Size(24, 24), hovered).OnClick(func() { a.editComment(v) })
				reveal(iconButton(c, "trash", a.tr("删除评论：", "Delete comment: ")+shortText(v.Body)).Size(24, 24), hovered).Disabled(a.pending > 0).OnClick(func() {
					a.apply(func(d *core.Data) error {
						current, ok := d.Comments[v.ID]
						if !ok {
							return nil
						}
						current.DeletedAt = time.Now().UnixMilli()
						d.Comments[v.ID] = current
						return nil
					}, nil)
				})
			})
			markdown.View(c, v.Body, a.mediaMarkdownOptions()).FontSize(14)
		})
	}
	target := a.commentTargets[issue]
	f := a.commentFormFor(issue, target)
	if !f.Editor.Focused(c) {
		f.composing = false
	}
	doc := f.Doc
	media := editorMediaTarget{kind: "issue", id: issue, local: true, changed: f.syncBody}
	// The composer is one box: what it answers, the text, then its tools and
	// the button that sends. Empty and unfocused it is a single line.
	box := ui.Column(c.Key("comment-composer")).Padding(8, 8, 8, 14).Gap(4).Radius(16).Border(1, a.tk.FillBorder()).Background(a.tk.BgContainer)
	if f.Editor.Focused(c) {
		box.Border(1, a.tk.FillBorderStrong())
	}
	box.Children(func() {
		if target != "" {
			ui.Row(c).Height(26).Gap(8).Children(func() {
				label := a.tr("编辑评论", "Editing comment")
				if f.ID == "" {
					parent := a.data.Comments[f.ParentID]
					label = a.tr("回复：", "Replying to: ") + shortText(parent.Body)
					if parent.DeletedAt != 0 {
						label = a.tr("原评论已删除", "Original comment deleted")
					}
				}
				ui.Text(c, label).Grow(1).MinWidth(0).SingleLine().FontSize(12).TextColor(a.tk.TextTertiary)
				cancel := a.tr("取消回复", "Cancel reply")
				if f.ID != "" {
					cancel = a.tr("取消编辑", "Cancel edit")
				}
				iconButton(c, "close", cancel).Size(24, 24).OnClick(func() { a.selectCommentComposer(issue, "") })
			})
		}
		label := a.tr("评论", "Add comment")
		if f.ParentID != "" {
			label = a.tr("回复", "Send reply")
		}
		if f.ID != "" {
			label = a.tr("保存", "Save comment")
		}
		parent, parentExists := a.data.Comments[f.ParentID]
		unavailable := f.ID == "" && f.ParentID != "" && (!parentExists || parent.DeletedAt != 0)
		send := func() {
			b := accentButton(c, action.Button(c, "", action.ButtonProps{Variant: action.Primary, Size: theme.Small})).Label(label).Size(28, 28).Padding(0).Center().
				Disabled(strings.TrimSpace(f.Body) == "" || a.pending > 0 || unavailable).OnClick(func() { a.submitComment(issue, target, f) })
			b.Children(func() { ui.Icon(c, lucide.ArrowUp()).FontSize(14) })
			overlayTip(c, b, label, "enter")
		}
		editor := a.richEditorWith(c, editorOptions{trailing: send}, "comment-compose-"+issue+"/"+target, a.tr("评论内容", "Comment body"), a.tr("写评论…", "Leave a comment…"), doc, &f.Editor, commentEditorHeight(doc), false, f.syncBody, func() { a.chooseEditorMedia(media, doc) }, media)
		editor.ObserveInput(func(ev ui.InputEvent) {
			switch ev.Kind {
			case ui.InputCompose:
				f.composing = ev.Text != ""
			case ui.InputText:
				f.composing = false
			}
		})
		for _, mods := range []ui.Modifiers{0, ui.Cmd} {
			f.Editor.OnShortcut(c, mods, ui.KeyEnter, func() {
				if !f.composing {
					a.submitComment(issue, target, f)
				}
			})
		}
		if f.wantFocus {
			f.Editor.Focus(c)
			editor.ScrollIntoView(ui.ScrollIntoViewOptions{Block: ui.ScrollCenter})
			f.wantFocus = false
		}
	})
}
