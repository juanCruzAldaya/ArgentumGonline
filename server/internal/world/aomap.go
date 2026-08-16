package world

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
)

// serverMap is the map file emitted by tools/aoconv.
//
// It deliberately carries no graphics. The simulation only needs to know where
// nobody can walk; keeping the tile artwork out means a modified client cannot
// learn anything about the map that the server does not already enforce.
type serverMap struct {
	Number  int    `json:"number"`
	Name    string `json:"name"`
	Width   int    `json:"width"`
	Height  int    `json:"height"`
	Blocked string `json:"blocked"`
}

// LoadMap reads a converted Argentum map and returns its collision grid.
func LoadMap(path string) (*Grid, int, string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, 0, "", err
	}

	var m serverMap
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, 0, "", fmt.Errorf("%s: %w", path, err)
	}
	if m.Width <= 0 || m.Height <= 0 {
		return nil, 0, "", fmt.Errorf("%s: dimensiones inválidas %dx%d", path, m.Width, m.Height)
	}

	bits, err := base64.StdEncoding.DecodeString(m.Blocked)
	if err != nil {
		return nil, 0, "", fmt.Errorf("%s: bitset ilegible: %w", path, err)
	}
	if want := (m.Width*m.Height + 7) / 8; len(bits) < want {
		return nil, 0, "", fmt.Errorf("%s: bitset de %d bytes, se esperaban %d", path, len(bits), want)
	}

	grid := NewGrid(m.Width, m.Height)
	for i := 0; i < m.Width*m.Height; i++ {
		if bits[i/8]&(1<<(i%8)) != 0 {
			grid.SetBlocked(i%m.Width, i/m.Width, true)
		}
	}
	return grid, m.Number, m.Name, nil
}
