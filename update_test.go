package main

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

type fakeUpdateRelease struct {
	version string
	notes   string
	date    time.Time
	install func(ctx context.Context, progress func(downloaded, total int64)) error
}

func (r *fakeUpdateRelease) Version() string { return r.version }
func (r *fakeUpdateRelease) Notes() string   { return r.notes }
func (r *fakeUpdateRelease) Date() time.Time { return r.date }
func (r *fakeUpdateRelease) Install(ctx context.Context, progress func(downloaded, total int64)) error {
	if r.install != nil {
		return r.install(ctx, progress)
	}
	if progress != nil {
		progress(512*1024, 1024*1024)
		progress(1024*1024, 1024*1024)
	}
	return nil
}

type fakeUpdateBackend struct {
	mu       sync.Mutex
	enabled  bool
	current  string
	checkFn  func(ctx context.Context) (updateRelease, error)
	relaunch func()
	checks   int
	restarts int
}

func (b *fakeUpdateBackend) Enabled() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.enabled
}

func (b *fakeUpdateBackend) CurrentVersion() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.current
}

func (b *fakeUpdateBackend) Check(ctx context.Context) (updateRelease, error) {
	b.mu.Lock()
	b.checks++
	fn := b.checkFn
	b.mu.Unlock()
	if fn == nil {
		return nil, nil
	}
	return fn(ctx)
}

func (b *fakeUpdateBackend) Relaunch() {
	b.mu.Lock()
	b.restarts++
	fn := b.relaunch
	b.mu.Unlock()
	if fn != nil {
		fn()
	}
}

func waitForStatus(t *testing.T, svc *updateService, want func(updateStatus) bool) updateStatus {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		st := svc.snapshot()
		if want(st) {
			return st
		}
		time.Sleep(10 * time.Millisecond)
	}
	st := svc.snapshot()
	t.Fatalf("timed out waiting for update status; last=%+v", st)
	return st
}

func TestUpdateServiceCheckDownloadAndRelaunch(t *testing.T) {
	dir := t.TempDir()
	rel := &fakeUpdateRelease{
		version: "0.2.0",
		notes:   "### Highlights\n- Faster timeline\n- Auto updates",
		date:    time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC),
	}
	backend := &fakeUpdateBackend{
		enabled: true,
		current: "0.1.2",
		checkFn: func(ctx context.Context) (updateRelease, error) { return rel, nil },
	}
	svc := newUpdateService(dir, backend, nil)
	svc.firstCheckWait = 5 * time.Millisecond
	svc.start()
	defer svc.stop()

	st := waitForStatus(t, svc, func(s updateStatus) bool { return s.Phase == updateAvailable })
	if st.Version != "0.2.0" || st.Notes != rel.notes || st.LastCheck.IsZero() {
		t.Fatalf("unexpected available status: %+v", st)
	}

	svc.startDownload()
	st = waitForStatus(t, svc, func(s updateStatus) bool { return s.Phase == updateReady })
	if st.Downloaded != 1024*1024 || st.Total != 1024*1024 {
		t.Fatalf("expected full download progress, got %d/%d", st.Downloaded, st.Total)
	}

	svc.relaunch()
	backend.mu.Lock()
	restarts := backend.restarts
	backend.mu.Unlock()
	if restarts != 1 {
		t.Fatalf("expected 1 relaunch, got %d", restarts)
	}

	prefs, err := readUpdatePrefs(dir)
	if err != nil || prefs.LastCheck.IsZero() || !prefs.autoCheckEnabled() {
		t.Fatalf("unexpected persisted prefs: %+v, err=%v", prefs, err)
	}
}

func TestUpdateServiceSkipVersionAndAutoDownload(t *testing.T) {
	dir := t.TempDir()
	var currentRel updateRelease = &fakeUpdateRelease{version: "0.2.0", notes: "v0.2.0 notes"}
	var relMu sync.Mutex
	backend := &fakeUpdateBackend{
		enabled: true,
		current: "0.1.2",
		checkFn: func(ctx context.Context) (updateRelease, error) {
			relMu.Lock()
			defer relMu.Unlock()
			return currentRel, nil
		},
	}
	svc := newUpdateService(dir, backend, nil)
	svc.firstCheckWait = time.Hour // drive checks explicitly
	svc.start()
	defer svc.stop()
	waitForStatus(t, svc, func(s updateStatus) bool { return s.Loaded })

	svc.checkNow(false)
	waitForStatus(t, svc, func(s updateStatus) bool { return s.Phase == updateAvailable && s.Version == "0.2.0" })

	svc.skipCurrent()
	st := waitForStatus(t, svc, func(s updateStatus) bool { return s.Phase == updateIdle && s.Skipped == "0.2.0" })
	if !st.BannerDismiss {
		t.Fatal("skipping should dismiss banner")
	}

	// Background check for the skipped version stays silent.
	svc.checkNow(false)
	time.Sleep(40 * time.Millisecond)
	if phase := svc.snapshot().Phase; phase != updateIdle {
		t.Fatalf("background check resurfaced skipped version: phase=%v", phase)
	}

	// Manual check still reveals the skipped version.
	svc.checkNow(true)
	waitForStatus(t, svc, func(s updateStatus) bool { return s.Phase == updateAvailable && s.Version == "0.2.0" })

	// Enable auto-download and ship v0.3.0 -> background check downloads automatically.
	svc.setAutoDownload(true)
	relMu.Lock()
	currentRel = &fakeUpdateRelease{version: "0.3.0", notes: "v0.3.0 notes"}
	relMu.Unlock()
	svc.checkNow(false)
	st = waitForStatus(t, svc, func(s updateStatus) bool { return s.Phase == updateReady && s.Version == "0.3.0" })
	if st.Skipped != "" {
		t.Fatalf("newer version should clear skipped version, got %q", st.Skipped)
	}

	prefs, err := readUpdatePrefs(dir)
	if err != nil || !prefs.AutoDownload || prefs.Skipped != "" {
		t.Fatalf("unexpected prefs after auto-download: %+v, err=%v", prefs, err)
	}
}

