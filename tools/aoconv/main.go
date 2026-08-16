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

	bodies, err := loadFacings(filepath.Join(*assets, "INIT", "Cuerpos.ini"), "BODY", "Walk")
	if err != nil {
		fatal("%v", err)
	}
	fmt.Printf("Cuerpos.ini:  %d cuerpos\n", len(bodies))

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
		wanted = append(wanted, describeSet(bodies, *body, "cuerpo")...)
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

// loadFacings parses Cuerpos.ini / Cabezas.ini, which share a shape: a section
// per entity holding one grh per facing.
func loadFacings(path, section, key string) (map[int][4]int, error) {
	lines, err := iniLines(path)
	if err != nil {
		return nil, err
	}

	out := map[int][4]int{}
	current := 0
	for _, line := range lines {
		if strings.HasPrefix(line, "[") {
			name := strings.Trim(line, "[]")
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
		if !ok || !strings.HasPrefix(rawKey, key) {
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
		line := scanner.Text()
		if idx := strings.IndexByte(line, '\''); idx >= 0 {
			line = line[:idx]
		}
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return out, scanner.Err()
}

func atoi(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "aoconv: "+format+"\n", args...)
	os.Exit(1)
}
