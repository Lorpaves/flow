package main

import (
	"context"
	"encoding/json"
	"errors"
	"flow/internal/core"
	"flow/internal/mcpserver"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	mcpPollInterval     = 2 * time.Second
	mcpToolCountFull    = 73
	mcpToolCountRead    = 24
	mcpTestTimeout      = 6 * time.Second
	mcpCanonicalAppPath = "/Applications/Flow.app/Contents/Resources/flow-mcp"
)

type mcpProbeResult struct {
	Version string
	Tools   int
}

type mcpBackend interface {
	Binary() (path string, found bool)
	Sessions(dir string) ([]core.MCPSession, error)
	Terminate(dir string, pid int) error
	ConfiguredClients() []string
	Probe(ctx context.Context, binary, dir string, readOnly bool) (mcpProbeResult, error)
}

type localMCPBackend struct{}

func (localMCPBackend) Binary() (string, bool) {
	return resolveMCPBinary()
}

func (localMCPBackend) Sessions(dir string) ([]core.MCPSession, error) {
	return core.ActiveMCPSessions(dir)
}

func (localMCPBackend) Terminate(dir string, pid int) error {
	return core.TerminateMCPSession(dir, pid)
}

func (localMCPBackend) ConfiguredClients() []string {
	return detectConfiguredMCPClients()
}

func (localMCPBackend) Probe(ctx context.Context, binary, dir string, readOnly bool) (mcpProbeResult, error) {
	if binary == "" {
		return mcpProbeResult{}, errors.New("mcp_binary_missing")
	}
	args := []string{}
	if dir != "" {
		args = append(args, "--data-dir", dir)
	}
	if readOnly {
		args = append(args, "--read-only")
	}
	cmd := exec.CommandContext(ctx, binary, args...)
	client := mcp.NewClient(&mcp.Implementation{Name: "flow-desktop-probe", Version: flowVersion()}, nil)
	cs, err := client.Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
	if err != nil {
		return mcpProbeResult{}, err
	}
	defer cs.Close()
	tools, err := cs.ListTools(ctx, nil)
	if err != nil {
		return mcpProbeResult{}, err
	}
	ver := mcpserver.Version
	if init := cs.InitializeResult(); init != nil && init.ServerInfo != nil && init.ServerInfo.Version != "" {
		ver = init.ServerInfo.Version
	}
	return mcpProbeResult{Version: ver, Tools: len(tools.Tools)}, nil
}

func resolveMCPBinary() (string, bool) {
	candidates := []string{}
	if resDir, err := mygo.App.Path(mygo.PathResources); err == nil && resDir != "" {
		candidates = append(candidates, filepath.Join(resDir, "flow-mcp"))
	}
	if exe, err := os.Executable(); err == nil && exe != "" {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), "..", "Resources", "flow-mcp"))
	}
	if wd, err := os.Getwd(); err == nil && wd != "" {
		candidates = append(candidates, filepath.Join(wd, ".mygo", "flow-mcp"))
	}
	candidates = append(candidates, mcpCanonicalAppPath)

	for _, c := range candidates {
		clean := filepath.Clean(c)
		if info, err := os.Stat(clean); err == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
			return clean, true
		}
	}
	if len(candidates) > 0 {
		return filepath.Clean(candidates[0]), false
	}
	return mcpCanonicalAppPath, false
}

// detectConfiguredMCPClients checks common local MCP client config files
// (read-only) to report which installed apps already configure a "flow" server.
func detectConfiguredMCPClients() []string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return nil
	}
	clients := []struct {
		name string
		path string
		toml bool
	}{
		{"Claude Desktop", filepath.Join(home, "Library", "Application Support", "Claude", "claude_desktop_config.json"), false},
		{"Claude Code", filepath.Join(home, ".claude.json"), false},
		{"Codex", filepath.Join(home, ".codex", "config.toml"), true},
		{"Antigravity", filepath.Join(home, ".gemini", "config", "mcp_config.json"), false},
		{"OpenCode", filepath.Join(home, ".config", "opencode", "opencode.json"), false},
		{"Cursor", filepath.Join(home, ".cursor", "mcp.json"), false},
	}
	var found []string
	for _, c := range clients {
		if c.toml {
			if hasFlowTOMLConfig(c.path) {
				found = append(found, c.name)
			}
			continue
		}
		if hasFlowMCPConfig(c.path) {
			found = append(found, c.name)
		}
	}
	return found
}

