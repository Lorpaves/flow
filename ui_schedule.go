package main

import (
	"context"
	"encoding/json"
	"errors"
	"flow/internal/core"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/choice"
	"github.com/lobehub/lobe-mygo/pkg/field"
)

func (a *app) editRecurrence(id string, form core.Issue) {
	a.flushDrafts()
	if v, ok := a.data.Recurrences[id]; ok {
		a.scheduleForm = v
	} else {
		if d := a.drafts[form.ID]; d != nil {
			form = d.issue
			form.Body, _ = json.Marshal(d.doc.Text())
			form.Plain = d.doc.Text().PlainText()
		}
		if form.Status == "done" || form.Status == "canceled" || form.Status == "duplicate" {
			form.Status = "todo"
		}
		a.scheduleForm = core.Recurrence{Form: form, Every: 1, Unit: "week", NextDue: time.Now().AddDate(0, 0, 7).Format("2006-01-02")}
	}
	a.scheduleEvery = strconv.Itoa(a.scheduleForm.Every)
	a.scheduleOpen = true
	a.scheduleError = ""
}
func (a *app) saveRecurrence() {
	form := a.scheduleForm
	n, e := strconv.Atoi(strings.TrimSpace(a.scheduleEvery))
	if e != nil || n < 1 || n > 365 {
		a.scheduleError = a.tr("间隔需为 1 到 365", "Interval must be 1 to 365")
		return
	}
	form.Every = n
	if old, ok := a.data.Recurrences[form.ID]; !ok || old.NextDue != form.NextDue {
		form.AnchorDay = 0
	}
	a.apply(func(d *core.Data) error { _, e := d.SaveRecurrence(form); return e }, func() { a.scheduleOpen = false; a.notice = a.tr("周期任务已保存", "Recurring issue saved") })
}
func (a *app) remindIssue(id string) {
	a.reminderIssue, a.reminderProject = id, ""
	a.reminderInput = "1h"
	a.scheduleError = ""
	a.reminderOpen = true
}
func (a *app) remindProject(id string) {
	a.reminderIssue, a.reminderProject = "", id
	a.reminderInput, a.scheduleError = "1h", ""
	a.reminderOpen = true
}
func (a *app) reminderTitle(r core.Reminder) string {
	if r.ProjectID != "" {
		return a.data.Projects[r.ProjectID].Name
	}
	i := a.data.Issues[r.IssueID]
	return i.Code() + "  " + i.Title
}
func (a *app) openReminder(r core.Reminder) {
	if r.ProjectID != "" {
		if p, ok := a.data.Projects[r.ProjectID]; ok {
			if p.DeletedAt != 0 {
				a.navigate("trash", "")
			} else {
				a.navigate("project", p.ID)
				a.projectTab = "overview"
			}
		}
	} else if i, ok := a.data.Issues[r.IssueID]; ok {
		if i.DeletedAt != 0 {
			a.navigate("trash", "")
			a.openIssue(i.ID)
		} else {
			a.openSearchIssue(i.ID, "")
		}
	}
}
func (a *app) saveReminder() {
	at, e := core.ParseReminder(strings.TrimSpace(a.reminderInput), time.Now())
	if e != nil {
		a.scheduleError = a.tr("请输入日期时间或相对时间，例如 1h、明天、2026-12-01 09:00", "Enter a date/time or relative time, such as 1h, tomorrow, or 2026-12-01 09:00")
		return
	}
	id, project := a.reminderIssue, a.reminderProject
	a.apply(func(d *core.Data) error {
		if project != "" {
			_, e := d.SetProjectReminder(project, at)
			return e
		}
		_, e := d.SetReminder(id, at)
		return e
	}, func() { a.reminderOpen = false; a.notice = a.tr("提醒已设置", "Reminder set") })
}
func (a *app) scheduleDialogs(c *ui.Context) {
	ui.DialogBase(c, &a.reminderOpen, func(backdrop, panel ui.Element) {
		a.dialogChrome(backdrop, panel)
		panel.Width(440).Padding(16).Gap(12)
		a.dialogTitle(c, a.tr("提醒我", "Remind me"), nil)
		field.Input(c.Key("reminder-when"), &a.reminderInput, field.InputProps{Placeholder: a.tr("1h、明天、2026-12-01 09:00", "1h, tomorrow, 2026-12-01 09:00"), Prefix: glyph("clock"), Invalid: a.scheduleError != ""}).AutoFocus().Label(a.tr("提醒时间", "Reminder time")).OnSubmit(a.saveReminder)
		ui.Row(c).Wrap().Gap(4).Children(func() {
			for _, v := range [][3]string{{"1h", "1 小时后", "In 1 hour"}, {"3h", "3 小时后", "In 3 hours"}, {"tomorrow", "明天", "Tomorrow"}, {"next week", "下周", "Next week"}} {
				quietButton(c, a.tr(v[1], v[2])).OnClick(func() { a.reminderInput = v[0] })
			}
			quietButton(c, a.tr("选日期…", "Choose date…")).OnClick(func() {
				a.chooseDate(a.tr("提醒日期", "Reminder date"), "", func(date string) {
					if date != "" {
						a.reminderInput = date + " 09:00"
					}
				})
			})
		})
		if a.scheduleError != "" {
			ui.Text(c, a.scheduleError).FontSize(12).TextColor(a.tk.Error)
		}
		a.dialogError(c)
		ui.Row(c).Gap(8).Margin(4, 0, 0, 0).Children(func() {
			// Worth knowing before relying on it, without a paragraph in the way.
			ui.Text(c, a.tr("仅在 Flow 运行时提醒", "Only while Flow is open")).FontSize(12).TextColor(a.tk.TextQuaternary).Grow(1)
			btn(c, a.tr("取消", "Cancel")).OnClick(func() { a.reminderOpen = false })
			primary(c, a.tr("提醒", "Set reminder")).Label(a.tr("设置提醒", "Set reminder")).Disabled(a.pending > 0).OnClick(a.saveReminder)
		})
	})
	ui.DialogBase(c, &a.scheduleOpen, func(backdrop, panel ui.Element) {
		a.dialogChrome(backdrop, panel)
		panel.Width(520).Padding(16).Gap(12)
		a.dialogTitle(c, a.tr("周期任务", "Recurring issue"), nil)
		field.Input(c.Key("recurrence-title"), &a.scheduleForm.Form.Title, field.InputProps{Placeholder: a.tr("标题", "Title")}).AutoFocus().Label(a.tr("周期任务标题", "Recurring issue title"))
		ui.Row(c).Gap(8).Children(func() {
			ui.Text(c, a.tr("每", "Every")).TextColor(a.tk.TextSecondary)
			ui.Column(c).Width(72).AlignItems(ui.Stretch).Children(func() {
				field.Input(c.Key("recurrence-every"), &a.scheduleEvery, field.InputProps{}).Label(a.tr("周期间隔", "Repeat interval"))
			})
			units := map[string][2]string{"day": {"天", "day(s)"}, "week": {"周", "week(s)"}, "month": {"月", "month(s)"}, "year": {"年", "year(s)"}}
			current := units[a.scheduleForm.Unit]
			a.chip(c, a.tr(current[0], current[1]), nil).Height(32).Label(a.tr("周期单位", "Repeat unit")).PopoverMenuButton(func(m *ui.Menu) {
				for _, unit := range []string{"day", "week", "month", "year"} {
					v := units[unit]
					if m.Item(a.tr(v[0], v[1])).Radio(a.scheduleForm.Unit == unit).Chosen() {
						a.scheduleForm.Unit = unit
					}
				}
			})
		})
		ui.Row(c).Wrap().Gap(6).Children(func() {
			a.dateButton(c, a.scheduleForm.NextDue, a.tr("下次截止日期", "Next due date"), func(v string) { a.scheduleForm.NextDue = v }, a.tr("下次截止", "Next due"))
			a.dateButton(c, a.scheduleForm.EndDate, a.tr("周期结束日期", "Recurrence end date"), func(v string) { a.scheduleForm.EndDate = v }, a.tr("结束日期", "Ends"))
			a.statusPicker(c, a.scheduleForm.Form.Status, func(v string) { a.scheduleForm.Form.Status = v })
			a.priorityPicker(c, a.scheduleForm.Form.Priority, func(v int) { a.scheduleForm.Form.Priority = v })
			a.projectPicker(c, a.scheduleForm.Form.ProjectID, func(v string) { a.scheduleForm.Form.ProjectID = v }).Disabled(a.scheduleForm.Form.ParentID != "")
		})
		choice.Checkbox(c, &a.scheduleForm.Paused, a.tr("暂停", "Paused"), choice.CheckboxProps{})
		if a.scheduleError != "" {
			ui.Text(c, a.scheduleError).FontSize(12).TextColor(a.tk.Error)
		}
		a.dialogError(c)
		ui.Row(c).Gap(8).Margin(4, 0, 0, 0).Children(func() {
			ui.Text(c, a.tr("Flow 关闭期间只补建最近一期", "While Flow is closed, only the latest occurrence is caught up")).FontSize(12).TextColor(a.tk.TextQuaternary).Grow(1).MinWidth(0)
			btn(c, a.tr("取消", "Cancel")).OnClick(func() { a.scheduleOpen = false })
			primary(c, a.tr("保存", "Save recurrence")).Label(a.tr("保存周期任务", "Save recurrence")).Disabled(a.pending > 0).OnClick(a.saveRecurrence)
		})
	})
}
func (a *app) remindersView(c *ui.Context) {
	ui.Scroll(c).Grow(1).MinHeight(0).Padding(24, 28).Gap(14).Children(func() {
		ui.Text(c, a.tr("提醒", "Reminders")).FontWeight(600)
		items := []core.Reminder{}
		for _, v := range a.data.Reminders {
			if v.DismissedAt == 0 {
				items = append(items, v)
			}
		}
		slices.SortFunc(items, func(x, y core.Reminder) int {
			if x.At < y.At {
				return -1
			}
			if x.At > y.At {
				return 1
			}
			return strings.Compare(x.ID, y.ID)
		})
		if len(items) == 0 {
			ui.Text(c, a.tr("暂无提醒。在任务菜单中选择“提醒我”。", "No reminders. Choose “Remind me” in an issue's menu.")).TextColor(a.theme.TextMuted)
		}
		for _, v := range items {
			title := a.reminderTitle(v)
			a.recordCard(ui.Row(c.Key(v.ID)), true).Padding(14).Gap(10).Children(func() {
				label := time.UnixMilli(v.At).Format("2006-01-02 15:04")
				if v.FiredAt != 0 {
					label += " · " + a.tr("已到期", "Due")
				}
				ui.Column(c).Grow(1).Gap(6).Children(func() {
					textButton(c, title).OnClick(func() { a.openReminder(v) })
					ui.Text(c, label).FontSize(12).TextColor(a.theme.TextMuted)
				})
				textButton(c, a.tr("延后 1 小时", "Snooze 1 hour")).Disabled(!a.data.ReminderActive(v)).OnClick(func() {
					a.apply(func(d *core.Data) error {
						_, e := d.SnoozeReminder(v.ID, time.Now().Add(time.Hour))
						return e
					}, nil)
				})
				dismissLabel := title
				if v.ProjectID == "" {
					dismissLabel = a.data.Issues[v.IssueID].Code()
				}
				iconButton(c, "close", a.tr("清除提醒：", "Dismiss reminder: ")+dismissLabel).OnClick(func() {
					a.apply(func(d *core.Data) error {
						r := d.Reminders[v.ID]
						r.DismissedAt = time.Now().UnixMilli()
						d.Reminders[v.ID] = r
						return nil
					}, nil)
				})
			})
		}
		a.divider(c)
		ui.Text(c, a.tr("周期任务", "Recurring issues")).FontWeight(600)
		plans := []core.Recurrence{}
		for _, v := range a.data.Recurrences {
			plans = append(plans, v)
		}
		slices.SortFunc(plans, func(x, y core.Recurrence) int { return strings.Compare(x.ID, y.ID) })
		if len(plans) == 0 {
			ui.Text(c, a.tr("在任务菜单中选择“转换为 → 周期任务”。", "Choose “Convert to → Recurring issue” in an issue's menu.")).TextColor(a.theme.TextMuted)
		}
		for _, v := range plans {
			a.recordCard(ui.Row(c.Key(v.ID)), true).Padding(14).Gap(10).Children(func() {
				ui.Column(c).Grow(1).Gap(6).Children(func() {
					ui.Text(c, v.Form.Title).Bold()
					label := v.NextDue
					if v.Paused {
						label += " · " + a.tr("已暂停", "Paused")
					}
					ui.Text(c, label).TextColor(a.theme.TextMuted)
					if v.LastError != "" {
						ui.Text(c, a.errorText(v.LastError)).TextColor(a.theme.Danger)
					}
				})
				textButton(c, a.tr("编辑", "Edit")).Label(a.tr("编辑周期任务：", "Edit recurrence: ") + v.Form.Title).OnClick(func() { a.editRecurrence(v.ID, v.Form) })
				iconButton(c, "trash", a.tr("删除周期任务：", "Delete recurrence: ")+v.Form.Title).OnClick(func() { a.apply(func(d *core.Data) error { delete(d.Recurrences, v.ID); return nil }, nil) })
			})
		}
	})
}
func scheduledDue(d core.Data, now time.Time) bool {
	today := now.Format("2006-01-02")
	for _, v := range d.Recurrences {
		if !v.Paused && v.NextDue <= today {
			return true
		}
	}
	for _, v := range d.Reminders {
		if v.DismissedAt == 0 && v.FiredAt == 0 && v.At <= now.UnixMilli() && d.ReminderActive(v) {
			return true
		}
	}
	return false
}
func runStoreSchedule(s *core.Store, now time.Time) (core.Data, []string, error) {
	data := s.Snapshot()
	if !scheduledDue(data, now) {
		return data, nil, nil
	}
	var fired []string
	data, e := s.UpdateFrom("scheduler", func(d *core.Data) error { _, fired = d.RunScheduled(now); return nil })
	return data, fired, e
}
func (a *app) deliverReminders(ids []string) {
	if len(ids) == 0 {
		return
	}
	a.notice = a.tr("有提醒到期", "Reminders are due.")
	if a.window == nil {
		return
	}
	w := a.window
	for _, id := range ids {
		reminder := a.data.Reminders[id]
		body := a.tr("任务提醒", "Issue reminder")
		if reminder.ProjectID != "" {
			body = a.tr("项目提醒", "Project reminder")
		}
		opts := mygo.NotificationOptions{ID: "flow-reminder:" + id, Title: a.reminderTitle(reminder), Body: body, Group: "flow-reminders"}
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			e := mygo.NewNotification(opts).ShowContext(ctx)
			if e != nil {
				w.Update(func() {
					if errors.Is(e, mygo.ErrNotificationsDenied) {
						a.notice = a.tr("系统通知未获允许；提醒记录已保留。", "System notifications are disabled; reminder records are preserved.")
					} else {
						a.notice = a.tr("系统通知未送达；提醒记录已保留。", "System notification delivery failed; reminder records are preserved.")
					}
				})
			}
		}()
	}
}
