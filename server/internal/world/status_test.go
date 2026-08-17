package world

import (
	"testing"

	"juegito/server/internal/protocol"
)

// Synthetic spell ids used across these tests. Real Hechizos.dat ids (9, 10,
// 14, 18-21) would work identically; small distinct numbers just make test
// failures easier to read.
const (
	spellParalyze    = 101
	spellImmobilize  = 102
	spellCureStatus  = 103
	spellInvisible   = 104
	spellHasteBuff   = 105
	spellSlowDebuff  = 106
	spellWeakBuff    = 107
	spellFrailDebuff = 108
)

func statusWorld(t *testing.T) *World {
	t.Helper()
	w := combatWorld(t) // already sets w.tick past every cooldown and loads items
	w.SetSpells(map[int]Spell{
		spellParalyze:    {ID: spellParalyze, Name: "Paralizar", Target: targetBoth, Mana: 10, Paralyzes: true},
		spellImmobilize:  {ID: spellImmobilize, Name: "Inmovilizar", Target: targetBoth, Mana: 10, Immobilizes: true},
		spellCureStatus:  {ID: spellCureStatus, Name: "Devolver Movilidad", Target: targetBoth, Mana: 10, RemovesParalysis: true},
		spellInvisible:   {ID: spellInvisible, Name: "Invisibilidad", Target: targetUser, Mana: 10, Invisibility: true},
		spellHasteBuff:   {ID: spellHasteBuff, Name: "Celeridad", Target: targetUser, Mana: 10, AffectsAgility: attributeBuff, MinAgility: 5, MaxAgility: 5},
		spellSlowDebuff:  {ID: spellSlowDebuff, Name: "Torpeza", Target: targetBoth, Mana: 10, AffectsAgility: attributeDebuff, MinAgility: 5, MaxAgility: 5},
		spellWeakBuff:    {ID: spellWeakBuff, Name: "Fuerza", Target: targetUser, Mana: 10, AffectsStrength: attributeBuff, MinStrength: 5, MaxStrength: 5},
		spellFrailDebuff: {ID: spellFrailDebuff, Name: "Debilidad", Target: targetBoth, Mana: 10, AffectsStrength: attributeDebuff, MinStrength: 5, MaxStrength: 5},
	})
	return w
}

// castKnown makes p able to cast spellID, so the tests below fail on the rule
// they are actually about rather than on a precondition.
//
// The mana line is not incidental. place() rolls a random class, and manaFor
// gives five of the twelve classes no mana whatsoever — a caster that happened
// to roll Guerrero would fail every cast here on the mana gate. Pinning a
// round 100 keeps that out of the way and keeps the spell-cost assertions
// ("mana = 90 after one 10-cost cast") readable.
func castKnown(w *World, p *Player, spellID int) {
	p.Spells = []int{spellID}
	p.lastCastTick = 0 // clear any cooldown from a previous cast in the same test
	p.Vitals.Mana, p.Vitals.MaxMana = 100, 100
}

func TestParalysisBlocksMovementAndMelee(t *testing.T) {
	w := statusWorld(t)
	caster, _ := place(t, w, "mago", 10, 10)
	victim, _ := place(t, w, "victima", 10, 11)
	arm(w, victim, Guerrero, 1, 0, 0)
	castKnown(w, caster, spellParalyze)

	w.cast(caster, spellParalyze, victim.ID)
	if !victim.paralyzed(w.tick) {
		t.Fatal("victim was not marked paralyzed")
	}

	before := [2]int{victim.X, victim.Y}
	w.movePlayer(victim, protocol.North)
	if [2]int{victim.X, victim.Y} != before {
		t.Error("a paralyzed player moved")
	}

	// Facing south, away from the caster standing north of the victim at
	// (10,10) — otherwise the target below would collide with the caster's
	// own tile.
	victim.Heading = protocol.South
	target, _ := place(t, w, "objetivo", 10, 12) // adjacent: one tile south
	full := target.Vitals.HP

	// attack() gates on canAct before it ever looks at the attack cooldown, so
	// this does not need to advance the tick between attempts — and must not:
	// paralysisDurationTicks is far shorter than an attack-cooldown-spaced
	// loop would run for, so advancing it here would let paralysis expire
	// mid-loop and land a hit for the wrong reason.
	for i := 0; i < 5; i++ {
		w.attack(victim)
	}
	if target.Vitals.HP != full {
		t.Error("a paralyzed player landed a melee hit")
	}
}

