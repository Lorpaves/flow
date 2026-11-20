package main

import (
	"encoding/json"
	"flow/internal/core"
	"github.com/egoist/mygo/ui"
	"testing"
	"time"
)

func timelineApp(t *testing.T) (*app, string, string, string) {
	t.Helper()
	a := testApp()
	projects := a.data.ProjectList(false, false)
	p, q := projects[0], projects[1]
	p.Name, p.Start, p.Due, p.Status = "Roadmap Design", "2026-11-02", "2026-11-06", "planned"
	q.Name, q.Start, q.Due, q.Status = "Roadmap Build", "2026-11-07", "2026-11-10", "active"
	q.Dependencies = []string{p.ID}
	a.data.Projects[p.ID], a.data.Projects[q.ID] = p, q
	m, e := a.data.SaveMilestone(core.Milestone{ProjectID: p.ID, Name: "Review checkpoint", Due: "2026-11-04"})
	if e != nil {
		t.Fatal(e)
	}
	i, e := a.data.AddIssue("Checkpoint task", p.ID, "", "todo")
	if e != nil {
		t.Fatal(e)
	}
	i.MilestoneID = m.ID
	a.data.PutIssue(i)
	a.navigate("projects", "")
	a.timelineStart = "2026-11-01"
	return a, p.ID, q.ID, m.ID
}

func TestTimelineWorkspaceZoomSelectionAndSession(t *testing.T) {
	a, p, _, m := timelineApp(t)
	tt := ui.NewTester(a.view, 1360, 880)
	click(t, tt, "Timeline")
	if !a.projectsTimeline {
		t.Fatal("timeline layout did not open")
	}
	for _, v := range []struct {
		key   ui.Key
		scale string
	}{{ui.KeyY, "year"}, {ui.KeyQ, "quarter"}, {ui.KeyW, "week"}, {ui.KeyM, "month"}} {
		tt.Key(0, v.key)
		if a.timelineScaleName() != v.scale {
			t.Fatal("zoom shortcut", v.scale, a.timelineScaleName())
		}
	}
	tt.Key(0, ui.KeyY)
	tt.Key(0, ui.KeyEqual)
	if a.timelineZoom != 1.25 {
		t.Fatal("keyboard zoom")
	}
	capture(t, tt, "30-timeline-year")
	tt.Pinch(1000, 390, 1.5)
	if a.timelineZoom <= 1.25 {
		t.Fatal("pinch did not zoom")
	}
	tt.Key(0, ui.KeyM)
	click(t, tt, "Timeline item: Roadmap Design")
	tt.Key(0, ui.KeyX)
	if a.timelineSelected["project/"+p] {
		t.Fatal("X did not toggle selected project")
	}
	tt.Key(0, ui.KeyRight)
	if a.timelineFocus != "milestone/"+m {
		t.Fatal("keyboard did not advance to milestone", a.timelineFocus)
	}
	tt.Key(0, ui.KeyEnter)
	if a.page != "project" || a.projectTab != "issues" || len(a.filtered()) != 1 {
		t.Fatal("milestone Enter did not show filtered issues", a.page, a.projectTab, a.viewConfig)
	}
	a.navigate("projects", "")
	a.timelineScale = "quarter"
	a.timelineZoom = 1.5
	tt.Frame()
	var saved desktopSession
	if err := json.Unmarshal([]byte(a.desktopFingerprint()), &saved); err != nil {
		t.Fatal(err)
	}
	b := newApp(a.data)
	b.restoreDesktopSession(saved, true, nil)
	if !b.projectsTimeline || b.timelineScale != "quarter" || b.timelineZoom != 1.5 || b.timelineStart != "2026-11-01" {
		t.Fatal("timeline view not restored")
	}
	capture(t, tt, "30-timeline-quarter")
}

