package core

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// Flow keeps its workspace in DefaultDataDir unless the user moved it, in
// which case a note there, location.json, names the folder it moved to. The
// note stays in the default folder, the one place every Flow process (the
// app and the MCP server) looks first.

const locationFile = "location.json"

// DefaultDataDir is ~/Library/Application Support/Flow on macOS.
func DefaultDataDir() (string, error) {
	root, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "Flow"), nil
}

// DataLocation returns the folder the workspace was moved to from
// defaultDir, or "" when it was not moved.
func DataLocation(defaultDir string) (string, error) {
	b, err := os.ReadFile(filepath.Join(defaultDir, locationFile))
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	var v struct {
		DataDir string `json:"dataDir"`
	}
	if err = json.Unmarshal(b, &v); err != nil || v.DataDir != "" && !filepath.IsAbs(v.DataDir) {
		return "", errors.New("invalid_location")
	}
	return v.DataDir, nil
}

// SetDataLocation records that the workspace of defaultDir now lives in dir.
// Moving it back to defaultDir removes the note.
func SetDataLocation(defaultDir, dir string) error {
	path := filepath.Join(defaultDir, locationFile)
	if dir == "" || filepath.Clean(dir) == filepath.Clean(defaultDir) {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	if !filepath.IsAbs(dir) {
		return errors.New("invalid_location")
	}
	b, err := json.MarshalIndent(map[string]string{"dataDir": filepath.Clean(dir)}, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(path, b)
}

// ResolveDataDir is where the workspace of defaultDir is: the folder it was
// moved to, when that folder exists, else defaultDir. missing names a moved
// folder that is not there now, such as one on a disk that is not connected,
// which is never created in its place.
func ResolveDataDir(defaultDir string) (dir, missing string, err error) {
	moved, err := DataLocation(defaultDir)
	if err != nil || moved == "" {
		return defaultDir, "", err
	}
	if info, statErr := os.Stat(moved); statErr != nil || !info.IsDir() {
		return defaultDir, moved, nil
	}
	return moved, "", nil
}
