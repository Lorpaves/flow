package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"time"

	"github.com/egoist/mygo/plugins/sqlite"
)

type Store struct {
	mu          sync.Mutex
	db          *sqlite.DB
	data        Data
	dir, path   string
	diskVersion int64
	revision    int64 // counts changes to data, whoever made them
}

// Version 9 adds template-owned media; older writers must not validate it away.
const schemaVersion = 9

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	unlock, err := diskLock(filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	defer unlock()
	db, err := sqlite.Open(context.Background(), path, sqlite.OpenOptions{})
	if err != nil {
		return nil, err
	}
	s := &Store{db: db, dir: filepath.Dir(path), path: path, data: Empty()}
	fail := func(e error) (*Store, error) { db.Close(); return nil, e }
	r, err := db.Query(context.Background(), "PRAGMA user_version")
	if err != nil {
		return fail(err)
	}
	if len(r.Values) > 0 && r.Values[0][0].(int64) > schemaVersion {
		return fail(errors.New("newer_database"))
	}
	stmts := []sqlite.Statement{
		{SQL: "CREATE TABLE IF NOT EXISTS projects (id TEXT PRIMARY KEY, data TEXT NOT NULL)"},
		{SQL: "CREATE TABLE IF NOT EXISTS issues (id TEXT PRIMARY KEY, number INTEGER NOT NULL UNIQUE, project_id TEXT REFERENCES projects(id) DEFERRABLE INITIALLY DEFERRED, parent_id TEXT REFERENCES issues(id) DEFERRABLE INITIALLY DEFERRED, data TEXT NOT NULL)"},
		{SQL: "CREATE TABLE IF NOT EXISTS labels (id TEXT PRIMARY KEY, data TEXT NOT NULL)"},
		{SQL: "CREATE TABLE IF NOT EXISTS settings (id TEXT PRIMARY KEY, data TEXT NOT NULL)"},
		{SQL: "CREATE TABLE IF NOT EXISTS views (id TEXT PRIMARY KEY, data TEXT NOT NULL)"},
		{SQL: "CREATE TABLE IF NOT EXISTS records (kind TEXT NOT NULL, id TEXT NOT NULL, data TEXT NOT NULL, PRIMARY KEY(kind,id))"},
		{SQL: "PRAGMA user_version=9"},
	}
	if _, err = db.Transaction(context.Background(), stmts); err != nil {
		return fail(err)
	}
	if err = s.reload(); err != nil {
		return fail(err)
	}
	next := s.data.Clone()
	if err = next.isolateTemplateMedia(); err != nil {
		return fail(err)
	}
	if !reflect.DeepEqual(next, s.data) {
		if err = next.Validate(); err != nil {
			return fail(err)
		}
		if err = s.persist(next, false); err != nil {
			return fail(err)
		}
		s.data = next
	}
	return s, nil
}

// Caller holds both the process mutex (after Open) and the workspace disk lock.
func (s *Store) reload() error {
	r, err := s.db.Query(context.Background(), "PRAGMA data_version")
	if err != nil {
		return err
	}
	version := r.Values[0][0].(int64)
	if s.diskVersion == version {
		return nil
	}
	data := Empty()
	for _, table := range []string{"projects", "issues", "labels", "settings", "views"} {
		r, e := s.db.Query(context.Background(), "SELECT data FROM "+table)
		if e != nil {
			return e
		}
		for _, row := range r.Values {
			raw := []byte(row[0].(string))
			switch table {
			case "views":
				var v SavedView
				e = json.Unmarshal(raw, &v)
				data.Views[v.ID] = v
			case "projects":
				var v Project
				e = json.Unmarshal(raw, &v)
				data.Projects[v.ID] = v
			case "issues":
				var v Issue
				e = json.Unmarshal(raw, &v)
				data.Issues[v.ID] = v
			case "labels":
				var v Label
				e = json.Unmarshal(raw, &v)
				data.Labels[v.ID] = v
			case "settings":
				e = json.Unmarshal(raw, &data.Preferences)
			}
			if e != nil {
				return e
			}
		}
	}
	r, err = s.db.Query(context.Background(), "SELECT kind,id,data FROM records")
	if err != nil {
		return err
	}
	for _, row := range r.Values {
		if err = data.readRecord(row[0].(string), row[1].(string), []byte(row[2].(string))); err != nil {
			return err
		}
	}
	data.normalizeJSON()
	if err = data.Validate(); err != nil {
		return err
	}
	s.data, s.diskVersion = data, version
	s.revision++
	return nil
}
func (s *Store) Close() error { s.mu.Lock(); defer s.mu.Unlock(); return s.db.Close() }

// Revision changes whenever the workspace this store holds does, whether
// this connection changed it or another did.
func (s *Store) Revision() int64 { s.mu.Lock(); defer s.mu.Unlock(); return s.revision }

