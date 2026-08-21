// Command aoconv reads Argentum Online's asset indices and turns them into
// something a modern client can use.
//
// AO addresses every image through a "grh" number. Graficos.ini maps each grh
// either to a rectangle inside a numbered sheet, or to a list of other grhs
// that form an animation. Bodies and heads are then defined as four grhs, one
// per facing.
//
// The index files are twenty years old and were exported by tooling that is no
// longer around, so this deliberately keeps a -dump mode: the only trustworthy
// way to confirm a record was parsed correctly is to look at the sprite.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"image"
	"image/draw"
	_ "image/jpeg" // AO's sheets are a mix of formats; register the decoders.
	"image/png"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Grh is one entry of the graphics index.
//
// A grh is either static — a rectangle in a sheet — or animated, in which case
// it holds the grh numbers of its frames and nothing else.
type Grh struct {
	Num   int
	File  int
	X     int
	Y     int
	W     int
	H     int
	Anim  []int
	Speed float64
}

func (g Grh) Animated() bool { return len(g.Anim) > 0 }

// Facings are stored in AO's order: up, right, down, left.
type Facing int

const (
	Up Facing = iota
	Right
	Down
	Left
)

var facingNames = [...]string{"arriba", "derecha", "abajo", "izquierda"}

func main() {
	var (
		assets = flag.String("assets", "", "directory holding INIT/ and Graficos/ (required)")
		dump   = flag.String("dump", "", "comma separated grh numbers to write out as PNGs")
		head   = flag.Int("head", 0, "dump all four facings of this head")
		body   = flag.Int("body", 0, "dump all four facings of this body")
		outDir = flag.String("out", "", "directory to write dumped PNGs into (required with -dump/-head/-body)")
		info   = flag.Bool("info", false, "print the parsed record for anything dumped")

		bundleDir = flag.String("bundle", "", "directory to write atlas.png and bundle.json into")
		overrides = flag.String("overrides", "overrides", "directory of replacement art painted over the packed atlas")
		bodyList  = flag.String("bodies", "1,2,3,4,5,6,7,8", "comma separated body ids to bundle")
		headList  = flag.String("heads", "1,2,3,4,5,6,7,8", "comma separated head ids to bundle")

		mapNum    = flag.Int("map", 0, "Argentum map number to convert into the bundle")
		mundosDir = flag.String("mundos", "", "directory holding Mapa*.map (default <assets>/Mundos)")
		serverOut = flag.String("server-out", "", "directory to write the server's data into")
		datDir    = flag.String("dat", "", "directory holding obj.dat and Hechizos.dat (default <assets>/Dat)")

		worlds    = flag.Bool("worlds", false, "build the composed worlds in worlds/layout.json")
		worldsDir = flag.String("worlds-dir", "worlds", "directory holding layout.json and water.json")
		tiledOut  = flag.String("tiled", "", "export the composed worlds as Tiled maps into this directory, for editing")

		audioDir  = flag.String("audio", "", "directory holding the original's WAV files and its MP3/ subdirectory (default <assets>/AUDIO)")
		soundsOut = flag.String("sounds", "", "directory to write the converted sound effects into")
		musicOut  = flag.String("music", "", "directory to copy the two music tracks into; the server serves them from there")
	)
	flag.Parse()

	if *assets == "" {
		fatal("-assets is required")
	}

	grhs, err := loadGraficos(filepath.Join(*assets, "INIT", "Graficos.ini"))
	if err != nil {
		fatal("%v", err)
	}
	fmt.Printf("Graficos.ini: %d grh\n", len(grhs))

	heads, err := loadFacings(filepath.Join(*assets, "INIT", "Cabezas.ini"), "HEAD", "Head")
	if err != nil {
		fatal("%v", err)
	}
	fmt.Printf("Cabezas.ini:  %d cabezas\n", len(heads))

	// Bodies come from Personajes.ini, NOT Cuerpos.ini.
	//
	// This is the file the real client actually loads — its CargarCuerpos
	// reads Personajes.ind despite the sub's name, and the client source
	// references Cuerpos.ini nowhere at all. Cuerpos.ini is stale leftover
	// data and it is wrong in three ways at once: it stops at 597 bodies
	// against Personajes.ini's 692, half its Walk2/Walk4 entries are 0 or -4
	// so bodies only face two directions, and its HeadOffsetX holds grh
	// numbers rather than pixel offsets. Body 1 side by side:
	//
	//	Cuerpos.ini      Walk 4582/0/4584/0     HeadOffset 4581,0
	//	Personajes.ini   Walk 4582/4584/4581/4583  HeadOffset 0,-4
	//
	// Every earlier oddity in this converter traces back to that file: the
	// facing order had to be found by trial because the data it was derived
	// from was garbage, head alignment needed a content-bounds heuristic
	// because the offsets were nonsense, and bodies had to be synthesised at
	// all because two of four directions were missing.
	bodies, err := loadBodies(filepath.Join(*assets, "INIT", "Personajes.ini"))
	if err != nil {
		fatal("%v", err)
	}
	fmt.Printf("Personajes.ini: %d cuerpos\n", len(bodies))

	// The three worn-equipment tables. Separate files, separate numbering,
	// and two different key names — Armas/Escudos use Dir1..4 while Cascos
	// uses Head1..4. The index in all of them is the heading, and the code is
	// authoritative that 1=N 2=E 3=S 4=W; the trailing comments in the files
	// disagree with each other and are simply wrong.
	weaponAnims, err := loadFacings(filepath.Join(*assets, "INIT", "Armas.dat"), "ARMA", "Dir")
	if err != nil {
		fatal("%v", err)
	}
	shieldAnims, err := loadFacings(filepath.Join(*assets, "INIT", "Escudos.dat"), "ESC", "Dir")
	if err != nil {
		fatal("%v", err)
	}
	helmetAnims, err := loadFacings(filepath.Join(*assets, "INIT", "Cascos.ini"), "HEAD", "Head")
	if err != nil {
		fatal("%v", err)
	}
	fmt.Printf("equipo:       %d armas, %d escudos, %d cascos\n",
		len(weaponAnims), len(shieldAnims), len(helmetAnims))

	dat := *datDir
	if dat == "" {
		dat = filepath.Join(*assets, "Dat")
	}

	items, err := loadItems(filepath.Join(dat, "obj.dat"))
	if err != nil {
		fatal("%v", err)
	}
	spells, err := loadSpells(filepath.Join(dat, "Hechizos.dat"))
	if err != nil {
		fatal("%v", err)
	}
	fxs, err := loadFxs(filepath.Join(*assets, "INIT", "Fxs.ini"))
	if err != nil {
		fatal("%v", err)
	}
	// Which of those a shop sells is what tells a starting kit apart from a
	// GM's toybox — see loadSold, and the server's loadout.go for what reads
	// it. Marked here rather than inside loadItems because it comes out of a
	// different file entirely.
	sold, err := loadSold(dat)
	if err != nil {
		fatal("%v", err)
	}
	inShops := 0
	for id, item := range items {
		if sold[id] {
			item.Sold = true
			items[id] = item
			inShops++
		}
	}
	fmt.Printf("obj.dat:      %d objetos que se pueden llevar, %d de ellos en tiendas\n",
		len(items), inShops)
	fmt.Printf("Hechizos.dat: %d hechizos\n", len(spells))
	fmt.Printf("Fxs.ini:      %d efectos\n", len(fxs))

	// Sonido. Independiente de todo lo demás — no toca el atlas ni los mapas —
	// así que corre acá, apenas los hechizos están leídos, que es lo único que
	// hace falta para saber qué convertir. Ver sounds.go.
	if *soundsOut != "" || *musicOut != "" {
		audio := *audioDir
		if audio == "" {
			audio = filepath.Join(*assets, "AUDIO")
		}
		if *soundsOut != "" {
			if err := convertSounds(audio, *soundsOut, spells); err != nil {
				fatal("%v", err)
			}
		}
		if *musicOut != "" {
			if err := copyMusic(audio, *musicOut); err != nil {
				fatal("%v", err)
			}
		}
	}

	var aoMap *AOMap
	var displayName string
	if *mapNum > 0 {
		dir := *mundosDir
		if dir == "" {
			dir = filepath.Join(*assets, "Mundos")
		}
		base := filepath.Join(dir, fmt.Sprintf("Mapa%d", *mapNum))

		aoMap, err = readAOMap(base+".map", *mapNum)
		if err != nil {
			fatal("%v", err)
		}
		displayName = mapName(base+".dat", *mapNum)
		blocked := 0
		for i := range aoMap.Tiles {
			if aoMap.Tiles[i].Blocked {
				blocked++
			}
		}
		fmt.Printf("Mapa%d:        \"%s\", versión %d, %d de %d tiles bloqueados\n",
			*mapNum, displayName, aoMap.Version, blocked, len(aoMap.Tiles))

		if *serverOut != "" {
			path := filepath.Join(*serverOut, fmt.Sprintf("map%d.json", *mapNum))
			if err := writeJSON(path, aoMap.serverMap(displayName)); err != nil {
				fatal("%v", err)
			}
			fmt.Printf("              %s escrito para el servidor\n", path)
		}
	}

	// tileGrhs is every graphic the world geometry needs, from whichever source
	// was asked for. It goes to the atlas as its own page: tiles are the half of
	// the content that grows with the map, and keeping them off the character
	// page is what lets four worlds fit at all.
	tileGrhs := map[int]bool{}
	if aoMap != nil {
		tileGrhs = aoMap.usedGrhs()
	}

	if *worlds {
		dir := *mundosDir
		if dir == "" {
			dir = filepath.Join(*assets, "Mundos")
		}
		used, err := writeWorlds(dir,
			filepath.Join(*worldsDir, "layout.json"),
			filepath.Join(*worldsDir, "water.json"),
			*bundleDir, *serverOut, newTilePalette(*assets, grhs))
		if err != nil {
			fatal("%v", err)
		}
		for grh := range used {
			tileGrhs[grh] = true
		}
	}

	if *tiledOut != "" {
		dir := *mundosDir
		if dir == "" {
			dir = filepath.Join(*assets, "Mundos")
		}
		if err := writeTiled(dir,
			filepath.Join(*worldsDir, "layout.json"),
			filepath.Join(*worldsDir, "water.json"),
			*assets, *tiledOut, grhs); err != nil {
			fatal("%v", err)
		}
	}

	if *bundleDir != "" {
		// Which bodies to pack is not a list anyone should be maintaining by
		// hand any more. Equipping armour *is* changing your body in Argentum
		// — obj.dat's NumRopaje becomes Char.body — so every armour that can
		// be found has to have its body in the atlas or the wearer turns
		// invisible. That is 300-odd bodies, plus the naked ones a character
		// falls back to with no armour on, plus the corpse.
		wantBodies := map[int]bool{ghostBody: true}
		for _, id := range nakedBodies {
			wantBodies[id] = true
		}
		for _, item := range items {
			if item.Type == ObjArmor && item.Body > 0 {
				wantBodies[item.Body] = true
			}
		}
		for _, id := range parseList(*bodyList) {
			wantBodies[id] = true
		}
		bodyIDs := make([]int, 0, len(wantBodies))
		for id := range wantBodies {
			bodyIDs = append(bodyIDs, id)
		}
		sort.Ints(bodyIDs)

		if err := writeBundle(grhs, bodies, heads, weaponAnims, shieldAnims, helmetAnims,
			*assets, bodyIDs, parseList(*headList), *bundleDir, *overrides,
			aoMap, displayName, tileGrhs, items, fxs); err != nil {
			fatal("%v", err)
		}

		// Items and spells go to the client for names and icons, and to the
		// server for the numbers combat runs on. One writer, so they cannot
		// drift apart.
		for _, dir := range []string{*bundleDir, *serverOut} {
			if dir == "" {
				continue
			}
			if err := writeJSON(filepath.Join(dir, "items.json"), items); err != nil {
				fatal("%v", err)
			}
			if err := writeJSON(filepath.Join(dir, "spells.json"), spells); err != nil {
				fatal("%v", err)
			}
		}
	}

	var wanted []int
	if *dump != "" {
		for _, field := range strings.Split(*dump, ",") {
			n, err := strconv.Atoi(strings.TrimSpace(field))
			if err != nil {
				fatal("grh inválido %q", field)
			}
			wanted = append(wanted, n)
		}
	}
	if *head > 0 {
		wanted = append(wanted, describeSet(heads, *head, "cabeza")...)
	}
	if *body > 0 {
		if b, ok := bodies[*body]; ok {
			wanted = append(wanted, describeSet(map[int][4]int{*body: b.Facings}, *body, "cuerpo")...)
		}
	}
	if len(wanted) == 0 {
		return
	}
	if *outDir == "" {
		fatal("-out is required when dumping")
	}
	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		fatal("%v", err)
	}

	for _, num := range wanted {
		if err := dumpGrh(grhs, *assets, num, *outDir, *info); err != nil {
			fmt.Printf("  grh %-6d ERROR  %v\n", num, err)
		}
	}
}

