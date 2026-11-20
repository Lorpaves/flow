package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flow/internal/core"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// syncEngine keeps this Mac's workspace in step with the other Macs that
// share a folder, iCloud Drive's Flow folder by default. The live database
// never goes there: each device writes its whole workspace to a file of its
// own, Devices/<device>.flow.json, and takes in the others' changes with a
// three-way merge (core.Merge) against the copy of their file it merged
// last, kept in sync/peers. As no file in the folder has two writers, iCloud
// never has to choose between two versions of one. Attached files are not in
// those files but in Files/<sha256>, each written once (snapshot_files.go):
// a file named by its contents has the same bytes whichever Mac writes it.
type syncEngine struct {
	dataDir   string
	store     *core.Store // a connection of its own, as the MCP server has
	state     syncState
	name      string               // this Mac's name, shown on the other Macs
	lastHash  string               // the workspace at the previous cycle, to wait for a pause in editing
	lastRev   int64                // the store's revision lastHash was taken at
	requested map[string]time.Time // files asked of iCloud, and when
}

// syncState is a device's sync settings and progress, kept in the data
// folder beside the workspace it belongs to.
type syncState struct {
	Enabled bool                `json:"enabled"`
	Folder  string              `json:"folder,omitempty"`
	Device  string              `json:"device,omitempty"`
	Machine string              `json:"machine,omitempty"`
	Pushed  string              `json:"pushed,omitempty"` // hash of the workspace last written to the folder
	Synced  int64               `json:"synced,omitempty"`
	Peers   map[string]syncPeer `json:"peers,omitempty"`
	// Tidied is when the Files folder was last pruned, and Unreferenced
	// the files no device named then, with when this Mac first saw them so.
	Tidied       int64            `json:"tidied,omitempty"`
	Unreferenced map[string]int64 `json:"unreferenced,omitempty"`
}

// syncPeer is what this device last took in from another one.
type syncPeer struct {
	Name      string `json:"name"`
	Size      int64  `json:"size"`
	ModTime   int64  `json:"modTime"`
	Hash      string `json:"hash"`
	WrittenAt int64  `json:"writtenAt"`
}

type syncReport struct {
	Applied   bool     // the workspace took in other devices' changes
	Pushed    bool     // this device's file was rewritten
	Devices   []string // the other devices in the folder
	Conflicts []string // devices whose changes could not be merged; their file went to backups
	Waiting   []string // devices whose changes wait for files iCloud has not downloaded yet
}

const (
	syncTick = 3 * time.Second
	// A file no device names is removed once it has stayed so this long, as
	// seen from this Mac: a Mac that has just added one writes it before its
	// snapshot, and iCloud may bring the two at different times.
	unreferencedFor = 7 * 24 * time.Hour
	tidyEvery       = time.Hour
)

func syncDirectory(dataDir string) string { return filepath.Join(dataDir, "sync") }

func readSyncState(dataDir string) (syncState, error) {
	var st syncState
	b, err := os.ReadFile(filepath.Join(syncDirectory(dataDir), "state.json"))
	if errors.Is(err, os.ErrNotExist) {
		return st, nil
	}
	if err == nil {
		err = json.Unmarshal(b, &st)
	}
	return st, err
}

func writeSyncState(dataDir string, st syncState) error {
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return core.WriteFile(filepath.Join(syncDirectory(dataDir), "state.json"), b)
}

func openSyncEngine(dataDir string, st syncState) (*syncEngine, error) {
	store, err := core.Open(filepath.Join(dataDir, "flow.db"))
	if err != nil {
		return nil, err
	}
	e := &syncEngine{dataDir: dataDir, store: store, state: st, name: computerName()}
	e.claimDevice()
	return e, nil
}

func (e *syncEngine) close() { e.store.Close() }

