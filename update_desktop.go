package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/egoist/mygo"
)

const (
	appFallbackVersion   = "0.1.4"
	updateFirstCheckWait = 10 * time.Second
	updateCheckInterval  = 24 * time.Hour
	updateRetryInterval  = time.Hour
	updatePrefsFile      = "updater.json"
)

// flowVersion returns the running app version, falling back to the build's
// declared version when running unpackaged (such as under go test or go run).
func flowVersion() string {
	if v := mygo.App.Version(); v != "" {
		return v
	}
	return appFallbackVersion
}

type updatePhase int

const (
	updateIdle updatePhase = iota
	updateChecking
	updateUpToDate
	updateAvailable
	updateDownloading
	updateReady
	updateError
)

type updatePrefs struct {
	AutoCheck    *bool     `json:"autoCheck,omitempty"`
	AutoDownload bool      `json:"autoDownload,omitempty"`
	Skipped      string    `json:"skipped,omitempty"`
	LastCheck    time.Time `json:"lastCheck,omitempty"`
}

func (p updatePrefs) autoCheckEnabled() bool {
	if p.AutoCheck == nil {
		return true
	}
	return *p.AutoCheck
}

func readUpdatePrefs(dir string) (updatePrefs, error) {
	if dir == "" {
		return updatePrefs{}, nil
	}
	f, err := os.Open(filepath.Join(dir, updatePrefsFile))
	if errors.Is(err, os.ErrNotExist) {
		return updatePrefs{}, nil
	}
	if err != nil {
		return updatePrefs{}, err
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, 1<<20+1))
	if err != nil {
		return updatePrefs{}, err
	}
	var p updatePrefs
	if len(raw) > 1<<20 || json.Unmarshal(raw, &p) != nil {
		return updatePrefs{}, errors.New("invalid_updater_prefs")
	}
	return p, nil
}

func writeUpdatePrefs(dir string, p updatePrefs) error {
	if dir == "" {
		return nil
	}
	raw, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return writeDesktopFile(dir, updatePrefsFile, append(raw, '\n'))
}

// updateRelease abstracts a found update so tests can drive progress,
// cancellation and errors without real archives or network access.
type updateRelease interface {
	Version() string
	Notes() string
	Date() time.Time
	Install(ctx context.Context, progress func(downloaded, total int64)) error
}

// updateBackend abstracts mygo.Updater and app relaunch.
type updateBackend interface {
	Enabled() bool
	CurrentVersion() string
	Check(ctx context.Context) (updateRelease, error)
	Relaunch()
}

type mygoUpdateRelease struct{ up *mygo.Update }

func (r mygoUpdateRelease) Version() string { return r.up.Version }
func (r mygoUpdateRelease) Notes() string   { return r.up.Notes }
func (r mygoUpdateRelease) Date() time.Time { return r.up.Date }
func (r mygoUpdateRelease) Install(ctx context.Context, progress func(downloaded, total int64)) error {
	return r.up.Install(ctx, progress)
}

type mygoUpdateBackend struct{ desktop *desktop }

func (b mygoUpdateBackend) Enabled() bool          { return mygo.Updater.Enabled() }
func (b mygoUpdateBackend) CurrentVersion() string { return flowVersion() }
func (b mygoUpdateBackend) Check(ctx context.Context) (updateRelease, error) {
	up, err := mygo.Updater.Check(ctx)
	if err != nil || up == nil {
		return nil, err
	}
	return mygoUpdateRelease{up: up}, nil
}
func (b mygoUpdateBackend) Relaunch() {
	if b.desktop != nil {
		b.desktop.requestRelaunch()
		return
	}
	mygo.App.Relaunch()
}

// updateStatus is the main-thread snapshot read by views, settings and menus.
type updateStatus struct {
	Loaded        bool
	Enabled       bool
	Current       string
	AutoCheck     bool
	AutoDownload  bool
	LastCheck     time.Time
	Skipped       string
	Phase         updatePhase
	Version       string
	Notes         string
	Date          time.Time
	Downloaded    int64
	Total         int64
	Err           string
	UserInitiated bool
	BannerDismiss bool
	ResultSeq     uint64
}

