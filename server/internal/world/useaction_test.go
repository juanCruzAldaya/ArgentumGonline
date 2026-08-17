package world

import (
	"testing"

	"juegito/server/internal/protocol"
)

// The point of separating E from U is that neither ever quietly does the
// other's job. A mistyped equip must not drink anything, and a mistyped use
// must not put a sword on.
func TestEquipActionRefusesAConsumable(t *testing.T) {
	w := itemWorld(t)
	p, conn := place(t, w, "wachin", 5, 5)
	p.Inventory = []protocol.InventorySlot{{Slot: 0, ItemID: 10, Amount: 5}} // Poción Roja
	p.Vitals.HP = 1

	w.useItem(p, 0, protocol.UseEquip)

	if p.Vitals.HP != 1 {
		t.Error("an equip action drank the potion")
	}
	if p.Inventory[0].Amount != 5 {
		t.Errorf("stack = %d, want 5 untouched", p.Inventory[0].Amount)
	}
	var result protocol.UseResult
	if err := w.codec.DecodePayload(conn.lastOfType(t, protocol.TypeUseResult), &result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result.Failed == "" {
		t.Error("the refusal was silent; the player has no way to know why nothing happened")
	}
}

func TestUseActionRefusesEquipment(t *testing.T) {
	w := itemWorld(t)
	p, conn := place(t, w, "wachin", 5, 5)
	p.Inventory = []protocol.InventorySlot{{Slot: 0, ItemID: 1, Amount: 1}} // Espada Vieja

	w.useItem(p, 0, protocol.UseUseUp)

	if p.Inventory[0].Equipped {
		t.Error("a use action equipped the sword")
	}
	if len(p.Inventory) != 1 || p.Inventory[0].Amount != 1 {
		t.Errorf("inventory = %+v: the sword was consumed", p.Inventory)
	}
	var result protocol.UseResult
	if err := w.codec.DecodePayload(conn.lastOfType(t, protocol.TypeUseResult), &result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result.Failed == "" {
		t.Error("the refusal was silent")
	}
}

// The overloaded click is still what a double-click sends, so the original's
// own "the item type picks the branch" behaviour has to survive intact.
func TestAutoActionStillBranchesOnItemType(t *testing.T) {
	w := itemWorld(t)
	p, _ := place(t, w, "wachin", 5, 5)
	p.Inventory = []protocol.InventorySlot{
		{Slot: 0, ItemID: 1, Amount: 1},  // Espada Vieja
		{Slot: 1, ItemID: 10, Amount: 5}, // Poción Roja
	}
	p.Vitals.HP = 1

	w.useItem(p, 0, protocol.UseAuto)
	if !p.Inventory[0].Equipped {
		t.Error("auto did not equip the sword")
	}

	w.tick += useCooldownTicks
	w.useItem(p, 1, protocol.UseAuto)
	if p.Vitals.HP == 1 {
		t.Error("auto did not drink the potion")
	}
}

func TestEquipActionStillEquips(t *testing.T) {
	w := itemWorld(t)
	p, _ := place(t, w, "wachin", 5, 5)
	p.Inventory = []protocol.InventorySlot{{Slot: 0, ItemID: 1, Amount: 1}}

	w.useItem(p, 0, protocol.UseEquip)

	if !p.Inventory[0].Equipped {
		t.Error("the equip action did not equip")
	}
}

// Stamina is deliberately not spent: a battle royale decided by who ran out of
// energy is not the game this is trying to be. Hechizos.dat's own cost is still
// parsed, so this pins the decision rather than the absence of a field.
func TestCastingDoesNotSpendStamina(t *testing.T) {
	w := statusWorld(t)
	caster, _ := place(t, w, "mago", 10, 10)
	victim, _ := place(t, w, "victima", 10, 11)
	castKnown(w, caster, spellParalyze)
	caster.Vitals.Stamina = 100

	w.cast(caster, spellParalyze, victim.ID)

	if !victim.paralyzed(w.tick) {
		t.Fatal("the cast did not land")
	}
	if caster.Vitals.Stamina != 100 {
		t.Errorf("stamina = %d, want 100 untouched", caster.Vitals.Stamina)
	}
}

// Zero stamina must not block a cast either — the gate is gone, not just the
// subtraction.
func TestZeroStaminaDoesNotBlockCasting(t *testing.T) {
	w := statusWorld(t)
	caster, _ := place(t, w, "mago", 10, 10)
	victim, _ := place(t, w, "victima", 10, 11)
	castKnown(w, caster, spellParalyze)
	caster.Vitals.Stamina = 0

	w.cast(caster, spellParalyze, victim.ID)

	if !victim.paralyzed(w.tick) {
		t.Error("an empty energy bar blocked a cast")
	}
}

// The heavy spells are handed out by what a class can actually pay for: a row
// in the book you can never afford is a dead row.
func TestHeavySpellsGoOnlyToClassesThatCanAffordThem(t *testing.T) {
	costs := map[int]int{spellFireStorm: 250, spellLightning: 460, spellApocalypse: 1000}

	for _, class := range allClasses {
		for _, race := range allRaces {
			mana := manaFor(class, race)
			for _, id := range heavySpellsFor(class) {
				if mana < costs[id] {
					t.Errorf("%v %v has %d mana but was given spell %d costing %d",
						race, class, mana, id, costs[id])
				}
			}
		}
	}
}

func TestOnlyTheMagoGetsApocalipsis(t *testing.T) {
	for _, class := range allClasses {
		has := false
		for _, id := range heavySpellsFor(class) {
			if id == spellApocalypse {
				has = true
			}
		}
		if has != (class == Mago) {
			t.Errorf("%v has Apocalipsis = %v, want %v", class, has, class == Mago)
		}
	}
}

func TestClassesWithNoManaGetNoHeavySpells(t *testing.T) {
	for _, class := range allClasses {
		if manaFor(class, Humano) == 0 && len(heavySpellsFor(class)) != 0 {
			t.Errorf("%v has no mana but was given %v", class, heavySpellsFor(class))
		}
	}
}

func TestTheHeavySpellsLandInTheBook(t *testing.T) {
	book := spellBook(Mago)
	for _, id := range []int{spellFireStorm, spellLightning, spellApocalypse} {
		found := false
		for _, slot := range book {
			if slot == id {
				found = true
			}
		}
		if !found {
			t.Errorf("spell %d is missing from a Mago's book", id)
		}
	}
	if len(book) != SpellSlots {
		t.Errorf("book grew to %d slots, want %d", len(book), SpellSlots)
	}
}
