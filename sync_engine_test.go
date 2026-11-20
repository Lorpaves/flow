package main

import (
	"flow/internal/core"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// syncMac is one Mac of a sync test: its data folder, the engine, and a
// second connection standing for the app's windows.
type syncMac struct {
	t      *testing.T
	engine *syncEngine
	app    *core.Store
}

func newSyncMac(t *testing.T, folder string, seed bool) *syncMac {
	t.Helper()
	dir := t.TempDir()
	app, err := core.Open(filepath.Join(dir, "flow.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.Close() })
	if _, err = app.Update(func(d *core.Data) error {
		d.Preferences.Initialized = true
		if seed {
			d.Seed()
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	e, err := openSyncEngine(dir, syncState{Enabled: true, Folder: folder})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(e.close)
	return &syncMac{t, e, app}
}

func (m *syncMac) cycle() syncReport {
	m.t.Helper()
	rep, err := m.engine.cycle(true)
	if err != nil {
		m.t.Fatal(err)
	}
	return rep
}

func (m *syncMac) data() core.Data {
	m.t.Helper()
	d, _, err := m.app.Refresh()
	if err != nil {
		m.t.Fatal(err)
	}
	return d
}

func (m *syncMac) edit(fn func(*core.Data)) {
	m.t.Helper()
	if _, err := m.app.Update(func(d *core.Data) error { fn(d); return nil }); err != nil {
		m.t.Fatal(err)
	}
}

func issueTitled(d core.Data, title string) (core.Issue, bool) {
	for _, i := range d.Issues {
		if i.Title == title {
			return i, true
		}
	}
	return core.Issue{}, false
}

func TestSyncCarriesEditsBetweenTwoMacs(t *testing.T) {
	folder := t.TempDir()
	imac := newSyncMac(t, folder, true)
	if err := imac.engine.join(false); err != nil {
		t.Fatal(err)
	}
	imac.cycle()
	macbook := newSyncMac(t, folder, false)
	if err := macbook.engine.join(true); err != nil {
		t.Fatal(err)
	}
	macbook.cycle()
	if workspaceHash(macbook.data()) != workspaceHash(imac.data()) {
		t.Fatal("the joining Mac did not take the workspace")
	}

	// Both Macs edit the same issue, each a different field, before they sync.
	target := imac.data().Issues[firstIssueID(imac.data())]
	imac.edit(func(d *core.Data) {
		i := d.Issues[target.ID]
		i.Title = "Renamed on the iMac"
		i.UpdatedAt++
		d.Issues[target.ID] = i
	})
	macbook.edit(func(d *core.Data) {
		i := d.Issues[target.ID]
		i.Status = "done"
		i.UpdatedAt += 2
		d.Issues[target.ID] = i
	})
	imac.cycle()
	macbook.cycle()
	imac.cycle()
	for _, m := range []*syncMac{imac, macbook} {
		got := m.data().Issues[target.ID]
		if got.Title != "Renamed on the iMac" || got.Status != "done" {
			t.Fatalf("%s has %q / %s", m.engine.name, got.Title, got.Status)
		}
	}
	if workspaceHash(macbook.data()) != workspaceHash(imac.data()) {
		t.Fatal("the Macs disagree after syncing")
	}

	// A quiet round changes nothing and writes nothing.
	if rep := imac.cycle(); rep.Applied || rep.Pushed {
		t.Fatalf("idle cycle did work: %+v", rep)
	}
}

func TestSyncNumbersIssuesMadeOfflineOnBothMacs(t *testing.T) {
	folder := t.TempDir()
	imac := newSyncMac(t, folder, true)
	imac.engine.join(false)
	imac.cycle()
	macbook := newSyncMac(t, folder, false)
	macbook.engine.join(true)
	macbook.cycle()

	imac.edit(func(d *core.Data) { d.AddIssue("From the iMac", "", "", "todo") })
	macbook.edit(func(d *core.Data) { d.AddIssue("From the MacBook", "", "", "todo") })
	imac.cycle()
	macbook.cycle()
	imac.cycle()
	macbook.cycle()
	a, b := imac.data(), macbook.data()
	x, okX := issueTitled(a, "From the iMac")
	y, okY := issueTitled(a, "From the MacBook")
	if !okX || !okY || x.Number == y.Number {
		t.Fatalf("issues %v %v share number %d", okX, okY, x.Number)
	}
	if workspaceHash(a) != workspaceHash(b) {
		t.Fatal("the Macs disagree after syncing new issues")
	}
}

func TestSyncSavesWhatCannotBeMerged(t *testing.T) {
	folder := t.TempDir()
	imac := newSyncMac(t, folder, true)
	imac.engine.join(false)
	imac.cycle()
	ids := make([]string, 0)
	for id := range imac.data().Projects {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	if len(ids) < 2 {
		t.Skip("seed has fewer than two projects")
	}
	// Another Mac made the first project depend on the second while this one
	// made the second depend on the first: together they would form a cycle.
	other := imac.data()
	p := other.Projects[ids[0]]
	p.Dependencies = []string{ids[1]}
	other.Projects[ids[0]] = p
	raw, err := core.EncodeSnapshot(other, core.SyncMeta{Device: "OTHER", DeviceName: "Old Mac"})
	if err != nil {
		t.Fatal(err)
	}
	if err = core.WriteFile(filepath.Join(folder, "Devices", "OTHER.flow.json"), raw); err != nil {
		t.Fatal(err)
	}
	imac.edit(func(d *core.Data) { p := d.Projects[ids[1]]; p.Dependencies = []string{ids[0]}; d.Projects[ids[1]] = p })

	rep := imac.cycle()
	if len(rep.Conflicts) != 1 || rep.Conflicts[0] != "Old Mac" {
		t.Fatalf("conflicts = %v", rep.Conflicts)
	}
	if deps := imac.data().Projects[ids[1]].Dependencies; len(deps) != 1 {
		t.Fatal("the local change was lost to the conflict")
	}
	backups, _ := filepath.Glob(filepath.Join(imac.engine.dataDir, "backups", "sync-conflict-Old Mac-*.flow.json"))
	if len(backups) != 1 {
		t.Fatalf("conflict copies = %v", backups)
	}
	if rep = imac.cycle(); len(rep.Conflicts) != 0 {
		t.Fatal("the same file conflicted twice")
	}
}

func TestSyncTakesANewDeviceOnAnotherMac(t *testing.T) {
	e := &syncEngine{state: syncState{Device: "COPIED", Machine: "some-other-mac"}}
	e.claimDevice()
	if machineID() != "" && e.state.Device == "COPIED" {
		t.Fatal("a data folder copied from another Mac kept that Mac's device")
	}
	same := &syncEngine{state: syncState{Device: "MINE", Machine: machineID()}}
	same.claimDevice()
	if same.state.Device != "MINE" {
		t.Fatal("this Mac lost its device")
	}
}

func TestICloudPathsAreRecognized(t *testing.T) {
	home, _ := os.UserHomeDir()
	if !inICloud(filepath.Join(home, "Library", "Mobile Documents", "com~apple~CloudDocs", "Flow")) {
		t.Fatal("iCloud Drive folder not recognized")
	}
	if inICloud(filepath.Join(home, "Library", "Application Support", "Flow")) {
		t.Fatal("Application Support taken for iCloud")
	}
	if within("/a/b", "/a/bc") || !within("/a/b", "/a/b/c") {
		t.Fatal("within is wrong")
	}
}

func firstIssueID(d core.Data) string {
	ids := make([]string, 0, len(d.Issues))
	for id := range d.Issues {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids[0]
}

func TestMovingTheWorkspaceCopiesItAndRetiresTheOld(t *testing.T) {
	from, to := t.TempDir(), filepath.Join(t.TempDir(), "Moved")
	s, err := core.Open(filepath.Join(from, "flow.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err = s.Update(func(d *core.Data) error { d.Seed(); d.Preferences.Initialized = true; return nil }); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{"desktop-session.json": "{}", "backups/old.flow.json": "{}", "sync/state.json": `{"enabled":true}`} {
		if err = core.WriteFile(filepath.Join(from, name), []byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err = moveWorkspace(s, from, to); err != nil {
		t.Fatal(err)
	}
	moved, err := core.Open(filepath.Join(to, "flow.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer moved.Close()
	if workspaceHash(moved.Snapshot()) != workspaceHash(s.Snapshot()) {
		t.Fatal("moved workspace differs")
	}
	for _, name := range []string{"desktop-session.json", "backups/old.flow.json", "sync/state.json"} {
		if _, err := os.Stat(filepath.Join(to, name)); err != nil {
			t.Fatalf("%s was not moved: %v", name, err)
		}
	}
	retireDatabase(from)
	if _, err := os.Stat(filepath.Join(from, "flow.db")); !os.IsNotExist(err) {
		t.Fatal("the old database is still in use")
	}
	if _, err := os.Stat(filepath.Join(from, "flow.db.moved")); err != nil {
		t.Fatal("the old database was not kept")
	}
}
