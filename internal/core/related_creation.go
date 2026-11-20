package core

import (
	"errors"
	"slices"
)

// CreationRelation describes the new issue's role relative to an existing one.
// Drafts may retain a deleted target so recovery never drops authored text.
type CreationRelation struct {
	IssueID string `json:"issueId"`
	Kind    string `json:"kind"` // related, sub_issue, parent_issue, blocked_issue, blocking_issue
}

func (r *CreationRelation) Clone() *CreationRelation {
	if r == nil {
		return nil
	}
	v := *r
	return &v
}
func (r *CreationRelation) Valid() bool {
	return r == nil || r.IssueID != "" && len(r.IssueID) <= 256 && slices.Contains([]string{"related", "sub_issue", "parent_issue", "blocked_issue", "blocking_issue"}, r.Kind)
}

// CreateWithRelation commits the issue, hierarchy and relation together even
// for in-memory callers. A missing/deleted target leaves the draft intact.
func (d *Data) CreateWithRelation(form Issue, relation *CreationRelation) (Issue, error) {
	if relation == nil {
		return d.CreateFromForm(form)
	}
	if !relation.Valid() {
		return Issue{}, errors.New("invalid_creation_relation")
	}
	target, ok := d.Issues[relation.IssueID]
	if !ok || target.DeletedAt != 0 {
		return Issue{}, errors.New("related_issue_unavailable")
	}
	if relation.Kind == "sub_issue" || relation.Kind == "parent_issue" {
		if form.ProjectID != "" && form.ProjectID != target.ProjectID {
			return Issue{}, errors.New("related_project_conflict")
		}
		form.ProjectID = target.ProjectID
		form.ParentID = target.ID
		if relation.Kind == "parent_issue" {
			form.ParentID = target.ParentID
		}
	}
	next := d.Clone()
	i, err := next.CreateFromForm(form)
	if err != nil {
		return Issue{}, err
	}
	switch relation.Kind {
	case "parent_issue":
		err = next.ReparentIssue(target.ID, i.ID)
	case "related":
		_, err = next.AddRelation(target.ID, i.ID, "related")
	case "blocked_issue":
		_, err = next.AddRelation(target.ID, i.ID, "blocks")
	case "blocking_issue":
		_, err = next.AddRelation(i.ID, target.ID, "blocks")
	}
	if err != nil {
		return Issue{}, err
	}
	if err = next.Validate(); err != nil {
		return Issue{}, err
	}
	*d = next
	return i, nil
}