// describeSet prints a body's or head's four facings and returns their grhs.
func describeSet(sets map[int][4]int, id int, label string) []int {
	set, ok := sets[id]
	if !ok {
		fatal("%s %d no existe", label, id)
	}
	fmt.Printf("\n%s %d:\n", label, id)
	var out []int
	for facing, grh := range set {
		fmt.Printf("  %-10s grh %d\n", facingNames[facing], grh)
		if grh > 0 {
			out = append(out, grh)
		}
	}
	return out
}

// dumpGrh writes one grh to a PNG. Animated grhs are written as a horizontal
// strip of their frames, which is the fastest way to eyeball whether the frame
// list was parsed in the right order.
func dumpGrh(grhs map[int]Grh, assets string, num int, outDir string, info bool) error {
	g, ok := grhs[num]
	if !ok {
		return fmt.Errorf("no existe en el índice")
	}

	frames := []Grh{g}
	if g.Animated() {
		frames = frames[:0]
		for _, frameNum := range g.Anim {
			frame, ok := grhs[frameNum]
			if !ok {
				return fmt.Errorf("frame %d no existe", frameNum)
			}
			if frame.Animated() {
				return fmt.Errorf("frame %d es a su vez una animación", frameNum)
			}
			frames = append(frames, frame)
		}
	}

	if info {
		if g.Animated() {
			fmt.Printf("  grh %-6d animación de %d frames, speed %.0f: %v\n",
				num, len(g.Anim), g.Speed, g.Anim)
		} else {
			fmt.Printf("  grh %-6d hoja %d  rect (%d,%d) %dx%d\n", num, g.File, g.X, g.Y, g.W, g.H)
		}
	}

	var totalW, maxH int
	for _, f := range frames {
		totalW += f.W
		if f.H > maxH {
			maxH = f.H
		}
	}
	if totalW == 0 || maxH == 0 {
		return fmt.Errorf("rectángulo vacío (%dx%d)", totalW, maxH)
	}

	canvas := image.NewNRGBA(image.Rect(0, 0, totalW, maxH))
	sheets := map[int]image.Image{}
	atX := 0
	for _, f := range frames {
		if _, cached := sheets[f.File]; !cached {
			sheet, err := openSheet(assets, f.File)
			if err != nil {
				return err
			}
			sheets[f.File] = sheet
		}
		sheet := sheets[f.File]

		src := image.Rect(f.X, f.Y, f.X+f.W, f.Y+f.H)
		if !src.In(sheet.Bounds()) {
			return fmt.Errorf("rect %v se sale de la hoja %d %v", src, f.File, sheet.Bounds())
		}
		draw.Draw(canvas, image.Rect(atX, 0, atX+f.W, f.H), sheet, src.Min, draw.Src)
		atX += f.W
	}

	path := filepath.Join(outDir, fmt.Sprintf("grh%d.png", num))
	out, err := os.Create(path)
	if err != nil {
		return err
	}
	defer out.Close()
	if err := png.Encode(out, canvas); err != nil {
		return err
	}
	fmt.Printf("  grh %-6d -> %s (%dx%d, %d frame(s))\n", num, filepath.Base(path), totalW, maxH, len(frames))
	return nil
}

