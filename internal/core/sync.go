package core

import (
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"slices"
)

// Devices share a workspace through a folder, iCloud Drive's or any other,
// without sharing the live database: SQLite in a synced folder is corrupted
// by two Macs writing it, and the file-level conflicts of a synced folder
// keep one side's file whole and drop the other. Each device keeps its own
// flow.db and writes its whole workspace, as a backup, to a file only it
// writes. It takes in another device's changes with Merge, against the copy
// of that device's file it merged last.

// SyncMeta names the device that wrote a sync snapshot.
type SyncMeta struct {
	Device     string `json:"device"`
	DeviceName string `json:"deviceName"`
	WrittenAt  int64  `json:"writtenAt"`
}

// snapshotFile is a backup with, for a sync snapshot, the device that wrote
// it. Older readers ignore the field, so every snapshot is also a backup
// that can be restored by hand.
type snapshotFile struct {
	Backup
	Sync  *SyncMeta `json:"sync,omitempty"`
	Files string    `json:"files,omitempty"` // where the files left out are (snapshot_files.go)
}

// EncodeSnapshot is the sync snapshot of d, written by the device in meta.
func EncodeSnapshot(d Data, meta SyncMeta) ([]byte, error) {
	return encodeSnapshot(d, &meta, false)
}

// WriteFile replaces path with b atomically.
func WriteFile(path string, b []byte) error { return writeFileAtomic(path, b) }

// Merge combines the changes local and remote each made since base, the
// version of remote that local last took in (Empty when it never did).
//
// It works record by record (an issue, a comment, a label…) and inside a
// record field by field, so that a title changed on one Mac and a status on
// the other both survive. When both changed the same field, the record
// edited last wins; records without a time, and ties, are settled by
// comparing their contents, so that every device makes the same choice and
// they agree after one exchange. A record removed on one side and changed on
// the other is kept. Issues and projects are removed only when deleted for
// good from the trash, which leaves an Erasure behind: one side no longer
// having one is otherwise no reason to drop it.
//
// The merged workspace is then repaired where the two sides' changes do not
// fit together (an issue numbered on both Macs, a label removed on one and
// added to an issue on the other…). conflicts counts the records both sides
// changed. An error means the result is still not a valid workspace.
func Merge(base, local, remote Data) (merged Data, conflicts int, err error) {
	fb, err := flatten(base)
	if err != nil {
		return Data{}, 0, err
	}
	fl, err := flatten(local)
	if err != nil {
		return Data{}, 0, err
	}
	fr, err := flatten(remote)
	if err != nil {
		return Data{}, 0, err
	}
	keys := map[recordKey]bool{}
	for _, f := range []flatData{fb, fl, fr} {
		for k := range f {
			keys[k] = true
		}
	}
	out := flatData{}
	for k := range keys {
		b, l, r := fb[k], fl[k], fr[k]
		var v json.RawMessage
		switch {
		case bytes.Equal(l, r):
			v = l
		case bytes.Equal(l, b):
			v = r
		case bytes.Equal(r, b):
			v = l
		case l == nil:
			v = r
		case r == nil:
			v = l
		default:
			v = mergeRecord(b, l, r)
			conflicts++
		}
		if v == nil && (k.kind == "issue" || k.kind == "project") && fl[recordKey{"erased", k.id}] == nil && fr[recordKey{"erased", k.id}] == nil {
			v = l
			if v == nil {
				v = r
			}
		}
		if v != nil {
			out[k] = v
		}
	}
	merged, err = unflatten(out)
	if err != nil {
		return Data{}, conflicts, err
	}
	merged.Preferences.NextNumber = max(merged.Preferences.NextNumber, local.Preferences.NextNumber, remote.Preferences.NextNumber)
	merged.applyErasures()
	merged.repairMerged()
	if err = merged.Validate(); err != nil {
		return Data{}, conflicts, err
	}
	return merged, conflicts, nil
}

type flatData map[recordKey]json.RawMessage

// flatten lists every record of d by kind and ID, the preferences as one
// record. Encoding a struct always yields the same bytes for the same value,
// so equal records compare equal byte for byte.
func flatten(d Data) (flatData, error) {
	out := flatData{}
	add := func(kind, id string, v any) error {
		b, e := json.Marshal(v)
		out[recordKey{kind, id}] = b
		return e
	}
	for id, v := range d.Projects {
		if e := add("project", id, v); e != nil {
			return nil, e
		}
	}
	for id, v := range d.Issues {
		if e := add("issue", id, v); e != nil {
			return nil, e
		}
	}
	for id, v := range d.Labels {
		if e := add("label", id, v); e != nil {
			return nil, e
		}
	}
	for id, v := range d.Views {
		if e := add("view", id, v); e != nil {
			return nil, e
		}
	}
	for k, v := range d.records() {
		if e := add(k.kind, k.id, v); e != nil {
			return nil, e
		}
	}
	return out, add("preferences", "", d.Preferences)
}

