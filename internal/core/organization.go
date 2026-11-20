package core

import (
	"errors"
	"slices"
	"strings"
)

// SaveLabel is shared by desktop and MCP, including scope changes. Moving a
// label to one project removes incompatible assignments in the same transaction.
func (d *Data) SaveLabel(v Label) (Label, error) {
	v.Name, v.Group = strings.TrimSpace(v.Name), strings.TrimSpace(v.Group)
	if !validLabelKind(v.Kind) || v.EntityKind() == "project" && v.ProjectID != "" {
		return v, errors.New("invalid_label_scope")
	}
	if old, ok := d.Labels[v.ID]; ok && old.EntityKind() != v.EntityKind() {
		return v, errors.New("immutable_label_kind")
	}
	if !validName(v.Name) || v.Group != "" && !validName(v.Group) {
		return v, errors.New("invalid_label")
	}
	if v.ProjectID != "" && !d.ownerActive("project", v.ProjectID) {
		return v, errors.New("invalid_label_scope")
	}
	if v.Color == "" {
		v.Color = "#a799e8"
	}
	if len(v.Color) != 7 || v.Color[0] != '#' || strings.Trim(v.Color[1:], "0123456789abcdefABCDEF") != "" {
		return v, errors.New("invalid_color")
	}
	for _, old := range d.Labels {
		if old.ID != v.ID && old.EntityKind() == v.EntityKind() && old.ProjectID == v.ProjectID && strings.EqualFold(old.Group, v.Group) && strings.EqualFold(old.Name, v.Name) {
			return v, errors.New("duplicate_label")
		}
	}
	if v.ID == "" {
		v.ID = ID()
	} else if _, ok := d.Labels[v.ID]; !ok {
		return v, errors.New("label_not_found")
	}
	d.Labels[v.ID] = v
	d.cleanLabelAssignments()
	return v, nil
}

func MergeLabelEdits(base, edit, current Label) Label {
	if edit.Name != base.Name {
		current.Name = edit.Name
	}
	if edit.Color != base.Color {
		current.Color = edit.Color
	}
	if edit.Group != base.Group {
		current.Group = edit.Group
	}
	if edit.ProjectID != base.ProjectID {
		current.ProjectID = edit.ProjectID
	}
	return current
}

func (d *Data) cleanLabelAssignments() {
	for id, p := range d.Projects {
		p.LabelIDs = slices.DeleteFunc(slices.Clone(p.LabelIDs), func(id string) bool { l, ok := d.Labels[id]; return !ok || l.EntityKind() != "project" })
		d.Projects[id] = p
	}
	clean := func(i *Issue) {
		i.LabelIDs = slices.DeleteFunc(slices.Clone(i.LabelIDs), func(id string) bool {
			l, ok := d.Labels[id]
			return !ok || l.EntityKind() != "issue" || l.ProjectID != "" && l.ProjectID != i.ProjectID
		})
	}
	for id, v := range d.Issues {
		clean(&v)
		d.Issues[id] = v
	}
	for id, v := range d.Drafts {
		clean(&v.Form)
		clean(&v.Base)
		d.Drafts[id] = v
	}
	for id, v := range d.Templates {
		clean(&v.Form)
		d.Templates[id] = v
	}
	for id, v := range d.Recurrences {
		clean(&v.Form)
		d.Recurrences[id] = v
	}
}

func (d Data) AvailableLabels(project string) []Label {
	out := d.LabelList()
	out = slices.DeleteFunc(out, func(l Label) bool { return l.ProjectID != "" && l.ProjectID != project })
	slices.SortStableFunc(out, func(a, b Label) int {
		if c := strings.Compare(strings.ToLower(a.Group), strings.ToLower(b.Group)); c != 0 {
			return c
		}
		return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})
	return out
}

func (d *Data) SaveFolder(v FavoriteFolder) (FavoriteFolder, error) {
	v.Name = strings.TrimSpace(v.Name)
	if !validName(v.Name) {
		return v, errors.New("invalid_folder")
	}
	if v.ID == "" {
		v.ID = ID()
	} else if _, ok := d.Folders[v.ID]; !ok {
		return v, errors.New("not_found")
	}
	seen := map[FavoriteRef]bool{}
	for _, ref := range v.Items {
		_, view := d.Views[ref.ID]
		if seen[ref] || !(d.ownerExists(ref.Kind, ref.ID) || ref.Kind == "view" && view) {
			return v, errors.New("invalid_favorite")
		}
		seen[ref] = true
	}
	v.Items = slices.Clone(v.Items)
	d.Folders[v.ID] = v
	return v, nil
}

func (d Data) FolderList() []FavoriteFolder {
	out := []FavoriteFolder{}
	for _, v := range d.Folders {
		v.Items = slices.Clone(v.Items)
		out = append(out, v)
	}
	slices.SortFunc(out, func(a, b FavoriteFolder) int {
		if a.Position < b.Position {
			return -1
		}
		if a.Position > b.Position {
			return 1
		}
		return strings.Compare(a.ID, b.ID)
	})
	return out
}

// Saved views have no trash, so remove folder references atomically with them.
func (d *Data) RemoveSavedView(id string) {
	delete(d.Views, id)
	for key, v := range d.Folders {
		v.Items = slices.DeleteFunc(slices.Clone(v.Items), func(ref FavoriteRef) bool { return ref.Kind == "view" && ref.ID == id })
		d.Folders[key] = v
	}
}
