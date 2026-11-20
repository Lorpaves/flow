package mcpserver

import (
	"context"
	"encoding/base64"
	"errors"
	"flow/internal/core"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type RecordListInput struct {
	Page
	OwnerKind string `json:"ownerKind,omitempty" jsonschema:"Optional issue or project filter; list_attachments also accepts draft or template"`
	OwnerID   string `json:"ownerId,omitempty" jsonschema:"Optional owning issue ID/FLW-number, project ID, draft ID or template ID for attachments"`
}
type DraftInput struct {
	Relation      *core.CreationRelation `json:"relation,omitempty" jsonschema:"For a new issue: issueId accepts ID or FLW-number; kind is related, sub_issue, parent_issue, blocked_issue or blocking_issue. Omission preserves an existing draft relation."`
	ClearRelation bool                   `json:"clearRelation,omitempty" jsonschema:"Explicitly remove an existing creation relation"`
	ID            string                 `json:"id,omitempty"`
	IssueID       string                 `json:"issueId,omitempty" jsonschema:"Omit for a new issue; existing issue reference for a recoverable edit"`
	Form          CreateIssueInput       `json:"form" jsonschema:"Complete draft properties; title may be empty"`
}
type CreateRelatedIssueInput struct {
	Relation core.CreationRelation `json:"relation" jsonschema:"New issue role relative to issueId (ID or FLW-number): related, sub_issue, parent_issue, blocked_issue or blocking_issue"`
	Form     CreateIssueInput      `json:"form"`
}
type TemplateInput struct {
	ID   string           `json:"id,omitempty"`
	Name string           `json:"name"`
	Form CreateIssueInput `json:"form"`
}
type CopyIssueInput struct {
	ID              string `json:"id"`
	Title           string `json:"title,omitempty"`
	IncludeChildren bool   `json:"includeChildren,omitempty"`
}
type RelationInput struct {
	From string `json:"from"`
	To   string `json:"to"`
	Kind string `json:"kind" jsonschema:"related, blocks (from blocks to), or duplicate (from duplicates to)"`
}
type CommentInput struct {
	ID       string `json:"id,omitempty"`
	IssueID  string `json:"issueId"`
	Body     string `json:"body" jsonschema:"Markdown"`
	ParentID string `json:"parentId,omitempty" jsonschema:"Existing comment ID to reply to on the same issue; immutable after creation"`
}
type CommentListInput struct {
	RecordListInput
	IncludeDeleted bool `json:"includeDeleted,omitempty" jsonschema:"Include soft-deleted comments to reconstruct thread ancestry"`
}
type DescriptionVersionInput struct {
	ID        string `json:"id" jsonschema:"Activity ID"`
	UseBefore bool   `json:"useBefore,omitempty"`
}
type AttachmentInput struct {
	OwnerKind string `json:"ownerKind" jsonschema:"issue, project, draft or template"`
	OwnerID   string `json:"ownerId"`
	Name      string `json:"name,omitempty"`
	MIME      string `json:"mime,omitempty"`
	Path      string `json:"path,omitempty" jsonschema:"Absolute local file path; maximum 32 MiB. Supply path or URL."`
	URL       string `json:"url,omitempty"`
}
type AttachmentReadInput struct {
	ID     string `json:"id"`
	Offset int    `json:"offset,omitempty"`
	Limit  int    `json:"limit,omitempty" jsonschema:"Byte count, default and maximum 1 MiB"`
}
type AttachmentExportInput struct {
	ID   string `json:"id"`
	Path string `json:"path" jsonschema:"Absolute destination; must not already exist"`
}
type MilestoneInput struct {
	ID          string `json:"id,omitempty"`
	ProjectID   string `json:"projectId"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Due         string `json:"due,omitempty"`
	Completed   bool   `json:"completed,omitempty"`
	Position    int64  `json:"position,omitempty"`
}
type DocumentInput struct {
	ID        string `json:"id,omitempty"`
	OwnerKind string `json:"ownerKind"`
	OwnerID   string `json:"ownerId"`
	Name      string `json:"name"`
	Body      string `json:"body" jsonschema:"Markdown"`
}
type ProjectUpdateInput struct {
	ID        string `json:"id,omitempty"`
	ProjectID string `json:"projectId"`
	Body      string `json:"body"`
	Health    string `json:"health" jsonschema:"on_track, at_risk, off_track"`
}
type ReminderInput struct {
	IssueID   string `json:"issueId,omitempty" jsonschema:"Exactly one of issueId or projectId"`
	ProjectID string `json:"projectId,omitempty" jsonschema:"Exactly one of projectId or issueId"`
	At        string `json:"at" jsonschema:"RFC3339 with offset, local YYYY-MM-DD HH:MM, or relative duration such as 2h"`
}
type SnoozeReminderInput struct {
	ID string `json:"id"`
	At string `json:"at" jsonschema:"RFC3339, local YYYY-MM-DD HH:MM, or relative duration"`
}
type RecurrenceInput struct {
	ID        string           `json:"id,omitempty"`
	Form      CreateIssueInput `json:"form"`
	Every     int              `json:"every"`
	Unit      string           `json:"unit" jsonschema:"day, week, month, year"`
	NextDue   string           `json:"nextDue"`
	EndDate   string           `json:"endDate,omitempty"`
	AnchorDay int              `json:"anchorDay,omitempty"`
	Paused    bool             `json:"paused,omitempty"`
}
type FolderInput struct {
	ID       string             `json:"id,omitempty"`
	Name     string             `json:"name"`
	Items    []core.FavoriteRef `json:"items"`
	Position int64              `json:"position,omitempty"`
}

func (a *API) registerRecords(s *mcp.Server, readOnly bool) {
	register(s, "list_drafts", "List recoverable drafts, including interrupted edits.", true, false, a.listDrafts)
	register(s, "get_draft", "Read a draft and its complete Markdown form.", true, false, a.getDraft)
	register(s, "list_templates", "List reusable issue templates.", true, false, a.listTemplates)
	register(s, "get_template", "Read a reusable issue template.", true, false, a.getTemplate)
	register(s, "list_activities", "Read issue or project history, including GUI and MCP changes.", true, false, a.listActivities)
	register(s, "list_comments", "Read comments with parentId thread links; filter by ownerKind=issue and ownerId. Set includeDeleted to retain deleted ancestors.", true, false, a.listComments)
	register(s, "list_relations", "Read relationships from either endpoint; filter by owning issue.", true, false, a.listRelations)
	register(s, "list_attachments", "List file/link metadata without reading file content.", true, false, a.listAttachments)
	register(s, "read_attachment", "Read file bytes as a bounded base64 chunk.", true, false, a.readAttachment)
	register(s, "list_milestones", "List milestones; filter by ownerKind=project and ownerId.", true, false, a.listMilestones)
	register(s, "list_documents", "List issue/project Markdown documents.", true, false, a.listDocuments)
	register(s, "list_project_updates", "List project updates and health.", true, false, a.listProjectUpdates)
	register(s, "list_reminders", "List scheduled, fired and dismissed reminders.", true, false, a.listReminders)
	register(s, "list_recurrences", "List local recurring issue schedules.", true, false, a.listRecurrences)
	register(s, "list_favorite_folders", "List favorite folders and their items.", true, false, a.listFolders)
	if readOnly {
		return
	}
	register(s, "create_related_issue", "Atomically create an issue and its relationship or hierarchy. A parent_issue inserts a new parent between the target and its old parent. blocked_issue is blocked by the target; blocking_issue blocks the target.", false, false, a.createRelatedIssue)
	register(s, "save_draft", "Create or replace a recoverable draft; does not submit an issue.", false, true, a.saveDraft)
	register(s, "submit_draft", "Create the draft issue or apply the edited fields of an existing issue.", false, true, a.submitDraft)
	register(s, "delete_draft", "Discard a saved draft.", false, true, a.deleteDraft)
	register(s, "save_template", "Create or replace an issue template.", false, true, a.saveTemplate)
	register(s, "delete_template", "Delete a reusable template.", false, true, a.deleteTemplate)
	register(s, "create_from_template", "Create an issue from a saved template.", false, false, a.createFromTemplate)
	register(s, "copy_issue", "Copy an issue, optionally with descendants, files and documents.", false, false, a.copyIssue)
	register(s, "convert_issue_to_project", "Move children/resources to a new project; preserve the original issue and history in trash.", false, true, a.convertIssue)
	register(s, "add_relation", "Add related, blocking or duplicate relationship; reject cycles.", false, true, a.addRelation)
	register(s, "remove_relation", "Remove a relationship. Removing duplicate linkage does not change status.", false, true, a.removeRelation)
	register(s, "save_comment", "Create or edit a Markdown comment; parentId creates a reply. Editing preserves the existing thread.", false, true, a.saveComment)
	register(s, "delete_comment", "Soft-delete a comment; its prior content remains in history.", false, true, a.deleteComment)
	register(s, "undo_activity", "Undo changed issue/project fields if those fields have not changed again.", false, true, a.undoActivity)
	register(s, "restore_description", "Restore a recorded issue, project or document description version.", false, true, a.restoreDescription)
	register(s, "add_attachment", "Import a local file or attach an HTTP(S) link to an issue, project, draft or template. Draft submission transfers attachments; templates keep independent copies.", false, false, a.addAttachment)
	register(s, "export_attachment", "Write attachment bytes to a new absolute local file; never overwrite.", false, false, a.exportAttachment)
	register(s, "delete_attachment", "Remove an attachment and its embedded bytes.", false, true, a.deleteAttachment)
	register(s, "save_milestone", "Create or replace a project milestone.", false, true, a.saveMilestone)
	register(s, "delete_milestone", "Remove a milestone and clear its issue assignments.", false, true, a.deleteMilestone)
	register(s, "save_document", "Create or replace a Markdown document.", false, true, a.saveDocument)
	register(s, "delete_document", "Remove a document; prior content remains in history.", false, true, a.deleteDocument)
	register(s, "save_project_update", "Create or edit a project update and set project health.", false, true, a.saveProjectUpdate)
	register(s, "delete_project_update", "Remove a project update; retain activity history.", false, true, a.deleteProjectUpdate)
	register(s, "set_reminder", "Schedule a local reminder. Delivery requires Flow to be running or reopened.", false, false, a.setReminder)
	register(s, "snooze_reminder", "Reschedule a reminder without changing its issue or project owner.", false, false, a.snoozeReminder)
	register(s, "dismiss_reminder", "Dismiss or cancel a local reminder.", false, true, a.dismissReminder)
	register(s, "save_recurrence", "Create or replace a recurrence. On resume, one latest overdue occurrence is generated; older missed dates are skipped.", false, true, a.saveRecurrence)
	register(s, "delete_recurrence", "Remove a recurring schedule without deleting generated issues.", false, true, a.deleteRecurrence)
	register(s, "run_scheduled", "Process due local recurrence/reminder schedules now; safe with the desktop running.", false, false, a.runScheduled)
	register(s, "save_favorite_folder", "Create or replace a favorite folder.", false, true, a.saveFolder)
	register(s, "delete_favorite_folder", "Remove a folder without deleting its contents.", false, true, a.deleteFolder)
}

func ownerRef(d core.Data, kind, id string) (string, error) {
	if kind == "issue" {
		i, e := resolveIssue(d, id)
		return i.ID, e
	}
	if kind == "project" {
		p, e := resolveProject(d, id)
		return p.ID, e
	}
	return "", errors.New("invalid_owner_kind")
}
func recordOwner(d core.Data, in RecordListInput) (RecordListInput, error) {
	if in.OwnerID == "" {
		if in.OwnerKind != "" && in.OwnerKind != "issue" && in.OwnerKind != "project" {
			return in, errors.New("invalid_owner_kind")
		}
		return in, nil
	}
	id, e := ownerRef(d, in.OwnerKind, in.OwnerID)
	in.OwnerID = id
	return in, e
}
func matchesOwner(in RecordListInput, kind, id string) bool {
	return (in.OwnerKind == "" || in.OwnerKind == kind) && (in.OwnerID == "" || in.OwnerID == id)
}
func containsQuery(text, query string) bool {
	return strings.Contains(strings.ToLower(text), strings.ToLower(strings.TrimSpace(query)))
}
func listRecords[T any](values map[string]T, in RecordListInput, owner func(T) (string, string), summary func(T) string) (any, error) {
	out := []T{}
	for _, id := range slices.Sorted(maps.Keys(values)) {
		v := values[id]
		kind, ownerID := owner(v)
		if matchesOwner(in, kind, ownerID) && containsQuery(summary(v), in.Query) {
			out = append(out, v)
		}
	}
	return pagination(out, in.Page)
}
func (a *API) recordsSnapshot(in RecordListInput) (core.Data, RecordListInput, error) {
	d, e := a.snapshot()
	if e != nil {
		return d, in, e
	}
	in, e = recordOwner(d, in)
	return d, in, e
}
func formInput(d core.Data, in CreateIssueInput) (core.Issue, error) {
	i := core.Issue{Title: in.Title, ProjectID: in.ProjectID, ParentID: in.ParentID, Status: in.Status, Priority: in.Priority, LabelIDs: slices.Clone(in.LabelIDs), Start: in.Start, Due: in.Due, MilestoneID: in.MilestoneID}
	if i.Status == "" {
		i.Status = "backlog"
	}
	if i.ParentID != "" {
		p, e := resolveIssue(d, i.ParentID)
		if e != nil {
			return i, e
		}
		i.ParentID, i.ProjectID = p.ID, p.ProjectID
	}
	err := setDescription(&i, in.Description)
	return i, err
}
func draftView(v core.IssueDraft, detail bool) map[string]any {
	out := map[string]any{"id": v.ID, "issueId": v.IssueID, "updatedAt": v.UpdatedAt, "title": v.Form.Title, "projectId": v.Form.ProjectID}
	if v.Relation != nil {
		out["relation"] = v.Relation
	}
	if detail {
		out["form"] = issueView(v.Form, true)
	}
	return out
}
func templateView(v core.IssueTemplate, detail bool) map[string]any {
	out := map[string]any{"id": v.ID, "name": v.Name, "updatedAt": v.UpdatedAt}
	if detail {
		out["form"] = issueView(v.Form, true)
	}
	return out
}
func (a *API) listDrafts(_ context.Context, in Page) (any, error) {
	d, e := a.snapshot()
	if e != nil {
		return nil, e
	}
	out := []map[string]any{}
	for _, id := range slices.Sorted(maps.Keys(d.Drafts)) {
		v := d.Drafts[id]
		if containsQuery(v.Form.Title+" "+v.Form.Plain, in.Query) {
			out = append(out, draftView(v, false))
		}
	}
	return pagination(out, in)
}
func (a *API) getDraft(_ context.Context, in IDInput) (any, error) {
	d, e := a.snapshot()
	if e != nil {
		return nil, e
	}
	v, ok := d.Drafts[in.ID]
	if !ok {
		return nil, errors.New("not_found")
	}
	return draftView(v, true), nil
}
func (a *API) listTemplates(_ context.Context, in Page) (any, error) {
	d, e := a.snapshot()
	if e != nil {
		return nil, e
	}
	out := []map[string]any{}
	for _, id := range slices.Sorted(maps.Keys(d.Templates)) {
		v := d.Templates[id]
		if containsQuery(v.Name, in.Query) {
			out = append(out, templateView(v, false))
		}
	}
	return pagination(out, in)
}
func (a *API) getTemplate(_ context.Context, in IDInput) (any, error) {
	d, e := a.snapshot()
	if e != nil {
		return nil, e
	}
	v, ok := d.Templates[in.ID]
	if !ok {
		return nil, errors.New("not_found")
	}
	return templateView(v, true), nil
}
func (a *API) saveDraft(_ context.Context, in DraftInput) (any, error) {
	return a.mutate(func(d *core.Data) (any, error) {
		form, e := formInput(*d, in.Form)
		if e != nil {
			return nil, e
		}
		v := core.IssueDraft{ID: in.ID, Form: form}
		if in.IssueID != "" {
			i, e := resolveIssue(*d, in.IssueID)
			if e != nil {
				return nil, e
			}
			v.IssueID, v.Base = i.ID, i
		}
		if in.ID != "" {
			old, ok := d.Drafts[in.ID]
			if !ok {
				return nil, errors.New("not_found")
			}
			if old.IssueID != v.IssueID {
				return nil, errors.New("draft_target_changed")
			}
			v.Base = old.Base
			v.Context, v.Relation = old.Context, old.Relation.Clone()
		}
		if in.Relation != nil && in.ClearRelation {
			return nil, errors.New("invalid_creation_relation")
		}
		if in.ClearRelation {
			v.Relation = nil
		}
		if in.Relation != nil {
			link, e := resolveCreationRelation(*d, *in.Relation)
			if e != nil {
				return nil, e
			}
			v.Relation = &link
		}
		saved, e := d.SaveDraft(v)
		return draftView(saved, true), e
	})
}

func resolveCreationRelation(d core.Data, relation core.CreationRelation) (core.CreationRelation, error) {
	if !relation.Valid() {
		return relation, errors.New("invalid_creation_relation")
	}
	target, err := resolveIssue(d, relation.IssueID)
	if err != nil {
		return relation, err
	}
	relation.IssueID = target.ID
	return relation, nil
}

func (a *API) createRelatedIssue(_ context.Context, in CreateRelatedIssueInput) (any, error) {
	return a.mutate(func(d *core.Data) (any, error) {
		form, err := formInput(*d, in.Form)
		if err != nil {
			return nil, err
		}
		relation, err := resolveCreationRelation(*d, in.Relation)
		if err != nil {
			return nil, err
		}
		i, err := d.CreateWithRelation(form, &relation)
		if err != nil {
			return nil, err
		}
		return issueView(i, true), nil
	})
}
func (a *API) submitDraft(_ context.Context, in IDInput) (any, error) {
	return a.mutate(func(d *core.Data) (any, error) {
		i, e := d.ApplyDraft(in.ID)
		if e != nil {
			return nil, e
		}
		return issueView(i, true), nil
	})
}
func (a *API) deleteDraft(_ context.Context, in IDInput) (any, error) {
	return a.mutate(func(d *core.Data) (any, error) {
		if _, ok := d.Drafts[in.ID]; !ok {
			return nil, errors.New("not_found")
		}
		d.RemoveDraft(in.ID)
		return map[string]any{"deleted": in.ID}, nil
	})
}
func (a *API) saveTemplate(_ context.Context, in TemplateInput) (any, error) {
	return a.mutate(func(d *core.Data) (any, error) {
		if in.ID != "" {
			if _, ok := d.Templates[in.ID]; !ok {
				return nil, errors.New("not_found")
			}
		}
		form, e := formInput(*d, in.Form)
		if e != nil {
			return nil, e
		}
		v, e := d.SaveTemplate(core.IssueTemplate{ID: in.ID, Name: in.Name, Form: form})
		return templateView(v, true), e
	})
}
func (a *API) deleteTemplate(_ context.Context, in IDInput) (any, error) {
	return a.mutate(func(d *core.Data) (any, error) {
		if _, ok := d.Templates[in.ID]; !ok {
			return nil, errors.New("not_found")
		}
		d.RemoveTemplate(in.ID)
		return map[string]any{"deleted": in.ID}, nil
	})
}
func (a *API) createFromTemplate(_ context.Context, in IDInput) (any, error) {
	return a.mutate(func(d *core.Data) (any, error) {
		i, e := d.CreateFromTemplate(in.ID)
		if e != nil {
			return nil, e
		}
		return issueView(i, true), nil
	})
}
func (a *API) copyIssue(_ context.Context, in CopyIssueInput) (any, error) {
	return a.mutate(func(d *core.Data) (any, error) {
		i, e := resolveIssue(*d, in.ID)
		if e != nil {
			return nil, e
		}
		copy, e := d.CopyIssue(i.ID, in.Title, in.IncludeChildren)
		if e != nil {
			return nil, e
		}
		return issueView(copy, true), nil
	})
}
func (a *API) convertIssue(_ context.Context, in IDInput) (any, error) {
	return a.mutate(func(d *core.Data) (any, error) {
		i, e := resolveIssue(*d, in.ID)
		if e != nil {
			return nil, e
		}
		return d.ConvertIssueToProject(i.ID)
	})
}
func (a *API) listActivities(_ context.Context, in RecordListInput) (any, error) {
	d, in, e := a.recordsSnapshot(in)
	if e != nil {
		return nil, e
	}
	if in.OwnerID == "" {
		return nil, errors.New("owner_required")
	}
	out := []core.Activity{}
	for _, v := range d.History(in.OwnerKind, in.OwnerID) {
		if containsQuery(v.Action+" "+v.Kind+" "+strings.Join(v.Fields, " "), in.Query) {
			out = append(out, v)
		}
	}
	return pagination(out, in.Page)
}
func (a *API) undoActivity(_ context.Context, in IDInput) (any, error) {
	return a.mutate(func(d *core.Data) (any, error) {
		if e := d.UndoActivity(in.ID); e != nil {
			return nil, e
		}
		return d.Activities[in.ID], nil
	})
}
func (a *API) restoreDescription(_ context.Context, in DescriptionVersionInput) (any, error) {
	return a.mutate(func(d *core.Data) (any, error) {
		if e := d.RestoreDescription(in.ID, in.UseBefore); e != nil {
			return nil, e
		}
		return map[string]any{"restoredFrom": in.ID}, nil
	})
}
func (a *API) listComments(_ context.Context, input CommentListInput) (any, error) {
	d, in, e := a.recordsSnapshot(input.RecordListInput)
	if e != nil {
		return nil, e
	}
	values := maps.Clone(d.Comments)
	for id, v := range values {
		if v.DeletedAt != 0 && !input.IncludeDeleted {
			delete(values, id)
		}
	}
	return listRecords(values, in, func(v core.Comment) (string, string) { return "issue", v.IssueID }, func(v core.Comment) string { return v.Body })
}
func (a *API) saveComment(_ context.Context, in CommentInput) (any, error) {
	return a.mutate(func(d *core.Data) (any, error) {
		i, e := resolveIssue(*d, in.IssueID)
		if e != nil {
			return nil, e
		}
		return d.SaveComment(core.Comment{ID: in.ID, IssueID: i.ID, Body: in.Body, ParentID: in.ParentID})
	})
}
func (a *API) deleteComment(_ context.Context, in IDInput) (any, error) {
	return a.mutate(func(d *core.Data) (any, error) {
		v, ok := d.Comments[in.ID]
		if !ok {
			return nil, errors.New("not_found")
		}
		v.DeletedAt = time.Now().UnixMilli()
		d.Comments[v.ID] = v
		return v, nil
	})
}
func (a *API) listRelations(_ context.Context, in RecordListInput) (any, error) {
	d, in, e := a.recordsSnapshot(in)
	if e != nil {
		return nil, e
	}
	out := []core.Relation{}
	for _, id := range slices.Sorted(maps.Keys(d.Relations)) {
		v := d.Relations[id]
		if (matchesOwner(in, "issue", v.From) || matchesOwner(in, "issue", v.To)) && containsQuery(v.Kind, in.Query) {
			out = append(out, v)
		}
	}
	return pagination(out, in.Page)
}
func (a *API) addRelation(_ context.Context, in RelationInput) (any, error) {
	return a.mutate(func(d *core.Data) (any, error) {
		from, e := resolveIssue(*d, in.From)
		if e != nil {
			return nil, e
		}
		to, e := resolveIssue(*d, in.To)
		if e != nil {
			return nil, e
		}
		return d.AddRelation(from.ID, to.ID, in.Kind)
	})
}
func (a *API) removeRelation(_ context.Context, in IDInput) (any, error) {
	return a.mutate(func(d *core.Data) (any, error) {
		if _, ok := d.Relations[in.ID]; !ok {
			return nil, errors.New("not_found")
		}
		delete(d.Relations, in.ID)
		return map[string]any{"deleted": in.ID}, nil
	})
}

func attachmentView(v core.Attachment) map[string]any {
	return map[string]any{"id": v.ID, "ownerKind": v.OwnerKind, "ownerId": v.OwnerID, "name": v.Name, "mime": v.MIME, "url": v.URL, "sha256": v.SHA256, "bytes": len(v.Data), "createdAt": v.CreatedAt}
}
func (a *API) listAttachments(_ context.Context, in RecordListInput) (any, error) {
	d, e := a.snapshot()
	if e == nil {
		if in.OwnerKind == "draft" || in.OwnerKind == "template" {
			if in.OwnerID != "" {
				_, e = attachmentOwnerRef(d, in.OwnerKind, in.OwnerID)
			}
		} else {
			in, e = recordOwner(d, in)
		}
	}
	if e != nil {
		return nil, e
	}
	out := []map[string]any{}
	for _, id := range slices.Sorted(maps.Keys(d.Attachments)) {
		v := d.Attachments[id]
		if matchesOwner(in, v.OwnerKind, v.OwnerID) && containsQuery(v.Name, in.Query) {
			out = append(out, attachmentView(v))
		}
	}
	return pagination(out, in.Page)
}
func (a *API) addAttachment(_ context.Context, in AttachmentInput) (any, error) {
	if (in.Path == "") == (in.URL == "") {
		return nil, errors.New("supply_file_or_url")
	}
	v := core.Attachment{OwnerKind: in.OwnerKind, Name: in.Name, MIME: in.MIME, URL: in.URL}
	if in.Path != "" {
		file, e := core.AttachmentFromFile(in.Path)
		if e != nil {
			return nil, e
		}
		v.Data = file.Data
		if v.Name == "" {
			v.Name = file.Name
		}
		if v.MIME == "" {
			v.MIME = file.MIME
		}
	}
	if v.Name == "" {
		v.Name = in.URL
	}
	return a.mutate(func(d *core.Data) (any, error) {
		owner, e := attachmentOwnerRef(*d, in.OwnerKind, in.OwnerID)
		if e != nil {
			return nil, e
		}
		v.OwnerID = owner
		saved, e := d.AddAttachment(v)
		return attachmentView(saved), e
	})
}
func (a *API) readAttachment(_ context.Context, in AttachmentReadInput) (any, error) {
	d, e := a.snapshot()
	if e != nil {
		return nil, e
	}
	v, ok := d.Attachments[in.ID]
	if !ok {
		return nil, errors.New("not_found")
	}
	if in.Limit == 0 {
		in.Limit = 1024 * 1024
	}
	if in.Offset < 0 || in.Limit < 1 || in.Limit > 1024*1024 {
		return nil, errors.New("invalid_byte_range")
	}
	start := min(in.Offset, len(v.Data))
	end := min(start+in.Limit, len(v.Data))
	out := attachmentView(v)
	out["base64"] = base64.StdEncoding.EncodeToString(v.Data[start:end])
	out["offset"] = start
	out["eof"] = end == len(v.Data)
	out["nextOffset"] = end
	return out, nil
}
func (a *API) exportAttachment(_ context.Context, in AttachmentExportInput) (any, error) {
	if !filepath.IsAbs(in.Path) {
		return nil, errors.New("absolute_path_required")
	}
	d, e := a.snapshot()
	if e != nil {
		return nil, e
	}
	v, ok := d.Attachments[in.ID]
	if !ok || v.URL != "" {
		return nil, errors.New("file_attachment_not_found")
	}
	f, e := os.OpenFile(in.Path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return nil, e
	}
	_, e = f.Write(v.Data)
	if e == nil {
		e = f.Sync()
	}
	closeErr := f.Close()
	if e == nil {
		e = closeErr
	}
	if e != nil {
		_ = os.Remove(in.Path)
		return nil, e
	}
	return map[string]any{"path": in.Path, "bytes": len(v.Data), "sha256": v.SHA256}, nil
}
func (a *API) deleteAttachment(_ context.Context, in IDInput) (any, error) {
	return a.mutate(func(d *core.Data) (any, error) {
		if _, ok := d.Attachments[in.ID]; !ok {
			return nil, errors.New("not_found")
		}
		delete(d.Attachments, in.ID)
		return map[string]any{"deleted": in.ID}, nil
	})
}

func (a *API) listMilestones(_ context.Context, in RecordListInput) (any, error) {
	d, in, e := a.recordsSnapshot(in)
	if e != nil {
		return nil, e
	}
	return listRecords(d.Milestones, in, func(v core.Milestone) (string, string) { return "project", v.ProjectID }, func(v core.Milestone) string { return v.Name + " " + v.Description })
}
func (a *API) saveMilestone(_ context.Context, in MilestoneInput) (any, error) {
	return a.mutate(func(d *core.Data) (any, error) {
		return d.SaveMilestone(core.Milestone{ID: in.ID, ProjectID: in.ProjectID, Name: in.Name, Description: in.Description, Due: in.Due, Completed: in.Completed, Position: in.Position})
	})
}
func (a *API) deleteMilestone(_ context.Context, in IDInput) (any, error) {
	return a.mutate(func(d *core.Data) (any, error) {
		if _, ok := d.Milestones[in.ID]; !ok {
			return nil, errors.New("not_found")
		}
		d.RemoveMilestone(in.ID)
		return map[string]any{"deleted": in.ID}, nil
	})
}
func (a *API) listDocuments(_ context.Context, in RecordListInput) (any, error) {
	d, in, e := a.recordsSnapshot(in)
	if e != nil {
		return nil, e
	}
	return listRecords(d.Documents, in, func(v core.Document) (string, string) { return v.OwnerKind, v.OwnerID }, func(v core.Document) string { return v.Name + " " + v.Body })
}
func (a *API) saveDocument(_ context.Context, in DocumentInput) (any, error) {
	return a.mutate(func(d *core.Data) (any, error) {
		id, e := ownerRef(*d, in.OwnerKind, in.OwnerID)
		if e != nil {
			return nil, e
		}
		return d.SaveDocument(core.Document{ID: in.ID, OwnerKind: in.OwnerKind, OwnerID: id, Name: in.Name, Body: in.Body})
	})
}
func (a *API) deleteDocument(_ context.Context, in IDInput) (any, error) {
	return a.mutate(func(d *core.Data) (any, error) {
		if _, ok := d.Documents[in.ID]; !ok {
			return nil, errors.New("not_found")
		}
		delete(d.Documents, in.ID)
		return map[string]any{"deleted": in.ID}, nil
	})
}
func (a *API) listProjectUpdates(_ context.Context, in RecordListInput) (any, error) {
	d, in, e := a.recordsSnapshot(in)
	if e != nil {
		return nil, e
	}
	return listRecords(d.Updates, in, func(v core.ProjectUpdate) (string, string) { return "project", v.ProjectID }, func(v core.ProjectUpdate) string { return v.Body })
}
func (a *API) saveProjectUpdate(_ context.Context, in ProjectUpdateInput) (any, error) {
	return a.mutate(func(d *core.Data) (any, error) {
		return d.SaveProjectUpdate(core.ProjectUpdate{ID: in.ID, ProjectID: in.ProjectID, Body: in.Body, Health: in.Health})
	})
}
func (a *API) deleteProjectUpdate(_ context.Context, in IDInput) (any, error) {
	return a.mutate(func(d *core.Data) (any, error) {
		if _, ok := d.Updates[in.ID]; !ok {
			return nil, errors.New("not_found")
		}
		delete(d.Updates, in.ID)
		return map[string]any{"deleted": in.ID}, nil
	})
}
func (a *API) listReminders(_ context.Context, in RecordListInput) (any, error) {
	d, in, e := a.recordsSnapshot(in)
	if e != nil {
		return nil, e
	}
	return listRecords(d.Reminders, in, func(v core.Reminder) (string, string) { return v.Owner() }, func(v core.Reminder) string {
		if v.ProjectID != "" {
			return d.Projects[v.ProjectID].Name
		}
		return d.Issues[v.IssueID].Title
	})
}
func (a *API) setReminder(_ context.Context, in ReminderInput) (any, error) {
	if (in.IssueID == "") == (in.ProjectID == "") {
		return nil, errors.New("invalid_reminder")
	}
	at, e := core.ParseReminder(in.At, time.Now())
	if e != nil {
		return nil, e
	}
	return a.mutate(func(d *core.Data) (any, error) {
		if in.ProjectID != "" {
			return d.SetProjectReminder(in.ProjectID, at)
		}
		i, e := resolveIssue(*d, in.IssueID)
		if e != nil {
			return nil, e
		}
		return d.SetReminder(i.ID, at)
	})
}
func (a *API) snoozeReminder(_ context.Context, in SnoozeReminderInput) (any, error) {
	at, e := core.ParseReminder(in.At, time.Now())
	if e != nil {
		return nil, e
	}
	return a.mutate(func(d *core.Data) (any, error) { return d.SnoozeReminder(in.ID, at) })
}
func (a *API) dismissReminder(_ context.Context, in IDInput) (any, error) {
	return a.mutate(func(d *core.Data) (any, error) {
		v, ok := d.Reminders[in.ID]
		if !ok {
			return nil, errors.New("not_found")
		}
		v.DismissedAt = time.Now().UnixMilli()
		d.Reminders[v.ID] = v
		return v, nil
	})
}
func (a *API) listRecurrences(_ context.Context, in Page) (any, error) {
	d, e := a.snapshot()
	if e != nil {
		return nil, e
	}
	out := []map[string]any{}
	for _, id := range slices.Sorted(maps.Keys(d.Recurrences)) {
		v := d.Recurrences[id]
		if containsQuery(v.Form.Title, in.Query) {
			out = append(out, map[string]any{"id": v.ID, "form": issueView(v.Form, true), "every": v.Every, "unit": v.Unit, "nextDue": v.NextDue, "endDate": v.EndDate, "anchorDay": v.AnchorDay, "paused": v.Paused, "lastIssueId": v.LastIssueID, "lastError": v.LastError})
		}
	}
	return pagination(out, in)
}
func (a *API) saveRecurrence(_ context.Context, in RecurrenceInput) (any, error) {
	return a.mutate(func(d *core.Data) (any, error) {
		if in.ID != "" {
			if _, ok := d.Recurrences[in.ID]; !ok {
				return nil, errors.New("not_found")
			}
		}
		form, e := formInput(*d, in.Form)
		if e != nil {
			return nil, e
		}
		v := core.Recurrence{ID: in.ID, Form: form, Every: in.Every, Unit: in.Unit, NextDue: in.NextDue, EndDate: in.EndDate, AnchorDay: in.AnchorDay, Paused: in.Paused}
		if old, ok := d.Recurrences[in.ID]; ok {
			v.LastIssueID = old.LastIssueID
		}
		return d.SaveRecurrence(v)
	})
}
func (a *API) deleteRecurrence(_ context.Context, in IDInput) (any, error) {
	return a.mutate(func(d *core.Data) (any, error) {
		if _, ok := d.Recurrences[in.ID]; !ok {
			return nil, errors.New("not_found")
		}
		delete(d.Recurrences, in.ID)
		return map[string]any{"deleted": in.ID}, nil
	})
}
func (a *API) runScheduled(_ context.Context, _ struct{}) (any, error) {
	return a.mutate(func(d *core.Data) (any, error) {
		created, fired := d.RunScheduled(time.Now())
		return map[string]any{"createdIssueIds": created, "firedReminderIds": fired}, nil
	})
}
func (a *API) listFolders(_ context.Context, in Page) (any, error) {
	d, e := a.snapshot()
	if e != nil {
		return nil, e
	}
	return listRecords(d.Folders, RecordListInput{Page: in}, func(core.FavoriteFolder) (string, string) { return "", "" }, func(v core.FavoriteFolder) string { return v.Name })
}
func (a *API) saveFolder(_ context.Context, in FolderInput) (any, error) {
	return a.mutate(func(d *core.Data) (any, error) {
		for n, ref := range in.Items {
			if ref.Kind == "issue" {
				i, e := resolveIssue(*d, ref.ID)
				if e != nil {
					return nil, e
				}
				in.Items[n].ID = i.ID
			}
		}
		v := core.FavoriteFolder{ID: in.ID, Name: strings.TrimSpace(in.Name), Items: slices.Clone(in.Items), Position: in.Position}
		return d.SaveFolder(v)
	})
}
func (a *API) deleteFolder(_ context.Context, in IDInput) (any, error) {
	return a.mutate(func(d *core.Data) (any, error) {
		if _, ok := d.Folders[in.ID]; !ok {
			return nil, errors.New("not_found")
		}
		delete(d.Folders, in.ID)
		return map[string]any{"deleted": in.ID}, nil
	})
}

func attachmentOwnerRef(d core.Data, kind, id string) (string, error) {
	if kind == "template" {
		if _, ok := d.Templates[id]; !ok {
			return "", errors.New("not_found")
		}
		return id, nil
	}
	if kind == "draft" {
		if _, ok := d.Drafts[id]; !ok {
			return "", errors.New("not_found")
		}
		return id, nil
	}
	return ownerRef(d, kind, id)
}
