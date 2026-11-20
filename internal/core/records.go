package core

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"slices"
	"strings"
	"unicode/utf8"
)

// Drafts retain an unsaved creation or an edit that cannot be applied yet.
// Form references may be stale: recovery must not discard a user's text.
type IssueDraft struct {
	Relation  *CreationRelation `json:"relation,omitempty"`
	ID        string            `json:"id"`
	IssueID   string            `json:"issueId,omitempty"`
	Context   string            `json:"context,omitempty"`
	Form      Issue             `json:"form"`
	Base      Issue             `json:"base"`
	UpdatedAt int64             `json:"updatedAt"`
}
type IssueTemplate struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Form      Issue  `json:"form"`
	UpdatedAt int64  `json:"updatedAt"`
}
type Activity struct {
	ID        string          `json:"id"`
	OwnerKind string          `json:"ownerKind"`
	OwnerID   string          `json:"ownerId"`
	Kind      string          `json:"kind"`
	TargetID  string          `json:"targetId"`
	Action    string          `json:"action"`
	Source    string          `json:"source"`
	At        int64           `json:"at"`
	Fields    []string        `json:"fields,omitempty"`
	Before    json.RawMessage `json:"before,omitempty"`
	After     json.RawMessage `json:"after,omitempty"`
	UndoneAt  int64           `json:"undoneAt,omitempty"`
}
type Comment struct {
	ID        string `json:"id"`
	IssueID   string `json:"issueId"`
	ParentID  string `json:"parentId,omitempty"`
	Body      string `json:"body"`
	CreatedAt int64  `json:"createdAt"`
	UpdatedAt int64  `json:"updatedAt"`
	DeletedAt int64  `json:"deletedAt,omitempty"`
}
type Relation struct {
	ID        string `json:"id"`
	From      string `json:"from"`
	To        string `json:"to"`
	Kind      string `json:"kind"` // related (symmetric), blocks, duplicate (from duplicates to)
	CreatedAt int64  `json:"createdAt"`
}

const MaxAttachmentBytes = 32 * 1024 * 1024
const MaxBackupBytes = 512 * 1024 * 1024