// Dir is the workspace folder holding the database and its backups.
func (s *Store) Dir() string    { return s.dir }
func (s *Store) Snapshot() Data { s.mu.Lock(); defer s.mu.Unlock(); return s.data.Clone() }
func (s *Store) Refresh() (Data, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := diskLock(s.dir)
	if err != nil {
		return Data{}, false, err
	}
	defer unlock()
	before := s.diskVersion
	if err = s.reload(); err != nil {
		return Data{}, false, err
	}
	return s.data.Clone(), before != s.diskVersion, nil
}
func (s *Store) Update(fn func(*Data) error) (Data, error) {
	return s.UpdateFrom("desktop", fn)
}
func (s *Store) UpdateFrom(source string, fn func(*Data) error) (Data, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := diskLock(s.dir)
	if err != nil {
		return Data{}, err
	}
	defer unlock()
	if err = s.reload(); err != nil {
		return Data{}, err
	}
	next := s.data.Clone()
	if err := fn(&next); err != nil {
		return Data{}, err
	}
	next.normalizeJSON()
	if reflect.DeepEqual(next, s.data) {
		return next.Clone(), nil
	}
	next.RecordChanges(s.data, source, time.Now())
	if err := next.Validate(); err != nil {
		return Data{}, err
	}
	if err := s.persist(next, false); err != nil {
		return Data{}, err
	}
	s.data = next
	s.revision++
	return next.Clone(), nil
}
func nullID(id string) any {
	if id == "" {
		return nil
	}
	return id
}
func (s *Store) persist(d Data, replace bool) error {
	stmts := []sqlite.Statement{}
	old := s.data
	if replace {
		stmts = append(stmts, sqlite.Statement{SQL: "DELETE FROM records"})
		stmts = append(stmts, sqlite.Statement{SQL: "DELETE FROM issues"}, sqlite.Statement{SQL: "DELETE FROM projects"}, sqlite.Statement{SQL: "DELETE FROM labels"}, sqlite.Statement{SQL: "DELETE FROM views"})
		old = Empty()
	}
	appendRow := func(table, id string, v any) error {
		b, e := json.Marshal(v)
		if e != nil {
			return e
		}
		stmts = append(stmts, sqlite.Statement{SQL: "INSERT INTO " + table + "(id,data) VALUES(?,?) ON CONFLICT(id) DO UPDATE SET data=excluded.data", Args: []any{id, string(b)}})
		return nil
	}
	for id, p := range d.Projects {
		if !reflect.DeepEqual(old.Projects[id], p) {
			if e := appendRow("projects", id, p); e != nil {
				return e
			}
		}
	}
	for id, v := range d.Views {
		if !reflect.DeepEqual(old.Views[id], v) {
			if e := appendRow("views", id, v); e != nil {
				return e
			}
		}
	}
	for id := range old.Views {
		if _, ok := d.Views[id]; !ok {
			stmts = append(stmts, sqlite.Statement{SQL: "DELETE FROM views WHERE id=?", Args: []any{id}})
		}
	}
	for id, l := range d.Labels {
		if !reflect.DeepEqual(old.Labels[id], l) {
			if e := appendRow("labels", id, l); e != nil {
				return e
			}
		}
	}
	for id, i := range d.Issues {
		if !reflect.DeepEqual(old.Issues[id], i) {
			b, e := json.Marshal(i)
			if e != nil {
				return e
			}
			stmts = append(stmts, sqlite.Statement{SQL: "INSERT INTO issues(id,number,project_id,parent_id,data) VALUES(?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET number=excluded.number,project_id=excluded.project_id,parent_id=excluded.parent_id,data=excluded.data", Args: []any{id, i.Number, nullID(i.ProjectID), nullID(i.ParentID), string(b)}})
		}
	}
	for id := range old.Labels {
		if _, ok := d.Labels[id]; !ok {
			stmts = append(stmts, sqlite.Statement{SQL: "DELETE FROM labels WHERE id=?", Args: []any{id}})
		}
	}
	previous, records := old.records(), d.records()
	for key, v := range records {
		if !reflect.DeepEqual(previous[key], v) {
			raw, e := json.Marshal(v)
			if e != nil {
				return e
			}
			stmts = append(stmts, sqlite.Statement{SQL: "INSERT INTO records(kind,id,data) VALUES(?,?,?) ON CONFLICT(kind,id) DO UPDATE SET data=excluded.data", Args: []any{key.kind, key.id, string(raw)}})
		}
	}
	for key := range previous {
		if _, ok := records[key]; !ok {
			stmts = append(stmts, sqlite.Statement{SQL: "DELETE FROM records WHERE kind=? AND id=?", Args: []any{key.kind, key.id}})
		}
	}
	if replace || !reflect.DeepEqual(old.Preferences, d.Preferences) {
		if e := appendRow("settings", "preferences", d.Preferences); e != nil {
			return e
		}
	}
	if len(stmts) == 0 {
		return nil
	}
	_, e := s.db.Transaction(context.Background(), stmts)
	return e
}

