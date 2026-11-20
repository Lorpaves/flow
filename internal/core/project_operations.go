package core

import (
	"errors"
	"slices"
	"strings"
	"time"
)

func (d *Data) SaveMilestone(v Milestone) (Milestone, error) {
	if !d.ownerActive("project", v.ProjectID) || !validName(v.Name) || !ValidDate(v.Due) {
		return v, errors.New("invalid_milestone")
	}
	if v.ID == "" {
		v.ID = ID()
		v.Position = time.Now().UnixMilli()
	} else {
		old, ok := d.Milestones[v.ID]
		if !ok || old.ProjectID != v.ProjectID {
			return v, errors.New("not_found")
		}
	}
	v.Name = strings.TrimSpace(v.Name)
	d.Milestones[v.ID] = v
	return v, nil
}
func (d *Data) SaveDocument(v Document) (Document, error) {
	if !d.ownerActive(v.OwnerKind, v.OwnerID) || !validName(v.Name) {
		return v, errors.New("invalid_document")
	}
	if v.ID == "" {
		v.ID = ID()
		v.CreatedAt = time.Now().UnixMilli()
	} else {
		old, ok := d.Documents[v.ID]
		if !ok || old.OwnerKind != v.OwnerKind || old.OwnerID != v.OwnerID {
			return v, errors.New("not_found")
		}
		v.CreatedAt = old.CreatedAt
	}
	v.Name = strings.TrimSpace(v.Name)
	v.UpdatedAt = time.Now().UnixMilli()
	d.Documents[v.ID] = v
	return v, nil
}
func (d *Data) SaveProjectUpdate(v ProjectUpdate) (ProjectUpdate, error) {
	if !d.ownerActive("project", v.ProjectID) || strings.TrimSpace(v.Body) == "" || !validHealth(v.Health) {
		return v, errors.New("invalid_project_update")
	}
	setHealth := v.ID == ""
	if v.ID == "" {
		v.ID = ID()
		v.CreatedAt = time.Now().UnixMilli()
	} else {
		old, ok := d.Updates[v.ID]
		if !ok || old.ProjectID != v.ProjectID {
			return v, errors.New("not_found")
		}
		v.CreatedAt = old.CreatedAt
		setHealth = v.Health != old.Health
	}
	v.UpdatedAt = time.Now().UnixMilli()
	d.Updates[v.ID] = v
	// Correcting an older update's wording must not roll back newer project
	// health. A new update or an explicit health change does set the health.
	if setHealth {
		p := d.Projects[v.ProjectID]
		p.Health = v.Health
		d.Projects[p.ID] = p
	}
	return v, nil
}

// ConvertIssueToProject preserves the original issue in recoverable trash (and
// thus its comments/history). Direct children become top-level project issues;
// deeper descendants retain their hierarchy. Resources move with the conversion.
func (d *Data) ConvertIssueToProject(id string) (Project, error) {
	i, ok := d.Issues[id]
	if !ok || i.DeletedAt != 0 {
		return Project{}, errors.New("not_found")
	}
	p := Project{ID: ID(), Name: i.Title, Description: i.Plain, Status: "planned", Priority: i.Priority, Due: i.Due, Color: "#a799e8", CreatedAt: time.Now().UnixMilli()}
	d.Projects[p.ID] = p
	children := []string{}
	for key, ch := range d.Issues {
		if ch.ParentID == id {
			children = append(children, key)
		}
	}
	for _, child := range children {
		ch := d.Issues[child]
		ch.ParentID = ""
		ch.ProjectID = p.ID
		d.cleanIssueScope(&ch)
		d.Issues[child] = ch
		for _, descendant := range d.Descendants(child) {
			v := d.Issues[descendant]
			v.ProjectID = p.ID
			d.cleanIssueScope(&v)
			d.Issues[descendant] = v
		}
	}
	for key, v := range d.Attachments {
		if v.OwnerKind == "issue" && v.OwnerID == id {
			v.OwnerKind, v.OwnerID = "project", p.ID
			d.Attachments[key] = v
		}
	}
	for key, v := range d.Documents {
		if v.OwnerKind == "issue" && v.OwnerID == id {
			v.OwnerKind, v.OwnerID = "project", p.ID
			d.Documents[key] = v
		}
	}
	d.TrashIssue(id)
	return p, nil
}

