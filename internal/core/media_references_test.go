package core

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTemplateMediaSurvivesSourceAndTemplateDeletion(t *testing.T) {
	s, _ := testStore(t)
	var templateID, issueID string
	data, err := s.Update(func(d *Data) error {
		draft, err := d.SaveDraft(IssueDraft{Form: Issue{Title: "Source"}})
		if err != nil {
			return err
		}
		file, err := d.AddAttachment(Attachment{OwnerKind: "draft", OwnerID: draft.ID, Name: "file.txt", Data: []byte("portable bytes")})
		if err != nil {
			return err
		}
		form := draft.Form
		form.Body = json.RawMessage(`{"paragraphs":[{"runs":[{"text":"file","attributes":{"link":"` + AttachmentURLPrefix + file.ID + `"}},{"text":"again","attributes":{"link":"` + AttachmentURLPrefix + file.ID + `"}}]}]}`)
		template, err := d.SaveTemplate(IssueTemplate{Name: "Own files", Form: form})
		if err != nil {
			return err
		}
		templateID = template.ID
		if strings.Contains(string(template.Form.Body), file.ID) {
			t.Fatal("template shares source")
		}
		d.RemoveDraft(draft.ID)
		issue, err := d.CreateFromTemplate(template.ID)
		if err != nil {
			return err
		}
		issueID = issue.ID
		d.RemoveTemplate(template.ID)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(data.Attachments) != 1 || len(data.Templates) != 0 || len(data.Drafts) != 0 {
		t.Fatal("copies were not deduplicated/cleaned")
	}
	for _, file := range data.Attachments {
		if file.OwnerKind != "issue" || file.OwnerID != issueID || string(file.Data) != "portable bytes" || strings.Count(string(data.Issues[issueID].Body), file.ID) != 2 {
			t.Fatal("template deletion broke task media")
		}
	}
	for _, activity := range data.Activities {
		if activity.OwnerID == templateID {
			t.Fatal("template produced orphaned history")
		}
	}
	backup := filepath.Join(t.TempDir(), "portable.flow.json")
	if err = WriteBackup(backup, data); err != nil {
		t.Fatal(err)
	}
	imported, err := ReadBackup(backup)
	if err != nil || len(imported.Attachments) != 1 {
		t.Fatal("media backup failed", err)
	}
}

func TestLegacyTemplateMediaMigrationAndImport(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(filepath.Join(dir, "flow.db"))
	if err != nil {
		t.Fatal(err)
	}
	var templateID, fileID string
	legacy, err := store.Update(func(d *Data) error {
		issue, err := d.AddIssue("Source", "", "", "todo")
		if err != nil {
			return err
		}
		file, err := d.AddAttachment(Attachment{OwnerKind: "issue", OwnerID: issue.ID, Name: "legacy.png", Data: []byte("legacy bytes")})
		if err != nil {
			return err
		}
		fileID, templateID = file.ID, ID()
		issue.Body = json.RawMessage(`{"attachment":{"url":"` + AttachmentURLPrefix + file.ID + `"},"text":"unrelated"}`)
		d.Templates[templateID] = IssueTemplate{ID: templateID, Name: "Legacy template", Form: issue}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.db.Execute(context.Background(), "PRAGMA user_version=8"); err != nil {
		t.Fatal(err)
	}
	store.Close()
	store, err = Open(filepath.Join(dir, "flow.db"))
	if err != nil {
		t.Fatal(err)
	}
	data := store.Snapshot()
	if strings.Contains(string(data.Templates[templateID].Form.Body), fileID) || len(data.Attachments) != 2 {
		t.Fatal("migration left template sharing bytes")
	}
	store.Close()
	store, err = Open(filepath.Join(dir, "flow.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if len(store.Snapshot().Attachments) != 2 {
		t.Fatal("migration not idempotent")
	}
	backup := filepath.Join(dir, "v8.json")
	raw, _ := json.Marshal(Backup{Format: "flow-backup", Version: 8, Data: legacy})
	if err = os.WriteFile(backup, raw, 0600); err != nil {
		t.Fatal(err)
	}
	imported, err := ReadBackup(backup)
	if err != nil || len(imported.Attachments) != 2 || strings.Contains(string(imported.Templates[templateID].Form.Body), fileID) {
		t.Fatal("old backup not isolated", err)
	}
}
