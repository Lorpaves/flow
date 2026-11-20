package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flow/internal/core"
	"fmt"
	"image"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
	"github.com/egoist/mygo/ui/richtext"
)

const attachmentURLPrefix = core.AttachmentURLPrefix

type editorMediaTarget struct {
	kind, id string
	// Local editors (comments, documents and updates) keep their own document,
	// while files belong to their containing issue or project.
	local   bool
	changed func()
}
type mediaImage struct{ source ui.ImageSource }

// Decode only bounded raster images. Unsupported or oversized pictures remain
// downloadable attachments instead of risking a huge allocation on preview.
func decodeMedia(data []byte) ui.ImageSource {
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || config.Width <= 0 || config.Height <= 0 || int64(config.Width)*int64(config.Height) > 32*1024*1024 {
		return nil
	}
	b, err := ui.DecodeBitmap(data)
	if err != nil {
		return nil
	}
	return b
}

func (a *app) mediaImage(v core.Attachment) *mediaImage {
	if a.mediaImages == nil {
		a.mediaImages = map[string]*mediaImage{}
	}
	key := v.ID + "/" + v.SHA256
	if cached := a.mediaImages[key]; cached != nil {
		return cached
	}
	cached := &mediaImage{}
	a.mediaImages[key] = cached
	if a.window == nil {
		cached.source = decodeMedia(v.Data)
		return cached
	}
	w := a.window
	data := v.Data // immutable snapshot; workers never read app state
	go func() {
		source := decodeMedia(data)
		w.Update(func() { cached.source = source })
	}()
	return cached
}

func (a *app) mediaDocument(i core.Issue) *richtext.Document {
	doc := documentFor(i)
	return richtext.NewDocument(a.hydrateMediaText(doc.Text()))
}

func (a *app) hydrateMediaText(text richtext.Text) richtext.Text {
	// Clone attachment objects before installing app-owned drawing callbacks.
	text = mapMediaText(text, nil)
	for _, p := range text.Paragraphs() {
		for _, r := range p.Runs {
			at := r.Attributes.Attachment
			if at == nil || !strings.HasPrefix(at.URL, attachmentURLPrefix) {
				continue
			}
			v, exists := a.data.Attachments[strings.TrimPrefix(at.URL, attachmentURLPrefix)]
			if !exists {
				continue
			}
			cached := a.mediaImage(v)
			if at.Width <= 0 || at.Height <= 0 || at.Width > 480 || at.Height > 300 {
				config, _, err := image.DecodeConfig(bytes.NewReader(v.Data))
				if err == nil && config.Width > 0 && config.Height > 0 {
					w, h := float32(config.Width), float32(config.Height)
					scale := min(float32(1), min(480/w, 300/h))
					at.Width, at.Height = w*scale, h*scale
				}
			}
			at.Draw = func(p *ui.Painter, rect ui.Rect) {
				if cached.source != nil {
					p.Image(cached.source, rect, ui.Contain)
				}
			}
		}
	}
	return text
}

func (a *app) mediaMarkdownOptions() *markdown.Options {
	// Options owns the framework image cache, so keep it between frames.
	keys := make([]string, 0, len(a.data.Attachments))
	for id, v := range a.data.Attachments {
		keys = append(keys, id+"/"+v.SHA256)
	}
	slices.Sort(keys)
	key := strings.Join(keys, ";")
	if a.mediaMarkdown != nil && a.mediaMarkdownKey == key {
		return a.mediaMarkdown
	}
	// The loader runs off the UI thread. Capture an immutable map snapshot.
	files := make(map[string][]byte, len(a.data.Attachments))
	for id, v := range a.data.Attachments {
		files[attachmentURLPrefix+id] = v.Data
	}
	a.mediaMarkdownKey = key
	a.mediaMarkdown = &markdown.Options{OnLink: func(url string) {
		if strings.HasPrefix(url, attachmentURLPrefix) {
			if v, ok := a.data.Attachments[strings.TrimPrefix(url, attachmentURLPrefix)]; ok {
				a.exportAttachment(v)
			}
		} else {
			a.services.OpenURL(url)
		}
	}, HTML: a.richHTMLView, LoadImage: func(url string) (ui.ImageSource, error) {
		if data, ok := files[url]; ok {
			if source := decodeMedia(data); source != nil {
				return source, nil
			}
		}
		return nil, errors.New("image_unavailable")
	}}
	return a.mediaMarkdown
}

