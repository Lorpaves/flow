package core

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestRichRecordsSurviveRestartBackupAndClone(t *testing.T) {
	s, path := testStore(t)
	var issue, attachment, draft, template, recurrence string
	before, e := s.Update(func(d *Data) error {
		d.Projects["p"] = Project{ID: "p", Name: "项目", Status: "active", Start: "2026-11-01", Due: "2026-12-01"}
		i, _ := d.AddIssue("可恢复内容", "p", "", "todo")
		issue = i.ID
		i.Body = json.RawMessage(`{"text":"中文正文"}`)
		i.Plain = "中文正文"
		d.Issues[i.ID] = i
		v, e := d.SaveDraft(IssueDraft{Form: i, Base: i, IssueID: i.ID})
		if e != nil {
			return e
		}
		draft = v.ID
		tpl, e := d.SaveTemplate(IssueTemplate{Name: "模板", Form: i})
		if e != nil {
			return e
		}
		template = tpl.ID
		file, e := d.AddAttachment(Attachment{OwnerKind: "issue", OwnerID: i.ID, Name: "test.bin", MIME: "application/octet-stream", Data: []byte{0, 1, 2, 255}})
		if e != nil {
			return e
		}
		attachment = file.ID
		if _, e = d.AddAttachment(Attachment{OwnerKind: "project", OwnerID: "p", Name: "Reference", URL: "https://example.com/resource"}); e != nil {
			return e
		}
		if _, e = d.SaveComment(Comment{IssueID: i.ID, Body: "记录修改原因"}); e != nil {
			return e
		}
		if _, e = d.SaveDocument(Document{OwnerKind: "project", OwnerID: "p", Name: "设计说明", Body: "# 中文\n\n- [ ] 验收"}); e != nil {
			return e
		}
		m, e := d.SaveMilestone(Milestone{ProjectID: "p", Name: "第一版", Due: "2026-11-30"})
		if e != nil {
			return e
		}
		i.MilestoneID = m.ID
		d.Issues[i.ID] = i
		if _, e = d.SaveProjectUpdate(ProjectUpdate{ProjectID: "p", Body: "本周完成共享模型", Health: "on_track"}); e != nil {
			return e
		}
		if _, e = d.SetReminder(i.ID, time.Now().Add(time.Hour)); e != nil {
			return e
		}
		r, e := d.SaveRecurrence(Recurrence{Form: i, Every: 1, Unit: "week", NextDue: "2026-11-15"})
		if e != nil {
			return e
		}
		recurrence = r.ID
		d.Folders["f"] = FavoriteFolder{ID: "f", Name: "工作", Items: []FavoriteRef{{Kind: "issue", ID: i.ID}}}
		return nil
	})
	if e != nil {
		t.Fatal(e)
	}
	copy := before.Clone()
	copy.Attachments[attachment].Data[0] = 99
	copy.Drafts[draft].Form.Body[0] = '!'
	copy.Templates[template].Form.Body[0] = '!'
	copy.Recurrences[recurrence].Form.Body[0] = '!'
	copy.Folders["f"].Items[0].ID = "changed"
	if before.Attachments[attachment].Data[0] != 0 || before.Drafts[draft].Form.Body[0] != '{' || before.Templates[template].Form.Body[0] != '{' || before.Recurrences[recurrence].Form.Body[0] != '{' || before.Folders["f"].Items[0].ID != issue {
		t.Fatal("mutable snapshot aliases stored records")
	}
	for _, a := range before.Activities {
		if a.Kind == "attachment" && strings.Contains(string(a.After), `"data"`) {
			t.Fatal("history duplicated attachment bytes")
		}
	}
	backup := filepath.Join(t.TempDir(), "complete.flow.json")
	if e = s.Backup(backup); e != nil {
		t.Fatal(e)
	}
	s.Close()
	reopened, e := Open(path)
	if e != nil {
		t.Fatal(e)
	}
	defer reopened.Close()
	if !reflect.DeepEqual(before, reopened.Snapshot()) {
		t.Fatal("restart changed records")
	}
	if _, e = reopened.Update(func(d *Data) error { delete(d.Drafts, draft); delete(d.Attachments, attachment); return nil }); e != nil {
		t.Fatal(e)
	}
	restored, e := reopened.Restore(backup)
	if e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(before, restored) {
		t.Fatal("backup did not restore records and history exactly")
	}
	bad := restored.Clone()
	file := bad.Attachments[attachment]
	file.Data[0] ^= 1
	bad.Attachments[attachment] = file
	if e = bad.Validate(); e == nil {
		t.Fatal("accepted corrupt attachment bytes")
	}
}

