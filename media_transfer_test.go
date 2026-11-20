package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"flow/internal/core"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
)

func mediaHTMLClipboard(html string) transfer.Data {
	return transfer.New(transfer.NewItem(transfer.Bytes(transfer.HTML, []byte(html))))
}

func TestHTMLMediaPastePersistsImageOnlyDraft(t *testing.T) {
	a := testApp()
	a.newTask("", "backlog")
	tt := ui.NewTester(a.view, 1120, 880)
	click(t, tt, "New issue description")
	png := testMediaPNG(t)
	tt.SetClipboardData(mediaHTMLClipboard(`<img alt="嵌入图片" src="data:image/png;base64,` + base64.StdEncoding.EncodeToString(png) + `">`))
	tt.Command("paste")
	tt.Advance(time.Second)
	if a.err != "" || len(a.data.Attachments) != 1 {
		t.Fatalf("paste: %s", a.err)
	}
	body := a.data.Drafts[a.composeID].Form.Body
	if !strings.Contains(string(body), attachmentURLPrefix) || strings.Contains(string(body), "base64") {
		t.Fatalf("image-only draft lost durable body: %s", body)
	}
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
	restarted := newApp(store.Snapshot())
	restarted.newTask("", "backlog")
	if !strings.Contains(restarted.newDoc.Text().Markdown(), attachmentURLPrefix) {
		t.Fatal("restart lost inline image")
	}
	for _, file := range restarted.data.Attachments {
		if !bytes.Equal(file.Data, png) {
			t.Fatal("HTML image bytes changed")
		}
	}
	for _, p := range restarted.newDoc.Text().Paragraphs() {
		for _, r := range p.Runs {
			if at := r.Attributes.Attachment; at != nil && (at.Draw == nil || at.Width != 480 || at.Height != 160) {
				t.Fatal("image drawing/size lost")
			}
		}
	}
	restarted.newIssue.Title = "HTML 图片恢复"
	preview := ui.NewTester(restarted.view, 1120, 880)
	click(t, preview, "Expand / collapse")
	capture(t, preview, "32-html-media")
}

func TestRichClipboardGetsIndependentFilesAndOneUndo(t *testing.T) {
	a := testApp()
	a.newTask("", "backlog")
	a.newIssue.Title = "Source"
	tt := ui.NewTester(a.view, 1120, 880)
	click(t, tt, "New issue description")
	png := testMediaPNG(t)
	html := `<p><strong>粗体前缀</strong></p><p><img src="data:image/png;base64,` + base64.StdEncoding.EncodeToString(png) + `"></p><p>末尾</p>`
	tt.SetClipboardData(mediaHTMLClipboard(html))
	tt.Command("paste")
	sourceID := a.composeID
	tt.Command("selectAll")
	tt.Command("copy")
	clipboard := tt.ClipboardData()
	a.stashComposition()
	a.loadComposition(core.ID(), "copy-target", core.Issue{Title: "Target", Status: "todo"}, nil)
	tt.Frame()
	click(t, tt, "New issue description")
	tt.SetClipboardData(clipboard)
	tt.Command("paste")
	if a.err != "" || len(a.data.Attachments) != 2 {
		t.Fatalf("rich paste: %s, %d", a.err, len(a.data.Attachments))
	}
	if !strings.Contains(a.newDoc.Text().Markdown(), "**粗体前缀**") {
		t.Fatal("formatting lost")
	}
	tt.Command("undo")
	if a.newDoc.Text().Len() != 0 {
		t.Fatal("paste did not undo in one step")
	}
	tt.Command("redo")
	a.data.RemoveDraft(sourceID)
	for _, file := range a.data.Attachments {
		if file.OwnerID != a.composeID || !bytes.Equal(file.Data, png) || !strings.Contains(a.newDoc.Text().Markdown(), file.ID) {
			t.Fatal("copy still depends on source")
		}
	}
	a.persistComposition()
	if err := a.data.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestRemoteHTMLMediaDownloadDeduplicatesAndKeepsOrder(t *testing.T) {
	png := testMediaPNG(t)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "image/png")
		w.Write(png)
	}))
	a := testApp()
	a.newTask("", "backlog")
	tt := ui.NewTester(a.view, 1120, 880)
	click(t, tt, "New issue description")
	tt.SetClipboardData(mediaHTMLClipboard(`<p>before</p><p><img src="` + server.URL + `/image.png"></p><p><em>middle</em><img src="` + server.URL + `/image.png"></p><p>after</p>`))
	tt.Command("paste")
	server.Close()
	if a.err != "" || len(a.data.Attachments) != 1 || requests.Load() != 1 {
		t.Fatalf("remote media: %s / %d", a.err, requests.Load())
	}
	md := a.newDoc.Text().Markdown()
	if strings.Contains(md, server.URL) || !strings.Contains(md, "*middle*") || strings.Count(md, attachmentURLPrefix) != 2 {
		t.Fatal(md)
	}
	if strings.Index(md, "before") > strings.Index(md, "middle") || strings.Index(md, "middle") > strings.Index(md, "after") {
		t.Fatal("order changed")
	}
}

