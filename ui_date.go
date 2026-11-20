package main

import (
	"flow/internal/core"
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/field"
	"github.com/lobehub/lobe-mygo/pkg/lucide"
)

type dateDialog struct {
	open                bool
	title, input, error string
	left, right         time.Time
	focused             time.Time
	dayHandle           ui.Handle
	save                func(string)
	// A range dialog edits start and input (the end) together; after a first
	// click on the calendar the next one picks the end.
	ranged, picking bool
	start           string
	saveRange       func(start, end string)
}

func (a *app) chooseDate(title, current string, save func(string)) {
	a.closePicker()
	now := time.Now()
	if date, err := time.ParseInLocation("2006-01-02", current, time.Local); err == nil {
		now = date
	}
	left := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	a.date = dateDialog{open: true, title: title, input: current, left: left, right: left.AddDate(0, 1, 0), focused: now, save: save}
}

// chooseRange opens the date dialog for the days something spans. A single day
// comes back as an empty start.
func (a *app) chooseRange(title, start, end string, save func(start, end string)) {
	first := start
	if first == "" {
		first = end
	}
	a.chooseDate(title, first, nil)
	a.date.input, a.date.start, a.date.ranged, a.date.saveRange = end, start, true, save
}
func (a *app) rangeButton(c *ui.Context, start, end, label string, save func(start, end string), empty ...string) ui.Element {
	text := a.shortRange(start, end)
	if text == "" {
		text = label
		if len(empty) > 0 {
			text = empty[0]
		}
	}
	return a.chip(c, text, func() { ui.Icon(c, glyph("calendar")).FontSize(14).TextColor(a.tk.TextTertiary) }).Label(label).OnClick(func() { a.chooseRange(label, start, end, save) })
}

// shortRange writes the days from start to end as briefly as they read:
// "11月15–18日", "Nov 30 – Dec 2".
func (a *app) shortRange(start, end string) string {
	if start == "" || start == end {
		return a.shortDate(end)
	}
	if end == "" {
		return a.shortDate(start) + " –"
	}
	from, e1 := time.ParseInLocation("2006-01-02", start, time.Local)
	to, e2 := time.ParseInLocation("2006-01-02", end, time.Local)
	if e1 != nil || e2 != nil {
		return start + " – " + end
	}
	if from.Year() == to.Year() && from.Month() == to.Month() {
		if a.data.Preferences.Locale != "en" {
			return fmt.Sprintf("%s–%d日", strings.TrimSuffix(a.shortDay(from), "日"), to.Day())
		}
		text := fmt.Sprintf("%s–%d", from.Format("Jan 2"), to.Day())
		if from.Year() != time.Now().Year() {
			text += from.Format(", 2006")
		}
		return text
	}
	if a.data.Preferences.Locale != "en" {
		return a.shortDay(from) + "–" + a.shortDay(to)
	}
	return a.shortDay(from) + " – " + a.shortDay(to)
}
func (a *app) dateButton(c *ui.Context, current, label string, save func(string), empty ...string) ui.Element {
	text := a.shortDate(current)
	if current == "" {
		text = label
		if len(empty) > 0 {
			text = empty[0]
		}
	}
	return a.chip(c, text, func() { ui.Icon(c, glyph("calendar")).FontSize(14).TextColor(a.tk.TextTertiary) }).Label(label).OnClick(func() { a.chooseDate(label, current, save) })
}

// dueChip is the due date of a list row, colored when it is close or past.
func (a *app) dueChip(c *ui.Context, due, status, label string, save func(string)) ui.Element {
	b := ui.ButtonBase(c).Label(label).Shrink(0).Height(24).Padding(0, 8).Gap(4).Radius(999).TextColor(a.dueColor(due, status)).Cursor(ui.CursorPointer)
	if b.Hovered() {
		b.Background(a.tk.FillTertiary)
	}
	b.Children(func() {
		ui.Icon(c, glyph("calendar")).FontSize(12)
		ui.Text(c, a.shortDate(due)).FontSize(12).SingleLine()
	})
	return b.OnClick(func() { a.chooseDate(label, due, save) })
}
func (a *app) saveDate() {
	if a.date.ranged {
		a.saveDateRange()
		return
	}
	date, err := core.ParseDate(a.date.input, time.Now())
	if err != nil {
		a.date.error = a.tr("请输入日期或相对时间，如 7 days、明天。", "Enter a date or relative time, such as 7 days or tomorrow.")
		return
	}
	fn := a.date.save
	a.date.open = false
	if fn != nil {
		fn(date)
	}
}
func (a *app) saveDateRange() {
	now := time.Now()
	start, e1 := core.ParseDate(a.date.start, now)
	end, e2 := core.ParseDate(a.date.input, now)
	if e1 != nil || e2 != nil {
		a.date.error = a.tr("请输入日期或相对时间，如 7 days、明天。", "Enter a date or relative time, such as 7 days or tomorrow.")
		return
	}
	if start != "" && end != "" && start > end {
		start, end = end, start
	}
	if start == end {
		start = ""
	}
	fn := a.date.saveRange
	a.date.open = false
	if fn != nil {
		fn(start, end)
	}
}