func TestHistoryUndoPreservesOtherFieldsAndRejectsConflicts(t *testing.T) {
	s, _ := testStore(t)
	var id, event string
	if _, e := s.Update(func(d *Data) error { i, e := d.AddIssue("first", "", "", "todo"); id = i.ID; return e }); e != nil {
		t.Fatal(e)
	}
	data, e := s.UpdateFrom("mcp", func(d *Data) error { i := d.Issues[id]; i.Title = "second"; return d.PutIssue(i) })
	if e != nil {
		t.Fatal(e)
	}
	for _, v := range data.History("issue", id) {
		if v.Action == "updated" {
			event = v.ID
			if v.Source != "mcp" {
				t.Fatal("lost activity source")
			}
			break
		}
	}
	if event == "" {
		t.Fatal("no property history")
	}
	if _, e = s.Update(func(d *Data) error { i := d.Issues[id]; i.Priority = 1; return d.PutIssue(i) }); e != nil {
		t.Fatal(e)
	}
	data, e = s.Update(func(d *Data) error { return d.UndoActivity(event) })
	if e != nil {
		t.Fatal(e)
	}
	if data.Issues[id].Title != "first" || data.Issues[id].Priority != 1 {
		t.Fatal("undo overwrote concurrent property")
	}
	if _, e = s.Update(func(d *Data) error { return d.UndoActivity(event) }); e == nil {
		t.Fatal("repeated undo accepted")
	}
	data, e = s.Update(func(d *Data) error { i := d.Issues[id]; i.Title = "third"; return d.PutIssue(i) })
	if e != nil {
		t.Fatal(e)
	}
	for _, v := range data.History("issue", id) {
		if v.Kind == "issue" && strings.Contains(string(v.After), `"title":"third"`) {
			event = v.ID
			break
		}
	}
	if _, e = s.Update(func(d *Data) error { i := d.Issues[id]; i.Title = "fourth"; return d.PutIssue(i) }); e != nil {
		t.Fatal(e)
	}
	before := s.Snapshot()
	if _, e = s.Update(func(d *Data) error { return d.UndoActivity(event) }); e == nil || e.Error() != "history_conflict" {
		t.Fatalf("expected conflict, got %v", e)
	}
	if !reflect.DeepEqual(before, s.Snapshot()) {
		t.Fatal("conflicting undo changed data")
	}
}

