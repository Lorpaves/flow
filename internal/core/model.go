package core

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

var Statuses = []string{"backlog", "todo", "progress", "review", "done", "canceled", "duplicate"}
var ProjectStatuses = []string{"planned", "active", "completed", "canceled"}

type Project struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	Status       string   `json:"status"`
	Color        string   `json:"color"`
	Due          string   `json:"due"`
	Start        string   `json:"start,omitempty"`
	Priority     int      `json:"priority,omitempty"`
	Health       string   `json:"health,omitempty"`
	Dependencies []string `json:"dependencies,omitempty"`
	LabelIDs     []string `json:"labelIds,omitempty"`
	Favorite     bool     `json:"favorite"`
	Archived     bool     `json:"archived"`
	DeletedAt    int64    `json:"deletedAt"`
	CreatedAt    int64    `json:"createdAt"`
}
type Issue struct {
	ID          string          `json:"id"`
	Number      int             `json:"number"`
	Title       string          `json:"title"`
	Body        json.RawMessage `json:"body,omitempty"`
	Plain       string          `json:"plain"`
	ProjectID   string          `json:"projectId"`
	ParentID    string          `json:"parentId"`
	MilestoneID string          `json:"milestoneId,omitempty"`
	Status      string          `json:"status"`
	Priority    int             `json:"priority"`
	LabelIDs    []string        `json:"labelIds"`
	Start       string          `json:"start,omitempty"`
	Due         string          `json:"due"`
	Position    int64           `json:"position"`
	Favorite    bool            `json:"favorite"`
	DeletedAt   int64           `json:"deletedAt"`
	CreatedAt   int64           `json:"createdAt"`
	UpdatedAt   int64           `json:"updatedAt"`
}

func (i Issue) Code() string { return fmt.Sprintf("FLW-%d", i.Number) }

// NormalizeDates drops a start that no longer comes before the due date: an
// issue of one day keeps only its due date.
func (i *Issue) NormalizeDates() {
	if i.Start != "" && (i.Due == "" || i.Start >= i.Due) {
		i.Start = ""
	}
}

type Label struct {
	Kind      string `json:"kind,omitempty"`
	ID        string `json:"id"`
	Name      string `json:"name"`
	Color     string `json:"color"`
	Group     string `json:"group,omitempty"`
	ProjectID string `json:"projectId,omitempty"`
}
type Preferences struct {
	Locale      string                `json:"locale"`
	Initialized bool                  `json:"initialized"`
	NextNumber  int                   `json:"nextNumber"`
	ViewStates  map[string]ViewConfig `json:"viewStates,omitempty"`
}
type Data struct {
	Drafts      map[string]IssueDraft     `json:"drafts,omitempty"`
	Templates   map[string]IssueTemplate  `json:"templates,omitempty"`
	Activities  map[string]Activity       `json:"activities,omitempty"`
	Comments    map[string]Comment        `json:"comments,omitempty"`
	Relations   map[string]Relation       `json:"relations,omitempty"`
	Attachments map[string]Attachment     `json:"attachments,omitempty"`
	Milestones  map[string]Milestone      `json:"milestones,omitempty"`
	Documents   map[string]Document       `json:"documents,omitempty"`
	Updates     map[string]ProjectUpdate  `json:"updates,omitempty"`
	Folders     map[string]FavoriteFolder `json:"folders,omitempty"`
	Reminders   map[string]Reminder       `json:"reminders,omitempty"`
	Recurrences map[string]Recurrence     `json:"recurrences,omitempty"`
	Erased      map[string]Erasure        `json:"erased,omitempty"` // issues and projects deleted for good (erase.go)
	Views       map[string]SavedView      `json:"views,omitempty"`
	Projects    map[string]Project        `json:"projects"`
	Issues      map[string]Issue          `json:"issues"`
	Labels      map[string]Label          `json:"labels"`
	Preferences Preferences               `json:"preferences"`
}

