package main

import (
	"flow/internal/core"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestDatePickerCancelRelativeAndCalendarSave(t *testing.T) {
	a := testApp()
	id := a.visibleIssueIDs()[0]
	a.openIssue(id)
	tt := ui.NewTester(a.view, 1360, 880)
	click(t, tt, "Due date")
	click(t, tt, "Tomorrow")
	click(t, tt, "Cancel")
	if a.drafts[id].issue.Due != "" {
		t.Fatal("cancel changed due date")
	}
	click(t, tt, "Due date")
	click(t, tt, "End date")
	tt.Type("7 days")
	click(t, tt, "Save date")
	tt.Advance(time.Second)
	want, _ := core.ParseDate("7 days", time.Now())
	if a.data.Issues[id].Due != want {
		t.Fatal("relative date not saved")
	}
	click(t, tt, "Due date")
	capture(t, tt, "20-date-calendar")
	day := time.Now().AddDate(0, 0, 1).Format("January 2, 2006")
	click(t, tt, day)
	click(t, tt, "Save date")
	tt.Advance(time.Second)
	want, _ = core.ParseDate("tomorrow", time.Now())
	if a.data.Issues[id].Due != want || a.data.Issues[id].Start != "" {
		t.Fatal("calendar date not saved")
	}

	// Two clicks on the calendar give the issue a span of days.
	click(t, tt, "Due date")
	from, to := time.Now().AddDate(0, 0, 2), time.Now().AddDate(0, 0, 5)
	click(t, tt, from.Format("January 2, 2006"))
	click(t, tt, to.Format("January 2, 2006"))
	capture(t, tt, "20-date-range")
	click(t, tt, "Save date")
	tt.Advance(time.Second)
	if i := a.data.Issues[id]; i.Start != from.Format("2006-01-02") || i.Due != to.Format("2006-01-02") {
		t.Fatal("range not saved", i.Start, i.Due)
	}
	// A due date set before the start leaves a one-day issue.
	a.apply(func(d *core.Data) error {
		i := d.Issues[id]
		i.Due = from.AddDate(0, 0, -1).Format("2006-01-02")
		return d.PutIssue(i)
	}, nil)
	if i := a.data.Issues[id]; i.Start != "" {
		t.Fatal("start kept after an earlier due date", i.Start, i.Due)
	}
	a.apply(func(d *core.Data) error {
		i := d.Issues[id]
		i.Start, i.Due = from.Format("2006-01-02"), to.Format("2006-01-02")
		return d.PutIssue(i)
	}, nil)
	click(t, tt, "Due date")
	click(t, tt, "Remove date")
	tt.Advance(time.Second)
	if i := a.data.Issues[id]; i.Start != "" || i.Due != "" {
		t.Fatal("remove left dates", i.Start, i.Due)
	}
}
func TestDisplayFilterSavedViewAndRestart(t *testing.T) {
	a := testApp()
	tt := ui.NewTester(a.view, 1360, 880)
	click(t, tt, "Display options")
	click(t, tt, "Change Grouping")
	tt.Type("Priority")
	tt.Key(0, ui.KeyEnter)
	click(t, tt, "Display id")
	click(t, tt, "Apply display options")
	if a.viewConfig.GroupBy != "priority" || a.showsProperty("id") {
		t.Fatalf("display configuration not applied: config=%+v draft=%+v", a.viewConfig, a.viewDraft)
	}
	click(t, tt, "Filter")
	if e := tt.ChooseMenuItem("Advanced filter…"); e != nil {
		t.Fatal(e)
	}
	click(t, tt, "Add condition")
	click(t, tt, "Filter value root0")
	tt.Type("Todo")
	tt.Key(0, ui.KeyEnter)
	tt.Command("selectAll")
	tt.Type("Done")
	tt.Key(0, ui.KeyEnter)
	tt.Key(0, ui.KeyEscape)
	click(t, tt, "Apply filters")
	if len(a.filtered()) == 0 {
		t.Fatal("filter lost done tasks")
	}
	for _, i := range a.filtered() {
		if i.Status != "done" {
			t.Fatal("filter editor did not apply")
		}
	}
	click(t, tt, "Filter")
	if e := tt.ChooseMenuItem("Save as view…"); e != nil {
		t.Fatal(e)
	}
	click(t, tt, "View name")
	tt.Type("Completed focus")
	click(t, tt, "Confirm save view")
	if a.savedViewID == "" {
		t.Fatal("view not saved")
	}
	capture(t, tt, "21-saved-view")
	restarted := newApp(a.data.Clone())
	restarted.openSavedView(a.savedViewID)
	if restarted.viewConfig.GroupBy != "priority" || len(restarted.filtered()) != len(a.filtered()) {
		t.Fatal("restart lost view")
	}
	click(t, tt, "Display options")
	click(t, tt, "Change Layout")
	click(t, tt, "List and detail")
	click(t, tt, "Apply display options")
	id := a.visibleIssueIDs()[0]
	a.openIssue(id)
	tt.Frame()
	capture(t, tt, "22-split-detail")
	if _, ok := tt.Find("Issue title"); !ok {
		t.Fatal("split detail missing")
	}
	if _, ok := tt.Find(a.data.Issues[id].Code() + " " + a.data.Issues[id].Title); !ok {
		t.Fatal("split list missing")
	}
}
