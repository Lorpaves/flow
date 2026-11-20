package mcpserver

import (
	"context"
	"flow/internal/core"
)

type ShiftScheduleInput struct {
	Targets []core.ScheduleTarget `json:"targets" jsonschema:"Items to move together. kind is project, milestone or issue. ID is the actual record ID."`
	Days    int                   `json:"days" jsonschema:"Signed calendar day offset"`
	Mode    string                `json:"mode,omitempty" jsonschema:"auto (default): bump planned dependencies on overlap and move project milestones; all: shift dependency chain or subsequent milestones; single: only explicit items"`
}

func (a *API) shiftSchedule(_ context.Context, in ShiftScheduleInput) (any, error) {
	return a.mutate(func(d *core.Data) (any, error) {
		changed, err := d.ShiftSchedule(in.Targets, in.Days, in.Mode)
		return map[string]any{"changed": changed}, err
	})
}
