package world

import "juegito/server/internal/protocol"

// groundStack is one item stack lying on a tile. Argentum's own map format
// carries exactly one object per tile (MapData().ObjInfo — a single
// ObjIndex/Amount pair, not a list), so ground loot follows the same
// constraint here: at most one stack per tile.
type groundStack struct {
	ItemID int
	Amount int
}

// lootEntry is one item's chance of being the thing scattered at a given
// ground spawn — see computeLootTable.
type lootEntry struct {
	Item   Item
	Weight float64
}

// computeLootTable is every item worth finding, weighted so the strong stuff
// is rare. Newbie-tier items are excluded — that is what everyone already
// spawns with (see loadout.go), so finding one on the ground would be a
// letdown, not a reward. The joke black potion is excluded too; nobody should
// have to guess whether a mystery potion is going to kill them outright.
//
// Weight is 1/(1+power), power being MaxHit for weapons and MaxDef for
// anything with armour class — a plain sword (MaxHit 8) and a warhammer
// (MaxHit 40) end up roughly 5x apart in spawn odds. Items with no power stat
// (potions, food, drink, rings) get a flat, comparatively generous weight so
// the map isn't only weapons and armour.
func computeLootTable(items map[int]Item) []lootEntry {
	const flatWeight = 0.5

	var table []lootEntry
	for _, item := range items {
		if item.PotionType == PotionBlack || item.Newbie {
			continue
		}
		var power int
		switch item.Type {
		case ItemWeapon:
			power = item.MaxHit
		case ItemArmor, ItemShield, ItemHelmet:
			power = item.MaxDef
		default:
			table = append(table, lootEntry{Item: item, Weight: flatWeight})
			continue
		}
		table = append(table, lootEntry{Item: item, Weight: 1.0 / float64(1+power)})
	}
	return table
}

// potionLootTable is every potion worth finding on the ground, flat-weighted.
//
// It is a separate table from computeLootTable on purpose: that one weights by
// power so a warhammer stays a lucky find, while potions are the thing you
// should always be able to top up on, so every kind is equally likely.
//
// What both tables share is that the newbie tier does not exist in this game.
// It is not a starting point when everyone spawns at the cap — it is just a
// worse copy of an item already in the world, and the worse copy is invisible:
// two red potions with the same icon and the same name that heal 30 and 10.
// Only the black potion is out beyond that, for the reason computeLootTable
// already gives.
func potionLootTable(items map[int]Item) []lootEntry {
	var table []lootEntry
	for _, item := range items {
		if item.Type != ItemPotion || item.PotionType == PotionBlack || item.Newbie {
			continue
		}
		table = append(table, lootEntry{Item: item, Weight: 1})
	}
	return table
}

// pickWeighted rolls one entry from the table, proportional to Weight.
func (w *World) pickWeighted(table []lootEntry) Item {
	total := 0.0
	for _, e := range table {
		total += e.Weight
	}
	roll := w.rng.Float64() * total
	for _, e := range table {
		roll -= e.Weight
		if roll <= 0 {
			return e.Item
		}
	}
	return table[len(table)-1].Item // float rounding fallback
}

// freeTilePool is every walkable, currently unoccupied tile, shuffled.
//
// Ground spawning draws from this rather than rejection-sampling random
// coordinates, which is what it used to do with a 30-attempt budget per item.
// That budget was fine while loot covered 166 of ~4900 walkable tiles; it is
// not fine now that potions target a quarter of the map, because as occupancy
// climbs a random tile is more often taken than not and a fixed budget starts
// silently placing fewer items than asked without any error to notice. Drawing
// from a shuffled pool places exactly what was asked for, in one pass, for as
// long as tiles remain — and stays deterministic under a fixed seed.
func (w *World) freeTilePool() []tileKey {
	pool := make([]tileKey, 0, w.grid.W*w.grid.H)
	for y := 0; y < w.grid.H; y++ {
		for x := 0; x < w.grid.W; x++ {
			if w.grid.Blocked(x, y) {
				continue
			}
			if _, taken := w.ground[tileKey{x, y}]; taken {
				continue
			}
			pool = append(pool, tileKey{x, y})
		}
	}
	w.rng.Shuffle(len(pool), func(i, j int) { pool[i], pool[j] = pool[j], pool[i] })
	return pool
}

// scatter places up to count stacks from table onto tiles taken off the front
// of pool, and returns whatever is left of the pool for the next caller.
// Called once at load, after the map and items are both in — ground contents
// are part of the match's starting state, not something that regenerates
// mid-match.
func (w *World) scatter(pool []tileKey, table []lootEntry, count int, amount func(Item) int) []tileKey {
	if len(table) == 0 {
		return pool
	}
	if count > len(pool) {
		count = len(pool)
	}
	for i := 0; i < count; i++ {
		item := w.pickWeighted(table)
		w.ground[pool[i]] = groundStack{ItemID: item.ID, Amount: amount(item)}
	}
	return pool[count:]
}

// spawnGroundLoot scatters the gear find — weapons, armour, rings, the
// occasional non-newbie consumable — weighted so the strong stuff is rare.
func (w *World) spawnGroundLoot(pool []tileKey, count int) []tileKey {
	rest := w.scatter(pool, computeLootTable(w.items), count, stackAmountFor)
	w.log.Info("loot esparcido", "pedido", count, "colocado", len(pool)-len(rest))
	return rest
}

// spawnGroundPotions carpets the map with potions, in its own pass and at its
// own density. Running them through the gear scatter would make a potion just
// another rare find competing with swords for the same tile; the point here is
// the opposite, that you are never more than a few tiles from a refill.
func (w *World) spawnGroundPotions(pool []tileKey, count int) []tileKey {
	rest := w.scatter(pool, potionLootTable(w.items), count, func(Item) int { return groundPotionStack })
	w.log.Info("pociones esparcidas", "pedido", count, "colocado", len(pool)-len(rest),
		"unidades", (len(pool)-len(rest))*groundPotionStack)
	return rest
}

