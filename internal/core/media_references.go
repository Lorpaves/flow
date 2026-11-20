package core

import (
	"bytes"
	"encoding/json"
	"errors"
	"slices"
	"strings"
)

// CloneBodyAttachments gives the target independent copies of referenced files.
// Only URL/link fields are examined; prose that mentions an ID is unchanged.
// Call within Store.Update with the operation creating the target record.
func (d *Data) CloneBodyAttachments(body json.RawMessage, kind, owner string) (json.RawMessage, error) {
	return d.cloneBodyAttachments(body, kind, owner, false)
}

func (d *Data) cloneBodyAttachments(body json.RawMessage, kind, owner string, allowMissing bool) (json.RawMessage, error) {
	if !d.attachmentOwnerActive(kind, owner) {
		return nil, errors.New("invalid_attachment")
	}
	if len(body) == 0 {
		return body, nil
	}
	var root any
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(&root); err != nil {
		return nil, err
	}
	refs := map[string]bool{}
	var visit func(any)
	visit = func(value any) {
		switch v := value.(type) {
		case []any:
			for _, child := range v {
				visit(child)
			}
		case map[string]any:
			for key, child := range v {
				if text, ok := child.(string); ok {
					if (key == "url" || key == "link") && strings.HasPrefix(text, AttachmentURLPrefix) {
						refs[strings.TrimPrefix(text, AttachmentURLPrefix)] = true
					}
				} else {
					visit(child)
				}
			}
		}
	}
	visit(root)
	ids := make([]string, 0, len(refs))
	for id := range refs {
		if _, exists := d.Attachments[id]; !exists {
			if allowMissing {
				continue
			}
			return nil, errors.New("attachment_not_found")
		}
		ids = append(ids, id)
	}
	slices.Sort(ids)
	mapping := map[string]string{}
	for _, id := range ids {
		file := d.Attachments[id]
		if file.OwnerKind == kind && file.OwnerID == owner {
			continue
		}
		file.OwnerKind, file.OwnerID = kind, owner
		copy, err := d.AddAttachment(file)
		if err != nil {
			return nil, err
		}
		mapping[AttachmentURLPrefix+id] = AttachmentURLPrefix + copy.ID
	}
	return remapAttachmentBody(body, mapping), nil
}

// Repair legacy shared references when opening/importing a workspace. An
// already-missing source cannot be recovered; retain its reference and all text.
func (d *Data) isolateTemplateMedia() error {
	for id, template := range d.Templates {
		body, err := d.cloneBodyAttachments(template.Form.Body, "template", id, true)
		if err != nil {
			return err
		}
		template.Form.Body = body
		d.Templates[id] = template
	}
	return nil
}

func (d *Data) RemoveTemplate(id string) {
	for key, file := range d.Attachments {
		if file.OwnerKind == "template" && file.OwnerID == id {
			delete(d.Attachments, key)
		}
	}
	delete(d.Templates, id)
}

func (d *Data) CreateFromTemplate(id string) (Issue, error) {
	v, exists := d.Templates[id]
	if !exists {
		return Issue{}, errors.New("not_found")
	}
	i, err := d.CreateFromForm(v.Form)
	if err != nil {
		return Issue{}, err
	}
	i.Body, err = d.CloneBodyAttachments(i.Body, "issue", i.ID)
	if err != nil {
		return Issue{}, err
	}
	d.Issues[i.ID] = i
	return i, nil
}
