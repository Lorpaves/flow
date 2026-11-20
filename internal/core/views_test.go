package core

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestSavedViewFiltersPersistenceAndCloneIsolation(t *testing.T) {
	s, path := testStore(t)
	var wanted string
	config := DefaultView()
	config.OrderBy = "priority"
	config.Filter = FilterGroup{Rules: []FilterRule{{Field: "due", Op: "before", Values: []string{"2027-01-01"}}}, Groups: []FilterGroup{{Any: true, Rules: []FilterRule{{Field: "status", Op: "is", Values: []string{"progress"}}, {Field: "title", Op: "contains", Values: []string{"中文"}}}}}}
	d, e := s.Update(func(d *Data) error {
		i, _ := d.AddIssue("中文筛选", "", "", "todo")
		i.Due = "2026-12-10"
		i.Priority = 2
		wanted = i.ID
		d.Issues[i.ID] = i
		i, _ = d.AddIssue("exclude by date", "", "", "progress")
		i.Due = "2027-02-01"
		d.Issues[i.ID] = i
		i, _ = d.AddIssue("exclude by group", "", "", "backlog")
		i.Due = "2026-11-01"
		d.Issues[i.ID] = i
		d.Views["v"] = SavedView{ID: "v", Name: "我的视图", Config: config}
		d.Preferences.ViewStates = map[string]ViewConfig{"issues/": config}
		return nil
	})
	if e != nil {
		t.Fatal(e)
	}
	got := d.QueryView(Filter{}, config, time.Now())
	if len(got) != 1 || got[0].ID != wanted {
		t.Fatalf("wrong matches: %+v", got)
	}
	copy := d.Clone()
	copy.Views["v"].Config.Filter.Groups[0].Rules[0].Values[0] = "done"
	copy.Preferences.ViewStates["issues/"].Properties[0] = "changed"
	if d.Views["v"].Config.Filter.Groups[0].Rules[0].Values[0] != "progress" || d.Preferences.ViewStates["issues/"].Properties[0] != "id" {
		t.Fatal("nested data alias escaped clone")
	}
	backup := filepath.Join(t.TempDir(), "view.flow.json")
	if e = s.Backup(backup); e != nil {
		t.Fatal(e)
	}
	s.Close()
	again, e := Open(path)
	if e != nil {
		t.Fatal(e)
	}
	defer again.Close()
	if !reflect.DeepEqual(d, again.Snapshot()) {
		t.Fatal("view changed across restart")
	}
	if _, e = again.Update(func(d *Data) error { delete(d.Views, "v"); return nil }); e != nil {
		t.Fatal(e)
	}
	restored, e := again.Restore(backup)
	if e != nil || !reflect.DeepEqual(d, restored) {
		t.Fatalf("backup lost view: %v", e)
	}
}
func TestUpgradeVersionOneDatabaseAndBackup(t *testing.T) {
	s, path := testStore(t)
	before, e := s.Update(func(d *Data) error { d.Seed(); return nil })
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.db.Execute(context.Background(), "DROP TABLE views"); e != nil {
		t.Fatal(e)
	}
	if _, e = s.db.Execute(context.Background(), "PRAGMA user_version=1"); e != nil {
		t.Fatal(e)
	}
	s.Close()
	again, e := Open(path)
	if e != nil {
		t.Fatal(e)
	}
	defer again.Close()
	if !reflect.DeepEqual(before, again.Snapshot()) {
		t.Fatal("migration changed old data")
	}
	legacy := filepath.Join(t.TempDir(), "legacy.json")
	blob, _ := json.Marshal(Backup{Format: "flow-backup", Version: 1, Data: before})
	if e = os.WriteFile(legacy, blob, 0600); e != nil {
		t.Fatal(e)
	}
	restored, e := again.Restore(legacy)
	if e != nil || !reflect.DeepEqual(before, restored) {
		t.Fatalf("legacy backup: %v", e)
	}
}
func TestRelativeDatesAndInvalidInput(t *testing.T) {
	now := time.Date(2026, 12, 30, 12, 0, 0, 0, time.UTC)
	for input, want := range map[string]string{"": "", "tomorrow": "2026-12-31", "7 days": "2027-01-06", "24h": "2026-12-31", "下周": "2027-01-06", "明天": "2026-12-31", "Jan 2": "2027-01-02", "2026-12-31": "2026-12-31"} {
		got, e := ParseDate(input, now)
		if e != nil || got != want {
			t.Fatalf("%q: %q %v, want %s", input, got, e, want)
		}
	}
	for _, input := range []string{"yesterday maybe", "2026-02-30", "999999999999999999 weeks"} {
		if _, e := ParseDate(input, now); e == nil {
			t.Fatalf("accepted %q", input)
		}
	}
}
