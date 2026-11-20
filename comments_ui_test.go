package main

import (
	"flow/internal/core"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestCommentEnterSendAndComposition(t *testing.T) {
	a := testApp()
	issue := a.filtered()[0].ID
	tt := ui.NewTester(func(c *ui.Context) { c.SetTheme(a.theme); a.commentsSection(c, issue) }, 1000, 680)
	click(t, tt, "Comment body")
	tt.Type("输入法确认前")
	tt.Compose("中文", 2)
	tt.Key(0, ui.KeyEnter)
	if len(a.data.Comments) != 0 {
		t.Fatal("composition Enter sent comment")
	}
	tt.Type("中文")
	tt.Compose("", 0)
	tt.Key(ui.Shift, ui.KeyEnter)
	tt.Type("第二行")
	tt.Key(0, ui.KeyEnter)
	if len(a.data.Comments) != 1 {
		t.Fatal("Enter did not send comment", a.err)
	}
	for _, v := range a.data.Comments {
		if v.Body != "输入法确认前中文<br>第二行" {
			t.Fatal("comment content changed", v.Body)
		}
	}
}

func TestReplyComposerScrollsIntoView(t *testing.T) {
	a := testApp()
	issue := a.filtered()[0].ID
	root, _ := a.data.SaveComment(core.Comment{IssueID: issue, Body: "Long issue discussion"})
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(a.theme)
		ui.Scroll(c).Fill().Children(func() {
			ui.Column(c).Gap(12).Padding(24).Children(func() { ui.Box(c).Height(1200); a.commentsSection(c, issue) })
		})
	}, 1000, 680)
	a.replyToComment(root)
	tt.Frame()
	tt.Frame()
	r, ok := tt.Find("Comment body")
	if !ok || r.Y < 0 || r.Y+r.H > 680 {
		t.Fatalf("reply editor is outside viewport: %+v", r)
	}
	tt.Type("Visible and focused")
	if a.commentFormFor(issue, "reply/"+root.ID).Body != "Visible and focused" {
		t.Fatal("visible composer did not receive typing")
	}
}

func TestCommentReplyKeyboardDraftsAndDeletedAncestor(t *testing.T) {
	a := testApp()
	issue := a.filtered()[0].ID
	root, e := a.data.SaveComment(core.Comment{IssueID: issue, Body: "Discussion root"})
	if e != nil {
		t.Fatal(e)
	}
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(a.theme)
		ui.Scroll(c).Fill().Children(func() { ui.Column(c).Padding(24).Gap(12).Children(func() { a.commentsSection(c, issue) }) })
	}, 1000, 760)
	click(t, tt, "Comment body")
	tt.Type("Unsent main comment")
	click(t, tt, "Comment: Discussion root")
	tt.Key(0, ui.KeyR)
	tt.Type("中文回复")
	tt.Key(ui.Shift, ui.KeyEnter)
	tt.Type("第二行")
	if len(a.data.Comments) != 1 {
		t.Fatal("Shift Enter sent a comment instead of inserting newline")
	}
	f := a.commentFormFor(issue, "reply/"+root.ID)
	if !strings.Contains(f.Body, "<br>") {
		t.Fatal("reply lost newline or focus", f.Body)
	}
	click(t, tt, "Cancel reply")
	if a.commentFormFor(issue, "").Body != "Unsent main comment" {
		t.Fatal("reply replaced main draft")
	}
	click(t, tt, "Reply to comment: Discussion root")
	if f.Body != "中文回复<br>第二行" {
		t.Fatal("cancel discarded reply draft")
	}
	tt.Key(ui.Cmd, ui.KeyEnter)
	if len(a.data.Comments) != 2 || a.commentTargets[issue] != "" || f.Body != "" {
		t.Fatalf("reply failed: %s", a.err)
	}
	var reply core.Comment
	for _, v := range a.data.Comments {
		if v.ParentID == root.ID {
			reply = v
		}
	}
	click(t, tt, "Edit comment: "+shortText(reply.Body))
	tt.Command("selectAll")
	tt.Type("编辑后的回复")
	tt.Key(ui.Cmd, ui.KeyEnter)
	if a.data.Comments[reply.ID].ParentID != root.ID || a.data.Comments[reply.ID].Body != "编辑后的回复" {
		t.Fatal("editing detached reply")
	}
	click(t, tt, "Reply to comment: Discussion root")
	tt.Type("删除后保留的草稿")
	parent := a.data.Comments[root.ID]
	parent.DeletedAt = time.Now().UnixMilli()
	a.data.Comments[root.ID] = parent
	tt.Frame()
	tt.Key(ui.Cmd, ui.KeyEnter)
	if a.err != "invalid_comment_parent" || f.Body != "删除后保留的草稿" || len(a.data.Comments) != 2 {
		t.Fatal("deleted parent lost draft or was resurrected", a.err, f.Body)
	}
	capture(t, tt, "24-comment-thread")
	click(t, tt, "Cancel reply")
	tt.Frame()
	tt.Frame()
	if _, ok := tt.Find("Deleted comment"); !ok {
		t.Fatal("deleted ancestor was not represented")
	}
}