func hasFlowTOMLConfig(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, 2<<20))
	if err != nil {
		return false
	}
	text := string(raw)
	return strings.Contains(text, "[mcp_servers.flow]") || strings.Contains(text, "flow-mcp")
}

func hasFlowMCPConfig(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, 2<<20))
	if err != nil {
		return false
	}
	var doc struct {
		MCPServers map[string]json.RawMessage `json:"mcpServers"`
		MCP        map[string]json.RawMessage `json:"mcp"`
	}
	if json.Unmarshal(raw, &doc) != nil {
		return false
	}
	for _, group := range []map[string]json.RawMessage{doc.MCPServers, doc.MCP} {
		for k, v := range group {
			if strings.EqualFold(k, "flow") || strings.Contains(string(v), "flow-mcp") {
				return true
			}
		}
	}
	return false
}

type mcpStatus struct {
	Loaded         bool
	Enabled        bool
	ReadOnly       bool
	BinaryPath     string
	BinaryFound    bool
	Sessions       []core.MCPSession
	ConfiguredApps []string
	Testing        bool
	TestOK         bool
	TestVersion    string
	TestTools      int
	TestErr        string
	TestAt         time.Time
	Err            string
}

func (s mcpStatus) clone() mcpStatus {
	out := s
	out.Sessions = slices.Clone(s.Sessions)
	out.ConfiguredApps = slices.Clone(s.ConfiguredApps)
	return out
}

func (s mcpStatus) key() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%t|%t|%t|%s|%t|%t|%t|%s|%d|%s|%s|",
		s.Loaded, s.Enabled, s.ReadOnly, s.BinaryPath, s.BinaryFound,
		s.Testing, s.TestOK, s.TestVersion, s.TestTools, s.TestErr, s.Err)
	for _, sess := range s.Sessions {
		fmt.Fprintf(&b, "%d:%t:%s:%d;", sess.PID, sess.ReadOnly, sess.Client, sess.StartedAt)
	}
	b.WriteByte('|')
	b.WriteString(strings.Join(s.ConfiguredApps, ","))
	return b.String()
}

type mcpService struct {
	backend      mcpBackend
	dir          string
	publish      func(func())
	now          func() time.Time
	pollInterval time.Duration

	statusMu sync.RWMutex
	status   mcpStatus

	mu       sync.Mutex
	cfg      core.MCPConfig
	cfgGen   uint64
	testing  bool
	started  bool
	stopped  bool
	refresh  chan struct{}
	stopCh   chan struct{}
	wg       sync.WaitGroup
	testCanc context.CancelFunc
}

func newMCPService(dir string, backend mcpBackend, publish func(func())) *mcpService {
	if publish == nil {
		publish = func(fn func()) { fn() }
	}
	return &mcpService{
		backend:      backend,
		dir:          dir,
		publish:      publish,
		now:          time.Now,
		pollInterval: mcpPollInterval,
		refresh:      make(chan struct{}, 1),
		stopCh:       make(chan struct{}),
		status: mcpStatus{
			Enabled: true,
		},
	}
}

func (s *mcpService) snapshot() mcpStatus {
	s.statusMu.RLock()
	defer s.statusMu.RUnlock()
	return s.status.clone()
}

func (s *mcpService) mutateStatus(fn func(*mcpStatus)) {
	s.publish(func() {
		s.statusMu.Lock()
		fn(&s.status)
		s.statusMu.Unlock()
	})
}

// startMCP begins monitoring local flow-mcp status and sessions once windows
// have loaded.
func (d *desktop) startMCP() {
	main := d.find("main")
	if d.mcp != nil || main == nil || main.window == nil {
		return
	}
	win := main.window
	var svc *mcpService
	svc = newMCPService(d.dataDir, localMCPBackend{}, func(fn func()) {
		win.Update(func() {
			before := svc.snapshot().key()
			fn()
			if svc.snapshot().key() != before {
				d.invalidate()
			}
		})
	})
	d.mcp = svc
	mygo.App.OnQuit(func() { svc.stop() })
	svc.start()
}

func (s *mcpService) start() {
	s.mu.Lock()
	if s.started || s.stopped {
		s.mu.Unlock()
		return
	}
	s.started = true
	s.wg.Add(1)
	s.mu.Unlock()
	go s.run()
}

func (s *mcpService) stop() {
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return
	}
	s.stopped = true
	cancel := s.testCanc
	s.testCanc = nil
	close(s.stopCh)
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
	}
}

func (s *mcpService) signalRefresh() {
	select {
	case s.refresh <- struct{}{}:
	default:
	}
}

