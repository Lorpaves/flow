package core

import (
	"cmp"
	"errors"
	"slices"
	"strconv"
	"strings"
	"time"
)

type FilterRule struct {
	ID     string   `json:"id,omitempty"`
	Field  string   `json:"field"`
	Op     string   `json:"op"`
	Values []string `json:"values,omitempty"`
}
type FilterGroup struct {
	Any    bool          `json:"any,omitempty"`
	Rules  []FilterRule  `json:"rules,omitempty"`
	Groups []FilterGroup `json:"groups,omitempty"`
}
type ViewConfig struct {
	Layout      string      `json:"layout"`
	GroupBy     string      `json:"groupBy"`
	Subgroup    string      `json:"subgroup"`
	OrderBy     string      `json:"orderBy"`
	Descending  bool        `json:"descending"`
	Completed   string      `json:"completed"`
	SubIssues   string      `json:"subIssues"`
	EmptyGroups bool        `json:"emptyGroups"`
	Properties  []string    `json:"properties"`
	Filter      FilterGroup `json:"filter"`
}
type SavedView struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	ProjectID string     `json:"projectId,omitempty"`
	Query     string     `json:"query,omitempty"`
	Config    ViewConfig `json:"config"`
}

func DefaultView() ViewConfig {
	return ViewConfig{EmptyGroups: true, Layout: "list", GroupBy: "status", Subgroup: "none", OrderBy: "manual", Completed: "all", SubIssues: "all", Properties: []string{"id", "status", "priority", "labels", "due"}}
}
func (g FilterGroup) Clone() FilterGroup {
	g.Rules = slices.Clone(g.Rules)
	for n := range g.Rules {
		g.Rules[n].Values = slices.Clone(g.Rules[n].Values)
	}
	g.Groups = slices.Clone(g.Groups)
	for n := range g.Groups {
		g.Groups[n] = g.Groups[n].Clone()
	}
	return g
}
func (v ViewConfig) Clone() ViewConfig {
	v.Properties = slices.Clone(v.Properties)
	v.Filter = v.Filter.Clone()
	return v
}
func (g FilterGroup) Validate(depth int) error {
	if depth > 8 || len(g.Rules) > 100 || len(g.Groups) > 30 {
		return errors.New("filter_too_complex")
	}
	for _, r := range g.Rules {
		if !slices.Contains([]string{"status", "priority", "labels", "project", "milestone", "title", "description", "due", "created", "updated", "parent", "favorite"}, r.Field) || !slices.Contains([]string{"is", "is_not", "contains", "not_contains", "all", "before", "after", "empty", "not_empty"}, r.Op) {
			return errors.New("invalid_filter")
		}
		if len(r.Values) > 100 {
			return errors.New("filter_too_complex")
		}
		if (r.Op == "before" || r.Op == "after") && (len(r.Values) != 1 || !ValidDate(r.Values[0]) || r.Values[0] == "") {
			return errors.New("invalid_filter_date")
		}
	}
	for _, child := range g.Groups {
		if e := child.Validate(depth + 1); e != nil {
			return e
		}
	}
	return nil
}
func (v ViewConfig) Validate() error {
	groups := []string{"none", "status", "priority", "project", "labels", "due"}
	if !slices.Contains([]string{"list", "board", "split"}, v.Layout) || !slices.Contains(groups, v.GroupBy) || !slices.Contains(groups, v.Subgroup) || !slices.Contains([]string{"manual", "priority", "title", "due", "created", "updated"}, v.OrderBy) || !slices.Contains([]string{"all", "hide", "week", "month"}, v.Completed) || !slices.Contains([]string{"all", "hide", "nested"}, v.SubIssues) {
		return errors.New("invalid_view")
	}
	for _, p := range v.Properties {
		if !slices.Contains([]string{"id", "status", "priority", "project", "labels", "due", "created", "updated"}, p) {
			return errors.New("invalid_view_property")
		}
	}
	return v.Filter.Validate(0)
}
func fieldValues(i Issue, field string) []string {
	switch field {
	case "status":
		return []string{i.Status}
	case "priority":
		return []string{strconv.Itoa(i.Priority)}
	case "labels":
		return i.LabelIDs
	case "project":
		return []string{i.ProjectID}
	case "milestone":
		return []string{i.MilestoneID}
	case "title":
		return []string{i.Title}
	case "description":
		return []string{i.Plain}
	case "due":
		return []string{i.Due}
	case "created":
		return []string{time.UnixMilli(i.CreatedAt).Format("2006-01-02")}
	case "updated":
		return []string{time.UnixMilli(i.UpdatedAt).Format("2006-01-02")}
	case "parent":
		return []string{i.ParentID}
	case "favorite":
		return []string{strconv.FormatBool(i.Favorite)}
	}
	return nil
}
func matchesRule(i Issue, r FilterRule) bool {
	values := fieldValues(i, r.Field)
	empty := len(values) == 0 || len(values) == 1 && values[0] == ""
	if r.Op == "empty" {
		return empty
	}
	if r.Op == "not_empty" {
		return !empty
	}
	match := false
	for _, want := range r.Values {
		found := false
		for _, got := range values {
			switch r.Op {
			case "contains", "not_contains":
				found = found || strings.Contains(strings.ToLower(got), strings.ToLower(want))
			case "before":
				found = found || got != "" && got < want
			case "after":
				found = found || got != "" && got > want
			default:
				found = found || strings.EqualFold(got, want)
			}
		}
		if r.Op == "all" && !found {
			return false
		}
		match = match || found
	}
	if r.Op == "is_not" || r.Op == "not_contains" {
		return !match
	}
	if r.Op == "all" {
		return true
	}
	return match
}
func (g FilterGroup) Matches(i Issue) bool {
	if len(g.Rules)+len(g.Groups) == 0 {
		return true
	}
	result := !g.Any
	combine := func(value bool) {
		if g.Any {
			result = result || value
		} else {
			result = result && value
		}
	}
	for _, r := range g.Rules {
		combine(matchesRule(i, r))
	}
	for _, child := range g.Groups {
		combine(child.Matches(i))
	}
	return result
}
func (d Data) QueryView(f Filter, v ViewConfig, now time.Time) []Issue {
	items := d.Query(f)
	items = slices.DeleteFunc(items, func(i Issue) bool {
		if !v.Filter.Matches(i) {
			return true
		}
		if v.SubIssues == "hide" && i.ParentID != "" {
			return true
		}
		if i.Status == "done" || i.Status == "canceled" || i.Status == "duplicate" {
			switch v.Completed {
			case "hide":
				return true
			case "week":
				return i.UpdatedAt < now.AddDate(0, 0, -7).UnixMilli()
			case "month":
				return i.UpdatedAt < now.AddDate(0, -1, 0).UnixMilli()
			}
		}
		return false
	})
	slices.SortStableFunc(items, func(a, b Issue) int {
		n := 0
		switch v.OrderBy {
		case "priority":
			ap, bp := a.Priority, b.Priority
			if ap == 0 {
				ap = 5
			}
			if bp == 0 {
				bp = 5
			}
			n = cmp.Compare(ap, bp)
		case "title":
			n = strings.Compare(strings.ToLower(a.Title), strings.ToLower(b.Title))
		case "due":
			ad, bd := a.Due, b.Due
			if ad == "" {
				ad = "9999"
			}
			if bd == "" {
				bd = "9999"
			}
			n = strings.Compare(ad, bd)
		case "created":
			n = cmp.Compare(a.CreatedAt, b.CreatedAt)
		case "updated":
			n = cmp.Compare(a.UpdatedAt, b.UpdatedAt)
		default:
			n = cmp.Compare(a.Position, b.Position)
		}
		if n == 0 {
			n = cmp.Compare(a.Number, b.Number)
		}
		if v.Descending {
			return -n
		}
		return n
	})
	return items
}
func (d Data) ViewList() []SavedView {
	out := []SavedView{}
	for _, v := range d.Views {
		out = append(out, v)
	}
	slices.SortFunc(out, func(a, b SavedView) int { return strings.Compare(a.Name, b.Name) })
	return out
}
