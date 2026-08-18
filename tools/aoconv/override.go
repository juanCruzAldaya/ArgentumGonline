package main

import (
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// applyOverrides paints hand-made art over frames the packer already placed.
//
// The atlas is a build artifact: regenerating it from the original Argentum
// data is the normal way to change what the game draws, and anything edited
// into atlas.png by hand is gone the next time aoconv runs. That is fine for
// the sprites that come from AO, and useless for the ones that do not — a
// replacement effect drawn from scratch has no source file for aoconv to read.
// This is where those live, so a rebuild reproduces them instead of reverting
// them.
//
// A file named anim<N>.png is a horizontal strip standing in for animated grh
// N: it is cut into as many equal cells as that animation has frames, and each
// cell replaces one. A file named grh<N>.png replaces the single static grh N.
//
// Sizes have to match exactly. aoconv has no image dependencies and pulling in
// a resampler to paper over a mismatched strip would hide the far likelier
// cause — art cut against a stale bundle, whose frames moved when the atlas
// was repacked.
func applyOverrides(atlas *image.NRGBA, b *Bundle, dir string) error {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}

	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".png") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	for _, name := range names {
		art, err := readPNG(filepath.Join(dir, name))
		if err != nil {
			return fmt.Errorf("override %s: %w", name, err)
		}
		stem := strings.TrimSuffix(name, ".png")

		var targets []int
		switch {
		case strings.HasPrefix(stem, "anim"):
			num, err := strconv.Atoi(stem[len("anim"):])
			if err != nil {
				return fmt.Errorf("override %s: %q no es un numero de grh", name, stem[len("anim"):])
			}
			anim, ok := b.Anims[num]
			if !ok {
				// The grh exists in Argentum but nothing in this bundle asked
				// for it, so its frames were never packed. Silence would look
				// like the art had been applied.
				return fmt.Errorf("override %s: el grh %d no es una animacion de este bundle", name, num)
			}
			targets = anim.Frames
		case strings.HasPrefix(stem, "grh"):
			num, err := strconv.Atoi(stem[len("grh"):])
			if err != nil {
				return fmt.Errorf("override %s: %q no es un numero de grh", name, stem[len("grh"):])
			}
			targets = []int{num}
		default:
			return fmt.Errorf("override %s: el nombre tiene que ser anim<N>.png o grh<N>.png", name)
		}

		cellW := art.Bounds().Dx() / len(targets)
		if cellW*len(targets) != art.Bounds().Dx() {
			return fmt.Errorf("override %s: %d px de ancho no se parten en %d frames iguales",
				name, art.Bounds().Dx(), len(targets))
		}
		for i, id := range targets {
			dst, ok := b.Frames[id]
			if !ok {
				return fmt.Errorf("override %s: el frame %d no quedo en el atlas", name, id)
			}
			if cellW != dst.W || art.Bounds().Dy() != dst.H {
				return fmt.Errorf("override %s: frame %d es de %dx%d y el arte trae %dx%d",
					name, id, dst.W, dst.H, cellW, art.Bounds().Dy())
			}
			// draw.Src, not draw.Over: this replaces the packed frame rather
			// than sitting on top of it. Compositing would leave the old
			// sprite showing through wherever the new art is transparent.
			draw.Draw(atlas,
				image.Rect(dst.X, dst.Y, dst.X+dst.W, dst.Y+dst.H),
				art, art.Bounds().Min.Add(image.Pt(i*cellW, 0)), draw.Src)
		}
		fmt.Printf("override:     %s -> %d frames\n", name, len(targets))
	}
	return nil
}

func readPNG(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return png.Decode(f)
}