func TestTimelineModifierDragAndDependencyLines(t *testing.T) {
	for _, mode := range []struct {
		mods             ui.Modifiers
		milestone, other string
	}{{ui.Cmd, "2026-11-04", "2026-11-07"}, {ui.Shift, "2026-11-06", "2026-11-09"}, {0, "2026-11-06", "2026-11-07"}} {
		a, p, q, m := timelineApp(t)
		a.projectsTimeline = true
		var window *ui.Context
		tt := ui.NewTester(func(c *ui.Context) { window = c; a.view(c) }, 1360, 880)
		bar, ok := tt.Find("Drag dates: Roadmap Design")
		if !ok {
			t.Fatal("missing scheduled project")
		}
		x, y := bar.X+bar.W/2, bar.Y+bar.H/2
		if mode.mods == 0 {
			_, _, cell := a.timelineRange()
			tt.Press(x, y)
			tt.Move(x+12, y)
			tt.Move(x+2*cell, y)
			tt.Release(x+2*cell, y)
		} else {
			// MyGo Tester pointer events carry zero modifiers even after
			// HoldModifiers. Exercise the shared modifier/action dispatch here;
			// actual modified pointer delivery remains a native acceptance item.
			a.shiftTimelineWithMode(timelineDrag{"project", p, "2026-11-02"}, "2026-11-04", timelineMode(mode.mods))
			tt.Frame()
		}
		if a.data.Projects[p].Start != "2026-11-04" || a.data.Milestones[m].Due != mode.milestone || a.data.Projects[q].Start != mode.other {
			t.Fatal("drag modifiers not respected", mode, a.data.Projects[p], a.data.Projects[q], a.data.Milestones[m], a.err)
		}
		if mode.mods == ui.Cmd {
			capture(t, tt, "30-timeline-dependency-conflict")
		}
		// Clicking the painted connector must expose the concrete dependency.
		start, _, cell := a.timelineRange()
		links := a.timelineGeometry(window, a.timelineEntries(), start, cell, 310)
		if len(links) != 1 {
			t.Fatal("dependency line missing", links)
		}
		bend, end := links[0].Points[1], links[0].Points[2]
		tt.ClickAt(bend.X, (bend.Y+end.Y)/2)
		if a.timelineLinkFrom != p || a.timelineLinkTo != q {
			t.Fatal("dependency line click did not select edge")
		}
		click(t, tt, "Remove dependency")
		if len(a.data.Projects[q].Dependencies) != 0 {
			t.Fatal("dependency removal failed")
		}
		from, ok := tt.Find("Link dependency: Roadmap Design")
		if !ok {
			t.Fatal("missing dependency handle")
		}
		to, ok := tt.Find("Drag dates: Roadmap Build")
		if !ok {
			t.Fatal("missing dependency destination")
		}
		x1, y1, x2, y2 := from.X+from.W/2, from.Y+from.H/2, to.X+to.W/2, to.Y+to.H/2
		tt.Press(x1, y1)
		tt.Move(x1+8, y1)
		tt.Move(x2, y2)
		tt.Release(x2, y2)
		if len(a.data.Projects[q].Dependencies) != 1 || a.data.Projects[q].Dependencies[0] != p {
			t.Fatal("drag connector did not create dependency", a.err)
		}
	}
}

func abs(v float32) float32 { return max(v, -v) }

// dragBy presses the element labeled label in its middle, drags it dx
// DIPs across and lets go.
func dragBy(t *testing.T, tt *ui.Tester, label string, dx float32) {
	t.Helper()
	r, ok := tt.Find(label)
	if !ok {
		t.Fatal("missing", label)
	}
	x, y := r.X+r.W/2, r.Y+r.H/2
	tt.Press(x, y)
	tt.Move(x+dx/2, y)
	tt.Move(x+dx, y)
	tt.Release(x+dx, y)
	tt.Frame()
}

