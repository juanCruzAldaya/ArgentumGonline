package main

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"sort"
)

// Exports a composed world as a Tiled map, so somebody with an editor open can
// add detail to it and hand it back.
//
// Two things make this more than a dump. The first is that Argentum's graphics
// are not a grid: a grh is an arbitrary rectangle inside a numbered sheet, and
// no uniform tileset can describe that. So the tileset is a collection of
// images, one PNG per grh. The second is that every one of those PNGs is named
// after its grh number, which is what makes the trip back possible — an edited
// map still says, tile by tile, which Argentum graphic it meant.
//
// The layer names are the ones agreed with whoever is drawing: piso, objetos,
// encima, techo, and bloqueado for collision. They map one to one onto the four
// Argentum layers plus the server's blocked bitset.

// blockedTileGID is the marker painted on the bloqueado layer. It is its own
// synthetic tile — a translucent red square — rather than a real graphic,
// because collision is not something Argentum's art carries and an editor needs
// something visible to paint with.
const blockedTileName = "bloqueado"

type tiledTileset struct {
	Columns    int         `json:"columns"`
	FirstGID   int         `json:"firstgid"`
	Grid       *tiledGrid  `json:"grid,omitempty"`
	Margin     int         `json:"margin"`
	Name       string      `json:"name"`
	Spacing    int         `json:"spacing"`
	TileCount  int         `json:"tilecount"`
	TileHeight int         `json:"tileheight"`
	TileWidth  int         `json:"tilewidth"`
	Tiles      []tiledTile `json:"tiles"`
}

type tiledGrid struct {
	Height      int    `json:"height"`
	Orientation string `json:"orientation"`
	Width       int    `json:"width"`
}

type tiledTile struct {
	ID          int    `json:"id"`
	Image       string `json:"image"`
	ImageHeight int    `json:"imageheight"`
	ImageWidth  int    `json:"imagewidth"`
	Type        string `json:"type,omitempty"`
}

type tiledLayer struct {
	Compression string  `json:"compression"`
	Data        string  `json:"data"`
	Encoding    string  `json:"encoding"`
	Height      int     `json:"height"`
	ID          int     `json:"id"`
	Name        string  `json:"name"`
	Opacity     float64 `json:"opacity"`
	Type        string  `json:"type"`
	Visible     bool    `json:"visible"`
	Width       int     `json:"width"`
	X           int     `json:"x"`
	Y           int     `json:"y"`
}

type tiledMap struct {
	CompressionLevel int            `json:"compressionlevel"`
	Height           int            `json:"height"`
	Infinite         bool           `json:"infinite"`
	Layers           []tiledLayer   `json:"layers"`
	NextLayerID      int            `json:"nextlayerid"`
	NextObjectID     int            `json:"nextobjectid"`
	Orientation      string         `json:"orientation"`
	RenderOrder      string         `json:"renderorder"`
	TiledVersion     string         `json:"tiledversion"`
	TileHeight       int            `json:"tileheight"`
	Tilesets         []tiledTileset `json:"tilesets"`
	TileWidth        int            `json:"tilewidth"`
	Type             string         `json:"type"`
	Version          string         `json:"version"`
	Width            int            `json:"width"`
}