// The source's PuedeLanzar never checks Paralizado: a paralyzed character can
// still cast, which is the whole point of the spell — stunned, not helpless.
// Casting Devolver Movilidad on yourself while paralyzed is the concrete case
// that matters: it is your only way out of the stun.
func TestParalysisDoesNotBlockCasting(t *testing.T) {
	w := statusWorld(t)
	caster, _ := place(t, w, "mago", 10, 10)
	castKnown(w, caster, spellCureStatus)
	caster.applyParalysis(w.tick + 1000)

	w.cast(caster, spellCureStatus, caster.ID)

	if caster.paralyzed(w.tick) {
		t.Error("a paralyzed caster could not cast Devolver Movilidad on themself")
	}
	if caster.Vitals.Mana != 90 {
		t.Errorf("mana = %d, want 90: the cast should have gone through and been paid for", caster.Vitals.Mana)
	}
}

func TestImmobilizeBlocksMovementButNotMelee(t *testing.T) {
	w := statusWorld(t)
	caster, _ := place(t, w, "mago", 10, 10)
	victim, _ := place(t, w, "victima", 10, 11)
	arm(w, victim, Guerrero, 1, 0, 0)
	castKnown(w, caster, spellImmobilize)

	w.cast(caster, spellImmobilize, victim.ID)
	if !victim.immobilized(w.tick) {
		t.Fatal("victim was not marked immobilized")
	}

	before := [2]int{victim.X, victim.Y}
	w.movePlayer(victim, protocol.North)
	if [2]int{victim.X, victim.Y} != before {
		t.Error("an immobilized player moved")
	}

	// Facing south, away from the caster standing north of the victim — see
	// the identical note in TestParalysisBlocksMovementAndMelee.
	victim.Heading = protocol.South
	target, _ := place(t, w, "objetivo", 10, 12) // adjacent: one tile south
	arm(w, target, Guerrero, 0, 0, 0)
	full := target.Vitals.HP
	landed := false
	for i := 0; i < 60 && !landed; i++ {
		w.tick += attackCooldownTicks
		w.attack(victim)
		landed = target.Vitals.HP != full
	}
	if !landed {
		t.Error("an immobilized player never landed a melee hit in sixty swings")
	}
}

func TestRemoveParalysisClearsBothParalysisAndImmobilize(t *testing.T) {
	w := statusWorld(t)
	caster, _ := place(t, w, "mago", 10, 10)
	castKnown(w, caster, spellCureStatus)

	caster.applyImmobilize(w.tick + 1000)
	w.cast(caster, spellCureStatus, caster.ID)
	if caster.immobilized(w.tick) {
		t.Error("immobilize survived Devolver Movilidad")
	}

	caster.Vitals.Mana = 100
	caster.lastCastTick = 0 // bypass the cast cooldown for this second cast
	caster.applyParalysis(w.tick + 1000)
	w.cast(caster, spellCureStatus, caster.ID)
	if caster.paralyzed(w.tick) {
		t.Error("paralysis survived Devolver Movilidad")
	}
}

func TestCastingEitherStatusClearsTheOther(t *testing.T) {
	w := statusWorld(t)
	caster, _ := place(t, w, "mago", 10, 10)
	victim, _ := place(t, w, "victima", 10, 11)

	victim.applyParalysis(w.tick + 1000)
	castKnown(w, caster, spellImmobilize)
	w.cast(caster, spellImmobilize, victim.ID)
	if victim.paralyzed(w.tick) {
		t.Error("casting Inmovilizar left the old paralysis active too")
	}
	if !victim.immobilized(w.tick) {
		t.Error("Inmovilizar did not take effect")
	}
}

