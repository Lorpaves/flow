package main

import (
	"flow/internal/core"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/richtext"
)

func TestProjectRecordsMilestonesAndDocumentsUI(t *testing.T) {
	a := testApp()
	p := a.data.ProjectList(false, false)[0]
	a.navigate("project", p.ID)
	tt := ui.NewTester(a.view, 1000, 680)
	click(t, tt, "Milestones")
	click(t, tt, "New milestone")
	tt.Type("中文阶段")
	click(t, tt, "Save milestone")
	if len(a.data.Milestones) != 1 || a.milestoneOpen {
		t.Fatalf("milestone did not save: %s", a.err)
	}
	m := a.data.MilestoneList(p.ID)[0]
	i := a.data.Query(core.Filter{ProjectID: p.ID})[0]
	click(t, tt, "Add issue to milestone: "+m.Name)
	tt.Type(i.Title)
	tt.Key(0, ui.KeyEnter)
	if a.data.Issues[i.ID].MilestoneID != m.ID {
		t.Fatalf("issue not assigned: %s", a.err)
	}
	click(t, tt, "Complete milestone: "+m.Name)
	if !a.data.Milestones[m.ID].Completed {
		t.Fatal("derived checkbox lost its change")
	}
	capture(t, tt, "18-project-milestones")
	click(t, tt, "Documents")
	click(t, tt, "New document")
	tt.Type("设计文档")
	click(t, tt, "Document body")
	tt.Type("正文与决策")
	click(t, tt, "Save document")
	if len(a.data.Documents) != 1 || a.documentOpen {
		t.Fatalf("document did not save: %s", a.err)
	}
	capture(t, tt, "19-project-documents")
	click(t, tt, "Updates")
	click(t, tt, "Write an update")
	click(t, tt, "Update body")
	tt.Type("完成第一阶段")
	click(t, tt, "Save update")
	if len(a.data.Updates) != 1 || a.updateOpen || a.data.Projects[p.ID].Health != "on_track" {
		t.Fatalf("update did not save: %s", a.err)
	}
	capture(t, tt, "20-project-updates")
	click(t, tt, "Milestones")
	click(t, tt, "Delete milestone: "+m.Name)
	if len(a.data.Milestones) != 0 || a.data.Issues[i.ID].MilestoneID != "" {
		t.Fatalf("deleted milestone left issue reference: %s", a.err)
	}
}

func TestProjectFormsPreserveConcurrentChanges(t *testing.T) {
	a := testApp()
	p := a.data.ProjectList(false, false)[0]
	a.editProject(p.ID)
	a.projectForm.Name = "本地改名"
	remote := a.data.Clone()
	current := remote.Projects[p.ID]
	current.Priority, current.Health = 1, "at_risk"
	remote.Projects[p.ID] = current
	a.acceptExternal(remote)
	a.saveProject()
	if got := a.data.Projects[p.ID]; got.Name != "本地改名" || got.Priority != 1 || got.Health != "at_risk" {
		t.Fatalf("project form overwrote remote properties: %+v, %s", got, a.err)
	}
	doc, e := a.data.SaveDocument(core.Document{OwnerKind: "project", OwnerID: p.ID, Name: "Original", Body: "**bold**\n"})
	if e != nil {
		t.Fatal(e)
	}
	a.editDocument("project", p.ID, doc.ID)
	a.documentForm.Name = "Renamed"
	remote = a.data.Clone()
	doc.Body = "MCP 新正文"
	remote.Documents[doc.ID] = doc
	a.acceptExternal(remote)
	a.saveDocument()
	if got := a.data.Documents[doc.ID]; got.Name != "Renamed" || got.Body != doc.Body {
		t.Fatalf("untouched Markdown serialization overwrote MCP body: %+v", got)
	}
	old, e := a.data.SaveProjectUpdate(core.ProjectUpdate{ProjectID: p.ID, Body: "旧更新", Health: "on_track"})
	if e != nil {
		t.Fatal(e)
	}
	a.editProjectUpdate(p.ID, old.ID)
	a.updateText = richtext.NewDocument(richtext.NewText("仅修正旧更新文字"))
	remote = a.data.Clone()
	if _, e = remote.SaveProjectUpdate(core.ProjectUpdate{ProjectID: p.ID, Body: "新风险", Health: "at_risk"}); e != nil {
		t.Fatal(e)
	}
	a.acceptExternal(remote)
	a.saveProjectUpdate()
	if got := a.data.Projects[p.ID].Health; got != "at_risk" {
		t.Fatalf("old update changed newer health to %s", got)
	}
	a.editDocument("project", p.ID, doc.ID)
	a.documentForm.Name = "Must not revive"
	remote = a.data.Clone()
	delete(remote.Documents, doc.ID)
	a.acceptExternal(remote)
	a.saveDocument()
	if _, exists := a.data.Documents[doc.ID]; exists || !a.documentOpen || a.err == "" {
		t.Fatal("deleted document was resurrected or form lost")
	}
}

func TestProjectTimelineAndDependencyCycleUI(t *testing.T) {
	a := testApp()
	projects := a.data.ProjectList(false, false)
	p, other := projects[0], projects[1]
	p.Start, p.Due = "2026-03-10", "2026-03-16"
	a.data.Projects[p.ID] = p
	a.navigate("project", p.ID)
	a.projectTab = "timeline"
	i := a.data.Query(core.Filter{ProjectID: p.ID})[0]
	i.Due = "2026-03-28"
	a.data.Issues[i.ID] = i
	tt := ui.NewTester(a.view, 1360, 880)
	label := "Drag dates: " + p.Name
	from, ok := tt.Find(label)
	if !ok || from.W <= 0 || from.H <= 0 {
		t.Fatalf("timeline has no visible bar: %s", label)
	}
	x, y := from.X+from.W/2, from.Y+from.H/2
	_, _, cell := a.timelineRange()
	tt.Press(x, y)
	tt.Move(x-12, y)
	tt.Move(x-2*cell, y)
	tt.Release(x-2*cell, y)
	if got := a.data.Projects[p.ID]; got.Start != "2026-03-08" || got.Due != "2026-03-14" {
		t.Fatalf("timeline drag failed or changed range duration: %+v / %s", got, a.err)
	}
	a.shiftTimeline(timelineDrag{Kind: "issue", ID: i.ID, Date: "2026-03-28"}, "2026-03-30")
	if got := a.data.Issues[i.ID].Due; got != "2026-03-30" {
		t.Fatalf("calendar shift across DST: %s", got)
	}
	capture(t, tt, "21-project-timeline")
	a.projectTab = "overview"
	dep := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(a.theme)
		ui.Column(c).Fill().Padding(20).Children(func() { a.projectDependencies(c, a.projectID) })
		a.dialogError(c)
	}, 1000, 680)
	click(t, dep, "Set dependencies")
	dep.Type(other.Name)
	dep.Key(0, ui.KeyEnter)
	dep.Key(0, ui.KeyEscape)
	if len(a.data.Projects[p.ID].Dependencies) != 1 {
		t.Fatalf("dependency not saved: %s", a.err)
	}
	a.projectID = other.ID
	dep.Frame()
	click(t, dep, "Set dependencies")
	dep.Type(p.Name)
	dep.Key(0, ui.KeyEnter)
	if len(a.data.Projects[other.ID].Dependencies) != 0 || !strings.Contains(a.err, "cycle") {
		t.Fatalf("cycle accepted: %s", a.err)
	}
}
