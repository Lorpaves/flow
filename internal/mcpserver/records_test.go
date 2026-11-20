package mcpserver

import (
	"context"
	"encoding/base64"
	"flow/internal/core"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMCPRelatedCreationAndDraftSubmission(t *testing.T) {
	cs, s := session(t, false)
	i := call(t, cs, "create_issue", map[string]any{"title": "Source"})
	for _, kind := range []string{"related", "sub_issue", "parent_issue", "blocked_issue", "blocking_issue"} {
		created := call(t, cs, "create_related_issue", map[string]any{"form": map[string]any{"title": kind}, "relation": map[string]any{"issueId": i["identifier"], "kind": kind}})
		id := created["id"].(string)
		d := s.Snapshot()
		if kind == "sub_issue" && d.Issues[id].ParentID != i["id"] || kind == "parent_issue" && d.Issues[i["id"].(string)].ParentID != id {
			t.Fatal("hierarchy wrong", kind)
		}
	}
	draft := call(t, cs, "save_draft", map[string]any{"form": map[string]any{"title": "Recovery"}, "relation": map[string]any{"issueId": i["identifier"], "kind": "blocking_issue"}})
	_, e := s.Update(func(d *core.Data) error {
		v := d.Drafts[draft["id"].(string)]
		v.Context = "related-draft"
		d.Drafts[v.ID] = v
		return nil
	})
	if e != nil {
		t.Fatal(e)
	}
	updated := call(t, cs, "save_draft", map[string]any{"id": draft["id"], "form": map[string]any{"title": "Recovered"}})
	if s.Snapshot().Drafts[draft["id"].(string)].Context != "related-draft" || updated["relation"].(map[string]any)["issueId"] != i["id"] {
		t.Fatal("draft update discarded context", updated)
	}
	created := call(t, cs, "submit_draft", map[string]any{"id": draft["id"]})
	found := false
	for _, r := range s.Snapshot().Relations {
		found = found || r.From == created["id"] && r.To == i["id"] && r.Kind == "blocks"
	}
	if !found || len(s.Snapshot().Drafts) != 0 {
		t.Fatal("submission lost relation or retained draft")
	}
	draft = call(t, cs, "save_draft", map[string]any{"form": map[string]any{"title": "Standalone"}, "relation": map[string]any{"issueId": i["id"], "kind": "related"}})
	call(t, cs, "save_draft", map[string]any{"id": draft["id"], "form": map[string]any{"title": "Standalone"}, "clearRelation": true})
	if s.Snapshot().Drafts[draft["id"].(string)].Relation != nil {
		t.Fatal("explicit detach failed")
	}
	before := len(s.Snapshot().Issues)
	r, e := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "create_related_issue", Arguments: map[string]any{"form": map[string]any{"title": "Invalid"}, "relation": map[string]any{"issueId": i["id"], "kind": "invalid"}}})
	if e != nil || !r.IsError || len(s.Snapshot().Issues) != before {
		t.Fatal("invalid creation left orphan", e)
	}
	read, _ := session(t, true)
	if _, e = read.CallTool(context.Background(), &mcp.CallToolParams{Name: "create_related_issue", Arguments: map[string]any{}}); e == nil {
		t.Fatal("read-only exposes creation")
	}
}

func TestMCPThreadedComments(t *testing.T) {
	cs, s := session(t, false)
	i := call(t, cs, "create_issue", map[string]any{"title": "Threads"})
	j := call(t, cs, "create_issue", map[string]any{"title": "Other"})
	root := call(t, cs, "save_comment", map[string]any{"issueId": i["identifier"], "body": "Parent"})
	reply := call(t, cs, "save_comment", map[string]any{"issueId": i["id"], "parentId": root["id"], "body": "Reply"})
	if reply["parentId"] != root["id"] {
		t.Fatal("MCP omitted thread link")
	}
	r, e := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "save_comment", Arguments: map[string]any{"issueId": j["id"], "parentId": root["id"], "body": "Wrong thread"}})
	if e != nil || !r.IsError {
		t.Fatal("cross-issue reply accepted", e)
	}
	call(t, cs, "delete_comment", map[string]any{"id": root["id"]})
	got := call(t, cs, "save_comment", map[string]any{"id": reply["id"], "issueId": i["id"], "body": "Edited"})
	if got["parentId"] != root["id"] {
		t.Fatal("MCP edit detached reply")
	}
	active := call(t, cs, "list_comments", map[string]any{"ownerKind": "issue", "ownerId": i["identifier"]})
	all := call(t, cs, "list_comments", map[string]any{"ownerKind": "issue", "ownerId": i["identifier"], "includeDeleted": true})
	if active["total"].(float64) != 1 || all["total"].(float64) != 2 || len(s.Snapshot().CommentThread(i["id"].(string))) != 2 {
		t.Fatal("thread ancestry/list filtering wrong")
	}
}

