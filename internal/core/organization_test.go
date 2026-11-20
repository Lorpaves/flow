package core

import (
	"slices"
	"testing"
)

func TestLabelScopeChangesAndFolderReferencesAreAtomic(t *testing.T) {
	s, _ := testStore(t)
	var p1, p2, issue1, issue2, label, folder, view string
	data, e := s.Update(func(d *Data) error {
		d.Seed()
		projects := d.ProjectList(false, false)
		p1, p2 = projects[0].ID, projects[1].ID
		i, _ := d.AddIssue("one", p1, "", "todo")
		j, _ := d.AddIssue("two", p2, "", "todo")
		issue1, issue2 = i.ID, j.ID
		l, err := d.SaveLabel(Label{Name: "Shared", Group: "Workflow", Color: "#aabbcc"})
		if err != nil {
			return err
		}
		label = l.ID
		i.LabelIDs = []string{label}
		j.LabelIDs = []string{label}
		d.Issues[i.ID], d.Issues[j.ID] = i, j
		if _, err = d.SaveDraft(IssueDraft{Form: j, Base: j}); err != nil {
			return err
		}
		if _, err = d.SaveTemplate(IssueTemplate{Name: "Template", Form: j}); err != nil {
			return err
		}
		v := SavedView{ID: ID(), Name: "Pinned", Config: DefaultView()}
		d.Views[v.ID] = v
		view = v.ID
		f, err := d.SaveFolder(FavoriteFolder{Name: "Work", Items: []FavoriteRef{{Kind: "issue", ID: i.ID}, {Kind: "view", ID: view}}})
		folder = f.ID
		return err
	})
	if e != nil {
		t.Fatal(e)
	}
	data, e = s.Update(func(d *Data) error { l := d.Labels[label]; l.ProjectID = p1; _, e := d.SaveLabel(l); return e })
	if e != nil {
		t.Fatal(e)
	}
	if !slices.Contains(data.Issues[issue1].LabelIDs, label) || slices.Contains(data.Issues[issue2].LabelIDs, label) {
		t.Fatal("scope change left incompatible issue labels")
	}
	for _, v := range data.Drafts {
		if slices.Contains(v.Form.LabelIDs, label) || slices.Contains(v.Base.LabelIDs, label) {
			t.Fatal("draft scope not cleaned")
		}
	}
	for _, v := range data.Templates {
		if slices.Contains(v.Form.LabelIDs, label) {
			t.Fatal("template scope not cleaned")
		}
	}
	if _, e = s.Update(func(d *Data) error {
		_, e := d.SaveLabel(Label{Name: "shared", Group: "workflow", ProjectID: p1})
		return e
	}); e == nil {
		t.Fatal("duplicate label accepted")
	}
	if _, e = s.Update(func(d *Data) error {
		_, e := d.SaveFolder(FavoriteFolder{ID: folder, Name: "Bad", Items: []FavoriteRef{{Kind: "issue", ID: "missing"}}})
		return e
	}); e == nil {
		t.Fatal("invalid folder accepted")
	}
	if len(s.Snapshot().Folders[folder].Items) != 2 {
		t.Fatal("failed folder save was not atomic")
	}
	data, e = s.Update(func(d *Data) error { d.RemoveSavedView(view); return nil })
	if e != nil {
		t.Fatal(e)
	}
	if len(data.Folders[folder].Items) != 1 {
		t.Fatal("view deletion left dangling favorite")
	}
	data, e = s.Update(func(d *Data) error { d.RemoveLabel(label); return nil })
	if e != nil {
		t.Fatal(e)
	}
	if len(data.Issues[issue1].LabelIDs) != 0 {
		t.Fatal("label deletion left assignment")
	}
}
