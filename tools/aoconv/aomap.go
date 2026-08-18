package main

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Argentum maps are always this size. The bounds are compiled into the original
// server as XMinMapSize/XMaxMapSize rather than stored in the file.
const (
	MapWidth  = 100
	MapHeight = 100

	// mapHeaderSize is version(2) + desc(255) + crc(4) + magic(4) + 8 reserved,
	// matching the sequence the VB6 loader reads before the first tile.
	mapHeaderSize = 2 + 255 + 4 + 4 + 8
)

// Tile is one map square. Argentum draws four layers: ground, then decals and
// objects, then anything the player walks behind, then roofs.
type Tile struct {
	Blocked bool
	Layers  [4]int
	Trigger int
}

// AOMap is a parsed .map file, or several of them merged into one world.
//
// W and H are carried per map rather than taken from the MapWidth/MapHeight
// constants because a composed world is not 100x100 — see world.go.
type AOMap struct {
	Number  int
	Version int
	Desc    string
	W, H    int
	Tiles   []Tile // row major, W*H
}

func (m *AOMap) at(x, y int) *Tile { return &m.Tiles[y*m.W+x] }

// ClientMap is what the renderer needs: the four graphic layers.
//
// Layer 1 covers every tile, so it ships as a flat array. The rest are sparse —
// most ground has nothing on it — so they ship as index/grh pairs, which for a
// typical map is an order of magnitude smaller than four dense arrays.
type ClientMap struct {
	Number int         `json:"number"`
	Name   string      `json:"name"`
	Width  int         `json:"width"`
	Height int         `json:"height"`
	Layer1 []int       `json:"layer1"`
	Layer2 map[int]int `json:"layer2"`
	Layer3 map[int]int `json:"layer3"`
	Layer4 map[int]int `json:"layer4"`

	// Roofed is every tile the map marks as being under a roof — eTrigger's
	// BAJOTECHO (1) and CASA (2), from Declares.bas:301-311. Ullathorpe has
	// 390 of them.
	//
	// Without this the client draws layer 4 unconditionally and a player who
	// walks into a house disappears under an opaque roof: 118 of Ullathorpe's
	// walkable tiles have one. That reads exactly like being trapped, because
	// you can no longer see yourself or the door you came in through. The
	// trigger was always parsed here and simply never handed on.
	Roofed []int `json:"roofed"`
}

// ServerMap is what the simulation needs, and nothing more: where you cannot
// walk. Keeping graphics out of it means a modified client cannot learn
// anything about the map that the server does not already enforce.
type ServerMap struct {
	Number  int    `json:"number"`
	Name    string `json:"name"`
	Width   int    `json:"width"`
	Height  int    `json:"height"`
	Blocked string `json:"blocked"` // base64 row-major bitset, LSB first
}

// readAOMap parses one Mapa{N}.map.
//
// The layout comes from CargarMapa in the original server's FileIO.bas: a fixed
// header, then tiles in y-outer/x-inner order, each a flag byte followed by
// only the fields its flags claim. There is no per-tile length, so a single
// misread byte desynchronises everything after it — hence the trailing check.
func readAOMap(path string, number int) (*AOMap, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(raw) < mapHeaderSize {
		return nil, fmt.Errorf("%s: %d bytes, demasiado corto para el header", path, len(raw))
	}

	m := &AOMap{
		Number:  number,
		Version: int(int16(binary.LittleEndian.Uint16(raw[0:2]))),
		Desc:    strings.TrimRight(decodeCP1252(raw[2:257]), "\x00 "),
		W:       MapWidth,
		H:       MapHeight,
		Tiles:   make([]Tile, MapWidth*MapHeight),
	}

	pos := mapHeaderSize
	need := func(n int) error {
		if pos+n > len(raw) {
			return fmt.Errorf("%s: se acabaron los bytes en %d", path, pos)
		}
		return nil
	}
	getInt32 := func() int {
		v := int(int32(binary.LittleEndian.Uint32(raw[pos : pos+4])))
		pos += 4
		return v
	}
	getInt16 := func() int {
		v := int(int16(binary.LittleEndian.Uint16(raw[pos : pos+2])))
		pos += 2
		return v
	}

	for y := 0; y < MapHeight; y++ {
		for x := 0; x < MapWidth; x++ {
			if err := need(5); err != nil {
				return nil, err
			}
			flags := raw[pos]
			pos++

			t := m.at(x, y)
			t.Blocked = flags&1 != 0
			t.Layers[0] = getInt32()

			for bit, layer := range map[byte]int{2: 1, 4: 2, 8: 3} {
				if flags&bit != 0 {
					if err := need(4); err != nil {
						return nil, err
					}
					t.Layers[layer] = getInt32()
				}
			}
			if flags&16 != 0 {
				if err := need(2); err != nil {
					return nil, err
				}
				t.Trigger = getInt16()
			}
		}
	}

	// A desynchronised parse almost always ends up short or long; landing
	// exactly on the end of the file is strong evidence every tile was read
	// with the right field widths.
	if pos != len(raw) {
		return nil, fmt.Errorf("%s: quedaron %d bytes sin leer (leídos %d de %d) — el formato no coincide",
			path, len(raw)-pos, pos, len(raw))
	}
	return m, nil
}

// mapName reads the display name out of the companion .dat, which is a plain
// INI. A missing name is not worth failing a conversion over.
func mapName(datPath string, number int) string {
	lines, err := iniLines(datPath)
	if err != nil {
		return ""
	}
	for _, line := range lines {
		if key, value, ok := strings.Cut(line, "="); ok && strings.EqualFold(strings.TrimSpace(key), "Name") {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

// usedGrhs returns every graphic the map references, so they can be packed into
// the atlas alongside the character sprites.
func (m *AOMap) usedGrhs() map[int]bool {
	out := map[int]bool{}
	for i := range m.Tiles {
		for _, grh := range m.Tiles[i].Layers {
			if grh > 0 {
				out[grh] = true
			}
		}
	}
	return out
}

func (m *AOMap) clientMap(name string) ClientMap {
	c := ClientMap{
		Number: m.Number,
		Name:   name,
		Width:  m.W,
		Height: m.H,
		Layer1: make([]int, len(m.Tiles)),
		Layer2: map[int]int{},
		Layer3: map[int]int{},
		Layer4: map[int]int{},
	}
	sparse := []map[int]int{nil, c.Layer2, c.Layer3, c.Layer4}
	for i := range m.Tiles {
		c.Layer1[i] = m.Tiles[i].Layers[0]
		for layer := 1; layer < 4; layer++ {
			if grh := m.Tiles[i].Layers[layer]; grh > 0 {
				sparse[layer][i] = grh
			}
		}
		// eTrigger BAJOTECHO=1, CASA=2. The rest — POSINVALIDA, ZONASEGURA,
		// ANTIPIQUETE, ZONAPELEA — are rules about NPCs, stealing and faction
		// state that this game does not have, so only the two roof triggers
		// are carried.
		if t := m.Tiles[i].Trigger; t == 1 || t == 2 {
			c.Roofed = append(c.Roofed, i)
		}
	}
	return c
}

func (m *AOMap) serverMap(name string) ServerMap {
	bits := make([]byte, (len(m.Tiles)+7)/8)
	for i := range m.Tiles {
		if m.Tiles[i].Blocked {
			bits[i/8] |= 1 << (i % 8)
		}
	}
	return ServerMap{
		Number:  m.Number,
		Name:    name,
		Width:   m.W,
		Height:  m.H,
		Blocked: base64.StdEncoding.EncodeToString(bits),
	}
}

func writeJSON(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