// openSheet loads a numbered sheet. AO shipped a mix of formats over the years,
// so the extension is discovered rather than assumed.
func openSheet(assets string, file int) (image.Image, error) {
	base := filepath.Join(assets, "Graficos", strconv.Itoa(file))
	for _, ext := range []string{".png", ".PNG", ".bmp", ".BMP", ".jpg", ".JPG"} {
		f, err := os.Open(base + ext)
		if err != nil {
			continue
		}
		defer f.Close()
		img, _, err := image.Decode(f)
		if err != nil {
			return nil, fmt.Errorf("decodificando hoja %d%s: %w", file, ext, err)
		}
		return img, nil
	}
	return nil, fmt.Errorf("hoja %d no encontrada", file)
}

// loadGraficos parses the [Graphics] section of Graficos.ini.
//
// Each value is dash separated and starts with a frame count. One frame means
// the rest describes a rectangle; more than one means the rest is a list of grh
// numbers followed by an animation speed.
func loadGraficos(path string) (map[int]Grh, error) {
	lines, err := iniLines(path)
	if err != nil {
		return nil, err
	}

	out := make(map[int]Grh, 32870)
	for _, line := range lines {
		key, value, ok := strings.Cut(line, "=")
		if !ok || !strings.HasPrefix(key, "Grh") {
			continue
		}
		num, err := strconv.Atoi(strings.TrimPrefix(key, "Grh"))
		if err != nil {
			continue
		}

		fields := strings.Split(value, "-")
		if len(fields) < 2 {
			continue
		}
		frames, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}

		switch {
		case frames == 1 && len(fields) >= 6:
			out[num] = Grh{
				Num:  num,
				File: atoi(fields[1]),
				X:    atoi(fields[2]),
				Y:    atoi(fields[3]),
				W:    atoi(fields[4]),
				H:    atoi(fields[5]),
			}
		case frames > 1 && len(fields) >= frames+2:
			anim := make([]int, 0, frames)
			for _, f := range fields[1 : frames+1] {
				anim = append(anim, atoi(f))
			}
			speed, _ := strconv.ParseFloat(strings.TrimSpace(fields[frames+1]), 64)
			out[num] = Grh{Num: num, Anim: anim, Speed: speed}
		}
	}
	return out, nil
}

