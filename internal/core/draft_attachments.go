package core

import (
	"bytes"
	"encoding/json"
	"errors"
)

const AttachmentURLPrefix = "flow-attachment://"

// Rebind only exact URL values, preserving literal prose mentioning a file ID.
func remapAttachmentBody(body json.RawMessage, mapping map[string]string) json.RawMessage {
	if len(body) == 0 || len(mapping) == 0 {
		return body
	}
	var root any
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if decoder.Decode(&root) != nil {
		return body
	}
	changed := false
	var visit func(any) any
	visit = func(v any) any {
		switch x := v.(type) {
		case string:
			if id := mapping[x]; id != "" {
				changed = true
				return id
			}
		case []any:
			for n, child := range x {
				x[n] = visit(child)
			}
		case map[string]any:
			for key, child := range x {
				if key == "url" || key == "link" {
					x[key] = visit(child)
				} else if _, ok := child.(string); !ok {
					x[key] = visit(child)
				}
			}
		}
		return v
	}
	root = visit(root)
	if !changed {
		return body
	}
	updated, _ := json.Marshal(root)
	return updated
}

// CopyDraft creates an independent recovery copy, including draft-owned files.
func (d *Data) CopyDraft(id string) (IssueDraft, error) {
	v, ok := d.Drafts[id]
	if !ok {
		return IssueDraft{}, errors.New("not_found")
	}
	old, sourceIssue := v.ID, v.IssueID
	v.ID, v.IssueID, v.Relation = ID(), "", nil
	v.Context = "copy/" + v.ID
	v.Form.ID, v.Form.ParentID, v.Form.DeletedAt = "", "", 0
	copy, err := d.SaveDraft(v)
	if err != nil {
		return IssueDraft{}, err
	}
	files := []Attachment{}
	for _, file := range d.Attachments {
		if file.OwnerKind == "draft" && file.OwnerID == old || sourceIssue != "" && file.OwnerKind == "issue" && file.OwnerID == sourceIssue {
			files = append(files, file)
		}
	}
	mapping := map[string]string{}
	for _, file := range files {
		oldID := file.ID
		file.OwnerKind, file.OwnerID = "draft", copy.ID
		file, err = d.AddAttachment(file)
		if err != nil {
			return IssueDraft{}, err
		}
		mapping[AttachmentURLPrefix+oldID] = AttachmentURLPrefix + file.ID
	}
	copy.Form.Body = remapAttachmentBody(copy.Form.Body, mapping)
	d.Drafts[copy.ID] = copy
	return copy, nil
}

// Attachments may belong to a recoverable draft before an issue exists.
// Keep this separate from ownerExists: drafts cannot own activities or documents.
func (d Data) attachmentOwnerExists(kind, id string) bool {
	if kind == "template" {
		_, ok := d.Templates[id]
		return ok
	}
	if kind == "draft" {
		_, ok := d.Drafts[id]
		return ok
	}
	return d.ownerExists(kind, id)
}

func (d Data) attachmentOwnerActive(kind, id string) bool {
	if kind == "draft" || kind == "template" {
		return d.attachmentOwnerExists(kind, id)
	}
	return d.ownerActive(kind, id)
}

func (d Data) HasDraftAttachments(id string) bool {
	for _, v := range d.Attachments {
		if v.OwnerKind == "draft" && v.OwnerID == id {
			return true
		}
	}
	return false
}

// RemoveDraft also removes its embedded files. Call after transferring files
// when submitting; abandoned drafts must not leave unowned attachment bytes.
func (d *Data) RemoveDraft(id string) {
	for key, v := range d.Attachments {
		if v.OwnerKind == "draft" && v.OwnerID == id {
			delete(d.Attachments, key)
		}
	}
	delete(d.Drafts, id)
}

// AdoptDraftAttachments is used in the same transaction that creates/applies
// the issue. Stable IDs preserve inline references in its rich-text body.
func (d *Data) AdoptDraftAttachments(draftID, issueID string) {
	for key, v := range d.Attachments {
		if v.OwnerKind == "draft" && v.OwnerID == draftID {
			v.OwnerKind, v.OwnerID = "issue", issueID
			d.Attachments[key] = v
		}
	}
}
