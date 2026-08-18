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

	w.useItem(p, 0, protocol.UseAuto)
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

	w.useItem(p, 0, protocol.UseAuto)
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

	w.useItem(p, 1, protocol.UseAuto)

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

func TestSwapSlotsExchangesTwoOccupiedSlots(t *testing.T) {
	w := itemWorld(t)
	p, _ := place(t, w, "wachin", 5, 5)
	p.Inventory = []protocol.InventorySlot{
		{Slot: 0, ItemID: 1, Amount: 1},
		{Slot: 1, ItemID: 3, Amount: 1},
	}

	w.swapSlots(p, 0, 1)

	if p.Inventory[0].Slot != 1 || p.Inventory[0].ItemID != 1 {
		t.Errorf("first slot = %+v, want the sword moved to slot 1", p.Inventory[0])
	}
	if p.Inventory[1].Slot != 0 || p.Inventory[1].ItemID != 3 {
		t.Errorf("second slot = %+v, want the shield moved to slot 0", p.Inventory[1])
	}
}

func TestSwapSlotsMovesIntoAnEmptySlot(t *testing.T) {
	w := itemWorld(t)
	p, _ := place(t, w, "wachin", 5, 5)
	p.Inventory = []protocol.InventorySlot{{Slot: 0, ItemID: 1, Amount: 1}}

	w.swapSlots(p, 0, 5)

	if len(p.Inventory) != 1 || p.Inventory[0].Slot != 5 {
		t.Errorf("inventory = %+v, want the only item relabeled to slot 5", p.Inventory)
	}
}

func TestSwapSlotsIgnoresAStaleSourceSlot(t *testing.T) {
	w := itemWorld(t)
	p, _ := place(t, w, "wachin", 5, 5)
	p.Inventory = []protocol.InventorySlot{{Slot: 0, ItemID: 1, Amount: 1}}

	w.swapSlots(p, 9, 0) // nothing at 9

	if p.Inventory[0].Slot != 0 {
		t.Error("swapping from an empty source should not have touched the real item")
	}
}

func TestHealthPotionHealsAndCapsAtMax(t *testing.T) {
	w := itemWorld(t)
	p, conn := place(t, w, "wachin", 5, 5)
	arm(w, p, Guerrero, 0, 0, 0)
	p.Vitals.MaxHP = 100
	p.Vitals.HP = 90
	p.Inventory = []protocol.InventorySlot{{Slot: 0, ItemID: 10, Amount: 2}}

	w.useItem(p, 0, protocol.UseAuto)

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

	w.useItem(p, 0, protocol.UseAuto)

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

	w.useItem(p, 0, protocol.UseAuto)

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

	w.useItem(p, 0, protocol.UseAuto)
	w.tick += useCooldownTicks // otherwise the second use is dropped by the cooldown
	w.useItem(p, 1, protocol.UseAuto)

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

	w.useItem(p, 0, protocol.UseAuto)

	if len(p.Inventory) != 0 {
		t.Errorf("inventory = %+v, want empty after the last unit was consumed", p.Inventory)
	}
}

func TestUseCooldownBlocksChuggingPotions(t *testing.T) {
	w := itemWorld(t)
	p, _ := place(t, w, "wachin", 5, 5)
	p.Vitals.MaxHP, p.Vitals.HP = 100, 10
	p.Inventory = []protocol.InventorySlot{{Slot: 0, ItemID: 10, Amount: 3}}

	w.useItem(p, 0, protocol.UseAuto)
	afterFirst := p.Vitals.HP
	w.useItem(p, 0, protocol.UseAuto) // immediately again, same tick

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

	w.useItem(p, 0, protocol.UseAuto)

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

	w.useItem(p, 0, protocol.UseAuto)

	if p.Vitals.HP != 50 {
		t.Error("a dead player drank a healing potion")
	}
}

// The kit is that class's basic gear, and "basic" has a definition in the
// data: a shop sells it. The Espada Legendaria below is the tier you cross the
// map for, so it must not be what you wake up holding — that is what keeps
// finding one on the ground worth something.
func TestStartingKitTakesShopGearNotTrophies(t *testing.T) {
	kit := computeStartingKit(map[int]Item{
		1: {ID: 1, Name: "Espada Legendaria", Type: ItemWeapon, MaxHit: 50},
		2: {ID: 2, Name: "Espada Corta", Type: ItemWeapon, MaxHit: 3, Sold: true},
		3: {ID: 3, Name: "Tunica", Type: ItemArmor, MaxDef: 2, Sold: true},
		4: {ID: 4, Name: "Armadura de Dragon", Type: ItemArmor, MaxDef: 65},
		10: {ID: 10, Name: "Pocion Roja", Type: ItemPotion, PotionType: PotionHealth,
			MinModificador: 30, MaxModificador: 30, Sold: true},
		15: {ID: 15, Name: "Pocion Negra", Type: ItemPotion, PotionType: PotionBlack, Sold: true},
		20: {ID: 20, Name: "Manzana", Type: ItemFood, Restores: 15, Sold: true},
		21: {ID: 21, Name: "Botella", Type: ItemDrink, Restores: 20, Sold: true},
	}, Guerrero, Humano)

	byItem := map[int]bool{}
	for _, slot := range kit.slots {
		byItem[slot.ItemID] = true
	}
	if byItem[15] {
		t.Error("the black potion ended up in the spawn loadout")
	}
	if byItem[1] || byItem[4] {
		t.Error("spawned wearing gear no shop sells; that tier belongs on the floor")
	}
	if !byItem[2] || !byItem[3] || !byItem[10] || !byItem[20] || !byItem[21] {
		t.Errorf("missing an expected kit slot: %+v", kit.slots)
	}
}

// The point of computing a kit per class: a Mago never spawns holding a weapon
// Mago is barred from, and among what is left it takes the one built for the
// fewest classes rather than the one with the biggest number. That ordering is
// the whole reason a Cazador ends up with a bow instead of an axe — see
// TestHunterGetsBowAndArrows, which pins the same rule against the real data.
func TestStartingKitPrefersTheClassSpecificWeapon(t *testing.T) {
	kit := computeStartingKit(map[int]Item{
		1: {ID: 1, Name: "Espada", Type: ItemWeapon, MaxHit: 10, Sold: true,
			ForbiddenClasses: []string{"MAGO"}},
		2: {ID: 2, Name: "Daga", Type: ItemWeapon, MaxHit: 3, Sold: true},
		3: {ID: 3, Name: "Baston de Mago", Type: ItemWeapon, MaxHit: 1, Sold: true,
			ForbiddenClasses: []string{"GUERRERO", "CAZADOR", "PALADIN"}},
	}, Mago, Humano)

	byItem := map[int]bool{}
	for _, slot := range kit.slots {
		byItem[slot.ItemID] = true
	}
	if byItem[1] {
		t.Error("gave a Mago a weapon explicitly forbidden to Mago")
	}
	if byItem[2] {
		t.Error("fell back to the universal dagger despite a class-specific staff being allowed")
	}
	if !byItem[3] {
		t.Error("did not pick the staff, the weapon built for the fewest classes Mago is in")
	}
}