type Backup struct {
	Format    string `json:"format"`
	Version   int    `json:"version"`
	CreatedAt string `json:"createdAt"`
	Data      Data   `json:"data"`
}

func WriteBackup(path string, d Data) error {
	b, e := encodeSnapshot(d, nil, true)
	if e != nil {
		return e
	}
	return writeFileAtomic(path, b)
}

// encodeSnapshot is a backup of d, with the device that wrote it when it is
// a sync snapshot. Backups are indented for people; snapshots are compact.
func encodeSnapshot(d Data, meta *SyncMeta, indent bool) ([]byte, error) {
	if e := d.Validate(); e != nil {
		return nil, e
	}
	return marshalSnapshot(snapshotFile{Backup: newBackup(d), Sync: meta}, indent)
}

func newBackup(d Data) Backup {
	return Backup{"flow-backup", schemaVersion, time.Now().UTC().Format(time.RFC3339), d}
}

func marshalSnapshot(v snapshotFile, indent bool) ([]byte, error) {
	var b []byte
	var e error
	if indent {
		b, e = json.MarshalIndent(v, "", "  ")
	} else {
		b, e = json.Marshal(v)
	}
	if e != nil {
		return nil, e
	}
	if len(b) > MaxBackupBytes {
		return nil, errors.New("backup_too_large")
	}
	return b, nil
}

// writeFileAtomic replaces path with b in one step, so a reader, or iCloud,
// never sees half a file.
func writeFileAtomic(path string, b []byte) error {
	if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		return e
	}
	f, e := os.CreateTemp(filepath.Dir(path), ".flow-backup-*")
	if e != nil {
		return e
	}
	name := f.Name()
	defer os.Remove(name)
	if _, e = f.Write(b); e != nil {
		f.Close()
		return e
	}
	if e = f.Sync(); e != nil {
		f.Close()
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	return os.Rename(name, path)
}

// ReadBackup reads a backup, or a sync snapshot with the files it keeps
// beside it.
func ReadBackup(path string) (Data, error) {
	b, e := ReadLimited(path)
	if e != nil {
		return Data{}, e
	}
	d, _, e := DecodeSnapshotWith(b, SnapshotFiles{Sources: []FileSource{FilesIn(filepath.Join(filepath.Dir(path), SnapshotFilesDir))}})
	return d, e
}

// ReadLimited reads a backup or snapshot file, refusing one larger than any
// Flow writes.
func ReadLimited(path string) ([]byte, error) {
	f, e := os.Open(path)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	b, e := io.ReadAll(io.LimitReader(f, MaxBackupBytes+1))
	if e != nil {
		return nil, e
	}
	if len(b) > MaxBackupBytes {
		return nil, errors.New("backup_too_large")
	}
	return b, nil
}

// DecodeSnapshot reads a backup, and the device that wrote it when it is a
// sync snapshot (the zero SyncMeta otherwise). A snapshot that keeps its
// files apart needs DecodeSnapshotWith.
func DecodeSnapshot(b []byte) (Data, SyncMeta, error) {
	return DecodeSnapshotWith(b, SnapshotFiles{})
}

// checked is a decoded snapshot's workspace once it is known to be one.
func (v snapshotFile) checked() (Data, SyncMeta, error) {
	if v.Format != "flow-backup" || (v.Version < 1 || v.Version > schemaVersion) {
		return Data{}, SyncMeta{}, errors.New("invalid_backup")
	}
	if v.Data.Views == nil {
		v.Data.Views = map[string]SavedView{}
	}
	v.Data.initRecords()
	v.Data.normalizeJSON()
	if e := v.Data.isolateTemplateMedia(); e != nil {
		return Data{}, SyncMeta{}, fmt.Errorf("invalid_backup: %w", e)
	}
	if e := v.Data.Validate(); e != nil {
		return Data{}, SyncMeta{}, fmt.Errorf("invalid_backup: %w", e)
	}
	var meta SyncMeta
	if v.Sync != nil {
		meta = *v.Sync
	}
	return v.Data, meta, nil
}
func (s *Store) Backup(path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := diskLock(s.dir)
	if err != nil {
		return err
	}
	defer unlock()
	if err = s.reload(); err != nil {
		return err
	}
	return WriteBackup(path, s.data)
}
func (s *Store) Restore(path string) (Data, error) {
	d, e := ReadBackup(path)
	if e != nil {
		return Data{}, e
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, e := diskLock(s.dir)
	if e != nil {
		return Data{}, e
	}
	defer unlock()
	if e = s.reload(); e != nil {
		return Data{}, e
	}
	backup := filepath.Join(s.dir, "backups", "before-restore-"+time.Now().Format("20060102-150405.000000000")+".flow.json")
	if e = WriteBackup(backup, s.data); e != nil {
		return Data{}, e
	}
	if e = s.persist(d, true); e != nil {
		return Data{}, e
	}
	s.data = d
	s.revision++
	return d.Clone(), nil
}
