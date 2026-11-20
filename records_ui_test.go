package main

import (
	"bytes"
	"flow/internal/core"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/richtext"
)

func TestRelationshipCommentHistoryAndRecoveryUI(t *testing.T) {
	a := testApp()
	items := a.filtered()
	id, target := items[0].ID, items[1].ID
	a.openIssue(id)
	a.chooseRelationship(id, "blocks")
	a.relationTarget = target
	tt := ui.NewTester(a.view, 1000, 680)
	click(t, tt, "Save relationship")
	if len(a.data.Relations) != 1 || a.relationOpen {
		t.Fatalf("relationship failed: %s", a.err)
	}
	a.chooseRelationship(target, "blocks")
	a.relationTarget = id
	tt.Frame()
	click(t, tt, "Save relationship")
	if !a.relationOpen || a.err == "" || len(a.data.Relations) != 1 {
		t.Fatal("blocking cycle accepted")
	}
	a.relationOpen = false
	a.err = ""
	activity := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(a.theme)
		ui.Column(c).Fill().Padding(20).Children(func() { a.activitySection(c, "issue", id) })
		a.historyDialog(c)
	}, 1000, 900)
	click(t, activity, "Comment body")
	activity.SetClipboardData(transfer.New(transfer.NewItem(transfer.Bytes(transfer.Markdown, []byte("**评论** 支持 Markdown")))))
	activity.Command("paste")
	click(t, activity, "Add comment")
	var commentID string
	for key := range a.data.Comments {
		commentID = key
	}
	if commentID == "" {
		t.Fatalf("comment missing: %s", a.err)
	}
	click(t, activity, "Edit comment: **评论** 支持 Markdown")
	click(t, activity, "Comment body")
	activity.Command("selectAll")
	activity.Type("修改后的评论")
	click(t, activity, "Save comment")
	if richtext.ParseMarkdown(a.data.Comments[commentID].Body).PlainText() != "修改后的评论" {
		t.Fatal("comment edit failed", a.data.Comments[commentID].Body)
	}
	old := a.data.Issues[id].Title
	a.apply(func(d *core.Data) error { i := d.Issues[id]; i.Title = "Changed title"; return d.PutIssue(i) }, nil)
	var historyID string
	for _, v := range a.data.History("issue", id) {
		if v.Kind == "issue" && a.data.CanUndoActivity(v) {
			historyID = v.ID
			break
		}
	}
	activity.Frame()
	click(t, activity, a.activityControlLabel("details", a.data.Activities[historyID]))
	activity.Frame()
	click(t, activity, "Close history")
	click(t, activity, a.activityControlLabel("undo", a.data.Activities[historyID]))
	if a.data.Issues[id].Title != old || a.drafts[id].issue.Title != old {
		t.Fatalf("undo not reflected: %s", a.err)
	}
	capture(t, activity, "14-activity-comments")
	click(t, activity, "Delete comment: "+shortText(a.data.Comments[commentID].Body))
	if a.data.Comments[commentID].DeletedAt == 0 {
		t.Fatal("comment deletion failed")
	}
}
func TestAttachmentImportExportAndLinkUI(t *testing.T) {
	a := testApp()
	id := a.filtered()[0].ID
	path := filepath.Join(t.TempDir(), "中文.txt")
	content := []byte("中文附件\x00\x01")
	if e := os.WriteFile(path, content, 0600); e != nil {
		t.Fatal(e)
	}
	a.importAttachments("issue", id, []string{path})
	if len(a.data.Attachments) != 1 {
		t.Fatalf("file import: %s", a.err)
	}
	var file core.Attachment
	for _, v := range a.data.Attachments {
		file = v
	}
	if file.Name != "中文.txt" || !bytes.Equal(file.Data, content) {
		t.Fatal("file bytes changed")
	}
	output := filepath.Join(t.TempDir(), "export.txt")
	if e := writeAttachmentExport(output, file); e != nil {
		t.Fatal(e)
	}
	got, e := os.ReadFile(output)
	if e != nil || !bytes.Equal(got, content) {
		t.Fatal("export mismatch", e)
	}
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(a.theme)
		ui.Column(c).Fill().Children(func() { a.attachmentSection(c, "issue", id) })
		a.attachmentDialog(c)
	}, 1000, 680)
	click(t, tt, "Add link")
	click(t, tt, "Link name")
	tt.Type("参考资料")
	click(t, tt, "Link URL")
	tt.Type("https://example.com/reference")
	click(t, tt, "Add")
	if len(a.data.Attachments) != 2 {
		t.Fatalf("link import: %s", a.err)
	}
	tt.Frame()
	tt.Frame()
	click(t, tt, "Delete attachment 中文.txt")
	if len(a.data.Attachments) != 1 {
		t.Fatal("file deletion failed")
	}
	capture(t, tt, "15-attachments")
}
func TestRecurringAndReminderGUIUseSharedScheduler(t *testing.T) {
	a := testApp()
	id := a.filtered()[0].ID
	a.editRecurrence("", a.data.Issues[id])
	a.scheduleForm.NextDue = time.Now().Format("2006-01-02")
	tt := ui.NewTester(a.view, 1000, 680)
	click(t, tt, "Repeat interval")
	tt.Command("selectAll")
	tt.Type("2")
	click(t, tt, "Save recurrence")
	if len(a.data.Recurrences) != 1 {
		t.Fatalf("recurrence save: %s", a.err)
	}
	a.remindIssue(id)
	a.reminderInput = time.Now().Add(-time.Minute).Format(time.RFC3339)
	tt.Frame()
	click(t, tt, "Set reminder")
	if len(a.data.Reminders) != 1 {
		t.Fatalf("reminder save: %s", a.err)
	}
	s, e := core.Open(filepath.Join(t.TempDir(), "schedule.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	_, e = s.Update(func(d *core.Data) error { *d = a.data.Clone(); return nil })
	if e != nil {
		t.Fatal(e)
	}
	before := len(a.data.Issues)
	data, fired, e := runStoreSchedule(s, time.Now())
	if e != nil {
		t.Fatal(e)
	}
	if len(fired) != 1 || len(data.Issues) != before+1 {
		t.Fatalf("schedule did not run: %d/%d", len(fired), len(data.Issues))
	}
	a.acceptExternal(data)
	a.deliverReminders(fired)
	a.navigate("reminders", "")
	tt.Frame()
	tt.Frame()
	click(t, tt, "Snooze 1 hour")
	if scheduledDue(a.data, time.Now()) {
		t.Fatal("snoozed reminder still due")
	}
	if a.data.Reminders[fired[0]].FiredAt != 0 {
		t.Fatal("snooze kept delivered flag")
	}
	click(t, tt, "Dismiss reminder: "+a.data.Issues[id].Code())
	capture(t, tt, "16-recurring-compact")
}