// loadFacings parses the four-graphics-per-entity index files: Cabezas.ini
// ([HEADn] Head1..4), Armas.dat ([ARMAn] Dir1..4), Escudos.dat ([ESCn] Dir1..4)
// and Personajes.ini's walk half ([BODYn] Walk1..4).
//
// Matching is case-insensitive because Armas.dat genuinely mixes cases within
// one file — it has both [Arma1] and [ARMA4].
//
// The facing index is the source's own E_Heading: 1 north, 2 east, 3 south,
// 4 west. Do not take this from the trailing comments in the files; [Arma1]
// labels Dir1 "norte" while [ESC3] labels Dir3 "norte", so they cannot both be
// right, and the client's array subscript settles it.
func loadFacings(path, section, key string) (map[int][4]int, error) {
	lines, err := iniLines(path)
	if err != nil {
		return nil, err
	}
	section, key = strings.ToUpper(section), strings.ToUpper(key)

	out := map[int][4]int{}
	current := 0
	for _, line := range lines {
		if strings.HasPrefix(line, "[") {
			name := strings.ToUpper(strings.Trim(line, "[]"))
			current = 0
			if strings.HasPrefix(name, section) {
				current = atoi(strings.TrimPrefix(name, section))
			}
			continue
		}
		if current == 0 {
			continue
		}

		rawKey, rawValue, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		rawKey = strings.ToUpper(strings.TrimSpace(rawKey))
		if !strings.HasPrefix(rawKey, key) {
			continue
		}
		facing, err := strconv.Atoi(strings.TrimPrefix(rawKey, key))
		if err != nil || facing < 1 || facing > 4 {
			continue
		}

		set := out[current]
		set[facing-1] = atoi(rawValue)
		out[current] = set
	}
	return out, nil
}