func TestNestedHierarchyMoveTrashRestoreAndRelations(t *testing.T) {
	d := Empty()
	d.Projects["p"] = Project{ID: "p", Name: "Original", Status: "active"}
	d.Projects["q"] = Project{ID: "q", Name: "Destination", Status: "active"}
	root, _ := d.AddIssue("root", "p", "", "todo")
	child, _ := d.AddIssue("child", "p", root.ID, "todo")
	grand, _ := d.AddIssue("grand", "p", child.ID, "todo")
	leaf, _ := d.AddIssue("leaf", "p", grand.ID, "todo")
	d.Labels["scoped"] = Label{ID: "scoped", Name: "Project label", ProjectID: "p"}
	grand.LabelIDs = []string{"scoped"}
	d.Issues[grand.ID] = grand
	if e := d.ReparentIssue(root.ID, leaf.ID); e == nil {
		t.Fatal("accepted parent cycle")
	}
	if e := d.MoveIssueProjects([]string{leaf.ID, root.ID}, "q"); e != nil {
		t.Fatal(e)
	}
	for _, id := range []string{root.ID, child.ID, grand.ID, leaf.ID} {
		if d.Issues[id].ProjectID != "q" {
			t.Fatal("descendant left in original project")
		}
	}
	if len(d.Issues[grand.ID].LabelIDs) != 0 {
		t.Fatal("project label crossed its scope")
	}
	d.TrashIssue(grand.ID)
	stamp := d.Issues[grand.ID].DeletedAt
	d.TrashIssue(root.ID)
	if e := d.RestoreIssue(root.ID); e != nil {
		t.Fatal(e)
	}
	if d.Issues[child.ID].DeletedAt != 0 || d.Issues[grand.ID].DeletedAt != stamp || d.Issues[leaf.ID].DeletedAt != stamp {
		t.Fatal("restore erased independent descendant deletion")
	}
	if e := d.RestoreIssue(grand.ID); e != nil {
		t.Fatal(e)
	}
	if d.Issues[leaf.ID].DeletedAt != 0 {
		t.Fatal("deep restore incomplete")
	}
	if _, e := d.AddRelation(root.ID, child.ID, "blocks"); e != nil {
		t.Fatal(e)
	}
	if _, e := d.AddRelation(child.ID, root.ID, "blocks"); e == nil {
		t.Fatal("accepted dependency cycle")
	}
	r, e := d.AddRelation(grand.ID, leaf.ID, "related")
	if e != nil {
		t.Fatal(e)
	}
	reverse, e := d.AddRelation(leaf.ID, grand.ID, "related")
	if e != nil || r.ID != reverse.ID {
		t.Fatal("related direction duplicated")
	}
	if _, e := d.AddRelation(leaf.ID, root.ID, "duplicate"); e != nil {
		t.Fatal(e)
	}
	if d.Issues[leaf.ID].Status != "duplicate" {
		t.Fatal("duplicate relationship did not mark issue")
	}
	if e := d.Validate(); e != nil {
		t.Fatal(e)
	}
}

func TestSchedulerCalendarCatchupAndConcurrentStores(t *testing.T) {
	s, path := testStore(t)
	second, e := Open(path)
	if e != nil {
		t.Fatal(e)
	}
	defer second.Close()
	var schedule, reminder, issue string
	if _, e = s.Update(func(d *Data) error {
		i, _ := d.AddIssue("monthly", "", "", "todo")
		issue = i.ID
		v, e := d.SaveRecurrence(Recurrence{Form: i, Every: 1, Unit: "month", NextDue: "2027-01-31"})
		if e != nil {
			return e
		}
		schedule = v.ID
		r, e := d.SetReminder(i.ID, time.Date(2027, 1, 31, 9, 0, 0, 0, time.UTC))
		reminder = r.ID
		return e
	}); e != nil {
		t.Fatal(e)
	}
	now := time.Date(2027, 2, 28, 12, 0, 0, 0, time.UTC)
	data, e := s.Update(func(d *Data) error {
		created, fired := d.RunScheduled(now)
		if len(created) != 1 || len(fired) != 1 {
			return errors.New("catchup count")
		}
		return nil
	})
	if e != nil {
		t.Fatal(e)
	}
	if data.Recurrences[schedule].NextDue != "2027-03-31" || data.Issues[data.Recurrences[schedule].LastIssueID].Due != "2027-02-28" || data.Reminders[reminder].FiredAt == 0 {
		t.Fatal("month-end schedule or reminder incorrect")
	}
	data, e = second.Update(func(d *Data) error {
		created, fired := d.RunScheduled(now)
		if len(created)+len(fired) != 0 {
			return errors.New("double schedule execution")
		}
		return nil
	})
	if e != nil {
		t.Fatal(e)
	}
	if len(data.Issues) != 2 || data.Issues[issue].Due != "" {
		t.Fatal("scheduler changed original or duplicated issue")
	}
	loc, e := time.LoadLocation("Europe/London")
	if e != nil {
		t.Fatal(e)
	}
	base := time.Date(2027, 3, 27, 0, 0, 0, 0, loc)
	if got := recurrenceDate(Recurrence{Every: 1, Unit: "day"}, base, 2); got.Day() != 29 || got.Hour() != 0 {
		t.Fatal("DST shifted calendar day")
	}
	leap := time.Date(2028, 2, 29, 0, 0, 0, 0, time.UTC)
	r := Recurrence{Every: 1, Unit: "year", AnchorDay: 29}
	if got := recurrenceDate(r, leap, 1).Format("2006-01-02"); got != "2029-02-28" {
		t.Fatal(got)
	}
}