func Empty() Data {
	d := Data{Views: map[string]SavedView{}, Projects: map[string]Project{}, Issues: map[string]Issue{}, Labels: map[string]Label{}, Preferences: Preferences{Locale: "zh-Hans", NextNumber: 1}}
	d.initRecords()
	return d
}
func ID() string { return rand.Text() }
func (d Data) Clone() Data {
	n := Empty()
	n.Preferences = d.Preferences
	if d.Preferences.ViewStates != nil {
		n.Preferences.ViewStates = map[string]ViewConfig{}
		for k, v := range d.Preferences.ViewStates {
			n.Preferences.ViewStates[k] = v.Clone()
		}
	}
	for k, v := range d.Views {
		v.Config = v.Config.Clone()
		n.Views[k] = v
	}
	for k, v := range d.Projects {
		v.Dependencies = slices.Clone(v.Dependencies)
		v.LabelIDs = slices.Clone(v.LabelIDs)
		n.Projects[k] = v
	}
	for k, v := range d.Issues {
		v.LabelIDs = slices.Clone(v.LabelIDs)
		v.Body = slices.Clone(v.Body)
		n.Issues[k] = v
	}
	for k, v := range d.Labels {
		n.Labels[k] = v
	}
	d.cloneRecords(&n)
	return n
}
func ValidDate(s string) bool {
	if s == "" {
		return true
	}
	_, e := time.Parse("2006-01-02", s)
	return e == nil
}
func validName(s string) bool {
	return strings.TrimSpace(s) != "" && utf8.ValidString(s) && utf8.RuneCountInString(s) <= 500
}
func (d Data) Validate() error {
	for id, v := range d.Views {
		if id == "" || id != v.ID || !validName(v.Name) {
			return errors.New("invalid_view")
		}
		if e := v.Config.Validate(); e != nil {
			return e
		}
		if v.ProjectID != "" {
			if _, ok := d.Projects[v.ProjectID]; !ok {
				return errors.New("invalid_project_reference")
			}
		}
	}
	for _, v := range d.Preferences.ViewStates {
		if e := v.Validate(); e != nil {
			return e
		}
	}
	if d.Projects == nil || d.Issues == nil || d.Labels == nil {
		return errors.New("invalid_data")
	}
	if d.Preferences.Locale != "en" && d.Preferences.Locale != "zh-Hans" {
		return errors.New("invalid_locale")
	}
	if d.Preferences.NextNumber < 1 {
		return errors.New("invalid_number")
	}
	for id, p := range d.Projects {
		if e := d.validateProjectLabels(p.LabelIDs); e != nil {
			return e
		}
		if id == "" || p.ID != id || !validName(p.Name) || !slices.Contains(ProjectStatuses, p.Status) || !ValidDate(p.Due) || !ValidDate(p.Start) || p.Priority < 0 || p.Priority > 4 || !validHealth(p.Health) || p.Start != "" && p.Due != "" && p.Start > p.Due {
			return errors.New("invalid_project")
		}
	}
	for id, l := range d.Labels {
		if id == "" || l.ID != id || !validName(l.Name) || !validLabelKind(l.Kind) || l.EntityKind() == "project" && l.ProjectID != "" {
			return errors.New("invalid_label")
		}
		if l.ProjectID != "" {
			if _, ok := d.Projects[l.ProjectID]; !ok {
				return errors.New("invalid_label_scope")
			}
		}
	}
	numbers := map[int]bool{}
	for id, i := range d.Issues {
		if id == "" || i.ID != id || !validName(i.Title) || i.Priority < 0 || i.Priority > 4 || !slices.Contains(Statuses, i.Status) || !ValidDate(i.Due) || !ValidDate(i.Start) || i.Start != "" && (i.Due == "" || i.Start >= i.Due) {
			return errors.New("invalid_issue")
		}
		if i.Number < 1 || numbers[i.Number] || i.Number >= d.Preferences.NextNumber {
			return errors.New("invalid_number")
		}
		numbers[i.Number] = true
		if len(i.Body) > 0 && !json.Valid(i.Body) {
			return errors.New("invalid_body")
		}
		if i.ProjectID != "" {
			p, ok := d.Projects[i.ProjectID]
			if !ok || (p.DeletedAt != 0 && i.DeletedAt == 0) {
				return errors.New("invalid_project_reference")
			}
		}
		if i.ParentID != "" {
			p, ok := d.Issues[i.ParentID]
			if !ok || p.ID == i.ID || p.ProjectID != i.ProjectID || (p.DeletedAt != 0 && i.DeletedAt == 0) {
				return errors.New("invalid_parent")
			}
		}
		for _, l := range i.LabelIDs {
			if label, ok := d.Labels[l]; !ok || label.EntityKind() != "issue" || label.ProjectID != "" && label.ProjectID != i.ProjectID {
				return errors.New("invalid_label_reference")
			}
		}
		if i.MilestoneID != "" {
			if m, ok := d.Milestones[i.MilestoneID]; !ok || m.ProjectID != i.ProjectID {
				return errors.New("invalid_milestone")
			}
		}
	}
	return d.validateRecords()
}
func (d *Data) AddIssue(title, project, parent, status string) (Issue, error) {
	if strings.TrimSpace(title) == "" {
		return Issue{}, errors.New("title_required")
	}
	if parent != "" {
		p, ok := d.Issues[parent]
		if !ok || p.DeletedAt != 0 {
			return Issue{}, errors.New("invalid_parent")
		}
		project = p.ProjectID
	}
	now := time.Now().UnixMilli()
	i := Issue{ID: ID(), Number: d.Preferences.NextNumber, Title: strings.TrimSpace(title), ProjectID: project, ParentID: parent, Status: status, Position: now, CreatedAt: now, UpdatedAt: now}
	d.Preferences.NextNumber++
	d.Issues[i.ID] = i
	return i, nil
}
func (d *Data) PutIssue(i Issue) error {
	old, ok := d.Issues[i.ID]
	if !ok {
		return errors.New("not_found")
	}
	if old.DeletedAt != 0 {
		return errors.New("issue_in_trash")
	}
	// Deletion and restoration must use their dedicated operations.
	i.DeletedAt = old.DeletedAt
	i.Number = old.Number
	i.CreatedAt = old.CreatedAt
	i.Title = strings.TrimSpace(i.Title)
	i.NormalizeDates()
	i.UpdatedAt = time.Now().UnixMilli()
	if i.ParentID != "" {
		p, ok := d.Issues[i.ParentID]
		if !ok || p.DeletedAt != 0 || p.ID == i.ID || d.IsDescendant(p.ID, i.ID) {
			return errors.New("invalid_parent")
		}
		i.ProjectID = p.ProjectID
	}
	d.Issues[i.ID] = i
	if old.ProjectID != i.ProjectID {
		d.cleanIssueScope(&i)
		d.Issues[i.ID] = i
		for _, id := range d.Descendants(i.ID) {
			ch := d.Issues[id]
			ch.ProjectID = i.ProjectID
			d.cleanIssueScope(&ch)
			d.Issues[id] = ch
		}
	}
	return nil
}

