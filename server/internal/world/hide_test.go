package world

import (
	"testing"

	"juegito/server/internal/protocol"
)

func TestHideMakesInvisible(t *testing.T) {
	w := statusWorld(t)
	p, _ := place(t, w, "ladron", 10, 10)

	w.hide(p)

	if !p.invisible(w.tick) {
		t.Fatal("hide() did not make the player invisible")
	}
	if !p.HiddenBySkill {
		t.Error("hide() did not mark the invisibility as skill-based")
	}
}

func TestHideRespectsCooldown(t *testing.T) {
	w := statusWorld(t)
	p, _ := place(t, w, "ladron", 10, 10)

	w.hide(p)
	p.InvisibleUntil = 0 // simulate having been revealed, without waiting out the cooldown
	w.hide(p)

	if p.invisible(w.tick) {
		t.Error("a second hide() inside the cooldown window took effect")
	}
}

func TestHideIsANoOpWhileAlreadyHidden(t *testing.T) {
	w := statusWorld(t)
	p, _ := place(t, w, "ladron", 10, 10)

	w.hide(p)
	first := p.InvisibleUntil
	w.tick += 5 // still well inside the hidden window
	w.hide(p)

	if p.InvisibleUntil != first {
		t.Error("hiding again while already hidden refreshed the duration; the source's own gate should have blocked it")
	}
}

func TestParalyzedPlayersCannotHide(t *testing.T) {
	w := statusWorld(t)
	p, _ := place(t, w, "ladron", 10, 10)
	p.applyParalysis(w.tick + 1000)

	w.hide(p)

	if p.invisible(w.tick) {
		t.Error("a paralyzed player hid")
	}
}

func TestDeadPlayersCannotHide(t *testing.T) {
	w := statusWorld(t)
	p, _ := place(t, w, "ladron", 10, 10)
	p.Dead = true

	w.hide(p)

	if p.invisible(w.tick) {
		t.Error("a dead player hid")
	}
}

// Only Ladron and Bandido keep walking while hidden in the source; everyone
// else is revealed the moment they take a real step.
func TestMovingRevealsHiddenPlayersExceptThiefAndBandit(t *testing.T) {
	w := statusWorld(t)
	p, _ := place(t, w, "guerrero", 10, 10)
	arm(w, p, Guerrero, 0, 0, 0)
	w.hide(p)

	w.movePlayer(p, protocol.East)

	if p.X != 11 {
		t.Fatalf("player did not actually move (x=%d), the test proves nothing", p.X)
	}
	if p.invisible(w.tick) {
		t.Error("a non-thief, non-bandit stayed hidden after moving")
	}
}

func TestThiefStaysHiddenWhileMoving(t *testing.T) {
	w := statusWorld(t)
	p, _ := place(t, w, "ladron", 10, 10)
	arm(w, p, Ladron, 0, 0, 0)
	w.hide(p)

	w.movePlayer(p, protocol.East)

	if p.X != 11 {
		t.Fatalf("player did not actually move (x=%d), the test proves nothing", p.X)
	}
	if !p.invisible(w.tick) {
		t.Error("a thief was revealed by moving while hidden")
	}
}

func TestBanditStaysHiddenWhileMoving(t *testing.T) {
	w := statusWorld(t)
	p, _ := place(t, w, "bandido", 10, 10)
	arm(w, p, Bandido, 0, 0, 0)
	w.hide(p)

	w.movePlayer(p, protocol.East)

	if !p.invisible(w.tick) {
		t.Error("a bandit was revealed by moving while hidden")
	}
}

// The spell shares the same InvisibleUntil field as Ocultarse but is a
// different mechanism in the source: it does not carry the "moving reveals
// you" rule at all, thief/bandit or not.
func TestMovingDoesNotBreakSpellInvisibility(t *testing.T) {
	w := statusWorld(t)
	caster, _ := place(t, w, "mago", 10, 10)
	arm(w, caster, Guerrero, 0, 0, 0) // deliberately NOT a thief/bandit
	castKnown(w, caster, spellInvisible)

	w.cast(caster, spellInvisible, caster.ID)
	if !caster.invisible(w.tick) {
		t.Fatal("the invisibility spell did not take effect")
	}

	w.movePlayer(caster, protocol.East)

	if !caster.invisible(w.tick) {
		t.Error("spell-based invisibility broke on movement; only Ocultarse should do that")
	}
}

func TestAttackingRevealsAHiddenPlayer(t *testing.T) {
	w := statusWorld(t)
	attacker, _ := place(t, w, "atacante", 10, 10)
	arm(w, attacker, Guerrero, 1, 0, 0)
	victim, _ := place(t, w, "victima", 10, 11)
	arm(w, victim, Guerrero, 0, 0, 0)
	attacker.Heading = protocol.South
	w.hide(attacker)

	w.attack(attacker)

	if attacker.invisible(w.tick) {
		t.Error("attacking did not reveal a hidden player")
	}
}

func TestCastingRevealsAHiddenPlayer(t *testing.T) {
	w := statusWorld(t)
	caster, _ := place(t, w, "mago", 10, 10)
	castKnown(w, caster, spellCureStatus)
	w.hide(caster)

	w.cast(caster, spellCureStatus, caster.ID)

	if caster.invisible(w.tick) {
		t.Error("casting did not reveal a hidden player")
	}
}