func TestMCPLabelScopeAndFavoriteViewDeletion(t *testing.T) {
	cs, s := session(t, false)
	p := call(t, cs, "create_project", map[string]any{"name": "Scoped project"})
	l := call(t, cs, "create_label", map[string]any{"name": "组织", "group": "设计"})
	i := call(t, cs, "create_issue", map[string]any{"title": "保持关联", "projectId": p["id"], "labelIds": []any{l["id"]}})
	j := call(t, cs, "create_issue", map[string]any{"title": "移除不适用关联", "labelIds": []any{l["id"]}})
	call(t, cs, "update_label", map[string]any{"id": l["id"], "projectId": p["id"], "color": "#112233"})
	d := s.Snapshot()
	if len(d.Issues[i["id"].(string)].LabelIDs) != 1 || len(d.Issues[j["id"].(string)].LabelIDs) != 0 {
		t.Fatal("MCP label scope diverged from desktop semantics")
	}
	v := call(t, cs, "save_view", map[string]any{"name": "收藏视图"})
	f := call(t, cs, "save_favorite_folder", map[string]any{"name": "工作", "items": []any{map[string]any{"kind": "view", "id": v["id"]}, map[string]any{"kind": "issue", "id": i["identifier"]}}})
	call(t, cs, "delete_view", map[string]any{"id": v["id"]})
	if got := s.Snapshot().Folders[f["id"].(string)]; len(got.Items) != 1 || got.Items[0].ID != i["id"] {
		t.Fatalf("view deletion left invalid folder: %+v", got)
	}
	call(t, cs, "delete_favorite_folder", map[string]any{"id": f["id"]})
	call(t, cs, "get_issue", map[string]any{"id": i["id"]})
}

func TestMCPDraftTemplateRelationshipsHistoryAndDocuments(t *testing.T) {
	cs, s := session(t, false)
	p := call(t, cs, "create_project", map[string]any{"name": "Full project", "start": "2026-11-01", "due": "2026-12-01", "priority": 2})
	pid := p["id"].(string)
	draft := call(t, cs, "save_draft", map[string]any{"form": map[string]any{"title": "恢复草稿", "description": "## 正文\n\n**完整**", "projectId": pid}})
	got := call(t, cs, "get_draft", map[string]any{"id": draft["id"]})
	form := got["form"].(map[string]any)
	if !strings.Contains(form["description"].(string), "**完整**") {
		t.Fatalf("lost formatted draft: %v", form)
	}
	i := call(t, cs, "submit_draft", map[string]any{"id": draft["id"]})
	id := i["id"].(string)
	if len(s.Snapshot().Drafts) != 0 {
		t.Fatal("submitted draft remained")
	}
	tpl := call(t, cs, "save_template", map[string]any{"name": "检查模板", "form": map[string]any{"title": "检查任务", "description": "- [ ] 检查", "projectId": pid}})
	other := call(t, cs, "create_from_template", map[string]any{"id": tpl["id"]})
	child := call(t, cs, "create_issue", map[string]any{"title": "Child", "parentId": i["identifier"]})
	grand := call(t, cs, "create_issue", map[string]any{"title": "Grandchild", "parentId": child["identifier"]})
	call(t, cs, "update_issue", map[string]any{"id": grand["id"], "parentId": other["id"]})
	if s.Snapshot().Issues[grand["id"].(string)].ParentID != other["id"] {
		t.Fatal("reparent failed")
	}
	relation := call(t, cs, "add_relation", map[string]any{"from": i["identifier"], "to": other["id"], "kind": "blocks"})
	relations := call(t, cs, "list_relations", map[string]any{"ownerKind": "issue", "ownerId": other["id"]})
	if relations["total"].(float64) != 1 {
		t.Fatal("incoming relationship invisible")
	}
	call(t, cs, "remove_relation", map[string]any{"id": relation["id"]})
	comment := call(t, cs, "save_comment", map[string]any{"issueId": i["identifier"], "body": "**评论**"})
	call(t, cs, "save_comment", map[string]any{"id": comment["id"], "issueId": id, "body": "修改后的评论"})
	comments := call(t, cs, "list_comments", map[string]any{"ownerKind": "issue", "ownerId": i["identifier"]})
	if comments["total"].(float64) != 1 {
		t.Fatal("comment missing")
	}
	m := call(t, cs, "save_milestone", map[string]any{"projectId": pid, "name": "验收", "due": "2026-11-30"})
	call(t, cs, "update_issue", map[string]any{"id": id, "milestoneId": m["id"]})
	doc := call(t, cs, "save_document", map[string]any{"ownerKind": "project", "ownerId": pid, "name": "设计文档", "body": "# 方案"})
	call(t, cs, "save_project_update", map[string]any{"projectId": pid, "body": "完成接口", "health": "on_track"})
	call(t, cs, "update_issue", map[string]any{"id": id, "title": "New title"})
	history := call(t, cs, "list_activities", map[string]any{"ownerKind": "issue", "ownerId": id, "limit": 200})
	var event string
	for _, raw := range history["items"].([]any) {
		v := raw.(map[string]any)
		after, ok := v["after"].(map[string]any)
		if ok && after["title"] == "New title" {
			event = v["id"].(string)
			break
		}
	}
	if event == "" {
		t.Fatal("no visible title history")
	}
	call(t, cs, "undo_activity", map[string]any{"id": event})
	if s.Snapshot().Issues[id].Title != "恢复草稿" {
		t.Fatal("undo not applied")
	}
	call(t, cs, "delete_milestone", map[string]any{"id": m["id"]})
	if s.Snapshot().Issues[id].MilestoneID != "" {
		t.Fatal("dangling milestone")
	}
	call(t, cs, "delete_document", map[string]any{"id": doc["id"]})
	call(t, cs, "delete_comment", map[string]any{"id": comment["id"]})
	copy := call(t, cs, "copy_issue", map[string]any{"id": id, "title": "Independent copy", "includeChildren": true})
	if copy["id"] == id {
		t.Fatal("copy reused identity")
	}
	if e := s.Snapshot().Validate(); e != nil {
		t.Fatal(e)
	}
}