// A deletion operation needs a unique token even when the system clock has
// lower resolution than UnixNano. Only members of that operation restore together.
func (d Data) deletionStamp() int64 {
	stamp := time.Now().UnixNano()
	for _, i := range d.Issues {
		if i.DeletedAt >= stamp {
			stamp = i.DeletedAt + 1
		}
	}
	for _, p := range d.Projects {
		if p.DeletedAt >= stamp {
			stamp = p.DeletedAt + 1
		}
	}
	return stamp
}
func (d *Data) TrashIssue(id string) {
	i, ok := d.Issues[id]
	if !ok || i.DeletedAt != 0 {
		return
	}
	stamp := d.deletionStamp()
	i.DeletedAt = stamp
	d.Issues[id] = i
	for _, k := range d.Descendants(id) {
		ch := d.Issues[k]
		if ch.DeletedAt == 0 {
			ch.DeletedAt = stamp
			d.Issues[k] = ch
		}
	}
}
func (d *Data) RestoreIssue(id string) error {
	i, ok := d.Issues[id]
	if !ok {
		return errors.New("not_found")
	}
	stamp := i.DeletedAt
	if p := d.Projects[i.ProjectID]; p.DeletedAt != 0 {
		return errors.New("restore_project_first")
	}
	if p := d.Issues[i.ParentID]; p.DeletedAt != 0 {
		return errors.New("restore_parent_first")
	}
	i.DeletedAt = 0
	d.Issues[id] = i
	for _, k := range d.Descendants(id) {
		ch := d.Issues[k]
		if ch.DeletedAt == stamp {
			ch.DeletedAt = 0
			d.Issues[k] = ch
		}
	}
	return nil
}
func (d *Data) TrashProject(id string) {
	p, ok := d.Projects[id]
	if !ok || p.DeletedAt != 0 {
		return
	}
	stamp := d.deletionStamp()
	p.DeletedAt = stamp
	d.Projects[id] = p
	for k, i := range d.Issues {
		if i.ProjectID == id && i.DeletedAt == 0 {
			i.DeletedAt = stamp
			d.Issues[k] = i
		}
	}
}
func (d *Data) RestoreProject(id string) {
	p := d.Projects[id]
	stamp := p.DeletedAt
	p.DeletedAt = 0
	d.Projects[id] = p
	for k, i := range d.Issues {
		if i.ProjectID == id && i.DeletedAt == stamp {
			i.DeletedAt = 0
			d.Issues[k] = i
		}
	}
}
func (d *Data) Move(id, status, before string) error {
	i, ok := d.Issues[id]
	if !ok || i.DeletedAt != 0 {
		return errors.New("not_found")
	}
	i.Status = status
	d.Issues[id] = i
	items := d.Query(Filter{Status: status, IncludeArchived: true})
	ids := []string{}
	for _, v := range items {
		if v.ID != id {
			ids = append(ids, v.ID)
		}
	}
	idx := slices.Index(ids, before)
	if idx < 0 {
		idx = len(ids)
	}
	ids = slices.Insert(ids, idx, id)
	for n, k := range ids {
		v := d.Issues[k]
		v.Position = int64(n+1) * 1024
		d.Issues[k] = v
	}
	return nil
}

