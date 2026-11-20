package core

import (
	"reflect"
	"testing"
	"time"
)

func scheduleFixture(t *testing.T) Data {
	t.Helper()
	d := Empty()
	for _, p := range []Project{
		{ID: "a", Name: "Design", Status: "planned", Start: "2026-03-25", Due: "2026-03-28"},
		{ID: "b", Name: "Build", Status: "planned", Start: "2026-03-29", Due: "2026-04-01", Dependencies: []string{"a"}},
		{ID: "c", Name: "Release", Status: "planned", Start: "2026-04-02", Due: "2026-04-04", Dependencies: []string{"b"}},
		{ID: "fixed", Name: "Running", Status: "active", Start: "2026-03-29", Due: "2026-04-05", Dependencies: []string{"a"}},
	} {
		if err := d.PutProject(p); err != nil {
			t.Fatal(err)
		}
	}
	d.Milestones["m1"] = Milestone{ID: "m1", ProjectID: "a", Name: "Review", Due: "2026-03-26"}
	d.Milestones["m2"] = Milestone{ID: "m2", ProjectID: "a", Name: "Handoff", Due: "2026-03-28"}
	d.Milestones["mb"] = Milestone{ID: "mb", ProjectID: "b", Name: "Test", Due: "2026-03-31"}
	return d
}

func TestTimelineConstraintsModesAndAtomicity(t *testing.T) {
	target := []ScheduleTarget{{"project", "a"}}
	t.Run("auto bump consumes slack across DST", func(t *testing.T) {
		d := scheduleFixture(t)
		if _, err := d.ShiftSchedule(target, 4, "auto"); err != nil {
			t.Fatal(err)
		}
		for id, want := range map[string]string{"a": "2026-03-29", "b": "2026-04-01", "c": "2026-04-04", "fixed": "2026-03-29"} {
			if got := d.Projects[id].Start; got != want {
				t.Fatalf("%s: %s want %s", id, got, want)
			}
		}
		if d.Milestones["m1"].Due != "2026-03-30" || d.Milestones["mb"].Due != "2026-04-03" {
			t.Fatal("milestones did not follow actual project movement")
		}
	})
	t.Run("full chain includes running project once", func(t *testing.T) {
		d := scheduleFixture(t)
		targets := append(target, ScheduleTarget{"project", "b"}, ScheduleTarget{"milestone", "m1"})
		if _, err := d.ShiftSchedule(targets, 4, "all"); err != nil {
			t.Fatal(err)
		}
		if d.Projects["fixed"].Start != "2026-04-02" || d.Projects["b"].Start != "2026-04-02" || d.Milestones["m1"].Due != "2026-03-30" {
			t.Fatal("selected overlap shifted twice or full chain incomplete")
		}
	})
	t.Run("single preserves all related dates", func(t *testing.T) {
		d := scheduleFixture(t)
		if _, err := d.ShiftSchedule(target, 4, "single"); err != nil {
			t.Fatal(err)
		}
		if d.Projects["b"].Start != "2026-03-29" || d.Milestones["m1"].Due != "2026-03-26" {
			t.Fatal("single moved related items")
		}
	})
	t.Run("earlier move bumps prerequisites", func(t *testing.T) {
		d := scheduleFixture(t)
		if _, err := d.ShiftSchedule([]ScheduleTarget{{"project", "c"}}, -4, "auto"); err != nil {
			t.Fatal(err)
		}
		if d.Projects["b"].Due != "2026-03-29" || d.Projects["a"].Due != "2026-03-26" {
			t.Fatal("prerequisites overlap after earlier move", d.Projects)
		}
	})
	t.Run("later milestones and rollback", func(t *testing.T) {
		d := scheduleFixture(t)
		if _, err := d.ShiftSchedule([]ScheduleTarget{{"milestone", "m1"}}, 2, "all"); err != nil {
			t.Fatal(err)
		}
		if d.Milestones["m2"].Due != "2026-03-30" || d.Projects["a"].Due != "2026-03-28" {
			t.Fatal("milestone chain changed project")
		}
		before := d.Clone()
		for _, bad := range []struct {
			targets []ScheduleTarget
			days    int
			mode    string
		}{{target, 3650000, "all"}, {append(target, ScheduleTarget{"issue", "missing"}), 1, "auto"}, {target, 1, "invalid"}} {
			if _, err := d.ShiftSchedule(bad.targets, bad.days, bad.mode); err == nil {
				t.Fatal("invalid shift accepted")
			}
			if !reflect.DeepEqual(d, before) {
				t.Fatal("partial dates leaked from failed operation")
			}
		}
	})
}

func TestMilestoneFilterFromTimeline(t *testing.T) {
	d := scheduleFixture(t)
	i, err := d.AddIssue("Review", "a", "", "todo")
	if err != nil {
		t.Fatal(err)
	}
	i.MilestoneID = "m1"
	if err = d.PutIssue(i); err != nil {
		t.Fatal(err)
	}
	d.AddIssue("Unassigned", "a", "", "todo")
	v := DefaultView()
	v.Filter.Rules = []FilterRule{{Field: "milestone", Op: "is", Values: []string{"m1"}}}
	if err = v.Validate(); err != nil {
		t.Fatal(err)
	}
	got := d.QueryView(Filter{}, v, time.Now())
	if len(got) != 1 || got[0].ID != i.ID {
		t.Fatal("milestone navigation included other tasks", got)
	}
}