// claimDevice gives this Mac a file of its own in the folder. A data folder
// copied from another Mac, as Migration Assistant does, brings that Mac's
// device along; this Mac then takes a new one, and merges the old one's file
// as another Mac's.
func (e *syncEngine) claimDevice() {
	machine := machineID()
	if e.state.Device != "" && (machine == "" || e.state.Machine == "" || e.state.Machine == machine) {
		e.state.Machine = cmpOr(e.state.Machine, machine)
		return
	}
	e.state.Device, e.state.Machine, e.state.Pushed = core.ID(), machine, ""
}

func cmpOr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func (e *syncEngine) devicesDir() string { return filepath.Join(e.state.Folder, "Devices") }
func (e *syncEngine) filesDir() string   { return filepath.Join(e.state.Folder, "Files") }
func (e *syncEngine) ownPath() string {
	return filepath.Join(e.devicesDir(), e.state.Device+".flow.json")
}
func (e *syncEngine) basePath(id string) string {
	return filepath.Join(syncDirectory(e.dataDir), "peers", id+".flow.json")
}

type peerFile struct {
	id, path      string
	size, modTime int64
}

type peerSnapshot struct {
	peerFile
	raw  []byte
	hash string
	data core.Data
	meta core.SyncMeta
}

// peers lists the other devices' files in the folder.
func (e *syncEngine) peers() ([]peerFile, error) {
	entries, err := os.ReadDir(e.devicesDir())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []peerFile
	for _, entry := range entries {
		name := entry.Name()
		id, ok := strings.CutSuffix(name, ".flow.json")
		if !ok || entry.IsDir() || strings.HasPrefix(name, ".") || id == e.state.Device {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue // removed meanwhile
		}
		out = append(out, peerFile{id, filepath.Join(e.devicesDir(), name), info.Size(), info.ModTime().UnixNano()})
	}
	return out, nil
}

// readPeer loads a device's file, unless it is the one known already. A
// file that does not decode, being written by a newer Flow or not yet whole,
// is left for a later cycle, as is one whose attached files iCloud has not
// brought yet; missing lists those.
func (e *syncEngine) readPeer(f peerFile, known string, local func() core.Data) (p peerSnapshot, readable bool, missing []string, err error) {
	raw, err := core.ReadLimited(f.path)
	if err != nil {
		return p, false, nil, err // iCloud may still be downloading it
	}
	p = peerSnapshot{peerFile: f, raw: raw, hash: hashBytes(raw)}
	if p.hash == known {
		return p, true, nil, nil
	}
	// A file this Mac has already is taken from its own workspace, so only
	// files new to it wait on iCloud.
	p.data, p.meta, err = core.DecodeSnapshotWith(raw, core.SnapshotFiles{Sources: []core.FileSource{core.FilesOf(local()), core.FilesIn(e.filesDir())}})
	var gone *core.MissingFilesError
	if errors.As(err, &gone) {
		return p, false, gone.SHA, nil
	}
	return p, err == nil, nil, nil
}

// fetch asks iCloud for the files it keeps only in iCloud, which it does not
// download by itself when the Mac saves space; once a minute at most each.
func (e *syncEngine) fetch(files []string) {
	if !inICloud(e.state.Folder) {
		return // another synced folder brings its files by itself
	}
	if e.requested == nil {
		e.requested = map[string]time.Time{}
	}
	for _, sha := range files {
		path := filepath.Join(e.filesDir(), sha)
		if _, err := os.Stat(core.ICloudPlaceholder(path)); err != nil || time.Since(e.requested[sha]) < time.Minute {
			continue
		}
		e.requested[sha] = time.Now()
		if cmd := exec.Command("/usr/bin/brctl", "download", path); cmd.Start() == nil {
			go cmd.Wait()
		}
	}
}

