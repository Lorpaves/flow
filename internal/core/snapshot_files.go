package core

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// A sync snapshot is rewritten whole after every pause in editing, so it
// keeps the bytes of attached files out of itself: each file lies once in a
// Files folder beside the Devices folder, named by its SHA-256, and the
// snapshot's attachment carries the checksum alone. A file is written once
// and never changes, so iCloud uploads it once; an edit afterwards uploads
// only the text of the workspace.
//
//	Flow/Devices/<device>.flow.json  the workspace, files left out
//	Flow/Files/<sha256>              one attached file's bytes
//
// The snapshot says so with "files": "../Files", the folder of its files
// from its own. Restoring such a snapshot by hand still works while that
// folder is beside it (ReadBackup).

// SnapshotFilesDir is where a snapshot keeps its files, from its own folder.
const SnapshotFilesDir = "../Files"

// FileSource gives the bytes of the file with a checksum, or false when it
// does not hold that file.
type FileSource func(sha string) ([]byte, bool)

// SnapshotFiles says where the files a snapshot left out come from.
type SnapshotFiles struct {
	Sources []FileSource
	// DropMissing leaves out the attachments no source has bytes for,
	// instead of failing. That is right for a merge base only when the
	// sources hold both sides of the merge: a file neither side has any
	// longer is gone from both, and the base's copy of it decides nothing.
	DropMissing bool
}

// MissingFilesError is a snapshot naming files no source holds yet, as
// while iCloud is still downloading them.
type MissingFilesError struct{ SHA []string }

func (e *MissingFilesError) Error() string { return "snapshot_files_missing" }

// EncodeSnapshotApart is the sync snapshot of d, written by the device in
// meta, with its files left out. files maps each file's checksum to its
// bytes, for the caller to store (StoreFiles) before the snapshot, so that a
// device which sees the snapshot can find its files.
func EncodeSnapshotApart(d Data, meta SyncMeta) (b []byte, files map[string][]byte, err error) {
	if err = d.Validate(); err != nil {
		return nil, nil, err
	}
	files = map[string][]byte{}
	apart := d
	apart.Attachments = make(map[string]Attachment, len(d.Attachments))
	for id, v := range d.Attachments {
		if v.URL == "" && len(v.Data) > 0 {
			files[v.SHA256] = v.Data
			v.Data = nil
		}
		apart.Attachments[id] = v
	}
	b, err = marshalSnapshot(snapshotFile{Backup: newBackup(apart), Sync: &meta, Files: SnapshotFilesDir}, false)
	return b, files, err
}

// DecodeSnapshotWith is DecodeSnapshot for a snapshot that may keep its files
// apart, taking their bytes from f. A file none of f's sources holds makes it
// a *MissingFilesError, unless f.DropMissing.
func DecodeSnapshotWith(b []byte, f SnapshotFiles) (Data, SyncMeta, error) {
	var v snapshotFile
	if e := json.Unmarshal(b, &v); e != nil {
		return Data{}, SyncMeta{}, errors.New("invalid_backup")
	}
	if v.Files != "" {
		if e := v.Data.fillFiles(f); e != nil {
			return Data{}, SyncMeta{}, e
		}
	}
	return v.checked()
}

// fillFiles puts back the bytes of the attachments a snapshot left out.
func (d *Data) fillFiles(f SnapshotFiles) error {
	var missing []string
	for id, v := range d.Attachments {
		if v.URL != "" || len(v.Data) > 0 || v.SHA256 == emptySHA {
			continue // a link, a file kept inline, or an empty file
		}
		var found bool
		for _, source := range f.Sources {
			if v.Data, found = source(v.SHA256); found {
				break
			}
		}
		switch {
		case found:
			d.Attachments[id] = v
		case f.DropMissing:
			delete(d.Attachments, id)
		default:
			missing = append(missing, v.SHA256)
		}
	}
	if len(missing) > 0 {
		slices.Sort(missing)
		return &MissingFilesError{slices.Compact(missing)}
	}
	return nil
}

var emptySHA = func() string { sum := sha256.Sum256(nil); return hex.EncodeToString(sum[:]) }()

// ValidSHA is a checksum as Flow writes them, which is also a safe file name.
func ValidSHA(s string) bool {
	if len(s) != 64 {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil && strings.ToLower(s) == s
}

// FilesIn is the files stored in dir. A file whose bytes do not match its
// name, as one cut short, is not held.
func FilesIn(dir string) FileSource {
	return func(sha string) ([]byte, bool) {
		if !ValidSHA(sha) {
			return nil, false
		}
		f, err := os.Open(filepath.Join(dir, sha))
		if err != nil {
			return nil, false
		}
		defer f.Close()
		b, err := io.ReadAll(io.LimitReader(f, MaxAttachmentBytes+1))
		if err != nil || len(b) > MaxAttachmentBytes {
			return nil, false
		}
		if sum := sha256.Sum256(b); hex.EncodeToString(sum[:]) != sha {
			return nil, false
		}
		return b, true
	}
}

// FilesOf is the files attached in d.
func FilesOf(d Data) FileSource {
	return func(sha string) ([]byte, bool) {
		for _, v := range d.Attachments {
			if v.SHA256 == sha && v.URL == "" && len(v.Data) > 0 {
				return v.Data, true
			}
		}
		return nil, false
	}
}

// StoreFiles writes into dir each of files not there already. A file that
// iCloud has removed from this Mac to save space, leaving its placeholder,
// is there already.
func StoreFiles(dir string, files map[string][]byte) error {
	for sha, b := range files {
		if !ValidSHA(sha) {
			return errors.New("invalid_attachment")
		}
		if HasFile(dir, sha, int64(len(b))) {
			continue
		}
		if err := writeFileAtomic(filepath.Join(dir, sha), b); err != nil {
			return err
		}
	}
	return nil
}

// HasFile reports whether dir holds the file with a checksum: whole, at its
// size, or as iCloud's placeholder for it.
func HasFile(dir, sha string, size int64) bool {
	if info, err := os.Stat(filepath.Join(dir, sha)); err == nil && info.Size() == size {
		return true
	}
	_, err := os.Stat(ICloudPlaceholder(filepath.Join(dir, sha)))
	return err == nil
}

// ICloudPlaceholder is the name iCloud gives path while the file is in
// iCloud only: ".<name>.icloud" in the same folder.
func ICloudPlaceholder(path string) string {
	return filepath.Join(filepath.Dir(path), "."+filepath.Base(path)+".icloud")
}

// SnapshotFileNames lists the checksums of the files a snapshot names,
// without decoding the rest of it, for pruning a Files folder.
func SnapshotFileNames(b []byte) ([]string, error) {
	var v struct {
		Data struct {
			Attachments map[string]struct {
				URL    string `json:"url"`
				SHA256 string `json:"sha256"`
			} `json:"attachments"`
		} `json:"data"`
	}
	if err := json.Unmarshal(b, &v); err != nil {
		return nil, errors.New("invalid_backup")
	}
	var out []string
	for _, a := range v.Data.Attachments {
		if a.URL == "" && a.SHA256 != "" {
			out = append(out, a.SHA256)
		}
	}
	return out, nil
}
