package main

import (
	"strings"
	"testing"
)

// The bug this guards against is not a crash: it is a server that comes up
// healthy-looking on an empty arena with no items and no spells, which is what
// reached production once. /healthz has to be able to say which of the two it
// is.
func TestGameStatusTellsALoadedWorldFromAnEmptyOne(t *testing.T) {
	loaded := gameStatus("Ciudad de Ullathorpe", 491, 50)
	if !strings.HasPrefix(loaded, "ok ") {
		t.Fatalf("un mundo cargado tiene que reportar ok, dio %q", loaded)
	}
	for _, want := range []string{"Ciudad de Ullathorpe", "items=491", "spells=50"} {
		if !strings.Contains(loaded, want) {
			t.Errorf("el estado no menciona %q: %q", want, loaded)
		}
	}

	for _, tc := range []struct {
		name          string
		mapName       string
		items, spells int
		wantMention   string
	}{
		{"sin nada", "", 0, 0, "sin mapa"},
		{"sin items", "Ciudad de Ullathorpe", 0, 50, "sin items"},
		{"sin hechizos", "Ciudad de Ullathorpe", 491, 0, "sin hechizos"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := gameStatus(tc.mapName, tc.items, tc.spells)
			if !strings.HasPrefix(got, "degradado:") {
				t.Fatalf("faltando datos el estado tiene que ser degradado, dio %q", got)
			}
			if !strings.Contains(got, tc.wantMention) {
				t.Errorf("el estado no dice qué falta (%q): %q", tc.wantMention, got)
			}
		})
	}
}
