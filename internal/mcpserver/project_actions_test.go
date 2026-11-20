package mcpserver

import (
	"context"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"testing"
)

func TestMCPProjectLabelsReminderOwnersAndSnooze(t *testing.T) {
	cs, s := session(t, false)
	l := call(t, cs, "create_label", map[string]any{"kind": "project", "name": "Launch"})
	p := call(t, cs, "create_project", map[string]any{"name": "Labeled project", "labelIds": []string{l["id"].(string)}})
	i := call(t, cs, "create_issue", map[string]any{"title": "Task", "projectId": p["id"]})
	labels := call(t, cs, "list_labels", map[string]any{"kind": "project"})
	if len(labels["items"].([]any)) != 1 {
		t.Fatal("project label absent")
	}
	labels = call(t, cs, "list_labels", map[string]any{})
	if len(labels["items"].([]any)) != 0 {
		t.Fatal("project label in default issue labels")
	}
	r := call(t, cs, "set_reminder", map[string]any{"projectId": p["id"], "at": "2026-01-01T09:00:00Z"})
	call(t, cs, "run_scheduled", map[string]any{})
	r = call(t, cs, "snooze_reminder", map[string]any{"id": r["id"], "at": "2h"})
	if r["projectId"] != p["id"] || s.Snapshot().Reminders[r["id"].(string)].FiredAt != 0 {
		t.Fatal("snooze lost owner")
	}
	listed := call(t, cs, "list_reminders", map[string]any{"ownerKind": "project", "ownerId": p["id"]})
	if listed["total"].(float64) != 1 {
		t.Fatal("owner filter missed reminder")
	}
	for _, args := range []map[string]any{{"at": "1h"}, {"issueId": i["id"], "projectId": p["id"], "at": "1h"}} {
		result, e := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "set_reminder", Arguments: args})
		if e != nil || !result.IsError {
			t.Fatal("ambiguous reminder owner accepted", e)
		}
	}
	result, e := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "update_issue", Arguments: map[string]any{"id": i["id"], "labelIds": []string{l["id"].(string)}}})
	if e != nil || !result.IsError {
		t.Fatal("cross-kind label accepted", e)
	}
	got := call(t, cs, "get_project", map[string]any{"id": p["id"]})
	if got["url"] != "flow://project/"+p["id"].(string) {
		t.Fatal("project URL missing")
	}
	call(t, cs, "update_project", map[string]any{"id": p["id"], "labelIds": []string{}})
	if len(s.Snapshot().Projects[p["id"].(string)].LabelIDs) != 0 {
		t.Fatal("explicit label clear failed")
	}
}
