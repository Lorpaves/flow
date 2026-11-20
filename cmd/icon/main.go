// Command icon renders Flow's mark into the app icon: three bars stepping
// forward on a dark tile, as work moves along a timeline. The app draws the
// same mark at small sizes (brand, in ui_shell.go); keep the two in step.
package main

import (
	"image"
	"image/png"
	"log"
	"os"

	"github.com/egoist/mygo/ui"
)

// The tile follows the macOS icon grid: 824 points of a 1024 canvas. The
// margin around it has to be transparent: macOS cuts an opaque canvas to its
// own icon shape, margin included, which frames the tile in the margin's
// color.
const (
	canvas = 1024
	inset  = 100
	tile   = canvas - 2*inset
)

// bars are the mark, measured inside the tile: left, top, width and the two
// ends of each bar's gradient.
var bars = []struct {
	x, y, w  float32
	from, to string
}{
	{174, 232, 312, "#ddd6fe", "#a78bfa"},
	{274, 364, 376, "#c7d2fe", "#818cf8"},
	{374, 496, 232, "#bae6fd", "#38bdf8"},
}

func main() {
	img := cutout(func(c *ui.Context) {
		ui.Box(c).Fill().Padding(inset).Children(func() {
			ui.Box(c).Size(tile, tile).Radius(186).Gradient(ui.Hex("#24242e"), ui.Hex("#0b0b0f"), 180).
				InsetShadow(0, 0, 0, 3, ui.RGBA(255, 255, 255, 0.07)).
				InsetShadow(0, 6, 10, 0, ui.RGBA(255, 255, 255, 0.06)).
				Children(func() {
					for n, b := range bars {
						ui.Box(c.Key(n)).Absolute().Left(b.x).Top(b.y).Size(b.w, 96).Radius(48).
							Gradient(ui.Hex(b.from), ui.Hex(b.to), 90).
							Shadow(0, 10, 28, 0, ui.Hex(b.to).Alpha(0.28))
					}
				})
		})
	})
	if err := os.MkdirAll("resources", 0755); err != nil {
		log.Fatal(err)
	}
	f, err := os.Create("resources/icon.png")
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	if err = png.Encode(f, img); err != nil {
		log.Fatal(err)
	}
}

// cutout renders a drawing on a transparent canvas. A frame is always painted
// over an opaque window, so the drawing is rendered twice, over black and
// over white: what the two frames agree on is the drawing, and where they
// differ the backdrop shows through by that much.
func cutout(draw func(c *ui.Context)) image.Image {
	over := func(backdrop ui.Color) *image.RGBA {
		return ui.Render(func(c *ui.Context) {
			ui.Box(c).Fill().Background(backdrop).Children(func() { draw(c) })
		}, canvas, canvas, 1)
	}
	black, white := over(ui.Hex("#000000")), over(ui.Hex("#ffffff"))
	out := image.NewRGBA(black.Bounds())
	for i := 0; i < len(out.Pix); i += 4 {
		// Over black a pixel is the drawing's color times its alpha, which is
		// how image.RGBA stores it; white adds the uncovered remainder.
		shown := 0
		for ch := 0; ch < 3; ch++ {
			shown += int(white.Pix[i+ch]) - int(black.Pix[i+ch])
		}
		alpha := 255 - max(0, min(255, (shown+1)/3))
		for ch := 0; ch < 3; ch++ {
			out.Pix[i+ch] = uint8(min(int(black.Pix[i+ch]), alpha))
		}
		out.Pix[i+3] = uint8(alpha)
	}
	return out
}
