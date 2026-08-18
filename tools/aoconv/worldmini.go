package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
)

// tilePalette answers "what colour is this graphic, seen from very far away".
//
// It exists so a world can be drawn as a map instead of as a floor plan. The
// collision bitset only knows wall from floor, which is two flat colours and
// reads like an architect's drawing; the tile art knows the difference between
// forest, sand, snow, water and cobblestone, and that is what makes a map
// legible at a glance.
//
// This is what Argentum itself ships: 325 BMPs under Graficos/MiniMapa, one per
// map, one pixel per tile. It never draws a minimap from tiles at runtime, and
// neither should we.
type tilePalette struct {
	assets string
	grhs   map[int]Grh
	sheets map[int]image.Image
	broken map[int]bool
	cache  map[int]color.NRGBA
}

func newTilePalette(assets string, grhs map[int]Grh) *tilePalette {
	return &tilePalette{
		assets: assets,
		grhs:   grhs,
		sheets: map[int]image.Image{},
		broken: map[int]bool{},
		cache:  map[int]color.NRGBA{},
	}
}

// colorOf averages a graphic's opaque pixels.
//
// "Opaque" means "not pure black": Argentum's sprites predate alpha and use
// black as the colour key, so averaging it in would drag every tile towards a
// muddy grey in proportion to how much empty space its rectangle has.
func (p *tilePalette) colorOf(grh int) (color.NRGBA, bool) {
	if c, ok := p.cache[grh]; ok {
		return c, c.A != 0
	}

	g, ok := p.grhs[grh]
	if !ok {
		p.cache[grh] = color.NRGBA{}
		return color.NRGBA{}, false
	}
	// An animated tile — water, mostly — is represented by its first frame.
	if g.Animated() {
		if len(g.Anim) == 0 {
			p.cache[grh] = color.NRGBA{}
			return color.NRGBA{}, false
		}
		c, ok := p.colorOf(g.Anim[0])
		p.cache[grh] = c
		return c, ok
	}

	if p.broken[g.File] {
		p.cache[grh] = color.NRGBA{}
		return color.NRGBA{}, false
	}
	sheet, cached := p.sheets[g.File]
	if !cached {
		var err error
		sheet, err = openSheet(p.assets, g.File)
		if err != nil {
			p.broken[g.File] = true
			p.cache[grh] = color.NRGBA{}
			return color.NRGBA{}, false
		}
		p.sheets[g.File] = sheet
	}

	rect := image.Rect(g.X, g.Y, g.X+g.W, g.Y+g.H)
	if !rect.In(sheet.Bounds()) {
		p.cache[grh] = color.NRGBA{}
		return color.NRGBA{}, false
	}

	var sr, sg, sb, n uint64
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		for x := rect.Min.X; x < rect.Max.X; x++ {
			r16, g16, b16, a16 := sheet.At(x, y).RGBA()
			r, gg, b := uint8(r16>>8), uint8(g16>>8), uint8(b16>>8)
			if a16 == 0 || (r == 0 && gg == 0 && b == 0) {
				continue
			}
			sr += uint64(r)
			sg += uint64(gg)
			sb += uint64(b)
			n++
		}
	}
	if n == 0 {
		p.cache[grh] = color.NRGBA{}
		return color.NRGBA{}, false
	}
	c := color.NRGBA{uint8(sr / n), uint8(sg / n), uint8(sb / n), 255}
	p.cache[grh] = c
	return c, true
}

// writeWorldMinimap paints one pixel per tile and writes it beside the world.
//
// Layers are drawn in the order the game draws them, minus the roofs: floor
// first, then whatever stands on it. Layer 4 is deliberately skipped — a roof
// hides the building under it, and on a map you want to see the building.
func writeWorldMinimap(path string, m *AOMap, pal *tilePalette) error {
	img := image.NewNRGBA(image.Rect(0, 0, m.W, m.H))
	missing := 0
	for i := range m.Tiles {
		t := m.Tiles[i]
		var c color.NRGBA
		found := false
		for _, layer := range []int{0, 1, 2} {
			if t.Layers[layer] <= 0 {
				continue
			}
			if lc, ok := pal.colorOf(t.Layers[layer]); ok {
				c, found = lc, true
			}
		}
		if !found {
			missing++
			c = color.NRGBA{20, 20, 24, 255}
		}
		img.SetNRGBA(i%m.W, i/m.W, c)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		return err
	}
	if missing > 0 {
		fmt.Printf("              %d tiles sin gráfico legible en el minimapa\n", missing)
	}
	return nil
}