// pickDay is a click on the calendar: one day, or the end of a range.
func (a *app) pickDay(date time.Time) {
	iso := date.Format("2006-01-02")
	a.date.focused = date
	a.date.error = ""
	if !a.date.ranged {
		a.date.input = iso
		return
	}
	start, _ := core.ParseDate(a.date.start, time.Now())
	if a.date.picking && iso >= start {
		a.date.input, a.date.picking = iso, false
		return
	}
	a.date.start, a.date.input, a.date.picking = iso, iso, true
}

// dateQuick are the shortcuts above the calendar; a range dialog also offers
// whole weeks.
func (a *app) dateQuick(c *ui.Context) {
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	iso := func(d time.Time) string { return d.Format("2006-01-02") }
	if !a.date.ranged {
		for _, quick := range [][2]string{{a.tr("今天", "Today"), "today"}, {a.tr("明天", "Tomorrow"), "tomorrow"}, {a.tr("本周末", "End of this week"), "end of this week"}, {a.tr("一周后", "In one week"), "in one week"}} {
			quietButton(c, quick[0]).OnClick(func() { a.date.input, _ = core.ParseDate(quick[1], now) })
		}
		return
	}
	sunday := today.AddDate(0, 0, (7-int(today.Weekday()))%7)
	for _, quick := range []struct {
		name       string
		start, end time.Time
	}{
		{a.tr("今天", "Today"), today, today},
		{a.tr("明天", "Tomorrow"), today.AddDate(0, 0, 1), today.AddDate(0, 0, 1)},
		{a.tr("本周", "This week"), today, sunday},
		{a.tr("下周", "Next week"), sunday.AddDate(0, 0, 1), sunday.AddDate(0, 0, 7)},
	} {
		quietButton(c, quick.name).OnClick(func() {
			a.date.start, a.date.input, a.date.picking, a.date.error = iso(quick.start), iso(quick.end), false, ""
			a.date.left = time.Date(quick.start.Year(), quick.start.Month(), 1, 0, 0, 0, 0, time.Local)
		})
	}
}
func (a *app) datePickerDialog(c *ui.Context) {
	ui.DialogBase(c, &a.date.open, func(backdrop, panel ui.Element) {
		a.dialogChrome(backdrop, panel)
		panel.Width(640).Padding(16).Gap(12)
		a.dialogTitle(c, a.date.title, func() { a.date.open = false })
		if a.date.ranged {
			ui.Row(c).FillWidth().Gap(8).Children(func() {
				changed := func() { a.date.error, a.date.picking = "", false }
				// Input returns the text field inside its frame; the column sizes the frame.
				ui.Column(c).Basis(0).Grow(1).Children(func() {
					field.Input(c.Key("range-start"), &a.date.start, field.InputProps{Placeholder: a.tr("开始", "Start"), Prefix: glyph("calendar"), Invalid: a.date.error != ""}).Label(a.tr("开始日期", "Start date")).AutoFocus().OnChange(changed)
				})
				ui.Icon(c, lucide.ArrowRight()).FontSize(14).TextColor(a.tk.TextQuaternary)
				ui.Column(c).Basis(0).Grow(1).Children(func() {
					field.Input(c.Key("range-end"), &a.date.input, field.InputProps{Placeholder: a.tr("结束", "End"), Invalid: a.date.error != ""}).Label(a.tr("结束日期", "End date")).OnChange(changed)
				})
			})
		} else {
			field.Input(c.Key("relative-date"), &a.date.input, field.InputProps{Placeholder: a.tr("明天、7 days、2026-12-31", "tomorrow, 7 days, 2026-12-31"), Prefix: glyph("calendar"), Invalid: a.date.error != ""}).Label(a.tr("日期或相对时间", "Date or relative time")).AutoFocus().OnChange(func() { a.date.error = "" })
		}
		ui.Row(c).Gap(4).Children(func() { a.dateQuick(c) })
		ui.Row(c).Gap(16).Children(func() {
			a.calendarMonth(c, a.date.left, false)
			a.calendarMonth(c, a.date.left.AddDate(0, 1, 0), true)
		})
		if a.date.error != "" {
			ui.Text(c, a.date.error).FontSize(12).TextColor(a.tk.Error)
		}
		ui.Row(c).Gap(8).Children(func() {
			quietButton(c, a.tr("移除日期", "Remove date")).OnClick(func() { a.date.input, a.date.start = "", ""; a.saveDate() })
			ui.Spacer(c)
			btn(c, a.tr("取消", "Cancel")).OnClick(func() { a.date.open = false })
			primary(c, a.tr("保存", "Save date")).Label(a.tr("保存日期", "Save date")).OnClick(a.saveDate)
		})
		c.OnShortcut(ui.Cmd, ui.KeyEnter, a.saveDate)
	})
}