func TestUpdateServiceCancelErrorAndDisabledBuild(t *testing.T) {
	dir := t.TempDir()
	started := make(chan struct{})
	rel := &fakeUpdateRelease{
		version: "0.2.0",
		install: func(ctx context.Context, progress func(downloaded, total int64)) error {
			progress(200, 1000)
			close(started)
			<-ctx.Done()
			return ctx.Err()
		},
	}
	backend := &fakeUpdateBackend{
		enabled: true,
		current: "0.1.2",
		checkFn: func(ctx context.Context) (updateRelease, error) { return rel, nil },
	}
	svc := newUpdateService(dir, backend, nil)
	svc.firstCheckWait = time.Hour
	svc.start()
	defer svc.stop()
	waitForStatus(t, svc, func(s updateStatus) bool { return s.Loaded })

	svc.checkNow(true)
	waitForStatus(t, svc, func(s updateStatus) bool { return s.Phase == updateAvailable })

	svc.startDownload()
	<-started
	svc.cancelDownload()
	st := waitForStatus(t, svc, func(s updateStatus) bool { return s.Phase == updateAvailable && s.Downloaded == 0 })
	if st.Err != "" {
		t.Fatalf("cancel should not set error, got %q", st.Err)
	}

	// Disabled build never runs background checks and reports updates_disabled manually.
	disabledBackend := &fakeUpdateBackend{
		enabled: false,
		current: "0.1.2",
		checkFn: func(ctx context.Context) (updateRelease, error) { return nil, errors.New("should not run") },
	}
	disabledSvc := newUpdateService(t.TempDir(), disabledBackend, nil)
	disabledSvc.firstCheckWait = 5 * time.Millisecond
	disabledSvc.start()
	defer disabledSvc.stop()
	waitForStatus(t, disabledSvc, func(s updateStatus) bool { return s.Loaded })
	time.Sleep(30 * time.Millisecond)
	disabledBackend.mu.Lock()
	checks := disabledBackend.checks
	disabledBackend.mu.Unlock()
	if checks != 0 {
		t.Fatalf("disabled build ran %d background checks", checks)
	}
	disabledSvc.checkNow(true)
	st = waitForStatus(t, disabledSvc, func(s updateStatus) bool { return s.Phase == updateError && s.Err == "updates_disabled" })
	if st.Enabled {
		t.Fatal("disabled build reported Enabled=true")
	}
}

func TestDesktopRelaunchFlushesDirtyDraftBeforeRestart(t *testing.T) {
	d, a, _, stores := managedTestDesktop(t)
	a.window = &mygo.Window{}
	defer func() { a.window = nil }()

	quitCalls, relaunchCalls := 0, 0
	d.quitAction = func() { quitCalls++ }
	d.relaunchAction = func() {
		relaunchCalls++
		// Simulate mygo.App.Relaunch() firing OnBeforeQuit -> d.requestQuit().
		d.requestQuit()
	}

	// Create a dirty draft in the active window so the first quit attempt
	// defers while saving.
	id := a.filtered()[0].ID
	a.openIssue(id)
	draft := a.drafts[id]
	draft.issue.Title = "Unsaved title before update restart"
	a.touch(draft, time.Now())

	d.requestRelaunch()
	if relaunchCalls != 1 || d.committingQuit || quitCalls != 0 {
		t.Fatalf("expected deferred relaunch while flushing draft: relaunch=%d commit=%v quit=%d", relaunchCalls, d.committingQuit, quitCalls)
	}

	drainDesktop(t, d, stores)
	if relaunchCalls != 2 || quitCalls != 0 || !d.committingQuit {
		t.Fatalf("expected relaunch after draft flush (not plain quit): relaunch=%d quit=%d commit=%v", relaunchCalls, quitCalls, d.committingQuit)
	}
	fresh, _, err := stores[0].Refresh()
	if err != nil || fresh.Issues[id].Title != "Unsaved title before update restart" {
		t.Fatalf("draft lost during update relaunch: %v %q", err, fresh.Issues[id].Title)
	}
}

