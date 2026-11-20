package main

import (
	"errors"
	"flow/internal/core"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
)

// syncService runs the sync engine of the app's workspace on a goroutine of
// its own, so that a slow iCloud never holds up a window. Its status is read
// and its requests are made on the main thread.
type syncService struct {
	requests chan syncRequest
	stop     chan struct{}
	done     chan struct{}
	status   syncStatus
}

type syncStatus struct {
	Loaded, Enabled, Busy bool
	Folder, Device, Err   string
	Joined                string // the folder this Mac already took the other Macs' work from
	At                    time.Time
	Devices               []string
	Waiting               int // other Macs whose changes wait for attached files to download
}

func (s syncStatus) key() string {
	return strings.Join([]string{strconv.FormatBool(s.Loaded), strconv.FormatBool(s.Enabled), strconv.FormatBool(s.Busy), s.Folder, s.Err, s.At.Format("15:04"), strconv.Itoa(len(s.Devices)), strconv.Itoa(s.Waiting)}, "|")
}

type syncRequest struct {
	enable, disable bool
	folder          string
	adopt           bool
	reply           func(error) // on the main thread
}

// startSync starts syncing once the windows have loaded the workspace.
func (d *desktop) startSync() {
	main := d.find("main")
	if d.sync != nil || d.dataDir == "" || d.disabled || main == nil || main.window == nil {
		return
	}
	s := &syncService{requests: make(chan syncRequest, 4), stop: make(chan struct{}), done: make(chan struct{})}
	d.sync = s
	win, dir := main.window, d.dataDir
	// Status arrives every few seconds; windows redraw only when what they
	// show of it changed.
	update := func(fn func()) {
		win.Update(func() {
			before := s.status.key()
			fn()
			if s.status.key() != before || len(s.requests) > 0 {
				d.invalidate()
			}
		})
	}
	mygo.App.OnQuit(func() {
		close(s.stop)
		select {
		case <-s.done:
		case <-time.After(5 * time.Second):
		}
	})
	go s.run(dir, update, func(names []string) {
		if a := d.active; a != nil && len(names) > 0 {
			a.notice = a.tr("与 ", "Some changes from ") + names[0] + a.tr(" 的部分修改无法合并，已存入 backups", " could not be merged and were saved to backups")
		}
	})
}

func (s *syncService) request(r syncRequest) {
	s.status.Busy = true
	s.requests <- r
}

func (s *syncService) run(dir string, update func(func()), conflicted func([]string)) {
	defer close(s.done)
	st, err := readSyncState(dir)
	var e *syncEngine
	publish := func(rep syncReport, err error, busy bool) {
		current := st
		if e != nil {
			current = e.state
		}
		status := syncStatus{Loaded: true, Enabled: current.Enabled, Folder: current.Folder, Device: current.Device, Busy: busy, Devices: rep.Devices, Waiting: len(rep.Waiting)}
		if len(current.Peers) > 0 {
			status.Joined = current.Folder
		}
		if current.Synced != 0 {
			status.At = time.UnixMilli(current.Synced)
		}
		if err != nil {
			status.Err = err.Error()
		}
		update(func() {
			if s.status.Busy && !busy && len(s.requests) > 0 {
				status.Busy = true // another request is waiting
			}
			if err == nil && len(status.Devices) == 0 && len(s.status.Devices) > 0 && status.Enabled {
				status.Devices = s.status.Devices
			}
			s.status = status
			if len(rep.Conflicts) > 0 {
				conflicted(rep.Conflicts)
			}
		})
	}
	open := func() error {
		if e != nil {
			return nil
		}
		eng, err := openSyncEngine(dir, st)
		if err == nil {
			e = eng
		}
		return err
	}
	if err == nil && st.Enabled {
		err = open()
	}
	if err == nil && e != nil {
		rep, cycleErr := e.cycle(true)
		publish(rep, cycleErr, false)
	} else {
		publish(syncReport{}, err, false)
	}
	ticker := time.NewTicker(syncTick)
	defer ticker.Stop()
	for {
		select {
		case <-s.stop:
			if e != nil {
				if e.state.Enabled {
					e.cycle(true)
				}
				e.close()
			}
			return
		case r := <-s.requests:
			var rep syncReport
			err := s.handle(dir, &st, &e, open, r, &rep)
			publish(rep, err, false)
			if r.reply != nil {
				update(func() { r.reply(err) })
			}
		case <-ticker.C:
			if e != nil && e.state.Enabled {
				rep, err := e.cycle(false)
				publish(rep, err, false)
			}
		}
	}
}

