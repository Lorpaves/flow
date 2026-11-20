package main

import (
	"context"
	"flag"
	"flow/internal/core"
	"flow/internal/mcpserver"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	log.SetOutput(os.Stderr)
	log.SetPrefix("flow-mcp: ")
	dir := flag.String("data-dir", os.Getenv("FLOW_DATA_DIR"), "Flow workspace directory (default: the app's, ~/Library/Application Support/Flow unless moved in its settings)")
	readOnly := flag.Bool("read-only", false, "Expose only query tools")
	version := flag.Bool("version", false, "Print version and exit")
	flag.Parse()
	if *version {
		fmt.Println("flow-mcp " + mcpserver.Version)
		return
	}
	if *dir == "" {
		def, err := core.DefaultDataDir()
		if err != nil {
			log.Fatal(err)
		}
		resolved, missing, err := core.ResolveDataDir(def)
		if err != nil {
			log.Fatal(err)
		}
		if missing != "" {
			// Never write a second workspace where the moved one should be.
			log.Fatalf("the Flow workspace was moved to %s, which is not available", missing)
		}
		*dir = resolved
	}
	abs, err := filepath.Abs(*dir)
	if err != nil {
		log.Fatal(err)
	}
	cfg, cfgErr := core.ReadMCPConfig(abs)
	if cfgErr != nil {
		log.Printf("ignoring invalid MCP config: %v", cfgErr)
	}
	if !cfg.IsEnabled() {
		log.Fatal("MCP access is disabled in Flow Settings")
	}
	effectiveReadOnly := *readOnly || cfg.ReadOnly

	store, err := core.Open(filepath.Join(abs, "flow.db"))
	if err != nil {
		log.Fatal(err)
	}
	defer store.Close()

	exe, _ := os.Executable()
	cleanupSession, _ := core.RegisterMCPSession(abs, core.MCPSession{
		PID:       os.Getpid(),
		StartedAt: time.Now().UnixMilli(),
		ReadOnly:  effectiveReadOnly,
		Version:   mcpserver.Version,
		Binary:    exe,
		Client:    detectParentClient(),
	})
	defer cleanupSession()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err = mcpserver.New(store, effectiveReadOnly).Run(ctx, &mcp.StdioTransport{}); err != nil && ctx.Err() == nil {
		log.Print(err)
		os.Exit(1)
	}
}

func detectParentClient() string {
	if runtime.GOOS == "windows" {
		return ""
	}
	ppid := os.Getppid()
	if ppid <= 1 {
		return ""
	}
	out, err := exec.Command("ps", "-o", "comm=", "-p", strconv.Itoa(ppid)).Output()
	if err != nil {
		return ""
	}
	name := strings.TrimSpace(string(out))
	if name == "" {
		return ""
	}
	return filepath.Base(name)
}