// updateService coordinates automatic and manual update checks, downloads and
// installs off the UI thread, publishing snapshots onto the main thread.
type updateService struct {
	backend        updateBackend
	dir            string
	publish        func(func())
	now            func() time.Time
	firstCheckWait time.Duration
	interval       time.Duration
	retryInterval  time.Duration

	// Snapshot read by UI views/menus; updated via publish under statusMu.
	statusMu sync.RWMutex
	status   updateStatus

	// Worker-side coordination state protected by mu.
	// Never call publish while holding mu.
	mu            sync.Mutex
	prefs         updatePrefs
	rel           updateRelease
	opCancel      context.CancelFunc
	gen           uint64
	busy          bool
	readyVersion  string
	lastFailed    time.Time
	launchChecked bool
	started       bool
	stopped       bool
	reschedule    chan struct{}
	stopCh        chan struct{}
	wg            sync.WaitGroup
}

func (s *updateService) snapshot() updateStatus {
	s.statusMu.RLock()
	defer s.statusMu.RUnlock()
	return s.status
}

func (s *updateService) updateStatus(fn func(*updateStatus)) {
	s.publish(func() {
		s.statusMu.Lock()
		fn(&s.status)
		s.statusMu.Unlock()
	})
}

func newUpdateService(dir string, backend updateBackend, publish func(func())) *updateService {
	if publish == nil {
		publish = func(fn func()) { fn() }
	}
	return &updateService{
		backend:        backend,
		dir:            dir,
		publish:        publish,
		now:            time.Now,
		firstCheckWait: updateFirstCheckWait,
		interval:       updateCheckInterval,
		retryInterval:  updateRetryInterval,
		reschedule:     make(chan struct{}, 1),
		stopCh:         make(chan struct{}),
		status: updateStatus{
			Enabled:   backend.Enabled(),
			Current:   backend.CurrentVersion(),
			AutoCheck: true,
		},
	}
}

// startUpdater starts background update checks once the windows have loaded.
func (d *desktop) startUpdater() {
	main := d.find("main")
	if d.updater != nil || main == nil || main.window == nil {
		return
	}
	win := main.window
	svc := newUpdateService(d.dataDir, mygoUpdateBackend{desktop: d}, func(fn func()) {
		win.Update(func() {
			fn()
			d.invalidate()
		})
	})
	d.updater = svc
	mygo.App.OnQuit(func() { svc.stop() })
	svc.start()
}

func (s *updateService) start() {
	s.mu.Lock()
	if s.started || s.stopped {
		s.mu.Unlock()
		return
	}
	s.started = true
	s.wg.Add(1)
	s.mu.Unlock()
	go s.run()
}

func (s *updateService) stop() {
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return
	}
	s.stopped = true
	cancel := s.opCancel
	s.opCancel = nil
	close(s.stopCh)
	s.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
	}
}

func (s *updateService) signalSchedule() {
	select {
	case s.reschedule <- struct{}{}:
	default:
	}
}

func (s *updateService) run() {
	defer s.wg.Done()
	prefs, _ := readUpdatePrefs(s.dir)
	enabled := s.backend.Enabled()
	current := s.backend.CurrentVersion()

	s.mu.Lock()
	s.prefs = prefs
	stopped := s.stopped
	s.mu.Unlock()
	if stopped {
		return
	}

	s.updateStatus(func(st *updateStatus) {
		st.Loaded = true
		st.Enabled = enabled
		st.Current = current
		st.AutoCheck = prefs.autoCheckEnabled()
		st.AutoDownload = prefs.AutoDownload
		st.LastCheck = prefs.LastCheck
		st.Skipped = prefs.Skipped
	})

	var timer *time.Timer
	stopTimer := func() {
		if timer != nil {
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer = nil
		}
	}
	defer stopTimer()

	arm := func() <-chan time.Time {
		stopTimer()
		s.mu.Lock()
		wait, ok := s.nextWaitLocked(enabled)
		s.mu.Unlock()
		if !ok {
			return nil
		}
		timer = time.NewTimer(wait)
		return timer.C
	}

	tick := arm()
	for {
		select {
		case <-s.stopCh:
			return
		case <-s.reschedule:
			tick = arm()
		case <-tick:
			tick = nil
			s.beginCheck(false)
			tick = arm()
		}
	}
}

func (s *updateService) nextWaitLocked(enabled bool) (time.Duration, bool) {
	if !enabled || !s.prefs.autoCheckEnabled() || s.readyVersion != "" {
		return 0, false
	}
	if !s.launchChecked {
		return max(0, s.firstCheckWait), true
	}
	now := s.now()
	if !s.lastFailed.IsZero() && s.lastFailed.After(s.prefs.LastCheck) {
		wait := s.retryInterval - now.Sub(s.lastFailed)
		if wait < time.Second {
			wait = time.Second
		}
		return wait, true
	}
	if s.prefs.LastCheck.IsZero() {
		return s.interval, true
	}
	wait := s.interval - now.Sub(s.prefs.LastCheck)
	if wait < time.Second {
		wait = time.Second
	}
	return wait, true
}

