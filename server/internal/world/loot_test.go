package world

import (
	"math/rand"
	"testing"
)

// lootItems is a table shaped like the real converted obj.dat in the ways the
// loot and kit code branches on: newbie and non-newbie tiers, weapons that
// differ in power, the black potion, and — because obj.dat really does carry
// them — a legacy duplicate of a newbie potion at a low id alongside the live
// one the source hands out.
func lootItems() map[int]Item {
	return map[int]Item{
		1: {ID: 1, Name: "Daga (Newbie)", Type: ItemWeapon, MinHit: 1, MaxHit: 2},
		2: {ID: 2, Name: "Espada Larga", Type: ItemWeapon, MinHit: 4, MaxHit: 8},
		3: {ID: 3, Name: "Martillo de Guerra", Type: ItemWeapon, MinHit: 20, MaxHit: 40},
		4: {ID: 4, Name: "Armadura de Cuero (H/E/EO) (Newbie)", Type: ItemArmor, MaxDef: 1},
		5: {ID: 5, Name: "Armadura de Placas", Type: ItemArmor, MaxDef: 15},
		6: {ID: 6, Name: "Anillo", Type: ItemRing},

		// The legacy/live newbie potion pairs, ids as in the real data.
		461: {ID: 461, Name: "Pocion Roja (Newbie)", Type: ItemPotion, PotionType: PotionHealth,
			MinModificador: 30, MaxModificador: 30},
		462: {ID: 462, Name: "Pocion Verde (Newbie)", Type: ItemPotion, PotionType: PotionStrength},
		855: {ID: 855, Name: "Pocion Amarilla (Newbie)", Type: ItemPotion, PotionType: PotionAgility},
		856: {ID: 856, Name: "Pocion Azul (Newbie)", Type: ItemPotion, PotionType: PotionMana},
		857: {ID: 857, Name: "Pocion Roja (Newbie)", Type: ItemPotion, PotionType: PotionHealth,
			MinModificador: 10, MaxModificador: 10},
		858: {ID: 858, Name: "Pocion Verde (Newbie)", Type: ItemPotion, PotionType: PotionStrength},

		900: {ID: 900, Name: "Pocion Roja", Type: ItemPotion, PotionType: PotionHealth},
		901: {ID: 901, Name: "Pocion Negra", Type: ItemPotion, PotionType: PotionBlack},
		902: {ID: 902, Name: "Manzana Roja (Newbie)", Type: ItemFood, Restores: 5},
		903: {ID: 903, Name: "Botella de Agua (Newbie)", Type: ItemDrink, Restores: 5},
	}
}

// lootWorld pins the rng before SetItems runs. World seeds itself from the
// clock, which is what a match wants — loot should not sit in the same place
// every game — and exactly what a test must not inherit.
func lootWorld(t *testing.T, width, height int, items map[int]Item) *World {
	t.Helper()
	w := newTestWorld(t, width, height)
	w.rng = rand.New(rand.NewSource(1))
	w.SetItems(items)
	return w
}

func TestStartingKitCarriesEveryUsablePotionType(t *testing.T) {
	items := lootItems()
	kit := computeStartingKit(items, Guerrero, Humano)

	byType := make(map[int]int) // potion type -> amount carried
	for _, slot := range kit.slots {
		if item := items[slot.ItemID]; item.Type == ItemPotion {
			byType[item.PotionType] += slot.Amount
		}
	}

	for _, potionType := range kitPotionTypes {
		if byType[potionType] != potionStack {
			t.Errorf("potion type %d: carrying %d, want %d", potionType, byType[potionType], potionStack)
		}
	}
	if len(byType) != len(kitPotionTypes) {
		t.Errorf("kit carries %d potion types, want exactly the %d in kitPotionTypes",
			len(byType), len(kitPotionTypes))
	}
	if byType[PotionBlack] != 0 {
		t.Error("the kit handed out black potions")
	}
}

// The old kit picked its second potion as "the first non-health one map
// iteration reaches", which Go randomises — the bag genuinely differed between
// server starts. Recomputing the same kit must now give the identical item ids.
func TestStartingKitIsDeterministicAcrossRuns(t *testing.T) {
	items := lootItems()
	first := computeStartingKit(items, Mago, Gnomo)

	for run := 0; run < 50; run++ {
		got := computeStartingKit(items, Mago, Gnomo)
		if len(got.slots) != len(first.slots) {
			t.Fatalf("run %d: %d slots, first run had %d", run, len(got.slots), len(first.slots))
		}
		for i := range got.slots {
			if got.slots[i] != first.slots[i] {
				t.Fatalf("run %d slot %d: %+v, first run had %+v", run, i, got.slots[i], first.slots[i])
			}
		}
	}
}

// Of the two Pocion Roja (Newbie) entries in obj.dat, 857 is the one
// AddItemsToNewUser actually hands out; 461 is unreferenced legacy data.
func TestStartingKitPrefersTheLiveNewbiePotionOverTheLegacyDuplicate(t *testing.T) {
	items := lootItems()
	kit := computeStartingKit(items, Guerrero, Humano)

	for _, slot := range kit.slots {
		if slot.ItemID == 461 || slot.ItemID == 462 {
			t.Errorf("kit picked legacy potion %d over its live counterpart", slot.ItemID)
		}
	}
}

