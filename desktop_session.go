package main

import (
	"encoding/json"
	"errors"
	"flow/internal/core"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"time"

	"github.com/egoist/mygo/ui"
)

// Desktop state belongs to this device, independently of shared workspace
// records. Runtime handles and bulk selections are never serialized.
type desktopSession struct {
	Appearance       string          `json:"appearance,omitempty"`
	Version          int             `json:"version"`
	Tabs             []navigationTab `json:"tabs"`
	Closed           []navigationTab `json:"closed,omitempty"`
	Active           int             `json:"active"`
	Next             int             `json:"next"`
	Recent           []string        `json:"recent,omitempty"`
	SidebarWidth     float32         `json:"sidebarWidth"`
	SidebarCollapsed bool            `json:"sidebarCollapsed"`
	FolderCollapsed  map[string]bool `json:"folderCollapsed,omitempty"`
	ProjectsTimeline bool            `json:"projectsTimeline,omitempty"`
	TimelineScale    string          `json:"timelineScale,omitempty"`
	TimelineStart    string          `json:"timelineStart,omitempty"`
	TimelineZoom     float32         `json:"timelineZoom,omitempty"`
}

func readDesktopSession(dir string) (desktopSession, bool, error) {
	var state desktopSession
	f, err := os.Open(filepath.Join(dir, "desktop-session.json"))
	if errors.Is(err, os.ErrNotExist) {
		return state, false, nil
	}
	if err != nil {
		return state, false, err
	}
	defer f.Close()
	const limit = 16 << 20
	b, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return state, false, err
	}
	if len(b) > limit {
		return state, false, errors.New("desktop_session_too_large")
	}
	if err = json.Unmarshal(b, &state); err != nil {
		return state, false, err
	}
	if state.Version != 1 {
		return state, false, errors.New("unsupported_desktop_session")
	}
	if len(state.Tabs) == 0 || len(state.Tabs) > 200 || len(state.Closed) > 20 {
		return state, false, errors.New("invalid_desktop_session")
	}
	ids := map[string]bool{}
	for _, tab := range append(slices.Clone(state.Tabs), state.Closed...) {
		n, e := strconv.Atoi(tab.ID)
		if e != nil || n <= 0 || n > 1_000_000_000 || ids[tab.ID] || len(tab.Back) > 50 || len(tab.Forward) > 50 {
			return state, false, errors.New("invalid_desktop_session")
		}
		ids[tab.ID] = true
		state.Next = max(state.Next, n)
	}
	if state.Next > 1_000_000_000 {
		return state, false, errors.New("invalid_desktop_session")
	}
	state.Active = max(0, min(len(state.Tabs)-1, state.Active))
	if state.SidebarWidth == 0 {
		state.SidebarWidth = 224
	}
	state.SidebarWidth = max(200, min(380, state.SidebarWidth))
	return state, true, nil
}

func writeDesktopSession(dir string, raw []byte) error {
	return writeDesktopFile(dir, "desktop-session.json", raw)
}

func writeDesktopFile(dir, filename string, raw []byte) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".desktop-state-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if _, err = f.Write(raw); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(name, filepath.Join(dir, filename))
}

func (a *app) sanitizeRoute(r route) route {
	if !slices.Contains([]string{"issues", "project", "projects", "favorites", "archive", "trash", "settings"}, r.Page) {
		r.Page = "issues"
	}
	if r.Project != "" {
		p, ok := a.data.Projects[r.Project]
		if !ok || p.DeletedAt != 0 {
			r.Project = ""
			if r.Page == "project" {
				r.Page = "projects"
			}
		}
	}
	if r.Page == "project" && r.Project == "" {
		r.Page = "projects"
	}
	if i, ok := a.data.Issues[r.Issue]; !ok || i.DeletedAt != 0 && r.Page != "trash" {
		r.Issue = ""
	}
	if i, ok := a.data.Issues[r.Focused]; !ok || i.DeletedAt != 0 && r.Page != "trash" {
		r.Focused = ""
	}
	if _, ok := a.data.Views[r.SavedView]; !ok {
		r.SavedView = ""
	}
	if _, ok := a.data.Labels[r.Label]; !ok {
		r.Label = ""
	}
	if r.Config.Validate() != nil {
		r.Config = core.DefaultView()
		r.Board = false
	}
	if r.Priority < -1 || r.Priority > 4 {
		r.Priority = -1
	}
	return r
}