func TestMCPAttachmentsSchedulesAndReadOnlyBoundary(t *testing.T) {
	cs, s := session(t, false)
	i := call(t, cs, "create_issue", map[string]any{"title": "Scheduled"})
	id := i["id"].(string)
	path := filepath.Join(t.TempDir(), "input.txt")
	content := []byte("中文附件\x00\xff")
	if e := os.WriteFile(path, content, 0600); e != nil {
		t.Fatal(e)
	}
	file := call(t, cs, "add_attachment", map[string]any{"ownerKind": "issue", "ownerId": i["identifier"], "path": path})
	list := call(t, cs, "list_attachments", map[string]any{"ownerKind": "issue", "ownerId": id})
	item := list["items"].([]any)[0].(map[string]any)
	if _, ok := item["data"]; ok {
		t.Fatal("list returned file content")
	}
	chunk := call(t, cs, "read_attachment", map[string]any{"id": file["id"], "offset": 3, "limit": 4})
	raw, e := base64.StdEncoding.DecodeString(chunk["base64"].(string))
	if e != nil || string(raw) != string(content[3:7]) {
		t.Fatal("chunk mismatch")
	}
	export := filepath.Join(t.TempDir(), "export.txt")
	call(t, cs, "export_attachment", map[string]any{"id": file["id"], "path": export})
	got, e := os.ReadFile(export)
	if e != nil || string(got) != string(content) {
		t.Fatal("export changed bytes")
	}
	r, e := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "export_attachment", Arguments: map[string]any{"id": file["id"], "path": export}})
	if e != nil || !r.IsError {
		t.Fatal("export overwrote existing file")
	}
	call(t, cs, "set_reminder", map[string]any{"issueId": id, "at": time.Now().Add(-time.Minute).UTC().Format(time.RFC3339)})
	call(t, cs, "save_recurrence", map[string]any{"form": map[string]any{"title": "Daily item"}, "every": 1, "unit": "day", "nextDue": time.Now().Format("2006-01-02")})
	result := call(t, cs, "run_scheduled", map[string]any{})
	if len(result["createdIssueIds"].([]any)) != 1 || len(result["firedReminderIds"].([]any)) != 1 {
		t.Fatal(result)
	}
	result = call(t, cs, "run_scheduled", map[string]any{})
	if len(result["createdIssueIds"].([]any))+len(result["firedReminderIds"].([]any)) != 0 {
		t.Fatal("duplicate scheduled execution")
	}
	call(t, cs, "save_favorite_folder", map[string]any{"name": "Focus", "items": []map[string]any{{"kind": "issue", "id": i["identifier"]}}})
	if len(s.Snapshot().Folders) != 1 {
		t.Fatal("favorite folder missing")
	}
	read, _ := session(t, true)
	for _, name := range []string{"save_draft", "add_attachment", "export_attachment", "run_scheduled", "save_comment", "undo_activity", "save_favorite_folder"} {
		if _, e = read.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: map[string]any{}}); e == nil {
			t.Fatalf("read-only exposes %s", name)
		}
	}
}
