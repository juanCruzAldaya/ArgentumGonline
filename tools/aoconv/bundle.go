package main

import (
	"encoding/json"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// atlasWidth is the packing width. Character frames top out around 50px a side,
// so this stays comfortably inside every GPU's texture limit while keeping the
// sheet roughly square.
const atlasWidth = 512

// Rect is where a static grh ended up inside the atlas.
type Rect struct {
	X int `json:"x"`
	Y int `json:"y"`
	W int `json:"w"`
	H int `json:"h"`
}

// Anim is an animated grh: the static grhs of its frames, plus AO's speed value.
type Anim struct {
	Frames []int   `json:"frames"`
	Speed  float64 `json:"speed"`
}

// Body is one character body: four facings plus where its artwork actually
// starts. The sprite rectangle is padded, so ContentTop is what a head has to
// line up against.
type Body struct {
	Facings []int `json:"facings"`
	// ContentTop is the first row with any opaque pixel, relative to the frame.
	ContentTop int `json:"top"`
}

// Head is one character head. Heads are drawn in a tall rectangle whose lower
// half is empty, so ContentBottom — not the rectangle — is where the neck is.
type Head struct {
	Facings []int `json:"facings"`
	// ContentBottom is the last row with any opaque pixel, relative to the rect.
	ContentBottom int `json:"bottom"`
}

// Bundle is everything the client needs to draw Argentum characters.
type Bundle struct {
	Atlas string `json:"atlas"`

	// Frames maps a static grh to its rectangle in the atlas.
	Frames map[int]Rect `json:"frames"`
	// Anims maps an animated grh to the static grhs it cycles through.
	Anims map[int]Anim `json:"anims"`

	// Bodies holds four grhs per body, sorted ascending.
	//
	// Cuerpos.ini/.ind label these up/right/down/left, but the labels do not
	// match the sprites: body 1's four grhs carry 6/6/5/5 frames, which pairs
	// them {up,down} and {left,right} rather than alternating. Sorting is the
	// one ordering that is stable across the whole file; which index is which
	// facing is decided client-side, where it can be seen.
	Bodies map[int]Body `json:"bodies"`

	// Heads holds four grhs per head in file order, which for Cabezas.ini is
	// self-consistent: up, right, down, left.
	Heads map[int]Head `json:"heads"`
}

// deriveBodies reconstructs bodies from Graficos.ini instead of from Cuerpos.
//
// Cuerpos.ini and Cuerpos.ind agree with each other but not with the sprites:
// grhs 4581-4592 are twelve consecutive character animations, yet they turn up
// scattered across five body records with facings missing. The index is
// misaligned in a way no record layout recovers.
//
// The graphics table is intact though, and it carries the structure directly: a
// body is four consecutive animated grhs whose frame blocks run back to back —
// 4581 spans frames 2531-2536, 4582 picks up at 2537, and so on. That was
// confirmed by rendering: 4581 is the character seen from behind.
//
// Contiguity alone is not enough: Argentum animates mounts, creatures and even
// items the same way, and a first pass happily packed horses, polar bears and
// staffs as "bodies". Filtering by frame size cut the animals but kept the
// staffs, so the seeds come from Cuerpos instead.
//
// That is the split that works: Cuerpos gets *membership* right — every large
// value in its records really is a body grh — and gets *structure* wrong.
// Graficos is the reverse. Seeding the scan with the numbers Cuerpos mentions,
// then snapping each to its run of four, uses each source for what it is good at.
func deriveBodies(grhs map[int]Grh, seeds []int) map[int][]int {
	lastFrame := func(g Grh) int { return g.Anim[len(g.Anim)-1] }

	// walkCycle reports whether a grh is one facing of a walk: an animation
	// whose own frames are consecutive.
	walkCycle := func(num int) (Grh, bool) {
		g, ok := grhs[num]
		if !ok || !g.Animated() || len(g.Anim) == 0 {
			return Grh{}, false
		}
		return g, lastFrame(g)-g.Anim[0] == len(g.Anim)-1
	}

	seeded := make(map[int]bool, len(seeds))
	for _, seed := range seeds {
		seeded[seed] = true
	}

	numbers := make([]int, 0, len(grhs))
	for num := range grhs {
		numbers = append(numbers, num)
	}
	sort.Ints(numbers)

	// Walk the table gathering maximal chains of animations whose frame blocks
	// run back to back. Bodies do not sit alone: grhs 4581-4592 form one chain
	// of twelve, which is three bodies of four facings — exactly the twelve that
	// Cuerpos scatters across its first five records.
	out := map[int][]int{}
	id := 0
	for i := 0; i < len(numbers); i++ {
		g, ok := walkCycle(numbers[i])
		if !ok {
			continue
		}

		chain := []int{numbers[i]}
		prev := g
		for next := numbers[i] + 1; ; next++ {
			g2, ok := walkCycle(next)
			if !ok || g2.Anim[0] != lastFrame(prev)+1 {
				break
			}
			chain = append(chain, next)
			prev = g2
		}
		i += len(chain) - 1

		// Only chains that Cuerpos vouches for are characters. Without this,
		// horses, polar bears and animated staffs all qualify.
		vouched := false
		for _, num := range chain {
			if seeded[num] {
				vouched = true
				break
			}
		}
		if !vouched {
			continue
		}

		for start := 0; start+4 <= len(chain); start += 4 {
			id++
			out[id] = append([]int(nil), chain[start:start+4]...)
		}
	}
	return out
}

// loadBodySeeds returns every grh-sized number mentioned anywhere in
// Cuerpos.ini. The field a value sits in cannot be trusted, but its presence
// can: these are the numbers Argentum considers to be character bodies.
func loadBodySeeds(path string) ([]int, error) {
	lines, err := iniLines(path)
	if err != nil {
		return nil, err
	}
	var out []int
	for _, line := range lines {
		_, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		// Head offsets are small; grh numbers are not.
		if n := atoi(value); n > 1000 {
			out = append(out, n)
		}
	}
	return out, nil
}

// writeBundle packs every frame the given bodies and heads need into one atlas
// and writes it alongside its JSON index.
func writeBundle(grhs map[int]Grh, heads map[int][4]int, assets string, wantBodies, wantHeads []int, outDir string) error {
	seeds, err := loadBodySeeds(filepath.Join(assets, "INIT", "Cuerpos.ini"))
	if err != nil {
		return err
	}
	bodies := deriveBodies(grhs, seeds)
	fmt.Printf("cuerpos derivados: %d (desde %d semillas de Cuerpos.ini)\n", len(bodies), len(seeds))

	b := Bundle{
		Atlas:  "atlas.png",
		Frames: map[int]Rect{},
		Anims:  map[int]Anim{},
		Bodies: map[int]Body{},
		Heads:  map[int]Head{},
	}

	// needed collects the static grhs to pack. Animations contribute their
	// frames; several bodies can share one, hence the set.
	needed := map[int]bool{}

	collect := func(grhNum int) error {
		g, ok := grhs[grhNum]
		if !ok {
			return fmt.Errorf("grh %d no existe", grhNum)
		}
		if !g.Animated() {
			needed[grhNum] = true
			return nil
		}
		anim := Anim{Frames: g.Anim, Speed: g.Speed}
		b.Anims[grhNum] = anim
		for _, frame := range g.Anim {
			if _, ok := grhs[frame]; !ok {
				return fmt.Errorf("grh %d referencia el frame inexistente %d", grhNum, frame)
			}
			needed[frame] = true
		}
		return nil
	}

	for _, id := range wantBodies {
		facings, ok := bodies[id]
		if !ok {
			return fmt.Errorf("cuerpo %d no existe (hay %d)", id, len(bodies))
		}
		for _, grhNum := range facings {
			if err := collect(grhNum); err != nil {
				return fmt.Errorf("cuerpo %d: %w", id, err)
			}
		}
		b.Bodies[id] = Body{Facings: append([]int(nil), facings...)}
	}

	for _, id := range wantHeads {
		set, ok := heads[id]
		if !ok {
			return fmt.Errorf("cabeza %d no existe", id)
		}
		facings := set[:]
		for _, grhNum := range facings {
			if grhNum == 0 {
				return fmt.Errorf("cabeza %d tiene una dirección vacía", id)
			}
			if err := collect(grhNum); err != nil {
				return fmt.Errorf("cabeza %d: %w", id, err)
			}
		}
		b.Heads[id] = Head{Facings: append([]int(nil), facings...)}
	}

	// Pack tallest first so the shelves stay tight.
	order := make([]int, 0, len(needed))
	for grhNum := range needed {
		order = append(order, grhNum)
	}
	sort.Slice(order, func(i, j int) bool {
		hi, hj := grhs[order[i]].H, grhs[order[j]].H
		if hi != hj {
			return hi > hj
		}
		return order[i] < order[j]
	})

	x, y, shelfHeight := 0, 0, 0
	for _, grhNum := range order {
		g := grhs[grhNum]
		if g.W <= 0 || g.H <= 0 {
			return fmt.Errorf("grh %d tiene rectángulo vacío", grhNum)
		}
		if x+g.W > atlasWidth {
			x = 0
			y += shelfHeight
			shelfHeight = 0
		}
		b.Frames[grhNum] = Rect{X: x, Y: y, W: g.W, H: g.H}
		x += g.W
		if g.H > shelfHeight {
			shelfHeight = g.H
		}
	}
	atlasHeight := y + shelfHeight
	if atlasHeight == 0 {
		return fmt.Errorf("no hay nada que empaquetar")
	}

	atlas := image.NewNRGBA(image.Rect(0, 0, atlasWidth, atlasHeight))
	sheets := map[int]image.Image{}
	for _, grhNum := range order {
		g := grhs[grhNum]
		if _, cached := sheets[g.File]; !cached {
			sheet, err := openSheet(assets, g.File)
			if err != nil {
				return err
			}
			sheets[g.File] = sheet
		}
		src := sheets[g.File]

		srcRect := image.Rect(g.X, g.Y, g.X+g.W, g.Y+g.H)
		if !srcRect.In(src.Bounds()) {
			return fmt.Errorf("grh %d: rect %v se sale de la hoja %d %v", grhNum, srcRect, g.File, src.Bounds())
		}
		dst := b.Frames[grhNum]
		draw.Draw(atlas, image.Rect(dst.X, dst.Y, dst.X+dst.W, dst.Y+dst.H), src, srcRect.Min, draw.Src)
	}

	keyBlackToTransparent(atlas)

	// Measure the artwork only now that transparency is real. The rectangles
	// are padded — a head is drawn in a 17x50 box whose lower 35 rows are
	// empty — so aligning by rectangle puts heads well above the shoulders.
	// Alignment has to key off where the pixels actually are.
	for id, body := range b.Bodies {
		body.ContentTop = contentTop(atlas, b.Frames[firstStaticFrame(grhs, body.Facings[0])])
		b.Bodies[id] = body
	}
	for id, head := range b.Heads {
		head.ContentBottom = contentBottom(atlas, b.Frames[firstStaticFrame(grhs, head.Facings[0])])
		b.Heads[id] = head
	}

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	atlasPath := filepath.Join(outDir, "atlas.png")
	f, err := os.Create(atlasPath)
	if err != nil {
		return err
	}
	if err := png.Encode(f, atlas); err != nil {
		f.Close()
		return err
	}
	f.Close()

	jsonPath := filepath.Join(outDir, "bundle.json")
	data, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(jsonPath, data, 0o644); err != nil {
		return err
	}

	info, _ := os.Stat(atlasPath)
	fmt.Printf("\natlas.png   %dx%d, %d KB, %d frames\n", atlasWidth, atlasHeight, info.Size()/1024, len(b.Frames))
	fmt.Printf("bundle.json %d cuerpos, %d cabezas, %d animaciones\n", len(b.Bodies), len(b.Heads), len(b.Anims))
	return nil
}

// keyBlackToTransparent applies Argentum's colour key. The sprites predate
// alpha channels: pure black is the transparent colour, and anything genuinely
// dark in the art was authored just off black to survive this exact step.
func keyBlackToTransparent(img *image.NRGBA) {
	for i := 0; i < len(img.Pix); i += 4 {
		if img.Pix[i] == 0 && img.Pix[i+1] == 0 && img.Pix[i+2] == 0 {
			img.Pix[i+3] = 0
		}
	}
}

// firstStaticFrame resolves a possibly animated grh down to a drawable one.
func firstStaticFrame(grhs map[int]Grh, num int) int {
	g, ok := grhs[num]
	if ok && g.Animated() && len(g.Anim) > 0 {
		return g.Anim[0]
	}
	return num
}

// contentTop is the first row of the rectangle holding any opaque pixel.
func contentTop(img *image.NRGBA, r Rect) int {
	for y := 0; y < r.H; y++ {
		for x := 0; x < r.W; x++ {
			if img.NRGBAAt(r.X+x, r.Y+y).A != 0 {
				return y
			}
		}
	}
	return 0
}

// contentBottom is the last row of the rectangle holding any opaque pixel.
func contentBottom(img *image.NRGBA, r Rect) int {
	for y := r.H - 1; y >= 0; y-- {
		for x := 0; x < r.W; x++ {
			if img.NRGBAAt(r.X+x, r.Y+y).A != 0 {
				return y
			}
		}
	}
	return r.H - 1
}