func TestInvalidRichMediaDoesNotPastePartialContent(t *testing.T) {
	a := testApp()
	a.newTask("", "backlog")
	a.newDoc.InsertText("keep")
	a.newDoc.SetSelection(0, 4)
	tt := ui.NewTester(a.view, 1120, 880)
	click(t, tt, "New issue description")
	tt.SetClipboardData(mediaHTMLClipboard(`<p>new words</p><img src="data:image/png;base64,` + base64.StdEncoding.EncodeToString(testMediaPNG(t)) + `"><img src="file:///unrequested/private/file">`))
	tt.Command("paste")
	if a.err == "" || len(a.data.Attachments) != 0 || a.newDoc.Text().PlainText() != "keep" {
		t.Fatal("partial paste modified content")
	}
}

func TestTemplateApplicationClonesMediaIntoNewDraft(t *testing.T) {
	a := testApp()
	a.newTask("", "backlog")
	a.newIssue.Title = "Template source"
	a.importEditorMedia(editorMediaTarget{kind: "draft", id: a.composeID}, a.newDoc, nil, transfer.New(transfer.NewItem(transfer.Bytes(transfer.PNG, testMediaPNG(t)))))
	form := a.compositionForm()
	template, err := a.data.SaveTemplate(core.IssueTemplate{Name: "Media template", Form: form})
	if err != nil {
		t.Fatal(err)
	}
	source := a.composeID
	a.useTemplate(template.ID)
	if a.composeID == source || a.err != "" {
		t.Fatal("template not applied", a.err)
	}
	a.data.RemoveTemplate(template.ID)
	a.data.RemoveDraft(source)
	for _, file := range a.data.Attachments {
		if file.OwnerID != a.composeID || !strings.Contains(a.newDoc.Text().Markdown(), file.ID) {
			t.Fatal("template retains shared files")
		}
	}
	if len(a.data.Attachments) != 1 {
		t.Fatal("unowned copies", len(a.data.Attachments))
	}
	body, _ := json.Marshal(a.newDoc.Text())
	var stored, shown any
	json.Unmarshal(a.data.Drafts[a.composeID].Form.Body, &stored)
	json.Unmarshal(body, &shown)
	if !reflect.DeepEqual(stored, shown) {
		t.Fatal("template draft differs from editor")
	}
}

func TestDelayedRichPastePreservesLaterEditsAndDraftSwitch(t *testing.T) {
	a := testApp()
	issue, _ := a.data.AddIssue("Source", "", "", "todo")
	file, _ := a.data.AddAttachment(core.Attachment{OwnerKind: "issue", OwnerID: issue.ID, Name: "reference.txt", Data: []byte("copied bytes")})
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
	a.importRichPaste(editorMediaTarget{kind: "draft", id: id}, doc, transfer.HTML, []byte(`<strong>pasted</strong> <a href="`+attachmentURLPrefix+file.ID+`">reference</a>`))
	work := <-a.jobs
	doc.SetSelection(doc.Text().Len(), doc.Text().Len())
	doc.InsertText(" after")
	a.stashComposition()
	a.loadComposition(core.ID(), "other", core.Issue{Title: "Other", Status: "todo"}, nil)
	data, err := work.run(store)
	work.done(data, err)
	if err != nil || !strings.Contains(doc.Text().PlainText(), "before after") || !strings.Contains(doc.Text().Markdown(), "**pasted**") || a.newDoc.Text().Len() != 0 {
		t.Fatal("delayed paste overwrote other editing", err)
	}
	for a.pending > 0 {
		next := <-a.jobs
		data, err = next.run(store)
		next.done(data, err)
	}
	if !strings.Contains(store.Snapshot().Drafts[id].Form.Plain, "pasted") {
		t.Fatal("switched draft did not save pasted body")
	}
}
