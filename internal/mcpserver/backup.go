package mcpserver

import (
	"context"
	"errors"
	"flow/internal/core"
	"os"
	"path/filepath"
	"time"
)

type BackupInput struct {
	Path string `json:"path,omitempty" jsonschema:"Absolute destination that must not exist yet; omitted writes a dated file to the workspace's backups folder"`
}
type RestoreInput struct {
	Path string `json:"path" jsonschema:"Absolute path of a Flow backup file"`
}

// createBackup writes the whole workspace, as the desktop's Export backup
// does, to a file that did not exist.
func (a *API) createBackup(_ context.Context, in BackupInput) (any, error) {
	path := in.Path
	if path == "" {
		path = filepath.Join(a.store.Dir(), "backups", "Flow-"+time.Now().Format("2006-01-02-150405")+".flow.json")
		if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
			return nil, e
		}
	}
	if !filepath.IsAbs(path) {
		return nil, errors.New("absolute_path_required")
	}
	// Taking the name first keeps an existing file from being replaced.
	f, e := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return nil, e
	}
	f.Close()
	if e = a.store.Backup(path); e != nil {
		os.Remove(path)
		return nil, e
	}
	info, e := os.Stat(path)
	if e != nil {
		return nil, e
	}
	return map[string]any{"path": path, "bytes": info.Size()}, nil
}

// restoreBackup replaces the workspace with a backup, as the desktop's
// Restore backup does, after saving the current one to the backups folder.
func (a *API) restoreBackup(_ context.Context, in RestoreInput) (any, error) {
	if !filepath.IsAbs(in.Path) {
		return nil, errors.New("absolute_path_required")
	}
	d, e := a.store.Restore(in.Path)
	if e != nil {
		return nil, e
	}
	return map[string]any{"restored": in.Path, "projects": len(d.ProjectList(false, false)), "issues": len(d.Query(core.Filter{})), "previousWorkspaceSavedIn": filepath.Join(a.store.Dir(), "backups")}, nil
}
