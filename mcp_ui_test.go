package main

import (
	"context"
	"errors"
	"flow/internal/core"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

type fakeMCPBackend struct {
	mu          sync.Mutex
	binary      string
	found       bool
	sessions    []core.MCPSession
	clients     []string
	probeFn     func(ctx context.Context, binary, dir string, readOnly bool) (mcpProbeResult, error)
	terminated  []int
	probeCalled int
}

func (b *fakeMCPBackend) Binary() (string, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.binary, b.found
}

func (b *fakeMCPBackend) Sessions(dir string) ([]core.MCPSession, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]core.MCPSession, len(b.sessions))
	copy(out, b.sessions)
	return out, nil
}

func (b *fakeMCPBackend) Terminate(dir string, pid int) error {
	b.mu.Lock()
	b.terminated = append(b.terminated, pid)
	kept := b.sessions[:0]
	for _, s := range b.sessions {
		if s.PID != pid {
			kept = append(kept, s)
		}
	}
	b.sessions = kept
	b.mu.Unlock()
	return nil
}

func (b *fakeMCPBackend) ConfiguredClients() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]string, len(b.clients))
	copy(out, b.clients)
	return out
}

func (b *fakeMCPBackend) Probe(ctx context.Context, binary, dir string, readOnly bool) (mcpProbeResult, error) {
	b.mu.Lock()
	b.probeCalled++
	fn := b.probeFn
	b.mu.Unlock()
	if fn != nil {
		return fn(ctx, binary, dir, readOnly)
	}
	if readOnly {
		return mcpProbeResult{Version: "0.1.3", Tools: mcpToolCountRead}, nil
	}
	return mcpProbeResult{Version: "0.1.3", Tools: mcpToolCountFull}, nil
}

func waitForMCPStatus(t *testing.T, svc *mcpService, want func(mcpStatus) bool) mcpStatus {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		st := svc.snapshot()
		if want(st) {
			return st
		}
		time.Sleep(10 * time.Millisecond)
	}
	st := svc.snapshot()
	t.Fatalf("timed out waiting for MCP status; last=%+v", st)
	return st
}

func TestMCPServiceStatePersistenceProbeAndDisconnect(t *testing.T) {
	dir := t.TempDir()
	backend := &fakeMCPBackend{
		binary: "/Applications/Flow.app/Contents/Resources/flow-mcp",
		found:  true,
		sessions: []core.MCPSession{
			{PID: 4242, StartedAt: time.Now().Add(-3 * time.Minute).UnixMilli(), ReadOnly: false, Version: "0.1.3", Client: "Claude"},
		},
		clients: []string{"Claude Desktop", "Antigravity"},
	}
	svc := newMCPService(dir, backend, nil)
	svc.pollInterval = time.Hour
	svc.start()
	defer svc.stop()

	st := waitForMCPStatus(t, svc, func(s mcpStatus) bool { return s.Loaded })
	if !st.Enabled || st.ReadOnly || !st.BinaryFound || len(st.Sessions) != 1 || len(st.ConfiguredApps) != 2 {
		t.Fatalf("unexpected initial MCP status: %+v", st)
	}

	svc.runTest()
	st = waitForMCPStatus(t, svc, func(s mcpStatus) bool { return !s.Testing && s.TestOK })
	if st.TestTools != mcpToolCountFull || st.TestVersion != "0.1.3" {
		t.Fatalf("unexpected probe result: %+v", st)
	}

	svc.setReadOnly(true)
	waitForMCPStatus(t, svc, func(s mcpStatus) bool { return s.ReadOnly })
	svc.runTest()
	st = waitForMCPStatus(t, svc, func(s mcpStatus) bool { return !s.Testing && s.TestTools == mcpToolCountRead })
	if !st.TestOK {
		t.Fatalf("expected read-only probe to pass: %+v", st)
	}

	svc.disconnectSession(4242)
	waitForMCPStatus(t, svc, func(s mcpStatus) bool { return len(s.Sessions) == 0 })

	svc.setEnabled(false)
	waitForMCPStatus(t, svc, func(s mcpStatus) bool { return !s.Enabled })
	cfg, err := core.ReadMCPConfig(dir)
	if err != nil || cfg.IsEnabled() || !cfg.ReadOnly {
		t.Fatalf("MCP config not persisted: %+v err=%v", cfg, err)
	}

	// Probe error reporting.
	svc.setEnabled(true)
	waitForMCPStatus(t, svc, func(s mcpStatus) bool { return s.Enabled })
	backend.mu.Lock()
	backend.probeFn = func(ctx context.Context, binary, dir string, readOnly bool) (mcpProbeResult, error) {
		return mcpProbeResult{}, errors.New("handshake failed")
	}
	backend.mu.Unlock()
	svc.runTest()
	st = waitForMCPStatus(t, svc, func(s mcpStatus) bool { return !s.Testing && s.TestErr != "" })
	if st.TestOK || !strings.Contains(st.TestErr, "handshake failed") {
		t.Fatalf("expected probe error, got %+v", st)
	}
}