// File bytes live in the same transaction and backup as their owning records.
// There are no machine-specific source paths in the portable data model.
type Attachment struct {
	ID        string `json:"id"`
	OwnerKind string `json:"ownerKind"`
	OwnerID   string `json:"ownerId"`
	Name      string `json:"name"`
	MIME      string `json:"mime,omitempty"`
	Data      []byte `json:"data,omitempty"`
	URL       string `json:"url,omitempty"`
	SHA256    string `json:"sha256,omitempty"`
	CreatedAt int64  `json:"createdAt"`
}
type Milestone struct {
	ID          string `json:"id"`
	ProjectID   string `json:"projectId"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Due         string `json:"due,omitempty"`
	Completed   bool   `json:"completed"`
	Position    int64  `json:"position"`
}
type Document struct {
	ID        string `json:"id"`
	OwnerKind string `json:"ownerKind"`
	OwnerID   string `json:"ownerId"`
	Name      string `json:"name"`
	Body      string `json:"body"`
	CreatedAt int64  `json:"createdAt"`
	UpdatedAt int64  `json:"updatedAt"`
}
type ProjectUpdate struct {
	ID        string `json:"id"`
	ProjectID string `json:"projectId"`
	Body      string `json:"body"`
	Health    string `json:"health"`
	CreatedAt int64  `json:"createdAt"`
	UpdatedAt int64  `json:"updatedAt"`
}
type FavoriteRef struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}
type FavoriteFolder struct {
	ID       string        `json:"id"`
	Name     string        `json:"name"`
	Items    []FavoriteRef `json:"items"`
	Position int64         `json:"position"`
}
type Reminder struct {
	ProjectID   string `json:"projectId,omitempty"`
	ID          string `json:"id"`
	IssueID     string `json:"issueId"`
	At          int64  `json:"at"`
	FiredAt     int64  `json:"firedAt,omitempty"`
	DismissedAt int64  `json:"dismissedAt,omitempty"`
}
type Recurrence struct {
	ID          string `json:"id"`
	Form        Issue  `json:"form"`
	Every       int    `json:"every"`
	Unit        string `json:"unit"`
	NextDue     string `json:"nextDue"`
	EndDate     string `json:"endDate,omitempty"`
	AnchorDay   int    `json:"anchorDay"`
	Paused      bool   `json:"paused"`
	LastIssueID string `json:"lastIssueId,omitempty"`
	LastError   string `json:"lastError,omitempty"`
}

func ensureMap[T any](m *map[string]T) {
	if *m == nil {
		*m = map[string]T{}
	}
}
func (d *Data) initRecords() {
	ensureMap(&d.Drafts)
	ensureMap(&d.Templates)
	ensureMap(&d.Activities)
	ensureMap(&d.Comments)
	ensureMap(&d.Relations)
	ensureMap(&d.Attachments)
	ensureMap(&d.Milestones)
	ensureMap(&d.Documents)
	ensureMap(&d.Updates)
	ensureMap(&d.Folders)
	ensureMap(&d.Reminders)
	ensureMap(&d.Recurrences)
	ensureMap(&d.Erased)
}
func cloneIssue(i Issue) Issue {
	i.Body = slices.Clone(i.Body)
	i.LabelIDs = slices.Clone(i.LabelIDs)
	return i
}

// RawMessage preserves indentation on decode. Compact it at storage boundaries
// so a pretty-printed portable backup restores the exact editor/history value.
func compactRaw(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return raw
	}
	var b bytes.Buffer
	if json.Compact(&b, raw) != nil {
		return raw
	}
	return b.Bytes()
}
func (d *Data) normalizeJSON() {
	for id, v := range d.Issues {
		v.Body = compactRaw(v.Body)
		d.Issues[id] = v
	}
	for id, v := range d.Drafts {
		v.Form.Body = compactRaw(v.Form.Body)
		v.Base.Body = compactRaw(v.Base.Body)
		d.Drafts[id] = v
	}
	for id, v := range d.Templates {
		v.Form.Body = compactRaw(v.Form.Body)
		d.Templates[id] = v
	}
	for id, v := range d.Recurrences {
		v.Form.Body = compactRaw(v.Form.Body)
		d.Recurrences[id] = v
	}
	for id, v := range d.Activities {
		v.Before = compactRaw(v.Before)
		v.After = compactRaw(v.After)
		d.Activities[id] = v
	}
}
func (d Data) cloneRecords(n *Data) {
	for id, v := range d.Erased {
		n.Erased[id] = v
	}
	for id, v := range d.Drafts {
		v.Form = cloneIssue(v.Form)
		v.Base = cloneIssue(v.Base)
		v.Relation = v.Relation.Clone()
		n.Drafts[id] = v
	}
	for id, v := range d.Templates {
		v.Form = cloneIssue(v.Form)
		n.Templates[id] = v
	}
	for id, v := range d.Activities {
		v.Fields = slices.Clone(v.Fields)
		v.Before = slices.Clone(v.Before)
		v.After = slices.Clone(v.After)
		n.Activities[id] = v
	}
	for id, v := range d.Comments {
		n.Comments[id] = v
	}
	for id, v := range d.Relations {
		n.Relations[id] = v
	}
	for id, v := range d.Attachments {
		v.Data = slices.Clone(v.Data)
		n.Attachments[id] = v
	}
	for id, v := range d.Milestones {
		n.Milestones[id] = v
	}
	for id, v := range d.Documents {
		n.Documents[id] = v
	}
	for id, v := range d.Updates {
		n.Updates[id] = v
	}
	for id, v := range d.Folders {
		v.Items = slices.Clone(v.Items)
		n.Folders[id] = v
	}
	for id, v := range d.Reminders {
		n.Reminders[id] = v
	}
	for id, v := range d.Recurrences {
		v.Form = cloneIssue(v.Form)
		n.Recurrences[id] = v
	}
}
func validHealth(s string) bool {
	return slices.Contains([]string{"", "on_track", "at_risk", "off_track"}, s)
}
func validURL(s string) bool {
	u, e := url.Parse(s)
	return e == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host != "" && u.User == nil
}
func validForm(i Issue) bool {
	return utf8.ValidString(i.Title) && utf8.RuneCountInString(i.Title) <= 500 && utf8.ValidString(i.Plain) && (len(i.Body) == 0 || json.Valid(i.Body)) && ValidDate(i.Start) && ValidDate(i.Due) && i.Priority >= 0 && i.Priority <= 4 && (i.Status == "" || slices.Contains(Statuses, i.Status))
}
func (d Data) ownerExists(kind, id string) bool {
	switch kind {
	case "issue":
		_, ok := d.Issues[id]
		return ok
	case "project":
		_, ok := d.Projects[id]
		return ok
	}
	return false
}
func (d Data) ownerActive(kind, id string) bool {
	if !d.ownerExists(kind, id) {
		return false
	}
	if kind == "issue" {
		return d.Issues[id].DeletedAt == 0
	}
	return d.Projects[id].DeletedAt == 0
}
func (d Data) validateRecords() error {
	parents := map[string][]string{}
	for id, i := range d.Issues {
		if i.ParentID != "" {
			parents[id] = []string{i.ParentID}
		}
	}
	if cyclic(parents) {
		return errors.New("parent_cycle")
	}
	dependencies := map[string][]string{}
	for id, p := range d.Projects {
		seen := map[string]bool{}
		for _, other := range p.Dependencies {
			if _, ok := d.Projects[other]; !ok || id == other || seen[other] {
				return errors.New("invalid_dependency")
			}
			seen[other] = true
		}
		dependencies[id] = p.Dependencies
	}
	if cyclic(dependencies) {
		return errors.New("dependency_cycle")
	}
	for id, v := range d.Drafts {
		if id == "" || id != v.ID || !validForm(v.Form) || !validForm(v.Base) || !v.Relation.Valid() || v.IssueID != "" && v.Relation != nil {
			return errors.New("invalid_draft")
		}
	}
	for id, v := range d.Templates {
		if id == "" || id != v.ID || !validName(v.Name) || !validForm(v.Form) {
			return errors.New("invalid_template")
		}
	}
	for id, v := range d.Activities {
		if id == "" || id != v.ID || !d.ownerExists(v.OwnerKind, v.OwnerID) || v.Kind == "" || v.TargetID == "" || v.Action == "" || len(v.Before) > 0 && !json.Valid(v.Before) || len(v.After) > 0 && !json.Valid(v.After) {
			return errors.New("invalid_activity")
		}
	}
	commentParents := map[string][]string{}
	for id, v := range d.Comments {
		if id == "" || id != v.ID || !d.ownerExists("issue", v.IssueID) || strings.TrimSpace(v.Body) == "" || !utf8.ValidString(v.Body) {
			return errors.New("invalid_comment")
		}
		if v.ParentID != "" {
			parent, exists := d.Comments[v.ParentID]
			if !exists || parent.IssueID != v.IssueID || v.ParentID == id {
				return errors.New("invalid_comment_parent")
			}
			commentParents[id] = []string{v.ParentID}
		}
	}
	if cyclic(commentParents) {
		return errors.New("comment_cycle")
	}
	blocks, duplicates, relationKeys := map[string][]string{}, map[string][]string{}, map[string]bool{}
	for id, v := range d.Relations {
		if id == "" || id != v.ID || !d.ownerExists("issue", v.From) || !d.ownerExists("issue", v.To) || v.From == v.To || !slices.Contains([]string{"related", "blocks", "duplicate"}, v.Kind) {
			return errors.New("invalid_relation")
		}
		from, to := v.From, v.To
		if v.Kind == "related" && from > to {
			from, to = to, from
		}
		key := v.Kind + "/" + from + "/" + to
		if relationKeys[key] {
			return errors.New("duplicate_relation")
		}
		relationKeys[key] = true
		if v.Kind == "blocks" {
			blocks[v.From] = append(blocks[v.From], v.To)
		}
		if v.Kind == "duplicate" {
			if len(duplicates[v.From]) > 0 {
				return errors.New("duplicate_target_exists")
			}
			duplicates[v.From] = []string{v.To}
		}
	}
	if cyclic(blocks) || cyclic(duplicates) {
		return errors.New("relation_cycle")
	}
	for id, v := range d.Attachments {
		if id == "" || id != v.ID || !d.attachmentOwnerExists(v.OwnerKind, v.OwnerID) || !validName(v.Name) || len(v.Data) > MaxAttachmentBytes {
			return errors.New("invalid_attachment")
		}
		if v.URL != "" {
			if !validURL(v.URL) || len(v.Data) != 0 || v.SHA256 != "" {
				return errors.New("invalid_attachment_url")
			}
		} else {
			hash := sha256.Sum256(v.Data)
			if hex.EncodeToString(hash[:]) != v.SHA256 {
				return errors.New("attachment_checksum_mismatch")
			}
		}
	}
	for id, v := range d.Milestones {
		if id == "" || id != v.ID || !d.ownerExists("project", v.ProjectID) || !validName(v.Name) || !ValidDate(v.Due) {
			return errors.New("invalid_milestone")
		}
	}
	for id, v := range d.Documents {
		if id == "" || id != v.ID || !d.ownerExists(v.OwnerKind, v.OwnerID) || !validName(v.Name) || !utf8.ValidString(v.Body) {
			return errors.New("invalid_document")
		}
	}
	for id, v := range d.Updates {
		if id == "" || id != v.ID || !d.ownerExists("project", v.ProjectID) || strings.TrimSpace(v.Body) == "" || !utf8.ValidString(v.Body) || !validHealth(v.Health) {
			return errors.New("invalid_project_update")
		}
	}
	for id, v := range d.Folders {
		if id == "" || id != v.ID || !validName(v.Name) {
			return errors.New("invalid_folder")
		}
		seen := map[FavoriteRef]bool{}
		for _, ref := range v.Items {
			_, view := d.Views[ref.ID]
			if seen[ref] || !(d.ownerExists(ref.Kind, ref.ID) || ref.Kind == "view" && view) {
				return errors.New("invalid_favorite")
			}
			seen[ref] = true
		}
	}
	for id, v := range d.Erased {
		if id == "" || id != v.ID || v.Kind != "issue" && v.Kind != "project" {
			return errors.New("invalid_erasure")
		}
	}
	for id, v := range d.Reminders {
		kind, owner := v.Owner()
		if id == "" || id != v.ID || kind == "" || !d.ownerExists(kind, owner) || v.At <= 0 {
			return errors.New("invalid_reminder")
		}
	}
	for id, v := range d.Recurrences {
		if id == "" || id != v.ID || !validForm(v.Form) || !validName(v.Form.Title) || v.Every < 1 || v.Every > 365 || !slices.Contains([]string{"day", "week", "month", "year"}, v.Unit) || v.NextDue == "" || !ValidDate(v.NextDue) || !ValidDate(v.EndDate) || v.AnchorDay < 1 || v.AnchorDay > 31 {
			return errors.New("invalid_recurrence")
		}
	}
	return nil
}
func cyclic(edges map[string][]string) bool {
	colors := map[string]uint8{}
	var visit func(string) bool
	visit = func(id string) bool {
		if colors[id] == 1 {
			return true
		}
		if colors[id] == 2 {
			return false
		}
		colors[id] = 1
		for _, to := range edges[id] {
			if visit(to) {
				return true
			}
		}
		colors[id] = 2
		return false
	}
	for id := range edges {
		if visit(id) {
			return true
		}
	}
	return false
}

type recordKey struct{ kind, id string }

func addRecords[T any](out map[recordKey]any, kind string, values map[string]T) {
	for id, value := range values {
		out[recordKey{kind, id}] = value
	}
}
func (d Data) records() map[recordKey]any {
	out := map[recordKey]any{}
	addRecords(out, "draft", d.Drafts)
	addRecords(out, "template", d.Templates)
	addRecords(out, "activity", d.Activities)
	addRecords(out, "comment", d.Comments)
	addRecords(out, "relation", d.Relations)
	addRecords(out, "attachment", d.Attachments)
	addRecords(out, "milestone", d.Milestones)
	addRecords(out, "document", d.Documents)
	addRecords(out, "update", d.Updates)
	addRecords(out, "folder", d.Folders)
	addRecords(out, "reminder", d.Reminders)
	addRecords(out, "recurrence", d.Recurrences)
	addRecords(out, "erased", d.Erased)
	return out
}
func decodeRecord[T any](m map[string]T, id string, raw []byte) error {
	var v T
	if e := json.Unmarshal(raw, &v); e != nil {
		return e
	}
	m[id] = v
	return nil
}
func (d *Data) readRecord(kind, id string, raw []byte) error {
	switch kind {
	case "draft":
		return decodeRecord(d.Drafts, id, raw)
	case "template":
		return decodeRecord(d.Templates, id, raw)
	case "activity":
		return decodeRecord(d.Activities, id, raw)
	case "comment":
		return decodeRecord(d.Comments, id, raw)
	case "relation":
		return decodeRecord(d.Relations, id, raw)
	case "attachment":
		return decodeRecord(d.Attachments, id, raw)
	case "milestone":
		return decodeRecord(d.Milestones, id, raw)
	case "document":
		return decodeRecord(d.Documents, id, raw)
	case "update":
		return decodeRecord(d.Updates, id, raw)
	case "folder":
		return decodeRecord(d.Folders, id, raw)
	case "reminder":
		return decodeRecord(d.Reminders, id, raw)
	case "recurrence":
		return decodeRecord(d.Recurrences, id, raw)
	case "erased":
		return decodeRecord(d.Erased, id, raw)
	default:
		return errors.New("unknown_record_kind")
	}
}