func (s *mcpService) run() {
	defer s.wg.Done()
	s.scan(true)
	ticker := time.NewTicker(s.pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-s.stopCh:
			return
		case <-s.refresh:
			s.scan(true)
		case <-ticker.C:
			s.scan(false)
		}
	}
}

func (s *mcpService) scan(includeClients bool) {
	s.mu.Lock()
	beforeGen := s.cfgGen
	s.mu.Unlock()

	cfg, cfgErr := core.ReadMCPConfig(s.dir)
	binPath, binFound := s.backend.Binary()
	sessions, sessErr := s.backend.Sessions(s.dir)
	var clients []string
	if includeClients {
		clients = s.backend.ConfiguredClients()
	}

	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return
	}
	applyCfg := s.cfgGen == beforeGen
	if applyCfg {
		s.cfg = cfg
	} else {
		cfg = s.cfg
	}
	s.mu.Unlock()

	errMsg := ""
	if applyCfg && cfgErr != nil {
		errMsg = cfgErr.Error()
	} else if sessErr != nil {
		errMsg = sessErr.Error()
	}

	s.mutateStatus(func(st *mcpStatus) {
		st.Loaded = true
		if applyCfg {
			st.Enabled = cfg.IsEnabled()
			st.ReadOnly = cfg.ReadOnly
		}
		st.BinaryPath = binPath
		st.BinaryFound = binFound
		st.Sessions = sessions
		if includeClients {
			st.ConfiguredApps = clients
		}
		st.Err = errMsg
	})
}

func (s *mcpService) refreshNow() {
	s.signalRefresh()
}

func (s *mcpService) setEnabled(on bool) {
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return
	}
	v := on
	s.cfg.Enabled = &v
	s.cfgGen++
	cfg := s.cfg
	s.mu.Unlock()

	err := core.WriteMCPConfig(s.dir, cfg)
	errMsg := ""
	if err != nil {
		errMsg = err.Error()
	}
	s.mutateStatus(func(st *mcpStatus) {
		st.Enabled = on
		st.Err = errMsg
		if !on {
			st.TestOK = false
			st.TestErr = ""
		}
	})
	s.signalRefresh()
}

func (s *mcpService) setReadOnly(on bool) {
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return
	}
	s.cfg.ReadOnly = on
	s.cfgGen++
	cfg := s.cfg
	s.mu.Unlock()

	err := core.WriteMCPConfig(s.dir, cfg)
	errMsg := ""
	if err != nil {
		errMsg = err.Error()
	}
	s.mutateStatus(func(st *mcpStatus) {
		st.ReadOnly = on
		st.Err = errMsg
	})
	s.signalRefresh()
}

func (s *mcpService) disconnectSession(pid int) {
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return
	}
	s.wg.Add(1)
	s.mu.Unlock()

	go func() {
		defer s.wg.Done()
		_ = s.backend.Terminate(s.dir, pid)
		time.Sleep(120 * time.Millisecond)
		s.scan(false)
	}()
}