func TestInvisiblePlayerIsHiddenFromOthersButNotSelf(t *testing.T) {
	w := statusWorld(t)
	caster, casterConn := place(t, w, "fantasma", 50, 50)
	_, otherConn := place(t, w, "otro", 51, 50)
	castKnown(w, caster, spellInvisible)

	w.cast(caster, spellInvisible, caster.ID)
	if !caster.invisible(w.tick) {
		t.Fatal("caster was not marked invisible")
	}

	w.step()

	var selfSnap protocol.Snapshot
	if err := w.codec.DecodePayload(casterConn.lastOfType(t, protocol.TypeSnapshot), &selfSnap); err != nil {
		t.Fatalf("decode: %v", err)
	}
	foundSelf := false
	for _, e := range selfSnap.Entities {
		if e.ID == uint32(caster.ID) {
			foundSelf = true
		}
	}
	if !foundSelf {
		t.Error("an invisible player cannot see themself")
	}

	var otherSnap protocol.Snapshot
	if err := w.codec.DecodePayload(otherConn.lastOfType(t, protocol.TypeSnapshot), &otherSnap); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, e := range otherSnap.Entities {
		if e.ID == uint32(caster.ID) {
			t.Error("an invisible player showed up in someone else's viewport")
		}
	}
}

// A modified client could still send a target id it inferred some other way
// (e.g. from an earlier snapshot, before the target turned invisible). The
// server has to re-check, not just decline to advertise the target.
func TestCannotTargetAnInvisiblePlayer(t *testing.T) {
	w := statusWorld(t)
	caster, casterConn := place(t, w, "atacante", 50, 50)
	victim, _ := place(t, w, "invisible", 51, 50)
	victim.InvisibleUntil = w.tick + 1000
	castKnown(w, caster, spellParalyze)

	w.cast(caster, spellParalyze, victim.ID)

	if victim.paralyzed(w.tick) {
		t.Error("landed a targeted spell on a player that should not be visible")
	}
	var event protocol.SpellEvent
	if err := w.codec.DecodePayload(casterConn.lastOfType(t, protocol.TypeSpell), &event); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if event.Failed == "" {
		t.Error("expected a Failed reason")
	}
}

func TestAgilityBuffRaisesEffectiveAttributeCappedAtDoubleBase(t *testing.T) {
	w := statusWorld(t)
	caster, _ := place(t, w, "veloz", 10, 10)
	arm(w, caster, Guerrero, 0, 0, 0)
	caster.Attributes.Agilidad = 20
	castKnown(w, caster, spellHasteBuff) // rolls a flat +5 in this test's table

	w.cast(caster, spellHasteBuff, caster.ID)

	if got := w.effectiveAttributes(caster).Agilidad; got != 25 {
		t.Errorf("effective agility = %d, want 25 (20 base + 5 buff)", got)
	}

	// A buff big enough to push past double the base must be clamped there,
	// not applied in full.
	caster.Attributes.Agilidad = 3
	caster.lastCastTick = 0                   // bypass the cast cooldown for this second cast
	w.cast(caster, spellHasteBuff, caster.ID) // rolls +5, base is 3, cap is +3
	if got := w.effectiveAttributes(caster).Agilidad; got != 6 {
		t.Errorf("effective agility = %d, want 6 (3 base, capped at double)", got)
	}
}

func TestStrengthDebuffFloorsAtOne(t *testing.T) {
	w := statusWorld(t)
	caster, _ := place(t, w, "mago", 10, 10)
	victim, _ := place(t, w, "debil", 10, 11)
	arm(w, victim, Guerrero, 0, 0, 0)
	victim.Attributes.Fuerza = 3
	castKnown(w, caster, spellFrailDebuff) // rolls a flat -5

	w.cast(caster, spellFrailDebuff, victim.ID)

	if got := w.effectiveAttributes(victim).Fuerza; got != 1 {
		t.Errorf("effective strength = %d, want 1 (floored, not negative)", got)
	}
}

func TestBuffExpiresAfterItsDuration(t *testing.T) {
	w := statusWorld(t)
	caster, _ := place(t, w, "mago", 10, 10)
	arm(w, caster, Guerrero, 0, 0, 0)
	caster.Attributes.Agilidad = 20
	castKnown(w, caster, spellHasteBuff)

	w.cast(caster, spellHasteBuff, caster.ID)
	if w.effectiveAttributes(caster).Agilidad == caster.Attributes.Agilidad {
		t.Fatal("buff never applied")
	}

	w.tick += buffDurationTicks + 1
	if got := w.effectiveAttributes(caster).Agilidad; got != caster.Attributes.Agilidad {
		t.Errorf("agility = %d, want back to base %d after the buff expired", got, caster.Attributes.Agilidad)
	}
}
