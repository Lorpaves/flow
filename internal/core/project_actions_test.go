package core

import (
	"path/filepath"
	"reflect"
	"slices"
	"testing"
	"time"
)

func TestProjectLabelsRemindersRoundTripAndIsolation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "flow.db")
	s, e := Open(path)
	if e != nil {
		t.Fatal(e)
	}
	var project, label, reminder, issueLabel, issue string
	now := time.Date(2026, 11, 8, 10, 0, 0, 0, time.UTC)
	_, e = s.Update(func(d *Data) error {
		d.Seed()
		p := d.ProjectList(false, false)[0]
		project = p.ID
		l, e := d.SaveLabel(Label{Kind: "project", Name: "Launch", Group: "Phase"})
		if e != nil {
			return e
		}
		label = l.ID
		l, e = d.SaveLabel(Label{Name: "Launch", Group: "Phase"})
		if e != nil {
			return e
		}
		issueLabel = l.ID
		p.LabelIDs = []string{label}
		if e = d.PutProject(p); e != nil {
			return e
		}
		r, e := d.SetProjectReminder(project, now.Add(-time.Hour))
		reminder = r.ID
		i, _ := d.AddIssue("Task", project, "", "todo")
		issue = i.ID
		return e
	})
	if e != nil {
		t.Fatal(e)
	}
	data := s.Snapshot()
	if len(data.LabelsForKind("project")) != 1 || slices.ContainsFunc(data.AvailableLabels(project), func(l Label) bool { return l.ID == label }) {
		t.Fatal("project labels leaked into issue menu")
	}
	copy := data.Clone()
	p := copy.Projects[project]
	p.LabelIDs[0] = "mutated"
	if data.Projects[project].LabelIDs[0] != label {
		t.Fatal("shallow project labels")
	}
	for _, change := range []func(*Data){func(d *Data) { p := d.Projects[project]; p.LabelIDs = []string{issueLabel}; d.Projects[project] = p }, func(d *Data) { i := d.Issues[issue]; i.LabelIDs = []string{label}; d.Issues[issue] = i }, func(d *Data) { r := d.Reminders[reminder]; r.IssueID = issue; d.Reminders[reminder] = r }} {
		if _, e = s.Update(func(d *Data) error { change(d); return nil }); e == nil {
			t.Fatal("invalid owner/type accepted")
		}
	}
	if _, e = s.Update(func(d *Data) error { l := d.Labels[label]; l.Kind = "issue"; _, e := d.SaveLabel(l); return e }); e == nil {
		t.Fatal("label kind changed")
	}
	_, e = s.Update(func(d *Data) error {
		d.TrashProject(project)
		if _, f := d.RunScheduled(now); len(f) != 0 {
			t.Fatal("deleted project notified")
		}
		d.RestoreProject(project)
		if _, f := d.RunScheduled(now); !slices.Contains(f, reminder) {
			t.Fatal("restored project missed reminder")
		}
		if _, f := d.RunScheduled(now); len(f) != 0 {
			t.Fatal("duplicate notification")
		}
		r, e := d.SnoozeReminder(reminder, now.Add(time.Hour))
		if r.ProjectID != project || r.FiredAt != 0 {
			t.Fatal("snooze changed owner or fired state")
		}
		return e
	})
	if e != nil {
		t.Fatal(e)
	}
	backup := filepath.Join(t.TempDir(), "backup.json")
	if e = s.Backup(backup); e != nil {
		t.Fatal(e)
	}
	want := s.Snapshot()
	got, e := ReadBackup(backup)
	if e != nil || !reflect.DeepEqual(got, want) {
		t.Fatal("backup mismatch", e)
	}
	s.Close()
	s, e = Open(path)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	if !reflect.DeepEqual(s.Snapshot(), want) {
		t.Fatal("restart lost project fields")
	}
	d, e := s.Update(func(d *Data) error { d.RemoveLabel(label); return nil })
	if e != nil || len(d.Projects[project].LabelIDs) != 0 {
		t.Fatal("label delete left dangling assignment", e)
	}
}
