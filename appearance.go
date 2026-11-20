package main

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/theme"
)

func validAppearance(mode string) bool { return mode == "dark" || mode == "light" || mode == "system" }

func (a *app) appearanceMode() string {
	if validAppearance(a.appearance) {
		return a.appearance
	}
	return "dark" // Preserve the appearance of existing installations.
}

// Flow's look is a bento kit: a pale page, tiles one step darker laid on it,
// white controls and records on the tiles, near-black ink, and violet for what
// is chosen, done or to be pressed, filled as a gradient where it is solid.
// Lobe's components are given the same palette, so theirs match Flow's own.
type palette struct {
	page, tile, surface, raised ui.Color // the page, a tile on it, a control on a tile, a menu over it all
	from, to                    ui.Color // the accent's gradient, left to right
}

var (
	lightPalette = palette{
		page: ui.Hex("#f9f9f7"), tile: ui.Hex("#f1f1ee"), surface: ui.Hex("#ffffff"), raised: ui.Hex("#ffffff"),
		from: ui.Hex("#5f3ff0"), to: ui.Hex("#9a6bff"),
	}
	darkPalette = palette{
		page: ui.Hex("#0c0c0e"), tile: ui.Hex("#161618"), surface: ui.Hex("#1f1f22"), raised: ui.Hex("#252529"),
		from: ui.Hex("#5f3ff0"), to: ui.Hex("#9a6bff"),
	}
)

func init() { theme.Customize(lightTokens, darkTokens) }

func lightTokens(t theme.Tokens) theme.Tokens {
	p := lightPalette
	t.BgLayout, t.BgBase, t.BgContainer, t.BgElevated = p.page, p.surface, p.surface, p.raised
	t.BgContainerSecondary, t.BgSpotlight = p.tile, ui.Hex("#1b1b1d")
	t.Text, t.TextHeading, t.TextBase = ui.Hex("#161616"), ui.Hex("#161616"), ui.Hex("#000000")
	t.TextSecondary, t.TextLabel = ui.Hex("#5e5e5a"), ui.Hex("#5e5e5a")
	t.TextTertiary, t.TextDescription, t.Icon = ui.Hex("#8a8a85"), ui.Hex("#8a8a85"), ui.Hex("#8a8a85")
	t.TextQuaternary, t.TextPlaceholder, t.TextDisabled = ui.Hex("#b3b3ae"), ui.Hex("#a9a9a4"), ui.Hex("#b3b3ae")
	t.Border, t.BorderSecondary, t.BorderBg = ui.Hex("#e3e3de"), ui.Hex("#ebebe7"), p.surface
	accent(&t, ui.Hex("#7352f5"), ui.Hex("#8468ff"), ui.Hex("#6343e6"), ui.Hex("#f4f1ff"), ui.Hex("#ebe5ff"), ui.Hex("#cdbfff"))
	t.BorderRadius, t.BorderRadiusLG, t.BorderRadiusSM, t.BorderRadiusXS = 10, 18, 8, 6
	return t
}

func darkTokens(t theme.Tokens) theme.Tokens {
	p := darkPalette
	t.BgLayout, t.BgBase, t.BgContainer, t.BgElevated = p.page, p.page, p.surface, p.raised
	t.BgContainerSecondary, t.BgSpotlight = p.tile, ui.Hex("#2c2c31")
	t.Text, t.TextHeading, t.TextBase = ui.Hex("#ededea"), ui.Hex("#ededea"), ui.Hex("#ffffff")
	t.TextSecondary, t.TextLabel = ui.Hex("#aaaaa5"), ui.Hex("#b5b5b0")
	t.TextTertiary, t.TextDescription, t.Icon = ui.Hex("#7f7f7b"), ui.Hex("#7f7f7b"), ui.Hex("#8a8a86")
	t.TextQuaternary, t.TextPlaceholder, t.TextDisabled = ui.Hex("#5b5b58"), ui.Hex("#66666a"), ui.Hex("#5b5b58")
	t.Border, t.BorderSecondary, t.BorderBg = ui.Hex("#2e2e33"), ui.Hex("#26262a"), p.surface
	accent(&t, ui.Hex("#9a80ff"), ui.Hex("#ad97ff"), ui.Hex("#876bf5"), ui.Hex("#1e1830"), ui.Hex("#272040"), ui.Hex("#4d3f8a"))
	t.BorderRadius, t.BorderRadiusLG, t.BorderRadiusSM, t.BorderRadiusXS = 10, 18, 8, 6
	return t
}

