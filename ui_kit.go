package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
	"github.com/lobehub/lobe-mygo/pkg/choice"
	"github.com/lobehub/lobe-mygo/pkg/display"
	"github.com/lobehub/lobe-mygo/pkg/layout"
	"github.com/lobehub/lobe-mygo/pkg/motion"
	"github.com/lobehub/lobe-mygo/pkg/overlay"
	"github.com/lobehub/lobe-mygo/pkg/theme"
)

// The app is drawn with Lobe UI: its tokens color every surface and its
// components are the controls. What follows are the few shapes Flow adds on
// top, each built from those tokens so that it reads as part of the same kit.

func (a *app) hairline() ui.Color    { return a.tk.BorderSecondary }
func (a *app) rowHover() ui.Color    { return a.tk.FillTertiary }
func (a *app) rowSelected() ui.Color { return a.tk.PrimaryBg }

// tile is the color of a tile: the surface a page lays its records on, which
// are white (surface) in turn.
func (a *app) tile() ui.Color { return a.pal().tile }

// paletteOf is the palette of the appearance c is drawn in, for helpers that
// have no app at hand.
func paletteOf(c *ui.Context) palette {
	if theme.TokensOf(c).Dark {
		return darkPalette
	}
	return lightPalette
}

var white, black = ui.Hex("#ffffff"), ui.Hex("#000000")

// accentFill fills e with the accent's gradient: lighter while hot, as under
// the pointer, and deeper while pressed.
func accentFill(c *ui.Context, e ui.Element, hot, pressed bool) ui.Element {
	p := paletteOf(c)
	from, to := p.from, p.to
	switch {
	case pressed:
		from, to = theme.Mix(from, black, .08), theme.Mix(to, black, .08)
	case hot:
		from, to = theme.Mix(from, white, .1), theme.Mix(to, white, .1)
	}
	return e.Gradient(from, to, 90)
}

// selectedFill tints a chosen row with the accent, fading out to the row's
// own surface, as the kit marks the option taken in a list.
func (a *app) selectedFill(e ui.Element) ui.Element {
	return e.Gradient(theme.Mix(a.tk.BgContainer, a.pal().from, .14), theme.Mix(a.tk.BgContainer, a.pal().to, .03), 90)
}

// iconButton is Lobe's ActionIcon at the size of a toolbar, round as the
// kit's: the label names it and shows as its tooltip.
func iconButton(c *ui.Context, name, label string) ui.Element {
	return action.ActionIcon(c, glyph(name), action.ActionIconProps{BlockSize: 28, GlyphSize: 16, Title: label}).Radius(999)
}

// hotIconButton is an iconButton whose tooltip also shows its shortcut, written
// as Lobe's Hotkey takes it ("mod+k"), so the key is taught where it is used
// instead of in text on the page.
func hotIconButton(c *ui.Context, name, label, keys string) ui.Element {
	b := action.ActionIcon(c, glyph(name), action.ActionIconProps{BlockSize: 28, GlyphSize: 16}).Label(label).Radius(999)
	overlay.Tooltip(c, b, label, overlay.TooltipProps{Hotkey: keys})
	return b
}

// searchPill lays a borderless Lobe search field, which search builds and
// returns, in the kit's white pill, ringed in the accent while it is typed in.
func (a *app) searchPill(c *ui.Context, width float32, search func() ui.Element) {
	pill := ui.Column(c).Width(width).Height(36).Shrink(0).Justify(ui.Center).AlignItems(ui.Stretch).Radius(999).Background(a.tk.BgContainer).Border(1, ui.Transparent)
	var input ui.Element
	pill.Children(func() { input = search() })
	if input.Focused() {
		pill.Border(1, a.tk.Primary).Shadow(0, 0, 0, 3, a.tk.PrimaryBg)
	} else {
		a.lift(pill)
	}
}

// overlayTip gives e Lobe's tooltip with a shortcut, written as Lobe's Hotkey
// takes it ("mod+k").
func overlayTip(c *ui.Context, e ui.Element, text, keys string) {
	overlay.Tooltip(c, e, text, overlay.TooltipProps{Hotkey: keys})
}

// textButton is a quiet text action: Lobe's Text button, small, as a pill.
func textButton(c *ui.Context, label string) ui.Element {
	return action.Button(c, label, action.ButtonProps{Variant: action.Text, Size: theme.Small}).Justify(ui.Start).Radius(999)
}

// quietButton is a textButton in secondary color, for actions inside a row.
func quietButton(c *ui.Context, label string) ui.Element {
	return textButton(c, label).TextColor(theme.TokensOf(c).TextSecondary)
}