type Filter struct {
	Query, ProjectID, Status, LabelID              string
	Priority                                       int
	HasPriority, Favorites, Trash, IncludeArchived bool
}

func (d Data) Query(f Filter) []Issue {
	q := strings.ToLower(strings.TrimSpace(f.Query))
	out := make([]Issue, 0)
	for _, i := range d.Issues {
		if (i.DeletedAt != 0) != f.Trash || f.ProjectID != "" && i.ProjectID != f.ProjectID || f.Status != "" && i.Status != f.Status || f.HasPriority && i.Priority != f.Priority || f.Favorites && !i.Favorite || f.LabelID != "" && !slices.Contains(i.LabelIDs, f.LabelID) {
			continue
		}
		p := d.Projects[i.ProjectID]
		if !f.IncludeArchived && !f.Trash && (p.Archived || p.DeletedAt != 0) {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(i.Code()+" "+i.Title+" "+i.Plain), q) {
			continue
		}
		out = append(out, i)
	}
	slices.SortFunc(out, func(a, b Issue) int {
		sa, sb := slices.Index(Statuses, a.Status), slices.Index(Statuses, b.Status)
		if sa != sb {
			return sa - sb
		}
		if a.Position < b.Position {
			return -1
		}
		if a.Position > b.Position {
			return 1
		}
		return a.Number - b.Number
	})
	return out
}
func (d Data) ProjectList(archived, trash bool) []Project {
	out := []Project{}
	for _, p := range d.Projects {
		if (p.DeletedAt != 0) == trash && (trash || p.Archived == archived) {
			out = append(out, p)
		}
	}
	slices.SortFunc(out, func(a, b Project) int {
		if order := strings.Compare(a.Name, b.Name); order != 0 {
			return order
		}
		return strings.Compare(a.ID, b.ID)
	})
	return out
}
func (l Label) EntityKind() string {
	if l.Kind == "" {
		return "issue"
	}
	return l.Kind
}
func validLabelKind(kind string) bool { return kind == "" || kind == "issue" || kind == "project" }
func (d Data) LabelList() []Label     { return d.LabelsForKind("issue") }
func (d Data) LabelsForKind(kind string) []Label {
	out := []Label{}
	for _, l := range d.Labels {
		if l.EntityKind() == kind {
			out = append(out, l)
		}
	}
	slices.SortFunc(out, func(a, b Label) int {
		if order := strings.Compare(a.Group+"/"+a.Name, b.Group+"/"+b.Name); order != 0 {
			return order
		}
		return strings.Compare(a.ID, b.ID)
	})
	return out
}
func (d Data) validateProjectLabels(ids []string) error {
	seen := map[string]bool{}
	for _, id := range ids {
		l, ok := d.Labels[id]
		if !ok || l.EntityKind() != "project" || seen[id] {
			return errors.New("invalid_project_label")
		}
		seen[id] = true
	}
	return nil
}
func (d Data) Progress(project string) (done, total int) {
	for _, i := range d.Issues {
		if i.ProjectID == project && i.DeletedAt == 0 && i.Status != "canceled" && i.Status != "duplicate" {
			total++
			if i.Status == "done" {
				done++
			}
		}
	}
	return
}

