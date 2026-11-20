package core

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
)

func TestStoreProcessWriter(t *testing.T) {
	path := os.Getenv("FLOW_TEST_WRITER_DB")
	if path == "" {
		t.Skip("subprocess helper")
	}
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for n := 0; n < 30; n++ {
		if _, err = s.Update(func(d *Data) error { _, e := d.AddIssue("MCP process", "", "", "todo"); return e }); err != nil {
			t.Fatal(err)
		}
	}
}
func TestCrossProcessWritesKeepAllIssuesAndUniqueNumbers(t *testing.T) {
	s, path := testStore(t)
	cmd := exec.Command(os.Args[0], "-test.run=^TestStoreProcessWriter$")
	cmd.Env = append(os.Environ(), "FLOW_TEST_WRITER_DB="+path)
	if e := cmd.Start(); e != nil {
		t.Fatal(e)
	}
	for n := 0; n < 30; n++ {
		if _, e := s.Update(func(d *Data) error { _, e := d.AddIssue("Desktop process", "", "", "todo"); return e }); e != nil {
			t.Fatal(e)
		}
	}
	if e := cmd.Wait(); e != nil {
		t.Fatal(e)
	}
	d, _, e := s.Refresh()
	if e != nil {
		t.Fatal(e)
	}
	if len(d.Issues) != 60 || d.Preferences.NextNumber != 61 {
		t.Fatalf("lost concurrent writes: %d issues, next %d", len(d.Issues), d.Preferences.NextNumber)
	}
	if e = d.Validate(); e != nil {
		t.Fatal(e)
	}
}

