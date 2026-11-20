package core

import (
	"slices"
	"testing"
	"time"
)

// An issue deleted for good takes its sub-issues and everything of theirs
// along, and leaves the workspace valid.
func TestEraseIssuesTakesWhatBelongsToThem(t *testing.T) {
	d := seeded(t)
	other := firstIssue(d)
	parent, e := d.AddIssue("Parent", "", "", "todo")
	if e != nil {
		t.Fatal(e)
	}
	child, _ := d.AddIssue("Child", "", parent.ID, "todo")
	comment, e := d.SaveComment(Comment{IssueID: child.ID, Body: "on the child"})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = d.SaveComment(Comment{IssueID: child.ID, ParentID: comment.ID, Body: "a reply"}); e != nil {
		t.Fatal(e)
	}
	if _, e = d.AddRelation(other.ID, parent.ID, "blocks"); e != nil {
		t.Fatal(e)
	}
	if _, e = d.AddAttachment(Attachment{OwnerKind: "issue", OwnerID: parent.ID, Name: "a.txt", Data: []byte("a")}); e != nil {
		t.Fatal(e)
	}
	if _, e = d.SaveDocument(Document{OwnerKind: "issue", OwnerID: child.ID, Name: "Notes"}); e != nil {
		t.Fatal(e)
	}
	if _, e = d.SetReminder(parent.ID, time.Now().Add(time.Hour)); e != nil {
		t.Fatal(e)
	}
	if e = d.EraseIssues([]string{parent.ID}); e == nil {
		t.Fatal("an issue outside the trash was erased")
	}
	d.TrashIssue(parent.ID)
	if e = d.EraseIssues([]string{parent.ID}); e != nil {
		t.Fatal(e)
	}
	for _, id := range []string{parent.ID, child.ID} {
		if _, ok := d.Issues[id]; ok {
			t.Fatal("an erased issue is still there")
		}
		if d.Erased[id].Kind != "issue" {
			t.Fatal("no erasure left for", id)
		}
	}
	if len(d.Comments) != 0 || len(d.Relations) != 0 || len(d.Attachments) != 0 || len(d.Documents) != 0 || len(d.Reminders) != 0 {
		t.Fatalf("left behind: %d comments, %d relations, %d attachments, %d documents, %d reminders", len(d.Comments), len(d.Relations), len(d.Attachments), len(d.Documents), len(d.Reminders))
	}
	if _, ok := d.Issues[other.ID]; !ok {
		t.Fatal("an issue related to the erased one went too")
	}
	if e = d.Validate(); e != nil {
		t.Fatal(e)
	}
}

func TestEraseProjectAndEmptyTrash(t *testing.T) {
	d := seeded(t)
	var project Project
	for _, p := range d.ProjectList(false, false) {
		project = p
		break
	}
	if _, e := d.SaveMilestone(Milestone{ProjectID: project.ID, Name: "Beta"}); e != nil {
		t.Fatal(e)
	}
	if _, e := d.SaveProjectUpdate(ProjectUpdate{ProjectID: project.ID, Body: "going well", Health: "on_track"}); e != nil {
		t.Fatal(e)
	}
	if _, e := d.SetProjectReminder(project.ID, time.Now().Add(time.Hour)); e != nil {
		t.Fatal(e)
	}
	d.Folders["f"] = FavoriteFolder{ID: "f", Name: "Starred", Items: []FavoriteRef{{Kind: "project", ID: project.ID}}}
	issues := 0
	for _, i := range d.Issues {
		if i.ProjectID == project.ID {
			issues++
		}
	}
	if e := d.EraseProject(project.ID); e == nil {
		t.Fatal("a project outside the trash was erased")
	}
	d.TrashProject(project.ID)
	loose := firstIssue(d)
	if loose.ProjectID == project.ID {
		for _, i := range d.Issues {
			if i.ProjectID != project.ID {
				loose = i
				break
			}
		}
	}
	d.TrashIssue(loose.ID)
	gotIssues, gotProjects := d.EmptyTrash()
	if gotProjects != 1 || gotIssues < issues+1 {
		t.Fatalf("emptied %d issues and %d projects", gotIssues, gotProjects)
	}
	if _, ok := d.Projects[project.ID]; ok || len(d.Milestones) != 0 || len(d.Updates) != 0 || len(d.Reminders) != 0 || len(d.Folders["f"].Items) != 0 {
		t.Fatal("the project or what was in it is still there")
	}
	for _, i := range d.Issues {
		if i.DeletedAt != 0 || i.ProjectID == project.ID {
			t.Fatal("the trash is not empty")
		}
	}
	if e := d.Validate(); e != nil {
		t.Fatal(e)
	}
}

