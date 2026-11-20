package core

import (
	"errors"
	"slices"
	"time"
)

// Erasure says that an issue or a project was deleted for good, from the
// trash. Flow keeps it after the record is gone: a merge otherwise keeps an
// issue or project one side no longer has (Merge), so another Mac that still
// has the record would bring it back.
type Erasure struct {
	ID   string `json:"id"`
	Kind string `json:"kind"` // "issue" or "project"
	At   int64  `json:"at"`
}

// EraseIssues deletes issues in the trash for good, with their sub-issues
// and what belongs to them: comments, history, relations, attachments,
// documents and reminders.
func (d *Data) EraseIssues(ids []string) error {
	gone := map[string]bool{}
	for _, id := range ids {
		i, ok := d.Issues[id]
		if !ok {
			return errors.New("not_found")
		}
		if i.DeletedAt == 0 {
			return errors.New("not_in_trash")
		}
		gone[id] = true
		for _, k := range d.Descendants(id) {
			gone[k] = true
		}
	}
	d.erase(nil, gone)
	return nil
}

// EraseProject deletes a project in the trash for good, with its issues and
// what belongs to either.
func (d *Data) EraseProject(id string) error {
	p, ok := d.Projects[id]
	if !ok {
		return errors.New("not_found")
	}
	if p.DeletedAt == 0 {
		return errors.New("not_in_trash")
	}
	d.erase(map[string]bool{id: true}, d.projectIssues(id))
	return nil
}

// EmptyTrash deletes for good everything in the trash, and reports how many
// issues and projects that was.
func (d *Data) EmptyTrash() (issues, projects int) {
	gone, goneProjects := map[string]bool{}, map[string]bool{}
	for id, p := range d.Projects {
		if p.DeletedAt != 0 {
			goneProjects[id] = true
			for k := range d.projectIssues(id) {
				gone[k] = true
			}
		}
	}
	for id, i := range d.Issues {
		if i.DeletedAt != 0 {
			gone[id] = true
		}
	}
	d.erase(goneProjects, gone)
	return len(gone), len(goneProjects)
}

func (d Data) projectIssues(id string) map[string]bool {
	out := map[string]bool{}
	for k, i := range d.Issues {
		if i.ProjectID == id {
			out[k] = true
			for _, c := range d.Descendants(k) {
				out[c] = true
			}
		}
	}
	return out
}

func (d *Data) erase(projects, issues map[string]bool) {
	ensureMap(&d.Erased)
	now := time.Now().UnixMilli()
	for id := range projects {
		delete(d.Projects, id)
		d.Erased[id] = Erasure{ID: id, Kind: "project", At: now}
	}
	for id := range issues {
		delete(d.Issues, id)
		d.Erased[id] = Erasure{ID: id, Kind: "issue", At: now}
	}
	d.dropOrphans()
}

// applyErasures removes what either side of a merge deleted for good, even
// where the other side changed or restored it meanwhile: deleting for good
// is the later and the more deliberate of the two.
func (d *Data) applyErasures() {
	removed := false
	for id, v := range d.Erased {
		switch v.Kind {
		case "project":
			if _, ok := d.Projects[id]; ok {
				delete(d.Projects, id)
				removed = true
			}
		case "issue":
			if _, ok := d.Issues[id]; ok {
				delete(d.Issues, id)
				removed = true
			}
		}
	}
	if removed {
		d.dropOrphans()
	}
}