func TestFreeTilePoolSkipsBlockedAndOccupiedTiles(t *testing.T) {
	w := newTestWorld(t, 10, 10)
	w.rng = rand.New(rand.NewSource(1))
	w.grid.SetBlocked(3, 3, true)
	w.grid.SetBlocked(4, 3, true)
	w.ground[tileKey{7, 7}] = groundStack{ItemID: 1, Amount: 1}

	pool := w.freeTilePool()
	if len(pool) != 100-2-1 {
		t.Errorf("pool has %d tiles, want %d", len(pool), 97)
	}

	seen := make(map[tileKey]bool, len(pool))
	for _, key := range pool {
		if w.grid.Blocked(key.X, key.Y) {
			t.Fatalf("pool contains blocked tile %v", key)
		}
		if _, taken := w.ground[key]; taken {
			t.Fatalf("pool contains occupied tile %v", key)
		}
		if seen[key] {
			t.Fatalf("pool contains %v twice", key)
		}
		seen[key] = true
	}
}

// Argentum's map format carries one object per tile, and both scatter passes
// draw from the same pool so that stays true no matter how dense potions get.
func TestGearAndPotionsNeverShareATile(t *testing.T) {
	w := lootWorld(t, 40, 40, lootItems())

	free := 0
	for y := 0; y < w.grid.H; y++ {
		for x := 0; x < w.grid.W; x++ {
			if !w.grid.Blocked(x, y) {
				free++
			}
		}
	}

	gear := free / groundLootTiles
	if gear < minGroundLoot {
		gear = minGroundLoot
	}
	potions := (free - gear) / groundPotionTiles
	if len(w.ground) != gear+potions {
		t.Errorf("ground holds %d stacks, want %d gear + %d potions = %d",
			len(w.ground), gear, potions, gear+potions)
	}
	if len(w.ground) > free {
		t.Errorf("placed %d stacks on %d walkable tiles", len(w.ground), free)
	}
}

func TestGroundPotionsAreDenseAndNeverBlack(t *testing.T) {
	w := lootWorld(t, 60, 60, lootItems())

	potionStacks, units := 0, 0
	for _, stack := range w.ground {
		item := w.items[stack.ItemID]
		if item.Type != ItemPotion {
			continue
		}
		if item.PotionType == PotionBlack {
			t.Fatalf("black potion %d was scattered on the ground", item.ID)
		}
		potionStacks++
		units += stack.Amount
	}

	// The gear pass can drop a non-newbie potion too, so this is a floor on
	// what the dedicated potion pass placed, not an exact count.
	wantAtLeast := (60*60 - 60*60/groundLootTiles) / groundPotionTiles
	if potionStacks < wantAtLeast {
		t.Errorf("%d potion stacks on a 60x60 map, want at least %d", potionStacks, wantAtLeast)
	}
	// Same floor logic for units: only the dedicated pass uses
	// groundPotionStack, while a potion the gear pass happened to drop carries
	// stackAmountFor's handful instead.
	if want := wantAtLeast * groundPotionStack; units < want {
		t.Errorf("%d potion units across %d stacks, want at least %d", units, potionStacks, want)
	}
}

// scatter is asked for a count derived from the pool, but it must survive
// being asked for more tiles than exist rather than placing off-map.
func TestScatterStopsWhenTilesRunOut(t *testing.T) {
	w := newTestWorld(t, 5, 5)
	w.rng = rand.New(rand.NewSource(1))
	w.items = lootItems()

	pool := w.freeTilePool()
	rest := w.scatter(pool, potionLootTable(w.items), 1000, func(Item) int { return 1 })

	if len(w.ground) != 25 {
		t.Errorf("placed %d stacks on a 25-tile map, want 25", len(w.ground))
	}
	if len(rest) != 0 {
		t.Errorf("pool has %d tiles left after being drained, want 0", len(rest))
	}
}

func TestPickupTakesTheStackOnYourOwnTile(t *testing.T) {
	w := lootWorld(t, 20, 20, lootItems())
	p, _ := place(t, w, "wachin", 5, 5)
	p.Inventory = nil

	// place() drops the player wherever it likes, which may already hold loot;
	// this test is about the pickup rule, so set the tile up explicitly.
	w.ground[tileKey{5, 5}] = groundStack{ItemID: 857, Amount: groundPotionStack}
	w.pickup(p)

	if _, still := w.ground[tileKey{5, 5}]; still {
		t.Error("the stack stayed on the ground after being picked up")
	}
	if len(p.Inventory) != 1 || p.Inventory[0].ItemID != 857 || p.Inventory[0].Amount != groundPotionStack {
		t.Errorf("inventory = %+v, want one stack of %d x857", p.Inventory, groundPotionStack)
	}

	// A second identical stack must merge rather than open a new slot — the
	// path that matters most now that potions come in by the thousand.
	w.ground[tileKey{5, 5}] = groundStack{ItemID: 857, Amount: groundPotionStack}
	w.pickup(p)
	if len(p.Inventory) != 1 || p.Inventory[0].Amount != 2*groundPotionStack {
		t.Errorf("inventory = %+v, want one merged stack of %d", p.Inventory, 2*groundPotionStack)
	}
}
