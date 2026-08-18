package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// worldBorder is how much of each Argentum map to throw away before stitching.
//
// The obvious answer is nine, which is how thick the ring of solid wall around
// a map measures. It is also wrong, and wrong in a way that is invisible until
// you try to walk: cropping nine produces a world whose seams are blocked on
// every single tile, 100 sealed cells with 13% of the terrain reachable.
//
// Argentum's maps were never meant to be geometrically adjacent. The original
// crosses between them by teleport, from a TileExit at x=12 to x=87 of the
// neighbour, so the three tiles between the wall ring and the exit line are
// scenery nobody ever walks through. Measured across a sample of maps, the
// first open column is exactly 12:
//
//	borde 9, 10, 11 -> 0 of 80-odd edge tiles walkable
//	borde 12        -> 73,6 of 76
//
// So the crop is the exit line, not the wall. buildWorld still checks the
// result rather than trusting this constant — see the connectivity pass.
const worldBorder = 12

// worldTile is the usable square each map contributes once its border is gone.
const worldTile = MapWidth - 2*worldBorder // 76

// connectedPercent is how much of a world's walkable ground has to be reachable
// from one place for the world to be usable. Not 100: real Argentum maps hold
// sealed courtyards and closed buildings, and a few hundred tiles behind a
// locked door are scenery, not a bug. A seam failure does not look like this —
// it takes the figure to 13%.
const connectedPercent = 90

// largestRegion is the size of the biggest four-connected walkable region.
//
// Flood fill from the first walkable tile of each unvisited region, keeping the
// largest. Iterative rather than recursive: a world is 600.000 tiles and the
// stack would not survive a recursive fill.
func largestRegion(m *AOMap) int {
	seen := make([]bool, len(m.Tiles))
	best := 0
	queue := make([]int, 0, 1024)
	for start := range m.Tiles {
		if seen[start] || m.Tiles[start].Blocked {
			continue
		}
		size := 0
		queue = append(queue[:0], start)
		seen[start] = true
		for len(queue) > 0 {
			i := queue[len(queue)-1]
			queue = queue[:len(queue)-1]
			size++
			x, y := i%m.W, i/m.W
			if x > 0 {
				step(m, seen, &queue, i-1)
			}
			if x < m.W-1 {
				step(m, seen, &queue, i+1)
			}
			if y > 0 {
				step(m, seen, &queue, i-m.W)
			}
			if y < m.H-1 {
				step(m, seen, &queue, i+m.W)
			}
		}
		if size > best {
			best = size
		}
	}
	return best
}

func step(m *AOMap, seen []bool, queue *[]int, i int) {
	if seen[i] || m.Tiles[i].Blocked {
		return
	}
	seen[i] = true
	*queue = append(*queue, i)
}

// World is one composed world: a grid of Argentum maps stitched into a single
// playable surface.
//
// The layouts are data, not something this tool derives. Composing them is a
// design job — the seam cost between two maps is measured against how well the
// original world's own neighbours match, and then annealed — and its output is
// reviewed by eye before it lands. Baking the result into worlds/layout.json
// keeps the build reproducible and lets a cell be swapped by hand.
type World struct {
	Number int    `json:"number"`
	Name   string `json:"name"`
	// Size is the grid's side in maps, including the ocean ring.
	Size int `json:"size"`
	// Core is the side of the playable middle. The ring between Core and Size
	// is ocean: real Argentum water maps, drawn for the horizon and forced
	// blocked so the world has an edge you can see.
	Core int     `json:"core"`
	Seam float64 `json:"seam"`
	// Grid is Size*Size Argentum map numbers, row major.
	Grid []int `json:"grid"`
}

// loadWorlds reads the composed layouts.
func loadWorlds(path string) ([]World, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out []World
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	for _, w := range out {
		if w.Size <= 0 || len(w.Grid) != w.Size*w.Size {
			return nil, fmt.Errorf("%s: el mundo %q dice %dx%d pero trae %d celdas",
				path, w.Name, w.Size, w.Size, len(w.Grid))
		}
	}
	return out, nil
}

// loadWaterGrhs reads the graphics that count as water.
//
// Argentum does not mark water as blocked in the .map: the original server
// stops you with a "navegando" flag and a boat, which this game has no concept
// of. Without this list a player walks straight out to sea, and the ocean ring
// stops being an edge at all.
//
// The list is derived, not hand-written: a grh counts as water when it turns up
// at least twenty times more often per tile in Argentum's 105 all-water maps
// than in its all-land ones. That covers 95% of the tiles of an ocean map, and
// the 2.8% of land tiles it also catches are the lakes and rivers, which should
// block too.
func loadWaterGrhs(path string) (map[int]bool, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var list []int
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	out := make(map[int]bool, len(list))
	for _, g := range list {
		out[g] = true
	}
	return out, nil
}

