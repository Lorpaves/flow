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

func TestCommentThreadsMigrationBackupAndConcurrentDeletion(t *testing.T) {
	s, path := testStore(t)
	var issue, other, root, reply string
	legacy, e := s.Update(func(d *Data) error {
		i, _ := d.AddIssue("Discussion", "", "", "todo")
		issue = i.ID
		j, _ := d.AddIssue("Other issue", "", "", "todo")
		other = j.ID
		v, e := d.SaveComment(Comment{IssueID: issue, Body: "Original"})
		root = v.ID
		return e
	})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.db.Execute(context.Background(), "PRAGMA user_version=3"); e != nil {
		t.Fatal(e)
	}
	s.Close()
	s, e = Open(path)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	if !reflect.DeepEqual(legacy, s.Snapshot()) {
		t.Fatal("schema 3 migration changed flat comments")
	}
	for _, version := range []int{1, 2, 3} {
		p := filepath.Join(t.TempDir(), "legacy.json")
		raw, _ := json.Marshal(Backup{Format: "flow-backup", Version: version, Data: legacy})
		os.WriteFile(p, raw, 0600)
		got, e := ReadBackup(p)
		if e != nil || !reflect.DeepEqual(got, legacy) {
			t.Fatalf("v%d backup: %v", version, e)
		}
	}
	_, e = s.Update(func(d *Data) error {
		v, e := d.SaveComment(Comment{IssueID: issue, ParentID: root, Body: "Reply"})
		reply = v.ID
		if e != nil {
			return e
		}
		_, e = d.SaveComment(Comment{IssueID: issue, ParentID: reply, Body: "Nested"})
		return e
	})
	if e != nil {
		t.Fatal(e)
	}
	before := s.Snapshot()
	if _, e = s.Update(func(d *Data) error {
		_, e := d.SaveComment(Comment{IssueID: other, ParentID: root, Body: "Wrong issue"})
		return e
	}); e == nil {
		t.Fatal("cross-issue reply accepted")
	}
	if _, e = s.Update(func(d *Data) error { v := d.Comments[root]; v.ParentID = reply; d.Comments[root] = v; return nil }); e == nil {
		t.Fatal("comment cycle accepted")
	}
	if !reflect.DeepEqual(before, s.Snapshot()) {
		t.Fatal("failed reply changed stored data")
	}
	_, e = s.Update(func(d *Data) error {
		v := d.Comments[root]
		v.DeletedAt = time.Now().UnixMilli()
		d.Comments[root] = v
		return nil
	})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Update(func(d *Data) error {
		_, e := d.SaveComment(Comment{IssueID: issue, ParentID: root, Body: "Late reply"})
		return e
	}); e == nil {
		t.Fatal("reply to deleted parent accepted")
	}
	_, e = s.Update(func(d *Data) error {
		_, e := d.SaveComment(Comment{ID: reply, IssueID: issue, Body: "Edited reply"})
		return e
	})
	if e != nil {
		t.Fatal(e)
	}
	thread := s.Snapshot().CommentThread(issue)
	if len(thread) != 3 || thread[0].Comment.ID != root || thread[0].Comment.DeletedAt == 0 || thread[1].Comment.ParentID != root || thread[2].Depth != 2 {
		t.Fatalf("lost thread ancestry: %+v", thread)
	}
	backup := filepath.Join(t.TempDir(), "threads.json")
	if e = s.Backup(backup); e != nil {
		t.Fatal(e)
	}
	raw, _ := os.ReadFile(backup)
	var b Backup
	json.Unmarshal(raw, &b)
	if b.Version != schemaVersion {
		t.Fatalf("backup version=%d", b.Version)
	}
	got, e := ReadBackup(backup)
	if e != nil || !reflect.DeepEqual(got, s.Snapshot()) {
		t.Fatal("thread backup mismatch", e)
	}
	s.Close()
	s, e = Open(path)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	if s.Snapshot().Comments[reply].ParentID != root {
		t.Fatal("restart lost reply parent")
	}
	version, e := s.db.Query(context.Background(), "PRAGMA user_version")
	if e != nil || version.Values[0][0].(int64) != schemaVersion {
		t.Fatal("schema upgrade missing", e)
	}
}
