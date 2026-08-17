package world

import (
	"testing"

	"juegito/server/internal/protocol"
)

// arm gives a player a class, skills and equipment so combat is deterministic
// enough to assert on. Join rolls these at random, which is right for the game
// and useless for a test.
func arm(w *World, p *Player, class Class, weapon, shield, armor int) {
	p.Class = class
	p.Attributes = rolledAttributes(Humano)
	p.Skills = startingSkills
	p.Vitals = vitalsFor(class, Humano)
	p.Inventory = nil
	for _, id := range []int{weapon, shield, armor} {
		if id > 0 {
			p.Inventory = append(p.Inventory, protocol.InventorySlot{ItemID: id, Amount: 1, Equipped: true})
		}
	}
}

func combatWorld(t *testing.T) *World {
	t.Helper()
	w := newTestWorld(t, 40, 40)
	w.SetItems(map[int]Item{
		1: {ID: 1, Name: "Espada", Type: ItemWeapon, MinHit: 5, MaxHit: 10},
		2: {ID: 2, Name: "Escudo", Type: ItemShield, MinDef: 1, MaxDef: 2},
		3: {ID: 3, Name: "Armadura", Type: ItemArmor, MinDef: 3, MaxDef: 6},
	})
	w.tick = 1000 // past the initial attack cooldown
	return w
}

func TestAttackOnlyReachesTheTileYouFace(t *testing.T) {
	w := combatWorld(t)
	attacker, _ := place(t, w, "atacante", 10, 10)
	victim, _ := place(t, w, "victima", 10, 12) // two tiles south, out of reach
	arm(w, attacker, Guerrero, 1, 0, 0)
	arm(w, victim, Guerrero, 0, 0, 0)

	attacker.Heading = protocol.South
	full := victim.Vitals.HP
	w.attack(attacker)

	if victim.Vitals.HP != full {
		t.Errorf("hp = %d, want %d: melee reached two tiles away", victim.Vitals.HP, full)
	}
}

func TestAttackHitsTheAdjacentTile(t *testing.T) {
	w := combatWorld(t)
	attacker, _ := place(t, w, "atacante", 10, 10)
	victim, _ := place(t, w, "victima", 10, 11)
	arm(w, attacker, Guerrero, 1, 0, 0)
	arm(w, victim, Guerrero, 0, 0, 0)
	attacker.Heading = protocol.South

	// Hit chance is clamped to at most 90%, so one swing can legitimately
	// miss. Swinging repeatedly asserts that damage happens at all without
	// depending on the roll.
	full := victim.Vitals.HP
	for i := 0; i < 60 && victim.Vitals.HP == full; i++ {
		w.tick += attackCooldownTicks
		w.attack(attacker)
	}
	if victim.Vitals.HP == full {
		t.Error("sixty swings at an adjacent target never landed")
	}
}

func TestAttackRespectsCooldown(t *testing.T) {
	w := combatWorld(t)
	attacker, _ := place(t, w, "atacante", 10, 10)
	victim, _ := place(t, w, "victima", 10, 11)
	arm(w, attacker, Guerrero, 1, 0, 0)
	arm(w, victim, Guerrero, 0, 0, 0)
	attacker.Heading = protocol.South

	w.attack(attacker)
	swungAt := attacker.lastAttackTick
	w.attack(attacker)

	if attacker.lastAttackTick != swungAt {
		t.Error("a second swing landed inside the cooldown")
	}
}

func TestDeathEliminatesFromTheMatch(t *testing.T) {
	w := combatWorld(t)
	attacker, _ := place(t, w, "atacante", 10, 10)
	victim, _ := place(t, w, "victima", 10, 11)
	arm(w, attacker, Guerrero, 1, 0, 0)
	arm(w, victim, Mago, 0, 0, 0)
	attacker.Heading = protocol.South

	victim.Vitals.HP = 1
	for i := 0; i < 60 && !victim.Dead; i++ {
		w.tick += attackCooldownTicks
		w.attack(attacker)
	}

	if !victim.Dead {
		t.Fatal("victim never died")
	}
	if victim.Vitals.HP != 0 {
		t.Errorf("hp = %d, want 0", victim.Vitals.HP)
	}
	if w.aliveCount() != 1 {
		t.Errorf("alive = %d, want 1: the dead still count", w.aliveCount())
	}
	// A corpse must not wall off the tile it fell on.
	if _, blocking := w.occupied[tileKey{victim.X, victim.Y}]; blocking {
		t.Error("the body still blocks its tile")
	}
}

func TestTheDeadDoNotAct(t *testing.T) {
	w := combatWorld(t)
	corpse, _ := place(t, w, "muerto", 10, 10)
	victim, _ := place(t, w, "victima", 10, 11)
	arm(w, corpse, Guerrero, 1, 0, 0)
	arm(w, victim, Guerrero, 0, 0, 0)
	corpse.Heading = protocol.South
	corpse.Dead = true

	full := victim.Vitals.HP
	for i := 0; i < 10; i++ {
		w.tick += attackCooldownTicks
		w.attack(corpse)
	}
	if victim.Vitals.HP != full {
		t.Error("a dead player landed a hit")
	}

	before := [2]int{corpse.X, corpse.Y}
	w.movePlayer(corpse, protocol.North)
	if [2]int{corpse.X, corpse.Y} != before {
		t.Error("a dead player walked")
	}
}

// Argentum clamps every hit roll between 10 and 90 percent, so no matchup is
// ever hopeless and none is ever certain. That band is the balance, and it is
// worth pinning down.
func TestHitChanceStaysInsideArgentumsBand(t *testing.T) {
	w := combatWorld(t)
	strong, _ := place(t, w, "fuerte", 10, 10)
	weak, _ := place(t, w, "debil", 10, 11)

	arm(w, strong, Guerrero, 1, 0, 0)
	strong.Skills = Skills{Armas: 100, Wrestling: 100, Tacticas: 100, Defensa: 100}
	strong.Vitals.Level = 50
	arm(w, weak, Mago, 0, 0, 0)
	weak.Skills = Skills{}

	if chance := w.hitChance(strong, weak); chance > maxHitChance {
		t.Errorf("hit chance = %.1f, want at most %.1f", chance, maxHitChance)
	}
	if chance := w.hitChance(weak, strong); chance < minHitChance {
		t.Errorf("hit chance = %.1f, want at least %.1f", chance, minHitChance)
	}
}

func TestArmourReducesDamage(t *testing.T) {
	w := combatWorld(t)
	naked, _ := place(t, w, "desnudo", 10, 10)
	plated, _ := place(t, w, "blindado", 12, 12)
	arm(w, naked, Guerrero, 0, 0, 0)
	arm(w, plated, Guerrero, 0, 2, 3)

	if w.armorAbsorption(naked) != 0 {
		t.Error("an unarmoured player absorbed damage")
	}
	// Shields are evasion in Argentum, not absorption: only the armour and
	// helmet soak, so the floor here is the armour's own minimum.
	if absorbed := w.armorAbsorption(plated); absorbed < 3 {
		t.Errorf("absorbed %d, want at least the armour's 3", absorbed)
	}
}
