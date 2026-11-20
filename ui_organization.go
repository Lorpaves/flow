package main

import (
	"flow/internal/core"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/field"
)

func (a *app) editLabel(id, name, project string) {
	v := a.data.Labels[id]
	if id == "" {
		v = core.Label{Name: name, ProjectID: project, Color: "#a799e8"}
	}
	a.labelForm, a.labelBase = v, v
	a.labelOpen = true
}
func (a *app) saveLabel() {
	form, base := a.labelForm, a.labelBase
	a.flushDrafts()
	a.apply(func(d *core.Data) error {
		if current, ok := d.Labels[form.ID]; ok {
			form = core.MergeLabelEdits(base, form, current)
		}
		_, e := d.SaveLabel(form)
		return e
	}, func() { a.labelOpen = false })
}
func (a *app) labelsManager(c *ui.Context) {
	a.sectionHeader(c, a.tr("标签", "Labels"), "", func() {
		actionButton(c, "plus", a.tr("任务标签", "Issue label")).Label(a.tr("新建标签", "New label")).OnClick(func() { a.editLabel("", "", "") })
		actionButton(c, "plus", a.tr("项目标签", "Project label")).Label(a.tr("新建项目标签", "New project label")).OnClick(func() { a.editLabel("", "", ""); a.labelForm.Kind = "project"; a.labelBase = a.labelForm })
	})
	a.card(ui.Column(c)).Padding(4).Children(func() {
		ui.Column(c).AlignItems(ui.Stretch).Margin(0, 0, 4, 0).Children(func() {
			field.SearchBar(c.Key("label-search"), &a.labelQuery, field.SearchBarProps{Placeholder: a.tr("搜索", "Search"), Variant: field.InputBorderless}).Label(a.tr("搜索标签", "Search labels"))
		})
		items := append(a.data.LabelList(), a.data.LabelsForKind("project")...)
		slices.SortStableFunc(items, func(x, y core.Label) int {
			return strings.Compare(strings.ToLower(x.Group+"/"+x.Name), strings.ToLower(y.Group+"/"+y.Name))
		})
		group, shown := "\x00", 0
		for _, l := range items {
			scope := ""
			if l.EntityKind() == "project" {
				scope = a.tr("项目标签", "Project label")
			}
			if p, ok := a.data.Projects[l.ProjectID]; ok {
				scope = p.Name
			}
			if !strings.Contains(strings.ToLower(l.Name+" "+l.Group+" "+scope), strings.ToLower(strings.TrimSpace(a.labelQuery))) {
				continue
			}
			shown++
			if l.Group != group {
				group = l.Group
				if group != "" {
					ui.Text(c.Key("group-"+group), group).FontSize(12).TextColor(a.tk.TextQuaternary).Padding(8, 8, 4, 8)
				}
			}
			row := ui.Row(c.Key(l.ID)).Height(36).Padding(0, 4, 0, 8).Gap(10).Radius(a.tk.BorderRadiusSM)
			hovered := row.Hovered()
			if hovered {
				row.Background(a.rowHover())
			}
			row.Children(func() {
				ui.Box(c).Size(8, 8).Radius(4).Background(ui.Hex(l.Color))
				edit := ui.ButtonBase(c).Label(a.tr("编辑标签：", "Edit label: ") + l.Name).Grow(1).MinWidth(0).Height(28).Justify(ui.Start).Cursor(ui.CursorPointer)
				edit.Children(func() { ui.Text(c, l.Name).SingleLine() }).OnClick(func() { a.editLabel(l.ID, "", "") })
				ui.Text(c, scope).FontSize(12).TextColor(a.tk.TextQuaternary)
				reveal(iconButton(c, "trash", a.tr("删除标签 ", "Delete label ")+l.Name), hovered).Disabled(a.pending > 0).OnClick(func() { a.flushDrafts(); a.apply(func(d *core.Data) error { d.RemoveLabel(l.ID); return nil }, nil) })
			})
		}
		if shown == 0 {
			ui.Text(c, a.tr("没有标签", "No labels")).FontSize(13).Padding(8).TextColor(a.tk.TextTertiary)
		}
	})
}

type favoriteDrag struct{ Ref core.FavoriteRef }