// checkNow starts an update check. When user is true, errors, up-to-date
// results and skipped versions are shown to the user.
func (s *updateService) checkNow(user bool) {
	s.beginCheck(user)
}

func (s *updateService) beginCheck(user bool) {
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return
	}
	enabled := s.backend.Enabled()
	if !enabled {
		s.mu.Unlock()
		if user {
			s.updateStatus(func(st *updateStatus) {
				st.Enabled = false
				st.Phase = updateError
				st.Err = "updates_disabled"
				st.UserInitiated = true
				st.ResultSeq++
			})
		}
		return
	}
	if s.readyVersion != "" {
		ver := s.readyVersion
		s.mu.Unlock()
		if user {
			s.updateStatus(func(st *updateStatus) {
				st.Phase = updateReady
				st.Version = ver
				st.UserInitiated = true
				st.BannerDismiss = false
				st.ResultSeq++
			})
		}
		return
	}
	if s.busy {
		s.mu.Unlock()
		if user {
			s.updateStatus(func(st *updateStatus) {
				st.UserInitiated = true
			})
		}
		return
	}
	s.gen++
	myGen := s.gen
	s.busy = true
	s.launchChecked = true
	ctx, cancel := context.WithCancel(context.Background())
	s.opCancel = cancel
	s.wg.Add(1)
	s.mu.Unlock()

	s.updateStatus(func(st *updateStatus) {
		st.Enabled = true
		st.Phase = updateChecking
		st.Err = ""
		st.UserInitiated = user
		st.Downloaded = 0
		st.Total = 0
	})

	go func() {
		defer s.wg.Done()
		defer cancel()
		rel, err := s.backend.Check(ctx)

		s.mu.Lock()
		if s.stopped || s.gen != myGen {
			s.mu.Unlock()
			return
		}
		s.busy = false
		s.opCancel = nil

		if err != nil {
			if errors.Is(err, context.Canceled) {
				s.mu.Unlock()
				s.updateStatus(func(st *updateStatus) {
					st.Phase = updateIdle
					st.Err = ""
				})
				return
			}
			if errors.Is(err, mygo.ErrUpdatesDisabled) {
				s.mu.Unlock()
				if user {
					s.updateStatus(func(st *updateStatus) {
						st.Enabled = false
						st.Phase = updateError
						st.Err = "updates_disabled"
						st.UserInitiated = true
						st.ResultSeq++
					})
				}
				return
			}
			s.lastFailed = s.now()
			s.mu.Unlock()
			s.signalSchedule()
			if user {
				msg := err.Error()
				s.updateStatus(func(st *updateStatus) {
					st.Phase = updateError
					st.Err = msg
					st.UserInitiated = true
					st.ResultSeq++
				})
			} else {
				s.updateStatus(func(st *updateStatus) {
					st.Phase = updateIdle
					st.Err = ""
				})
			}
			return
		}

		checkedAt := s.now()
		s.prefs.LastCheck = checkedAt
		if rel == nil {
			s.rel = nil
			prefs := s.prefs
			s.mu.Unlock()
			_ = writeUpdatePrefs(s.dir, prefs)
			s.signalSchedule()
			s.updateStatus(func(st *updateStatus) {
				st.LastCheck = checkedAt
				st.Phase = updateUpToDate
				st.Version = ""
				st.Notes = ""
				st.Date = time.Time{}
				st.Err = ""
				st.BannerDismiss = false
				st.ResultSeq++
			})
			return
		}

		ver, notes, date := rel.Version(), rel.Notes(), rel.Date()
		s.rel = rel
		if !user && s.prefs.Skipped == ver {
			prefs := s.prefs
			s.mu.Unlock()
			_ = writeUpdatePrefs(s.dir, prefs)
			s.signalSchedule()
			s.updateStatus(func(st *updateStatus) {
				st.LastCheck = checkedAt
				st.Phase = updateIdle
				st.Err = ""
			})
			return
		}
		if s.prefs.Skipped != "" && s.prefs.Skipped != ver {
			s.prefs.Skipped = ""
		}
		skipped := s.prefs.Skipped
		autoDL := !user && s.prefs.AutoDownload
		prefs := s.prefs
		s.mu.Unlock()

		_ = writeUpdatePrefs(s.dir, prefs)
		s.signalSchedule()
		s.updateStatus(func(st *updateStatus) {
			st.LastCheck = checkedAt
			st.Skipped = skipped
			st.Phase = updateAvailable
			st.Version = ver
			st.Notes = notes
			st.Date = date
			st.Downloaded = 0
			st.Total = 0
			st.Err = ""
			st.BannerDismiss = false
			st.ResultSeq++
		})
		if autoDL {
			s.startDownload()
		}
	}()
}

