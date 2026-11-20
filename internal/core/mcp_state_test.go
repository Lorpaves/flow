package core

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMCPConfigRoundTripAndDefaults(t *testing.T) {
	dir := t.TempDir()
	cfg, err := ReadMCPConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.IsEnabled() || cfg.ReadOnly {
		t.Fatalf("unexpected default MCP config: %+v", cfg)
	}

	off := false
	if err := WriteMCPConfig(dir, MCPConfig{Enabled: &off, ReadOnly: true}); err != nil {
		t.Fatal(err)
	}
	got, err := ReadMCPConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.IsEnabled() || !got.ReadOnly {
		t.Fatalf("unexpected persisted MCP config: %+v", got)
	}
}

func TestMCPSessionsPruneStaleAndTrackLive(t *testing.T) {
	dir := t.TempDir()
	now := time.Now().UnixMilli()
	// Register a live session using the current test process PID.
	cleanup, err := RegisterMCPSession(dir, MCPSession{
		PID:       os.Getpid(),
		StartedAt: now,
		ReadOnly:  true,
		Version:   "0.1.3",
		Client:    "Claude",
	})
	if err != nil {
		t.Fatal(err)
	}
	// Write a stale session with an impossible PID that should be pruned.
	stalePath := filepath.Join(mcpSessionsDir(dir), "99999999.json")
	if err := writeFileAtomic(stalePath, []byte(`{"pid":99999999,"startedAt":1,"version":"0.1.0"}`)); err != nil {
		t.Fatal(err)
	}

	active, err := ActiveMCPSessions(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 1 || active[0].PID != os.Getpid() || !active[0].ReadOnly || active[0].Client != "Claude" {
		t.Fatalf("unexpected active sessions: %+v", active)
	}
	if _, err := os.Stat(stalePath); !os.IsNotExist(err) {
		t.Fatalf("stale session file was not pruned: err=%v", err)
	}

	cleanup()
	active, err = ActiveMCPSessions(dir)
	if err != nil || len(active) != 0 {
		t.Fatalf("expected 0 sessions after cleanup, got %+v err=%v", active, err)
	}
}