// accent makes violet the primary color, and the color of focus and links, in
// every shade Lobe asks for.
func accent(t *theme.Tokens, base, hover, active, bg, bgHover, border ui.Color) {
	t.Primary, t.PrimaryHover, t.PrimaryActive = base, hover, active
	t.PrimaryBg, t.PrimaryBgHover, t.PrimaryBorder, t.PrimaryBorderHover = bg, bgHover, border, theme.Mix(border, bg, .4)
	t.PrimaryFill, t.PrimaryFillSecondary, t.PrimaryFillTertiary, t.PrimaryFillQuaternary = base.Alpha(.36), base.Alpha(.22), base.Alpha(.14), base.Alpha(.07)
	t.PrimaryText, t.PrimaryTextHover, t.PrimaryTextActive = active, base, active
	t.Info, t.InfoHover, t.InfoActive, t.InfoText, t.InfoTextHover, t.InfoTextActive = base, hover, active, active, base, active
	t.InfoBg, t.InfoBgHover, t.InfoBorder, t.InfoBorderHover = bg, bgHover, border, theme.Mix(border, bg, .4)
	t.InfoFill, t.InfoFillSecondary, t.InfoFillTertiary, t.InfoFillQuaternary = base.Alpha(.16), base.Alpha(.11), base.Alpha(.08), base.Alpha(.03)
	t.Link, t.LinkHover, t.LinkActive = active, base, active
}

// flowTheme is Lobe UI's theme for the appearance asked for: its tokens, and
// the MyGo theme made from them, which the framework's own widgets follow.
func flowTheme(dark bool) (*theme.Tokens, *ui.Theme) {
	tk := theme.Light()
	if dark {
		tk = theme.Dark()
	}
	t := tk.Theme()
	// Tooltips of MyGo's own sit on Lobe's raised surface, as Lobe's do; text
	// on the accent is white in either appearance.
	t.Inverse, t.InverseText, t.AccentText = tk.BgElevated, tk.TextLabel, ui.Hex("#ffffff")
	return tk, t
}

// pal is the palette of the appearance shown.
func (a *app) pal() palette {
	if a.tk.Dark {
		return darkPalette
	}
	return lightPalette
}

func (a *app) themeFrame(c *ui.Context) {
	dark := a.appearanceMode() == "dark" || a.appearanceMode() == "system" && c.Theme().Dark
	if a.theme == nil || a.tk == nil || a.theme.Dark != dark {
		a.tk, a.theme = flowTheme(dark)
		a.seamless = nil
	}
	if a.seamless == nil {
		// The divider beside the sidebar shows only under the pointer, in the
		// accent: the kit parts its tiles with space, not lines.
		t := *a.theme
		t.Border = ui.Transparent
		a.seamless = &t
	}
	c.SetTheme(a.theme)
}

func (a *app) tone(dark, light string) ui.Color {
	if a.theme.Dark {
		return ui.Hex(dark)
	}
	return ui.Hex(light)
}

func (a *app) applyNativeAppearance() {
	if a.window != nil {
		mygo.Theme.SetSource(mygo.ThemeSource(a.appearanceMode()))
	}
}

func (a *app) setAppearance(mode string) {
	if !validAppearance(mode) || mode == a.appearanceMode() {
		return
	}
	if a.desktop != nil {
		a.desktop.setAppearance(mode)
		return
	}
	a.appearance = mode
	a.applyNativeAppearance()
	a.persistDesktopSession()
}

func (a *app) appearanceSettings(c *ui.Context) {
	a.settingRow(c, a.tr("外观", "Appearance"), func() {
		modes := []string{"system", "light", "dark"}
		current := 0
		for n, mode := range modes {
			if mode == a.appearanceMode() {
				current = n
			}
		}
		a.segmented(c, "appearance", []string{a.tr("跟随系统", "System"), a.tr("浅色", "Light"), a.tr("深色", "Dark")}, current, false, func(n int) { a.setAppearance(modes[n]) })
	})
}
