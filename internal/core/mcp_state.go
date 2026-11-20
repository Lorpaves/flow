package core

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

const (
	mcpDirName      = "mcp"
	mcpConfigName   = "config.json"
	mcpSessionsName = "sessions"
)

// MCPConfig holds desktop-managed settings for local flow-mcp processes
// accessing a workspace.
type MCPConfig struct {
	// Enabled defaults to true when nil so existing MCP clients keep working.
	Enabled *bool `json:"enabled,omitempty"`
	// ReadOnly forces flow-mcp to expose only query tools even when the client
	// did not pass --read-only.
	ReadOnly bool `json:"readOnly,omitempty"`
}

func (c MCPConfig) IsEnabled() bool {
	return c.Enabled == nil || *c.Enabled
}

// MCPSession describes one live flow-mcp stdio process attached to a workspace.
type MCPSession struct {
	PID       int    `json:"pid"`
	StartedAt int64  `json:"startedAt"`
	ReadOnly  bool   `json:"readOnly"`
	Version   string `json:"version"`
	Binary    string `json:"binary,omitempty"`
	Client    string `json:"client,omitempty"`
}

func mcpConfigPath(dir string) string {
	return filepath.Join(dir, mcpDirName, mcpConfigName)
}

func mcpSessionsDir(dir string) string {
	return filepath.Join(dir, mcpDirName, mcpSessionsName)
}

// ReadMCPConfig loads workspace MCP settings, returning default settings when
// no config file exists yet.
func ReadMCPConfig(dir string) (MCPConfig, error) {
	if dir == "" {
		return MCPConfig{}, nil
	}
	f, err := os.Open(mcpConfigPath(dir))
	if errors.Is(err, os.ErrNotExist) {
		return MCPConfig{}, nil
	}
	if err != nil {
		return MCPConfig{}, err
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, 1<<20+1))
	if err != nil {
		return MCPConfig{}, err
	}
	var cfg MCPConfig
	if len(raw) > 1<<20 || json.Unmarshal(raw, &cfg) != nil {
		return MCPConfig{}, errors.New("invalid_mcp_config")
	}
	return cfg, nil
}

// WriteMCPConfig persists workspace MCP settings atomically.
func WriteMCPConfig(dir string, cfg MCPConfig) error {
	if dir == "" {
		return nil
	}
	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(mcpConfigPath(dir), append(raw, '\n'))
}

// RegisterMCPSession records a live flow-mcp process under <dir>/mcp/sessions
// and returns a cleanup callback that removes its presence file on exit.
func RegisterMCPSession(dir string, s MCPSession) (func(), error) {
	if dir == "" || s.PID <= 0 {
		return func() {}, nil
	}
	path := filepath.Join(mcpSessionsDir(dir), strconv.Itoa(s.PID)+".json")
	raw, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return func() {}, err
	}
	if err := writeFileAtomic(path, append(raw, '\n')); err != nil {
		return func() {}, err
	}
	return func() { _ = os.Remove(path) }, nil
}

// ActiveMCPSessions returns all live flow-mcp sessions attached to dir and
// automatically removes stale presence files from exited processes.
func ActiveMCPSessions(dir string) ([]MCPSession, error) {
	if dir == "" {
		return nil, nil
	}
	sessDir := mcpSessionsDir(dir)
	entries, err := os.ReadDir(sessDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []MCPSession
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		path := filepath.Join(sessDir, e.Name())
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var s MCPSession
		if json.Unmarshal(raw, &s) != nil || s.PID <= 0 || !mcpProcessAlive(s.PID) {
			_ = os.Remove(path)
			continue
		}
		out = append(out, s)
	}
	slices.SortFunc(out, func(a, b MCPSession) int {
		if a.StartedAt != b.StartedAt {
			if a.StartedAt < b.StartedAt {
				return -1
			}
			return 1
		}
		return a.PID - b.PID
	})
	return out, nil
}

// TerminateMCPSession gracefully asks a registered flow-mcp session to exit.
// Only PIDs with a matching session record under dir are ever signaled.
func TerminateMCPSession(dir string, pid int) error {
	if dir == "" || pid <= 0 {
		return nil
	}
	path := filepath.Join(mcpSessionsDir(dir), strconv.Itoa(pid)+".json")
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var s MCPSession
	if json.Unmarshal(raw, &s) != nil || s.PID != pid {
		_ = os.Remove(path)
		return nil
	}
	if !mcpProcessAlive(pid) {
		_ = os.Remove(path)
		return nil
	}
	return mcpTerminateProcess(pid)
}