// dropOrphans removes the records whose owner is gone, and the references
// to it, until every record has its owner again: what a project or an issue
// deleted for good held goes with it.
func (d *Data) dropOrphans() {
	d.initRecords()
	for changed := true; changed; {
		changed = false
		for id, i := range d.Issues {
			_, project := d.Projects[i.ProjectID]
			_, parent := d.Issues[i.ParentID]
			if i.ProjectID != "" && !project || i.ParentID != "" && !parent {
				// An issue another Mac made in an erased project, or under
				// an erased issue, before it knew: it stays, on its own.
				if !project {
					i.ProjectID, i.MilestoneID = "", ""
					i.LabelIDs = slices.DeleteFunc(slices.Clone(i.LabelIDs), func(l string) bool { return d.Labels[l].ProjectID != "" })
				}
				if !parent || d.Issues[i.ParentID].ProjectID != i.ProjectID {
					i.ParentID = ""
				}
				d.Issues[id] = i
				changed = true
			}
		}
	}
	for id, v := range d.Milestones {
		if !d.ownerExists("project", v.ProjectID) {
			delete(d.Milestones, id)
		}
	}
	for id, i := range d.Issues {
		if _, ok := d.Milestones[i.MilestoneID]; i.MilestoneID != "" && !ok {
			i.MilestoneID = ""
			d.Issues[id] = i
		}
	}
	for id, v := range d.Labels {
		if v.ProjectID != "" && !d.ownerExists("project", v.ProjectID) {
			delete(d.Labels, id)
		}
	}
	for id, v := range d.Views {
		if v.ProjectID != "" && !d.ownerExists("project", v.ProjectID) {
			delete(d.Views, id)
		}
	}
	for id, p := range d.Projects {
		p.Dependencies = slices.DeleteFunc(slices.Clone(p.Dependencies), func(other string) bool { return !d.ownerExists("project", other) })
		d.Projects[id] = p
	}
	for id, v := range d.Activities {
		if !d.ownerExists(v.OwnerKind, v.OwnerID) {
			delete(d.Activities, id)
		}
	}
	for changed := true; changed; {
		changed = false
		for id, v := range d.Comments {
			_, parent := d.Comments[v.ParentID]
			if !d.ownerExists("issue", v.IssueID) || v.ParentID != "" && !parent {
				delete(d.Comments, id)
				changed = true
			}
		}
	}
	for id, v := range d.Relations {
		if !d.ownerExists("issue", v.From) || !d.ownerExists("issue", v.To) {
			delete(d.Relations, id)
		}
	}
	for id, v := range d.Documents {
		if !d.ownerExists(v.OwnerKind, v.OwnerID) {
			delete(d.Documents, id)
		}
	}
	for id, v := range d.Updates {
		if !d.ownerExists("project", v.ProjectID) {
			delete(d.Updates, id)
		}
	}
	for id, v := range d.Reminders {
		if kind, owner := v.Owner(); !d.ownerExists(kind, owner) {
			delete(d.Reminders, id)
		}
	}
	for id, v := range d.Drafts {
		if v.IssueID != "" && !d.ownerExists("issue", v.IssueID) || v.Relation != nil && !d.ownerExists("issue", v.Relation.IssueID) {
			delete(d.Drafts, id)
		}
	}
	for id, v := range d.Attachments {
		if !d.attachmentOwnerExists(v.OwnerKind, v.OwnerID) {
			delete(d.Attachments, id)
		}
	}
	for id, v := range d.Folders {
		v.Items = slices.DeleteFunc(slices.Clone(v.Items), func(ref FavoriteRef) bool {
			_, view := d.Views[ref.ID]
			return !(d.ownerExists(ref.Kind, ref.ID) || ref.Kind == "view" && view)
		})
		d.Folders[id] = v
	}
	// Forms that would make an issue in what is gone make one without it.
	clear := func(form Issue) Issue {
		if form.ProjectID != "" && !d.ownerExists("project", form.ProjectID) {
			form.ProjectID, form.MilestoneID = "", ""
		}
		if form.ParentID != "" && !d.ownerExists("issue", form.ParentID) {
			form.ParentID = ""
		}
		if _, ok := d.Milestones[form.MilestoneID]; form.MilestoneID != "" && !ok {
			form.MilestoneID = ""
		}
		form.LabelIDs = slices.DeleteFunc(slices.Clone(form.LabelIDs), func(l string) bool { _, ok := d.Labels[l]; return !ok })
		return form
	}
	for id, v := range d.Templates {
		v.Form = clear(v.Form)
		d.Templates[id] = v
	}
	for id, v := range d.Recurrences {
		v.Form = clear(v.Form)
		d.Recurrences[id] = v
	}
	for id, v := range d.Drafts {
		v.Form, v.Base = clear(v.Form), clear(v.Base)
		d.Drafts[id] = v
	}
}