// MergeProjectEdits applies only fields the open form changed. In particular,
// editing a description must not reset a newer MCP status or dependency edit.
func MergeProjectEdits(base, edit, current Project) Project {
	if edit.Name != base.Name {
		current.Name = edit.Name
	}
	if edit.Description != base.Description {
		current.Description = edit.Description
	}
	if edit.Status != base.Status {
		current.Status = edit.Status
	}
	if edit.Color != base.Color {
		current.Color = edit.Color
	}
	if edit.Start != base.Start {
		current.Start = edit.Start
	}
	if edit.Due != base.Due {
		current.Due = edit.Due
	}
	if edit.Priority != base.Priority {
		current.Priority = edit.Priority
	}
	if edit.Health != base.Health {
		current.Health = edit.Health
	}
	if !slices.Equal(edit.LabelIDs, base.LabelIDs) {
		current.LabelIDs = slices.Clone(edit.LabelIDs)
	}
	if !slices.Equal(edit.Dependencies, base.Dependencies) {
		current.Dependencies = slices.Clone(edit.Dependencies)
	}
	return current
}
func (d *Data) PutProject(p Project) error {
	old, exists := d.Projects[p.ID]
	if exists {
		if old.DeletedAt != 0 {
			return errors.New("project_in_trash")
		}
		p.CreatedAt, p.DeletedAt = old.CreatedAt, old.DeletedAt
	}
	p.Name = strings.TrimSpace(p.Name)
	if p.ID == "" || !validName(p.Name) || !ValidDate(p.Start) || !ValidDate(p.Due) || p.Start != "" && p.Due != "" && p.Start > p.Due || !slices.Contains(ProjectStatuses, p.Status) || p.Priority < 0 || p.Priority > 4 || !validHealth(p.Health) {
		return errors.New("invalid_project")
	}
	if e := d.validateProjectLabels(p.LabelIDs); e != nil {
		return e
	}
	p.LabelIDs = slices.Clone(p.LabelIDs)
	p.Dependencies = slices.Clone(p.Dependencies)
	d.Projects[p.ID] = p
	return nil
}
func MergeMilestoneEdits(base, edit, current Milestone) Milestone {
	if edit.Name != base.Name {
		current.Name = edit.Name
	}
	if edit.Description != base.Description {
		current.Description = edit.Description
	}
	if edit.Due != base.Due {
		current.Due = edit.Due
	}
	if edit.Completed != base.Completed {
		current.Completed = edit.Completed
	}
	return current
}
func MergeDocumentEdits(base, edit, current Document) Document {
	if edit.Name != base.Name {
		current.Name = edit.Name
	}
	if edit.Body != base.Body {
		current.Body = edit.Body
	}
	return current
}
func MergeProjectUpdateEdits(base, edit, current ProjectUpdate) ProjectUpdate {
	if edit.Body != base.Body {
		current.Body = edit.Body
	}
	if edit.Health != base.Health {
		current.Health = edit.Health
	}
	return current
}
func (d Data) MilestoneList(project string) []Milestone {
	out := []Milestone{}
	for _, v := range d.Milestones {
		if v.ProjectID == project {
			out = append(out, v)
		}
	}
	slices.SortFunc(out, func(x, y Milestone) int {
		if x.Position < y.Position {
			return -1
		}
		if x.Position > y.Position {
			return 1
		}
		return strings.Compare(x.ID, y.ID)
	})
	return out
}
func (d Data) MilestoneProgress(id string) (done, total int) {
	for _, i := range d.Issues {
		if i.MilestoneID == id && i.DeletedAt == 0 && i.Status != "canceled" && i.Status != "duplicate" {
			total++
			if i.Status == "done" {
				done++
			}
		}
	}
	return
}
