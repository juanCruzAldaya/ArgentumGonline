package world

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"
)

// El sonido de un hechizo no lo usa el servidor —lo hace sonar el cliente— pero
// vive en el mismo spells.json que sí lee, y por eso se chequea acá.
//
// Este test existe porque el campo estuvo bien en el conversor y mal en los
// datos: aoconv leía WAV= de Hechizos.dat y lo emitía, pero el spells.json
// commiteado era de antes y no lo tenía, así que los 50 hechizos se lanzaban en
// silencio. Nada fallaba: el cliente pedía el sonido 0, que no existe, y no
// pasaba nada. Un dato viejo no rompe nada, que es exactamente lo que lo hace
// difícil de ver.
func TestEveryShippedSpellCarriesItsSound(t *testing.T) {
	raw, err := os.ReadFile("../../maps/spells.json")
	if err != nil {
		t.Fatalf("leyendo spells.json: %v", err)
	}
	var spells map[int]struct {
		Name string `json:"name"`
		Wav  int    `json:"wav"`
	}
	if err := json.Unmarshal(raw, &spells); err != nil {
		t.Fatalf("parseando spells.json: %v", err)
	}
	if len(spells) == 0 {
		t.Fatal("spells.json vacío")
	}

	for id, spell := range spells {
		if spell.Wav == 0 {
			t.Errorf("el hechizo %d (%s) no trae wav: regenerá los datos con tools/aoconv (OPERACION §3)",
				id, spell.Name)
			continue
		}
		// Y que el sonido que nombra esté efectivamente convertido: la lista de
		// tools/aoconv/sounds.go es a mano, así que un hechizo puede nombrar un
		// WAV que nadie empaquetó y volver al mismo silencio por otro camino.
		path := fmt.Sprintf("../../../client/assets/ao/sfx/%d.wav", spell.Wav)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("el hechizo %d (%s) suena el wav %d, que no está convertido: %v",
				id, spell.Name, spell.Wav, err)
		}
	}
}
