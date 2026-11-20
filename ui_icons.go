package main

import (
	"fmt"

	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/lucide"
)

// Icons come from Lucide, as Lobe UI's do, so that the app's own controls and
// the library's components share one set. They are looked up by role rather
// than by glyph, which lets a role change its picture in one place.
var icons = map[string]func() *ui.SVG{
	"issues":    lucide.SquareCheck,
	"project":   lucide.Box,
	"search":    lucide.Search,
	"plus":      lucide.Plus,
	"star":      lucide.Star,
	"archive":   lucide.Archive,
	"trash":     lucide.Trash2,
	"settings":  lucide.SlidersHorizontal,
	"gear":      lucide.Settings,
	"up":        lucide.ChevronUp,
	"down":      lucide.ChevronDown,
	"back":      lucide.ArrowLeft,
	"forward":   lucide.ArrowRight,
	"caret":     lucide.ChevronRight,
	"list":      lucide.List,
	"board":     lucide.Columns3,
	"label":     lucide.Tag,
	"more":      lucide.Ellipsis,
	"check":     lucide.Check,
	"close":     lucide.X,
	"calendar":  lucide.Calendar,
	"arrow":     lucide.ArrowRight,
	"sidebar":   lucide.PanelLeft,
	"clock":     lucide.Clock,
	"filter":    lucide.ListFilter,
	"compose":   lucide.SquarePen,
	"pencil":    lucide.Pencil,
	"link":      lucide.Link,
	"paperclip": lucide.Paperclip,
	"layers":    lucide.Layers,
	"help":      lucide.CircleHelp,
	"expand":    lucide.Maximize2,
	"code":      lucide.Code,
	"bookmark":  lucide.Bookmark,
	"subissue":  lucide.CornerDownRight,
	"comment":   lucide.MessageSquare,
	"milestone": lucide.Diamond,
	"document":  lucide.FileText,
	"relation":  lucide.Waypoints,
	"folder":    lucide.Folder,
	"block":     lucide.Blocks,
	"bold":      lucide.Bold,
	"italic":    lucide.Italic,
	"template":  lucide.LayoutTemplate,
	"timeline":  lucide.ChartGantt,
	"repeat":    lucide.Repeat,
	"undo":      lucide.Undo2,
	"redo":      lucide.Redo2,
}

// Workflow states are drawn, not typed: glyphs such as ◌ and ◐ differ in size
// and weight between fonts, which made a column of them look uneven.
var statusIcons = func() map[string]*ui.SVG {
	ring := `<circle cx="7" cy="7" r="6" fill="none" stroke="currentColor" stroke-width="1.5"/>`
	disc := `M7 0a7 7 0 1 0 0 14A7 7 0 0 0 7 0Z`
	shapes := map[string]string{
		"backlog":   `<circle cx="7" cy="7" r="6" fill="none" stroke="currentColor" stroke-width="1.5" stroke-dasharray="1.4 1.74"/>`,
		"todo":      ring,
		"progress":  ring + `<path fill="currentColor" d="M7 3.5a3.5 3.5 0 0 1 0 7Z"/>`,
		"review":    ring + `<path fill="currentColor" d="M7 7V3.5a3.5 3.5 0 1 1-3.5 3.5Z"/>`,
		"done":      `<path fill="currentColor" fill-rule="evenodd" d="` + disc + `M4.4 6.8L5.98 8.39 9.68 4.33 10.72 5.27 6.02 10.41 3.4 7.8Z"/>`,
		"canceled":  `<path fill="currentColor" fill-rule="evenodd" d="` + disc + `M8.38 4.7L9.3 5.62 7.92 7 9.3 8.38 8.38 9.3 7 7.92 5.62 9.3 4.7 8.38 6.08 7 4.7 5.62 5.62 4.7 7 6.08Z"/>`,
		"duplicate": `<path fill="currentColor" fill-rule="evenodd" d="` + disc + `M8.66 4.42L9.58 5.34 5.34 9.58 4.42 8.66Z"/>`,
	}
	out := map[string]*ui.SVG{}
	for k, v := range shapes {
		out[k] = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 14 14">` + v + `</svg>`))
	}
	return out
}()

// Priority reads as signal strength: more solid bars mean more urgent.
var priorityIcons = func() [5]*ui.SVG {
	bar := func(x, y, h float32, solid bool) string {
		opacity := `1`
		if !solid {
			opacity = `.32`
		}
		return fmt.Sprintf(`<rect x="%g" y="%g" width="3" height="%g" rx="1" fill="currentColor" fill-opacity="%s"/>`, x, y, h, opacity)
	}
	bars := func(n int) string {
		return bar(1.5, 8, 6, n >= 1) + bar(6.5, 5, 9, n >= 2) + bar(11.5, 2, 12, n >= 3)
	}
	shapes := [5]string{
		`<rect x="1.5" y="7.25" width="3" height="1.5" rx=".75" fill="currentColor"/><rect x="6.5" y="7.25" width="3" height="1.5" rx=".75" fill="currentColor"/><rect x="11.5" y="7.25" width="3" height="1.5" rx=".75" fill="currentColor"/>`,
		`<path fill="currentColor" fill-rule="evenodd" d="M3.5 1h9A2.5 2.5 0 0 1 15 3.5v9a2.5 2.5 0 0 1-2.5 2.5h-9A2.5 2.5 0 0 1 1 12.5v-9A2.5 2.5 0 0 1 3.5 1ZM7.1 4H8.9V9.2H7.1ZM7.1 10.4H8.9V12.2H7.1Z"/>`,
		bars(3), bars(2), bars(1),
	}
	var out [5]*ui.SVG
	for n, v := range shapes {
		out[n] = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16">` + v + `</svg>`))
	}
	return out
}()

func icon(c *ui.Context, name string) ui.Element { return ui.Icon(c, glyph(name)).Size(16, 16) }

func glyph(name string) *ui.SVG {
	if load, ok := icons[name]; ok {
		return load()
	}
	return lucide.Circle()
}

func statusColor(s string, dark bool) ui.Color {
	if !dark {
		switch s {
		case "todo":
			return ui.Hex("#6f717c")
		case "progress":
			return ui.Hex("#b47d06")
		case "review":
			return ui.Hex("#25834e")
		case "done":
			return ui.Hex("#7352f5") // the accent: finished work is the app's color
		default:
			return ui.Hex("#9b9dab")
		}
	}
	switch s {
	case "todo":
		return ui.Hex("#c2c4cf")
	case "progress":
		return ui.Hex("#e9c15a")
	case "review":
		return ui.Hex("#62c08c")
	case "done":
		return ui.Hex("#9a80ff")
	default:
		return ui.Hex("#7d7f8c")
	}
}

func statusMark(c *ui.Context, s string) ui.Element {
	svg := statusIcons[s]
	if svg == nil {
		svg = statusIcons["backlog"]
	}
	return ui.Icon(c, svg).Size(14, 14).TextColor(statusColor(s, c.Theme().Dark))
}

func priorityColor(p int, t *ui.Theme) ui.Color {
	switch {
	case p == 1 && t.Dark:
		return ui.Hex("#f2894b")
	case p == 1:
		return ui.Hex("#d5620f")
	case p <= 0 || p > 4:
		return t.TextMuted.Alpha(.7)
	}
	return t.TextMuted
}

func priorityMark(c *ui.Context, p int) ui.Element {
	if p < 0 || p > 4 {
		p = 0
	}
	return ui.Icon(c, priorityIcons[p]).Size(16, 16).TextColor(priorityColor(p, c.Theme()))
}
