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

// atlasWidth is the packing width.
//
// This was 1024 until spell FX joined the bundle: the shelf packer sorts every
// needed frame tallest-first and stacks shelves top to bottom, and 50 FX
// animations' worth of explosion- and aura-sized frames pushed the height to
// 19168px at that width — past 16384, the GL_MAX_TEXTURE_SIZE most GPUs cap
// 2D textures at. Godot does not error on an over-limit texture; it silently
// samples the wrong pixels for anything below the cutoff, which is what
// "todo se rompió" looked like: every sprite in the game, not just FX, came
// out wrong. 2048 keeps the height comfortably clear of that ceiling again.
const atlasWidth = 2048

// The atlas pages, by role. Page 0 is everything that moves — characters, the
// gear they wear, spell effects, item icons — and page 1 is world tiles. They
// split cleanly because nothing is on both, and separately they leave room the
// single page did not.
const (
	pageChars = 0
	pageTiles = 1
)

// pageFiles names each page's PNG. Index is Rect.P.
var pageFiles = [...]string{"atlas.png", "atlas_tiles.png"}

// maxTextureSize is GL_MAX_TEXTURE_SIZE on the overwhelming majority of
// GPUs. Nothing enforces it for us: Godot imports an oversized texture
// without complaint and then samples it wrong, so the check lives here.
const maxTextureSize = 16384

// Rect is where a static grh ended up inside the atlas.
//
// P is which atlas page it landed on, and is omitted for page 0 so the common
// case stays out of the JSON — see Bundle.Pages.
type Rect struct {
	X int `json:"x"`
	Y int `json:"y"`
	W int `json:"w"`
	H int `json:"h"`
	P int `json:"p,omitempty"`
}

// Anim is an animated grh: the static grhs of its frames, plus AO's speed value.
type Anim struct {
	Frames []int   `json:"frames"`
	Speed  float64 `json:"speed"`
}

// Body is one character body: the four walk animations in heading order, plus
// the offset the head is drawn at relative to the body's own position.
//
// HeadOffset is real data out of Personajes.ini, not a measurement. The client
// draws the head at the body position plus this, so it varies per body — the
// tall races carry -4 on Y, Gnomo and Enano +6 — and no amount of inspecting
// pixels recovers it, which is what the old ContentTop/ContentBottom pair was
// trying and failing to do.
type Body struct {
	Facings    []int  `json:"facings"`
	HeadOffset [2]int `json:"headOffset"`
}

// Head is one character head, four facings in heading order. Static: the real
// client passes Animate=0 for heads and helmets, so they never cycle.
type Head struct {
	Facings []int `json:"facings"`
}

// Gear is one weapon, shield or helmet animation set — four facings, indexed
// by the same heading order as everything else.
//
// Weapons and shields are full-body overlay sprites drawn at the character's
// own position with no offset, not attachments anchored to a hand. Helmets are
// the exception: they follow the head, offset by the body's HeadOffset plus
// OFFSET_HEAD.
type Gear struct {
	Facings []int `json:"facings"`
}

