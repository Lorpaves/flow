package core

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
)

func seeded(t *testing.T) Data {
	t.Helper()
	d := Empty()
	d.Seed()
	if e := d.Validate(); e != nil {
		t.Fatal(e)
	}
	return d
}

func firstIssue(d Data) Issue {
	ids := make([]string, 0, len(d.Issues))
	for id := range d.Issues {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return d.Issues[ids[0]]
}

func mustMerge(t *testing.T, base, local, remote Data) Data {
	t.Helper()
	m, _, e := Merge(base, local, remote)
	if e != nil {
		t.Fatal(e)
	}
	return m
}

func TestMergeKeepsEachSidesFieldsOfOneIssue(t *testing.T) {
	base := seeded(t)
	i := firstIssue(base)
	local, remote := base.Clone(), base.Clone()
	li := local.Issues[i.ID]
	li.Title, li.UpdatedAt = "Renamed here", i.UpdatedAt+10
	local.Issues[i.ID] = li
	ri := remote.Issues[i.ID]
	ri.Status, ri.UpdatedAt = "done", i.UpdatedAt+20
	remote.Issues[i.ID] = ri

	got := mustMerge(t, base, local, remote).Issues[i.ID]
	if got.Title != "Renamed here" || got.Status != "done" || got.UpdatedAt != i.UpdatedAt+20 {
		t.Fatalf("merged issue = %+v", got)
	}
}

// One Mac moved an issue's start, the other its due date before that start:
// each was valid alone, so the merge drops the start rather than failing.
func TestMergeDropsAStartThatPassedTheOtherSidesDueDate(t *testing.T) {
	base := seeded(t)
	i := firstIssue(base)
	i.Start, i.Due = "2026-11-03", "2026-11-05"
	base.Issues[i.ID] = i
	local, remote := base.Clone(), base.Clone()
	li := local.Issues[i.ID]
	li.Start, li.UpdatedAt = "2026-11-04", i.UpdatedAt+10
	local.Issues[i.ID] = li
	ri := remote.Issues[i.ID]
	ri.Due, ri.UpdatedAt = "2026-11-02", i.UpdatedAt+20
	remote.Issues[i.ID] = ri

	got := mustMerge(t, base, local, remote).Issues[i.ID]
	if got.Start != "" || got.Due != "2026-11-02" {
		t.Fatalf("merged dates = %q..%q", got.Start, got.Due)
	}
}

func TestMergeSettlesTheSameFieldAlikeOnBothDevices(t *testing.T) {
	base := seeded(t)
	i := firstIssue(base)
	a, b := base.Clone(), base.Clone()
	ai := a.Issues[i.ID]
	ai.Title, ai.UpdatedAt = "Older edit", i.UpdatedAt+1
	a.Issues[i.ID] = ai
	bi := b.Issues[i.ID]
	bi.Title, bi.UpdatedAt = "Newer edit", i.UpdatedAt+2
	b.Issues[i.ID] = bi

	onA, onB := mustMerge(t, base, a, b), mustMerge(t, base, b, a)
	if onA.Issues[i.ID].Title != "Newer edit" || !reflect.DeepEqual(onA, onB) {
		t.Fatalf("devices disagree: %q vs %q", onA.Issues[i.ID].Title, onB.Issues[i.ID].Title)
	}
}

func TestMergeRenumbersIssuesCreatedOnBothDevices(t *testing.T) {
	base := seeded(t)
	a, b := base.Clone(), base.Clone()
	x, e := a.AddIssue("Made on the iMac", "", "", "todo")
	if e != nil {
		t.Fatal(e)
	}
	y, e := b.AddIssue("Made on the MacBook", "", "", "todo")
	if e != nil {
		t.Fatal(e)
	}
	if x.Number != y.Number {
		t.Fatal("test needs a shared number")
	}
	onA, onB := mustMerge(t, base, a, b), mustMerge(t, base, b, a)
	if onA.Issues[x.ID].Number == onA.Issues[y.ID].Number {
		t.Fatal("issues share a number after the merge")
	}
	if onA.Issues[x.ID].Number != onB.Issues[x.ID].Number || onA.Issues[y.ID].Number != onB.Issues[y.ID].Number {
		t.Fatal("devices numbered the issues differently")
	}
	if onA.Preferences.NextNumber != base.Preferences.NextNumber+2 {
		t.Fatalf("next number = %d", onA.Preferences.NextNumber)
	}
}

func TestMergeDropsALabelRemovedElsewhereFromAnIssue(t *testing.T) {
	base := seeded(t)
	var label Label
	for _, l := range base.Labels {
		if l.EntityKind() == "issue" {
			label = l
			break
		}
	}
	local, remote := base.Clone(), base.Clone()
	delete(local.Labels, label.ID)
	for id, i := range local.Issues {
		i.LabelIDs = slices.DeleteFunc(i.LabelIDs, func(l string) bool { return l == label.ID })
		local.Issues[id] = i
	}
	i := firstIssue(remote)
	if !slices.Contains(i.LabelIDs, label.ID) {
		i.LabelIDs = append(i.LabelIDs, label.ID)
	}
	i.UpdatedAt++
	remote.Issues[i.ID] = i

	got := mustMerge(t, base, local, remote)
	if _, ok := got.Labels[label.ID]; ok || slices.Contains(got.Issues[i.ID].LabelIDs, label.ID) {
		t.Fatal("removed label survived the merge")
	}
}

func TestMergeKeepsWhatOneSideChangedAndTheOtherRemoved(t *testing.T) {
	base := seeded(t)
	var label Label
	for _, l := range base.Labels {
		label = l
		break
	}
	local, remote := base.Clone(), base.Clone()
	delete(local.Labels, label.ID)
	for id, i := range local.Issues {
		i.LabelIDs = slices.DeleteFunc(i.LabelIDs, func(l string) bool { return l == label.ID })
		local.Issues[id] = i
	}
	for id, p := range local.Projects {
		p.LabelIDs = slices.DeleteFunc(p.LabelIDs, func(l string) bool { return l == label.ID })
		local.Projects[id] = p
	}
	renamed := remote.Labels[label.ID]
	renamed.Name = "Renamed elsewhere"
	remote.Labels[label.ID] = renamed

	if got := mustMerge(t, base, local, remote); got.Labels[label.ID].Name != "Renamed elsewhere" {
		t.Fatal("a label renamed on one device was lost to its removal on the other")
	}
}

func TestMergeWithoutABaseJoinsTwoWorkspaces(t *testing.T) {
	a, b := seeded(t), seeded(t)
	got := mustMerge(t, Empty(), a, b)
	if len(got.Issues) != len(a.Issues)+len(b.Issues) {
		t.Fatalf("joined %d issues, want %d", len(got.Issues), len(a.Issues)+len(b.Issues))
	}
}

func TestSnapshotIsAlsoABackup(t *testing.T) {
	d := seeded(t)
	b, e := EncodeSnapshot(d, SyncMeta{Device: "dev1", DeviceName: "iMac", WrittenAt: 42})
	if e != nil {
		t.Fatal(e)
	}
	got, meta, e := DecodeSnapshot(b)
	if e != nil || meta.Device != "dev1" || meta.DeviceName != "iMac" || meta.WrittenAt != 42 {
		t.Fatalf("meta = %+v, err = %v", meta, e)
	}
	if !reflect.DeepEqual(got, d) {
		t.Fatal("snapshot did not round-trip")
	}
	path := filepath.Join(t.TempDir(), "dev1.flow.json")
	if e = WriteFile(path, b); e != nil {
		t.Fatal(e)
	}
	if restored, e := ReadBackup(path); e != nil || !reflect.DeepEqual(restored, d) {
		t.Fatalf("snapshot is not a readable backup: %v", e)
	}
}

func TestApplyReplacesWithoutHistoryAndOthersSeeIt(t *testing.T) {
	s, path := testStore(t)
	other, e := Open(path)
	if e != nil {
		t.Fatal(e)
	}
	defer other.Close()
	incoming := seeded(t)
	_, changed, e := s.Apply(func(Data) (Data, error) { return incoming, nil })
	if e != nil || !changed {
		t.Fatalf("changed = %v, err = %v", changed, e)
	}
	seen, refreshed, e := other.Refresh()
	if e != nil || !refreshed || len(seen.Issues) != len(incoming.Issues) || len(seen.Activities) != len(incoming.Activities) {
		t.Fatalf("other connection saw %d issues, %d activities (refreshed %v, %v)", len(seen.Issues), len(seen.Activities), refreshed, e)
	}
	if _, changed, e = s.Apply(func(d Data) (Data, error) { return d, nil }); e != nil || changed {
		t.Fatalf("applying the same workspace changed it: %v", e)
	}
}

func TestCopyToMakesAnOpenableCopy(t *testing.T) {
	s, _ := testStore(t)
	if _, e := s.Update(func(d *Data) error { d.Seed(); return nil }); e != nil {
		t.Fatal(e)
	}
	copyPath := filepath.Join(t.TempDir(), "flow.db")
	if e := s.CopyTo(copyPath); e != nil {
		t.Fatal(e)
	}
	c, e := Open(copyPath)
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	if !reflect.DeepEqual(c.Snapshot(), s.Snapshot()) {
		t.Fatal("copy differs from the database")
	}
}

func TestDataLocationPointsToAMovedWorkspace(t *testing.T) {
	def, moved := t.TempDir(), t.TempDir()
	if dir, missing, e := ResolveDataDir(def); e != nil || dir != def || missing != "" {
		t.Fatalf("unmoved: %q %q %v", dir, missing, e)
	}
	if e := SetDataLocation(def, moved); e != nil {
		t.Fatal(e)
	}
	if dir, _, e := ResolveDataDir(def); e != nil || dir != moved {
		t.Fatalf("moved: %q %v", dir, e)
	}
	gone := filepath.Join(moved, "unplugged")
	if e := SetDataLocation(def, gone); e != nil {
		t.Fatal(e)
	}
	if dir, missing, e := ResolveDataDir(def); e != nil || dir != def || missing != gone {
		t.Fatalf("missing: %q %q %v", dir, missing, e)
	}
	if _, e := os.Stat(gone); !os.IsNotExist(e) {
		t.Fatal("resolving created the missing folder")
	}
	if e := SetDataLocation(def, def); e != nil {
		t.Fatal(e)
	}
	if _, e := os.Stat(filepath.Join(def, locationFile)); !os.IsNotExist(e) {
		t.Fatal("moving back left the note")
	}
}