func hashBytes(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func workspaceHash(d core.Data) string {
	b, _ := json.Marshal(d)
	return hashBytes(b)
}

// cycle takes in what the other devices changed since the last cycle, then
// writes this device's file when the workspace changed and editing paused
// (or at once with force).
func (e *syncEngine) cycle(force bool) (syncReport, error) {
	var rep syncReport
	if info, err := os.Stat(e.state.Folder); err != nil || !info.IsDir() {
		if err == nil {
			err = errors.New("sync_folder_not_a_folder")
		}
		return rep, err
	}
	if e.state.Peers == nil {
		e.state.Peers = map[string]syncPeer{}
	}
	files, err := e.peers()
	if err != nil {
		return rep, err
	}
	var local *core.Data
	localData := func() core.Data {
		if local == nil {
			d := e.store.Snapshot()
			local = &d
		}
		return *local
	}
	var changed []peerSnapshot
	for _, f := range files {
		known, ok := e.state.Peers[f.id]
		if ok && known.Size == f.size && known.ModTime == f.modTime {
			continue
		}
		p, readable, missing, err := e.readPeer(f, known.Hash, localData)
		if err != nil {
			return rep, err
		}
		if len(missing) > 0 {
			rep.Waiting = append(rep.Waiting, cmpOr(known.Name, f.id))
			e.fetch(missing)
		}
		if !readable {
			continue
		}
		if ok && p.hash == known.Hash {
			known.Size, known.ModTime = f.size, f.modTime
			e.state.Peers[f.id] = known
			continue
		}
		changed = append(changed, p)
	}
	if len(changed) > 0 {
		bases := map[string]core.Data{}
		for _, p := range changed {
			bases[p.id] = core.Empty()
			if raw, err := core.ReadLimited(e.basePath(p.id)); err == nil {
				// The base's files are the two sides' own: one neither side
				// has any longer is gone from both, and is left out of it.
				sources := []core.FileSource{core.FilesOf(p.data), core.FilesOf(localData()), core.FilesIn(e.filesDir())}
				if base, _, err := core.DecodeSnapshotWith(raw, core.SnapshotFiles{Sources: sources, DropMissing: true}); err == nil {
					bases[p.id] = base
				}
			}
		}
		var failed []peerSnapshot
		_, applied, err := e.store.Apply(func(local core.Data) (core.Data, error) {
			failed = nil
			for _, p := range changed {
				merged, _, err := core.Merge(bases[p.id], local, p.data)
				if err != nil {
					failed = append(failed, p)
					continue
				}
				local = merged
			}
			return local, nil
		})
		if err != nil {
			return rep, err
		}
		rep.Applied = applied
		for _, p := range failed {
			name := cmpOr(p.meta.DeviceName, p.id)
			// Saved whole, files and all, so that it restores on its own.
			path := filepath.Join(e.dataDir, "backups", "sync-conflict-"+safeFileName(name)+"-"+time.Now().Format("20060102-150405")+".flow.json")
			if err := core.WriteBackup(path, p.data); err != nil {
				return rep, err
			}
			rep.Conflicts = append(rep.Conflicts, name)
		}
		for _, p := range changed {
			if err := core.WriteFile(e.basePath(p.id), p.raw); err != nil {
				return rep, err
			}
			e.state.Peers[p.id] = syncPeer{Name: p.meta.DeviceName, Size: p.size, ModTime: p.modTime, Hash: p.hash, WrittenAt: p.meta.WrittenAt}
		}
	}
	for _, f := range files {
		if p, ok := e.state.Peers[f.id]; ok {
			rep.Devices = append(rep.Devices, cmpOr(p.Name, f.id))
		}
	}

	data, _, err := e.store.Refresh()
	if err != nil {
		return rep, err
	}
	hash := e.lastHash
	if rev := e.store.Revision(); rev != e.lastRev || hash == "" {
		hash, e.lastRev = workspaceHash(data), rev
	}
	paused := hash == e.lastHash
	e.lastHash = hash
	if hash != e.state.Pushed && (force || paused || rep.Applied) {
		b, files, err := core.EncodeSnapshotApart(data, core.SyncMeta{Device: e.state.Device, DeviceName: e.name, WrittenAt: time.Now().UnixMilli()})
		if err != nil {
			return rep, err
		}
		// The files first: a Mac that sees the snapshot looks for them.
		if err = core.StoreFiles(e.filesDir(), files); err != nil {
			return rep, err
		}
		if err = core.WriteFile(e.ownPath(), b); err != nil {
			return rep, err
		}
		e.state.Pushed, rep.Pushed = hash, true
	}
	if err = e.tidyFiles(data, time.Now()); err != nil {
		return rep, err
	}
	e.state.Synced = time.Now().UnixMilli()
	return rep, writeSyncState(e.dataDir, e.state)
}

// tidyFiles, once an hour, puts back any of this Mac's files missing from
// the Files folder, and removes the files no device has named for a week.
// It leaves everything when it cannot read every device's snapshot, since
// one it cannot read may name any file.
func (e *syncEngine) tidyFiles(local core.Data, now time.Time) error {
	if now.Sub(time.UnixMilli(e.state.Tidied)) < tidyEvery {
		return nil
	}
	e.state.Tidied = now.UnixMilli()
	named, own := map[string]bool{}, map[string][]byte{}
	for _, v := range local.Attachments {
		if v.URL == "" && len(v.Data) > 0 {
			named[v.SHA256], own[v.SHA256] = true, v.Data
		}
	}
	// Another Mac may have removed one while this Mac's snapshot naming it
	// was still on its way; writing it again heals that.
	if err := core.StoreFiles(e.filesDir(), own); err != nil {
		return err
	}
	devices, err := os.ReadDir(e.devicesDir())
	if err != nil {
		return nil
	}
	for _, entry := range devices {
		name := entry.Name()
		if strings.HasSuffix(name, ".flow.json.icloud") {
			return nil // a snapshot iCloud keeps only in iCloud
		}
		if !strings.HasSuffix(name, ".flow.json") || strings.HasPrefix(name, ".") || entry.IsDir() {
			continue
		}
		raw, err := core.ReadLimited(filepath.Join(e.devicesDir(), name))
		if err != nil {
			return nil
		}
		shas, err := core.SnapshotFileNames(raw)
		if err != nil {
			return nil
		}
		for _, sha := range shas {
			named[sha] = true
		}
	}
	entries, err := os.ReadDir(e.filesDir())
	if err != nil {
		return nil
	}
	seen := map[string]bool{}
	if e.state.Unreferenced == nil {
		e.state.Unreferenced = map[string]int64{}
	}
	for _, entry := range entries {
		sha := entry.Name()
		if s, ok := strings.CutSuffix(strings.TrimPrefix(sha, "."), ".icloud"); ok && strings.HasPrefix(sha, ".") {
			sha = s
		}
		if !core.ValidSHA(sha) || entry.IsDir() {
			continue
		}
		seen[sha] = true
		if named[sha] {
			delete(e.state.Unreferenced, sha)
			continue
		}
		first, ok := e.state.Unreferenced[sha]
		if !ok {
			e.state.Unreferenced[sha] = now.UnixMilli()
			continue
		}
		if now.Sub(time.UnixMilli(first)) >= unreferencedFor {
			if err := os.Remove(filepath.Join(e.filesDir(), entry.Name())); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
			delete(e.state.Unreferenced, sha)
		}
	}
	for sha := range e.state.Unreferenced {
		if !seen[sha] {
			delete(e.state.Unreferenced, sha)
		}
	}
	return nil
}

// join starts syncing with a folder other devices may already use. With
// adopt this Mac takes the workspace of the device that wrote last in place
// of its own, which is backed up first; otherwise the two are merged. The
// other devices' files then count as taken in, so that one a Mac left long
// ago brings back nothing removed since.
func (e *syncEngine) join(adopt bool) error {
	files, err := e.peers()
	if err != nil {
		return err
	}
	var all []peerSnapshot
	newest := -1
	local := e.store.Snapshot()
	for _, f := range files {
		p, readable, _, err := e.readPeer(f, "", func() core.Data { return local })
		if err != nil {
			return err
		}
		if !readable {
			continue
		}
		all = append(all, p)
		if newest < 0 || p.meta.WrittenAt > all[newest].meta.WrittenAt {
			newest = len(all) - 1
		}
	}
	if newest >= 0 {
		incoming := all[newest].data
		if adopt {
			local, _, err := e.store.Refresh()
			if err != nil {
				return err
			}
			if err = core.WriteBackup(filepath.Join(e.dataDir, "backups", "before-sync-"+time.Now().Format("20060102-150405")+".flow.json"), local); err != nil {
				return err
			}
			_, _, err = e.store.Apply(func(core.Data) (core.Data, error) { return incoming, nil })
		} else {
			_, _, err = e.store.Apply(func(local core.Data) (core.Data, error) {
				merged, _, err := core.Merge(core.Empty(), local, incoming)
				return merged, err
			})
		}
		if err != nil {
			return err
		}
	}
	e.state.Peers = map[string]syncPeer{}
	for _, p := range all {
		if err := core.WriteFile(e.basePath(p.id), p.raw); err != nil {
			return err
		}
		e.state.Peers[p.id] = syncPeer{Name: p.meta.DeviceName, Size: p.size, ModTime: p.modTime, Hash: p.hash, WrittenAt: p.meta.WrittenAt}
	}
	e.lastHash, e.state.Pushed = "", ""
	return nil
}

// syncFolderDevices names the devices already syncing through folder,
// leaving out device (this Mac's).
func syncFolderDevices(folder, device string) ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(folder, "Devices"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var names []string
	for _, entry := range entries {
		id, ok := strings.CutSuffix(entry.Name(), ".flow.json")
		if !ok || entry.IsDir() || strings.HasPrefix(entry.Name(), ".") || id == device {
			continue
		}
		raw, err := core.ReadLimited(filepath.Join(folder, "Devices", entry.Name()))
		if err != nil {
			return nil, err
		}
		var v struct {
			Sync *core.SyncMeta `json:"sync"`
		}
		if json.Unmarshal(raw, &v) != nil || v.Sync == nil {
			continue
		}
		names = append(names, cmpOr(v.Sync.DeviceName, id))
	}
	return names, nil
}

// iCloudDrive is this Mac's iCloud Drive folder, or "" when iCloud Drive is
// off.
func iCloudDrive() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	dir := filepath.Join(home, "Library", "Mobile Documents", "com~apple~CloudDocs")
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return ""
	}
	return dir
}

