package world

import (
	"testing"

	"juegito/server/internal/protocol"
)

// The expected numbers below are not this port's inventions: each one is
// initialMana + 44 * manaPerLevel for that class at that race's Inteligencia,
// which is what replaying SetAttributesToNewUser plus 44 SubirNivel calls in
// the VB6 source produces. They are spelled out rather than recomputed from the
// same helpers under test, so a change to either formula fails here loudly
// instead of agreeing with itself.
//
// Inteligencia here is baseAttribute (20, the original's MaxDados) plus the
// race modifier: Humano 20, Elfo/Drow 22, Enano 18, Gnomo 24.
func TestManaAtTheLevelCapMatchesTheSourceFormulas(t *testing.T) {
	tests := []struct {
		class Class
		race  Race
		want  int
	}{
		// Mago: INT*3 at level 1, then RoundToEven(2.8*INT) per level-up.
		{Mago, Gnomo, 3020},  // INT 24: 72 + 44*67
		{Mago, Elfo, 2794},   // INT 22: 66 + 44*62
		{Mago, Humano, 2524}, // INT 20: 60 + 44*56
		{Mago, Enano, 2254},  // INT 18: 54 + 44*50

		// The six classes the source starts at a flat 50 regardless of race,
		// which then diverge purely through their per-level INT multiple.
		{Clerigo, Gnomo, 2162}, // 50 + 44*48
		{Druida, Humano, 1810}, // 50 + 44*40
		{Bardo, Enano, 1634},   // 50 + 44*36
		{Paladin, Gnomo, 1106}, // 50 + 44*24, a plain 1*INT
		{Asesino, Humano, 930},
		{Bandido, Gnomo, 754}, // INT/3*2 = 16; 50 + 44*16

		// Non-casters. The source gives them nothing at level 1 and no
		// AumentoMANA arm, so they finish the climb at zero.
		{Guerrero, Gnomo, 0},
		{Cazador, Elfo, 0},
		{Pirata, Humano, 0},
		{Ladron, Drow, 0},
		{Trabajador, Enano, 0},
	}

	for _, tt := range tests {
		if got := manaFor(tt.class, tt.race); got != tt.want {
			t.Errorf("manaFor(%v, %v) = %d, want %d", tt.class, tt.race, got, tt.want)
		}
	}
}

// Race only reaches mana through Inteligencia, so a class whose formula never
// multiplies by INT must be race-blind — including the non-casters, whose zero
// no race can lift.
func TestRaceMovesManaOnlyForClassesWhoseFormulaUsesInteligencia(t *testing.T) {
	for _, class := range allClasses {
		var seen []int
		for _, race := range allRaces {
			seen = append(seen, manaFor(class, race))
		}
		usesInteligencia := manaPerLevel(class, 10) != manaPerLevel(class, 20)

		spread := false
		for _, v := range seen[1:] {
			if v != seen[0] {
				spread = true
			}
		}
		if spread != usesInteligencia {
			t.Errorf("%v: mana varies by race = %v, but its per-level formula uses Inteligencia = %v (%v)",
				class, spread, usesInteligencia, seen)
		}
	}
}

func TestSpawnVitalsCarryTheClassAndRaceMana(t *testing.T) {
	for _, class := range allClasses {
		for _, race := range allRaces {
			vitals := vitalsFor(class, race)
			want := manaFor(class, race)
			if vitals.MaxMana != want {
				t.Errorf("%v %v: MaxMana = %d, want %d", race, class, vitals.MaxMana, want)
			}
			// Everyone spawns topped up; startingVitals' placeholder 100 must
			// not survive for the classes whose real answer is 0.
			if vitals.Mana != want {
				t.Errorf("%v %v: spawned at %d/%d mana, want full", race, class, vitals.Mana, want)
			}
			if vitals.MaxMana > statMaxMana {
				t.Errorf("%v %v: MaxMana %d exceeds STAT_MAXMAN %d", race, class, vitals.MaxMana, statMaxMana)
			}
		}
	}
}

// A Guerrero knows the same spell list as a Mago in this game but can never
// pay for any of it. That is the source's design and the visible consequence of
// the zeroes above, so it is pinned here rather than left to be rediscovered.
func TestANonCasterCannotAffordASpell(t *testing.T) {
	w := statusWorld(t)
	caster, conn := place(t, w, "guerrero", 10, 10)
	victim, _ := place(t, w, "victima", 10, 11)

	caster.Class, caster.Race = Guerrero, Humano
	caster.Vitals = vitalsFor(Guerrero, Humano)
	caster.Spells = []int{spellParalyze}
	caster.lastCastTick = 0

	w.cast(caster, spellParalyze, victim.ID)
	if victim.paralyzed(w.tick) {
		t.Fatal("a Guerrero with 0 mana landed a paralysis")
	}
	// The mana gate has to say so, not swallow the cast: a player who can
	// never cast should learn why the first time they try.
	var event protocol.SpellEvent
	if err := w.codec.DecodePayload(conn.lastOfType(t, protocol.TypeSpell), &event); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if event.Failed == "" {
		t.Error("the failed cast reported no reason")
	}
}
