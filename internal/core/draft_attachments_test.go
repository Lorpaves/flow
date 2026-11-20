package core

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDraftAttachmentLifecycleAndBackup(t *testing.T) {
	s, _ := testStore(t)
	var draftID, fileID, copyID string
	before, err := s.Update(func(d *Data) error {
		v, err := d.SaveDraft(IssueDraft{Form: Issue{Title: "Media", Status: "todo"}})
		if err != nil {
			return err
		}
		draftID = v.ID
		file, err := d.AddAttachment(Attachment{OwnerKind: "draft", OwnerID: v.ID, Name: "image.png", Data: []byte("bytes")})
		if err != nil {
			return err
		}
		fileID = file.ID
		v.Form.Body = json.RawMessage(`{"paragraphs":[{"runs":[{"text":"image","attachment":{"url":"` + AttachmentURLPrefix + file.ID + `"}}]}]}`)
		d.Drafts[v.ID] = v
		copy, err := d.CopyDraft(v.ID)
		copyID = copy.ID
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(before.Activities) != 0 {
		t.Fatal("draft media entered activity feed")
	}
	if strings.Contains(string(before.Drafts[copyID].Form.Body), fileID) {
		t.Fatal("copy retained old attachment reference")
	}
	backupPath := filepath.Join(t.TempDir(), "backup.json")
	if err = WriteBackup(backupPath, before); err != nil {
		t.Fatal(err)
	}
	backup, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatal(err)
	}
	var envelope Backup
	if err = json.Unmarshal(backup, &envelope); err != nil || envelope.Version != schemaVersion || len(envelope.Data.Attachments) != 2 {
		t.Fatal("backup lost draft media", err)
	}
	// A failing submit rolls back both owner transfer and issue creation.
	_, err = s.Update(func(d *Data) error { d.AdoptDraftAttachments(draftID, "missing"); return errors.New("abort") })
	if err == nil || s.Snapshot().Attachments[fileID].OwnerKind != "draft" {
		t.Fatal("failed mutation leaked ownership")
	}
	var issue Issue
	after, err := s.Update(func(d *Data) error {
		d.RemoveDraft(draftID)
		var err error
		issue, err = d.ApplyDraft(copyID)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Drafts) != 0 || len(after.Attachments) != 1 {
		t.Fatal("discard/submit did not clean draft owners")
	}
	for _, file := range after.Attachments {
		if file.OwnerID != issue.ID || file.OwnerKind != "issue" || !strings.Contains(string(issue.Body), file.ID) {
			t.Fatal("adoption broke inline reference")
		}
	}
	if len(after.History("issue", issue.ID)) < 2 {
		t.Fatal("submission omitted attachment activity")
	}
	copy, err := after.CopyIssue(issue.ID, "Copied issue", false)
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range after.Attachments {
		if file.OwnerID == copy.ID && !strings.Contains(string(copy.Body), file.ID) {
			t.Fatal("issue copy retained source file reference")
		}
	}
	recovery, err := after.SaveDraft(IssueDraft{IssueID: issue.ID, Base: issue, Form: issue})
	if err != nil {
		t.Fatal(err)
	}
	recoveryCopy, err := after.CopyDraft(recovery.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !after.HasDraftAttachments(recoveryCopy.ID) || string(recoveryCopy.Form.Body) == string(issue.Body) {
		t.Fatal("recovered issue copy did not make independent media")
	}
	if err = after.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestAttachmentRebindingPreservesUnrelatedBody(t *testing.T) {
	body := json.RawMessage(`{"text":"flow-attachment://old","revision":9007199254740993,"attachment":{"url":"flow-attachment://old"}}`)
	updated := remapAttachmentBody(body, map[string]string{"flow-attachment://old": "flow-attachment://new"})
	if !strings.Contains(string(updated), `"text":"flow-attachment://old"`) || !strings.Contains(string(updated), `9007199254740993`) || !strings.Contains(string(updated), `"url":"flow-attachment://new"`) {
		t.Fatal(string(updated))
	}
	unchanged := json.RawMessage(`{ "text": "unchanged", "revision": 9007199254740993 }`)
	if string(remapAttachmentBody(unchanged, map[string]string{"flow-attachment://old": "flow-attachment://new"})) != string(unchanged) {
		t.Fatal("unrelated body was reserialized")
	}
}