// BodyIndex is one Personajes.ini record: four walk animations plus where the
// head sits on this body.
type BodyIndex struct {
	Facings    [4]int
	HeadOffset [2]int
}

// loadBodies parses Personajes.ini. Same [BODYn] Walk1..4 shape loadFacings
// handles, plus the two head-offset keys, which are the real reason this file
// matters: the client draws the head at the body's own position *plus* this
// offset, so it is per-body data and not something a converter can measure or
// guess. Humano/Elfo/Drow bodies carry -4 on Y, the short races +6.
func loadBodies(path string) (map[int]BodyIndex, error) {
	facings, err := loadFacings(path, "BODY", "Walk")
	if err != nil {
		return nil, err
	}
	lines, err := iniLines(path)
	if err != nil {
		return nil, err
	}

	out := make(map[int]BodyIndex, len(facings))
	for id, set := range facings {
		out[id] = BodyIndex{Facings: set}
	}

	current := 0
	for _, line := range lines {
		if strings.HasPrefix(line, "[") {
			name := strings.ToUpper(strings.Trim(line, "[]"))
			current = 0
			if strings.HasPrefix(name, "BODY") {
				current = atoi(strings.TrimPrefix(name, "BODY"))
			}
			continue
		}
		if current == 0 {
			continue
		}
		rawKey, rawValue, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		body, known := out[current]
		if !known {
			continue
		}
		switch strings.ToUpper(strings.TrimSpace(rawKey)) {
		case "HEADOFFSETX":
			body.HeadOffset[0] = atoi(rawValue)
		case "HEADOFFSETY":
			body.HeadOffset[1] = atoi(rawValue)
		default:
			continue
		}
		out[current] = body
	}
	return out, nil
}