// groundArrowStack is how many arrows one find on the ground is worth.
//
// It is a resupply, not a refill: at the melee interval an archer empties this
// in half a minute of sustained shooting, which is more than any single fight
// takes and less than a match. One arrow — what the default below used to hand
// out, back when nothing fired them — would have been an item that exists only
// to be disappointing.
const groundArrowStack = 25

// stackAmountFor is how many units a single gear-scatter spawn carries — a
// found weapon or piece of armour is one, a found consumable a small handful,
// a found quiver enough to fight with. Potions scattered by spawnGroundPotions
// do not come through here; they use groundPotionStack.
func stackAmountFor(item Item) int {
	switch item.Type {
	case ItemPotion, ItemFood, ItemDrink:
		return 3
	case ItemArrow:
		return groundArrowStack
	default:
		return 1
	}
}

// groundItemsInView is Ground for one player's snapshot — same viewport
// window as viewportOf, so loot obeys the same interest-management rule
// entities do.
//
// It walks the viewport and looks each tile up, rather than walking the ground
// map and discarding what falls outside. Both are correct; only one has a cost
// that does not depend on how much loot the world holds. That stopped being an
// academic distinction when maps went from Ullathorpe's 10.000 tiles to a
// composed world's 672.400: the same loot density that put ~1.400 stacks on
// the ground now puts ~90.000, and scanning all of them for every player on
// every tick is 90 million map reads a second with a full match connected.
// The viewport is 221 tiles no matter how big the world gets.
func (w *World) groundItemsInView(p *Player) []protocol.GroundItem {
	const halfW, halfH = ViewportW / 2, ViewportH / 2
	var out []protocol.GroundItem
	for dy := -halfH; dy <= halfH; dy++ {
		for dx := -halfW; dx <= halfW; dx++ {
			key := tileKey{p.X + dx, p.Y + dy}
			stack, ok := w.ground[key]
			if !ok {
				continue
			}
			out = append(out, protocol.GroundItem{X: key.X, Y: key.Y, ItemID: stack.ItemID, Amount: stack.Amount})
		}
	}
	return out
}

// pickup is Agarrar: take whatever sits on the player's own tile. Stacks onto
// an existing unequipped slot of the same item if one exists, otherwise opens
// a fresh slot.
func (w *World) pickup(p *Player) {
	if p.Dead {
		return
	}
	key := tileKey{p.X, p.Y}
	stack, ok := w.ground[key]
	if !ok {
		return
	}

	// Un cofre no se levanta, se abre. Es la misma tecla porque es el mismo
	// gesto —estás parado encima de algo y lo agarrás— y porque una tecla nueva
	// para un solo tipo de objeto es una tecla que hay que enseñar.
	if w.isChest(stack.ItemID) {
		w.openChest(p, key)
		return
	}

	for i := range p.Inventory {
		if !p.Inventory[i].Equipped && p.Inventory[i].ItemID == stack.ItemID {
			p.Inventory[i].Amount += stack.Amount
			delete(w.ground, key)
			w.sendLoadout(p)
			return
		}
	}

	p.Inventory = append(p.Inventory, protocol.InventorySlot{
		Slot: freeSlotNumber(p.Inventory), ItemID: stack.ItemID, Amount: stack.Amount,
	})
	delete(w.ground, key)
	w.sendLoadout(p)
}

// dropItem is Tirar: place one whole inventory slot on the player's own tile,
// only if it's currently empty — Argentum's one-object-per-tile rule applies
// to what you leave behind too.
func (w *World) dropItem(p *Player, slotIndex int) {
	if p.Dead {
		return
	}
	key := tileKey{p.X, p.Y}
	if _, occupied := w.ground[key]; occupied {
		return
	}

	slot, idx := findSlot(p.Inventory, slotIndex)
	if idx < 0 {
		return
	}
	w.ground[key] = groundStack{ItemID: slot.ItemID, Amount: slot.Amount}
	p.Inventory = append(p.Inventory[:idx], p.Inventory[idx+1:]...)
	w.sendLoadout(p)
}

// scatterInventory is what classic AO's hardcore drop-on-death already is —
// dropping everything a player carried at the moment of elimination is a
// battle royale mechanic that needed no invention, just wiring up. Each stack
// looks for its own free tile working outward from the death spot, so five
// items don't all collapse onto the one tile the rule allows.
func (w *World) scatterInventory(p *Player) {
	if len(p.Inventory) == 0 {
		return
	}
	tiles := w.freeTilesNear(p.X, p.Y, len(p.Inventory))
	for i, slot := range p.Inventory {
		if i >= len(tiles) {
			break // ran out of nearby free ground; the rest is lost, not stuck mid-air
		}
		w.ground[tiles[i]] = groundStack{ItemID: slot.ItemID, Amount: slot.Amount}
	}
	p.Inventory = nil
}

// freeTilesNear spiral-searches outward from (x,y) for up to want unblocked,
// currently empty tiles.
func (w *World) freeTilesNear(x, y, want int) []tileKey {
	var out []tileKey
	for radius := 0; radius <= 6 && len(out) < want; radius++ {
		for dx := -radius; dx <= radius; dx++ {
			for dy := -radius; dy <= radius; dy++ {
				if len(out) >= want {
					return out
				}
				tx, ty := x+dx, y+dy
				if w.grid.Blocked(tx, ty) {
					continue
				}
				key := tileKey{tx, ty}
				if _, taken := w.ground[key]; taken {
					continue
				}
				out = append(out, key)
			}
		}
	}
	return out
}