// Bundle is everything the client needs to draw Argentum characters.
type Bundle struct {
	// Atlas is page 0's filename, kept as its own field so a bundle stays
	// readable by anything that only ever knew about one atlas.
	Atlas string `json:"atlas"`

	// Pages is every atlas image, indexed by Rect.P.
	//
	// The split exists because of a hard ceiling, not tidiness: one texture
	// cannot exceed GL_MAX_TEXTURE_SIZE (16384 on most GPUs) and Godot does
	// not error when it does — it silently samples the wrong pixels for every
	// sprite in the game (see DIFICULTADES §12). Four composed worlds' tiles
	// plus the characters come to 15296px on a single page, which works and
	// leaves no room at all. Split by role, tiles land on their own page and
	// both come in under 55%.
	Pages []string `json:"pages"`

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

	// Worn equipment, keyed by the anim index obj.dat gives each item. Three
	// separate tables from three separate files, and the numbering does NOT
	// carry across them: weapon 4, shield 4 and helmet 4 are unrelated.
	Weapons map[int]Gear `json:"weapons"`
	Shields map[int]Gear `json:"shields"`
	Helmets map[int]Gear `json:"helmets"`

	// Fxs holds every spell effect animation, keyed by the 1-based index
	// Hechizos.dat's FXgrh field names. The client plays these anchored to
	// whoever the spell targets, offset by OffsetX/OffsetY, the same way the
	// original draws them.
	Fxs map[int]Fx `json:"fxs"`
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
func writeBundle(grhs map[int]Grh, bodies map[int]BodyIndex, heads, weapons, shields, helmets map[int][4]int, assets string, wantBodies, wantHeads []int, outDir, overrideDir string, aoMap *AOMap, mapDisplayName string, tileGrhs map[int]bool, items map[int]Item, fxs map[int]Fx) error {
	b := Bundle{
		Atlas:   "atlas.png",
		Frames:  map[int]Rect{},
		Anims:   map[int]Anim{},
		Bodies:  map[int]Body{},
		Heads:   map[int]Head{},
		Weapons: map[int]Gear{},
		Shields: map[int]Gear{},
		Helmets: map[int]Gear{},
		Fxs:     map[int]Fx{},
	}

	// needed collects the static grhs to pack, and page records which atlas
	// each one goes on. Animations contribute their frames; several bodies can
	// share one, hence the set.
	//
	// A grh asked for by two different callers keeps the lower page number, so
	// anything a character needs stays on page 0 even if a map floor happens to
	// use the same graphic. Measured on the current content that never happens
	// — the overlap between world tiles and character frames is exactly zero —
	// but the rule costs nothing and makes the split independent of that.
	needed := map[int]bool{}
	page := map[int]int{}

	collect := func(grhNum, onPage int) error {
		g, ok := grhs[grhNum]
		if !ok {
			return fmt.Errorf("grh %d no existe", grhNum)
		}
		mark := func(frame int) {
			needed[frame] = true
			if p, seen := page[frame]; !seen || onPage < p {
				page[frame] = onPage
			}
		}
		if !g.Animated() {
			mark(grhNum)
			return nil
		}
		anim := Anim{Frames: g.Anim, Speed: g.Speed}
		b.Anims[grhNum] = anim
		for _, frame := range g.Anim {
			if _, ok := grhs[frame]; !ok {
				return fmt.Errorf("grh %d referencia el frame inexistente %d", grhNum, frame)
			}
			mark(frame)
		}
		return nil
	}

	skippedBodies := 0
	for _, id := range wantBodies {
		body, ok := bodies[id]
		if !ok {
			// obj.dat points a few armours at bodies past the end of
			// Personajes.ini (5771 against 692 records). The real client
			// tolerates this — Char_SetBody falls back to body 0 rather than
			// failing — so a stale NumRopaje must not take the whole build
			// down. The server falls back the same way; see appearance.go.
			skippedBodies++
			continue
		}
		usable := true
		for _, grhNum := range body.Facings {
			if grhNum == 0 {
				// Personajes.ini has a handful of records with a hole in one
				// direction. A body that cannot face all four ways would draw
				// nothing when turned that way, so skip it here and let the
				// server fall back rather than ship a character that vanishes
				// when it walks east.
				usable = false
				break
			}
			if err := collect(grhNum, pageChars); err != nil {
				usable = false
				break
			}
		}
		if !usable {
			skippedBodies++
			continue
		}
		b.Bodies[id] = Body{
			Facings:    append([]int(nil), body.Facings[:]...),
			HeadOffset: body.HeadOffset,
		}
	}
	fmt.Printf("cuerpos: %d empaquetados, %d omitidos (inexistentes o con una dirección vacía)\n",
		len(b.Bodies), skippedBodies)

	// Worn equipment. Anim index 2 is deliberately absent from all three
	// tables — it is the source's "nothing equipped" sentinel (NingunArma,
	// NingunEscudo and NingunCasco are all 2, not 0), and Escudos.dat even
	// says so in a comment on its first line. Anything else that fails to
	// resolve is dropped rather than fatal, the same as a body with a hole.
	gear := func(name string, table map[int][4]int, want map[int]bool, into map[int]Gear) {
		for id := range want {
			set, ok := table[id]
			if !ok {
				continue
			}
			usable := true
			for _, grhNum := range set {
				if grhNum == 0 {
					usable = false
					break
				}
				if err := collect(grhNum, pageChars); err != nil {
					usable = false
					break
				}
			}
			if usable {
				into[id] = Gear{Facings: append([]int(nil), set[:]...)}
			}
		}
		fmt.Printf("%s: %d de %d pedidos\n", name, len(into), len(want))
	}

	wantWeapons, wantShields, wantHelmets := map[int]bool{}, map[int]bool{}, map[int]bool{}
	for _, item := range items {
		switch item.Type {
		case ObjWeapon:
			if item.Anim > 0 {
				wantWeapons[item.Anim] = true
			}
			if item.DwarfAnim > 0 {
				wantWeapons[item.DwarfAnim] = true
			}
		case ObjShield:
			if item.Anim > 0 {
				wantShields[item.Anim] = true
			}
		case ObjHelmet:
			if item.Anim > 0 {
				wantHelmets[item.Anim] = true
			}
		}
	}
	gear("armas", weapons, wantWeapons, b.Weapons)
	gear("escudos", shields, wantShields, b.Shields)
	gear("cascos", helmets, wantHelmets, b.Helmets)

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
			if err := collect(grhNum, pageChars); err != nil {
				return fmt.Errorf("cabeza %d: %w", id, err)
			}
		}
		b.Heads[id] = Head{Facings: append([]int(nil), facings...)}
	}
	_ = assets

	if len(items) > 0 {
		// Every carriable item contributes its inventory icon, so slots can
		// show the real thing instead of an empty square.
		missing := 0
		for _, item := range items {
			if err := collect(item.Grh, pageChars); err != nil {
				missing++
			}
		}
		fmt.Printf("items: %d iconos", len(items))
		if missing > 0 {
			fmt.Printf(", %d sin gráfico (se omiten)", missing)
		}
		fmt.Println()
	}

	if len(fxs) > 0 {
		// Every spell effect ships regardless of which spells the map's loot
		// table actually offers — 50 animations is cheap next to the atlas a
		// map already contributes, and it means a rebalanced spell list never
		// needs a bundle rebuild for its FX to work.
		missing := 0
		for id, fx := range fxs {
			if fx.Grh == 0 {
				missing++
				continue
			}
			if err := collect(fx.Grh, pageChars); err != nil {
				missing++
				continue
			}
			b.Fxs[id] = fx
		}
		fmt.Printf("fx:           %d empaquetados", len(b.Fxs))
		if missing > 0 {
			fmt.Printf(", %d omitidos (sin gráfico)", missing)
		}
		fmt.Println()
	}

	if len(tileGrhs) > 0 {
		// A tile referencing a graphic that is not in the index is a data bug in
		// a twenty year old map, not a reason to refuse the whole conversion.
		missing := 0
		for grh := range tileGrhs {
			if err := collect(grh, pageTiles); err != nil {
				missing++
			}
		}
		fmt.Printf("tiles: %d grh distintos", len(tileGrhs))
		if missing > 0 {
			fmt.Printf(", %d sin entrada en el índice (se omiten)", missing)
		}
		fmt.Println()
	}

	// Open every sheet up front and drop whatever cannot be decoded.
	//
	// 83 of ao-libre's 3147 PNGs have a corrupted signature — an extra 0x0d
	// before the trailing 0x0a, the signature of a line-ending conversion
	// applied to a binary file somewhere in that repository's history. The
	// damage is lossy and cannot be reliably undone, so those graphics are
	// simply unavailable. Two percent of bad upstream data is not a reason to
	// refuse to build anything.
	sheets := map[int]image.Image{}
	broken := map[int]bool{}
	order := make([]int, 0, len(needed))
	for grhNum := range needed {
		file := grhs[grhNum].File
		if broken[file] {
			continue
		}
		if _, cached := sheets[file]; !cached {
			sheet, err := openSheet(assets, file)
			if err != nil {
				broken[file] = true
				continue
			}
			sheets[file] = sheet
		}
		order = append(order, grhNum)
	}
	if len(broken) > 0 {
		fmt.Printf("hojas ilegibles y omitidas: %d\n", len(broken))
	}

	// Pack tallest first so the shelves stay tight.
	sort.Slice(order, func(i, j int) bool {
		hi, hj := grhs[order[i]].H, grhs[order[j]].H
		if hi != hj {
			return hi > hj
		}
		return order[i] < order[j]
	})

	// Pack and paint one page at a time. Each is an independent shelf pack, so
	// a page's height depends only on what is on it.
	atlases := make([]*image.NRGBA, len(pageFiles))
	for pageNum := range pageFiles {
		onPage := make([]int, 0, len(order))
		for _, grhNum := range order {
			if page[grhNum] == pageNum {
				onPage = append(onPage, grhNum)
			}
		}
		if len(onPage) == 0 {
			continue
		}

		x, y, shelfHeight := 0, 0, 0
		for _, grhNum := range onPage {
			g := grhs[grhNum]
			if g.W <= 0 || g.H <= 0 {
				return fmt.Errorf("grh %d tiene rectángulo vacío", grhNum)
			}
			if x+g.W > atlasWidth {
				x = 0
				y += shelfHeight
				shelfHeight = 0
			}
			b.Frames[grhNum] = Rect{X: x, Y: y, W: g.W, H: g.H, P: pageNum}
			x += g.W
			if g.H > shelfHeight {
				shelfHeight = g.H
			}
		}
		height := y + shelfHeight

		// Godot does not error above GL_MAX_TEXTURE_SIZE, it just samples the
		// wrong pixels for every sprite at once. Refusing here turns a silent
		// visual catastrophe into a build failure that names the cause.
		if height > maxTextureSize {
			return fmt.Errorf("la página %d (%s) quedó en %dpx de alto, por encima del límite de %d de la GPU — hay que dividirla",
				pageNum, pageFiles[pageNum], height, maxTextureSize)
		}

		atlas := image.NewNRGBA(image.Rect(0, 0, atlasWidth, height))
		for _, grhNum := range onPage {
			g := grhs[grhNum]
			src := sheets[g.File]
			srcRect := image.Rect(g.X, g.Y, g.X+g.W, g.Y+g.H)
			if !srcRect.In(src.Bounds()) {
				// An index entry pointing outside its own sheet is another piece
				// of twenty year old data rot. Drop the frame, not the build.
				delete(b.Frames, grhNum)
				continue
			}
			dst := b.Frames[grhNum]
			draw.Draw(atlas, image.Rect(dst.X, dst.Y, dst.X+dst.W, dst.Y+dst.H), src, srcRect.Min, draw.Src)
		}
		atlases[pageNum] = atlas
	}

	for _, atlas := range atlases {
		if atlas != nil {
			keyBlackToTransparent(atlas)
		}
	}

	// After the colour key, not before: the replacement art already carries a
	// real alpha channel, and running Argentum's black-is-transparent rule over
	// it would punch holes in whatever it draws in near-black.
	// Overrides are character and effect art, so they land on page 0.
	if atlases[pageChars] != nil {
		if err := applyOverrides(atlases[pageChars], &b, overrideDir); err != nil {
			return err
		}
	}

	// Head alignment used to be measured here, by finding the topmost opaque
	// row of a body and the bottommost of a head. That was a workaround for
	// reading head offsets out of Cuerpos.ini, where the field holds grh
	// numbers rather than pixels. Personajes.ini carries the real offsets, so
	// the measurement is gone: the client now places heads the way the
	// original does, at the body's position plus the body's own HeadOffset.

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	for pageNum, atlas := range atlases {
		if atlas == nil {
			continue
		}
		b.Pages = append(b.Pages, pageFiles[pageNum])
		f, err := os.Create(filepath.Join(outDir, pageFiles[pageNum]))
		if err != nil {
			return err
		}
		if err := png.Encode(f, atlas); err != nil {
			f.Close()
			return err
		}
		f.Close()
	}

	jsonPath := filepath.Join(outDir, "bundle.json")
	data, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(jsonPath, data, 0o644); err != nil {
		return err
	}

	fmt.Println()
	for pageNum, atlas := range atlases {
		if atlas == nil {
			continue
		}
		name := pageFiles[pageNum]
		info, _ := os.Stat(filepath.Join(outDir, name))
		frames := 0
		for _, r := range b.Frames {
			if r.P == pageNum {
				frames++
			}
		}
		h := atlas.Bounds().Dy()
		fmt.Printf("%-16s %dx%d (%d%% del límite), %d KB, %d frames\n",
			name, atlasWidth, h, 100*h/maxTextureSize, info.Size()/1024, frames)
	}
	fmt.Printf("bundle.json %d cuerpos, %d cabezas, %d animaciones, %d fx\n", len(b.Bodies), len(b.Heads), len(b.Anims), len(b.Fxs))

	if aoMap != nil {
		mapPath := filepath.Join(outDir, fmt.Sprintf("map%d.json", aoMap.Number))
		if err := writeJSON(mapPath, aoMap.clientMap(mapDisplayName)); err != nil {
			return err
		}
		mapInfo, _ := os.Stat(mapPath)
		fmt.Printf("map%d.json  %d KB  \"%s\"\n", aoMap.Number, mapInfo.Size()/1024, mapDisplayName)
	}
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
