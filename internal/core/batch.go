package core

import (
	"errors"
	"slices"
)

// MoveIssueProjects changes a selection within the Store.Update transaction.
// Children move with a selected parent, or into their existing parent's project.
func (d *Data) MoveIssueProjects(ids []string, project string) error {
	if project != "" {
		p, ok := d.Projects[project]
		if !ok || p.DeletedAt != 0 {
			return errors.New("invalid_project_reference")
		}
	}
	selected := map[string]bool{}
	for _, id := range ids {
		selected[id] = true
	}
	for _, id := range ids {
		i, ok := d.Issues[id]
		if !ok || i.DeletedAt != 0 {
			return errors.New("issue_not_found")
		}
		ancestorSelected := false
		for ancestor := i.ParentID; ancestor != ""; ancestor = d.Issues[ancestor].ParentID {
			if selected[ancestor] {
				ancestorSelected = true
				break
			}
		}
		if i.ParentID != "" && !ancestorSelected && d.Issues[i.ParentID].ProjectID != project {
			return errors.New("move_parent_with_child")
		}
	}
	ordered := slices.Clone(ids)
	depth := func(id string) int {
		n := 0
		for id = d.Issues[id].ParentID; id != ""; id = d.Issues[id].ParentID {
			n++
		}
		return n
	}
	slices.SortFunc(ordered, func(a, b string) int { return depth(a) - depth(b) })
	for _, id := range ordered {
		i := d.Issues[id]
		i.ProjectID = project
		if e := d.PutIssue(i); e != nil {
			return e
		}
	}
	return nil
}