func testStore(t *testing.T) (*Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "flow.db")
	s, e := Open(path)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { s.Close() })
	return s, path
}
func TestPersistenceRelationshipsAndRestore(t *testing.T) {
	s, path := testStore(t)
	var parent, child, project string
	d, e := s.Update(func(d *Data) error {
		d.Seed()
		for id := range d.Projects {
			project = id
			break
		}
		p, _ := d.AddIssue("父任务", project, "", "todo")
		parent = p.ID
		c, _ := d.AddIssue("中文子任务", project, parent, "backlog")
		child = c.ID
		return nil
	})
	if e != nil {
		t.Fatal(e)
	}
	backup := filepath.Join(t.TempDir(), "workspace.flow.json")
	if e = s.Backup(backup); e != nil {
		t.Fatal(e)
	}
	_, e = s.Update(func(d *Data) error { p := d.Issues[parent]; p.Status = "done"; return d.PutIssue(p) })
	if e != nil {
		t.Fatal(e)
	}
	if s.Snapshot().Issues[child].Status != "backlog" {
		t.Fatal("parent completion changed child")
	}
	_, e = s.Update(func(d *Data) error { d.TrashProject(project); return nil })
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Update(func(d *Data) error { return d.RestoreIssue(child) }); e == nil {
		t.Fatal("restored child into deleted project")
	}
	if _, e = s.Update(func(d *Data) error { d.RestoreProject(project); return nil }); e != nil {
		t.Fatal(e)
	}
	if s.Snapshot().Issues[child].DeletedAt != 0 {
		t.Fatal("project restoration did not restore descendants")
	}
	restored, e := s.Restore(backup)
	if e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(d, restored) {
		t.Fatal("backup round-trip changed data")
	}
	files, _ := filepath.Glob(filepath.Join(filepath.Dir(path), "backups", "*.flow.json"))
	if len(files) != 1 {
		t.Fatalf("expected automatic backup, got %v", files)
	}
	if e = s.Close(); e != nil {
		t.Fatal(e)
	}
	reopened, e := Open(path)
	if e != nil {
		t.Fatal(e)
	}
	defer reopened.Close()
	if !reflect.DeepEqual(d, reopened.Snapshot()) {
		t.Fatal("relaunch changed data")
	}
}
func TestFailedTransactionLeavesMemoryAndDiskUnchanged(t *testing.T) {
	s, path := testStore(t)
	_, e := s.db.Execute(context.Background(), `CREATE TRIGGER reject_issues BEFORE INSERT ON issues BEGIN SELECT RAISE(ABORT, 'injected storage failure'); END`)
	if e != nil {
		t.Fatal(e)
	}
	before := s.Snapshot()
	_, e = s.Update(func(d *Data) error {
		p := Project{ID: "new-project", Name: "Atomic", Status: "active"}
		d.Projects[p.ID] = p
		_, err := d.AddIssue("Rejected", p.ID, "", "todo")
		return err
	})
	if e == nil {
		t.Fatal("expected injected SQLite failure")
	}
	if !reflect.DeepEqual(before, s.Snapshot()) {
		t.Fatal("failed transaction changed in-memory state")
	}
	s.Close()
	again, e := Open(path)
	if e != nil {
		t.Fatal(e)
	}
	defer again.Close()
	if len(again.Snapshot().Projects) != 0 {
		t.Fatal("partial project insert escaped transaction rollback")
	}
}
func TestInvalidBackupPreservesWorkspace(t *testing.T) {
	s, _ := testStore(t)
	before, e := s.Update(func(d *Data) error { d.Seed(); return nil })
	if e != nil {
		t.Fatal(e)
	}
	p := filepath.Join(t.TempDir(), "bad.json")
	os.WriteFile(p, []byte(`{"format":"flow-backup","version":99}`), 0600)
	if _, e = s.Restore(p); e == nil {
		t.Fatal("accepted future backup version")
	}
	if !reflect.DeepEqual(before, s.Snapshot()) {
		t.Fatal("invalid restore changed data")
	}
}
func TestRelationshipValidationAndReorder(t *testing.T) {
	d := Empty()
	d.Projects["p"] = Project{ID: "p", Name: "Project", Status: "active"}
	a, _ := d.AddIssue("first", "p", "", "todo")
	b, _ := d.AddIssue("second", "p", "", "todo")
	child, _ := d.AddIssue("child", "p", a.ID, "todo")
	if _, e := d.AddIssue("grandchild", "p", child.ID, "todo"); e != nil {
		t.Fatalf("nested sub-issue: %v", e)
	}
	if e := d.Move(b.ID, "todo", a.ID); e != nil {
		t.Fatal(e)
	}
	q := d.Query(Filter{Status: "todo"})
	if q[0].ID != b.ID {
		t.Fatal("move-before order not saved")
	}
	d.TrashIssue(child.ID)
	prior := d.Issues[child.ID].DeletedAt
	d.TrashIssue(a.ID)
	if e := d.RestoreIssue(a.ID); e != nil {
		t.Fatal(e)
	}
	if d.Issues[child.ID].DeletedAt != prior {
		t.Fatal("restored independently deleted child")
	}
	x := d.Issues[b.ID]
	x.ParentID = b.ID
	d.Issues[b.ID] = x
	if e := d.Validate(); e == nil {
		t.Fatal("accepted self-parent")
	}
}
func TestSearchAndFilterTenThousand(t *testing.T) {
	d := Empty()
	for n := 0; n < 10000; n++ {
		i, _ := d.AddIssue("Task", "", "", "todo")
		if n == 9910 {
			i.Title = "中文检索目标"
			i.Plain = "Offline search context"
			i.Priority = 2
		}
		d.Issues[i.ID] = i
	}
	items := d.Query(Filter{Query: "中文检索", HasPriority: true, Priority: 2})
	if len(items) != 1 {
		t.Fatalf("got %d search results", len(items))
	}
	if len(d.Query(Filter{Query: "OFFLINE SEARCH"})) != 1 {
		t.Fatal("case-insensitive body search failed")
	}
	if len(d.Query(Filter{Query: items[0].Code()})) != 1 {
		t.Fatal("identifier search failed")
	}
}
func TestValidationFailureDoesNotConsumeIssueNumber(t *testing.T) {
	s, _ := testStore(t)
	_, e := s.Update(func(d *Data) error { d.AddIssue("valid", "", "", "todo"); return errors.New("cancelled") })
	if e == nil {
		t.Fatal("missing error")
	}
	if s.Snapshot().Preferences.NextNumber != 1 {
		t.Fatal("cancelled update consumed number")
	}
}