func (s *syncService) handle(dir string, st *syncState, e **syncEngine, open func() error, r syncRequest, rep *syncReport) error {
	if r.disable {
		st.Enabled = false
		if *e != nil {
			(*e).state.Enabled = false
			*st = (*e).state
			(*e).close()
			*e = nil
		}
		return writeSyncState(dir, *st)
	}
	if !r.enable {
		return nil
	}
	if err := os.MkdirAll(filepath.Join(r.folder, "Devices"), 0755); err != nil {
		return err
	}
	fresh := st.Folder != r.folder || len(st.Peers) == 0
	st.Enabled, st.Folder = true, r.folder
	if fresh {
		st.Peers, st.Pushed = nil, ""
	}
	if *e != nil {
		(*e).state.Enabled, (*e).state.Folder = true, r.folder
		if fresh {
			(*e).state.Peers, (*e).state.Pushed = nil, ""
		}
	} else if err := open(); err != nil {
		st.Enabled = false
		return err
	}
	if fresh {
		if err := (*e).join(r.adopt); err != nil {
			(*e).state.Enabled = false
			*st = (*e).state
			writeSyncState(dir, *st)
			return err
		}
	}
	var err error
	*rep, err = (*e).cycle(true)
	*st = (*e).state
	return err
}

// syncStatusText is the line under the iCloud switch, and whether it
// reports a problem.
func (a *app) syncStatusText(s syncStatus) (string, bool) {
	switch {
	case s.Busy:
		return a.tr("正在同步…", "Syncing…"), false
	case !s.Enabled:
		return "", false
	case s.Err != "":
		return a.syncError(s.Err), true
	case s.At.IsZero():
		return a.tr("正在同步…", "Syncing…"), false
	case s.Waiting > 0:
		return a.tr("正在从 iCloud 下载附件…", "Downloading attachments from iCloud…"), false
	}
	text := a.tr("已同步 · ", "Synced · ") + s.At.Format("15:04")
	switch n := len(s.Devices); {
	case n == 1:
		text += a.tr(" · 另有 1 台 Mac", " · 1 other Mac")
	case n > 1:
		text += a.tr(" · 另有 "+strconv.Itoa(n)+" 台 Mac", " · "+strconv.Itoa(n)+" other Macs")
	}
	return text, false
}

func (a *app) syncError(err string) string {
	lower := strings.ToLower(err)
	switch {
	case strings.Contains(lower, "no such file"):
		return a.tr("找不到同步文件夹", "Sync folder not found")
	case strings.Contains(lower, "operation not permitted"), strings.Contains(lower, "permission denied"):
		return a.tr("没有权限访问同步文件夹", "No permission to use the sync folder")
	}
	return err
}

