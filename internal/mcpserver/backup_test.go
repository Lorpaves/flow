package mcpserver

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestBackupAndRestoreReplaceTheWorkspace(t *testing.T) {
	cs, s := session(t, false)
	call(t, cs, "create_issue", map[string]any{"title": "kept"})
	saved := call(t, cs, "create_backup", map[string]any{})
	path := saved["path"].(string)
	if filepath.Dir(path) != filepath.Join(s.Dir(), "backups") || saved["bytes"].(float64) <= 0 {
		t.Fatal("default backup location", saved)
	}
	// An explicit path is never overwritten.
	other := filepath.Join(t.TempDir(), "mine.flow.json")
	if e := os.WriteFile(other, []byte("keep"), 0600); e != nil {
		t.Fatal(e)
	}
	r, e := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "create_backup", Arguments: map[string]any{"path": other}})
	if e != nil || !r.IsError {
		t.Fatal("backup replaced an existing file", r, e)
	}
	if b, _ := os.ReadFile(other); string(b) != "keep" {
		t.Fatal("existing file changed")
	}
	call(t, cs, "create_issue", map[string]any{"title": "after the backup"})
	restored := call(t, cs, "restore_backup", map[string]any{"path": path})
	if restored["issues"].(float64) != float64(len(s.Snapshot().Issues)) {
		t.Fatal("restore result", restored)
	}
	for _, i := range s.Snapshot().Issues {
		if i.Title == "after the backup" {
			t.Fatal("restore kept a later issue")
		}
	}
	// The workspace before the restore was saved first.
	entries, _ := os.ReadDir(filepath.Join(s.Dir(), "backups"))
	found := false
	for _, v := range entries {
		found = found || strings.HasPrefix(v.Name(), "before-restore-")
	}
	if !found {
		t.Fatal("no backup of the replaced workspace")
	}
}

func TestFormsAndFiltersCarryStartAndMilestone(t *testing.T) {
	cs, _ := session(t, false)
	p := call(t, cs, "create_project", map[string]any{"name": "Range"})
	m := call(t, cs, "save_milestone", map[string]any{"projectId": p["id"], "name": "Beta"})
	tpl := call(t, cs, "save_template", map[string]any{"name": "span", "form": map[string]any{"title": "spanning", "projectId": p["id"], "milestoneId": m["id"], "start": "2026-11-15", "due": "2026-11-18"}})
	i := call(t, cs, "create_from_template", map[string]any{"id": tpl["id"]})
	if i["start"] != "2026-11-15" || i["due"] != "2026-11-18" {
		t.Fatal("template dropped the start", i)
	}
	call(t, cs, "create_issue", map[string]any{"title": "elsewhere", "projectId": p["id"]})
	list := call(t, cs, "list_issues", map[string]any{"milestoneId": m["id"]})
	if list["total"].(float64) != 1 {
		t.Fatal("milestone filter", list)
	}
	r, e := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "list_issues", Arguments: map[string]any{"milestoneId": "missing"}})
	if e != nil || !r.IsError {
		t.Fatal("unknown milestone accepted", r, e)
	}
}