func TestRecordsMigrationAndAtomicFailure(t *testing.T) {
	s, path := testStore(t)
	before, e := s.Update(func(d *Data) error { _, e := d.AddIssue("legacy", "", "", "todo"); return e })
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.db.Execute(context.Background(), "DROP TABLE records"); e != nil {
		t.Fatal(e)
	}
	if _, e = s.db.Execute(context.Background(), "PRAGMA user_version=2"); e != nil {
		t.Fatal(e)
	}
	s.Close()
	before.Activities = map[string]Activity{}
	s, e = Open(path)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	if !reflect.DeepEqual(before, s.Snapshot()) {
		t.Fatal("schema 2 migration changed user data")
	}
	for _, version := range []int{1, 2} {
		raw, _ := json.Marshal(Backup{Format: "flow-backup", Version: version, Data: before})
		p := filepath.Join(t.TempDir(), "legacy.json")
		if e = os.WriteFile(p, raw, 0600); e != nil {
			t.Fatal(e)
		}
		got, e := ReadBackup(p)
		if e != nil || !reflect.DeepEqual(before, got) {
			t.Fatalf("backup v%d: %v", version, e)
		}
	}
	if _, e = s.db.Execute(context.Background(), `CREATE TRIGGER fail_records BEFORE INSERT ON records BEGIN SELECT RAISE(ABORT, 'record write failed'); END`); e != nil {
		t.Fatal(e)
	}
	_, e = s.Update(func(d *Data) error {
		for id, i := range d.Issues {
			i.Title = "must roll back"
			d.Issues[id] = i
			_, e := d.SaveComment(Comment{IssueID: id, Body: "must roll back too"})
			return e
		}
		return nil
	})
	if e == nil {
		t.Fatal("injected failure not detected")
	}
	if !reflect.DeepEqual(before, s.Snapshot()) {
		t.Fatal("record failure partially committed")
	}
	s.Close()
	again, e := Open(path)
	if e != nil {
		t.Fatal(e)
	}
	defer again.Close()
	if !reflect.DeepEqual(before, again.Snapshot()) {
		t.Fatal("disk escaped rollback")
	}
}

func TestProjectConversionPreservesDescendantsAndResources(t *testing.T) {
	d := Empty()
	root, _ := d.AddIssue("Convert me", "", "", "todo")
	child, _ := d.AddIssue("child", "", root.ID, "todo")
	grand, _ := d.AddIssue("grand", "", child.ID, "todo")
	file, _ := d.AddAttachment(Attachment{OwnerKind: "issue", OwnerID: root.ID, Name: "file.txt", Data: []byte("keep")})
	comment, _ := d.SaveComment(Comment{IssueID: root.ID, Body: "retain history"})
	p, e := d.ConvertIssueToProject(root.ID)
	if e != nil {
		t.Fatal(e)
	}
	if d.Issues[root.ID].DeletedAt == 0 || d.Issues[child.ID].ParentID != "" || d.Issues[grand.ID].ParentID != child.ID || d.Issues[grand.ID].ProjectID != p.ID || d.Attachments[file.ID].OwnerID != p.ID || d.Comments[comment.ID].Body != "retain history" {
		t.Fatal("conversion lost relationships or content")
	}
	if e = d.Validate(); e != nil {
		t.Fatal(e)
	}
	d.Projects["other"] = Project{ID: "other", Name: "Other", Status: "planned", Dependencies: []string{p.ID}}
	p.Dependencies = []string{"other"}
	d.Projects[p.ID] = p
	if e = d.Validate(); e == nil {
		t.Fatal("accepted project dependency cycle")
	}
}
