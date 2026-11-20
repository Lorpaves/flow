package core

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strings"
	"time"
)

type trackedRecord struct {
	kind, id, ownerKind, ownerID string
	value                        any
}

func (d Data) tracked() map[recordKey]trackedRecord {
	out := map[recordKey]trackedRecord{}
	add := func(kind, id, ownerKind, ownerID string, value any) {
		out[recordKey{kind, id}] = trackedRecord{kind, id, ownerKind, ownerID, value}
	}
	for id, v := range d.Issues {
		add("issue", id, "issue", id, v)
	}
	for id, v := range d.Projects {
		add("project", id, "project", id, v)
	}
	for id, v := range d.Comments {
		add("comment", id, "issue", v.IssueID, v)
	}
	for id, v := range d.Relations {
		add("relation", id, "issue", v.From, v)
	}
	for id, v := range d.Attachments {
		// Drafts have no activity feed. Adoption appears as attachment creation
		// on the issue, and discarded drafts leave no ownerless history.
		if v.OwnerKind == "draft" || v.OwnerKind == "template" {
			continue
		}
		v.Data = nil
		add("attachment", id, v.OwnerKind, v.OwnerID, v)
	}
	for id, v := range d.Documents {
		add("document", id, v.OwnerKind, v.OwnerID, v)
	}
	for id, v := range d.Milestones {
		add("milestone", id, "project", v.ProjectID, v)
	}
	for id, v := range d.Updates {
		add("update", id, "project", v.ProjectID, v)
	}
	return out
}
func jsonObject(v any) map[string]json.RawMessage {
	b, _ := json.Marshal(v)
	out := map[string]json.RawMessage{}
	_ = json.Unmarshal(b, &out)
	return out
}
func jsonSame(a, b json.RawMessage) bool {
	if len(a) == 0 || len(b) == 0 {
		return len(a) == len(b)
	}
	var av, bv any
	ad, bd := json.NewDecoder(bytes.NewReader(a)), json.NewDecoder(bytes.NewReader(b))
	ad.UseNumber()
	bd.UseNumber()
	if ad.Decode(&av) != nil || bd.Decode(&bv) != nil {
		return false
	}
	return reflect.DeepEqual(av, bv)
}
func changedFields(before, after any) []string {
	b, a := jsonObject(before), jsonObject(after)
	keys := map[string]bool{}
	for k := range b {
		keys[k] = true
	}
	for k := range a {
		keys[k] = true
	}
	out := []string{}
	for k := range keys {
		if slices.Contains([]string{"id", "number", "createdAt", "updatedAt", "position"}, k) {
			continue
		}
		if !jsonSame(a[k], b[k]) {
			out = append(out, k)
		}
	}
	slices.Sort(out)
	return out
}

