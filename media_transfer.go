package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flow/internal/core"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/richtext"
	xhtml "golang.org/x/net/html"
)

// HTML data images must retain their source until Flow owns their bytes. The
// framework parser otherwise turns them into memory-only drawing objects.
func parseMediaHTML(raw []byte) (richtext.Text, error) {
	dom, err := xhtml.Parse(bytes.NewReader(raw))
	if err != nil {
		return richtext.Text{}, err
	}
	sources := map[string]string{}
	var visit func(*xhtml.Node)
	visit = func(n *xhtml.Node) {
		if n.Type == xhtml.ElementNode && n.Data == "img" {
			for i := range n.Attr {
				if n.Attr[i].Key == "src" {
					key := fmt.Sprintf("flow-import://html/%d", len(sources))
					sources[key], n.Attr[i].Val = n.Attr[i].Val, key
				}
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(dom)
	var html bytes.Buffer
	if err := xhtml.Render(&html, dom); err != nil {
		return richtext.Text{}, err
	}
	return mapMediaText(richtext.ParseHTML(html.String()), sources), nil
}

func mapMediaText(text richtext.Text, urls map[string]string) richtext.Text {
	var parts []richtext.Paragraph
	for _, p := range text.Paragraphs() {
		p.Runs = slices.Clone(p.Runs)
		for i := range p.Runs {
			a := &p.Runs[i].Attributes
			if to, ok := urls[a.Link]; ok {
				a.Link = to
			}
			if a.Attachment != nil {
				at := *a.Attachment
				if to, ok := urls[at.URL]; ok {
					at.URL = to
				}
				a.Attachment = &at
			}
		}
		parts = append(parts, p)
	}
	return richtext.TextOf(parts...)
}

var pastedImageClient = &http.Client{Timeout: 30 * time.Second}

func readPastedImage(source, name string) (core.Attachment, error) {
	var data []byte
	var err error
	if strings.HasPrefix(source, "data:") {
		meta, payload, found := strings.Cut(strings.TrimPrefix(source, "data:"), ",")
		if !found || !strings.HasPrefix(strings.ToLower(meta), "image/") {
			return core.Attachment{}, errors.New("invalid_clipboard_image")
		}
		if strings.HasSuffix(strings.ToLower(meta), ";base64") {
			data, err = io.ReadAll(io.LimitReader(base64.NewDecoder(base64.StdEncoding, strings.NewReader(payload)), core.MaxAttachmentBytes+1))
		} else {
			var decoded string
			decoded, err = url.PathUnescape(payload)
			data = []byte(decoded)
		}
	} else {
		u, parseErr := url.Parse(source)
		if parseErr != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil {
			return core.Attachment{}, errors.New("invalid_clipboard_image")
		}
		response, fetchErr := pastedImageClient.Get(source)
		if fetchErr != nil {
			return core.Attachment{}, errors.New("image_import_failed")
		}
		defer response.Body.Close()
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			return core.Attachment{}, errors.New("image_import_failed")
		}
		data, err = io.ReadAll(io.LimitReader(response.Body, core.MaxAttachmentBytes+1))
		if name == "" {
			name = path.Base(u.Path)
		}
	}
	if err != nil {
		return core.Attachment{}, errors.New("image_import_failed")
	}
	if len(data) > core.MaxAttachmentBytes {
		return core.Attachment{}, errors.New("attachment_too_large")
	}
	if decodeMedia(data) == nil {
		return core.Attachment{}, errors.New("invalid_clipboard_image")
	}
	if strings.TrimSpace(name) == "" || name == "/" || name == "." {
		name = "Pasted image"
	}
	return core.Attachment{Name: name, MIME: http.DetectContentType(data), Data: data}, nil
}

func prepareRichPaste(format transfer.Format, raw []byte, files map[string]core.Attachment) (richtext.Text, map[string]core.Attachment, error) {
	var content richtext.Text
	var err error
	switch format {
	case richtext.Format:
		err = json.Unmarshal(raw, &content)
	case transfer.HTML:
		content, err = parseMediaHTML(raw)
	case transfer.Markdown:
		content = richtext.ParseMarkdown(string(raw))
	}
	if err != nil {
		return content, nil, err
	}
	records := map[string]core.Attachment{}
	for _, paragraph := range content.Paragraphs() {
		for _, run := range paragraph.Runs {
			refs := []string{run.Attributes.Link}
			at := run.Attributes.Attachment
			if at != nil {
				refs = append(refs, at.URL)
			}
			for _, ref := range refs {
				if _, exists := records[ref]; exists {
					continue
				}
				if strings.HasPrefix(ref, attachmentURLPrefix) {
					file, exists := files[strings.TrimPrefix(ref, attachmentURLPrefix)]
					if !exists {
						return content, nil, errors.New("attachment_not_found")
					}
					records[ref] = file
				} else if at != nil && ref == at.URL {
					file, readErr := readPastedImage(ref, at.Name)
					if readErr != nil {
						return content, nil, readErr
					}
					records[ref] = file
				}
			}
		}
	}
	return content, records, nil
}

// Ordinary text/HTML continues through mygo. Rich pastes containing media use
// one transactional import and one editor undo step, preserving all formatting.
func (a *app) pasteRichMedia(target editorMediaTarget, doc *richtext.Document, services ui.Services) bool {
	data := services.ReadClipboardData(richtext.Format, transfer.HTML, transfer.Markdown)
	for _, format := range []transfer.Format{richtext.Format, transfer.HTML, transfer.Markdown} {
		raw, err := data.Read(format)
		if err != nil {
			continue
		}
		text := string(raw)
		if strings.Contains(text, attachmentURLPrefix) || format == transfer.HTML && strings.Contains(strings.ToLower(text), "<img") || format == transfer.Markdown && strings.Contains(text, "![") {
			a.importRichPaste(target, doc, format, raw)
			return true
		}
		// A richer non-media representation must win over a lower-priority one.
		if format != richtext.Format || !strings.Contains(text, `"attachment"`) {
			return false
		}
	}
	return false
}

func (a *app) importRichPaste(target editorMediaTarget, doc *richtext.Document, format transfer.Format, raw []byte) {
	anchor, caret := doc.Selection()
	version := doc.Version()
	files := maps.Clone(a.data.Attachments) // immutable attachment byte snapshots
	var initial *core.IssueDraft
	if target.kind == "draft" && target.id == a.composeID {
		initial = &core.IssueDraft{ID: target.id, Context: a.composeKey, Form: a.compositionForm(), Relation: a.newRelation.Clone()}
	}
	var insertion richtext.Text
	read := func() (map[string]core.Attachment, error) {
		var records map[string]core.Attachment
		var err error
		insertion, records, err = prepareRichPaste(format, raw, files)
		return records, err
	}
	write := func(d *core.Data, records map[string]core.Attachment) error {
		if initial != nil {
			if _, exists := d.Drafts[target.id]; !exists {
				if _, err := d.SaveDraft(*initial); err != nil {
					return err
				}
			}
		}
		mapping := map[string]string{}
		for _, ref := range slices.Sorted(maps.Keys(records)) {
			file := records[ref]
			// Even same-owner pastes get independent bytes: deleting the original
			// attachment must not invalidate content pasted as a new copy.
			file.OwnerKind, file.OwnerID = target.kind, target.id
			copy, err := d.AddAttachment(file)
			if err != nil {
				return err
			}
			mapping[ref] = attachmentURLPrefix + copy.ID
		}
		insertion = mapMediaText(insertion, mapping)
		return nil
	}
	after := func() {
		original := doc
		if target.kind == "issue" && !target.local {
			if draft := a.drafts[target.id]; draft != nil {
				doc = draft.doc
			}
		}
		insertion = a.hydrateMediaText(insertion)
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
