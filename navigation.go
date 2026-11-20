package main

import (
	"flow/internal/core"
	"fmt"
	"maps"
	"slices"

	"github.com/egoist/mygo/ui"
)

// Each tab owns its route history and list state. Rich text drafts remain shared
// so switching a tab never forks unsaved changes to the same issue.
type route struct {
	Config                                                          core.ViewConfig
	SavedView                                                       string
	Page, Project, Issue, ProjectTab, Query, Status, Label, Focused string
	Priority                                                        int
	Board                                                           bool
	Order, Return                                                   []string        `json:"-"`
	Selected                                                        map[string]bool `json:"-"`
	Collapsed                                                       map[string]bool
	List                                                            ui.ListState `json:"-"`
}
type navigationTab struct {
	ID            string
	Route         route
	Back, Forward []route
	Pinned        bool
}
type navigation struct {
	Back, Forward    []route
	Recent           []string
	Tabs             []navigationTab
	Closed           []navigationTab
	Active           int
	Next             int
	Restoring        bool
	SidebarCollapsed bool
	RevealTab        string
}

func (a *app) currentRoute() route {
	return route{Config: a.viewConfig.Clone(), SavedView: a.savedViewID, Page: a.page, Project: a.projectID, Issue: a.issueID, ProjectTab: a.projectTab, Query: a.query, Status: a.statusFilter, Label: a.labelFilter, Focused: a.keptFocus, Priority: a.priorityFilter, Board: a.board, Order: slices.Clone(a.issueOrder), Return: slices.Clone(a.returnIssues), Selected: maps.Clone(a.selected), Collapsed: maps.Clone(a.collapsed), List: a.list}
}
func (a *app) restoreRoute(r route) {
	r = a.sanitizeRoute(r)
	a.flushDrafts()
	a.closePicker()
	a.navigation.Restoring = true
	a.page, a.projectID, a.projectTab = r.Page, r.Project, r.ProjectTab
	a.query, a.statusFilter, a.labelFilter, a.priorityFilter = r.Query, r.Status, r.Label, r.Priority
	a.board, a.focusedIssue, a.keptFocus = r.Board, r.Focused, r.Focused
	a.viewConfig, a.savedViewID = r.Config.Clone(), r.SavedView
	if a.viewConfig.Layout == "" {
		a.viewConfig = core.DefaultView()
	}
	a.selected, a.collapsed = maps.Clone(r.Selected), maps.Clone(r.Collapsed)
	if a.selected == nil {
		a.selected = map[string]bool{}
	}
	if a.collapsed == nil {
		a.collapsed = map[string]bool{}
	}
	a.list, a.cacheKey = r.List, ""
	a.issueID = ""
	if r.Issue != "" {
		a.openIssue(r.Issue)
	}
	if r.Order != nil {
		a.issueOrder = slices.Clone(r.Order)
	}
	a.returnIssues = slices.Clone(r.Return)
	a.navigation.Restoring = false
	if len(a.navigation.Tabs) > a.navigation.Active {
		a.navigation.RevealTab = a.navigation.Tabs[a.navigation.Active].ID
	}
}
func (a *app) rememberNavigation() {
	if a.navigation.Restoring {
		return
	}
	a.navigation.Back = append(a.navigation.Back, a.currentRoute())
	if len(a.navigation.Back) > 50 {
		a.navigation.Back = a.navigation.Back[len(a.navigation.Back)-50:]
	}
	a.navigation.Forward = nil
	if a.issueID != "" {
		a.navigation.Recent = slices.DeleteFunc(a.navigation.Recent, func(id string) bool { return id == a.issueID })
		a.navigation.Recent = append([]string{a.issueID}, a.navigation.Recent...)
		if len(a.navigation.Recent) > 50 {
			a.navigation.Recent = a.navigation.Recent[:50]
		}
	}
}
func (a *app) historyStep(delta int) {
	from, to := &a.navigation.Back, &a.navigation.Forward
	if delta > 0 {
		from, to = to, from
	}
	if len(*from) == 0 {
		return
	}
	r := (*from)[len(*from)-1]
	*from = (*from)[:len(*from)-1]
	*to = append(*to, a.currentRoute())
	a.restoreRoute(r)
}
func (a *app) openSearchIssue(id, query string) {
	// Global results form their own list; do not inherit an unrelated project's
	// filters or issue sequence. Back navigation retains the originating page.
	a.navigate("issues", "")
	a.viewConfig = core.DefaultView()
	a.board = false
	a.query = query
	a.openIssue(id)
}