// cp1252High maps the bytes where Windows-1252 differs from Latin-1. Every
// other byte in the range 0xA0-0xFF already equals its Unicode code point.
var cp1252High = map[byte]rune{
	0x80: '€', 0x82: '‚', 0x83: 'ƒ', 0x84: '„', 0x85: '…', 0x86: '†', 0x87: '‡',
	0x88: 'ˆ', 0x89: '‰', 0x8A: 'Š', 0x8B: '‹', 0x8C: 'Œ', 0x8E: 'Ž',
	0x91: '‘', 0x92: '’', 0x93: '“', 0x94: '”', 0x95: '•', 0x96: '–', 0x97: '—',
	0x98: '˜', 0x99: '™', 0x9A: 'š', 0x9B: '›', 0x9C: 'œ', 0x9E: 'ž', 0x9F: 'Ÿ',
}

// decodeCP1252 converts one Argentum data line to UTF-8.
//
// These files predate UTF-8 and are Windows-1252, so "Poción" and "Dardo
// Mágico" arrive as bytes that are not valid UTF-8 at all. Reading them as
// UTF-8 does not merely look wrong, it silently mangles every accented name in
// the game.
func decodeCP1252(b []byte) string {
	var sb strings.Builder
	sb.Grow(len(b))
	for _, c := range b {
		switch {
		case c < 0x80:
			sb.WriteByte(c)
		case cp1252High[c] != 0:
			sb.WriteRune(cp1252High[c])
		default:
			sb.WriteRune(rune(c))
		}
	}
	return sb.String()
}

// stripVBComment removes the trailing comments these files carry, without
// eating apostrophes that belong to the text.
//
// Two shapes exist and they need different rules. Section headers glue the
// comment straight on ("[OBJ46]'Nieve 1"), while values separate it with
// whitespace ("Walk1=4582  ' arriba"). Spell and item descriptions are prose,
// so cutting at every apostrophe would silently truncate them; requiring the
// preceding whitespace keeps in-word apostrophes intact.
func stripVBComment(line string) string {
	if strings.HasPrefix(line, "[") {
		if close := strings.IndexByte(line, ']'); close >= 0 {
			if idx := strings.IndexByte(line[close:], '\''); idx >= 0 {
				return line[:close+idx]
			}
		}
		return line
	}

	for i := 1; i < len(line); i++ {
		if line[i] == '\'' && (line[i-1] == ' ' || line[i-1] == '\t') {
			return line[:i]
		}
	}
	return line
}

// iniLines reads an index file, stripping the trailing VB comments that some of
// them carry ("Walk1=4582  ' arriba").
func iniLines(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var out []string
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := decodeCP1252(scanner.Bytes())
		line = stripVBComment(line)
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return out, scanner.Err()
}

// parseList reads a comma separated list of ids, skipping anything unparseable
// so a stray trailing comma is not fatal.
func parseList(s string) []int {
	var out []int
	for _, field := range strings.Split(s, ",") {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		if n, err := strconv.Atoi(field); err == nil {
			out = append(out, n)
		}
	}
	return out
}

func atoi(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "aoconv: "+format+"\n", args...)
	os.Exit(1)
}