func (d *Data) Seed() {
	d.Preferences.Initialized = true
	p := Project{ID: ID(), Name: "Flow 1.0", Description: "一个安静、专注的地方，让想法成为现实。\nA calm space to turn ideas into meaningful work.", Status: "active", Color: "#a799e8", Favorite: true, CreatedAt: time.Now().UnixMilli()}
	d.Projects[p.ID] = p
	p2 := Project{ID: ID(), Name: "Personal space", Description: "给生活里的重要事情留一点空间。", Status: "planned", Color: "#78b5a6", CreatedAt: time.Now().UnixMilli()}
	d.Projects[p2.ID] = p2
	for _, l := range []Label{{ID: "design", Name: "Design", Color: "#b19bea"}, {ID: "feature", Name: "Feature", Color: "#86b8ed"}, {ID: "bug", Name: "Bug", Color: "#df8699"}} {
		d.Labels[l.ID] = l
	}
	titles := []string{"整理第一版的核心体验", "设计清晰的项目导航", "让每一次编辑都安心保存", "打磨键盘操作与中文输入", "为任务加入标签与优先级", "完成列表与看板的切换", "让备份和恢复足够简单", "写下产品的第一份说明", "验证离线状态下的完整体验"}
	states := []string{"backlog", "backlog", "todo", "progress", "progress", "review", "todo", "done", "done"}
	for n, title := range titles {
		i, _ := d.AddIssue(title, p.ID, "", states[n])
		i.Priority = []int{0, 3, 2, 1, 3, 2, 2, 4, 0}[n]
		i.LabelIDs = []string{[]string{"design", "feature", "feature", "bug", "feature", "design", "feature", "design", "bug"}[n]}
		i.Plain = "把重要的事情拆成清晰、可以完成的下一步。\n\n完成后检查实际体验，并记录需要改进的细节。"
		i.Position = int64(n+1) * 1024
		d.Issues[i.ID] = i
	}
	d.AddIssue("留出一个下午，读完手边的书", p2.ID, "", "todo")
}
