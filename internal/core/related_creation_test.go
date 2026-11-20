package core

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestRelatedCreationDirectionsHierarchyAndRollback(t *testing.T) {
	for _, kind := range []string{"related", "sub_issue", "parent_issue", "blocked_issue", "blocking_issue"} {
		t.Run(kind, func(t *testing.T) {
			d := Empty()
			root, _ := d.AddIssue("Root", "", "", "todo")
			target, _ := d.AddIssue("Target", "", root.ID, "todo")
			child, _ := d.AddIssue("Child", "", target.ID, "todo")
			created, err := d.CreateWithRelation(Issue{Title: "New", Status: "todo"}, &CreationRelation{IssueID: target.ID, Kind: kind})
			if err != nil {
				t.Fatal(err)
			}
			if kind == "sub_issue" && created.ParentID != target.ID {
				t.Fatal("sub issue has wrong parent")
			}
			if kind == "parent_issue" && (created.ParentID != root.ID || d.Issues[target.ID].ParentID != created.ID || d.Issues[child.ID].ParentID != target.ID) {
				t.Fatal("inserting parent changed existing descendants")
			}
			if kind == "related" && len(d.Relations) != 1 {
				t.Fatal("missing related link")
			}
			for _, r := range d.Relations {
				if kind == "blocked_issue" && (r.From != target.ID || r.To != created.ID || r.Kind != "blocks") {
					t.Fatal("blocked direction reversed")
				}
				if kind == "blocking_issue" && (r.From != created.ID || r.To != target.ID || r.Kind != "blocks") {
					t.Fatal("blocking direction reversed")
				}
			}
			if err := d.Validate(); err != nil {
				t.Fatal(err)
			}
			before := d.Clone()
			if _, err := d.CreateWithRelation(Issue{Title: "Invalid", Due: "bad-date"}, &CreationRelation{IssueID: target.ID, Kind: kind}); err == nil || !reflect.DeepEqual(before, d) {
				t.Fatal("invalid creation partially changed data")
			}
			d.TrashIssue(target.ID)
			before = d.Clone()
			if _, err := d.CreateWithRelation(Issue{Title: "Late"}, &CreationRelation{IssueID: target.ID, Kind: kind}); err == nil || !reflect.DeepEqual(before, d) {
				t.Fatal("deleted target accepted or partial commit")
			}
		})
	}
}

func TestRelatedDraftMigrationBackupDeletionAndSubmission(t *testing.T) {
	s, path := testStore(t)
	var target, draftID string
	legacy, err := s.Update(func(d *Data) error {
		i, _ := d.AddIssue("Source", "", "", "todo")
		target = i.ID
		_, e := d.SaveComment(Comment{IssueID: target, Body: "Preserve comments"})
		return e
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.Execute(context.Background(), "PRAGMA user_version=4"); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if !reflect.DeepEqual(legacy, s.Snapshot()) {
		t.Fatal("schema 4 data changed during migration")
	}
	oldBackup := filepath.Join(t.TempDir(), "v4.json")
	raw, _ := json.Marshal(Backup{Format: "flow-backup", Version: 4, Data: legacy})
	os.WriteFile(oldBackup, raw, 0600)
	if got, e := ReadBackup(oldBackup); e != nil || !reflect.DeepEqual(got, legacy) {
		t.Fatal("v4 backup unreadable", e)
	}
	_, err = s.Update(func(d *Data) error {
		v, e := d.SaveDraft(IssueDraft{Form: Issue{Title: "Retained related draft", Status: "todo"}, Relation: &CreationRelation{IssueID: target, Kind: "blocking_issue"}})
		draftID = v.ID
		return e
	})
	if err != nil {
		t.Fatal(err)
	}
	clone := s.Snapshot()
	v := clone.Drafts[draftID]
	v.Relation.Kind = "parent_issue"
	if s.Snapshot().Drafts[draftID].Relation.Kind != "blocking_issue" {
		t.Fatal("draft relation alias escaped store")
	}
	backup := filepath.Join(t.TempDir(), "v5.json")
	if err = s.Backup(backup); err != nil {
		t.Fatal(err)
	}
	data, err := ReadBackup(backup)
	if err != nil || data.Drafts[draftID].Relation.Kind != "blocking_issue" {
		t.Fatal("backup lost creation link", err)
	}
	s.Close()
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, err = s.Update(func(d *Data) error { d.TrashIssue(target); return nil })
	if err != nil {
		t.Fatal(err)
	}
	before := s.Snapshot()
	if _, err = s.Update(func(d *Data) error { _, e := d.ApplyDraft(draftID); return e }); err == nil || !reflect.DeepEqual(before, s.Snapshot()) {
		t.Fatal("failed submission removed draft or created orphan")
	}
	_, err = s.Update(func(d *Data) error { return d.RestoreIssue(target) })
	if err != nil {
		t.Fatal(err)
	}
	var created Issue
	_, err = s.Update(func(d *Data) error { var e error; created, e = d.ApplyDraft(draftID); return e })
	if err != nil {
		t.Fatal(err)
	}
	data = s.Snapshot()
	if _, ok := data.Drafts[draftID]; ok {
		t.Fatal("submitted draft still present")
	}
	for _, r := range data.Relations {
		if r.From != created.ID || r.To != target {
			t.Fatal("submitted relation changed direction")
		}
	}
	if len(data.Relations) != 1 {
		t.Fatal("submitted relation absent")
	}
}