func unflatten(f flatData) (Data, error) {
	d := Empty()
	for k, raw := range f {
		var e error
		switch k.kind {
		case "project":
			var v Project
			e = json.Unmarshal(raw, &v)
			d.Projects[k.id] = v
		case "issue":
			var v Issue
			e = json.Unmarshal(raw, &v)
			d.Issues[k.id] = v
		case "label":
			var v Label
			e = json.Unmarshal(raw, &v)
			d.Labels[k.id] = v
		case "view":
			var v SavedView
			e = json.Unmarshal(raw, &v)
			d.Views[k.id] = v
		case "preferences":
			var v Preferences
			e = json.Unmarshal(raw, &v)
			d.Preferences = v
		default:
			e = d.readRecord(k.kind, k.id, raw)
		}
		if e != nil {
			return Data{}, e
		}
	}
	d.normalizeJSON()
	return d, nil
}

// mergeRecord merges one record both sides changed, field by field.
func mergeRecord(base, local, remote json.RawMessage) json.RawMessage {
	remoteWins := newer(remote, local)
	var b, l, r map[string]json.RawMessage
	if json.Unmarshal(local, &l) != nil || json.Unmarshal(remote, &r) != nil {
		if remoteWins {
			return remote
		}
		return local
	}
	_ = json.Unmarshal(base, &b) // a record new on both sides has no base
	out := map[string]json.RawMessage{}
	for _, m := range []map[string]json.RawMessage{b, l, r} {
		for field := range m {
			bv, lv, rv := b[field], l[field], r[field]
			var v json.RawMessage
			switch {
			case bytes.Equal(lv, rv):
				v = lv
			case bytes.Equal(lv, bv):
				v = rv
			case bytes.Equal(rv, bv):
				v = lv
			case remoteWins:
				v = rv
			default:
				v = lv
			}
			if v != nil {
				out[field] = v
			}
		}
	}
	merged, e := json.Marshal(out)
	if e != nil {
		return local
	}
	return merged
}

// newer reports whether a was edited after b, by their updatedAt when both
// records have one, else by their contents, the same way on every device.
func newer(a, b json.RawMessage) bool {
	var ta, tb struct {
		UpdatedAt int64 `json:"updatedAt"`
	}
	_ = json.Unmarshal(a, &ta)
	_ = json.Unmarshal(b, &tb)
	if ta.UpdatedAt != tb.UpdatedAt {
		return ta.UpdatedAt > tb.UpdatedAt
	}
	return bytes.Compare(a, b) > 0
}