// startDownload downloads, verifies and stages the currently available update.
func (s *updateService) startDownload() {
	s.mu.Lock()
	if s.stopped || s.busy || s.rel == nil || s.readyVersion != "" {
		s.mu.Unlock()
		return
	}
	s.gen++
	myGen := s.gen
	s.busy = true
	rel := s.rel
	ver := rel.Version()
	ctx, cancel := context.WithCancel(context.Background())
	s.opCancel = cancel
	s.wg.Add(1)
	s.mu.Unlock()

	s.updateStatus(func(st *updateStatus) {
		st.Phase = updateDownloading
		st.Version = ver
		st.Downloaded = 0
		st.Total = 0
		st.Err = ""
		st.BannerDismiss = false
	})

	go func() {
		defer s.wg.Done()
		defer cancel()
		err := rel.Install(ctx, func(downloaded, total int64) {
			s.mu.Lock()
			active := !s.stopped && s.gen == myGen
			s.mu.Unlock()
			if !active {
				return
			}
			s.updateStatus(func(st *updateStatus) {
				if st.Phase == updateDownloading {
					st.Downloaded = downloaded
					st.Total = total
				}
			})
		})

		s.mu.Lock()
		if s.stopped || s.gen != myGen {
			s.mu.Unlock()
			return
		}
		s.busy = false
		s.opCancel = nil
		if err == nil {
			s.readyVersion = ver
			s.mu.Unlock()
			s.signalSchedule()
			s.updateStatus(func(st *updateStatus) {
				st.Phase = updateReady
				st.Version = ver
				if st.Total > 0 {
					st.Downloaded = st.Total
				}
				st.Err = ""
				st.BannerDismiss = false
			})
			return
		}
		s.mu.Unlock()
		if errors.Is(err, context.Canceled) {
			s.updateStatus(func(st *updateStatus) {
				st.Phase = updateAvailable
				st.Downloaded = 0
				st.Total = 0
				st.Err = ""
			})
			return
		}
		msg := err.Error()
		s.updateStatus(func(st *updateStatus) {
			st.Phase = updateError
			st.Err = msg
		})
	}()
}

// cancelDownload aborts an in-flight download and returns to updateAvailable.
func (s *updateService) cancelDownload() {
	s.mu.Lock()
	cancel := s.opCancel
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// skipCurrent skips the currently offered version for future background checks.
func (s *updateService) skipCurrent() {
	s.mu.Lock()
	if s.busy || s.rel == nil || s.readyVersion != "" {
		s.mu.Unlock()
		return
	}
	ver := s.rel.Version()
	s.prefs.Skipped = ver
	s.rel = nil
	prefs := s.prefs
	s.mu.Unlock()

	_ = writeUpdatePrefs(s.dir, prefs)
	s.updateStatus(func(st *updateStatus) {
		st.Skipped = ver
		st.Phase = updateIdle
		st.Version = ""
		st.Notes = ""
		st.Date = time.Time{}
		st.Err = ""
		st.BannerDismiss = true
	})
}

// dismissBanner hides the in-app update banner for this session without
// skipping the version.
func (s *updateService) dismissBanner() {
	s.updateStatus(func(st *updateStatus) {
		st.BannerDismiss = true
	})
}

func (s *updateService) setAutoCheck(on bool) {
	s.mu.Lock()
	v := on
	s.prefs.AutoCheck = &v
	prefs := s.prefs
	s.mu.Unlock()

	_ = writeUpdatePrefs(s.dir, prefs)
	s.signalSchedule()
	s.updateStatus(func(st *updateStatus) {
		st.AutoCheck = on
	})
}

func (s *updateService) setAutoDownload(on bool) {
	s.mu.Lock()
	s.prefs.AutoDownload = on
	prefs := s.prefs
	s.mu.Unlock()

	_ = writeUpdatePrefs(s.dir, prefs)
	s.updateStatus(func(st *updateStatus) {
		st.AutoDownload = on
	})
}

// relaunch restarts Flow into the staged update.
func (s *updateService) relaunch() {
	s.mu.Lock()
	ready := s.readyVersion != ""
	s.mu.Unlock()
	if !ready {
		return
	}
	s.backend.Relaunch()
}
