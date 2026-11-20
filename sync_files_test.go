package main

import (
	"bytes"
	"flow/internal/core"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func (m *syncMac) attach(body []byte) core.Attachment {
	m.t.Helper()
	var file core.Attachment
	m.edit(func(d *core.Data) {
		var err error
		if file, err = d.AddAttachment(core.Attachment{OwnerKind: "issue", OwnerID: firstIssueID(*d), Name: "photo.png", Data: body}); err != nil {
			m.t.Fatal(err)
		}
	})
	return file
}

// An attached file crosses to the other Mac once, in Files; the snapshots
// carry its checksum, and rewriting one after an edit leaves the file be.
func TestSyncKeepsAttachedFilesApart(t *testing.T) {
	folder := t.TempDir()
	imac := newSyncMac(t, folder, true)
	imac.engine.join(false)
	imac.cycle()
	mbp := newSyncMac(t, folder, false)
	if err := mbp.engine.join(true); err != nil {
		t.Fatal(err)
	}
	mbp.cycle()

	body := bytes.Repeat([]byte{0x89, 'P', 'N', 'G'}, 64*1024)
	file := imac.attach(body)
	imac.cycle()
	stored := filepath.Join(folder, "Files", file.SHA256)
	before, err := os.Stat(stored)
	if err != nil || before.Size() != int64(len(body)) {
		t.Fatalf("the file is not in Files: %v", err)
	}
	if snap, _ := os.Stat(imac.engine.ownPath()); snap.Size() > int64(len(body))/4 {
		t.Fatalf("the snapshot carries the file: %d bytes", snap.Size())
	}
	mbp.cycle()
	if got := mbp.data().Attachments[file.ID]; !bytes.Equal(got.Data, body) {
		t.Fatal("the other Mac did not get the file")
	}

	imac.edit(func(d *core.Data) {
		i := d.Issues[firstIssueID(*d)]
		i.Title = "Renamed with a file"
		d.Issues[i.ID] = i
	})
	if rep := imac.cycle(); !rep.Pushed {
		t.Fatal("the edit was not written")
	}
	if after, _ := os.Stat(stored); !after.ModTime().Equal(before.ModTime()) {
		t.Fatal("an edit wrote the file again")
	}
	mbp.cycle()
	if _, ok := issueTitled(mbp.data(), "Renamed with a file"); !ok {
		t.Fatal("the edit did not cross")
	}
	if got := mbp.data().Attachments[file.ID]; !bytes.Equal(got.Data, body) {
		t.Fatal("the edit lost the file")
	}
	// Each snapshot alone, with Files beside it, is a backup.
	if restored, err := core.ReadBackup(imac.engine.ownPath()); err != nil || !bytes.Equal(restored.Attachments[file.ID].Data, body) {
		t.Fatalf("the snapshot does not restore: %v", err)
	}
}

// A snapshot whose new file iCloud has not brought yet waits for it, and
// is taken in, whole, once it is there.
func TestSyncWaitsForAFileStillDownloading(t *testing.T) {
	folder := t.TempDir()
	imac := newSyncMac(t, folder, true)
	imac.engine.join(false)
	imac.cycle()
	mbp := newSyncMac(t, folder, false)
	mbp.engine.join(true)
	mbp.cycle()

	file := imac.attach([]byte("not here yet"))
	imac.edit(func(d *core.Data) { i := d.Issues[firstIssueID(*d)]; i.Title = "With a new file"; d.Issues[i.ID] = i })
	imac.cycle()
	stored := filepath.Join(folder, "Files", file.SHA256)
	if err := os.Rename(stored, core.ICloudPlaceholder(stored)); err != nil {
		t.Fatal(err)
	}
	rep := mbp.cycle()
	if len(rep.Waiting) != 1 || rep.Applied {
		t.Fatalf("waiting = %v, applied = %v", rep.Waiting, rep.Applied)
	}
	if _, ok := issueTitled(mbp.data(), "With a new file"); ok {
		t.Fatal("half a change was taken in")
	}
	if err := os.Rename(core.ICloudPlaceholder(stored), stored); err != nil {
		t.Fatal(err)
	}
	if rep = mbp.cycle(); len(rep.Waiting) != 0 || !rep.Applied {
		t.Fatalf("waiting = %v, applied = %v", rep.Waiting, rep.Applied)
	}
	if _, ok := mbp.data().Attachments[file.ID]; !ok {
		t.Fatal("the file did not arrive")
	}
}

// A file no device names any more is removed after a week, as seen from one
// Mac; one a Mac still has is put back if it went missing.
func TestSyncPrunesFilesNoDeviceNames(t *testing.T) {
	folder := t.TempDir()
	imac := newSyncMac(t, folder, true)
	imac.engine.join(false)
	kept := imac.attach([]byte("kept"))
	gone := imac.attach([]byte("removed"))
	imac.cycle()
	imac.edit(func(d *core.Data) { delete(d.Attachments, gone.ID) })
	imac.cycle()
	files := filepath.Join(folder, "Files")
	tidy := func(at time.Time) {
		t.Helper()
		imac.engine.state.Tidied = 0
		if err := imac.engine.tidyFiles(imac.data(), at); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now()
	tidy(now)
	tidy(now.Add(3 * 24 * time.Hour))
	if _, err := os.Stat(filepath.Join(files, gone.SHA256)); err != nil {
		t.Fatal("a file was removed before its week")
	}
	tidy(now.Add(8 * 24 * time.Hour))
	if _, err := os.Stat(filepath.Join(files, gone.SHA256)); err == nil {
		t.Fatal("a file no device names was kept")
	}
	if err := os.Remove(filepath.Join(files, kept.SHA256)); err != nil {
		t.Fatal(err)
	}
	tidy(now.Add(9 * 24 * time.Hour))
	if b, err := os.ReadFile(filepath.Join(files, kept.SHA256)); err != nil || string(b) != "kept" {
		t.Fatal("a file this Mac has was not put back")
	}
	// A snapshot it cannot read may name anything: nothing is removed then.
	stray := []byte("named by an unreadable snapshot")
	if err := core.StoreFiles(files, map[string][]byte{hashBytes(stray): stray}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(folder, "Devices", ".OTHER.flow.json.icloud"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	tidy(now.Add(10 * 24 * time.Hour))
	tidy(now.Add(20 * 24 * time.Hour))
	if _, err := os.Stat(filepath.Join(files, hashBytes(stray))); err != nil {
		t.Fatal("a file was removed while a snapshot could not be read")
	}
}