func (a *app) restoreDesktopSession(state desktopSession, exists bool, loadErr error) {
	if loadErr != nil {
		// Preserve a corrupt or newer file so this build cannot silently replace it.
		a.desktopDisabled = true
		a.notice = a.tr("未能恢复桌面会话，原文件已保留：", "Could not restore desktop session; original file kept: ") + loadErr.Error()
	}
	if exists {
		if validAppearance(state.Appearance) {
			a.appearance = state.Appearance
		}
		if a.desktop != nil && validAppearance(a.desktop.appearance) {
			a.appearance = a.desktop.appearance
		}
		a.applyNativeAppearance()
		sanitize := func(tabs []navigationTab) {
			for n := range tabs {
				tabs[n].Route = a.sanitizeRoute(tabs[n].Route)
				for k := range tabs[n].Back {
					tabs[n].Back[k] = a.sanitizeRoute(tabs[n].Back[k])
				}
				for k := range tabs[n].Forward {
					tabs[n].Forward[k] = a.sanitizeRoute(tabs[n].Forward[k])
				}
			}
		}
		sanitize(state.Tabs)
		sanitize(state.Closed)
		a.navigation.Tabs, a.navigation.Closed = state.Tabs, state.Closed
		a.navigation.Active, a.navigation.Next = state.Active, state.Next
		a.navigation.Recent = slices.Clone(state.Recent[:min(50, len(state.Recent))])
		a.navigation.SidebarCollapsed, a.sidebarWidth = state.SidebarCollapsed, state.SidebarWidth
		a.folderCollapsed = maps.Clone(state.FolderCollapsed)
		a.projectsTimeline = state.ProjectsTimeline
		if slices.Contains([]string{"week", "month", "quarter", "year"}, state.TimelineScale) {
			a.timelineScale = state.TimelineScale
		}
		if core.ValidDate(state.TimelineStart) {
			a.timelineStart = state.TimelineStart
		}
		if state.TimelineZoom >= .5 && state.TimelineZoom <= 4 {
			a.timelineZoom = state.TimelineZoom
		}
		tab := state.Tabs[state.Active]
		a.navigation.Back, a.navigation.Forward = tab.Back, tab.Forward
		a.restoreRoute(tab.Route)
	}
	a.desktopLoaded = true
	a.desktopSaved = a.desktopFingerprint()
	a.desktopObserved = a.desktopSaved
}

func (a *app) desktopFingerprint() string {
	a.storeTab()
	state := desktopSession{Version: 1, Appearance: a.appearance, Tabs: a.navigation.Tabs, Closed: a.navigation.Closed,
		Active: a.navigation.Active, Next: a.navigation.Next, Recent: a.navigation.Recent,
		SidebarWidth: a.sidebarWidth, SidebarCollapsed: a.navigation.SidebarCollapsed, FolderCollapsed: a.folderCollapsed,
		ProjectsTimeline: a.projectsTimeline, TimelineScale: a.timelineScale, TimelineStart: a.timelineStart, TimelineZoom: a.timelineZoom}
	b, err := json.Marshal(state)
	if err != nil {
		panic(fmt.Sprintf("invalid in-memory desktop session: %v", err))
	}
	return string(b)
}

func (a *app) desktopDirty() bool {
	return a.desktopLoaded && !a.desktopDisabled && a.ready && a.dataDir != "" && a.desktopFingerprint() != a.desktopSaved
}

func (a *app) persistDesktopSession() {
	if a.desktopBusy || !a.desktopDirty() {
		return
	}
	raw, dir := a.desktopFingerprint(), a.desktopDirectory()
	a.desktopBusy = true
	a.pending++
	done := func(_ core.Data, err error) {
		a.desktopBusy = false
		a.pending--
		if err != nil {
			a.err = err.Error()
			a.cancelClosing()
			return
		}
		a.desktopSaved = raw
		if a.closing() {
			a.persistDesktopSession()
		}
		a.finishQuit()
	}
	if a.window == nil {
		done(core.Data{}, writeDesktopSession(dir, []byte(raw)))
		return
	}
	a.jobs <- job{run: func(_ *core.Store) (core.Data, error) { return core.Data{}, writeDesktopSession(dir, []byte(raw)) }, done: done}
}

func (a *app) observeDesktopSession(c *ui.Context) {
	if !a.desktopLoaded || a.desktopDisabled || a.dataDir == "" {
		return
	}
	raw := a.desktopFingerprint()
	if raw != a.desktopObserved {
		a.desktopObserved, a.desktopChanged = raw, c.Now()
	}
	if raw != a.desktopSaved && !a.desktopBusy && a.err == "" {
		left := 600*time.Millisecond - c.Now().Sub(a.desktopChanged)
		if left <= 0 {
			a.persistDesktopSession()
		} else {
			c.After(left)
		}
	}
}
