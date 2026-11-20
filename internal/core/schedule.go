package core

import (
	"errors"
	"fmt"
	"slices"
	"time"
)

func clampCalendarDate(year int, month time.Month, day int, loc *time.Location) time.Time {
	last := time.Date(year, month+1, 0, 0, 0, 0, 0, loc).Day()
	return time.Date(year, month, min(day, last), 0, 0, 0, 0, loc)
}
func recurrenceDate(v Recurrence, base time.Time, periods int) time.Time {
	step := v.Every * periods
	switch v.Unit {
	case "day":
		return base.AddDate(0, 0, step)
	case "week":
		return base.AddDate(0, 0, step*7)
	case "month":
		target := time.Date(base.Year(), base.Month()+time.Month(step), 1, 0, 0, 0, 0, base.Location())
		return clampCalendarDate(target.Year(), target.Month(), v.AnchorDay, base.Location())
	case "year":
		return clampCalendarDate(base.Year()+step, base.Month(), v.AnchorDay, base.Location())
	}
	return base
}
func (d *Data) SaveRecurrence(v Recurrence) (Recurrence, error) {
	if v.ID == "" {
		v.ID = ID()
	}
	date, e := time.Parse("2006-01-02", v.NextDue)
	if e != nil {
		return v, errors.New("invalid_recurrence")
	}
	if v.AnchorDay == 0 {
		v.AnchorDay = date.Day()
	}
	if !validName(v.Form.Title) || !validForm(v.Form) || v.Every < 1 || v.Every > 365 || !slices.Contains([]string{"day", "week", "month", "year"}, v.Unit) || !ValidDate(v.EndDate) || v.AnchorDay < 1 || v.AnchorDay > 31 {
		return v, errors.New("invalid_recurrence")
	}
	v.Form = cloneIssue(v.Form)
	v.Form.ID = ""
	v.Form.Number = 0
	v.Form.DeletedAt = 0
	v.Form.CreatedAt = 0
	v.Form.UpdatedAt = 0
	d.Recurrences[v.ID] = v
	return v, nil
}

// A reminder belongs to exactly one issue or project. Existing issue records
// keep their original shape and IDs.
func (r Reminder) Owner() (string, string) {
	if r.IssueID != "" && r.ProjectID == "" {
		return "issue", r.IssueID
	}
	if r.ProjectID != "" && r.IssueID == "" {
		return "project", r.ProjectID
	}
	return "", ""
}
func (d Data) ReminderActive(r Reminder) bool { kind, id := r.Owner(); return d.ownerActive(kind, id) }
func (d *Data) SetReminder(issueID string, at time.Time) (Reminder, error) {
	return d.setOwnedReminder("issue", issueID, at)
}
func (d *Data) SetProjectReminder(projectID string, at time.Time) (Reminder, error) {
	return d.setOwnedReminder("project", projectID, at)
}
func (d *Data) setOwnedReminder(kind, id string, at time.Time) (Reminder, error) {
	if !d.ownerActive(kind, id) || at.UnixMilli() <= 0 {
		return Reminder{}, errors.New("invalid_reminder")
	}
	v := Reminder{ID: ID(), At: at.UnixMilli()}
	if kind == "project" {
		v.ProjectID = id
	} else {
		v.IssueID = id
	}
	d.Reminders[v.ID] = v
	return v, nil
}
func (d *Data) SnoozeReminder(id string, at time.Time) (Reminder, error) {
	v, ok := d.Reminders[id]
	if !ok || !d.ReminderActive(v) || at.UnixMilli() <= 0 {
		return v, errors.New("invalid_reminder")
	}
	v.At, v.FiredAt, v.DismissedAt = at.UnixMilli(), 0, 0
	d.Reminders[id] = v
	return v, nil
}

// RunScheduled is called inside Store.Update. Reopening catches up one issue
// per overdue recurrence, for the most recent scheduled date, and advances to
// the next future date. It never floods a long-closed workspace with old copies.
// The original month day is retained, so Jan 31 -> Feb 28 -> Mar 31.
func (d *Data) RunScheduled(now time.Time) ([]string, []string) {
	created, fired := []string{}, []string{}
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	keys := []string{}
	for id := range d.Recurrences {
		keys = append(keys, id)
	}
	slices.Sort(keys)
	for _, id := range keys {
		v := d.Recurrences[id]
		if v.Paused {
			continue
		}
		start, e := time.ParseInLocation("2006-01-02", v.NextDue, now.Location())
		if e != nil || start.After(today) {
			continue
		}
		end := today
		if v.EndDate != "" {
			if deadline, e := time.ParseInLocation("2006-01-02", v.EndDate, now.Location()); e == nil && deadline.Before(end) {
				end = deadline
			}
		}
		if start.After(end) {
			v.Paused = true
			d.Recurrences[id] = v
			continue
		}
		// Find the last elapsed period without iterating every missed day.
		high := max(1, (end.Year()-start.Year()+2)*366/v.Every+1)
		low := 0
		for low < high {
			mid := (low + high + 1) / 2
			if recurrenceDate(v, start, mid).After(end) {
				high = mid - 1
			} else {
				low = mid
			}
		}
		due := recurrenceDate(v, start, low)
		form := cloneIssue(v.Form)
		// A form spanning days keeps its length, ending on the occurrence.
		if from, to, e := spanDays(form.Start, form.Due); e == nil {
			form.Start = due.AddDate(0, 0, -dayCount(from, to)).Format("2006-01-02")
		} else {
			form.Start = ""
		}
		form.Due = due.Format("2006-01-02")
		i, e := d.CreateFromForm(form)
		if e != nil {
			v.Paused = true
			v.LastError = e.Error()
			d.Recurrences[id] = v
			continue
		}
		v.LastIssueID = i.ID
		v.LastError = ""
		v.NextDue = recurrenceDate(v, start, low+1).Format("2006-01-02")
		if v.EndDate != "" && v.NextDue > v.EndDate {
			v.Paused = true
		}
		d.Recurrences[id] = v
		created = append(created, i.ID)
	}
	for id, v := range d.Reminders {
		if v.FiredAt == 0 && v.DismissedAt == 0 && v.At <= now.UnixMilli() && d.ReminderActive(v) {
			v.FiredAt = now.UnixMilli()
			d.Reminders[id] = v
			fired = append(fired, id)
		}
	}
	slices.Sort(fired)
	return created, fired
}
func ParseReminder(input string, now time.Time) (time.Time, error) {
	if value, e := time.Parse(time.RFC3339, input); e == nil {
		return value, nil
	}
	for _, layout := range []string{"2006-01-02 15:04", "2006-01-02T15:04"} {
		if value, e := time.ParseInLocation(layout, input, now.Location()); e == nil {
			return value, nil
		}
	}
	if duration, e := time.ParseDuration(input); e == nil && duration > 0 {
		return now.Add(duration), nil
	}
	if date, e := ParseDate(input, now); e == nil && date != "" {
		value, e := time.ParseInLocation("2006-01-02", date, now.Location())
		if e == nil {
			return value.Add(9 * time.Hour), nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid_reminder_time")
}

// spanDays parses the days a form spans, from start to due.
func spanDays(start, due string) (time.Time, time.Time, error) {
	from, e := time.Parse("2006-01-02", start)
	if e != nil {
		return from, from, e
	}
	to, e := time.Parse("2006-01-02", due)
	return from, to, e
}

// dayCount is how many calendar days separate from and to.
func dayCount(from, to time.Time) int { return int(to.Sub(from).Hours()/24 + .5) }