// repairMerged makes the records of a merge fit together again where each
// side's changes were valid alone. Every rule depends only on the merged
// records, so the devices repair alike.
func (d *Data) repairMerged() {
	// An issue created on two Macs before they synced has the same number on
	// both. The one created first keeps it; the others are numbered anew.
	ids := make([]string, 0, len(d.Issues))
	highest := 0
	for id, i := range d.Issues {
		ids = append(ids, id)
		highest = max(highest, i.Number)
	}
	slices.SortFunc(ids, func(a, b string) int {
		return cmp.Or(cmp.Compare(d.Issues[a].CreatedAt, d.Issues[b].CreatedAt), cmp.Compare(a, b))
	})
	next := max(d.Preferences.NextNumber, highest+1)
	taken := map[int]bool{}
	for _, id := range ids {
		i := d.Issues[id]
		if i.Number < 1 || taken[i.Number] {
			i.Number = next
			next++
			d.Issues[id] = i
		}
		taken[i.Number] = true
	}
	d.Preferences.NextNumber = next

	for _, id := range ids {
		i := d.Issues[id]
		if p, ok := d.Projects[i.ProjectID]; i.ProjectID != "" && !ok {
			i.ProjectID = ""
		} else if ok && p.DeletedAt != 0 && i.DeletedAt == 0 {
			i.DeletedAt = p.DeletedAt
		}
		i.LabelIDs = slices.DeleteFunc(slices.Clone(i.LabelIDs), func(l string) bool {
			label, ok := d.Labels[l]
			return !ok || label.EntityKind() != "issue" || label.ProjectID != "" && label.ProjectID != i.ProjectID
		})
		if m, ok := d.Milestones[i.MilestoneID]; i.MilestoneID != "" && (!ok || m.ProjectID != i.ProjectID) {
			i.MilestoneID = ""
		}
		// One Mac moved the start and the other the due date past it.
		i.NormalizeDates()
		d.Issues[id] = i
	}
	for id, p := range d.Projects {
		if p.Start != "" && p.Due != "" && p.Start > p.Due {
			p.Start = ""
			d.Projects[id] = p
		}
	}
	// A parent moved or trashed on one Mac and a sub-issue on the other: the
	// sub-issue follows its parent into the trash, or leaves it.
	for changed := true; changed; {
		changed = false
		for _, id := range ids {
			i := d.Issues[id]
			if i.ParentID == "" {
				continue
			}
			p, ok := d.Issues[i.ParentID]
			switch {
			case !ok || p.ID == i.ID || p.ProjectID != i.ProjectID:
				i.ParentID = ""
			case p.DeletedAt != 0 && i.DeletedAt == 0:
				i.DeletedAt = p.DeletedAt
			default:
				continue
			}
			d.Issues[id] = i
			changed = true
		}
	}
	for _, id := range ids {
		// Each Mac set a parent that makes the two issues each other's
		// ancestor: the issue walked from first lets go.
		seen := map[string]bool{id: true}
		for at := d.Issues[id].ParentID; at != ""; at = d.Issues[at].ParentID {
			if seen[at] {
				i := d.Issues[id]
				i.ParentID = ""
				d.Issues[id] = i
				break
			}
			seen[at] = true
		}
	}

	for id, p := range d.Projects {
		seen := map[string]bool{}
		p.Dependencies = slices.DeleteFunc(slices.Clone(p.Dependencies), func(other string) bool {
			_, ok := d.Projects[other]
			drop := !ok || other == id || seen[other]
			seen[other] = true
			return drop
		})
		d.Projects[id] = p
	}
	for id, v := range d.Comments {
		if parent, ok := d.Comments[v.ParentID]; v.ParentID != "" && (!ok || parent.IssueID != v.IssueID || v.ParentID == id) {
			v.ParentID = ""
			d.Comments[id] = v
		}
	}
	// The same relation added on both Macs: the first one stays.
	relations := make([]Relation, 0, len(d.Relations))
	for _, v := range d.Relations {
		relations = append(relations, v)
	}
	slices.SortFunc(relations, func(a, b Relation) int {
		return cmp.Or(cmp.Compare(a.CreatedAt, b.CreatedAt), cmp.Compare(a.ID, b.ID))
	})
	keys, duplicates := map[string]bool{}, map[string]bool{}
	for _, v := range relations {
		from, to := v.From, v.To
		if v.Kind == "related" && from > to {
			from, to = to, from
		}
		key := v.Kind + "/" + from + "/" + to
		if keys[key] || v.Kind == "duplicate" && duplicates[v.From] {
			delete(d.Relations, v.ID)
			continue
		}
		keys[key] = true
		if v.Kind == "duplicate" {
			duplicates[v.From] = true
		}
	}
	// A file added to a comment, document or draft another Mac removed.
	for id, v := range d.Attachments {
		if !d.attachmentOwnerExists(v.OwnerKind, v.OwnerID) {
			delete(d.Attachments, id)
		}
	}
	for id, v := range d.Folders {
		seen := map[FavoriteRef]bool{}
		v.Items = slices.DeleteFunc(slices.Clone(v.Items), func(ref FavoriteRef) bool {
			_, view := d.Views[ref.ID]
			drop := seen[ref] || !(d.ownerExists(ref.Kind, ref.ID) || ref.Kind == "view" && view)
			seen[ref] = true
			return drop
		})
		d.Folders[id] = v
	}
}

// Apply replaces the workspace with what fn makes of it, as Restore does,
// but without a backup or change history: for changes already recorded
// where they were made, as those another device synced. changed reports
// whether anything differs.
func (s *Store) Apply(fn func(Data) (Data, error)) (d Data, changed bool, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := diskLock(s.dir)
	if err != nil {
		return Data{}, false, err
	}
	defer unlock()
	if err = s.reload(); err != nil {
		return Data{}, false, err
	}
	next, err := fn(s.data.Clone())
	if err != nil {
		return Data{}, false, err
	}
	next.initRecords()
	next.normalizeJSON()
	if reflect.DeepEqual(next, s.data) {
		return next, false, nil
	}
	if err = next.Validate(); err != nil {
		return Data{}, false, err
	}
	if err = s.persist(next, true); err != nil {
		return Data{}, false, err
	}
	s.data = next
	s.revision++
	return next.Clone(), true, nil
}

// CopyTo writes a copy of the database to path, a file that must not exist
// yet. Every Flow writer commits under the workspace lock, which CopyTo
// holds, so the file is whole while it is read; a journal left by a writer
// that crashed mid-commit goes along, for SQLite to roll back on opening.
func (s *Store) CopyTo(path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := diskLock(s.dir)
	if err != nil {
		return err
	}
	defer unlock()
	if _, err = os.Stat(path); err == nil {
		return errors.New("copy_exists")
	}
	source := s.path
	if err = copyFile(source+"-journal", path+"-journal"); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return copyFile(source, path)
}

func copyFile(from, to string) error {
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer in.Close()
	if err = os.MkdirAll(filepath.Dir(to), 0700); err != nil {
		return err
	}
	out, err := os.CreateTemp(filepath.Dir(to), ".flow-copy-*")
	if err != nil {
		return err
	}
	name := out.Name()
	defer os.Remove(name)
	if _, err = io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err = out.Sync(); err != nil {
		out.Close()
		return err
	}
	if err = out.Close(); err != nil {
		return err
	}
	return os.Rename(name, to)
}