// Another Mac that still has an issue deleted for good here, unchanged or
// restored from its trash, does not bring it back, whichever side merges.
func TestMergeKeepsAnIssueErasedOnEitherSide(t *testing.T) {
	base := seeded(t)
	i := firstIssue(base)
	base.TrashIssue(i.ID)
	here, there := base.Clone(), base.Clone()
	if e := here.EraseIssues([]string{i.ID}); e != nil {
		t.Fatal(e)
	}
	for _, other := range []Data{there, func() Data { d := there.Clone(); _ = d.RestoreIssue(i.ID); return d }()} {
		for _, m := range []Data{mustMerge(t, base, here, other), mustMerge(t, base, other, here)} {
			if _, ok := m.Issues[i.ID]; ok {
				t.Fatal("the erased issue came back")
			}
			if _, ok := m.Erased[i.ID]; !ok {
				t.Fatal("the erasure was lost")
			}
		}
	}
	// An issue made on the other Mac in a project erased here stays, on
	// its own.
	base = seeded(t)
	var project string
	for id := range base.Projects {
		project = id
		break
	}
	base.TrashProject(project)
	here, there = base.Clone(), base.Clone()
	if e := here.EraseProject(project); e != nil {
		t.Fatal(e)
	}
	made, e := there.AddIssue("Made meanwhile", "", "", "todo")
	if e != nil {
		t.Fatal(e)
	}
	made.ProjectID = project
	there.Issues[made.ID] = made
	m := mustMerge(t, base, here, there)
	if _, ok := m.Projects[project]; ok {
		t.Fatal("the erased project came back")
	}
	if got, ok := m.Issues[made.ID]; !ok || got.ProjectID != "" {
		t.Fatal("the issue made meanwhile was lost or kept its project")
	}
}

// Deleting for good through the store leaves no history of what is gone.
func TestEraseThroughTheStoreRecordsNoOrphanHistory(t *testing.T) {
	s, _ := testStore(t)
	var id string
	if _, e := s.Update(func(d *Data) error {
		d.Seed()
		i := firstIssue(*d)
		id = i.ID
		_, e := d.SaveComment(Comment{IssueID: id, Body: "soon gone"})
		return e
	}); e != nil {
		t.Fatal(e)
	}
	if _, e := s.Update(func(d *Data) error { d.TrashIssue(id); return nil }); e != nil {
		t.Fatal(e)
	}
	got, e := s.Update(func(d *Data) error { return d.EraseIssues([]string{id}) })
	if e != nil {
		t.Fatal(e)
	}
	if slices.ContainsFunc(mapsValues(got.Activities), func(v Activity) bool { return v.OwnerID == id }) {
		t.Fatal("history of the erased issue remains")
	}
	if _, ok := got.Erased[id]; !ok {
		t.Fatal("the erasure was not stored")
	}
	reopened, _, e := s.Refresh()
	if e != nil || len(reopened.Erased) != 1 {
		t.Fatalf("erasures after reload: %d, %v", len(reopened.Erased), e)
	}
}

func mapsValues[K comparable, V any](m map[K]V) []V {
	out := make([]V, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	return out
}
