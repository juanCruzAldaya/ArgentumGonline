package world

import (
	"testing"

	"juegito/server/internal/protocol"
)

// itemWorld returns a world stocked with one item of every kind useItem needs
// to branch on, IDs chosen to read easily in test failures.
func itemWorld(t *testing.T) *World {
	t.Helper()
	w := newTestWorld(t, 20, 20)
	w.tick = 1000
	w.SetItems(map[int]Item{
		1: {ID: 1, Name: "Espada Vieja", Type: ItemWeapon, MinHit: 1, MaxHit: 3},
		2: {ID: 2, Name: "Espada Nueva", Type: ItemWeapon, MinHit: 5, MaxHit: 10},
		3: {ID: 3, Name: "Escudo", Type: ItemShield, MinDef: 1, MaxDef: 2},
		10: {ID: 10, Name: "Poción Roja", Type: ItemPotion, PotionType: PotionHealth,
			MinModificador: 30, MaxModificador: 30},
		11: {ID: 11, Name: "Poción Azul", Type: ItemPotion, PotionType: PotionMana},
		12: {ID: 12, Name: "Poción Amarilla", Type: ItemPotion, PotionType: PotionAgility,
			MinModificador: 5, MaxModificador: 5},
		13: {ID: 13, Name: "Poción Verde", Type: ItemPotion, PotionType: PotionStrength,
			MinModificador: 5, MaxModificador: 5},
		14: {ID: 14, Name: "Poción Violeta", Type: ItemPotion, PotionType: PotionCurePoison},
		15: {ID: 15, Name: "Poción Negra", Type: ItemPotion, PotionType: PotionBlack},
		20: {ID: 20, Name: "Manzana", Type: ItemFood, Restores: 15},
		21: {ID: 21, Name: "Botella de Agua", Type: ItemDrink, Restores: 20},
	})
	return w
}