func (s *mcpService) runTest() {
	s.mu.Lock()
	if s.stopped || s.testing {
		s.mu.Unlock()
		return
	}
	enabled := s.cfg.IsEnabled()
	readOnly := s.cfg.ReadOnly
	s.mu.Unlock()

	if !enabled {
		s.mutateStatus(func(st *mcpStatus) {
			st.Testing = false
			st.TestOK = false
			st.TestErr = "mcp_disabled"
			st.TestAt = s.now()
		})
		return
	}

	binPath, binFound := s.backend.Binary()
	if !binFound {
		s.mutateStatus(func(st *mcpStatus) {
			st.BinaryPath = binPath
			st.BinaryFound = false
			st.Testing = false
			st.TestOK = false
			st.TestErr = "mcp_binary_missing"
			st.TestAt = s.now()
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), mcpTestTimeout)
	s.mu.Lock()
	if s.stopped || s.testing {
		s.mu.Unlock()
		cancel()
		return
	}
	s.testing = true
	s.testCanc = cancel
	s.wg.Add(1)
	s.mu.Unlock()

	s.mutateStatus(func(st *mcpStatus) {
		st.Testing = true
		st.TestErr = ""
	})

	go func() {
		defer s.wg.Done()
		defer cancel()
		res, err := s.backend.Probe(ctx, binPath, s.dir, readOnly)

		s.mu.Lock()
		stopped := s.stopped
		s.testing = false
		s.testCanc = nil
		s.mu.Unlock()
		if stopped {
			return
		}

		now := s.now()
		if err != nil {
			msg := err.Error()
			s.mutateStatus(func(st *mcpStatus) {
				st.Testing = false
				st.TestOK = false
				st.TestErr = msg
				st.TestAt = now
			})
			s.scan(false)
			return
		}
		s.mutateStatus(func(st *mcpStatus) {
			st.Testing = false
			st.TestOK = true
			st.TestVersion = res.Version
			st.TestTools = res.Tools
			st.TestErr = ""
			st.TestAt = now
		})
		s.scan(false)
	}()
}

// mcpClientArgs returns the CLI flags for clients connecting to flow-mcp.
// Default workspace locations need no --data-dir because flow-mcp follows
// Flow's location.json automatically; FLOW_DATA_DIR overrides are passed
// explicitly so dev/preview sessions connect to the same workspace.
func mcpClientArgs(dataDir string, readOnly bool) []string {
	args := []string{}
	if custom := os.Getenv("FLOW_DATA_DIR"); custom != "" && dataDir != "" {
		args = append(args, "--data-dir", dataDir)
	}
	if readOnly {
		args = append(args, "--read-only")
	}
	return args
}

func mcpJSONConfig(binaryPath, dataDir string, readOnly bool) string {
	if binaryPath == "" {
		binaryPath = mcpCanonicalAppPath
	}
	type serverEntry struct {
		Command string   `json:"command"`
		Args    []string `json:"args"`
	}
	doc := struct {
		MCPServers map[string]serverEntry `json:"mcpServers"`
	}{
		MCPServers: map[string]serverEntry{
			"flow": {
				Command: binaryPath,
				Args:    mcpClientArgs(dataDir, readOnly),
			},
		},
	}
	raw, _ := json.MarshalIndent(doc, "", "  ")
	return string(raw)
}

type mcpClientPreset struct {
	label string
	json  bool
	build func(binaryPath, dataDir string, readOnly bool) string
}

var mcpClientPresets = []mcpClientPreset{
	{label: "JSON", json: true, build: mcpJSONConfig},
	{label: "Claude", build: mcpClaudeCLICommand},
	{label: "Codex", build: mcpCodexCLICommand},
	{label: "Antigravity", build: mcpAgyCLICommand},
	{label: "OpenCode", build: mcpOpenCodeCLICommand},
}

func mcpCLICommand(prefix []string, binaryPath, dataDir string, readOnly bool) string {
	if binaryPath == "" {
		binaryPath = mcpCanonicalAppPath
	}
	parts := append(slices.Clone(prefix), shellQuote(binaryPath))
	for _, arg := range mcpClientArgs(dataDir, readOnly) {
		parts = append(parts, shellQuote(arg))
	}
	return strings.Join(parts, " ")
}

func mcpClaudeCLICommand(binaryPath, dataDir string, readOnly bool) string {
	return mcpCLICommand([]string{"claude", "mcp", "add", "--transport", "stdio", "flow", "--"}, binaryPath, dataDir, readOnly)
}

func mcpCodexCLICommand(binaryPath, dataDir string, readOnly bool) string {
	return mcpCLICommand([]string{"codex", "mcp", "add", "flow", "--"}, binaryPath, dataDir, readOnly)
}

func mcpAgyCLICommand(binaryPath, dataDir string, readOnly bool) string {
	return mcpCLICommand([]string{"agy", "mcp", "add", "flow", "--"}, binaryPath, dataDir, readOnly)
}

func mcpOpenCodeCLICommand(binaryPath, dataDir string, readOnly bool) string {
	return mcpCLICommand([]string{"opencode", "mcp", "add", "--global", "flow", "--"}, binaryPath, dataDir, readOnly)
}

func shellQuote(s string) string {
	if s == "" {
		return `""`
	}
	if !strings.ContainsAny(s, " \t\n\"'\\$`!#&*()[]{}|;<>?") {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func formatSessionDuration(startedAt int64, now time.Time, zh bool) string {
	if startedAt <= 0 {
		return ""
	}
	d := max(0, now.Sub(time.UnixMilli(startedAt)))
	switch {
	case d < time.Minute:
		if zh {
			return "刚刚连接"
		}
		return "just now"
	case d < time.Hour:
		m := int(d / time.Minute)
		if zh {
			return "已连接 " + strconv.Itoa(m) + " 分钟"
		}
		return strconv.Itoa(m) + "m"
	default:
		h := int(d / time.Hour)
		if zh {
			return "已连接 " + strconv.Itoa(h) + " 小时"
		}
		return strconv.Itoa(h) + "h"
	}
}