// actionButton is a quiet button led by an icon.
func actionButton(c *ui.Context, name, text string) ui.Element {
	tk := theme.TokensOf(c)
	return action.Button(c, "", action.ButtonProps{Variant: action.Text, Size: theme.Small}).Radius(999).TextColor(tk.TextSecondary).Children(func() {
		ui.Icon(c, glyph(name)).FontSize(14)
		ui.Text(c, text).SingleLine()
	})
}

// btn and primary are Lobe's Default and Primary buttons at dialog size, as
// the kit's pills: a white one, and one filled with the accent.
func btn(c *ui.Context, label string) ui.Element {
	return action.Button(c, label, action.ButtonProps{}).Radius(999).PaddingX(16)
}

func primary(c *ui.Context, label string) ui.Element {
	return accentButton(c, action.Button(c, label, action.ButtonProps{Variant: action.Primary})).PaddingX(16)
}

// dangerButton is a pill for what cannot be undone, in a red deep enough
// for its white label.
func dangerButton(c *ui.Context, label string) ui.Element {
	red := theme.TokensOf(c).ErrorActive
	b := action.Button(c, label, action.ButtonProps{Variant: action.Primary}).Radius(999).PaddingX(16).Border(1, ui.Transparent).TextColor(white)
	switch {
	case b.Pressed():
		red = theme.Mix(red, black, .16)
	case b.Hovered():
		red = theme.Mix(red, black, .08)
	}
	return b.Background(red)
}

// accentButton turns one of Lobe's Primary buttons into the kit's: a pill of
// the accent's gradient with white text, glowing faintly in its color.
func accentButton(c *ui.Context, b ui.Element) ui.Element {
	accentFill(c, b, b.Hovered(), b.Pressed()).Radius(999).Border(1, ui.Transparent).TextColor(white)
	return b.Shadow(0, 4, 12, -4, paletteOf(c).from.Alpha(.6))
}

// check is Lobe's Checkbox over a value kept elsewhere: toggle runs after the
// click has been taken.
func check(c *ui.Context, on bool, label string, size float32, toggle func()) ui.Element {
	return choice.Checkbox(c, &on, label, choice.CheckboxProps{Size: size}).OnClick(toggle)
}

// reveal keeps an element in the layout but shows it only when wanted, as the
// row controls that appear under the pointer. The pointer reaches a hidden one
// only while it is over the row, which reveals it, and Tab reveals the one it
// lands on, so the keyboard never focuses something unseen.
func reveal(e ui.Element, show bool) ui.Element {
	if !show && !e.FocusVisible() {
		e.Opacity(0)
	}
	return e
}

// kbd draws one keycap per space-separated key, as "⌘ K", in the look of
// Lobe's Hotkey; it takes the keys as written rather than as a binding, for
// the shortcut list, whose rows also name gestures.
func (a *app) kbd(c *ui.Context, keys string) {
	if keys == "" {
		return
	}
	ui.Row(c).Gap(3).Children(func() {
		for n, key := range strings.Fields(keys) {
			ui.Text(c.Key(n), key).FontSize(11).MinWidth(20).Padding(3, 6).Radius(a.tk.BorderRadiusXS).TextAlign(ui.Center).
				Background(a.tk.FillTertiary).TextColor(a.tk.TextSecondary)
		}
	})
}

// dialogChrome gives a dialog the surface of Lobe's Modal.
func (a *app) dialogChrome(backdrop, panel ui.Element) {
	backdrop.Background(a.tk.BgLayout.Alpha(.6))
	panel.Radius(24).Background(a.tk.BgElevated).TextColor(a.tk.Text)
	a.tk.Elevate(panel, theme.BoxShadow)
	a.appear(panel, motion.Modal())
}

// popoverChrome gives a menu the surface of Lobe's DropdownMenu.
func (a *app) popoverChrome(panel ui.Element) {
	a.popoverSurface(panel)
	a.appear(panel, motion.Popup())
}

// popoverSurface is the look of a menu without its arrival, for a panel that
// hops between anchors, as the issue preview does while stepping.
func (a *app) popoverSurface(panel ui.Element) {
	panel.Margin(6, 0, 0, 0).Padding(6).Radius(16).Background(a.tk.BgElevated).TextColor(a.tk.Text)
	a.tk.Elevate(panel, theme.BoxShadowSecondary)
}

// appear eases a floating panel in as Lobe's own overlays arrive. Only its
// arrival moves: once open it follows its content at once, so a menu that
// shrinks while its search is typed in never lags behind. A view without a
// window, as under the tester, shows panels at once, so that a frame taken
// right after opening one already holds it.
func (a *app) appear(panel ui.Element, preset ui.ElementTransition) {
	if a.window == nil && !a.motion {
		return
	}
	panel.Transition(ui.ElementTransition{Colors: true, Duration: preset.Duration, Ease: preset.Ease, Enter: preset.Enter})
}