// buildWorld stitches a world's maps into one AOMap.
//
// Every source map is cropped to its playable middle and dropped into place, so
// the seams butt directly rather than leaving 18 tiles of wall between maps.
// Two things then get forced blocked that the source data leaves open: every
// tile of the ocean ring, and every water tile anywhere.
func buildWorld(mundosDir string, w World, water map[int]bool) (*AOMap, int, error) {
	side := w.Size * worldTile
	out := &AOMap{
		Number: w.Number,
		W:      side,
		H:      side,
		Tiles:  make([]Tile, side*side),
	}

	// One map number can appear in several cells; parse each file once.
	cache := map[int]*AOMap{}
	drowned := 0
	for cell, number := range w.Grid {
		src, ok := cache[number]
		if !ok {
			var err error
			base := filepath.Join(mundosDir, fmt.Sprintf("Mapa%d", number))
			src, err = readAOMap(base+".map", number)
			if err != nil {
				return nil, 0, fmt.Errorf("mundo %q, celda %d: %w", w.Name, cell, err)
			}
			cache[number] = src
		}

		cx, cy := cell%w.Size, cell/w.Size
		ring := cx < (w.Size-w.Core)/2 || cy < (w.Size-w.Core)/2 ||
			cx >= (w.Size+w.Core)/2 || cy >= (w.Size+w.Core)/2

		for y := 0; y < worldTile; y++ {
			for x := 0; x < worldTile; x++ {
				t := *src.at(worldBorder+x, worldBorder+y)
				if ring {
					t.Blocked = true
				} else if water[t.Layers[0]] && !t.Blocked {
					t.Blocked = true
					drowned++
				}
				out.Tiles[(cy*worldTile+y)*side+(cx*worldTile+x)] = t
			}
		}
	}
	return out, drowned, nil
}

// writeWorlds converts every composed world and reports what came out.
//
// It returns the union of every graphic all the worlds reference, so the atlas
// can pack the tiles for all four rather than only the one being played — the
// server picks a world per match and the client has to be able to draw any.
func writeWorlds(mundosDir, layoutPath, waterPath string, clientDir, serverDir string, pal *tilePalette) (map[int]bool, error) {
	worlds, err := loadWorlds(layoutPath)
	if err != nil {
		return nil, err
	}
	water, err := loadWaterGrhs(waterPath)
	if err != nil {
		return nil, err
	}

	used := map[int]bool{}
	for _, w := range worlds {
		built, drowned, err := buildWorld(mundosDir, w, water)
		if err != nil {
			return nil, err
		}
		for grh := range built.usedGrhs() {
			used[grh] = true
		}

		blocked := 0
		for i := range built.Tiles {
			if built.Tiles[i].Blocked {
				blocked++
			}
		}
		walkable := len(built.Tiles) - blocked

		// A world that is walkable but not connected is worse than a broken
		// build, because it looks finished: the map draws, the player spawns,
		// and only walking into the first seam reveals that the world is a
		// hundred sealed cells. That is exactly what cropping the wall ring
		// instead of the exit line produced, and nothing in the pipeline
		// noticed. So the connectivity is measured here, and a world that
		// fails is refused rather than shipped.
		reachable := largestRegion(built)
		if pct := 100 * reachable / walkable; pct < connectedPercent {
			return nil, fmt.Errorf("mundo %q: solo el %d%% del terreno caminable es alcanzable (%d de %d) — las costuras están tapiadas",
				w.Name, pct, reachable, walkable)
		}

		if clientDir != "" {
			path := filepath.Join(clientDir, fmt.Sprintf("map%d.json", w.Number))
			if err := writeJSON(path, built.clientMap(w.Name)); err != nil {
				return nil, err
			}
			// The map the player opens, drawn from the tile art rather than
			// from the collision bitset. Baked here because Go walks 577.600
			// tiles in milliseconds and the client would spend a visible
			// pause doing the same thing on every join.
			if pal != nil {
				mini := filepath.Join(clientDir, fmt.Sprintf("map%d_mini.png", w.Number))
				if err := writeWorldMinimap(mini, built, pal); err != nil {
					return nil, err
				}
			}
		}
		if serverDir != "" {
			path := filepath.Join(serverDir, fmt.Sprintf("map%d.json", w.Number))
			if err := writeJSON(path, built.serverMap(w.Name)); err != nil {
				return nil, err
			}
		}

		distinct := map[int]bool{}
		for _, n := range w.Grid {
			distinct[n] = true
		}
		fmt.Printf("mundo %d %-8s %dx%d tiles, %d mapas (%d distintos), %d caminables (%d%% conectado), %d tiles de agua cerrados\n",
			w.Number, w.Name, built.W, built.H, len(w.Grid), len(distinct), walkable,
			100*reachable/walkable, drowned)
	}

	all := make([]int, 0, len(used))
	for grh := range used {
		all = append(all, grh)
	}
	sort.Ints(all)
	fmt.Printf("los %d mundos usan %d grh distintos en total\n", len(worlds), len(all))
	return used, nil
}
