package main

import (
	"flow/internal/core"
	"slices"
	"strconv"
	"strings"
)

type issueGroup struct {
	Key, Title, Field, Value string
	Items                    []core.Issue
}

func (a *app) grouped(items []core.Issue, field string, empty bool) []issueGroup {
	groups := []issueGroup{}
	index := map[string]int{}
	add := func(value, title string) {
		if _, ok := index[value]; !ok {
			index[value] = len(groups)
			groups = append(groups, issueGroup{Key: field + "/" + value, Title: title, Field: field, Value: value})
		}
	}
	switch field {
	case "status":
		for _, s := range core.Statuses {
			add(s, a.status(s))
		}
	case "priority":
		for _, p := range []int{1, 2, 3, 4, 0} {
			add(strconv.Itoa(p), a.priority(p))
		}
	case "project":
		for _, p := range a.data.ProjectList(false, false) {
			add(p.ID, p.Name)
		}
		add("", a.tr("无项目", "No project"))
	case "labels":
		for _, l := range a.data.LabelList() {
			add(l.ID, l.Name)
		}
		add("", a.tr("无标签", "No labels"))
	case "none", "":
		add("", a.tr("任务", "Issues"))
	}
	for _, i := range items {
		values := []string{""}
		switch field {
		case "status":
			values = []string{i.Status}
		case "priority":
			values = []string{strconv.Itoa(i.Priority)}
		case "project":
			values = []string{i.ProjectID}
		case "labels":
			if len(i.LabelIDs) > 0 {
				values = i.LabelIDs
			}
		case "due":
			values = []string{i.Due}
		}
		for _, v := range values {
			title := v
			if title == "" {
				title = a.tr("无日期", "No date")
			}
			add(v, title)
			n := index[v]
			groups[n].Items = append(groups[n].Items, i)
		}
	}
	if field == "due" {
		slices.SortFunc(groups, func(a, b issueGroup) int {
			if a.Value == "" {
				return 1
			}
			if b.Value == "" {
				return -1
			}
			return strings.Compare(a.Value, b.Value)
		})
	}
	if !empty {
		groups = slices.DeleteFunc(groups, func(g issueGroup) bool { return len(g.Items) == 0 })
	}
	return groups
}

func (a *app) newGroupedTask(groups ...issueGroup) {
	status, project := "backlog", a.projectID
	for _, g := range groups {
		if g.Field == "status" {
			status = g.Value
		}
		if g.Field == "project" {
			project = g.Value
		}
	}
	previous := a.projectID
	a.projectID = project
	a.newTask("", status)
	a.projectID = previous
	for _, g := range groups {
		switch g.Field {
		case "priority":
			a.newIssue.Priority, _ = strconv.Atoi(g.Value)
		case "labels":
			if g.Value != "" {
				a.newIssue.LabelIDs = []string{g.Value}
			}
		case "due":
			a.newIssue.Due = g.Value
			a.newDueOpen = g.Value != ""
		}
	}
}
func (a *app) issueRows(items []core.Issue) []issueRow {
	roots := items
	children := map[string][]core.Issue{}
	if a.viewConfig.SubIssues == "nested" {
		roots = nil
		present := map[string]bool{}
		for _, i := range items {
			present[i.ID] = true
		}
		for _, i := range items {
			if i.ParentID != "" && present[i.ParentID] {
				children[i.ParentID] = append(children[i.ParentID], i)
			} else {
				roots = append(roots, i)
			}
		}
	}
	rows := []issueRow{}
	var appendIssue func(core.Issue, int, string)
	appendIssue = func(i core.Issue, depth int, prefix string) {
		rows = append(rows, issueRow{issue: i, depth: depth, key: prefix + "/" + i.ID})
		for _, child := range children[i.ID] {
			appendIssue(child, depth+1, prefix)
		}
	}
	// Only a board keeps its empty groups: there they are columns to drop
	// into. In a list, and in trash, they are headers over nothing.
	empty := a.viewConfig.EmptyGroups && a.board && a.page != "trash"
	for _, g := range a.grouped(roots, a.viewConfig.GroupBy, empty) {
		if a.viewConfig.GroupBy != "none" {
			rows = append(rows, issueRow{header: g.Key, title: g.Title, count: len(g.Items), group: g, key: g.Key})
		}
		if a.collapsed[g.Key] && !a.board {
			continue
		}
		if a.viewConfig.Subgroup != "none" && a.viewConfig.Subgroup != "" {
			for _, sub := range a.grouped(g.Items, a.viewConfig.Subgroup, empty) {
				key := g.Key + "/" + sub.Key
				rows = append(rows, issueRow{header: key, title: sub.Title, count: len(sub.Items), group: sub, parentGroup: g, key: key, depth: 1})
				if !a.collapsed[key] || a.board {
					for _, i := range sub.Items {
						appendIssue(i, 1, key)
					}
				}
			}
		} else {
			for _, i := range g.Items {
				appendIssue(i, 0, g.Key)
			}
		}
	}
	return rows
}
func (a *app) dragIDs(id string) []string {
	ids := []string{}
	if !a.selected[id] {
		return []string{id}
	}
	for _, i := range a.filtered() {
		if a.selected[i.ID] {
			ids = append(ids, i.ID)
		}
	}
	return ids
}
func (a *app) dropIssues(ids []string, g issueGroup, before string) {
	a.apply(func(d *core.Data) error {
		if g.Field == "project" {
			if e := d.MoveIssueProjects(ids, g.Value); e != nil {
				return e
			}
		}
		for _, id := range ids {
			i, ok := d.Issues[id]
			if !ok || i.DeletedAt != 0 {
				continue
			}
			switch g.Field {
			case "status":
				i.Status = g.Value
			case "priority":
				i.Priority, _ = strconv.Atoi(g.Value)
			case "project":
				i.ProjectID = g.Value
			case "due":
				i.Due = g.Value
			case "labels":
				if g.Value == "" {
					i.LabelIDs = nil
				} else if !slices.Contains(i.LabelIDs, g.Value) {
					i.LabelIDs = append(slices.Clone(i.LabelIDs), g.Value)
				}
			}
			if e := d.PutIssue(i); e != nil {
				return e
			}
			if !slices.Contains(ids, before) {
				if e := d.Move(id, i.Status, before); e != nil {
					return e
				}
			}
		}
		return nil
	}, nil)
}
