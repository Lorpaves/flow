package main

import (
	"encoding/json"
	"flow/internal/core"
	"fmt"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
	"github.com/egoist/mygo/ui/richtext"
	"github.com/lobehub/lobe-mygo/pkg/theme"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

type job struct {
	run  func(*core.Store) (core.Data, error)
	done func(core.Data, error)
}
type draft struct {
	titleEditor                ui.Handle
	wantTitle, wantDescription bool
	recoveryID                 string
	editing                    bool
	editor                     ui.Handle
	issue                      core.Issue
	base                       core.Issue
	doc                        *richtext.Document
	bodySaved                  string
	dirty, busy                bool
	revision                   int
	changed                    time.Time
}
type app struct {
	desktop                                     *desktop
	windowID                                    string
	windowVisible                               bool
	mediaMarkdown                               *markdown.Options
	mediaMarkdownKey                            string
	mediaImages                                 map[string]*mediaImage
	paletteScope, paletteTarget                 string
	paletteLimit                                int
	paletteList                                 ui.ListState
	helpOpen                                    bool
	helpQuery                                   string
	keySequence                                 string
	keySequenceUntil                            time.Time
	peekOpen, peekRendered                      bool
	peekID                                      string
	childOpen                                   bool
	childParent, childQuery                     string
	desktopLoaded, desktopDisabled, desktopBusy bool
	desktopSaved, desktopObserved               string
	desktopMenuKey                              string
	desktopChanged                              time.Time
	labelOpen                                   bool
	labelForm, labelBase                        core.Label
	labelQuery                                  string
	projectLabels                               *pickerState
	folderOpen                                  bool
	folderForm, folderBase                      core.FavoriteFolder
	folderQuery                                 string
	folderCollapsed                             map[string]bool
	sidebarWidth                                float32
	projectBase                                 core.Project
	milestoneOpen, documentOpen, updateOpen     bool
	milestoneForm, milestoneBase                core.Milestone
	documentForm, documentBase                  core.Document
	updateForm, updateBase                      core.ProjectUpdate
	documentEditor, updateEditor                ui.Handle
	documentText, updateText                    *richtext.Document
	projectResourceQuery                        string
	timelineStart                               string
	projectsTimeline                            bool
	timelineScale, timelineFocus                string
	timelineZoom                                float32
	timelineSelected                            map[string]bool
	timelineRows                                map[string]*ui.Handle
	timelineScroll                              ui.ScrollState
	timelineLinkFrom, timelineLinkTo            string
	timelineList                                ui.ListState
	timelineAdjust                              timelineAdjust

	newEditor, templateEditor ui.Handle

	scheduleOpen, reminderOpen                                                  bool
	scheduleForm                                                                core.Recurrence
	scheduleEvery, scheduleError, reminderIssue, reminderInput, reminderProject string

	attachmentOpen                                                        bool
	attachmentOwnerKind, attachmentOwnerID, attachmentName, attachmentURL string

	relationOpen                                bool
	relationIssue, relationKind, relationTarget string
	historyOpen                                 bool
	historyRecord                               core.Activity
	historyLimits                               map[string]int
	commentForms                                map[string]*commentForm
	commentTargets                              map[string]string

	services                              ui.Services
	pendingLink                           string
	data                                  core.Data
	window                                *mygo.Window
	jobs                                  chan job
	ready                                 bool
	pending                               int
	err, notice, dataDir                  string
	quitRequested, closeRequested         bool
	closeAction                           func(quit bool)
	appearance                            string
	restoring                             bool
	page, projectID, issueID, projectTab  string
	board                                 bool
	query, statusFilter, labelFilter      string
	priorityFilter                        int
	search                                ui.Handle
	list                                  ui.ListState
	boardLists                            [7]ui.ListState
	selected                              map[string]bool
	collapsed                             map[string]bool
	drafts                                map[string]*draft
	newOpen, projectOpen, paletteOpen     bool
	newIssue                              core.Issue
	newRelation                           *core.CreationRelation
	newBody                               string
	projectForm                           core.Project
	formError, paletteQuery, labelName    string
	theme                                 *ui.Theme
	cacheKey                              string
	cacheItems                            []core.Issue
	dataRevision                          int
	activePicker                          *pickerState
	propertyPickers                       map[ui.Key]*pickerState
	focusedIssue                          string
	revealIssue                           bool
	issueOrder                            []string
	returnIssues                          []string
	paletteIndex                          int
	newDoc                                *richtext.Document
	newTitle                              ui.Handle
	createMore, newDueOpen                bool
	composeKey                            string
	composeSubmitting                     string
	compositions                          map[string]composition
	createdIssue                          string
	navigation                            navigation
	date                                  dateDialog
	viewConfig, viewDraft                 core.ViewConfig
	displayOpen, filterOpen, saveViewOpen bool
	savedViewID, savedViewName            string
	boardStates                           map[string]*ui.ListState
	composeID, composeObserved            string
	composeSaved                          map[string]string
	composeBusy                           map[string]bool
	composeQueued                         map[string]core.IssueDraft
	composeChanged                        time.Time
	composeExpanded                       bool
	templateOpen                          bool
	templateID, templateName              string
	templateForm                          core.Issue
	templateDoc                           *richtext.Document

	// Pointer-driven list focus and the transient notice; see trackFocus.
	hoverHold, hoverHeld    bool
	hoverX, hoverY          float32
	focusSeen, selectAnchor string
	keptFocus               string // the focus last chosen by a key, click or command
	childStats              map[string][2]int
	childStatsRevision      int
	motion                  bool // forces panel transitions where no window exists
	tk                      *theme.Tokens
	seamless                *ui.Theme             // the theme without borders, for the sidebar's divider
	marquee                 *marquee              // the box being dragged over the list, if any
	marqueeRows             map[string]*ui.Handle // each list row's box
	marqueeControls         map[string]*ui.Handle // what a press in the list acts on
	eraseOpen               bool
	erasing                 eraseRequest // what the open erase dialog would delete for good
	appUpdateOpen           bool
	mcpOpen                 bool
	segments                map[string]*boundIndex
	sectionOpen             map[string]bool
}

type composition struct {
	id       string
	issue    core.Issue
	doc      *richtext.Document
	relation *core.CreationRelation
}

func newApp(d core.Data) *app {
	tk, t := flowTheme(true)
	a := &app{tk: tk, viewConfig: core.DefaultView(), boardStates: map[string]*ui.ListState{}, data: d, ready: true, jobs: make(chan job, 256), page: "issues", projectTab: "issues", priorityFilter: -1, selected: map[string]bool{}, collapsed: map[string]bool{}, drafts: map[string]*draft{}, theme: t}
	a.loadViewConfig()
	a.sidebarWidth = 224
	return a
}
func (a *app) tr(zh, en string) string {
	if a.data.Preferences.Locale == "en" {
		return en
	}
	return zh
}
func (a *app) acceptExternal(data core.Data) {
	a.data = data
	a.dataRevision++
	a.refreshCleanDrafts(data)
	if a.window != nil {
		mygo.App.SetLocale(data.Preferences.Locale)
	}
}
func (a *app) refreshCleanDrafts(data core.Data) {
	for id, d := range a.drafts {
		if d.dirty || d.busy {
			continue
		}
		i, exists := data.Issues[id]
		if !exists || i.DeletedAt != 0 && d.base.DeletedAt == 0 {
			delete(a.drafts, id)
			if a.issueID == id {
				a.issueID = ""
			}
			continue
		}
		if !slices.Equal(i.Body, d.base.Body) {
			d.doc = a.mediaDocument(i)
		}
		d.bodySaved = documentJSON(d.doc)
		d.issue, d.base = i, i
	}
}

func (a *app) status(s string) string {
	zh := map[string]string{"backlog": "待整理", "todo": "待办", "progress": "进行中", "review": "审核中", "done": "已完成", "canceled": "已取消", "duplicate": "重复"}
	en := map[string]string{"backlog": "Backlog", "todo": "Todo", "progress": "In Progress", "review": "In Review", "done": "Done", "canceled": "Canceled", "duplicate": "Duplicate"}
	return a.tr(zh[s], en[s])
}
func (a *app) priority(p int) string {
	zh := []string{"无优先级", "紧急", "高", "中", "低"}
	en := []string{"No priority", "Urgent", "High", "Medium", "Low"}
	if p < 0 || p > 4 {
		p = 0
	}
	return a.tr(zh[p], en[p])
}
func (a *app) projectStatus(s string) string {
	zh := map[string]string{"planned": "计划中", "active": "进行中", "completed": "已完成", "canceled": "已取消"}
	en := map[string]string{"planned": "Planned", "active": "In progress", "completed": "Completed", "canceled": "Canceled"}
	return a.tr(zh[s], en[s])
}
func (a *app) errorText(err string) string {
	if text, ok := a.recordErrorText(err); ok {
		return text
	}
	m := map[string][2]string{"issue_in_trash": {"请先恢复任务再编辑", "Restore the issue before editing"}, "move_parent_with_child": {"请同时选择父任务，或先解除父子关系", "Select the parent too, or remove the parent relationship first"}, "title_required": {"请输入标题", "Please enter a title"}, "invalid_issue": {"请检查标题、日期和任务属性", "Check the title, date and issue properties"}, "invalid_project": {"请检查项目名称和日期", "Check the project name and date"}, "invalid_parent": {"父子任务必须属于同一项目，不能形成循环", "Parent and child must share a project without a cycle"}, "restore_project_first": {"请先恢复所属项目", "Restore the project first"}, "restore_parent_first": {"请先恢复父任务", "Restore the parent issue first"}, "invalid_backup": {"这不是有效的 Flow 备份", "This is not a valid Flow backup"}, "backup_too_large": {"备份文件超过 512 MB", "The backup exceeds 512 MB"}}
	for k, v := range m {
		if strings.HasPrefix(err, k) {
			return a.tr(v[0], v[1])
		}
	}
	return a.tr("操作失败，内容已保留：", "The operation failed. Your edits are kept: ") + err
}
func (a *app) apply(fn func(*core.Data) error, after func()) {
	a.applyWithFailure(fn, after, nil)
}
func (a *app) applyWithFailure(fn func(*core.Data) error, after func(), failed func()) {
	a.pending++
	a.err = ""
	done := func(data core.Data, err error) {
		a.pending--
		if err != nil {
			a.err = err.Error()
			a.cancelClosing()
			if failed != nil {
				failed()
			}
			return
		}
		a.data = data
		a.dataRevision++
		a.refreshCleanDrafts(data)
		if after != nil {
			after()
		}
		a.finishQuit()
	}
	if a.window == nil {
		next := a.data.Clone()
		err := fn(&next)
		if err == nil {
			next.RecordChanges(a.data, "desktop", time.Now())
			err = next.Validate()
		}
		done(next, err)
		return
	}
	a.jobs <- job{run: func(s *core.Store) (core.Data, error) { return s.Update(fn) }, done: done}
}
func (a *app) navigate(page, project string) {
	a.rememberNavigation()
	a.flushDrafts()
	a.closePicker()
	a.focusedIssue, a.keptFocus, a.createdIssue = "", "", ""
	a.issueOrder, a.returnIssues = nil, nil
	a.page = page
	a.projectID = project
	a.issueID = ""
	a.projectTab = "issues"
	a.query = ""
	a.statusFilter = ""
	a.labelFilter = ""
	a.priorityFilter = -1
	a.selected = map[string]bool{}
	a.list = ui.ListState{}
	a.cacheKey = ""
	a.savedViewID = ""
	a.loadViewConfig()
}
func (a *app) openIssue(id string) {
	a.peekOpen = false
	a.flushDrafts()
	a.closePicker()
	i, ok := a.data.Issues[id]
	if !ok {
		return
	}
	if a.issueID != id {
		a.rememberNavigation()
	}
	if a.issueID == "" {
		a.issueOrder = a.visibleIssueIDs()
		a.returnIssues = nil
	} else if a.issueID != id && !slices.Contains(a.issueOrder, id) {
		a.returnIssues = append(a.returnIssues, a.issueID)
	}
	a.issueID, a.focusedIssue, a.keptFocus = id, id, id
	a.ensureIssueDraft(i)
}
func (a *app) ensureIssueDraft(i core.Issue) {
	id := i.ID
	if a.drafts[id] == nil || (!a.drafts[id].dirty && !a.drafts[id].busy) {
		doc := a.mediaDocument(i)
		a.drafts[id] = &draft{issue: i, base: i, doc: doc, bodySaved: documentJSON(doc)}
	}
}

func documentJSON(doc *richtext.Document) string {
	raw, _ := json.Marshal(doc.Text())
	return string(raw)
}
func (a *app) touch(d *draft, now time.Time) { d.dirty = true; d.revision++; d.changed = now }
func (a *app) hasDirty() bool {
	if a.desktopDirty() {
		return true
	}
	if a.compositionDirty() || len(a.composeQueued) > 0 {
		return true
	}
	for _, d := range a.drafts {
		if d.dirty || d.busy {
			return true
		}
	}
	return false
}

// Apply only fields edited in this draft. A queued save must not undo a newer
// list action, reorder, project move, or trash operation.
func mergeDraft(base, edited, current core.Issue) core.Issue {
	return core.MergeIssueEdits(base, edited, current)
}
func (a *app) saveDraft(id string) {
	d := a.drafts[id]
	if d == nil || !d.dirty || d.busy {
		return
	}
	i := d.issue
	i.LabelIDs = slices.Clone(i.LabelIDs)
	base := d.base
	if d.recoveryID == "" {
		d.recoveryID = core.ID()
	}
	recoveryID := d.recoveryID
	recovered := false
	body := documentJSON(d.doc)
	if body != d.bodySaved {
		i.Body = json.RawMessage(body)
		i.Plain = d.doc.Text().PlainText()
	} else {
		// Loading rich text normalizes JSON and hydrates media dimensions.
		// A title/property-only save must not claim that as a body edit.
		i.Body, i.Plain = base.Body, base.Plain
	}
	rev := d.revision
	d.busy = true
	d.dirty = false
	a.pending++
	done := func(data core.Data, err error) {
		a.pending--
		d.busy = false
		if err != nil {
			d.dirty = true
			a.err = err.Error()
			a.cancelClosing()
			return
		}
		a.data = data
		a.dataRevision++
		if recovered {
			a.notice = a.tr("修改已保存在草稿中，可稍后恢复", "Edits saved in Drafts for recovery")
			if a.closing() && d.dirty {
				a.saveDraft(id)
			}
			a.finishQuit()
			return
		}
		saved := data.Issues[id]
		if d.revision == rev {
			d.issue = saved
		} else {
			edited := d.issue
			edited.Body, edited.Plain = i.Body, i.Plain
			d.issue = mergeDraft(i, edited, saved)
		}
		savedDoc := a.mediaDocument(saved)
		canonical := documentJSON(savedDoc)
		if documentJSON(d.doc) == body && body != canonical {
			d.doc = savedDoc
		}
		d.bodySaved = canonical
		d.base = saved
		if a.closing() && d.dirty {
			a.saveDraft(id)
		}
		a.finishQuit()
	}
	fn := func(data *core.Data) error {
		candidate := data.Clone()
		err := candidate.PutIssue(mergeDraft(base, i, candidate.Issues[id]))
		if err == nil {
			err = candidate.Validate()
		}
		if err != nil {
			_, saveErr := data.SaveDraft(core.IssueDraft{ID: recoveryID, IssueID: id, Form: i, Base: base})
			recovered = saveErr == nil
			return saveErr
		}
		*data = candidate
		data.AdoptDraftAttachments(recoveryID, id)
		data.RemoveDraft(recoveryID)
		return nil
	}
	if a.window == nil {
		next := a.data.Clone()
		err := fn(&next)
		if err == nil {
			next.RecordChanges(a.data, "desktop", time.Now())
			err = next.Validate()
		}
		done(next, err)
		return
	}
	a.jobs <- job{run: func(s *core.Store) (core.Data, error) { return s.Update(fn) }, done: done}
}
func (a *app) flushDrafts() {
	a.persistDesktopSession()
	a.persistComposition()
	for _, v := range a.composeQueued {
		a.queueComposition(v)
	}
	for id := range a.drafts {
		a.saveDraft(id)
	}
}
func (a *app) editProject(id string) {
	a.formError = ""
	a.projectForm = a.data.Projects[id]
	if id == "" {
		a.projectForm = core.Project{ID: core.ID(), Status: "planned", Color: "#a799e8", CreatedAt: time.Now().UnixMilli()}
	}
	a.projectBase = a.projectForm
	a.projectForm.Dependencies = slices.Clone(a.projectForm.Dependencies)
	a.projectForm.LabelIDs = slices.Clone(a.projectForm.LabelIDs)
	a.projectOpen = true
}
func (a *app) saveProject() {
	p := a.projectForm
	p.Name = strings.TrimSpace(p.Name)
	if p.Name == "" || !core.ValidDate(p.Due) {
		a.formError = "invalid_project"
		return
	}
	base := a.projectBase
	a.apply(func(d *core.Data) error {
		if current, ok := d.Projects[p.ID]; ok {
			return d.PutProject(core.MergeProjectEdits(base, p, current))
		}
		return d.PutProject(p)
	}, func() { a.projectOpen = false; a.navigate("project", p.ID) })
}
func (a *app) setStatus(id, status string) {
	a.apply(func(d *core.Data) error { i := d.Issues[id]; i.Status = status; return d.PutIssue(i) }, nil)
}
func (a *app) moveIssue(id, status, before string) {
	a.apply(func(d *core.Data) error { return d.Move(id, status, before) }, nil)
}
func (a *app) trashIssue(id string) {
	a.flushDrafts()
	a.apply(func(d *core.Data) error { d.TrashIssue(id); return nil }, func() {
		if a.issueID == id {
			a.issueID = ""
		}
		delete(a.drafts, id)
	})
}
func (a *app) filtered() []core.Issue {
	configJSON, _ := json.Marshal(a.viewConfig)
	key := fmt.Sprintf("%d|%s|%s|%s|%s|%s|%d|%s", a.dataRevision, a.page, a.projectID, a.query, a.statusFilter, a.labelFilter, a.priorityFilter, configJSON)
	if key != a.cacheKey {
		a.cacheKey = key
		a.cacheItems = a.data.QueryView(core.Filter{Query: a.query, ProjectID: a.projectID, Status: a.statusFilter, LabelID: a.labelFilter, HasPriority: a.priorityFilter >= 0, Priority: a.priorityFilter, Favorites: a.page == "favorites", Trash: a.page == "trash", IncludeArchived: a.page == "project" || a.page == "archive"}, a.viewConfig, time.Now())
	}
	visible := make(map[string]bool, len(a.cacheItems))
	for _, i := range a.cacheItems {
		visible[i.ID] = true
	}
	for id := range a.selected {
		if !visible[id] {
			delete(a.selected, id)
		}
	}
	return a.cacheItems
}
func (a *app) backup() {
	if a.window == nil {
		return
	}
	a.flushDrafts()
	w := a.window
	title := a.tr("导出备份", "Export backup")
	go func() {
		path, e := mygo.Dialog.Save(mygo.SaveDialogOptions{Parent: w, Title: title, DefaultPath: "Flow-" + time.Now().Format("2006-01-02") + ".flow.json", Filters: []mygo.FileFilter{{Name: "Flow backup", Extensions: []string{"json"}}}})
		if path == "" && e == nil {
			return
		}
		w.Update(func() {
			if e != nil {
				a.err = e.Error()
				return
			}
			a.pending++
			a.jobs <- job{run: func(s *core.Store) (core.Data, error) { e := s.Backup(path); return s.Snapshot(), e }, done: func(d core.Data, e error) {
				a.pending--
				if e != nil {
					a.err = e.Error()
				} else {
					a.notice = a.tr("备份已保存：", "Backup saved: ") + path
				}
				a.finishQuit()
			}}
		})
	}()
}
func (a *app) restore() {
	if a.window == nil || a.workspaceBusy() || a.isRestoring() {
		return
	}
	w := a.window
	title := a.tr("恢复备份", "Restore backup")
	message := a.tr("用备份替换当前工作区？", "Replace this workspace with the backup?")
	detail := a.tr("当前数据将先自动备份到 backups 文件夹。", "Current data will be backed up to the backups folder first.")
	cancel, restore := a.tr("取消", "Cancel"), a.tr("恢复", "Restore")
	go func() {
		paths, e := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: w, Title: title, Filters: []mygo.FileFilter{{Name: "Flow backup", Extensions: []string{"json"}}}})
		if e != nil {
			w.Update(func() { a.err = e.Error() })
			return
		}
		if len(paths) == 0 {
			return
		}
		result, e := mygo.Dialog.Message(mygo.MessageOptions{Parent: w, Type: mygo.MessageQuestion, Message: message, Detail: detail, Buttons: []string{cancel, restore}, CancelButton: 0, DefaultButton: 0})
		if e != nil || result.Button != 1 {
			return
		}
		w.Update(func() {
			if a.workspaceBusy() || a.isRestoring() {
				return
			}
			a.restoring = true
			if a.desktop != nil {
				a.desktop.restoring = true
				a.desktop.invalidate()
			}
			a.pending++
			a.jobs <- job{run: func(s *core.Store) (core.Data, error) { return s.Restore(paths[0]) }, done: func(d core.Data, e error) {
				a.pending--
				a.restoring = false
				if a.desktop != nil {
					a.desktop.restoring = false
					a.desktop.invalidate()
				}
				if e != nil {
					a.err = e.Error()
					a.cancelClosing()
					return
				}
				a.data = d
				a.dataRevision++
				a.resetTransientForms()
				a.navigate("issues", "")
				if a.desktop != nil {
					for _, other := range a.desktop.apps {
						if other != a {
							other.acceptExternal(d.Clone())
							other.resetTransientForms()
							other.navigate("issues", "")
						}
					}
				}
				mygo.App.SetLocale(d.Preferences.Locale)
				a.notice = a.tr("备份已恢复", "Backup restored")
				a.finishQuit()
			}}
		})
	}()
}
func (a *app) backupDirectory() string { return filepath.Join(a.dataDir, "backups") }

// A restored workspace must not receive queued form snapshots from the old one.
func (a *app) resetTransientForms() {
	a.paletteOpen, a.helpOpen = false, false
	a.keySequence, a.paletteTarget = "", ""
	a.labelOpen, a.folderOpen = false, false
	a.drafts = map[string]*draft{}
	a.newOpen = false
	a.newDoc = nil
	a.composeID = ""
	a.composeKey = ""
	a.composeSubmitting = ""
	a.compositions = nil
	a.composeSaved = nil
	a.composeBusy = nil
	a.composeQueued = nil
	a.templateOpen = false
	a.relationOpen = false
	a.historyOpen = false
	a.attachmentOpen = false
	a.scheduleOpen = false
	a.reminderOpen = false
	a.commentForms = nil
	a.historyLimits = nil
	a.milestoneOpen = false
	a.documentOpen = false
	a.updateOpen = false
}
