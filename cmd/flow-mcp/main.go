package main

import (
	"context"
	"flag"
	"flow/internal/core"
	"flow/internal/mcpserver"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

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
	store, err := core.Open(filepath.Join(abs, "flow.db"))
	if err != nil {
		log.Fatal(err)
	}
	defer store.Close()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err = mcpserver.New(store, *readOnly).Run(ctx, &mcp.StdioTransport{}); err != nil && ctx.Err() == nil {
		log.Print(err)
		os.Exit(1)
	}
}