// inICloud reports whether iCloud syncs path: it is in iCloud Drive, in an
// app's iCloud folder, or on a Desktop or in Documents that iCloud Drive
// holds.
func inICloud(path string) bool {
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	if within(filepath.Join(home, "Library", "Mobile Documents"), path) {
		return true
	}
	if drive := iCloudDrive(); drive != "" {
		for _, name := range []string{"Desktop", "Documents"} {
			if _, err := os.Lstat(filepath.Join(drive, name)); err == nil && within(filepath.Join(home, name), path) {
				return true
			}
		}
	}
	return false
}

// within reports whether path is dir or inside it.
func within(dir, path string) bool {
	rel, err := filepath.Rel(dir, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

var unsafeName = regexp.MustCompile(`[/\\:*?"<>|\x00-\x1f]+`)

func safeFileName(s string) string {
	s = strings.TrimSpace(unsafeName.ReplaceAllString(s, "-"))
	if s == "" {
		return "device"
	}
	return s
}

var platformUUID = regexp.MustCompile(`"IOPlatformUUID" = "([^"]+)"`)

// machineID tells this Mac from others, or is "" where it cannot be read.
func machineID() string {
	out, err := exec.Command("/usr/sbin/ioreg", "-rd1", "-c", "IOPlatformExpertDevice").Output()
	if err != nil {
		return ""
	}
	if m := platformUUID.FindSubmatch(out); m != nil {
		return string(m[1])
	}
	return ""
}

// computerName is the name the user gave this Mac.
func computerName() string {
	if out, err := exec.Command("/usr/sbin/scutil", "--get", "ComputerName").Output(); err == nil {
		if name := strings.TrimSpace(string(out)); name != "" {
			return name
		}
	}
	host, _ := os.Hostname()
	return strings.TrimSuffix(host, ".local")
}