// RecordChanges runs inside the same transaction as the edit, so failed writes
// cannot leave phantom history. Attachment file bytes are deliberately excluded.
func (d *Data) RecordChanges(before Data, source string, now time.Time) {
	old, next := before.tracked(), d.tracked()
	keys := map[recordKey]bool{}
	for k := range old {
		keys[k] = true
	}
	for k := range next {
		keys[k] = true
	}
	ordered := make([]recordKey, 0, len(keys))
	for k := range keys {
		ordered = append(ordered, k)
	}
	slices.SortFunc(ordered, func(a, b recordKey) int {
		if a.kind != b.kind {
			return strings.Compare(a.kind, b.kind)
		}
		return strings.Compare(a.id, b.id)
	})
	for _, key := range ordered {
		prev, had := old[key]
		cur, has := next[key]
		if had && has && reflect.DeepEqual(prev.value, cur.value) {
			continue
		}
		fields := changedFields(prev.value, cur.value)
		if had && has && len(fields) == 0 {
			continue
		}
		action := "updated"
		if !had {
			action = "created"
		}
		if !has {
			action = "removed"
			cur = prev
		}
		if had && has {
			b, a := jsonObject(prev.value), jsonObject(cur.value)
			if !jsonSame(b["deletedAt"], a["deletedAt"]) {
				var stamp int64
				_ = json.Unmarshal(a["deletedAt"], &stamp)
				if stamp == 0 {
					action = "restored"
				} else {
					action = "trashed"
				}
			}
		}
		// What belonged to an issue or project deleted for good has no
		// history left to be told in.
		if !d.ownerExists(cur.ownerKind, cur.ownerID) {
			continue
		}
		v := Activity{ID: ID(), OwnerKind: cur.ownerKind, OwnerID: cur.ownerID, Kind: cur.kind, TargetID: cur.id, Action: action, Source: source, At: now.UnixMilli(), Fields: fields}
		if had {
			v.Before, _ = json.Marshal(prev.value)
		}
		if has {
			v.After, _ = json.Marshal(cur.value)
		}
		d.Activities[v.ID] = v
		// A relation is visible in the activity of both participants.
		if relation, ok := cur.value.(Relation); ok && key.kind == "relation" && d.ownerExists("issue", relation.To) {
			v.ID = ID()
			v.OwnerID = relation.To
			d.Activities[v.ID] = v
		}
	}
}
func (d Data) History(kind, id string) []Activity {
	out := []Activity{}
	for _, v := range d.Activities {
		if v.OwnerKind == kind && v.OwnerID == id {
			out = append(out, v)
		}
	}
	slices.SortFunc(out, func(a, b Activity) int {
		if a.At > b.At {
			return -1
		}
		if a.At < b.At {
			return 1
		}
		return strings.Compare(a.ID, b.ID)
	})
	return out
}
func (d Data) CanUndoActivity(v Activity) bool {
	if v.Action != "updated" || v.UndoneAt != 0 || len(v.Before) == 0 || len(v.After) == 0 || len(v.Fields) == 0 {
		return false
	}
	allowed := []string{}
	if v.Kind == "issue" {
		allowed = []string{"title", "body", "plain", "projectId", "parentId", "milestoneId", "status", "priority", "labelIds", "start", "due", "favorite"}
	}
	if v.Kind == "project" {
		allowed = []string{"name", "description", "status", "color", "due", "start", "priority", "health", "dependencies", "favorite", "archived"}
	}
	for _, field := range v.Fields {
		if !slices.Contains(allowed, field) {
			return false
		}
	}
	return true
}
func (d *Data) UndoActivity(id string) error {
	v, ok := d.Activities[id]
	if !ok || !d.CanUndoActivity(v) {
		return errors.New("history_not_undoable")
	}
	if !d.ownerActive(v.Kind, v.TargetID) {
		return errors.New("not_found")
	}
	var before, after map[string]json.RawMessage
	if json.Unmarshal(v.Before, &before) != nil || json.Unmarshal(v.After, &after) != nil {
		return errors.New("invalid_activity")
	}
	var current map[string]json.RawMessage
	if v.Kind == "issue" {
		current = jsonObject(d.Issues[v.TargetID])
	} else {
		current = jsonObject(d.Projects[v.TargetID])
	}
	// Undo only the fields this event changed. Unrelated concurrent edits survive;
	// a newer edit to the same field requires an explicit user decision.
	for _, field := range v.Fields {
		if !jsonSame(current[field], after[field]) {
			return errors.New("history_conflict")
		}
	}
	for _, field := range v.Fields {
		if value, ok := before[field]; ok {
			current[field] = value
		} else {
			delete(current, field)
		}
	}
	raw, e := json.Marshal(current)
	if e != nil {
		return e
	}
	if v.Kind == "issue" {
		var i Issue
		if e = json.Unmarshal(raw, &i); e != nil {
			return e
		}
		if e = d.PutIssue(i); e != nil {
			return e
		}
	} else {
		var p Project
		if e = json.Unmarshal(raw, &p); e != nil {
			return e
		}
		d.Projects[p.ID] = p
	}
	v.UndoneAt = time.Now().UnixMilli()
	d.Activities[id] = v
	return nil
}
func (d *Data) RestoreDescription(activityID string, useBefore bool) error {
	v, ok := d.Activities[activityID]
	if !ok || (v.Kind != "issue" && v.Kind != "project" && v.Kind != "document") {
		return errors.New("invalid_activity")
	}
	raw := v.After
	if useBefore {
		raw = v.Before
	}
	if len(raw) == 0 {
		return errors.New("invalid_activity")
	}
	if !d.ownerActive(v.OwnerKind, v.OwnerID) {
		return errors.New("not_found")
	}
	switch v.Kind {
	case "issue":
		var old Issue
		if e := json.Unmarshal(raw, &old); e != nil {
			return e
		}
		i := d.Issues[v.TargetID]
		i.Body, i.Plain = old.Body, old.Plain
		return d.PutIssue(i)
	case "project":
		var old Project
		if e := json.Unmarshal(raw, &old); e != nil {
			return e
		}
		p := d.Projects[v.TargetID]
		p.Description = old.Description
		d.Projects[p.ID] = p
	case "document":
		var old Document
		if e := json.Unmarshal(raw, &old); e != nil {
			return e
		}
		doc, ok := d.Documents[v.TargetID]
		if !ok {
			return errors.New("not_found")
		}
		doc.Body = old.Body
		doc.UpdatedAt = time.Now().UnixMilli()
		d.Documents[doc.ID] = doc
	}
	return nil
}