// encodeLayer packs a layer's gids the way Tiled reads them fastest: little
// endian uint32s, zlib'd, base64'd. A 760x760 layer is 2,3 MB of raw gids and a
// few hundred KB once compressed, which is the difference between a file an
// editor opens and one it chews on.
func encodeLayer(gids []uint32) (string, error) {
	raw := make([]byte, len(gids)*4)
	for i, g := range gids {
		binary.LittleEndian.PutUint32(raw[i*4:], g)
	}
	var buf bytes.Buffer
	zw := zlib.NewWriter(&buf)
	if _, err := zw.Write(raw); err != nil {
		return "", err
	}
	if err := zw.Close(); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

// writeTileImage crops one grh out of its sheet and writes it as a PNG with
// Argentum's colour key already applied, so it arrives with real transparency
// rather than a black box.
func writeTileImage(path string, grhs map[int]Grh, sheets map[int]image.Image, assets string, num int) (int, int, error) {
	g, ok := grhs[num]
	if !ok {
		return 0, 0, fmt.Errorf("grh %d no existe", num)
	}
	// An animation is represented by its first frame: Tiled can animate a tile,
	// but a still is enough to draw over and keeps the grh number recoverable.
	if g.Animated() {
		if len(g.Anim) == 0 {
			return 0, 0, fmt.Errorf("grh %d es una animación vacía", num)
		}
		g, ok = grhs[g.Anim[0]]
		if !ok {
			return 0, 0, fmt.Errorf("grh %d referencia un frame inexistente", num)
		}
	}

	sheet, cached := sheets[g.File]
	if !cached {
		var err error
		sheet, err = openSheet(assets, g.File)
		if err != nil {
			return 0, 0, err
		}
		sheets[g.File] = sheet
	}

	rect := image.Rect(g.X, g.Y, g.X+g.W, g.Y+g.H)
	if !rect.In(sheet.Bounds()) {
		return 0, 0, fmt.Errorf("grh %d se sale de su hoja", num)
	}
	out := image.NewNRGBA(image.Rect(0, 0, g.W, g.H))
	draw.Draw(out, out.Bounds(), sheet, rect.Min, draw.Src)
	keyBlackToTransparent(out)

	f, err := os.Create(path)
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()
	if err := png.Encode(f, out); err != nil {
		return 0, 0, err
	}
	return g.W, g.H, nil
}

// blockedMarker writes the tile painted on the collision layer.
func blockedMarker(path string) error {
	img := image.NewNRGBA(image.Rect(0, 0, 32, 32))
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			a := uint8(90)
			// A border, so a field of them still reads as individual tiles.
			if x < 2 || y < 2 || x > 29 || y > 29 {
				a = 200
			}
			img.SetNRGBA(x, y, color.NRGBA{220, 40, 40, a})
		}
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

// writeTiled exports every composed world for an editor.
func writeTiled(mundosDir, layoutPath, waterPath, assets, outDir string, grhs map[int]Grh) error {
	worlds, err := loadWorlds(layoutPath)
	if err != nil {
		return err
	}
	water, err := loadWaterGrhs(waterPath)
	if err != nil {
		return err
	}

	tileDir := filepath.Join(outDir, "tiles")
	if err := os.MkdirAll(tileDir, 0o755); err != nil {
		return err
	}
	if err := blockedMarker(filepath.Join(tileDir, blockedTileName+".png")); err != nil {
		return err
	}

	sheets := map[int]image.Image{}
	written := map[int]bool{}
	sizes := map[int][2]int{}

	for _, world := range worlds {
		built, _, err := buildWorld(mundosDir, world, water)
		if err != nil {
			return err
		}

		// Every graphic this world names, in a stable order, so gids do not
		// shuffle between runs.
		used := map[int]bool{}
		for i := range built.Tiles {
			for _, grh := range built.Tiles[i].Layers {
				if grh > 0 {
					used[grh] = true
				}
			}
		}
		order := make([]int, 0, len(used))
		for grh := range used {
			order = append(order, grh)
		}
		sort.Ints(order)

		// gid 1 is the collision marker; the graphics follow.
		gidOf := map[int]uint32{}
		tiles := []tiledTile{{
			ID: 0, Image: "tiles/" + blockedTileName + ".png",
			ImageWidth: 32, ImageHeight: 32, Type: blockedTileName,
		}}
		skipped := 0
		for _, grh := range order {
			path := filepath.Join(tileDir, fmt.Sprintf("%d.png", grh))
			w, h := 0, 0
			if written[grh] {
				w, h = sizes[grh][0], sizes[grh][1]
			} else {
				w, h, err = writeTileImage(path, grhs, sheets, assets, grh)
				if err != nil {
					// Twenty year old data has holes; one missing graphic is
					// not a reason to refuse the export.
					skipped++
					continue
				}
				written[grh] = true
				sizes[grh] = [2]int{w, h}
			}
			gidOf[grh] = uint32(len(tiles) + 1)
			tiles = append(tiles, tiledTile{
				ID: len(tiles), Image: fmt.Sprintf("tiles/%d.png", grh),
				ImageWidth: w, ImageHeight: h,
			})
		}

		side := built.W
		names := []string{"piso", "objetos", "encima", "techo"}
		layers := make([]tiledLayer, 0, 5)
		for n, name := range names {
			gids := make([]uint32, side*side)
			for i := range built.Tiles {
				if grh := built.Tiles[i].Layers[n]; grh > 0 {
					gids[i] = gidOf[grh]
				}
			}
			data, err := encodeLayer(gids)
			if err != nil {
				return err
			}
			layers = append(layers, tiledLayer{
				Compression: "zlib", Data: data, Encoding: "base64",
				Height: side, Width: side, ID: len(layers) + 1, Name: name,
				Opacity: 1, Type: "tilelayer", Visible: true,
			})
		}

		blocked := make([]uint32, side*side)
		for i := range built.Tiles {
			if built.Tiles[i].Blocked {
				blocked[i] = 1
			}
		}
		data, err := encodeLayer(blocked)
		if err != nil {
			return err
		}
		layers = append(layers, tiledLayer{
			Compression: "zlib", Data: data, Encoding: "base64",
			Height: side, Width: side, ID: len(layers) + 1, Name: blockedTileName,
			Opacity: 0.5, Type: "tilelayer", Visible: true,
		})

		m := tiledMap{
			CompressionLevel: -1, Height: side, Width: side,
			Infinite: false, Layers: layers,
			NextLayerID: len(layers) + 1, NextObjectID: 1,
			Orientation: "orthogonal", RenderOrder: "right-down",
			TiledVersion: "1.10", Version: "1.10", Type: "map",
			TileWidth: 32, TileHeight: 32,
			Tilesets: []tiledTileset{{
				FirstGID: 1, Name: "argentum", Columns: 0,
				Grid:      &tiledGrid{Orientation: "orthogonal", Width: 32, Height: 32},
				TileCount: len(tiles), TileWidth: 32, TileHeight: 32, Tiles: tiles,
			}},
		}

		path := filepath.Join(outDir, world.Name+".tmj")
		raw, err := json.Marshal(m)
		if err != nil {
			return err
		}
		if err := os.WriteFile(path, raw, 0o644); err != nil {
			return err
		}
		info, _ := os.Stat(path)
		fmt.Printf("%-10s %dx%d, %d tiles distintos, %d KB",
			world.Name+".tmj", side, side, len(tiles)-1, info.Size()/1024)
		if skipped > 0 {
			fmt.Printf(", %d gráficos ilegibles omitidos", skipped)
		}
		fmt.Println()
	}

	fmt.Printf("tiles/     %d PNG\n", len(written)+1)
	return nil
}
