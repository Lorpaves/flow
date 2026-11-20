package mcpserver

import (
	"context"
	"errors"
	"flow/internal/core"
	"strings"
	"time"
)

func saveViewSchema() map[string]any {
	str := map[string]any{"type": "string"}
	stringsSchema := map[string]any{"type": "array", "items": str}
	rule := map[string]any{"type": "object", "properties": map[string]any{"id": str, "field": str, "op": str, "values": stringsSchema}, "required": []string{"field", "op"}, "additionalProperties": false}
	group := map[string]any{"type": "object", "properties": map[string]any{"any": map[string]any{"type": "boolean"}, "rules": map[string]any{"type": "array", "items": rule}, "groups": map[string]any{"type": "array", "items": map[string]any{"$ref": "#/$defs/group"}}}, "additionalProperties": false}
	props := map[string]any{}
	for _, key := range []string{"layout", "groupBy", "subgroup", "orderBy", "completed", "subIssues"} {
		props[key] = str
	}
	props["descending"] = map[string]any{"type": "boolean"}
	props["emptyGroups"] = map[string]any{"type": "boolean"}
	props["properties"] = stringsSchema
	props["filter"] = map[string]any{"$ref": "#/$defs/group"}
	config := map[string]any{"type": "object", "properties": props, "required": []string{"layout", "groupBy", "subgroup", "orderBy", "completed", "subIssues", "properties"}, "additionalProperties": false}
	return map[string]any{"type": "object", "$defs": map[string]any{"group": group}, "properties": map[string]any{"id": str, "name": str, "projectId": str, "query": str, "config": config}, "required": []string{"name"}, "additionalProperties": false}
}

type SaveViewInput struct {
	ID        string           `json:"id,omitempty" jsonschema:"Existing view ID to replace; omitted creates a new view"`
	Name      string           `json:"name"`
	ProjectID string           `json:"projectId,omitempty"`
	Query     string           `json:"query,omitempty"`
	Config    *core.ViewConfig `json:"config,omitempty" jsonschema:"View configuration; omitted uses existing configuration or defaults for a new view"`
}
type QueryViewInput struct {
	ID string `json:"id"`
	Page
}

func (a *API) listViews(_ context.Context, in Page) (any, error) {
	d, e := a.snapshot()
	if e != nil {
		return nil, e
	}
	out := []core.SavedView{}
	for _, v := range d.ViewList() {
		if strings.Contains(strings.ToLower(v.Name), strings.ToLower(in.Query)) {
			out = append(out, v)
		}
	}
	return pagination(out, in)
}
func (a *API) getView(_ context.Context, in IDInput) (any, error) {
	d, e := a.snapshot()
	if e != nil {
		return nil, e
	}
	v, ok := d.Views[in.ID]
	if !ok {
		return nil, errors.New("view_not_found")
	}
	return v, nil
}
func (a *API) saveView(_ context.Context, in SaveViewInput) (any, error) {
	return a.mutate(func(d *core.Data) (any, error) {
		id := in.ID
		config := core.DefaultView()
		if id == "" {
			id = core.ID()
		} else {
			v, ok := d.Views[id]
			if !ok {
				return nil, errors.New("view_not_found")
			}
			config = v.Config.Clone()
		}
		if in.Config != nil {
			config = in.Config.Clone()
		}
		v := core.SavedView{ID: id, Name: strings.TrimSpace(in.Name), ProjectID: in.ProjectID, Query: in.Query, Config: config}
		if d.Views == nil {
			d.Views = map[string]core.SavedView{}
		}
		d.Views[id] = v
		return v, nil
	})
}
func (a *API) deleteView(_ context.Context, in IDInput) (any, error) {
	return a.mutate(func(d *core.Data) (any, error) {
		if _, ok := d.Views[in.ID]; !ok {
			return nil, errors.New("view_not_found")
		}
		d.RemoveSavedView(in.ID)
		return map[string]any{"deleted": in.ID}, nil
	})
}
func (a *API) queryView(_ context.Context, in QueryViewInput) (any, error) {
	d, e := a.snapshot()
	if e != nil {
		return nil, e
	}
	v, ok := d.Views[in.ID]
	if !ok {
		return nil, errors.New("view_not_found")
	}
	query := v.Query
	if in.Query != "" {
		query = in.Query
	}
	out := []map[string]any{}
	for _, i := range d.QueryView(core.Filter{Query: query, ProjectID: v.ProjectID}, v.Config, time.Now()) {
		out = append(out, issueView(i, false))
	}
	return pagination(out, in.Page)
}