// dialogTitle heads a dialog as Lobe's Modal does, with its close button.
func (a *app) dialogTitle(c *ui.Context, title string, close func()) {
	ui.Row(c).MinHeight(32).Gap(8).Children(func() {
		ui.Text(c, title).FontWeight(600).Grow(1)
		if close != nil {
			iconButton(c, "close", a.tr("关闭", "Close")).OnClick(close)
		}
	})
}

// dialogFooter is a dialog's row of actions, aligned to the end.
func dialogFooter(c *ui.Context, actions func()) {
	ui.Row(c).Gap(8).Margin(4, 0, 0, 0).Justify(ui.End).Children(actions)
}

// sectionHeader titles a block of a detail page; actions builds its
// right-aligned controls.
func (a *app) sectionHeader(c *ui.Context, title, count string, actions func()) {
	ui.Row(c).Height(28).Gap(8).Children(func() {
		ui.Text(c, title).FontSize(13).FontWeight(500).TextColor(a.tk.TextSecondary)
		if count != "" {
			ui.Text(c, count).FontSize(12).TextColor(a.tk.TextQuaternary)
		}
		ui.Spacer(c)
		if actions != nil {
			actions()
		}
	})
}

// divider is Lobe's hairline between blocks of a page.
func (a *app) divider(c *ui.Context) ui.Element {
	return layout.Divider(c, layout.DividerProps{})
}

// card is the surface of a record laid on a tile: white, rounded, and set
// off from the tile by its color alone, as the kit's are.
func (a *app) card(e ui.Element) ui.Element {
	return a.lift(e.Radius(a.tk.BorderRadiusLG).Background(a.tk.BgContainer))
}

// recordCard is the box of one record in a list: a white card where the list
// lies on a tile, a bordered one inside a card that is white already.
func (a *app) recordCard(e ui.Element, onTile bool) ui.Element {
	if onTile {
		return a.lift(e.Radius(16).Background(a.tk.BgContainer))
	}
	return e.Radius(14).Border(1, a.tk.BorderSecondary)
}

// lift gives a white control on a tile the kit's barely-there shadow.
func (a *app) lift(e ui.Element) ui.Element {
	if a.tk.Dark {
		return e.Shadow(0, 1, 2, 0, black.Alpha(.3))
	}
	return e.Shadow(0, 1, 3, 0, black.Alpha(.05)).Shadow(0, 0, 0, 1, black.Alpha(.025))
}

// labelPill shows a label as a neutral Lobe tag led by the label's color, which
// keeps a row of them calm where filled tags of every hue would shout.
func (a *app) labelPill(c *ui.Context, color, name string) {
	ui.Row(c).Height(22).Padding(0, 8).Gap(6).Radius(999).Background(a.tk.FillTertiary).Shrink(0).Children(func() {
		ui.Box(c).Size(7, 7).Radius(4).Background(ui.Hex(color))
		ui.Text(c, name).FontSize(12).SingleLine().TextColor(a.tk.TextSecondary)
	})
}

// empty fills a page that has nothing to show: Lobe's Empty, with one action.
func (a *app) empty(c *ui.Context, mark, title, label string, act func()) {
	ui.Column(c).Grow(1).Center().Children(func() {
		p := display.EmptyProps{Icon: glyph(mark), Title: title}
		if act != nil {
			p.Action = func() { btn(c, label).OnClick(act) }
		}
		display.Empty(c, p)
	})
}

func (a *app) shortDate(iso string) string {
	date, err := time.ParseInLocation("2006-01-02", iso, time.Local)
	if err != nil {
		return iso
	}
	return a.shortDay(date)
}

func (a *app) shortTime(ms int64) string { return a.shortDay(time.UnixMilli(ms)) }

func (a *app) shortDay(date time.Time) string {
	if a.data.Preferences.Locale != "en" {
		if date.Year() != time.Now().Year() {
			return fmt.Sprintf("%d年%d月%d日", date.Year(), int(date.Month()), date.Day())
		}
		return fmt.Sprintf("%d月%d日", int(date.Month()), date.Day())
	}
	if date.Year() != time.Now().Year() {
		return date.Format("Jan 2, 2006")
	}
	return date.Format("Jan 2")
}

// dueColor warns about dates that need attention on work that is still open.
func (a *app) dueColor(iso, status string) ui.Color {
	if status == "done" || status == "canceled" || status == "duplicate" {
		return a.tk.TextTertiary
	}
	date, err := time.ParseInLocation("2006-01-02", iso, time.Local)
	if err != nil {
		return a.tk.TextTertiary
	}
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	switch {
	case date.Before(today):
		return a.tk.Error
	case date.Before(today.AddDate(0, 0, 2)):
		return a.tk.Gold
	}
	return a.tk.TextTertiary
}
