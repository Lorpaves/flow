package mcpserver

import (
	"context"
	"flow/internal/core"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMCPViewsUseSharedQueryAndValidateNestedFilters(t *testing.T) {
	cs, s := session(t, false)
	call(t, cs, "create_issue", map[string]any{"title": "Keep this", "status": "todo", "priority": 2})
	call(t, cs, "create_issue", map[string]any{"title": "Exclude", "status": "done", "priority": 1})
	config := core.DefaultView()
	config.GroupBy = "priority"
	config.Filter = core.FilterGroup{Groups: []core.FilterGroup{{Any: true, Rules: []core.FilterRule{{Field: "status", Op: "is", Values: []string{"todo"}}, {Field: "title", Op: "contains", Values: []string{"missing"}}}}}}
	v := call(t, cs, "save_view", map[string]any{"name": "Focus", "config": config})
	id := v["id"].(string)
	result := call(t, cs, "query_view", map[string]any{"id": id})
	if result["total"].(float64) != 1 {
		t.Fatalf("wrong matches %v", result)
	}
	if s.Snapshot().Views[id].Config.GroupBy != "priority" {
		t.Fatal("view did not persist")
	}
	config.Filter.Rules = []core.FilterRule{{Field: "imaginary", Op: "is"}}
	r, e := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "save_view", Arguments: map[string]any{"id": id, "name": "Invalid", "config": config}})
	if e != nil || !r.IsError {
		t.Fatalf("invalid filter accepted: %v %v", r, e)
	}
	if s.Snapshot().Views[id].Name != "Focus" {
		t.Fatal("invalid save replaced view")
	}
	call(t, cs, "delete_view", map[string]any{"id": id})
	if len(s.Snapshot().Views) != 0 || len(s.Snapshot().Issues) != 2 {
		t.Fatal("view delete affected issues")
	}
}
