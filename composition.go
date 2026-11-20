package main

import (
	"encoding/json"
	"flow/internal/core"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/richtext"
)

func documentFor(i core.Issue) *richtext.Document {
	var body richtext.Text
	if len(i.Body) == 0 || json.Unmarshal(i.Body, &body) != nil {
		body = richtext.NewText(i.Plain)
	}
	return richtext.NewDocument(body)
}
func (a *app) compositionForm() core.Issue {
	form := a.newIssue
	form.LabelIDs = slices.Clone(form.LabelIDs)
	if a.newDoc != nil {
		form.Body, _ = json.Marshal(a.newDoc.Text())
		form.Plain = a.newDoc.Text().PlainText()
	}
	return form
}
func compositionFingerprint(form core.Issue, relation ...*core.CreationRelation) string {
	if strings.TrimSpace(form.Title) == "" && strings.TrimSpace(form.Plain) == "" {
		hasMedia := false
		for _, paragraph := range documentFor(form).Text().Paragraphs() {
			for _, run := range paragraph.Runs {
				hasMedia = hasMedia || run.Attributes.Attachment != nil
			}
		}
		if !hasMedia {
			return ""
		}
	}
	raw, _ := json.Marshal(form)
	if len(relation) > 0 && relation[0] != nil {
		raw, _ = json.Marshal(struct {
			Form     core.Issue
			Relation *core.CreationRelation
		}{form, relation[0]})
	}
	return string(raw)
}
func (a *app) compositionDirty() bool {
	return a.composeID != "" && compositionFingerprint(a.compositionForm(), a.newRelation) != a.composeSaved[a.composeID]
}
func (a *app) initCompositions() {
	if a.compositions == nil {
		a.compositions = map[string]composition{}
	}
	if a.composeSaved == nil {
		a.composeSaved = map[string]string{}
	}
	if a.composeQueued == nil {
		a.composeQueued = map[string]core.IssueDraft{}
	}
	if a.composeBusy == nil {
		a.composeBusy = map[string]bool{}
	}
}
func (a *app) persistComposition() {
	a.initCompositions()
	if a.composeID == "" || a.composeID == a.composeSubmitting {
		return
	}
	a.queueComposition(core.IssueDraft{ID: a.composeID, Context: a.composeKey, Form: a.compositionForm(), Relation: a.newRelation.Clone()})
}
func (a *app) queueComposition(v core.IssueDraft) {
	id := v.ID
	fingerprint := compositionFingerprint(v.Form, v.Relation)
	if fingerprint == a.composeSaved[id] && !a.composeBusy[id] {
		delete(a.composeQueued, id)
		return
	}
	a.composeQueued[id] = v
	if a.composeBusy[id] {
		return
	}
	a.composeBusy[id] = true
	a.applyWithFailure(func(d *core.Data) error {
		if fingerprint == "" && !d.HasDraftAttachments(id) {
			d.RemoveDraft(id)
			return nil
		}
		_, e := d.SaveDraft(v)
		return e
	}, func() {
		a.composeBusy[id] = false
		a.composeSaved[id] = fingerprint
		next := a.composeQueued[id]
		if compositionFingerprint(next.Form, next.Relation) == fingerprint {
			delete(a.composeQueued, id)
		} else {
			a.queueComposition(next)
		}
		if a.closing() && a.composeID == id && a.compositionDirty() {
			a.persistComposition()
		}
	}, func() { a.composeBusy[id] = false })
}
func (a *app) observeComposition(c *ui.Context) {
	if a.composeID == "" {
		return
	}
	fingerprint := compositionFingerprint(a.compositionForm(), a.newRelation)
	if fingerprint != a.composeObserved {
		a.composeObserved = fingerprint
		a.composeChanged = c.Now()
	}
	if a.compositionDirty() && !a.composeBusy[a.composeID] && a.err == "" {
		left := 600*time.Millisecond - c.Now().Sub(a.composeChanged)
		if !a.newOpen || left <= 0 {
			a.persistComposition()
		} else {
			c.After(left)
		}
	}
}
func (a *app) stashComposition() {
	a.initCompositions()
	if a.newDoc != nil {
		a.persistComposition()
		a.compositions[a.composeKey] = composition{id: a.composeID, issue: a.compositionForm(), doc: a.newDoc, relation: a.newRelation.Clone()}
	}
}
func (a *app) loadComposition(id, context string, form core.Issue, doc *richtext.Document, relation ...*core.CreationRelation) {
	a.initCompositions()
	a.composeID, a.composeKey = id, context
	a.newIssue, a.newDoc = form, doc
	a.newRelation = nil
	if len(relation) > 0 {
		a.newRelation = relation[0].Clone()
	}
	if a.newDoc == nil {
		a.newDoc = a.mediaDocument(form)
	}
	a.newBody = a.newDoc.Text().PlainText()
	a.composeObserved = compositionFingerprint(a.compositionForm(), a.newRelation)
	a.composeChanged = time.Now()
	a.newDueOpen = form.Due != ""
	a.formError = ""
	a.newOpen = true
	a.newTitle.Focus()
}
func (a *app) newTask(parent, status string) {
	a.closePicker()
	a.stashComposition()
	form := core.Issue{ProjectID: a.projectID, ParentID: parent, Status: status}
	if status == "" {
		form.Status = "backlog"
	}
	if parent != "" {
		form.ProjectID = a.data.Issues[parent].ProjectID
	}
	context := form.ProjectID + "/" + parent + "/" + form.Status
	a.loadCreationContext(context, form, nil)
}
func (a *app) loadCreationContext(context string, form core.Issue, relation *core.CreationRelation) {
	context = a.creationContext(context)
	if saved, ok := a.compositions[context]; ok {
		a.loadComposition(saved.id, context, saved.issue, saved.doc, saved.relation)
		return
	}
	var latest core.IssueDraft
	for _, v := range a.data.Drafts {
		if v.IssueID == "" && v.Context == context && v.UpdatedAt > latest.UpdatedAt {
			latest = v
		}
	}
	if latest.ID != "" {
		a.composeSaved[latest.ID] = compositionFingerprint(latest.Form, latest.Relation)
		a.loadComposition(latest.ID, context, latest.Form, nil, latest.Relation)
		return
	}
	a.loadComposition(core.ID(), context, form, nil, relation)
}
func (a *app) closeComposition() { a.persistComposition(); a.newOpen = false; a.closePicker() }
func (a *app) discardComposition() {
	id, context := a.composeID, a.composeKey
	a.apply(func(d *core.Data) error { d.RemoveDraft(id); return nil }, func() {
		delete(a.compositions, context)
		delete(a.composeSaved, id)
		if a.composeID == id {
			form := a.newIssue
			form.Title = ""
			form.Body = nil
			form.Plain = ""
			a.loadComposition(core.ID(), context, form, nil, a.newRelation)
		}
	})
}
func (a *app) resumeDraft(id string, asCopy bool) {
	v, ok := a.data.Drafts[id]
	if !ok {
		return
	}
	if v.IssueID != "" && !asCopy {
		current, ok := a.data.Issues[v.IssueID]
		if !ok || current.DeletedAt != 0 {
			a.err = "issue_in_trash"
			return
		}
		a.openSearchIssue(v.IssueID, "")
		form := core.MergeIssueEdits(v.Base, v.Form, current)
		a.drafts[v.IssueID] = &draft{recoveryID: id, issue: form, base: current, doc: a.mediaDocument(form), bodySaved: documentJSON(a.mediaDocument(current)), editing: true}
		a.touch(a.drafts[v.IssueID], time.Now())
		return
	}
	a.stashComposition()
	form := v.Form
	relation := v.Relation.Clone()
	if asCopy {
		var copied core.IssueDraft
		a.apply(func(d *core.Data) error { var err error; copied, err = d.CopyDraft(id); return err }, func() {
			a.composeSaved[copied.ID] = compositionFingerprint(copied.Form)
			a.loadComposition(copied.ID, copied.Context, copied.Form, nil)
		})
		return
	}
	a.composeSaved[id] = compositionFingerprint(v.Form, relation)
	a.loadComposition(id, v.Context, form, nil, relation)
}
func (a *app) createIssue() {
	if a.pending > 0 || a.composeSubmitting != "" {
		return
	}
	form := a.compositionForm()
	if strings.TrimSpace(form.Title) == "" {
		a.formError = "title_required"
		return
	}
	if !core.ValidDate(form.Due) {
		a.formError = "invalid_issue"
		return
	}
	id, context := a.composeID, a.composeKey
	relation := a.newRelation.Clone()
	submitted := compositionFingerprint(form, relation)
	a.composeSubmitting = id
	var created string
	a.applyWithFailure(func(d *core.Data) error {
		i, e := d.CreateWithRelation(form, relation)
		if e != nil {
			return e
		}
		created = i.ID
		d.AdoptDraftAttachments(id, i.ID)
		d.RemoveDraft(id)
		return nil
	}, func() {
		a.composeSubmitting = ""
		a.createdIssue = created
		a.notice = a.tr("已创建 ", "Created ") + a.data.Issues[created].Code()
		delete(a.composeSaved, id)
		if a.composeID != id {
			for key, cached := range a.compositions {
				if cached.id != id {
					continue
				}
				if compositionFingerprint(cached.issue, cached.relation) != submitted {
					a.queueComposition(core.IssueDraft{ID: id, Context: key, Form: cached.issue, Relation: cached.relation.Clone()})
				} else {
					delete(a.compositions, key)
				}
			}
			return
		}
		// A native write completes later. Preserve typing, property changes or
		// a detached relation made after submit as an unsubmitted recovery draft.
		if compositionFingerprint(a.compositionForm(), a.newRelation) != submitted {
			a.persistComposition()
			return
		}
		if cached, ok := a.compositions[context]; ok && cached.id == id {
			delete(a.compositions, context)
		}
		wasOpen := a.newOpen
		form.Title, form.Plain = "", ""
		form.Body = nil
		a.loadComposition(core.ID(), context, form, nil, relation)
		a.newOpen = wasOpen && a.createMore
	}, func() {
		a.composeSubmitting = ""
		failure := a.err
		if a.composeID == id {
			a.persistComposition()
		} else {
			for key, cached := range a.compositions {
				if cached.id == id {
					a.queueComposition(core.IssueDraft{ID: id, Context: key, Form: cached.issue, Relation: cached.relation.Clone()})
				}
			}
		}
		a.err = failure
	})
}
func (a *app) startTemplate(id string, form core.Issue) {
	a.templateID = id
	a.templateForm = form
	a.templateName = form.Title
	if v, ok := a.data.Templates[id]; ok {
		a.templateName = v.Name
		a.templateForm = v.Form
	}
	a.templateDoc = a.mediaDocument(a.templateForm)
	a.templateOpen = true
}
func (a *app) saveTemplate() {
	v := core.IssueTemplate{ID: a.templateID, Name: strings.TrimSpace(a.templateName), Form: a.templateForm}
	a.apply(func(d *core.Data) error { _, e := d.SaveTemplate(v); return e }, func() { a.templateOpen = false; a.notice = a.tr("模板已保存", "Template saved") })
}
func (a *app) useTemplate(id string) {
	v, ok := a.data.Templates[id]
	if !ok {
		return
	}
	a.stashComposition()
	// Applying a template creates a separate recoverable draft and keeps the old one.
	form := v.Form
	var relation *core.CreationRelation
	if a.newOpen {
		relation = a.newRelation.Clone()
		form.ParentID = a.newIssue.ParentID
		if form.ParentID != "" || relation != nil && (relation.Kind == "parent_issue" || relation.Kind == "sub_issue") {
			form.ProjectID = a.newIssue.ProjectID
		}
	}
	context := form.ProjectID + "/" + form.ParentID + "/" + form.Status
	if relation != nil {
		context = "related/" + relation.IssueID + "/" + relation.Kind
	}
	context = a.creationContext(context)
	originalID, originalDoc := a.composeID, a.newDoc
	version := uint64(0)
	if originalDoc != nil {
		version = originalDoc.Version()
	}
	var saved core.IssueDraft
	a.apply(func(d *core.Data) error {
		var err error
		saved, err = d.SaveDraft(core.IssueDraft{Context: context, Form: form, Relation: relation})
		if err != nil {
			return err
		}
		saved.Form.Body, err = d.CloneBodyAttachments(saved.Form.Body, "draft", saved.ID)
		if err != nil {
			return err
		}
		d.Drafts[saved.ID] = saved
		return nil
	}, func() {
		// A template chosen before another edit must not replace that new edit.
		if a.composeID != originalID || a.newDoc != originalDoc || originalDoc != nil && originalDoc.Version() != version {
			a.notice = a.tr("模板草稿已保存", "Template draft saved")
			return
		}
		a.loadComposition(saved.ID, context, saved.Form, nil, relation)
	})
}