func (a *app) favoriteTitle(ref core.FavoriteRef) (string, bool) {
	switch ref.Kind {
	case "issue":
		i, ok := a.data.Issues[ref.ID]
		return i.Code() + " " + i.Title, ok && i.DeletedAt == 0
	case "project":
		p, ok := a.data.Projects[ref.ID]
		return p.Name, ok && p.DeletedAt == 0
	case "view":
		v, ok := a.data.Views[ref.ID]
		return v.Name, ok
	}
	return "", false
}
func (a *app) openFavorite(ref core.FavoriteRef) {
	switch ref.Kind {
	case "issue":
		a.openSearchIssue(ref.ID, "")
	case "project":
		a.navigate("project", ref.ID)
		a.projectTab = "overview"
	case "view":
		a.openSavedView(ref.ID)
	}
}
func (a *app) addFavoriteToFolder(folder string, ref core.FavoriteRef) {
	a.apply(func(d *core.Data) error {
		v, ok := d.Folders[folder]
		if !ok {
			return nil
		}
		if !slices.Contains(v.Items, ref) {
			v.Items = append(slices.Clone(v.Items), ref)
		}
		_, e := d.SaveFolder(v)
		return e
	}, nil)
}
func (a *app) removeFavoriteFromFolder(folder string, ref core.FavoriteRef) {
	a.apply(func(d *core.Data) error {
		v, ok := d.Folders[folder]
		if !ok {
			return nil
		}
		v.Items = slices.DeleteFunc(slices.Clone(v.Items), func(i core.FavoriteRef) bool { return i == ref })
		_, e := d.SaveFolder(v)
		return e
	}, nil)
}
func (a *app) reorderFavorite(folder string, ref core.FavoriteRef, delta int) {
	a.apply(func(d *core.Data) error {
		v, ok := d.Folders[folder]
		if !ok {
			return nil
		}
		v.Items = slices.Clone(v.Items)
		n := slices.Index(v.Items, ref)
		if n >= 0 && n+delta >= 0 && n+delta < len(v.Items) {
			v.Items[n], v.Items[n+delta] = v.Items[n+delta], v.Items[n]
		}
		_, e := d.SaveFolder(v)
		return e
	}, nil)
}
func (a *app) reorderFolder(id string, delta int) {
	a.apply(func(d *core.Data) error {
		items := d.FolderList()
		n := slices.IndexFunc(items, func(v core.FavoriteFolder) bool { return v.ID == id })
		if n < 0 || n+delta < 0 || n+delta >= len(items) {
			return nil
		}
		items[n], items[n+delta] = items[n+delta], items[n]
		for k, v := range items {
			v.Position = int64(k)
			d.Folders[v.ID] = v
		}
		return nil
	}, nil)
}
func (a *app) editFolder(id string) {
	v := a.data.Folders[id]
	if id == "" {
		v.Position = time.Now().UnixMilli()
	}
	a.folderBase = v
	a.folderBase.Items = slices.Clone(v.Items)
	a.folderForm = v
	a.folderForm.Items = slices.Clone(v.Items)
	a.folderOpen = true
}
func (a *app) saveFolder() {
	form, base := a.folderForm, a.folderBase
	a.apply(func(d *core.Data) error {
		if current, ok := d.Folders[form.ID]; ok {
			if form.Name != base.Name {
				current.Name = form.Name
			}
			current.Items = slices.Clone(current.Items)
			for _, ref := range base.Items {
				if !slices.Contains(form.Items, ref) {
					current.Items = slices.DeleteFunc(current.Items, func(v core.FavoriteRef) bool { return v == ref })
				}
			}
			for _, ref := range form.Items {
				if !slices.Contains(base.Items, ref) && !slices.Contains(current.Items, ref) {
					current.Items = append(current.Items, ref)
				}
			}
			form = current
		}
		_, e := d.SaveFolder(form)
		return e
	}, func() { a.folderOpen = false })
}
func (a *app) favoriteRow(c *ui.Context, folder string, ref core.FavoriteRef) {
	name, ok := a.favoriteTitle(ref)
	if !ok {
		return
	}
	active := ref.Kind == "issue" && a.issueID == ref.ID || ref.Kind == "view" && a.savedViewID == ref.ID
	b := a.sideItem(c.Key(ref.Kind+ref.ID), active).Label(a.tr("打开收藏：", "Open favorite: ") + name).MinWidth(0).Drag(favoriteDrag{ref}).OnClick(func() { a.openFavorite(ref) })
	b.Children(func() {
		a.sideMark(c, func() {
			switch ref.Kind {
			case "project":
				ui.Box(c).Size(8, 8).Radius(3).Background(ui.Hex(a.data.Projects[ref.ID].Color))
			case "issue":
				statusMark(c, a.data.Issues[ref.ID].Status)
			default:
				ui.Icon(c, glyph("layers")).FontSize(16)
			}
		})
		ui.Text(c, name).Grow(1).MinWidth(0).SingleLine()
	})
	b.ContextMenu(func(m *ui.Menu) {
		if folder != "" {
			if m.Item(a.tr("上移", "Move up")).Chosen() {
				a.reorderFavorite(folder, ref, -1)
			}
			if m.Item(a.tr("下移", "Move down")).Chosen() {
				a.reorderFavorite(folder, ref, 1)
			}
			if m.Item(a.tr("移出文件夹", "Remove from folder")).Chosen() {
				a.removeFavoriteFromFolder(folder, ref)
			}
		}
		for _, f := range a.data.FolderList() {
			if f.ID != folder && m.Item(a.tr("添加到：", "Add to: ")+f.Name).Chosen() {
				a.addFavoriteToFolder(f.ID, ref)
			}
		}
	})
}