// enableSync turns on syncing through folder, iCloud Drive's Flow folder
// when it is "". When other Macs already sync there and this one has a
// workspace of its own, it asks whether to merge the two or take theirs.
func (a *app) enableSync(folder string) {
	if a.window == nil || a.desktop == nil || a.desktop.sync == nil || a.desktop.sync.status.Busy {
		return
	}
	w, sync := a.window, a.desktop.sync
	hasWork := len(a.data.Issues) > 0 || len(a.data.Projects) > 0
	device, joined := sync.status.Device, sync.status.Joined
	tr := a.tr
	go func() {
		if folder == "" {
			drive := iCloudDrive()
			if drive == "" {
				mygo.Dialog.Message(mygo.MessageOptions{Parent: w, Type: mygo.MessageWarning, Message: tr("这台 Mac 没有打开 iCloud 云盘", "iCloud Drive is off on this Mac"), Detail: tr("在“系统设置 › Apple 账户 › iCloud”中打开 iCloud 云盘后再试。", "Turn on iCloud Drive in System Settings › Apple Account › iCloud, then try again.")})
				return
			}
			folder = filepath.Join(drive, "Flow")
		}
		others, err := syncFolderDevices(folder, device)
		if err != nil {
			w.Update(func() { a.err = err.Error() })
			return
		}
		if folder == joined {
			others = nil // synced here before: the usual merge picks up where it left off
		}
		adopt := len(others) > 0 && !hasWork
		if len(others) > 0 && hasWork {
			result, err := mygo.Dialog.Message(mygo.MessageOptions{Parent: w, Type: mygo.MessageQuestion,
				Message:      tr("iCloud 中已有 Flow 数据", "Flow data is already in iCloud"),
				Detail:       tr("来自 ", "From ") + others[0] + tr("。合并会保留两边的内容；也可以改用 iCloud 中的数据，这台 Mac 上的数据会先备份。", ". Merging keeps both sides; or use the data in iCloud, after this Mac's is backed up."),
				Buttons:      []string{tr("取消", "Cancel"), tr("合并", "Merge"), tr("使用 iCloud 中的数据", "Use iCloud data")},
				CancelButton: 0, DefaultButton: 1})
			if err != nil || result.Button == 0 {
				return
			}
			adopt = result.Button == 2
		}
		w.Update(func() {
			sync.request(syncRequest{enable: true, folder: folder, adopt: adopt, reply: func(err error) {
				if err != nil {
					a.err = err.Error()
				}
			}})
			a.desktop.invalidate()
		})
	}()
}

func (a *app) disableSync() {
	if a.desktop == nil || a.desktop.sync == nil || a.desktop.sync.status.Busy {
		return
	}
	a.desktop.sync.request(syncRequest{disable: true})
	a.desktop.invalidate()
}

// changeDataLocation moves the workspace to a folder the user picks, or
// switches to the workspace already there, and restarts Flow in it.
func (a *app) changeDataLocation() {
	if a.window == nil || a.desktop == nil || os.Getenv("FLOW_DATA_DIR") != "" || a.workspaceBusy() || a.isRestoring() {
		return
	}
	w, current, tr := a.window, a.dataDir, a.tr
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: w, Title: tr("选择数据文件夹", "Choose a data folder"), ButtonLabel: tr("选择", "Choose"), Directory: true, CreateDirectories: true, DefaultPath: filepath.Dir(current)})
		if err != nil {
			w.Update(func() { a.err = err.Error() })
			return
		}
		if len(paths) == 0 {
			return
		}
		target, from := realPath(paths[0]), realPath(current)
		message := func(text, detail string, buttons ...string) int {
			if len(buttons) == 0 {
				buttons = []string{tr("好", "OK")}
			}
			result, err := mygo.Dialog.Message(mygo.MessageOptions{Parent: w, Type: mygo.MessageQuestion, Message: text, Detail: detail, Buttons: buttons, CancelButton: 0, DefaultButton: len(buttons) - 1})
			if err != nil {
				return 0
			}
			return result.Button
		}
		switch {
		case target == from:
			return
		case within(from, target) || within(target, from):
			message(tr("不能选这个文件夹", "This folder cannot be used"), tr("它在当前数据文件夹里，或者包含当前数据文件夹。", "It is inside the current data folder, or contains it."))
			return
		case inICloud(target):
			if message(tr("数据库不能放在 iCloud 云盘里", "The database cannot live in iCloud Drive"),
				tr("两台 Mac 同时打开时会互相覆盖、损坏数据。改用 iCloud 同步：数据库留在这台 Mac 上，各台 Mac 通过这个文件夹交换修改。", "Two Macs opening it at once would overwrite and corrupt it. Use iCloud sync instead: the database stays on this Mac, and the Macs exchange changes through this folder."),
				tr("取消", "Cancel"), tr("用这个文件夹同步", "Sync through this folder")) == 1 {
				w.Update(func() { a.enableSync(target) })
			}
			return
		}
		_, statErr := os.Stat(filepath.Join(target, "flow.db"))
		existing := statErr == nil
		if existing {
			if message(tr("这个文件夹里已有 Flow 数据", "This folder already has Flow data"), tr("Flow 将改用其中的数据并重新启动，当前数据仍留在原位置。", "Flow will switch to it and restart; the current data stays where it is."), tr("取消", "Cancel"), tr("切换并重新启动", "Switch and Restart")) != 1 {
				return
			}
		} else if message(tr("把数据移到这个文件夹？", "Move the data to this folder?"), tr("Flow 会复制全部数据并重新启动。原位置的数据库改名为 flow.db.moved 保留。", "Flow copies all its data there and restarts. The old database is kept, renamed flow.db.moved."), tr("取消", "Cancel"), tr("移动并重新启动", "Move and Restart")) != 1 {
			return
		}
		w.Update(func() { a.relocate(target, existing) })
	}()
}