func TestTimelineBarEdgesChangeStartAndEnd(t *testing.T) {
	a, p, _, _ := timelineApp(t)
	a.projectsTimeline = true
	tt := ui.NewTester(a.view, 1360, 880)
	_, _, cell := a.timelineRange()
	project := func() core.Project { return a.data.Projects[p] }
	dragBy(t, tt, "Drag end date: Roadmap Design", 2*cell)
	if got := project(); got.Start != "2026-11-02" || got.Due != "2026-11-08" {
		t.Fatal("end edge did not move the due date alone", got.Start, got.Due, a.err)
	}
	capture(t, tt, "30-timeline-resize")
	dragBy(t, tt, "Drag start date: Roadmap Design", -cell)
	if got := project(); got.Start != "2026-11-01" || got.Due != "2026-11-08" {
		t.Fatal("start edge did not move the start alone", got.Start, got.Due, a.err)
	}
	// Under half a day snaps back.
	dragBy(t, tt, "Drag end date: Roadmap Design", cell/3)
	if got := project(); got.Due != "2026-11-08" {
		t.Fatal("a short drag changed the date", got.Due)
	}
	// Dragged past the other end, the start stops there: one day.
	dragBy(t, tt, "Drag start date: Roadmap Design", 20*cell)
	if got := project(); got.Start != "" || got.Due != "2026-11-08" {
		t.Fatal("start dragged past the end", got.Start, got.Due, a.err)
	}
	// A one-day issue grows from its due date.
	a.navigate("project", p)
	a.projectTab = "timeline"
	var issue core.Issue
	for _, i := range a.data.Issues {
		if i.Title == "Checkpoint task" {
			issue = i
		}
	}
	issue.Due = "2026-11-04"
	a.data.PutIssue(issue)
	tt.Frame()
	dragBy(t, tt, "Drag end date: "+issue.Code()+" Checkpoint task", 2*cell)
	if i := a.data.Issues[issue.ID]; i.Start != "2026-11-04" || i.Due != "2026-11-06" {
		t.Fatal("issue end edge", i.Start, i.Due, a.err)
	}
	dragBy(t, tt, "Drag dates: "+issue.Code()+" Checkpoint task", -cell)
	if i := a.data.Issues[issue.ID]; i.Start != "2026-11-03" || i.Due != "2026-11-05" {
		t.Fatal("moving the bar changed its length", i.Start, i.Due, a.err)
	}
}

func TestTimelineSchedulesIssueAcrossDays(t *testing.T) {
	a, p, _, _ := timelineApp(t)
	a.navigate("project", p)
	a.projectTab = "timeline"
	var issue core.Issue
	for _, i := range a.data.Issues {
		if i.Title == "Checkpoint task" {
			issue = i
		}
	}
	tt := ui.NewTester(a.view, 1360, 880)
	row := "Timeline date: " + issue.Code() + " Checkpoint task"
	click(t, tt, row)
	click(t, tt, "Start date")
	tt.Type("2026-11-03")
	click(t, tt, "End date")
	tt.Type("2026-11-05")
	click(t, tt, "Save date")
	tt.Advance(time.Second)
	if i := a.data.Issues[issue.ID]; i.Start != "2026-11-03" || i.Due != "2026-11-05" {
		t.Fatal("issue range not saved", i.Start, i.Due, a.err)
	}
	start, _, cell := a.timelineRange()
	bar, ok := tt.Find("Drag dates: " + issue.Code() + " Checkpoint task")
	item, _ := tt.Find("Timeline item: " + issue.Code() + " Checkpoint task")
	if !ok {
		t.Fatal("issue has no bar")
	}
	// The bar spans Nov 3 to 5: three days from where Nov 3 begins.
	left := item.X - 10 + 310 + float32(dayDistance(start, calendarDay("2026-11-03")))*cell
	if abs(bar.X-left) > 2 || abs(bar.W-3*cell) > 3 {
		t.Fatal("bar does not cover Nov 3-5", bar, left, cell)
	}
	capture(t, tt, "30-timeline-issue-range")
	// Dragging moves the whole span.
	a.shiftTimelineWithMode(timelineDrag{"issue", issue.ID, "2026-11-04"}, "2026-11-06", "auto")
	if i := a.data.Issues[issue.ID]; i.Start != "2026-11-05" || i.Due != "2026-11-07" {
		t.Fatal("drag did not move the span", i.Start, i.Due)
	}
	// One day picked again leaves only the due date.
	a.setTimelineDates(timelineEntry{Kind: "issue", ID: issue.ID}, "", "2026-11-09")
	if i := a.data.Issues[issue.ID]; i.Start != "" || i.Due != "2026-11-09" {
		t.Fatal("single day kept a start", i.Start, i.Due)
	}
}
