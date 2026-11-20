package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"flow/internal/core"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui/richtext"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func session(t *testing.T, readOnly bool) (*mcp.ClientSession, *core.Store) {
	t.Helper()
	s, e := core.Open(filepath.Join(t.TempDir(), "flow.db"))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { s.Close() })
	t1, t2 := mcp.NewInMemoryTransports()
	server := New(s, readOnly)
	ss, e := server.Connect(context.Background(), t1, nil)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { ss.Close() })
	c := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	cs, e := c.Connect(context.Background(), t2, nil)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { cs.Close() })
	return cs, s
}
func call(t *testing.T, cs *mcp.ClientSession, name string, in any) map[string]any {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	r, e := cs.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: in})
	if e != nil {
		t.Fatal(e)
	}
	if r.IsError {
		detail, _ := json.Marshal(r.Content)
		t.Fatalf("%s: %s", name, detail)
	}
	b, e := json.Marshal(r.StructuredContent)
	if e != nil {
		t.Fatal(e)
	}
	var out map[string]any
	if e = json.Unmarshal(b, &out); e != nil {
		t.Fatal(e)
	}
	return out
}
func TestMCPWorkflowAndValidation(t *testing.T) {
	cs, s := session(t, false)
	ctx := context.Background()
	tools, e := cs.ListTools(ctx, nil)
	if e != nil {
		t.Fatal(e)
	}
	if len(tools.Tools) != 73 {
		t.Fatalf("got %d tools", len(tools.Tools))
	}
	for _, tool := range tools.Tools {
		if tool.Annotations == nil || tool.Annotations.OpenWorldHint == nil || *tool.Annotations.OpenWorldHint {
			t.Fatal("missing local annotations")
		}
	}
	p := call(t, cs, "create_project", map[string]any{"name": "MCP 项目"})
	pid := p["id"].(string)
	l := call(t, cs, "create_label", map[string]any{"name": "Agent"})
	lid := l["id"].(string)
	i := call(t, cs, "create_issue", map[string]any{"title": "中文 MCP", "description": "## Context\n\n**important**", "projectId": pid, "labelIds": []string{lid}})
	code := i["identifier"].(string)
	child := call(t, cs, "create_issue", map[string]any{"title": "子任务", "parentId": code})
	call(t, cs, "update_issue", map[string]any{"id": code, "status": "done", "priority": 1, "labelIds": []string{}})
	got := call(t, cs, "get_issue", map[string]any{"id": code})
	if !strings.Contains(got["description"].(string), "**important**") || got["priority"].(float64) != 1 {
		t.Fatalf("bad detail: %v", got)
	}
	c := call(t, cs, "get_issue", map[string]any{"id": child["id"]})
	if c["status"] != "backlog" {
		t.Fatal("parent completion affected child")
	}
	page := call(t, cs, "list_issues", map[string]any{"projectId": pid, "limit": 1})
	if page["total"].(float64) != 2 || page["nextOffset"].(float64) != 1 {
		t.Fatalf("bad pagination: %v", page)
	}
	before := s.Snapshot().Preferences.NextNumber
	r, e := cs.CallTool(ctx, &mcp.CallToolParams{Name: "create_issue", Arguments: map[string]any{"title": "invalid", "status": "imaginary"}})
	if e != nil || !r.IsError {
		t.Fatalf("invalid status accepted: %v %v", r, e)
	}
	if s.Snapshot().Preferences.NextNumber != before {
		t.Fatal("invalid call consumed number")
	}
	call(t, cs, "trash_issue", map[string]any{"id": code})
	call(t, cs, "restore_issue", map[string]any{"id": code})
	call(t, cs, "update_project", map[string]any{"id": pid, "archived": true})
	archived := call(t, cs, "list_projects", map[string]any{"archived": true})
	if archived["total"].(float64) != 1 {
		t.Fatal("archive missing")
	}
	call(t, cs, "trash_project", map[string]any{"id": pid})
	call(t, cs, "restore_project", map[string]any{"id": pid})
	call(t, cs, "delete_label", map[string]any{"id": lid})
	if !s.Snapshot().Preferences.Initialized {
		t.Fatal("MCP workspace remains uninitialized")
	}
	res, e := cs.ReadResource(ctx, &mcp.ReadResourceParams{URI: "flow://guide"})
	if e != nil || len(res.Contents) != 1 {
		t.Fatalf("resource: %v", e)
	}
}
func TestReadOnlyOmitsWriteTools(t *testing.T) {
	cs, _ := session(t, true)
	list, e := cs.ListTools(context.Background(), nil)
	if e != nil {
		t.Fatal(e)
	}
	if len(list.Tools) != 24 {
		t.Fatal(len(list.Tools))
	}
	for _, v := range list.Tools {
		if !v.Annotations.ReadOnlyHint {
			t.Fatal(v.Name)
		}
	}
	if _, e = cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "create_issue", Arguments: map[string]any{"title": "forbidden"}}); e == nil {
		t.Fatal("write tool exposed")
	}
}
func TestMCPStdioHelper(t *testing.T) {
	path := os.Getenv("FLOW_TEST_MCP_DB")
	if path == "" {
		t.Skip("subprocess helper")
	}
	s, e := core.Open(path)
	if e != nil {
		os.Exit(2)
	}
	e = New(s, false).Run(context.Background(), &mcp.StdioTransport{})
	s.Close()
	if e != nil {
		os.Exit(3)
	}
	os.Exit(0)
}
func TestActualStdioTransport(t *testing.T) {
	dir := t.TempDir()
	binary := os.Getenv("FLOW_MCP_BINARY")
	var cmd *exec.Cmd
	if binary != "" {
		cmd = exec.Command(binary, "--data-dir", dir)
	} else {
		cmd = exec.Command(os.Args[0], "-test.run=^TestMCPStdioHelper$")
		cmd.Env = append(os.Environ(), "FLOW_TEST_MCP_DB="+filepath.Join(dir, "flow.db"))
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	client := mcp.NewClient(&mcp.Implementation{Name: "stdio-test", Version: "1"}, nil)
	cs, e := client.Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
	if e != nil {
		t.Fatalf("stdio initialization: %v; %s", e, stderr.String())
	}
	const structuredDescription = "No HTTP server.<br>第二行\n\n---\n\n- 一项<br>延续\n- 二项\n\n<details><summary>可折叠😀</summary><p>隐藏正文</p><details open><summary>内层</summary><p>保留正文</p></details></details>\n\n| 名称 | 状态 |\n| --- | --- |\n| **表格😀**<br>第二行 | 已保存 |"
	i := call(t, cs, "create_issue", map[string]any{"title": "真正的 stdio", "description": structuredDescription})
	got := call(t, cs, "get_issue", map[string]any{"id": i["identifier"]})
	if got["title"] != "真正的 stdio" {
		t.Fatal(got)
	}
	if got["description"] != structuredDescription {
		t.Fatal("stdio Markdown lost line breaks or divider", got)
	}
	call(t, cs, "update_issue", map[string]any{"id": i["identifier"], "description": got["description"], "priority": 2})
	label := call(t, cs, "create_label", map[string]any{"name": "Stdio project label", "kind": "project"})
	project := call(t, cs, "create_project", map[string]any{"name": "Stdio roadmap", "start": "2026-03-25", "due": "2026-03-28", "labelIds": []string{label["id"].(string)}})
	nextProject := call(t, cs, "create_project", map[string]any{"name": "Stdio dependent", "start": "2026-03-29", "due": "2026-04-01", "dependencies": []string{project["id"].(string)}})
	milestone := call(t, cs, "save_milestone", map[string]any{"name": "Stdio checkpoint", "projectId": project["id"], "due": "2026-03-27"})
	reminder := call(t, cs, "set_reminder", map[string]any{"projectId": project["id"], "at": "2h"})
	call(t, cs, "snooze_reminder", map[string]any{"id": reminder["id"], "at": "3h"})
	call(t, cs, "shift_schedule", map[string]any{"targets": []map[string]any{{"kind": "project", "id": project["id"]}}, "days": 4, "mode": "auto"})
	projectResult := call(t, cs, "get_project", map[string]any{"id": project["id"]})
	if projectResult["project"].(map[string]any)["start"] != "2026-03-29" || projectResult["url"] != "flow://project/"+project["id"].(string) {
		t.Fatal("stdio project schedule or URL", projectResult)
	}
	v := call(t, cs, "save_view", map[string]any{"name": "Stdio view", "query": "真正的"})
	viewResult := call(t, cs, "query_view", map[string]any{"id": v["id"]})
	if viewResult["total"].(float64) != 1 {
		t.Fatal("stdio view query did not return issue")
	}
	draft := call(t, cs, "save_draft", map[string]any{"form": map[string]any{"title": "stdio 草稿", "description": "恢复内容"}, "relation": map[string]any{"issueId": i["identifier"], "kind": "blocking_issue"}})
	related := call(t, cs, "create_related_issue", map[string]any{"form": map[string]any{"title": "stdio 被阻塞任务"}, "relation": map[string]any{"issueId": i["identifier"], "kind": "blocked_issue"}})
	call(t, cs, "save_template", map[string]any{"name": "stdio 模板", "form": map[string]any{"title": "重复工作"}})
	comment := call(t, cs, "save_comment", map[string]any{"issueId": i["identifier"], "body": "来自 stdio 的评论"})
	reply := call(t, cs, "save_comment", map[string]any{"issueId": i["identifier"], "parentId": comment["id"], "body": "来自 stdio 的回复"})
	if reply["parentId"] != comment["id"] {
		t.Fatal("stdio reply missing parent")
	}
	if comment["id"] == nil || draft["id"] == nil {
		t.Fatal("new stdio records missing IDs")
	}
	call(t, cs, "add_attachment", map[string]any{"ownerKind": "issue", "ownerId": i["identifier"], "name": "参考", "url": "https://example.com/reference"})
	mediaDraft := call(t, cs, "save_draft", map[string]any{"form": map[string]any{"title": "stdio media draft"}})
	mediaPath := filepath.Join(dir, "stdio-media.txt")
	if err := os.WriteFile(mediaPath, []byte("draft file bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	mediaFile := call(t, cs, "add_attachment", map[string]any{"ownerKind": "draft", "ownerId": mediaDraft["id"], "path": mediaPath})
	mediaList := call(t, cs, "list_attachments", map[string]any{"ownerKind": "draft", "ownerId": mediaDraft["id"]})
	if mediaList["total"].(float64) != 1 {
		t.Fatal("stdio draft media missing", mediaList)
	}
	mediaIssue := call(t, cs, "submit_draft", map[string]any{"id": mediaDraft["id"]})
	adopted := call(t, cs, "list_attachments", map[string]any{"ownerKind": "issue", "ownerId": mediaIssue["id"]})
	if adopted["total"].(float64) != 1 {
		t.Fatal("stdio draft media not adopted", adopted)
	}
	mediaTemplate := call(t, cs, "save_template", map[string]any{"name": "stdio media template", "form": map[string]any{"title": "Media from template", "description": "[file](flow-attachment://" + mediaFile["id"].(string) + ")"}})
	templateFiles := call(t, cs, "list_attachments", map[string]any{"ownerKind": "template", "ownerId": mediaTemplate["id"]})
	if templateFiles["total"].(float64) != 1 {
		t.Fatal("stdio template media missing", templateFiles)
	}
	fromTemplate := call(t, cs, "create_from_template", map[string]any{"id": mediaTemplate["id"]})
	call(t, cs, "delete_template", map[string]any{"id": mediaTemplate["id"]})
	independent := call(t, cs, "list_attachments", map[string]any{"ownerKind": "issue", "ownerId": fromTemplate["id"]})
	if independent["total"].(float64) != 1 {
		t.Fatal("template deletion lost issue file", independent)
	}
	mediaBody := "**Media** [file](flow-attachment://" + mediaFile["id"].(string) + ")"
	mediaComment := call(t, cs, "save_comment", map[string]any{"issueId": mediaIssue["id"], "body": mediaBody})
	mediaDocument := call(t, cs, "save_document", map[string]any{"ownerKind": "issue", "ownerId": mediaIssue["id"], "name": "Media document", "body": mediaBody})
	mediaCopy := call(t, cs, "copy_issue", map[string]any{"id": mediaIssue["id"], "title": "Independent document media"})
	projectFile := call(t, cs, "add_attachment", map[string]any{"ownerKind": "project", "ownerId": project["id"], "path": mediaPath})
	updateBody := "**Progress** [file](flow-attachment://" + projectFile["id"].(string) + ")"
	mediaUpdate := call(t, cs, "save_project_update", map[string]any{"projectId": project["id"], "health": "on_track", "body": updateBody})
	if e = cs.Close(); e != nil {
		t.Fatal(e)
	}
	s, e := core.Open(filepath.Join(dir, "flow.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	data := s.Snapshot()
	var structuredBody richtext.Text
	if err := json.Unmarshal(data.Issues[i["id"].(string)].Body, &structuredBody); err != nil {
		t.Fatal(err)
	}
	if structuredBody.ParagraphCount() != 12 || !structuredBody.Equal(richtext.ParseMarkdown(structuredDescription)) {
		t.Fatal("stdio update and database reopen changed block structure", structuredBody.Markdown())
	}
	if data.Projects[nextProject["id"].(string)].Start != "2026-04-01" || data.Milestones[milestone["id"].(string)].Due != "2026-03-31" || data.Reminders[reminder["id"].(string)].ProjectID != project["id"] || data.Labels[label["id"].(string)].EntityKind() != "project" {
		t.Fatal("stdio project timeline, label or reminder not persisted")
	}
	if len(data.Issues) != 5 || len(data.Drafts) != 1 || len(data.Templates) != 1 || len(data.Comments) != 3 || len(data.Attachments) != 5 || len(data.Activities) < 3 || data.Comments[reply["id"].(string)].ParentID != comment["id"] {
		t.Fatal("stdio records/history not persisted")
	}
	if data.Comments[mediaComment["id"].(string)].Body != mediaBody || data.Documents[mediaDocument["id"].(string)].Body != mediaBody || data.Updates[mediaUpdate["id"].(string)].Body != updateBody {
		t.Fatal("stdio rich resource body lost media")
	}
	var copiedBody, copiedFile string
	for _, v := range data.Documents {
		if v.OwnerID == mediaCopy["id"] {
			copiedBody = v.Body
		}
	}
	for _, v := range data.Attachments {
		if v.OwnerID == mediaCopy["id"] {
			copiedFile = v.ID
		}
	}
	if copiedFile == "" || !strings.Contains(copiedBody, copiedFile) || strings.Contains(copiedBody, mediaFile["id"].(string)) {
		t.Fatal("stdio copied document retains source media")
	}
	if file := data.Attachments[mediaFile["id"].(string)]; file.OwnerKind != "issue" || file.OwnerID != mediaIssue["id"] || string(file.Data) != "draft file bytes" || file.MIME == "" {
		t.Fatal("stdio media ownership/bytes not persisted")
	}
	for _, file := range data.Attachments {
		if file.OwnerID == fromTemplate["id"] && (string(file.Data) != "draft file bytes" || !strings.Contains(string(data.Issues[file.OwnerID].Body), file.ID) || file.ID == mediaFile["id"]) {
			t.Fatal("stdio template clone not independent")
		}
	}
	if r := data.Drafts[draft["id"].(string)].Relation; r == nil || r.IssueID != i["id"] || r.Kind != "blocking_issue" {
		t.Fatal("stdio related draft lost context")
	}
	if len(data.Relations) != 1 {
		t.Fatal("stdio related creation missing relation")
	}
	for _, r := range data.Relations {
		if r.From != i["id"] || r.To != related["id"] || r.Kind != "blocks" {
			t.Fatal("stdio blocking direction changed", r)
		}
	}

}