func (a *app) persistMediaDocument(target editorMediaTarget, doc *richtext.Document) {
	if doc == nil {
		return
	}
	if target.changed != nil {
		target.changed()
	}
	if target.local {
		return
	}
	if target.kind == "issue" {
		if d := a.drafts[target.id]; d != nil && d.doc == doc {
			a.touch(d, time.Now())
		}
		return
	}
	if target.kind != "draft" {
		return
	}
	if a.composeID == target.id && a.newDoc == doc {
		a.persistComposition()
		return
	}
	for key, cached := range a.compositions {
		if cached.id == target.id && cached.doc == doc {
			cached.issue.Body, _ = json.Marshal(doc.Text())
			cached.issue.Plain = doc.Text().PlainText()
			a.compositions[key] = cached
			a.queueComposition(core.IssueDraft{ID: target.id, Context: key, Form: cached.issue, Relation: cached.relation.Clone()})
			return
		}
	}
}

// File reads/decoding precede the transaction. The document is captured, so a
// delayed picker/import cannot insert content into a newly opened issue/draft.
func (a *app) importEditorMedia(target editorMediaTarget, doc *richtext.Document, paths []string, data transfer.Data) {
	paths = slices.Clone(paths)
	anchor, caret, version := 0, 0, uint64(0)
	if doc != nil {
		anchor, caret = doc.Selection()
		version = doc.Version()
	}
	var initial *core.IssueDraft
	if target.kind == "draft" && a.composeID == target.id {
		initial = &core.IssueDraft{ID: target.id, Context: a.composeKey, Form: a.compositionForm(), Relation: a.newRelation.Clone()}
	}
	if target.kind == "draft" && initial == nil {
		for key, cached := range a.compositions {
			if cached.id == target.id {
				form := cached.issue
				form.Body, _ = json.Marshal(cached.doc.Text())
				form.Plain = cached.doc.Text().PlainText()
				initial = &core.IssueDraft{ID: target.id, Context: key, Form: form, Relation: cached.relation.Clone()}
				break
			}
		}
	}
	var saved []core.Attachment
	var images []ui.ImageSource
	read := func() ([]core.Attachment, error) {
		var records []core.Attachment
		for _, path := range paths {
			v, err := core.AttachmentFromFile(path)
			if err != nil {
				return nil, err
			}
			records = append(records, v)
		}
		for index, item := range data.Items() {
			if !slices.Contains(item.Formats(), transfer.PNG) {
				continue
			}
			b, err := item.Read(transfer.PNG)
			if err != nil {
				return nil, err
			}
			if len(b) > core.MaxAttachmentBytes {
				return nil, errors.New("attachment_too_large")
			}
			if _, format, err := image.DecodeConfig(bytes.NewReader(b)); err != nil || format != "png" {
				return nil, errors.New("invalid_attachment_file")
			}
			records = append(records, core.Attachment{Name: fmt.Sprintf("Image-%s-%d.png", time.Now().Format("20060102-150405"), index+1), MIME: "image/png", Data: b})
		}
		images = make([]ui.ImageSource, len(records))
		for n, v := range records {
			images[n] = decodeMedia(v.Data)
		}
		return records, nil
	}
	write := func(d *core.Data, records []core.Attachment) error {
		if len(records) == 0 {
			return nil
		}
		if initial != nil {
			if _, exists := d.Drafts[target.id]; !exists {
				if _, err := d.SaveDraft(*initial); err != nil {
					return err
				}
			}
		}
		for _, v := range records {
			v.OwnerKind, v.OwnerID = target.kind, target.id
			v, err := d.AddAttachment(v)
			if err != nil {
				return err
			}
			saved = append(saved, v)
		}
		return nil
	}
	after := func() {
		if doc == nil {
			return
		}
		original := doc
		if target.kind == "issue" && !target.local {
			if d := a.drafts[target.id]; d != nil {
				doc = d.doc
			}
		}
		parts := []richtext.Paragraph{}
		for n, v := range saved {
			attrs := richtext.Attributes{Link: attachmentURLPrefix + v.ID}
			text := v.Name
			if source := images[n]; source != nil {
				w, h := ui.ImageSize(source)
				scale := min(float32(1), min(480/w, 300/h))
				attrs.Attachment = &richtext.Attachment{Name: v.Name, URL: attachmentURLPrefix + v.ID, Image: source, Width: w * scale, Height: h * scale}
				text = string(richtext.ObjectReplacement)
			}
			parts = append(parts, richtext.Paragraph{Runs: []richtext.Run{{Text: text, Attributes: attrs}}})
		}
		if len(parts) == 0 {
			return
		}
		// A single insertion gives media and its surrounding line breaks one undo.
		parts = append(parts, richtext.Paragraph{})
		insertion := richtext.TextOf(parts...)
		unchanged := doc == original && doc.Version() == version
		doc.Edit(func(e *richtext.Edit) {
			start, end := min(anchor, caret), max(anchor, caret)
			if !unchanged {
				start, end = e.Text().Len(), e.Text().Len()
			}
			e.Replace(start, end, insertion)
			e.Select(start+insertion.Len(), start+insertion.Len())
		})
		a.persistMediaDocument(target, doc)
	}
	if a.window == nil {
		records, err := read()
		if err != nil {
			a.err = err.Error()
			return
		}
		a.apply(func(d *core.Data) error { return write(d, records) }, after)
		return
	}
	a.runDataJob(func(s *core.Store) (core.Data, error) {
		records, err := read()
		if err != nil {
			return core.Data{}, err
		}
		return s.Update(func(d *core.Data) error { return write(d, records) })
	}, after)
}

