package main

import (
	"bytes"
	"encoding/base64"
	"flow/internal/core"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/richtext"
)

func discussionHTML(t *testing.T) transfer.Data {
	return mediaHTMLClipboard(`<p><strong>评论与文档图片</strong></p><p><img src="data:image/png;base64,` + base64.StdEncoding.EncodeToString(testMediaPNG(t)) + `"></p>`)
}

func TestRichCommentMediaSendEditAndReopen(t *testing.T) {
	a := testApp()
	issue := a.filtered()[0].ID
	a.openIssue(issue)
	original := a.drafts[issue].doc.Text().Markdown()
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(a.theme)
		ui.Scroll(c).Fill().Children(func() { ui.Column(c).Padding(24).Gap(12).Children(func() { a.commentsSection(c, issue) }) })
	}, 1000, 800)
	click(t, tt, "Comment body")
	tt.SetClipboardData(discussionHTML(t))
	tt.Command("paste")
	f := a.commentFormFor(issue, "")
	if !strings.Contains(f.Body, "**评论与文档图片**") || !strings.Contains(f.Body, attachmentURLPrefix) || a.drafts[issue].doc.Text().Markdown() != original {
		t.Fatal("comment media went to wrong editor", f.Body, a.err)
	}
	tt.Command("undo")
	if f.Doc.Text().Len() != 0 {
		t.Fatal("media paste was not one undo step")
	}
	tt.Command("redo")
	tt.Key(0, ui.KeyEnter)
	if len(a.data.Comments) != 1 || f.Body != "" {
		t.Fatal("rich comment did not send", a.err)
	}
	path := filepath.Join(t.TempDir(), "flow.db")
	store, err := core.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.Update(func(d *core.Data) error { *d = a.data.Clone(); return nil }); err != nil {
		t.Fatal(err)
	}
	store.Close()
	store, err = core.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	b := newApp(store.Snapshot())
	var comment core.Comment
	for _, v := range b.data.Comments {
		comment = v
	}
	b.editComment(comment)
	edited := b.commentFormFor(issue, "edit/"+comment.ID)
	assertHydratedDiscussionMedia(t, b, edited.Doc, "issue", issue)
	if !strings.Contains(edited.Doc.Text().Markdown(), "**评论与文档图片**") {
		t.Fatal("format lost after reopen")
	}
	preview := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(b.theme)
		ui.Scroll(c).Fill().Children(func() { ui.Column(c).Padding(24).Gap(12).Children(func() { b.commentsSection(c, issue) }) })
	}, 1000, 800)
	capture(t, preview, "33-comment-media")
}

func assertHydratedDiscussionMedia(t *testing.T, a *app, doc *richtext.Document, kind, id string) {
	t.Helper()
	count := 0
	for _, p := range doc.Text().Paragraphs() {
		for _, r := range p.Runs {
			at := r.Attributes.Attachment
			if at == nil {
				continue
			}
			count++
			file := a.data.Attachments[strings.TrimPrefix(at.URL, attachmentURLPrefix)]
			if at.Draw == nil || at.Width <= 0 || file.OwnerKind != kind || file.OwnerID != id || !bytes.Equal(file.Data, testMediaPNG(t)) {
				t.Fatal("media hydration/ownership/bytes lost", at, file.ID)
			}
		}
	}
	if count != 1 {
		t.Fatal("expected one inline image", count, doc.Text().Markdown())
	}
}

func TestDocumentAndUpdateMediaStayInOwnEditor(t *testing.T) {
	for _, kind := range []string{"issue", "project", "update"} {
		t.Run(kind, func(t *testing.T) {
			a := testApp()
			issue := a.filtered()[0].ID
			a.openIssue(issue)
			original := a.drafts[issue].doc.Text().Markdown()
			owner := a.data.ProjectList(false, false)[0].ID
			label := "Document body"
			if kind == "update" {
				a.editProjectUpdate(owner, "")
				label = "Update body"
			} else {
				if kind == "issue" {
					owner = issue
				}
				a.editDocument(kind, owner, "")
				a.documentForm.Name = "媒体文档"
			}
			tt := ui.NewTester(a.view, 1120, 880)
			click(t, tt, label)
			tt.SetClipboardData(discussionHTML(t))
			tt.Command("paste")
			if a.drafts[issue].doc.Text().Markdown() != original {
				t.Fatal("resource paste modified issue description")
			}
			if kind == "update" {
				click(t, tt, "Save update")
				if a.updateOpen || len(a.data.Updates) != 1 {
					t.Fatal("update save failed", a.err)
				}
				for _, v := range a.data.Updates {
					a.editProjectUpdate(owner, v.ID)
				}
				assertHydratedDiscussionMedia(t, a, a.updateText, "project", owner)
			} else {
				click(t, tt, "Save document")
				if a.documentOpen || len(a.data.Documents) != 1 {
					t.Fatal("document save failed", a.err)
				}
				for _, v := range a.data.Documents {
					a.editDocument(kind, owner, v.ID)
				}
				assertHydratedDiscussionMedia(t, a, a.documentText, kind, owner)
			}
		})
	}
}

