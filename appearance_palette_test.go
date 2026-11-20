package main

import (
	"math"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/theme"
)

// Lobe's components read the palette Flow draws with, so a Primary button, a
// checkbox or a switch is violet as Flow's own marks are, in either appearance.
func TestLobeComponentsShareFlowsPalette(t *testing.T) {
	for _, dark := range []bool{false, true} {
		tk, th := flowTheme(dark)
		lobe, p := theme.Light(), lightPalette
		if dark {
			lobe, p = theme.Dark(), darkPalette
		}
		if *lobe != *tk {
			t.Fatal("components read another palette", dark)
		}
		if tk.BgLayout != p.page || tk.BgContainer != p.surface || tk.BgContainerSecondary != p.tile || th.Background != p.page {
			t.Fatal("surfaces", dark, tk.BgLayout, tk.BgContainer)
		}
		if tk.Primary.B < 200 || tk.Primary.R > tk.Primary.B || th.Focus.A == 0 || th.AccentText != white {
			t.Fatal("the accent is not violet", dark, tk.Primary)
		}
		// White text stays readable across the accent's gradient (WCAG AA
		// for normal text) where a button's label sits: its middle.
		if r := contrast(white, theme.Mix(p.from, p.to, .5)); r < 4.5 {
			t.Fatalf("white on the accent is %.2f:1 (dark %v)", r, dark)
		}
		if r := contrast(white, tk.Primary); !dark && r < 4.5 {
			t.Fatalf("white on the primary color is %.2f:1", r)
		}
		// Ink on a white card and on the tile stays at AA too.
		for _, bg := range []ui.Color{p.surface, p.tile, p.page} {
			if r := contrast(tk.Text, bg); r < 7 {
				t.Fatalf("text on %v is %.2f:1 (dark %v)", bg, r, dark)
			}
			if r := contrast(tk.TextSecondary, bg); r < 4.5 {
				t.Fatalf("secondary text on %v is %.2f:1 (dark %v)", bg, r, dark)
			}
		}
	}
}

func contrast(a, b ui.Color) float64 {
	l := func(c ui.Color) float64 {
		ch := func(v uint8) float64 {
			s := float64(v) / 255
			if s <= .03928 {
				return s / 12.92
			}
			return math.Pow((s+.055)/1.055, 2.4)
		}
		return .2126*ch(c.R) + .7152*ch(c.G) + .0722*ch(c.B)
	}
	x, y := l(a), l(b)
	return (max(x, y) + .05) / (min(x, y) + .05)
}
