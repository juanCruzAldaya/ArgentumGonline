package world

import (
	"testing"

	"juegito/server/internal/protocol"
)

// Death used to be inlined at three separate call sites that had already
// drifted apart. These pin what all three now share.
func TestKillTurnsTheVictimIntoAGhost(t *testing.T) {
	w := combatWorld(t)
	victim, _ := place(t, w, "victima", 5, 5)
	killer, _ := place(t, w, "asesino", 5, 6)
	victim.Body, victim.Head = 3, 4
	victim.Heading = protocol.North

	w.kill(victim, killer)

	if !victim.Dead {
		t.Fatal("the victim is not dead")
	}
	if victim.Body != ghostBody || victim.Head != ghostHead {
		t.Errorf("body/head = %d/%d, want Argentum's corpse %d/%d",
			victim.Body, victim.Head, ghostBody, ghostHead)
	}
	if victim.Heading != protocol.South {
		t.Errorf("heading = %v, want South: the source lays a corpse facing south", victim.Heading)
	}
	if victim.Vitals.HP != 0 {
		t.Errorf("HP = %d, want 0", victim.Vitals.HP)
	}
	if _, blocking := w.occupied[tileKey{5, 5}]; blocking {
		t.Error("the corpse still blocks its tile: it could wall off a doorway for the rest of the match")
	}
}

// The living spawn pool must not hand out the corpse — it did until the ghost
// went in, so a share of players spawned already looking dead, and with no
// sideways walk frames since a corpse has none.
func TestTheLiveSpawnPoolExcludesTheCorpseBody(t *testing.T) {
	for _, body := range availableBodies {
		if body == ghostBody {
			t.Errorf("body %d (iCuerpoMuerto) is in the live spawn pool", body)
		}
	}
	for _, head := range availableHeads {
		if head == ghostHead {
			t.Errorf("head %d (iCabezaMuerto) is in the live spawn pool", head)
		}
	}
}

func TestKillClearsStatusEffectsAndHiding(t *testing.T) {
	w := combatWorld(t)
	victim, _ := place(t, w, "victima", 5, 5)
	killer, _ := place(t, w, "asesino", 5, 6)

	victim.ParalyzedUntil = w.tick + 500
	victim.ImmobilizedUntil = w.tick + 500
	victim.InvisibleUntil = w.tick + 500
	victim.HiddenBySkill = true
	victim.AgilityDelta, victim.AgilityUntil = 5, w.tick+500
	victim.StrengthDelta, victim.StrengthUntil = -5, w.tick+500

	w.kill(victim, killer)

	if victim.paralyzed(w.tick) || victim.immobilized(w.tick) {
		t.Error("the ghost is still counting down a paralysis")
	}
	if victim.invisible(w.tick) {
		t.Error("the ghost is still hidden")
	}
	if victim.AgilityDelta != 0 || victim.StrengthDelta != 0 {
		t.Error("the ghost is still carrying attribute buffs")
	}
}

func TestKillCreditsTheKiller(t *testing.T) {
	w := combatWorld(t)
	killer, _ := place(t, w, "asesino", 5, 6)
	for i := 0; i < 3; i++ {
		victim, _ := place(t, w, "victima", 10+i, 10)
		w.kill(victim, killer)
	}
	if killer.Kills != 3 {
		t.Errorf("kills = %d, want 3", killer.Kills)
	}
}

// The black potion credits nobody: kill() takes a nil killer and must not
// panic reaching for one.
func TestASelfInflictedDeathCreditsNobody(t *testing.T) {
	w := combatWorld(t)
	p, _ := place(t, w, "wachin", 5, 5)

	w.kill(p, nil)

	if !p.Dead {
		t.Fatal("the player is not dead")
	}
	if p.Kills != 0 {
		t.Errorf("kills = %d, want 0: dying is not an achievement", p.Kills)
	}
}

func TestKillIsIdempotent(t *testing.T) {
	w := combatWorld(t)
	victim, _ := place(t, w, "victima", 5, 5)
	killer, _ := place(t, w, "asesino", 5, 6)

	w.kill(victim, killer)
	w.kill(victim, killer)

	if killer.Kills != 1 {
		t.Errorf("kills = %d, want 1: killing a corpse counted twice", killer.Kills)
	}
}

func TestSnapshotCarriesTheInspectionFields(t *testing.T) {
	w := combatWorld(t)
	me, _ := place(t, w, "wachin", 5, 5)
	other, _ := place(t, w, "rival", 6, 5)
	other.Class = Mago
	other.Kills = 7

	var seen *protocol.EntityState
	for i, e := range w.viewportOf(me) {
		if e.ID == uint32(other.ID) {
			seen = &w.viewportOf(me)[i]
		}
	}
	if seen == nil {
		t.Fatal("the other player is not in the viewport")
	}
	if seen.Desc != "Mago" {
		t.Errorf("Desc = %q, want %q", seen.Desc, "Mago")
	}
	if seen.Kills != 7 {
		t.Errorf("Kills = %d, want 7", seen.Kills)
	}
	if seen.Clan != "" {
		t.Errorf("Clan = %q, want empty: there is no guild system yet", seen.Clan)
	}
}
