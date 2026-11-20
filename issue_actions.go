package main

import (
	"encoding/json"
	"errors"
	"flow/internal/core"
	"net/url"
	"regexp"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/richtext"
)

var branchCharacters = regexp.MustCompile(`[^a-z0-9]+`)

func issueURL(id string) string { return "flow://issue/" + url.PathEscape(id) }
func (a *app) copyIssue(i core.Issue, kind string) {
	if d := a.drafts[i.ID]; d != nil && (d.dirty || d.busy) {
		i = d.issue
		i.Body, _ = json.Marshal(d.doc.Text())
		i.Plain = d.doc.Text().PlainText()
	}
	text := i.Code()
	switch kind {
	case "title":
		text = i.Title
	case "markdown":
		body := documentFor(i).Text().Markdown()
		text = "# " + i.Title + "\n\n[" + i.Code() + "](" + issueURL(i.ID) + ")\n\n" + body
	case "url":
		text = issueURL(i.ID)
	case "branch":
		slug := strings.Trim(branchCharacters.ReplaceAllString(strings.ToLower(i.Title), "-"), "-")
		if slug == "" {
			slug = "issue"
		}
		text = strings.ToLower(i.Code()) + "-" + slug
	case "prompt":
		var body richtext.Text
		if json.Unmarshal(i.Body, &body) != nil {
			body = richtext.NewText(i.Plain)
		}
		text = i.Code() + ": " + i.Title + "\n" + issueURL(i.ID) + "\n\n" + body.Markdown()
	}
	a.services.WriteClipboard(text)
	a.notice = a.tr("已复制", "Copied")
}
func (a *app) openLink(raw string) error {
	u, e := url.Parse(raw)
	if e != nil || u.Scheme != "flow" || (u.Host != "issue" && u.Host != "project") || u.RawQuery != "" || u.Fragment != "" {
		return errors.New("invalid_flow_link")
	}
	ref := strings.TrimPrefix(u.Path, "/")
	if ref == "" || strings.Contains(ref, "/") {
		return errors.New("invalid_flow_link")
	}
	if !a.ready {
		a.pendingLink = raw
		return nil
	}
	if u.Host == "project" {
		p, ok := a.data.Projects[ref]
		if !ok {
			return errors.New("project_not_found")
		}
		if p.DeletedAt != 0 {
			a.navigate("trash", "")
		} else {
			a.navigate("project", p.ID)
			a.projectTab = "overview"
		}
		return nil
	}
	for _, i := range a.data.Issues {
		if i.ID == ref || strings.EqualFold(i.Code(), ref) {
			if i.DeletedAt != 0 {
				a.navigate("trash", "")
				a.openIssue(i.ID)
			} else {
				a.openSearchIssue(i.ID, "")
			}
			return nil
		}
	}
	return errors.New("issue_not_found")
}
func (a *app) selectedIDs() []string {
	ids := []string{}
	for _, i := range a.filtered() {
		if a.selected[i.ID] {
			ids = append(ids, i.ID)
		}
	}
	return ids
}
func (a *app) batchUpdate(ids []string, edit func(*core.Issue)) {
	ids = slices.Clone(ids)
	a.apply(func(d *core.Data) error {
		for _, id := range ids {
			i, ok := d.Issues[id]
			if !ok || i.DeletedAt != 0 {
				continue
			}
			edit(&i)
			if e := d.PutIssue(i); e != nil {
				return e
			}
		}
		return nil
	}, nil)
}
func (a *app) batchPropertyControls(c *ui.Context, ids []string) {
	projectButton := textButton(c, a.tr("项目", "Project")).Label(a.tr("批量项目", "Set project"))
	projects := []pickerItem{{id: "", label: a.tr("无项目", "No project")}}
	for _, p := range a.data.ProjectList(false, false) {
		projects = append(projects, pickerItem{id: p.ID, label: p.Name})
	}
	a.picker(c, projectButton, a.tr("批量项目", "Set project"), "", 0, projects, false, func(value string) { a.apply(func(d *core.Data) error { return d.MoveIssueProjects(ids, value) }, nil) })
	textButton(c, a.tr("标签", "Labels")).Label(a.tr("批量标签", "Set labels")).PopoverMenuButton(func(m *ui.Menu) {
		m.Submenu(a.tr("添加标签", "Add label"), func(m *ui.Menu) {
			for _, l := range a.data.LabelList() {
				if l.ProjectID != "" && slices.ContainsFunc(ids, func(id string) bool { return a.data.Issues[id].ProjectID != l.ProjectID }) {
					continue
				}
				if m.Item(l.Name).Chosen() {
					a.batchUpdate(ids, func(i *core.Issue) {
						if !slices.Contains(i.LabelIDs, l.ID) {
							i.LabelIDs = append(slices.Clone(i.LabelIDs), l.ID)
						}
					})
				}
			}
		})
		m.Submenu(a.tr("移除标签", "Remove label"), func(m *ui.Menu) {
			for _, l := range a.data.LabelList() {
				if m.Item(l.Name).Chosen() {
					a.batchUpdate(ids, func(i *core.Issue) {
						i.LabelIDs = slices.DeleteFunc(slices.Clone(i.LabelIDs), func(id string) bool { return id == l.ID })
					})
				}
			}
		})
		if m.Item(a.tr("清除所有标签", "Clear all labels")).Chosen() {
			a.batchUpdate(ids, func(i *core.Issue) { i.LabelIDs = nil })
		}
	})
	textButton(c, a.tr("日期", "Due date")).Label(a.tr("批量截止日期", "Set due date")).OnClick(func() {
		a.chooseDate(a.tr("批量截止日期", "Set due date"), "", func(value string) { a.batchUpdate(ids, func(i *core.Issue) { i.Due = value }) })
	})
}
