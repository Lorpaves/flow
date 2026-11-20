package main

import (
	"errors"
	"flow/internal/core"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/field"
)

func (a *app) runDataJob(run func(*core.Store) (core.Data, error), after func()) {
	if a.window == nil {
		return
	}
	a.pending++
	a.err = ""
	a.jobs <- job{run: run, done: func(data core.Data, e error) {
		a.pending--
		if e != nil {
			a.err = e.Error()
			a.cancelClosing()
			return
		}
		a.acceptExternal(data)
		if after != nil {
			after()
		}
		a.finishQuit()
	}}
}
func (a *app) importAttachments(kind, id string, paths []string) {
	if len(paths) == 0 {
		return
	}
	paths = slices.Clone(paths)
	read := func(d *core.Data) error {
		for _, path := range paths {
			v, e := core.AttachmentFromFile(path)
			if e != nil {
				return e
			}
			v.OwnerKind, v.OwnerID = kind, id
			if _, e = d.AddAttachment(v); e != nil {
				return e
			}
		}
		return nil
	}
	if a.window == nil {
		a.apply(read, nil)
		return
	}
	// File I/O precedes the SQLite transaction; no picker or file read holds its lock.
	a.runDataJob(func(s *core.Store) (core.Data, error) {
		records := []core.Attachment{}
		for _, path := range paths {
			v, e := core.AttachmentFromFile(path)
			if e != nil {
				return core.Data{}, e
			}
			v.OwnerKind, v.OwnerID = kind, id
			records = append(records, v)
		}
		return s.Update(func(d *core.Data) error {
			for _, v := range records {
				if _, e := d.AddAttachment(v); e != nil {
					return e
				}
			}
			return nil
		})
	}, nil)
}
func (a *app) chooseAttachments(kind, id string) {
	if a.window == nil {
		return
	}
	w := a.window
	title := a.tr("添加附件", "Attach files")
	go func() {
		paths, e := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: w, Title: title, Multiple: true})
		w.Update(func() {
			if e != nil {
				a.err = e.Error()
				return
			}
			a.importAttachments(kind, id, paths)
		})
	}()
}
func writeAttachmentExport(path string, v core.Attachment) error {
	if v.URL != "" {
		return errors.New("attachment_is_link")
	}
	f, e := os.CreateTemp(filepath.Dir(path), ".flow-attachment-*")
	if e != nil {
		return e
	}
	temp := f.Name()
	defer os.Remove(temp)
	if _, e = f.Write(v.Data); e != nil {
		f.Close()
		return e
	}
	if e = f.Sync(); e != nil {
		f.Close()
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	return os.Rename(temp, path)
}
func (a *app) exportAttachment(v core.Attachment) {
	if v.URL != "" {
		a.services.OpenURL(v.URL)
		return
	}
	if a.window == nil {
		return
	}
	w := a.window
	title := a.tr("保存附件", "Save attachment")
	go func() {
		path, e := mygo.Dialog.Save(mygo.SaveDialogOptions{Parent: w, Title: title, DefaultPath: v.Name})
		w.Update(func() {
			if e != nil {
				a.err = e.Error()
				return
			}
			if path == "" {
				return
			}
			a.runDataJob(func(s *core.Store) (core.Data, error) { return s.Snapshot(), writeAttachmentExport(path, v) }, func() { a.notice = a.tr("附件已保存", "Attachment saved") })
		})
	}()
}
func (a *app) addLink(kind, id string) {
	a.attachmentOwnerKind, a.attachmentOwnerID = kind, id
	a.attachmentName, a.attachmentURL = "", ""
	a.attachmentOpen = true
}

// attachmentSection lists what is attached. An issue shows it only once it
// holds something, since adding starts from the row under the description.
func (a *app) attachmentSection(c *ui.Context, kind, id string) {
	items := a.attachmentsOf(kind, id)
	if len(items) == 0 && kind == "issue" {
		return
	}
	ui.Column(c.Key("attachments-" + kind)).Gap(6).Children(func() {
		a.sectionHeader(c, a.tr("附件", "Attachments"), "", func() {
			iconButton(c, "link", a.tr("添加链接", "Add link")).OnClick(func() { a.addLink(kind, id) })
			iconButton(c, "paperclip", a.tr("添加文件", "Attach files")).OnClick(func() { a.chooseAttachments(kind, id) })
		})
		a.attachmentItems(c, kind, id)
	})
}
func (a *app) attachmentsOf(kind, id string) []core.Attachment {
	items := []core.Attachment{}
	for _, v := range a.data.Attachments {
		if v.OwnerKind == kind && v.OwnerID == id {
			items = append(items, v)
		}
	}
	slices.SortFunc(items, func(x, y core.Attachment) int { return strings.Compare(x.ID, y.ID) })
	return items
}
func (a *app) attachmentItems(c *ui.Context, kind, id string) {
	for _, v := range a.attachmentsOf(kind, id) {
		row := ui.Row(c.Key("attachment-"+v.ID)).Height(40).Gap(10).Padding(0, 4, 0, 10).Radius(a.tk.BorderRadiusSM)
		hovered := row.Hovered()
		if hovered {
			row.Background(a.rowHover())
		}
		row.Children(func() {
			mark := "paperclip"
			if v.URL != "" {
				mark = "link"
			}
			ui.Icon(c, glyph(mark)).FontSize(15).TextColor(a.tk.TextTertiary)
			open := a.tr("另存为…", "Save as…")
			if v.URL != "" {
				open = a.tr("打开链接", "Open link")
			}
			name := ui.ButtonBase(c).Label(open + " " + v.Name).Grow(1).MinWidth(0).Height(28).Gap(8).Justify(ui.Start).Cursor(ui.CursorPointer)
			name.Children(func() {
				ui.Text(c, v.Name).SingleLine()
				detail := v.URL
				if detail == "" {
					detail = fmt.Sprintf("%.1f KB", float64(len(v.Data))/1024)
				}
				ui.Text(c, detail).SingleLine().MinWidth(0).FontSize(12).TextColor(a.tk.TextQuaternary)
			}).OnClick(func() { a.exportAttachment(v) })
			reveal(iconButton(c, "trash", a.tr("删除附件 ", "Delete attachment ")+v.Name), hovered).OnClick(func() { a.apply(func(d *core.Data) error { delete(d.Attachments, v.ID); return nil }, nil) })
		})
	}
}
func (a *app) attachmentDialog(c *ui.Context) {
	ui.DialogBase(c, &a.attachmentOpen, func(backdrop, panel ui.Element) {
		a.dialogChrome(backdrop, panel)
		panel.Width(480).Padding(16).Gap(12)
		a.dialogTitle(c, a.tr("添加链接", "Add link"), nil)
		field.Input(c.Key("attachment-name"), &a.attachmentName, field.InputProps{Placeholder: a.tr("名称", "Name")}).AutoFocus().Label(a.tr("链接名称", "Link name"))
		field.Input(c.Key("attachment-url"), &a.attachmentURL, field.InputProps{Placeholder: "https://…", Prefix: glyph("link")}).Label(a.tr("链接地址", "Link URL"))
		a.dialogError(c)
		dialogFooter(c, func() {
			btn(c, a.tr("取消", "Cancel")).OnClick(func() { a.attachmentOpen = false })
			primary(c, a.tr("添加", "Add")).Disabled(a.pending > 0).OnClick(func() {
				v := core.Attachment{OwnerKind: a.attachmentOwnerKind, OwnerID: a.attachmentOwnerID, Name: strings.TrimSpace(a.attachmentName), URL: strings.TrimSpace(a.attachmentURL)}
				a.apply(func(d *core.Data) error { _, e := d.AddAttachment(v); return e }, func() { a.attachmentOpen = false })
			})
		})
	})
}
