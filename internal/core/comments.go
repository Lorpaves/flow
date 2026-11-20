package core

import (
	"slices"
	"strings"
)

type CommentEntry struct {
	Comment Comment
	Depth   int
}

// CommentThread keeps chronological siblings together and retains a deleted
// ancestor only when it has a visible descendant. The UI renders a tombstone,
// never the deleted body's contents.
func (d Data) CommentThread(issue string) []CommentEntry {
	visible := map[string]bool{}
	for id, v := range d.Comments {
		if v.IssueID != issue || v.DeletedAt != 0 {
			continue
		}
		for id != "" && !visible[id] {
			visible[id] = true
			id = d.Comments[id].ParentID
		}
	}
	children := map[string][]Comment{}
	for id := range visible {
		v, ok := d.Comments[id]
		if ok {
			children[v.ParentID] = append(children[v.ParentID], v)
		}
	}
	for _, items := range children {
		slices.SortFunc(items, func(a, b Comment) int {
			if a.CreatedAt < b.CreatedAt {
				return -1
			}
			if a.CreatedAt > b.CreatedAt {
				return 1
			}
			return strings.Compare(a.ID, b.ID)
		})
	}
	result := []CommentEntry{}
	var visit func(string, int)
	visit = func(parent string, depth int) {
		for _, v := range children[parent] {
			result = append(result, CommentEntry{v, depth})
			visit(v.ID, depth+1)
		}
	}
	visit("", 0)
	return result
}
