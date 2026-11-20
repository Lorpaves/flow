package core

import (
	"testing"
	"time"
)

// An issue spanning days keeps its start through every way a form becomes
// an issue: a copy, a template, a recurrence.
func TestFormsKeepTheStartDate(t *testing.T) {
	d := Empty()
	d.Seed()
	i, e := d.AddIssue("span", "", "", "todo")
	if e != nil {
		t.Fatal(e)
	}
	i.Start, i.Due = "2026-11-15", "2026-11-18"
	if e = d.PutIssue(i); e != nil {
		t.Fatal(e)
	}
	copy, e := d.CopyIssue(i.ID, "", false)
	if e != nil || copy.Start != "2026-11-15" || copy.Due != "2026-11-18" {
		t.Fatal("copy lost the start", copy.Start, copy.Due, e)
	}
	v, e := d.SaveTemplate(IssueTemplate{Name: "span", Form: d.Issues[i.ID]})
	if e != nil {
		t.Fatal(e)
	}
	made, e := d.CreateFromForm(v.Form)
	if e != nil || made.Start != "2026-11-15" || made.Due != "2026-11-18" {
		t.Fatal("template lost the start", made.Start, made.Due, e)
	}
	// A start without a due date is no span.
	form := d.Issues[i.ID]
	form.Due = ""
	if made, e = d.CreateFromForm(form); e != nil || made.Start != "" {
		t.Fatal("a lone start was kept", made.Start, e)
	}
	r, e := d.SaveRecurrence(Recurrence{Form: d.Issues[i.ID], Every: 1, Unit: "week", NextDue: "2026-11-25"})
	if e != nil {
		t.Fatal(e)
	}
	created, _ := d.RunScheduled(time.Date(2026, 11, 25, 9, 0, 0, 0, time.Local))
	if len(created) != 1 {
		t.Fatal("recurrence did not run", created)
	}
	if got := d.Issues[created[0]]; got.Start != "2026-11-22" || got.Due != "2026-11-25" {
		t.Fatal("recurrence did not keep the span", got.Start, got.Due, d.Recurrences[r.ID].LastError)
	}
	if e = d.Validate(); e != nil {
		t.Fatal(e)
	}
}