func (a *app) calendarMonth(c *ui.Context, month time.Time, right bool) {
	ui.Column(c.Key(month.Format("2006-01"))).Width(292).Gap(4).Children(func() {
		ui.Row(c).Children(func() {
			if !right {
				iconButton(c, "back", a.tr("上个月", "Previous month")).OnClick(func() { a.date.left = a.date.left.AddDate(0, -1, 0) })
			}
			ui.Text(c, a.tr(fmt.Sprintf("%d年%d月", month.Year(), month.Month()), month.Format("January 2006"))).Grow(1).TextAlign(ui.Center).FontWeight(500)
			if right {
				iconButton(c, "arrow", a.tr("下个月", "Next month")).OnClick(func() { a.date.left = a.date.left.AddDate(0, 1, 0) })
			}
		})
		ui.Row(c).Children(func() {
			for n, day := range []string{"Mo", "Tu", "We", "Th", "Fr", "Sa", "Su"} {
				ui.Text(c, a.tr([]string{"一", "二", "三", "四", "五", "六", "日"}[n], day)).Width(41).TextAlign(ui.Center).FontSize(11).TextColor(a.tk.TextQuaternary)
			}
		})
		start := month.AddDate(0, 0, -(int(month.Weekday())+6)%7)
		selected, _ := core.ParseDate(a.date.input, time.Now())
		// first..selected is the range shown; first is empty for a single day.
		first := ""
		if a.date.ranged {
			first, _ = core.ParseDate(a.date.start, time.Now())
			if first > selected {
				first, selected = selected, first
			}
			if selected == "" {
				selected, first = first, ""
			}
		}
		for week := 0; week < 6; week++ {
			ui.Row(c.Key(week)).Children(func() {
				for day := 0; day < 7; day++ {
					date := start.AddDate(0, 0, week*7+day)
					iso := date.Format("2006-01-02")
					b := ui.ButtonBase(c.Key(iso)).Size(41, 34).Radius(999).Center().FontSize(13).Cursor(ui.CursorPointer).Label(a.tr(date.Format("2006年1月2日"), date.Format("January 2, 2006")))
					if date.Month() != month.Month() {
						b.TextColor(a.tk.TextQuaternary)
					}
					inMonth := date.Month() == month.Month()
					// The day chosen is the accent's pill, today a gray one, as
					// the kit's calendar marks them.
					today := iso == time.Now().Format("2006-01-02")
					if inMonth && (iso == selected || iso == first) {
						accentFill(c, b, b.Hovered(), b.Pressed()).TextColor(white).FontWeight(600)
					} else if inMonth && first != "" && iso > first && iso < selected {
						b.Background(a.tk.Primary.Alpha(.14)).Radius(0)
					} else if b.Hovered() {
						b.Background(a.tk.FillSecondary)
					} else if today {
						b.Background(a.tk.FillTertiary).FontWeight(600)
					}
					if iso == a.date.focused.Format("2006-01-02") && date.Month() == month.Month() {
						b.Bind(&a.date.dayHandle)
					}
					b.Children(func() { ui.Text(c, fmt.Sprint(date.Day())) }).OnClick(func() { a.pickDay(date) })
				}
			})
		}
		move := func(days, months int) {
			a.date.focused = a.date.focused.AddDate(0, months, days)
			a.date.input = a.date.focused.Format("2006-01-02")
			if a.date.ranged {
				// Arrows move the end; a start past it moves along.
				if start, _ := core.ParseDate(a.date.start, time.Now()); start > a.date.input {
					a.date.start = a.date.input
				}
				a.date.picking = false
			}
			if a.date.focused.Before(a.date.left) || !a.date.focused.Before(a.date.left.AddDate(0, 2, 0)) {
				a.date.left = time.Date(a.date.focused.Year(), a.date.focused.Month(), 1, 0, 0, 0, 0, time.Local)
			}
			a.date.dayHandle.Focus()
		}
		c.OnShortcut(0, ui.KeyLeft, func() { move(-1, 0) })
		c.OnShortcut(0, ui.KeyRight, func() { move(1, 0) })
		c.OnShortcut(0, ui.KeyUp, func() { move(-7, 0) })
		c.OnShortcut(0, ui.KeyDown, func() { move(7, 0) })
		c.OnShortcut(0, ui.KeyPageUp, func() { move(0, -1) })
		c.OnShortcut(0, ui.KeyPageDown, func() { move(0, 1) })
	})
}