func TestEquipTogglesOnAndOff(t *testing.T) {
	w := itemWorld(t)
	p, conn := place(t, w, "wachin", 5, 5)
	p.Inventory = []protocol.InventorySlot{{Slot: 0, ItemID: 1, Amount: 1}}

	w.useItem(p, 0)
	if !p.Inventory[0].Equipped {
		t.Fatal("weapon did not equip")
	}
	var result protocol.UseResult
	if err := w.codec.DecodePayload(conn.lastOfType(t, protocol.TypeUseResult), &result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !result.Equipped {
		t.Error("UseResult did not report Equipped")
	}

	w.useItem(p, 0)
	if p.Inventory[0].Equipped {
		t.Error("clicking an equipped weapon should have taken it off")
	}
}

func TestEquippingReplacesTheOldOneInThatSlot(t *testing.T) {
	w := itemWorld(t)
	p, _ := place(t, w, "wachin", 5, 5)
	p.Inventory = []protocol.InventorySlot{
		{Slot: 0, ItemID: 1, Amount: 1, Equipped: true}, // Espada Vieja, already worn
		{Slot: 1, ItemID: 2, Amount: 1},                 // Espada Nueva, in the bag
	}

	w.useItem(p, 1)

	if p.Inventory[0].Equipped {
		t.Error("the old weapon is still marked equipped")
	}
	if !p.Inventory[1].Equipped {
		t.Error("the new weapon never got equipped")
	}
	weapon, ok := w.equippedWeapon(p)
	if !ok || weapon.ID != 2 {
		t.Errorf("equippedWeapon = %+v, ok=%v, want item 2", weapon, ok)
	}
}

func TestHealthPotionHealsAndCapsAtMax(t *testing.T) {
	w := itemWorld(t)
	p, conn := place(t, w, "wachin", 5, 5)
	arm(w, p, Guerrero, 0, 0, 0)
	p.Vitals.MaxHP = 100
	p.Vitals.HP = 90
	p.Inventory = []protocol.InventorySlot{{Slot: 0, ItemID: 10, Amount: 2}}

	w.useItem(p, 0)

	if p.Vitals.HP != 100 {
		t.Errorf("hp = %d, want 100 (30 rolled, capped at max)", p.Vitals.HP)
	}
	var result protocol.UseResult
	if err := w.codec.DecodePayload(conn.lastOfType(t, protocol.TypeUseResult), &result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result.HealedHP != 10 {
		t.Errorf("HealedHP = %d, want 10 (only what actually fit)", result.HealedHP)
	}
	if p.Inventory[0].Amount != 1 {
		t.Errorf("stack = %d, want 1 after drinking one", p.Inventory[0].Amount)
	}
}

// The source ignores Pocion Azul's own MinModificador/MaxModificador entirely
// and computes the restore from a formula instead. This is the one case where
// "read the item's own fields" would have been the wrong port.
func TestManaPotionUsesTheFormulaNotItemFields(t *testing.T) {
	w := itemWorld(t)
	p, _ := place(t, w, "wachin", 5, 5)
	arm(w, p, Mago, 0, 0, 0)
	p.Vitals.Level = maxLevel
	p.Vitals.MaxMana = 200
	p.Vitals.Mana = 0
	p.Inventory = []protocol.InventorySlot{{Slot: 0, ItemID: 11, Amount: 1}}

	w.useItem(p, 0)

	want := 200*4/100 + maxLevel/2 + 40/maxLevel
	if p.Vitals.Mana != want {
		t.Errorf("mana = %d, want %d from Porcentaje(max,4)+lvl/2+40/lvl", p.Vitals.Mana, want)
	}
}

func TestAgilityPotionAppliesATemporaryBuff(t *testing.T) {
	w := itemWorld(t)
	p, _ := place(t, w, "wachin", 5, 5)
	arm(w, p, Guerrero, 0, 0, 0)
	p.Attributes.Agilidad = 20
	p.Inventory = []protocol.InventorySlot{{Slot: 0, ItemID: 12, Amount: 1}}

	w.useItem(p, 0)

	if got := w.effectiveAttributes(p).Agilidad; got != 25 {
		t.Errorf("effective agility = %d, want 25 (20 base + 5 from the potion)", got)
	}
}

func TestFoodRestoresHungerAndDrinkRestoresThirst(t *testing.T) {
	w := itemWorld(t)
	p, _ := place(t, w, "wachin", 5, 5)
	p.Vitals.MaxHunger, p.Vitals.Hunger = 100, 50
	p.Vitals.MaxThirst, p.Vitals.Thirst = 100, 50
	p.Inventory = []protocol.InventorySlot{
		{Slot: 0, ItemID: 20, Amount: 1},
		{Slot: 1, ItemID: 21, Amount: 1},
	}

	w.useItem(p, 0)
	w.tick += useCooldownTicks // otherwise the second use is dropped by the cooldown
	w.useItem(p, 1)

	if p.Vitals.Hunger != 65 {
		t.Errorf("hunger = %d, want 65 (50 + 15 from the apple)", p.Vitals.Hunger)
	}
	if p.Vitals.Thirst != 70 {
		t.Errorf("thirst = %d, want 70 (50 + 20 from the water) — this is the MinAgu key bug's regression test", p.Vitals.Thirst)
	}
}

func TestConsumingRemovesTheSlotAtZero(t *testing.T) {
	w := itemWorld(t)
	p, _ := place(t, w, "wachin", 5, 5)
	p.Inventory = []protocol.InventorySlot{{Slot: 0, ItemID: 20, Amount: 1}}

	w.useItem(p, 0)

	if len(p.Inventory) != 0 {
		t.Errorf("inventory = %+v, want empty after the last unit was consumed", p.Inventory)
	}
}

func TestUseCooldownBlocksChuggingPotions(t *testing.T) {
	w := itemWorld(t)
	p, _ := place(t, w, "wachin", 5, 5)
	p.Vitals.MaxHP, p.Vitals.HP = 100, 10
	p.Inventory = []protocol.InventorySlot{{Slot: 0, ItemID: 10, Amount: 3}}

	w.useItem(p, 0)
	afterFirst := p.Vitals.HP
	w.useItem(p, 0) // immediately again, same tick

	if p.Vitals.HP != afterFirst {
		t.Errorf("hp changed on the second drink, want the cooldown to have blocked it")
	}
	if p.Inventory[0].Amount != 2 {
		t.Errorf("stack = %d, want 2: only the first drink should have consumed one", p.Inventory[0].Amount)
	}
}

func TestBlackPotionKillsTheDrinker(t *testing.T) {
	w := itemWorld(t)
	p, conn := place(t, w, "wachin", 5, 5)
	p.Inventory = []protocol.InventorySlot{{Slot: 0, ItemID: 15, Amount: 1}}

	w.useItem(p, 0)

	if !p.Dead {
		t.Error("the black potion did not kill the drinker")
	}
	var result protocol.UseResult
	if err := w.codec.DecodePayload(conn.lastOfType(t, protocol.TypeUseResult), &result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !result.Died {
		t.Error("UseResult did not report Died")
	}
}

func TestDeadPlayersCannotUseItems(t *testing.T) {
	w := itemWorld(t)
	p, _ := place(t, w, "wachin", 5, 5)
	p.Vitals.HP = 50
	p.Dead = true
	p.Inventory = []protocol.InventorySlot{{Slot: 0, ItemID: 10, Amount: 1}}

	w.useItem(p, 0)

	if p.Vitals.HP != 50 {
		t.Error("a dead player drank a healing potion")
	}
}

func TestBestLoadoutPicksTheStrongestOfEachSlotAndOneOfEachPotion(t *testing.T) {
	loadout := computeBestLoadout(map[int]Item{
		1:  {ID: 1, Type: ItemWeapon, MaxHit: 3},
		2:  {ID: 2, Type: ItemWeapon, MaxHit: 10}, // strongest weapon
		3:  {ID: 3, Type: ItemShield, MaxDef: 2},
		10: {ID: 10, Type: ItemPotion, PotionType: PotionHealth},
		11: {ID: 11, Type: ItemPotion, PotionType: PotionMana},
		15: {ID: 15, Type: ItemPotion, PotionType: PotionBlack}, // must be excluded
		20: {ID: 20, Type: ItemFood, Restores: 15},
	})

	byItem := map[int]bool{}
	for _, slot := range loadout.slots {
		byItem[slot.ItemID] = true
		if slot.ItemID == 15 {
			t.Error("the black potion ended up in the spawn loadout")
		}
	}
	if !byItem[2] {
		t.Error("did not pick the strongest weapon (id 2, 10 max hit)")
	}
	if byItem[1] {
		t.Error("picked the weaker weapon alongside the stronger one")
	}
	if !byItem[3] || !byItem[10] || !byItem[11] || !byItem[20] {
		t.Errorf("missing an expected slot: %+v", loadout.slots)
	}
}