func discussionAsyncStore(t *testing.T, a *app) *core.Store {
	t.Helper()
	s, e := core.Open(filepath.Join(t.TempDir(), "flow.db"))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { s.Close() })
	if _, e = s.Update(func(d *core.Data) error { *d = a.data.Clone(); return nil }); e != nil {
		t.Fatal(e)
	}
	a.window = &mygo.Window{}
	return s
}
func finishDiscussionJob(t *testing.T, a *app, s *core.Store) {
	t.Helper()
	select {
	case work := <-a.jobs:
		data, e := work.run(s)
		work.done(data, e)
		if e != nil {
			t.Fatal(e)
		}
	default:
		t.Fatal("no pending write")
	}
}

func TestDiscussionPendingWritesPreserveLaterEditing(t *testing.T) {
	a := testApp()
	issue := a.filtered()[0].ID
	project := a.data.ProjectList(false, false)[0].ID
	s := discussionAsyncStore(t, a)
	f := a.commentFormFor(issue, "")
	f.Doc.InsertText("sent text")
	a.submitComment(issue, "", f)
	f.Doc.InsertText(" later typing")
	finishDiscussionJob(t, a, s)
	if f.Doc.Text().PlainText() != "sent text later typing" || len(a.data.Comments) != 1 {
		t.Fatal("comment typing discarded")
	}
	a.editDocument("project", project, "")
	a.documentForm.Name = "Initial"
	a.documentText.InsertText("saved")
	a.saveDocument()
	a.documentForm.Name = "Renamed while saving"
	a.documentText.InsertText(" later")
	finishDiscussionJob(t, a, s)
	if !a.documentOpen || a.documentForm.ID == "" || a.documentText.Text().PlainText() != "saved later" || a.documentForm.Name != "Renamed while saving" {
		t.Fatal("continued document editing lost")
	}
	a.saveDocument()
	finishDiscussionJob(t, a, s)
	if a.documentOpen || len(a.data.Documents) != 1 {
		t.Fatal("continued edit duplicated document")
	}
	for _, v := range a.data.Documents {
		if v.Body != "saved later" || v.Name != "Renamed while saving" {
			t.Fatal("later edits not persisted", v)
		}
	}
	a.editDocument("project", project, "")
	a.documentForm.Name = "First"
	a.saveDocument()
	a.editDocument("project", project, "")
	a.documentForm.Name = "Other"
	second := a.documentText
	finishDiscussionJob(t, a, s)
	if !a.documentOpen || a.documentText != second || a.documentForm.Name != "Other" {
		t.Fatal("save closed another document")
	}
	a.editProjectUpdate(project, "")
	a.updateText.InsertText("progress")
	a.saveProjectUpdate()
	a.updateText.InsertText(" later")
	a.updateForm.Health = "at_risk"
	finishDiscussionJob(t, a, s)
	if !a.updateOpen || a.updateForm.ID == "" || a.updateText.Text().PlainText() != "progress later" || a.updateForm.Health != "at_risk" {
		t.Fatal("continued update lost")
	}
	a.saveProjectUpdate()
	finishDiscussionJob(t, a, s)
	if a.updateOpen || len(a.data.Updates) != 1 || a.data.Projects[project].Health != "at_risk" {
		t.Fatal("continued update duplicated or health lost")
	}
	a.editProjectUpdate(project, "")
	a.updateText.InsertText("first")
	a.saveProjectUpdate()
	a.editProjectUpdate(project, "")
	a.updateText.InsertText("other")
	other := a.updateText
	finishDiscussionJob(t, a, s)
	if !a.updateOpen || a.updateText != other || a.updateText.Text().PlainText() != "other" {
		t.Fatal("save closed another update")
	}
}

func TestDelayedCommentMediaPreservesThreadAndLaterText(t *testing.T) {
	a := testApp()
	issue := a.filtered()[0].ID
	a.openIssue(issue)
	root, _ := a.data.SaveComment(core.Comment{IssueID: issue, Body: "Root"})
	file, _ := a.data.AddAttachment(core.Attachment{OwnerKind: "issue", OwnerID: issue, Name: "reference.txt", Data: []byte("reference bytes")})
	s := discussionAsyncStore(t, a)
	f := a.commentFormFor(issue, "")
	f.Doc.InsertText("before")
	f.Doc.SetSelection(0, f.Doc.Text().Len())
	a.importRichPaste(editorMediaTarget{kind: "issue", id: issue, local: true, changed: f.syncBody}, f.Doc, transfer.HTML, []byte(`<strong>pasted</strong><a href="`+attachmentURLPrefix+file.ID+`">reference</a>`))
	f.Doc.SetSelection(f.Doc.Text().Len(), f.Doc.Text().Len())
	f.Doc.InsertText(" later")
	a.replyToComment(root)
	reply := a.commentFormFor(issue, "reply/"+root.ID)
	reply.Doc.InsertText("other thread")
	finishDiscussionJob(t, a, s)
	if !strings.Contains(f.Body, "before later") || !strings.Contains(f.Body, "**pasted**") || reply.Doc.Text().PlainText() != "other thread" || strings.Contains(a.drafts[issue].doc.Text().PlainText(), "pasted") {
		t.Fatal("delayed media replaced typing or another editor", f.Body)
	}
}
