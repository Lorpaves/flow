package core

import (
	"bytes"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func withFile(t *testing.T, body []byte) (Data, Attachment) {
	t.Helper()
	d := seeded(t)
	v, e := d.AddAttachment(Attachment{OwnerKind: "issue", OwnerID: firstIssue(d).ID, Name: "photo.png", Data: body})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = d.AddAttachment(Attachment{OwnerKind: "issue", OwnerID: firstIssue(d).ID, Name: "Spec", URL: "https://example.com/spec"}); e != nil {
		t.Fatal(e)
	}
	return d, v
}

// A sync snapshot carries the checksum of a file, not its bytes; the bytes
// lie once in Files beside Devices, and the snapshot still restores by hand.
func TestSnapshotKeepsFilesApartAndStillRestores(t *testing.T) {
	body := bytes.Repeat([]byte("image bytes "), 4096)
	d, file := withFile(t, body)
	raw, files, e := EncodeSnapshotApart(d, SyncMeta{Device: "dev1", DeviceName: "iMac"})
	if e != nil {
		t.Fatal(e)
	}
	if bytes.Contains(raw, []byte(base64.StdEncoding.EncodeToString(body)[:64])) || len(raw) > len(body)/4 {
		t.Fatalf("the snapshot carries the file (%d bytes)", len(raw))
	}
	if len(files) != 1 || !bytes.Equal(files[file.SHA256], body) {
		t.Fatalf("files = %d", len(files))
	}
	if d.Attachments[file.ID].Data == nil {
		t.Fatal("encoding changed the workspace")
	}
	folder := t.TempDir()
	if e = StoreFiles(filepath.Join(folder, "Files"), files); e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(folder, "Devices", "dev1.flow.json")
	if e = WriteFile(path, raw); e != nil {
		t.Fatal(e)
	}
	got, e := ReadBackup(path)
	if e != nil || !reflect.DeepEqual(got, d) {
		t.Fatalf("restoring the snapshot: %v", e)
	}

	// Without the folder of files, or with a file cut short, it waits.
	var missing *MissingFilesError
	if _, _, e = DecodeSnapshot(raw); !errors.As(e, &missing) || len(missing.SHA) != 1 || missing.SHA[0] != file.SHA256 {
		t.Fatalf("decoding without the files: %v", e)
	}
	if e = os.WriteFile(filepath.Join(folder, "Files", file.SHA256), body[:100], 0600); e != nil {
		t.Fatal(e)
	}
	if _, e = ReadBackup(path); !errors.As(e, &missing) {
		t.Fatalf("a file cut short was taken: %v", e)
	}
	// A merge base leaves out what no side holds any longer.
	base, _, e := DecodeSnapshotWith(raw, SnapshotFiles{DropMissing: true})
	if e != nil || len(base.Attachments) != len(d.Attachments)-1 {
		t.Fatalf("base: %v, %d attachments", e, len(base.Attachments))
	}
	// Taken from a workspace that has the file, it needs no folder at all.
	if got, _, e = DecodeSnapshotWith(raw, SnapshotFiles{Sources: []FileSource{FilesOf(d)}}); e != nil || !reflect.DeepEqual(got, d) {
		t.Fatalf("from the workspace: %v", e)
	}
	// Snapshots written inline, as before, still read.
	inline, e := EncodeSnapshot(d, SyncMeta{Device: "dev1"})
	if e != nil {
		t.Fatal(e)
	}
	if got, _, e = DecodeSnapshot(inline); e != nil || !reflect.DeepEqual(got, d) {
		t.Fatalf("inline snapshot: %v", e)
	}
	if names, e := SnapshotFileNames(raw); e != nil || len(names) != 1 || names[0] != file.SHA256 {
		t.Fatalf("names = %v, %v", names, e)
	}
}

// Checksums come from files other Macs wrote: only real ones name a file,
// and a file there already, or held by iCloud alone, is not written again.
func TestStoredFilesAreNamedByTheirChecksumOnly(t *testing.T) {
	dir := t.TempDir()
	if e := os.WriteFile(filepath.Join(filepath.Dir(dir), "secret"), []byte("x"), 0600); e != nil {
		t.Fatal(e)
	}
	for _, name := range []string{"../secret", "", "ABC", "../" + emptySHA[3:]} {
		if _, ok := FilesIn(dir)(name); ok || ValidSHA(name) {
			t.Fatalf("%q was taken for a file", name)
		}
	}
	if e := StoreFiles(dir, map[string][]byte{"../secret": []byte("y")}); e == nil {
		t.Fatal("a file was written outside the folder")
	}
	_, file := withFile(t, []byte("abc"))
	path := filepath.Join(dir, file.SHA256)
	if e := StoreFiles(dir, map[string][]byte{file.SHA256: []byte("abc")}); e != nil {
		t.Fatal(e)
	}
	before, _ := os.Stat(path)
	if e := StoreFiles(dir, map[string][]byte{file.SHA256: []byte("abc")}); e != nil {
		t.Fatal(e)
	}
	if after, _ := os.Stat(path); !after.ModTime().Equal(before.ModTime()) {
		t.Fatal("a stored file was written again")
	}
	if e := os.Rename(path, ICloudPlaceholder(path)); e != nil {
		t.Fatal(e)
	}
	if e := StoreFiles(dir, map[string][]byte{file.SHA256: []byte("abc")}); e != nil {
		t.Fatal(e)
	}
	if _, e := os.Stat(path); e == nil {
		t.Fatal("a file iCloud holds was written again")
	}
}
