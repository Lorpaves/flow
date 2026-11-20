package core

import "testing"

func TestProjectBatchRequiresParentAndMovesChildren(t *testing.T) {
	d := Empty()
	d.Projects["a"] = Project{ID: "a", Name: "A", Status: "active"}
	d.Projects["b"] = Project{ID: "b", Name: "B", Status: "active"}
	p, _ := d.AddIssue("parent", "a", "", "todo")
	c, _ := d.AddIssue("child", "a", p.ID, "todo")
	if e := d.MoveIssueProjects([]string{c.ID}, "b"); e == nil {
		t.Fatal("child silently ignored a project move")
	}
	if d.Issues[c.ID].ProjectID != "a" {
		t.Fatal("failed move changed data")
	}
	if e := d.MoveIssueProjects([]string{c.ID, p.ID}, "b"); e != nil {
		t.Fatal(e)
	}
	if d.Issues[p.ID].ProjectID != "b" || d.Issues[c.ID].ProjectID != "b" {
		t.Fatal("parent/child batch depends on selection order")
	}
	if e := d.Validate(); e != nil {
		t.Fatal(e)
	}
}
