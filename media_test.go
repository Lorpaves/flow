package main

import (
	"bytes"
	"flow/internal/core"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
)

func testMediaPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 480, 160))
	for y := 0; y < 160; y++ {
		for x := 0; x < 480; x++ {
			img.SetRGBA(x, y, color.RGBA{uint8(80 + x/4), uint8(70 + y/2), 170, 255})
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestCreationMediaPasteRestoreSubmitAndUndo(t *testing.T) {
	a := testApp()
	a.newTask("", "backlog")
	a.newIssue.Title = "图片与文件验收"
	tt := ui.NewTester(a.view, 1120, 880)
	click(t, tt, "New issue description")
	tt.SetClipboardData(transfer.New(transfer.NewItem(transfer.Bytes(transfer.PNG, testMediaPNG(t)))))
	// Native Edit-menu paste is an InputCommand, unlike a synthesized key.
	tt.Command("paste")
	id := a.composeID
	if len(a.data.Attachments) != 1 || !strings.Contains(a.newDoc.Text().Markdown(), attachmentURLPrefix) {
		t.Fatalf("media paste failed: %s / %s", a.err, a.newDoc.Text().Markdown())
	}
	tt.Command("undo")
	if a.newDoc.Text().Len() != 0 {
		t.Fatal("image insertion did not undo in one step")
	}
	tt.Command("redo")
	tt.Advance(time.Second)
	if !strings.Contains(string(a.data.Drafts[id].Form.Body), attachmentURLPrefix) {
		t.Fatal("inline reference not autosaved")
	}
	click(t, tt, "Expand / collapse")
	capture(t, tt, "31-create-media")
	dbPath := filepath.Join(t.TempDir(), "flow.db")
	store, err := core.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.Update(func(d *core.Data) error { *d = a.data.Clone(); return nil }); err != nil {
		t.Fatal(err)
	}
	store.Close()
	store, err = core.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	b := newApp(store.Snapshot())
	b.newTask("", "backlog")
	var restored bool
	for _, p := range b.newDoc.Text().Paragraphs() {
		for _, r := range p.Runs {
			if at := r.Attributes.Attachment; at != nil {
				restored = at.Draw != nil
			}
		}
	}
	if !restored || b.composeID != id {
		t.Fatal("restart lost image drawing or draft")
	}
	b.createIssue()
	for _, v := range b.data.Attachments {
		if v.OwnerKind != "issue" || v.OwnerID != b.createdIssue {
			t.Fatal("attachment not adopted")
		}
	}
	if b.data.HasDraftAttachments(id) || len(b.data.Drafts) != 0 {
		t.Fatal("submitted draft remains")
	}
	b.openIssue(b.createdIssue)
	tb := ui.NewTester(b.view, 1120, 880)
	var imageName string
	for _, attachment := range b.data.Attachments {
		imageName = attachment.Name
	}
	// Wait for the loaded image's geometry, not a delay that can capture
	// the placeholder on a slow runner. The fixture is 480 by 160 pixels.
	guard := time.NewTimer(time.Minute)
	defer guard.Stop()
	for {
		tb.Frame()
		if bounds, ok := tb.Find(imageName); ok && bounds.H >= 120 {
			break
		}
		select {
		case <-guard.C:
			t.Fatal("inline image never replaced its loading placeholder")
		default:
			runtime.Gosched()
		}
	}
	capture(t, tb, "31-issue-media")
}

func TestMediaFileDropAndPasteKeepPlainRichText(t *testing.T) {
	a := testApp()
	a.newTask("", "backlog")
	tt := ui.NewTester(a.view, 1120, 880)
	path := filepath.Join(t.TempDir(), "说明.txt")
	if err := os.WriteFile(path, []byte("文件内容"), 0600); err != nil {
		t.Fatal(err)
	}
	area, ok := tt.Find("New issue description")
	if !ok || !tt.DropFiles(area.X+12, area.Y+12, path) {
		t.Fatal("native file drop target missing")
	}
	if len(a.data.Attachments) != 1 || !strings.Contains(a.newDoc.Text().PlainText(), "说明.txt") {
		t.Fatalf("file drop failed: %s", a.err)
	}
	click(t, tt, "New issue description")
	tt.Command("selectAll")
	tt.Key(0, ui.KeyBackspace)
	// An attachment-only draft must survive clearing all description text.
	a.closeComposition()
	if !a.data.HasDraftAttachments(a.composeID) || a.data.Drafts[a.composeID].ID == "" {
		t.Fatal("empty draft erased its files")
	}
	a.newTask("", "backlog")
	tt.Frame()
	click(t, tt, "New issue description")
	files, _ := transfer.FileData(path)
	tt.SetClipboardData(files)
	tt.Key(ui.Cmd, ui.KeyV)
	if len(a.data.Attachments) != 2 {
		t.Fatal("file clipboard did not import")
	}
	tt.SetClipboardData(transfer.New(transfer.NewItem(transfer.Bytes(transfer.HTML, []byte("<b>rich text</b>")), transfer.Bytes(transfer.Text, []byte("rich text")))))
	tt.Command("paste")
	if !strings.Contains(a.newDoc.Text().Markdown(), "**rich text**") {
		t.Fatal("ordinary HTML paste changed")
	}
	tt.SetClipboard("普通文字")
	tt.Command("pasteAndMatchStyle")
	if !strings.Contains(a.newDoc.Text().PlainText(), "普通文字") {
		t.Fatal("matching style paste changed")
	}
	a.discardComposition()
	if len(a.data.Attachments) != 0 || len(a.data.Drafts) != 0 {
		t.Fatal("discard orphaned draft attachments")
	}
}

func TestDelayedMediaImportPreservesTypingAndOriginalDraft(t *testing.T) {
	a := testApp()
	a.newTask("", "backlog")
	a.newIssue.Title = "Original"
	doc, id := a.newDoc, a.composeID
	doc.InsertText("before")
	doc.SetSelection(0, doc.Text().Len())
	store, err := core.Open(filepath.Join(t.TempDir(), "flow.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err = store.Update(func(d *core.Data) error { *d = a.data.Clone(); return nil }); err != nil {
		t.Fatal(err)
	}
	a.window = &mygo.Window{}
	path := filepath.Join(t.TempDir(), "file.txt")
	os.WriteFile(path, []byte("content"), 0600)
	a.importEditorMedia(editorMediaTarget{kind: "draft", id: id}, doc, []string{path}, transfer.Data{})
	work := <-a.jobs
	// Changes while disk I/O is outstanding are never replaced by the paste.
	doc.SetSelection(doc.Text().Len(), doc.Text().Len())
	doc.InsertText(" after")
	a.stashComposition()
	a.loadComposition(core.ID(), "other", core.Issue{Title: "Other", Status: "todo"}, nil)
	data, err := work.run(store)
	work.done(data, err)
	if err != nil || a.err != "" {
		t.Fatalf("delayed import: %v / %s", err, a.err)
	}
	if !strings.Contains(doc.Text().PlainText(), "before after") || !strings.Contains(doc.Text().PlainText(), "file.txt") || a.newDoc.Text().Len() != 0 {
		t.Fatal("late import changed selection/text or inserted into another draft")
	}
	// Drain queued composition saves in their actual order.
	for a.pending > 0 {
		next := <-a.jobs
		data, err = next.run(store)
		next.done(data, err)
	}
	if !strings.Contains(store.Snapshot().Drafts[id].Form.Plain, "file.txt") {
		t.Fatal("switched draft did not save media body")
	}
}

func TestInvalidMediaBatchRollsBack(t *testing.T) {
	a := testApp()
	a.newTask("", "backlog")
	path := filepath.Join(t.TempDir(), "valid.txt")
	os.WriteFile(path, []byte("keep"), 0600)
	a.importEditorMedia(editorMediaTarget{kind: "draft", id: a.composeID}, a.newDoc, []string{path, path + ".missing"}, transfer.Data{})
	if a.err == "" || len(a.data.Attachments) != 0 || len(a.data.Drafts) != 0 || a.newDoc.Text().Len() != 0 {
		t.Fatal("partial import committed")
	}
	if decodeMedia([]byte("not an image")) != nil {
		t.Fatal("invalid image decoded")
	}
}

func TestHeadingFourSlashMenu(t *testing.T) {
	a := testApp()
	a.newTask("", "backlog")
	tt := ui.NewTester(a.view, 1120, 880)
	click(t, tt, "New issue description")
	tt.Type("/")
	click(t, tt, "Search block types")
	tt.Type("Heading 4")
	tt.Key(0, ui.KeyEnter)
	if a.newDoc.Text().ParagraphStyleAt(0).Heading != 4 {
		t.Fatal("H4 missing")
	}
}
