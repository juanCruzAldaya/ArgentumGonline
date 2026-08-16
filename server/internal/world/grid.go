package world

import "math/rand"

// Grid is the static collision layer of a map: which tiles can be walked on.
// It never changes during a match, which is why the client can be handed the
// whole thing once at join time and never think about it again.
type Grid struct {
	W, H    int
	blocked []bool
}

func NewGrid(w, h int) *Grid {
	return &Grid{W: w, H: h, blocked: make([]bool, w*h)}
}

func (g *Grid) InBounds(x, y int) bool {
	return x >= 0 && y >= 0 && x < g.W && y < g.H
}

// Blocked reports whether a tile cannot be entered. Out-of-bounds counts as
// blocked so callers never need a separate bounds check before moving.
func (g *Grid) Blocked(x, y int) bool {
	if !g.InBounds(x, y) {
		return true
	}
	return g.blocked[y*g.W+x]
}

func (g *Grid) SetBlocked(x, y int, v bool) {
	if !g.InBounds(x, y) {
		return
	}
	g.blocked[y*g.W+x] = v
}

// PackedBitset returns the collision layer as a row-major bitset, one bit per
// tile, LSB first within each byte. A 100x100 map fits in 1250 bytes, so the
// client can be sent the whole map in the welcome frame.
func (g *Grid) PackedBitset() []byte {
	out := make([]byte, (len(g.blocked)+7)/8)
	for i, b := range g.blocked {
		if b {
			out[i/8] |= 1 << (i % 8)
		}
	}
	return out
}

// GenerateDemoMap builds a walkable arena ringed by a wall with a scattering of
// rectangular obstacles. It is deterministic for a given seed so that the
// server, every client and every bot agree on the world without shipping a map
// file around yet.
func GenerateDemoMap(w, h int, seed int64) *Grid {
	g := NewGrid(w, h)
	rng := rand.New(rand.NewSource(seed))

	for x := 0; x < w; x++ {
		g.SetBlocked(x, 0, true)
		g.SetBlocked(x, h-1, true)
	}
	for y := 0; y < h; y++ {
		g.SetBlocked(0, y, true)
		g.SetBlocked(w-1, y, true)
	}

	// Roughly one obstacle per 90 tiles keeps the arena readable while still
	// giving movement something to path around.
	obstacles := (w * h) / 90
	for i := 0; i < obstacles; i++ {
		ow := 1 + rng.Intn(4)
		oh := 1 + rng.Intn(4)
		ox := 2 + rng.Intn(max(1, w-ow-4))
		oy := 2 + rng.Intn(max(1, h-oh-4))
		for y := oy; y < oy+oh; y++ {
			for x := ox; x < ox+ow; x++ {
				g.SetBlocked(x, y, true)
			}
		}
	}
	return g
}