func realPath(path string) string {
	path = filepath.Clean(path)
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved
	}
	return path
}

// relocate points Flow at target, moving the workspace there first unless
// target holds one, and restarts the app.
func (a *app) relocate(target string, existing bool) {
	if a.workspaceBusy() || a.isRestoring() {
		a.notice = a.tr("正在保存，请稍后再试", "Still saving; try again in a moment")
		return
	}
	defaultDir, err := core.DefaultDataDir()
	if err != nil {
		a.err = err.Error()
		return
	}
	d, source := a.desktop, a.dataDir
	d.restoring, a.restoring = true, true
	d.invalidate()
	a.pending++
	a.jobs <- job{run: func(s *core.Store) (core.Data, error) {
		if !existing {
			if err := moveWorkspace(s, source, target); err != nil {
				return s.Snapshot(), err
			}
		}
		if err := core.SetDataLocation(defaultDir, target); err != nil {
			return s.Snapshot(), err
		}
		if !existing {
			retireDatabase(source)
		}
		return s.Snapshot(), nil
	}, done: func(_ core.Data, err error) {
		a.pending--
		if err != nil {
			d.restoring, a.restoring = false, false
			a.err = err.Error()
			d.invalidate()
			return
		}
		mygo.App.Relaunch()
	}}
}

// moveWorkspace copies the workspace of from into to and checks the copy.
// The window and session files go along so that Flow reopens as it was.
func moveWorkspace(s *core.Store, from, to string) error {
	if err := os.MkdirAll(to, 0700); err != nil {
		return err
	}
	db := filepath.Join(to, "flow.db")
	if err := s.CopyTo(db); err != nil {
		return err
	}
	check := func() error {
		copied, err := core.Open(db)
		if err != nil {
			return err
		}
		defer copied.Close()
		current, _, err := s.Refresh()
		if err != nil {
			return err
		}
		if workspaceHash(copied.Snapshot()) != workspaceHash(current) {
			return errors.New("workspace_changed_while_moving")
		}
		return nil
	}
	if err := check(); err != nil {
		os.Remove(db)
		return err
	}
	for _, name := range []string{"backups", "sync", "windows", "desktop-session.json", "desktop-windows.json", "window-state.json", "updater.json"} {
		if err := copyTree(filepath.Join(from, name), filepath.Join(to, name)); err != nil {
			return err
		}
	}
	return nil
}

// copyTree copies a file or a folder, missing or not, keeping files already
// at the destination, as backups from an earlier stay there.
func copyTree(from, to string) error {
	return filepath.WalkDir(from, func(path string, entry fs.DirEntry, err error) error {
		if errors.Is(err, os.ErrNotExist) && path == from {
			return nil
		}
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(from, path)
		if err != nil {
			return err
		}
		dest := filepath.Join(to, rel)
		if entry.IsDir() {
			return os.MkdirAll(dest, 0700)
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		if _, err := os.Stat(dest); err == nil {
			return nil
		}
		return copyPlain(path, dest)
	})
}

func copyPlain(from, to string) error {
	b, err := os.ReadFile(from)
	if err != nil {
		return err
	}
	return core.WriteFile(to, b)
}

// retireDatabase renames the database a move left behind, so that it is
// kept but never taken for the workspace again.
func retireDatabase(dir string) {
	old := filepath.Join(dir, "flow.db")
	name := old + ".moved"
	if _, err := os.Stat(name); err == nil {
		name = old + ".moved-" + time.Now().Format("20060102-150405")
	}
	os.Rename(old, name)
}