func TestUpdateUIEndToEnd(t *testing.T) {
	d, a, _, _ := managedTestDesktop(t)
	releaseFinish := make(chan struct{})
	rel := &fakeUpdateRelease{
		version: "0.2.0",
		notes:   "## What's New\n- In-app Lobe update UI\n- Signed delta downloads",
		date:    time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC),
		install: func(ctx context.Context, progress func(downloaded, total int64)) error {
			progress(4*1024*1024, 10*1024*1024)
			<-releaseFinish
			progress(10*1024*1024, 10*1024*1024)
			return nil
		},
	}
	checkReady := make(chan struct{})
	backend := &fakeUpdateBackend{
		enabled: true,
		current: "0.1.2",
		checkFn: func(ctx context.Context) (updateRelease, error) {
			<-checkReady
			return rel, nil
		},
	}
	svc := newUpdateService(d.dataDir, backend, nil)
	svc.firstCheckWait = time.Hour
	d.updater = svc
	svc.start()
	defer svc.stop()
	waitForStatus(t, svc, func(s updateStatus) bool { return s.Loaded })

	tt := ui.NewTester(a.view, 1280, 820)
	// Menu item starts checking quietly without popping a modal while checking.
	var checkMenu *mygo.MenuItem
	for _, top := range a.desktopMenuItems() {
		for _, item := range top.Submenu {
			if item.ID == "check-updates" {
				checkMenu = item
			}
		}
	}
	if checkMenu == nil {
		t.Fatal("missing check-updates menu item")
	}
	checkMenu.Click(nil, nil)
	tt.Frame()
	if a.appUpdateOpen {
		t.Fatal("update dialog should not open before check finds an update")
	}
	close(checkReady)
	waitForStatus(t, svc, func(s updateStatus) bool { return s.Phase == updateAvailable })
	tt.Frame()
	if !a.appUpdateOpen {
		t.Fatal("update dialog did not open after check found a new version")
	}
	if !a.updateHasBadge() {
		t.Fatal("expected settings badge when update is available")
	}

	// Dismiss modal, verify banner appears, then reopen via banner button.
	click(t, tt, "Remind me later")
	if a.appUpdateOpen {
		t.Fatal("Remind me later did not close dialog")
	}
	tt.Frame()
	click(t, tt, "View details")
	if !a.appUpdateOpen {
		t.Fatal("banner View details did not reopen dialog")
	}
	tt.Frame()

	// Start download from the dialog, verify progress frame, then finish.
	click(t, tt, "Download & install")
	waitForStatus(t, svc, func(s updateStatus) bool { return s.Phase == updateDownloading && s.Downloaded > 0 })
	tt.Frame()
	tt.Advance(200 * time.Millisecond)

	close(releaseFinish)
	waitForStatus(t, svc, func(s updateStatus) bool { return s.Phase == updateReady })
	tt.Frame()

	// Menu label switches to Restart to Update when ready.
	var readyMenu *mygo.MenuItem
	for _, top := range a.desktopMenuItems() {
		for _, item := range top.Submenu {
			if item.ID == "check-updates" {
				readyMenu = item
			}
		}
	}
	if readyMenu == nil || readyMenu.Label != "Restart to Update to v0.2.0" {
		t.Fatalf("expected ready menu label, got %+v", readyMenu)
	}

	click(t, tt, "Restart & update")
	backend.mu.Lock()
	restarts := backend.restarts
	backend.mu.Unlock()
	if restarts != 1 {
		t.Fatalf("Restart & update did not trigger relaunch (got %d)", restarts)
	}

	// Switch to Settings view in Chinese and toggle update mode segmented control.
	a.appUpdateOpen = false
	a.data.Preferences.Locale = "zh-Hans"
	a.navigate("settings", "")
	tt.Frame()
	click(t, tt, "自动下载")
	waitForStatus(t, svc, func(s updateStatus) bool { return s.AutoCheck && s.AutoDownload })
	click(t, tt, "关闭")
	waitForStatus(t, svc, func(s updateStatus) bool { return !s.AutoCheck && !s.AutoDownload })
	tt.Frame()
}

func TestUpdateCheckUpToDateDoesNotOpenDialog(t *testing.T) {
	d, a, _, _ := managedTestDesktop(t)
	backend := &fakeUpdateBackend{
		enabled: true,
		current: "0.1.3",
		checkFn: func(ctx context.Context) (updateRelease, error) { return nil, nil },
	}
	svc := newUpdateService(d.dataDir, backend, nil)
	svc.firstCheckWait = time.Hour
	d.updater = svc
	svc.start()
	defer svc.stop()
	waitForStatus(t, svc, func(s updateStatus) bool { return s.Loaded })

	a.data.Preferences.Locale = "zh-Hans"
	a.navigate("settings", "")
	tt := ui.NewTester(a.view, 1280, 820)
	tt.Frame()

	click(t, tt, "检查更新")
	waitForStatus(t, svc, func(s updateStatus) bool { return s.Phase == updateUpToDate })
	tt.Frame()
	if a.appUpdateOpen {
		t.Fatal("up-to-date check should not open update dialog")
	}
}