func (a *app) ensureTabs() {
	if len(a.navigation.Tabs) == 0 {
		a.navigation.Next++
		a.navigation.Tabs = []navigationTab{{ID: fmt.Sprint(a.navigation.Next)}}
		a.navigation.Active = 0
	}
}
func (a *app) storeTab() {
	a.ensureTabs()
	t := &a.navigation.Tabs[a.navigation.Active]
	t.Route = a.currentRoute()
	t.Back, t.Forward = slices.Clone(a.navigation.Back), slices.Clone(a.navigation.Forward)
}
func (a *app) selectTab(n int) {
	if n < 0 || n >= len(a.navigation.Tabs) || n == a.navigation.Active {
		return
	}
	a.storeTab()
	t := a.navigation.Tabs[n]
	a.navigation.Active = n
	a.navigation.Back, a.navigation.Forward = slices.Clone(t.Back), slices.Clone(t.Forward)
	a.restoreRoute(t.Route)
}
func (a *app) newTab() {
	if len(a.navigation.Tabs) >= 200 {
		a.notice = a.tr("请先关闭一些标签页", "Close some tabs before opening another")
		return
	}
	a.storeTab()
	a.navigation.Next++
	r := route{Config: core.DefaultView(), Page: "issues", ProjectTab: "issues", Priority: -1}
	a.navigation.Tabs = append(a.navigation.Tabs, navigationTab{ID: fmt.Sprint(a.navigation.Next), Route: r})
	a.selectTab(len(a.navigation.Tabs) - 1)
}
func (a *app) closeTab(n int) {
	if n < 0 || n >= len(a.navigation.Tabs) || a.navigation.Tabs[n].Pinned {
		return
	}
	a.storeTab()
	a.rememberClosedTab(a.navigation.Tabs[n])
	if len(a.navigation.Tabs) == 1 {
		a.navigation.Next++
		r := route{Config: core.DefaultView(), Page: "issues", ProjectTab: "issues", Priority: -1}
		a.navigation.Tabs = []navigationTab{{ID: fmt.Sprint(a.navigation.Next), Route: r}}
		a.navigation.Active = 0
		a.navigation.Back, a.navigation.Forward = nil, nil
		a.restoreRoute(r)
		return
	}
	active := a.navigation.Active
	a.navigation.Tabs = slices.Delete(a.navigation.Tabs, n, n+1)
	if n < active {
		a.navigation.Active--
		return
	}
	if n != active {
		return
	}
	a.navigation.Active = min(n, len(a.navigation.Tabs)-1)
	t := a.navigation.Tabs[a.navigation.Active]
	a.navigation.Back, a.navigation.Forward = slices.Clone(t.Back), slices.Clone(t.Forward)
	a.restoreRoute(t.Route)
}

func (a *app) rememberClosedTab(tab navigationTab) {
	a.navigation.Closed = append(a.navigation.Closed, tab)
	if len(a.navigation.Closed) > 20 {
		a.navigation.Closed = a.navigation.Closed[len(a.navigation.Closed)-20:]
	}
}

func (a *app) reopenTab() {
	if len(a.navigation.Closed) == 0 || len(a.navigation.Tabs) >= 200 {
		return
	}
	a.storeTab()
	n := len(a.navigation.Closed) - 1
	tab := a.navigation.Closed[n]
	a.navigation.Closed = a.navigation.Closed[:n]
	a.navigation.Tabs = append(a.navigation.Tabs, tab)
	a.selectTab(len(a.navigation.Tabs) - 1)
}

func (a *app) pinTab(n int) {
	if n < 0 || n >= len(a.navigation.Tabs) {
		return
	}
	a.storeTab()
	activeID := a.navigation.Tabs[a.navigation.Active].ID
	a.navigation.Tabs[n].Pinned = !a.navigation.Tabs[n].Pinned
	slices.SortStableFunc(a.navigation.Tabs, func(x, y navigationTab) int {
		if x.Pinned == y.Pinned {
			return 0
		}
		if x.Pinned {
			return -1
		}
		return 1
	})
	a.navigation.Active = slices.IndexFunc(a.navigation.Tabs, func(t navigationTab) bool { return t.ID == activeID })
	a.navigation.RevealTab = activeID
}

func (a *app) closeOtherTabs(n int) {
	if n < 0 || n >= len(a.navigation.Tabs) {
		return
	}
	a.storeTab()
	target := a.navigation.Tabs[n]
	var kept []navigationTab
	for _, tab := range a.navigation.Tabs {
		if tab.ID == target.ID || tab.Pinned {
			kept = append(kept, tab)
		} else {
			a.rememberClosedTab(tab)
		}
	}
	a.navigation.Tabs = kept
	a.navigation.Active = slices.IndexFunc(kept, func(t navigationTab) bool { return t.ID == target.ID })
	a.navigation.Back, a.navigation.Forward = slices.Clone(target.Back), slices.Clone(target.Forward)
	a.restoreRoute(target.Route)
}

func (a *app) routeTitle(r route) string {
	if i, ok := a.data.Issues[r.Issue]; ok {
		return i.Code() + " " + i.Title
	}
	if p, ok := a.data.Projects[r.Project]; ok {
		return p.Name
	}
	switch r.Page {
	case "settings":
		return a.tr("设置", "Settings")
	case "projects":
		return a.tr("项目", "Projects")
	case "trash":
		return a.tr("回收站", "Trash")
	case "favorites":
		return a.tr("收藏", "Favorites")
	default:
		return a.tr("所有任务", "All issues")
	}
}

// Navigation lives in the page header; old tab records remain readable for
// desktop-session compatibility, but no tab actions are exposed in the UI.
func (a *app) navigationControls(c *ui.Context) {
	hotIconButton(c, "sidebar", a.tr("切换侧栏", "Toggle sidebar"), "[").OnClick(func() { a.navigation.SidebarCollapsed = !a.navigation.SidebarCollapsed })
	ui.Row(c).Gap(2).Children(func() {
		iconButton(c, "back", a.tr("后退", "Go back")).Disabled(len(a.navigation.Back) == 0).OnClick(func() { a.historyStep(-1) })
		iconButton(c, "forward", a.tr("前进", "Go forward")).Disabled(len(a.navigation.Forward) == 0).OnClick(func() { a.historyStep(1) })
		iconButton(c, "clock", a.tr("最近访问", "Recently viewed")).PopoverMenuButton(func(m *ui.Menu) {
			for _, id := range a.navigation.Recent {
				if i, ok := a.data.Issues[id]; ok && i.DeletedAt == 0 {
					if m.Item(i.Code() + " " + i.Title).Chosen() {
						a.openSearchIssue(id, "")
					}
				}
			}
		})
	})
}
