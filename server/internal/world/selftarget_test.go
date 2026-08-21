package world

import (
	"testing"

	"juegito/server/internal/protocol"
)

const spellBolt = 109

// selfTargetWorld adds a damaging spell to the status fixtures, which are all
// status effects and carry no damage of their own.
func selfTargetWorld(t *testing.T) *World {
	t.Helper()
	w := statusWorld(t)
	w.spells[spellBolt] = Spell{
		ID: spellBolt, Name: "Descarga Eléctrica", Target: targetBoth, Mana: 10,
		AffectsHP: effectDamages, MinHP: 20, MaxHP: 20,
	}
	return w
}

// Casting something hostile on yourself is refused outright. A misclick in the
// middle of a fight used to be able to kill you.
func TestOffensiveSpellsCannotBeCastOnYourself(t *testing.T) {
	for _, tc := range []struct {
		name    string
		spellID int
	}{
		{"daño", spellBolt},
		{"paralizar", spellParalyze},
		{"inmovilizar", spellImmobilize},
		{"torpeza", spellSlowDebuff},
		{"debilidad", spellFrailDebuff},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := selfTargetWorld(t)
			p, conn := place(t, w, "wachin", 5, 5)
			castKnown(w, p, tc.spellID)
			hpBefore, manaBefore := p.Vitals.HP, p.Vitals.Mana

			w.cast(p, tc.spellID, p.ID)

			if p.Vitals.HP != hpBefore {
				t.Errorf("se hizo %d de daño a si mismo", hpBefore-p.Vitals.HP)
			}
			if p.paralyzed(w.tick) {
				t.Error("se paralizo a si mismo")
			}
			if p.AgilityDelta < 0 || p.StrengthDelta < 0 {
				t.Errorf("se debilito a si mismo: agi=%d fue=%d", p.AgilityDelta, p.StrengthDelta)
			}
			// Un error no puede costar mana: si cobrara, el misclick seguiria
			// teniendo precio aunque no haga daño.
			if p.Vitals.Mana != manaBefore {
				t.Errorf("mana %d -> %d, el intento no tiene que costar nada", manaBefore, p.Vitals.Mana)
			}
			var event protocol.SpellEvent
			if err := w.codec.DecodePayload(conn.lastOfType(t, protocol.TypeSpell), &event); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if event.Failed == "" {
				t.Error("no se le dijo por que no puede")
			}
		})
	}
}

// La otra mitad de la regla: lanzarse algo a favor sigue siendo legal, que es
// para lo que existe la mayoria de los hechizos de apoyo.
func TestSupportSpellsStillLandOnYourself(t *testing.T) {
	w := selfTargetWorld(t)
	p, _ := place(t, w, "wachin", 5, 5)

	castKnown(w, p, spellHasteBuff)
	w.cast(p, spellHasteBuff, p.ID)
	if p.AgilityDelta <= 0 {
		t.Errorf("no se pudo dar celeridad a si mismo: delta=%d", p.AgilityDelta)
	}

	castKnown(w, p, spellInvisible)
	w.cast(p, spellInvisible, p.ID)
	if !p.invisible(w.tick) {
		t.Error("no se pudo volver invisible a si mismo")
	}
}
