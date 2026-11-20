package core

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

func (d Data) IsDescendant(id, ancestor string) bool {
	seen := map[string]bool{}
	for id != "" && !seen[id] {
		seen[id] = true
		id = d.Issues[id].ParentID
		if id == ancestor && id != "" {
			return true
		}
	}
	return false
}
func (d Data) Descendants(id string) []string {
	children := map[string][]string{}
	for key, i := range d.Issues {
		children[i.ParentID] = append(children[i.ParentID], key)
	}
	for _, ids := range children {
		slices.SortFunc(ids, func(a, b string) int {
			if d.Issues[a].Position < d.Issues[b].Position {
				return -1
			}
			if d.Issues[a].Position > d.Issues[b].Position {
				return 1
			}
			return strings.Compare(a, b)
		})
	}
	out, queue, seen := []string{}, []string{id}, map[string]bool{id: true}
	for len(queue) > 0 {
		next := queue[0]
		queue = queue[1:]
		for _, child := range children[next] {
			if !seen[child] {
				seen[child] = true
				out = append(out, child)
				queue = append(queue, child)
			}
		}
	}
	return out
}
func (d Data) cleanIssueScope(i *Issue) {
	i.LabelIDs = slices.DeleteFunc(slices.Clone(i.LabelIDs), func(id string) bool {
		l, ok := d.Labels[id]
		return !ok || l.EntityKind() != "issue" || l.ProjectID != "" && l.ProjectID != i.ProjectID
	})
	if m, ok := d.Milestones[i.MilestoneID]; !ok || m.ProjectID != i.ProjectID {
		i.MilestoneID = ""
	}
}
func (d *Data) ReparentIssue(id, parent string) error {
	i, ok := d.Issues[id]
	if !ok {
		return errors.New("not_found")
	}
	i.ParentID = parent
	return d.PutIssue(i)
}

