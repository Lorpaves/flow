package main

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
)

type desktopWindowState struct {
	ID      string `json:"id"`
	Visible bool   `json:"visible"`
}

type desktopWindowsState struct {
	Version    int                  `json:"version"`
	Next       int                  `json:"next"`
	Active     string               `json:"active"`
	Appearance string               `json:"appearance"`
	Windows    []desktopWindowState `json:"windows"`
}

func readDesktopWindows(dir string) (state desktopWindowsState, exists bool, err error) {
	f, err := os.Open(filepath.Join(dir, "desktop-windows.json"))
	if errors.Is(err, os.ErrNotExist) {
		return state, false, nil
	}
	if err != nil {
		return state, false, err
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, 1<<20+1))
	if err != nil {
		return state, false, err
	}
	if len(raw) > 1<<20 || json.Unmarshal(raw, &state) != nil || state.Version != 1 || len(state.Windows) == 0 || len(state.Windows) > 1000 || state.Next < 0 || state.Next > 1_000_000_000 {
		return state, false, errors.New("invalid_desktop_windows")
	}
	seen := map[string]bool{}
	for _, w := range state.Windows {
		if seen[w.ID] {
			return state, false, errors.New("invalid_desktop_windows")
		}
		seen[w.ID] = true
		if w.ID != "main" {
			n, e := strconv.Atoi(w.ID)
			if e != nil || n <= 0 || n > 1_000_000_000 || strconv.Itoa(n) != w.ID {
				return state, false, errors.New("invalid_desktop_windows")
			}
			state.Next = max(state.Next, n)
		}
	}
	if !seen["main"] || !seen[state.Active] || !validAppearance(state.Appearance) {
		return state, false, errors.New("invalid_desktop_windows")
	}
	return state, true, nil
}

func writeDesktopWindows(dir string, raw []byte) error {
	return writeDesktopFile(dir, "desktop-windows.json", raw)
}