// sidebarFavorites lists what was starred, in folders or loose. It takes no
// room until something is.
func (a *app) sidebarFavorites(c *ui.Context) {
	if a.folderCollapsed == nil {
		a.folderCollapsed = map[string]bool{}
	}
	folders := a.data.FolderList()
	grouped := map[core.FavoriteRef]bool{}
	for _, folder := range folders {
		for _, ref := range folder.Items {
			grouped[ref] = true
		}
	}
	loose := []core.FavoriteRef{}
	for _, p := range a.data.ProjectList(false, false) {
		if ref := (core.FavoriteRef{Kind: "project", ID: p.ID}); p.Favorite && !grouped[ref] {
			loose = append(loose, ref)
		}
	}
	for _, i := range a.data.Query(core.Filter{Favorites: true}) {
		if ref := (core.FavoriteRef{Kind: "issue", ID: i.ID}); !grouped[ref] {
			loose = append(loose, ref)
		}
	}
	if len(folders)+len(loose) == 0 {
		return
	}
	a.sideSection(c, a.tr("收藏夹", "Starred"), a.tr("新建收藏文件夹", "New favorite folder"), false, nil, func() { a.editFolder("") })
	for _, folder := range folders {
		ui.Column(c.Key("folder-" + folder.ID)).Gap(1).Children(func() {
			closed := a.folderCollapsed[folder.ID]
			b := a.sideItem(c, false).Label(a.tr("收藏文件夹：", "Favorite folder: ") + folder.Name).OnClick(func() { a.folderCollapsed[folder.ID] = !a.folderCollapsed[folder.ID] })
			if _, over := ui.DragOver[favoriteDrag](b); over {
				b.Background(a.tk.FillSecondary)
			}
			if drag, drop := ui.Drop[favoriteDrag](b); drop {
				a.addFavoriteToFolder(folder.ID, drag.Ref)
			}
			b.Children(func() {
				a.sideMark(c, func() {
					turn := float32(90)
					if closed {
						turn = 0
					}
					ui.Icon(c, glyph("caret")).FontSize(14).Rotate(turn)
				})
				ui.Text(c, folder.Name).Grow(1).MinWidth(0).SingleLine()
			})
			b.ContextMenu(func(m *ui.Menu) {
				if m.Item(a.tr("编辑文件夹", "Edit folder")).Chosen() {
					a.editFolder(folder.ID)
				}
				if m.Item(a.tr("上移", "Move up")).Chosen() {
					a.reorderFolder(folder.ID, -1)
				}
				if m.Item(a.tr("下移", "Move down")).Chosen() {
					a.reorderFolder(folder.ID, 1)
				}
				if m.Item(a.tr("删除文件夹", "Delete folder")).Chosen() {
					a.apply(func(d *core.Data) error { delete(d.Folders, folder.ID); return nil }, nil)
				}
			})
			if !closed {
				ui.Column(c).Padding(0, 0, 0, 14).Gap(1).Children(func() {
					for _, ref := range folder.Items {
						a.favoriteRow(c, folder.ID, ref)
					}
				})
			}
		})
	}
	for _, ref := range loose {
		a.favoriteRow(c, "", ref)
	}
}
func (a *app) organizationDialogs(c *ui.Context) {
	ui.DialogBase(c, &a.labelOpen, func(backdrop, panel ui.Element) {
		a.dialogChrome(backdrop, panel)
		panel.Width(540).Padding(16).Gap(12)
		ui.Text(c, a.tr("编辑标签", "Edit label")).FontWeight(600)
		field.Input(c.Key("label-name"), &a.labelForm.Name, field.InputProps{}).AutoFocus().Label(a.tr("标签名称", "Label name")).Placeholder(a.tr("名称", "Name"))
		field.Input(c.Key("label-group"), &a.labelForm.Group, field.InputProps{}).Label(a.tr("标签分组", "Label group")).Placeholder(a.tr("分组（可选）", "Group (optional)"))
		field.Input(c.Key("label-color"), &a.labelForm.Color, field.InputProps{}).Label(a.tr("标签颜色", "Label color")).Placeholder("#a799e8")
		if a.labelForm.EntityKind() == "issue" {
			items := []pickerItem{{id: "", label: a.tr("整个工作区", "Entire workspace"), checked: a.labelForm.ProjectID == ""}}
			for _, p := range a.data.ProjectList(false, false) {
				items = append(items, pickerItem{id: p.ID, label: p.Name, checked: a.labelForm.ProjectID == p.ID})
			}
			scope := a.tr("整个工作区", "Entire workspace")
			if p, ok := a.data.Projects[a.labelForm.ProjectID]; ok {
				scope = p.Name
			}
			a.picker(c, a.chip(c, scope, nil).Label(a.tr("标签作用域", "Label scope")), a.tr("选择作用域", "Choose label scope"), "", 0, items, false, func(id string) { a.labelForm.ProjectID = id })
			if a.labelForm.ID != "" && a.labelForm.ProjectID != a.labelBase.ProjectID {
				ui.Text(c, a.tr("其他项目中的这个标签会被移除。", "This label will be removed from issues in other projects.")).FontSize(12).TextColor(a.tk.Warning).Wrap()
			}
		}
		a.dialogError(c)
		dialogFooter(c, func() {
			btn(c, a.tr("取消", "Cancel")).OnClick(func() { a.labelOpen = false })
			primary(c, a.tr("保存", "Save label")).Label(a.tr("保存标签", "Save label")).Disabled(a.pending > 0).OnClick(a.saveLabel)
		})
	})
	ui.DialogBase(c, &a.folderOpen, func(backdrop, panel ui.Element) {
		a.dialogChrome(backdrop, panel)
		panel.Width(580).Padding(16).Gap(12)
		ui.Text(c, a.tr("收藏文件夹", "Favorite folder")).FontWeight(600)
		field.Input(c.Key("folder-name"), &a.folderForm.Name, field.InputProps{}).AutoFocus().Label(a.tr("文件夹名称", "Folder name"))
		items := []pickerItem{}
		add := func(ref core.FavoriteRef) {
			if title, ok := a.favoriteTitle(ref); ok {
				items = append(items, pickerItem{id: ref.Kind + ":" + ref.ID, label: title, checked: slices.Contains(a.folderForm.Items, ref)})
			}
		}
		for _, p := range a.data.ProjectList(false, false) {
			add(core.FavoriteRef{Kind: "project", ID: p.ID})
		}
		for _, v := range a.data.ViewList() {
			add(core.FavoriteRef{Kind: "view", ID: v.ID})
		}
		for _, i := range a.data.Query(core.Filter{}) {
			add(core.FavoriteRef{Kind: "issue", ID: i.ID})
		}
		a.picker(c, a.chip(c, a.tr("添加…", "Add…"), func() { ui.Icon(c, glyph("plus")).FontSize(14) }).Label(a.tr("添加任务、项目或视图", "Add issues, projects, or views")), a.tr("搜索收藏项", "Search favorite items"), "", 0, items, true, func(key string) {
			kind, id, _ := strings.Cut(key, ":")
			ref := core.FavoriteRef{Kind: kind, ID: id}
			if n := slices.Index(a.folderForm.Items, ref); n >= 0 {
				a.folderForm.Items = slices.Delete(a.folderForm.Items, n, n+1)
			} else {
				a.folderForm.Items = append(a.folderForm.Items, ref)
			}
		})
		ui.Scroll(c).MaxHeight(200).Gap(5).Children(func() {
			for _, ref := range a.folderForm.Items {
				title, ok := a.favoriteTitle(ref)
				if !ok {
					title = a.tr("已移除的项目", "Unavailable item")
				}
				ui.Row(c.Key(ref.Kind + ref.ID)).Children(func() {
					ui.Text(c, title).Grow(1).SingleLine()
					iconButton(c, "close", a.tr("移除收藏项：", "Remove favorite item: ")+title).OnClick(func() {
						a.folderForm.Items = slices.DeleteFunc(a.folderForm.Items, func(v core.FavoriteRef) bool { return ref == v })
					})
				})
			}
		})
		a.dialogError(c)
		dialogFooter(c, func() {
			btn(c, a.tr("取消", "Cancel")).OnClick(func() { a.folderOpen = false })
			primary(c, a.tr("保存", "Save folder")).Label(a.tr("保存文件夹", "Save folder")).Disabled(a.pending > 0).OnClick(a.saveFolder)
		})
	})
}