func TestMCPSettingsAndDialogUIEndToEnd(t *testing.T) {
	d, a, _, _ := managedTestDesktop(t)
	backend := &fakeMCPBackend{
		binary: "/Applications/Flow.app/Contents/Resources/flow-mcp",
		found:  true,
		sessions: []core.MCPSession{
			{PID: 7777, StartedAt: time.Now().Add(-2 * time.Minute).UnixMilli(), ReadOnly: false, Version: "0.1.3", Client: "Cursor"},
		},
		clients: []string{"Cursor"},
	}
	svc := newMCPService(d.dataDir, backend, nil)
	svc.pollInterval = time.Hour
	d.mcp = svc
	svc.start()
	defer svc.stop()
	waitForMCPStatus(t, svc, func(s mcpStatus) bool { return s.Loaded })

	a.data.Preferences.Locale = "zh-Hans"
	a.navigate("settings", "")
	tt := ui.NewTester(a.view, 1280, 900)
	tt.Frame()

	// Toggle read-only mode in Settings.
	click(t, tt, "切换 MCP 只读模式")
	waitForMCPStatus(t, svc, func(s mcpStatus) bool { return s.ReadOnly })
	tt.Frame()

	// Open MCP dialog, test server, copy JSON, switch to Claude Code tab, copy CLI, disconnect session, close.
	click(t, tt, "配置 MCP")
	if !a.mcpOpen {
		t.Fatal("MCP dialog did not open")
	}
	tt.Frame()

	click(t, tt, "测试 MCP 连接")
	waitForMCPStatus(t, svc, func(s mcpStatus) bool { return !s.Testing && s.TestOK && s.TestTools == mcpToolCountRead })
	tt.Frame()

	click(t, tt, "复制 MCP 配置")
	tt.Frame()
	if got := tt.Clipboard(); !strings.Contains(got, `"flow"`) || !strings.Contains(got, `"--read-only"`) {
		t.Fatalf("clipboard missing MCP JSON config: %q", got)
	}

	for _, tc := range []struct {
		tab  string
		want string
	}{
		{"Claude", "claude mcp add --transport stdio flow --"},
		{"Codex", "codex mcp add flow --"},
		{"Antigravity", "agy mcp add flow --"},
		{"OpenCode", "opencode mcp add --global flow --"},
	} {
		click(t, tt, tc.tab)
		tt.Frame()
		click(t, tt, "复制 MCP 配置")
		tt.Frame()
		if got := tt.Clipboard(); !strings.Contains(got, tc.want) || !strings.Contains(got, "--read-only") {
			t.Fatalf("%s tab clipboard mismatch: got %q, want %q", tc.tab, got, tc.want)
		}
	}

	click(t, tt, "断开会话 7777")
	waitForMCPStatus(t, svc, func(s mcpStatus) bool { return len(s.Sessions) == 0 })
	tt.Frame()

	tt.Key(0, ui.KeyEscape)
	if a.mcpOpen {
		t.Fatal("Escape did not close MCP dialog")
	}
	tt.Frame()

	// Disable MCP toggle in Settings.
	click(t, tt, "切换 MCP 开关")
	waitForMCPStatus(t, svc, func(s mcpStatus) bool { return !s.Enabled })
	tt.Frame()
}