// CreateFromForm consumes a validated draft/template without reusing identity,
// timestamps, deletion state or favorites. Creation and draft removal can share
// the surrounding Store.Update transaction.
func (d *Data) CreateFromForm(form Issue) (Issue, error) {
	if !validForm(form) || !validName(form.Title) {
		return Issue{}, errors.New("invalid_issue")
	}
	if form.ProjectID != "" && !d.ownerActive("project", form.ProjectID) {
		return Issue{}, errors.New("invalid_project_reference")
	}
	if form.Status == "" {
		form.Status = "backlog"
	}
	i, e := d.AddIssue(form.Title, form.ProjectID, form.ParentID, form.Status)
	if e != nil {
		return Issue{}, e
	}
	i.Body, i.Plain, i.Priority, i.Start, i.Due, i.MilestoneID = slices.Clone(form.Body), form.Plain, form.Priority, form.Start, form.Due, form.MilestoneID
	i.LabelIDs = slices.Clone(form.LabelIDs)
	i.NormalizeDates()
	d.cleanIssueScope(&i)
	d.Issues[i.ID] = i
	return i, nil
}
func (d *Data) SaveDraft(v IssueDraft) (IssueDraft, error) {
	if !validForm(v.Form) || !validForm(v.Base) || !v.Relation.Valid() || v.IssueID != "" && v.Relation != nil {
		return v, errors.New("invalid_draft")
	}
	if v.ID == "" {
		v.ID = ID()
	}
	v.UpdatedAt = time.Now().UnixMilli()
	v.Form = cloneIssue(v.Form)
	v.Base = cloneIssue(v.Base)
	v.Relation = v.Relation.Clone()
	d.Drafts[v.ID] = v
	return v, nil
}
func (d *Data) CreateDraftIssue(id string) (Issue, error) {
	v, ok := d.Drafts[id]
	if !ok {
		return Issue{}, errors.New("not_found")
	}
	i, e := d.CreateWithRelation(v.Form, v.Relation)
	if e != nil {
		return Issue{}, e
	}
	d.AdoptDraftAttachments(id, i.ID)
	d.RemoveDraft(id)
	return i, nil
}
func MergeIssueEdits(base, edited, current Issue) Issue {
	if edited.Title != base.Title {
		current.Title = edited.Title
	}
	if !slices.Equal(edited.Body, base.Body) || edited.Plain != base.Plain {
		current.Body, current.Plain = slices.Clone(edited.Body), edited.Plain
	}
	if edited.Status != base.Status {
		current.Status = edited.Status
	}
	if edited.Priority != base.Priority {
		current.Priority = edited.Priority
	}
	if edited.ProjectID != base.ProjectID {
		current.ProjectID = edited.ProjectID
	}
	if edited.ParentID != base.ParentID {
		current.ParentID = edited.ParentID
	}
	if edited.MilestoneID != base.MilestoneID {
		current.MilestoneID = edited.MilestoneID
	}
	if !slices.Equal(edited.LabelIDs, base.LabelIDs) {
		current.LabelIDs = slices.Clone(edited.LabelIDs)
	}
	if edited.Due != base.Due {
		current.Due = edited.Due
	}
	if edited.Start != base.Start {
		current.Start = edited.Start
	}
	if edited.Favorite != base.Favorite {
		current.Favorite = edited.Favorite
	}
	return current
}
func (d *Data) ApplyDraft(id string) (Issue, error) {
	v, ok := d.Drafts[id]
	if !ok {
		return Issue{}, errors.New("not_found")
	}
	if v.IssueID == "" {
		return d.CreateDraftIssue(id)
	}
	i, ok := d.Issues[v.IssueID]
	if !ok {
		return Issue{}, errors.New("not_found")
	}
	if e := d.PutIssue(MergeIssueEdits(v.Base, v.Form, i)); e != nil {
		return Issue{}, e
	}
	d.AdoptDraftAttachments(id, i.ID)
	d.RemoveDraft(id)
	return d.Issues[i.ID], nil
}
func (d *Data) SaveTemplate(v IssueTemplate) (IssueTemplate, error) {
	if !validName(v.Name) || !validForm(v.Form) {
		return v, errors.New("invalid_template")
	}
	if v.ID == "" {
		v.ID = ID()
	}
	v.Form = cloneIssue(v.Form)
	v.Form.ID = ""
	v.Form.ParentID = ""
	v.Form.Number = 0
	v.Form.DeletedAt = 0
	v.Form.Favorite = false
	v.Form.CreatedAt = 0
	v.Form.UpdatedAt = 0
	v.UpdatedAt = time.Now().UnixMilli()
	previous, existed := d.Templates[v.ID]
	d.Templates[v.ID] = v
	body, err := d.CloneBodyAttachments(v.Form.Body, "template", v.ID)
	if err != nil {
		if existed {
			d.Templates[v.ID] = previous
		} else {
			delete(d.Templates, v.ID)
		}
		return v, err
	}
	v.Form.Body = body
	d.Templates[v.ID] = v
	return v, nil
}
func (d *Data) CopyIssue(id, title string, children bool) (Issue, error) {
	original, ok := d.Issues[id]
	if !ok || original.DeletedAt != 0 {
		return Issue{}, errors.New("not_found")
	}
	ids := []string{id}
	if children {
		ids = append(ids, d.Descendants(id)...)
	}
	mapping := map[string]string{}
	var first Issue
	for _, oldID := range ids {
		form := d.Issues[oldID]
		if form.DeletedAt != 0 {
			continue
		}
		if oldID == id && strings.TrimSpace(title) != "" {
			form.Title = title
		}
		if parent, ok := mapping[form.ParentID]; ok {
			form.ParentID = parent
		}
		i, e := d.CreateFromForm(form)
		if e != nil {
			return Issue{}, e
		}
		mapping[oldID] = i.ID
		if oldID == id {
			first = i
		}
	}
	// Iterate a snapshot: newly inserted attachments/documents are not copied again.
	attachments := slices.Collect(mapValues(d.Attachments))
	documents := slices.Collect(mapValues(d.Documents))
	attachmentMapping := map[string]string{}
	for _, v := range attachments {
		if to := mapping[v.OwnerID]; v.OwnerKind == "issue" && to != "" {
			oldID := v.ID
			v.ID = ID()
			attachmentMapping[AttachmentURLPrefix+oldID] = AttachmentURLPrefix + v.ID
			v.OwnerID = to
			v.CreatedAt = time.Now().UnixMilli()
			v.Data = slices.Clone(v.Data)
			d.Attachments[v.ID] = v
		}
	}
	for _, v := range documents {
		if to := mapping[v.OwnerID]; v.OwnerKind == "issue" && to != "" {
			v.Body = remapMarkdownAttachments(v.Body, attachmentMapping)
			v.ID = ID()
			v.OwnerID = to
			v.CreatedAt = time.Now().UnixMilli()
			v.UpdatedAt = v.CreatedAt
			d.Documents[v.ID] = v
		}
	}
	for _, id := range mapping {
		i := d.Issues[id]
		i.Body = remapAttachmentBody(i.Body, attachmentMapping)
		d.Issues[id] = i
	}
	return d.Issues[first.ID], nil
}
func (d *Data) AddRelation(from, to, kind string) (Relation, error) {
	if !d.ownerActive("issue", from) || !d.ownerActive("issue", to) || from == to || !slices.Contains([]string{"related", "blocks", "duplicate"}, kind) {
		return Relation{}, errors.New("invalid_relation")
	}
	if kind == "related" && from > to {
		from, to = to, from
	}
	for _, r := range d.Relations {
		if r.Kind == kind && (r.From == from && r.To == to || kind == "related" && r.From == to && r.To == from) {
			return r, nil
		}
	}
	r := Relation{ID: ID(), From: from, To: to, Kind: kind, CreatedAt: time.Now().UnixMilli()}
	d.Relations[r.ID] = r
	if e := d.validateRecords(); e != nil {
		delete(d.Relations, r.ID)
		return Relation{}, e
	}
	if kind == "duplicate" {
		i := d.Issues[from]
		i.Status = "duplicate"
		if e := d.PutIssue(i); e != nil {
			delete(d.Relations, r.ID)
			return Relation{}, e
		}
	}
	return r, nil
}
func (d *Data) SaveComment(v Comment) (Comment, error) {
	if !d.ownerActive("issue", v.IssueID) || strings.TrimSpace(v.Body) == "" {
		return v, errors.New("invalid_comment")
	}
	if v.ID == "" {
		if v.ParentID != "" {
			parent, ok := d.Comments[v.ParentID]
			if !ok || parent.IssueID != v.IssueID || parent.DeletedAt != 0 {
				return v, errors.New("invalid_comment_parent")
			}
		}
		v.ID = ID()
		v.CreatedAt = time.Now().UnixMilli()
	} else {
		old, ok := d.Comments[v.ID]
		if !ok || old.IssueID != v.IssueID || old.DeletedAt != 0 {
			return v, errors.New("not_found")
		}
		v.CreatedAt = old.CreatedAt
		if v.ParentID != "" && v.ParentID != old.ParentID {
			return v, errors.New("invalid_comment_parent")
		}
		// Editing a body cannot move the reply to a different thread. Older
		// callers may omit parentId, so retain its original association.
		v.ParentID = old.ParentID
	}
	v.DeletedAt = 0
	v.UpdatedAt = time.Now().UnixMilli()
	d.Comments[v.ID] = v
	return v, nil
}
func (d *Data) AddAttachment(v Attachment) (Attachment, error) {
	if !d.attachmentOwnerActive(v.OwnerKind, v.OwnerID) || !validName(v.Name) {
		return v, errors.New("invalid_attachment")
	}
	if len(v.Data) > MaxAttachmentBytes {
		return v, errors.New("attachment_too_large")
	}
	if v.URL != "" {
		if !validURL(v.URL) || len(v.Data) != 0 {
			return v, errors.New("invalid_attachment_url")
		}
		v.SHA256 = ""
	} else {
		v.Name = filepath.Base(v.Name)
		sum := sha256.Sum256(v.Data)
		v.SHA256 = hex.EncodeToString(sum[:])
		v.Data = slices.Clone(v.Data)
	}
	v.ID = ID()
	v.CreatedAt = time.Now().UnixMilli()
	d.Attachments[v.ID] = v
	return v, nil
}
func (d *Data) RemoveLabel(id string) {
	delete(d.Labels, id)
	d.cleanLabelAssignments()
}
func (d *Data) RemoveMilestone(id string) {
	delete(d.Milestones, id)
	for key, i := range d.Issues {
		if i.MilestoneID == id {
			i.MilestoneID = ""
			d.Issues[key] = i
		}
	}
}

// mapValues has deterministic callers only where ordering is irrelevant.
func mapValues[T any](m map[string]T) func(func(T) bool) {
	return func(yield func(T) bool) {
		for _, v := range m {
			if !yield(v) {
				return
			}
		}
	}
}
