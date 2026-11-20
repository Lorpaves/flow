package main

import (
	"flow/internal/core"
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestLabelEditingScopeAndInlineCreation(t *testing.T) {
	a := testApp()
	projects := a.data.ProjectList(false, false)
	p, other := projects[0], projects[1]
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(a.theme)
		ui.Column(c).Fill().Padding(24).Gap(10).Children(func() { a.labelsManager(c) })
		a.organizationDialogs(c)
	}, 1000, 680)
	click(t, tt, "New label")
	tt.Type("范围标签")
	click(t, tt, "Label group")
	tt.Type("设计")
	click(t, tt, "Label scope")
	tt.Type(p.Name)
	tt.Key(0, ui.KeyEnter)
	click(t, tt, "Save label")
	var id string
	for _, l := range a.data.Labels {
		if l.Name == "范围标签" {
			id = l.ID
		}
	}
	if id == "" || a.data.Labels[id].Group != "设计" || a.data.Labels[id].ProjectID != p.ID {
		t.Fatalf("label did not save: %s", a.err)
	}
	if slices.ContainsFunc(a.data.AvailableLabels(other.ID), func(v core.Label) bool { return v.ID == id }) {
		t.Fatal("scoped label available in another project")
	}
	click(t, tt, "Edit label: 范围标签")
	tt.Command("selectAll")
	tt.Type("本地改名")
	remote := a.data.Clone()
	v := remote.Labels[id]
	v.Color = "#112233"
	remote.Labels[id] = v
	a.acceptExternal(remote)
	click(t, tt, "Save label")
	if got := a.data.Labels[id]; got.Name != "本地改名" || got.Color != "#112233" {
		t.Fatalf("rename overwrote concurrent color: %+v", got)
	}
	capture(t, tt, "22-label-management")
	a.navigate("project", p.ID)
	a.newTask("", "backlog")
	full := ui.NewTester(a.view, 1000, 680)
	capture(t, full, "22-inline-label-before")
	click(t, full, "New issue labels")
	full.Type("新建并选择")
	full.Key(0, ui.KeyEnter)
	if len(a.newIssue.LabelIDs) != 1 {
		t.Fatalf("inline creation did not select label: %s", a.err)
	}
	created := a.data.Labels[a.newIssue.LabelIDs[0]]
	if created.Name != "新建并选择" || created.ProjectID != p.ID {
		t.Fatalf("inline label scope: %+v", created)
	}
	full.Key(0, ui.KeyEscape)
	a.closeComposition()
}

func TestFavoriteFoldersDragConcurrencyAndSidebarResize(t *testing.T) {
	a := testApp()
	p := a.data.ProjectList(false, false)[0]
	p.Favorite = true
	a.data.Projects[p.ID] = p
	tt := ui.NewTester(a.view, 1360, 880)
	click(t, tt, "New favorite folder")
	tt.Type("工作收藏")
	click(t, tt, "Save folder")
	folders := a.data.FolderList()
	if len(folders) != 1 {
		t.Fatalf("folder did not save: %s", a.err)
	}
	id := folders[0].ID
	from, ok := tt.Find("Open favorite: " + p.Name)
	if !ok {
		t.Fatal("loose favorite missing")
	}
	to, ok := tt.Find("Favorite folder: 工作收藏")
	if !ok {
		t.Fatal("folder missing")
	}
	x, y := from.X+from.W/2, from.Y+from.H/2
	dx, dy := to.X+to.W/2, to.Y+to.H/2
	tt.Press(x, y)
	tt.Move(x+12, y)
	tt.Move(dx, dy)
	tt.Release(dx, dy)
	ref := core.FavoriteRef{Kind: "project", ID: p.ID}
	if !slices.Contains(a.data.Folders[id].Items, ref) {
		t.Fatalf("favorite drag failed: %s", a.err)
	}
	click(t, tt, "Favorite folder: 工作收藏")
	if _, ok := tt.Find("Open favorite: " + p.Name); ok {
		t.Fatal("collapsed folder still displays its item")
	}
	click(t, tt, "Favorite folder: 工作收藏")
	divider, ok := tt.Find("Divider")
	if !ok {
		t.Fatal("sidebar resize control absent")
	}
	tt.Press(divider.X+divider.W/2, divider.Y+divider.H/2)
	tt.Move(divider.X+divider.W/2+70, divider.Y+divider.H/2)
	tt.Release(divider.X+divider.W/2+70, divider.Y+divider.H/2)
	if a.sidebarWidth < 280 || a.sidebarWidth > 320 {
		t.Fatalf("sidebar did not resize: %v", a.sidebarWidth)
	}
	capture(t, tt, "23-favorite-folders")
	a.editFolder(id)
	a.folderForm.Name = "Local rename"
	remote := a.data.Clone()
	folder := remote.Folders[id]
	i := a.data.Query(core.Filter{})[0]
	extra := core.FavoriteRef{Kind: "issue", ID: i.ID}
	folder.Items = append(folder.Items, extra)
	remote.Folders[id] = folder
	a.acceptExternal(remote)
	a.saveFolder()
	if got := a.data.Folders[id]; got.Name != "Local rename" || !slices.Contains(got.Items, extra) {
		t.Fatalf("folder rename discarded MCP item: %+v", got)
	}
	a.apply(func(d *core.Data) error { delete(d.Folders, id); return nil }, nil)
	if _, ok := a.data.Projects[p.ID]; !ok {
		t.Fatal("deleting folder deleted its project")
	}
}
