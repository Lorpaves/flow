package core

import (
	"errors"
	"slices"
	"time"
)

// ScheduleTarget identifies an item on a timeline without retaining a stale record.
type ScheduleTarget struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

func shiftDate(raw string, days int) (string, error) {
	if raw == "" {
		return "", nil
	}
	d, err := time.Parse("2006-01-02", raw)
	if err != nil {
		return "", errors.New("invalid_date")
	}
	d = d.AddDate(0, 0, days)
	if d.Year() < 1 || d.Year() > 9999 {
		return "", errors.New("invalid_date")
	}
	return d.Format("2006-01-02"), nil
}

func dateDistance(from, to string) int {
	a, _ := time.Parse("2006-01-02", from)
	b, _ := time.Parse("2006-01-02", to)
	// Unix seconds avoid time.Duration saturation for distant calendar dates.
	return int((b.Unix() - a.Unix()) / 86400)
}

// ShiftSchedule changes dates atomically. auto bumps planned dependencies only
// when the move creates an overlap; all shifts the connected project chain (or
// the selected milestone and subsequent dated milestones); single leaves all
// related dates in place. Explicitly selected items always move together.
func (d *Data) ShiftSchedule(targets []ScheduleTarget, days int, mode string) ([]ScheduleTarget, error) {
	if mode == "" {
		mode = "auto"
	}
	if !slices.Contains([]string{"auto", "all", "single"}, mode) || len(targets) == 0 || days < -3650000 || days > 3650000 {
		return nil, errors.New("invalid_timeline_shift")
	}
	next := d.Clone()
	if err := next.Validate(); err != nil {
		return nil, err
	}
	selected := map[ScheduleTarget]bool{}
	for _, t := range targets {
		valid := false
		switch t.Kind {
		case "project":
			p, ok := next.Projects[t.ID]
			valid = ok && p.DeletedAt == 0
		case "milestone":
			m, ok := next.Milestones[t.ID]
			valid = ok && next.ownerActive("project", m.ProjectID)
		case "issue":
			i, ok := next.Issues[t.ID]
			valid = ok && i.DeletedAt == 0
		}
		if !valid {
			return nil, errors.New("not_found")
		}
		selected[t] = true
	}
	if mode == "all" {
		queue := slices.Clone(targets)
		for len(queue) > 0 {
			t := queue[0]
			queue = queue[1:]
			if t.Kind == "project" {
				p := next.Projects[t.ID]
				for _, other := range next.ProjectList(false, false) {
					if slices.Contains(p.Dependencies, other.ID) || slices.Contains(other.Dependencies, p.ID) {
						x := ScheduleTarget{"project", other.ID}
						if !selected[x] {
							selected[x] = true
							queue = append(queue, x)
						}
					}
				}
			} else if t.Kind == "milestone" {
				m := next.Milestones[t.ID]
				if m.Due != "" {
					for _, other := range next.MilestoneList(m.ProjectID) {
						if other.Due >= m.Due {
							selected[ScheduleTarget{"milestone", other.ID}] = true
						}
					}
				}
			}
		}
	}
	changed := map[ScheduleTarget]bool{}
	shiftMilestone := func(id string, delta int) error {
		m := next.Milestones[id]
		due, err := shiftDate(m.Due, delta)
		if err != nil {
			return err
		}
		m.Due = due
		next.Milestones[id] = m
		if m.Due != "" {
			changed[ScheduleTarget{"milestone", id}] = true
		}
		return nil
	}
	shiftProject := func(id string, delta int) error {
		p := next.Projects[id]
		var err error
		if p.Start, err = shiftDate(p.Start, delta); err != nil {
			return err
		}
		if p.Due, err = shiftDate(p.Due, delta); err != nil {
			return err
		}
		next.Projects[id] = p
		changed[ScheduleTarget{"project", id}] = true
		if mode != "single" {
			for _, m := range next.MilestoneList(id) {
				if err = shiftMilestone(m.ID, delta); err != nil {
					return err
				}
			}
		}
		return nil
	}
	// Stable order makes overlapping selections and dependency diamonds deterministic.
	ordered := []ScheduleTarget{}
	for t := range selected {
		ordered = append(ordered, t)
	}
	slices.SortFunc(ordered, func(a, b ScheduleTarget) int {
		if a.Kind == "project" && b.Kind != "project" {
			return -1
		}
		if b.Kind == "project" && a.Kind != "project" {
			return 1
		}
		if a.ID < b.ID {
			return -1
		}
		if a.ID > b.ID {
			return 1
		}
		return 0
	})
	queue := []string{}
	for _, t := range ordered {
		if changed[t] {
			continue
		}
		switch t.Kind {
		case "project":
			if err := shiftProject(t.ID, days); err != nil {
				return nil, err
			}
			queue = append(queue, t.ID)
		case "milestone":
			if err := shiftMilestone(t.ID, days); err != nil {
				return nil, err
			}
		case "issue":
			i := next.Issues[t.ID]
			var err error
			if i.Start, err = shiftDate(i.Start, days); err != nil {
				return nil, err
			}
			if i.Due, err = shiftDate(i.Due, days); err != nil {
				return nil, err
			}
			if err := next.PutIssue(i); err != nil {
				return nil, err
			}
			changed[t] = true
		}
	}
	if mode == "auto" && days != 0 {
		for len(queue) > 0 {
			id := queue[0]
			queue = queue[1:]
			p := next.Projects[id]
			for _, other := range next.ProjectList(false, false) {
				if other.Status != "planned" || selected[ScheduleTarget{"project", other.ID}] {
					continue
				}
				delta := 0
				start := other.Start
				if start == "" {
					start = other.Due
				}
				if days > 0 && slices.Contains(other.Dependencies, id) && p.Due != "" && start != "" && p.Due > start {
					delta = dateDistance(start, p.Due)
				}
				pStart := p.Start
				if pStart == "" {
					pStart = p.Due
				}
				if days < 0 && slices.Contains(p.Dependencies, other.ID) && other.Due != "" && pStart != "" && other.Due > pStart {
					delta = dateDistance(other.Due, pStart)
				}
				if delta != 0 {
					if err := shiftProject(other.ID, delta); err != nil {
						return nil, err
					}
					queue = append(queue, other.ID)
				}
			}
		}
	}
	if err := next.Validate(); err != nil {
		return nil, err
	}
	*d = next
	out := []ScheduleTarget{}
	for t := range changed {
		out = append(out, t)
	}
	slices.SortFunc(out, func(a, b ScheduleTarget) int {
		if a.Kind+a.ID < b.Kind+b.ID {
			return -1
		}
		if a == b {
			return 0
		}
		return 1
	})
	return out, nil
}