func (a *app) chooseEditorMedia(target editorMediaTarget, doc *richtext.Document) {
	if a.window == nil {
		return
	}
	w, title := a.window, a.tr("添加附件", "Attach files")
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: w, Title: title, Multiple: true})
		w.Update(func() {
			if err != nil {
				a.err = err.Error()
				return
			}
			if len(paths) > 0 {
				a.importEditorMedia(target, doc, paths, transfer.Data{})
			}
		})
	}()
}

func (a *app) pasteEditorMedia(target editorMediaTarget, doc *richtext.Document, services ui.Services) bool {
	formats := services.ClipboardFormats()
	if !slices.Contains(formats, transfer.FileList) && a.pasteRichMedia(target, doc, services) {
		return true
	}
	if !slices.Contains(formats, transfer.FileList) && !slices.Contains(formats, transfer.PNG) {
		return false
	}
	data := services.ReadClipboardData(transfer.FileList, transfer.PNG)
	if slices.Contains(data.Formats(), transfer.FileList) {
		paths, err := data.Files()
		if err != nil {
			a.err = err.Error()
			return true
		}
		a.importEditorMedia(target, doc, paths, transfer.Data{})
	} else {
		a.importEditorMedia(target, doc, nil, data)
	}
	return true
}

func (a *app) editorMediaDrop(zone ui.Element, target editorMediaTarget, doc *richtext.Document) {
	if paths := zone.DroppedFiles(); len(paths) > 0 {
		a.importEditorMedia(target, doc, paths, transfer.Data{})
	}
	options := transfer.DropOptions{Formats: []transfer.Format{transfer.FileList, transfer.PNG}, Operations: transfer.Copy}
	if drop, ok := ui.DropData(zone, options); ok {
		if slices.Contains(drop.Data.Formats(), transfer.FileList) {
			paths, err := drop.Data.Files()
			if err != nil {
				a.err = err.Error()
			} else {
				a.importEditorMedia(target, doc, paths, transfer.Data{})
			}
		} else {
			a.importEditorMedia(target, doc, nil, drop.Data)
		}
	}
	_, over := ui.DataDragOver(zone, options)
	if over || zone.FileDragOver() {
		zone.Background(a.theme.Accent.Alpha(.12))
	}
}
