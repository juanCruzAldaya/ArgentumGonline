package world

import "juegito/server/internal/protocol"

// bestLoadout is what every player spawns with: computed from whatever obj.dat
// actually contains rather than a hardcoded item list, so it stays correct if
// the converted item set ever changes and never silently hands out a
// second-best weapon because some id drifted.
type bestLoadout struct {
	slots []protocol.InventorySlot
}

// computeBestLoadout scans the item table once per class, at load time, for
// the strongest item that class is actually allowed to wear in each
// equipment slot, plus one representative of each potion type and food and
// drink — those never carry a class restriction. "Best" and "battle royale"
// have a natural alliance here: every player armed with the best of
// everything THEIR CLASS CAN USE is what "empezá con el mejor equipo, según
// tu clase" concretely means. A Mago handed the same greatsword a Guerrero
// gets would spawn holding an item its own class is barred from equipping —
// classForbidsUse is the same check EquiparInvItem itself runs.
func computeBestLoadout(items map[int]Item, class Class) bestLoadout {
	var best struct {
		weapon, shield, armor, helmet, ring Item
	}
	haveWeapon, haveShield, haveArmor, haveHelmet, haveRing := false, false, false, false, false

	var potions [7]Item // indexed by PotionType; zero value means "none found"
	var food, drink Item
	haveFood, haveDrink := false, false

	for _, item := range items {
		if classForbidsUse(item, class) {
			continue
		}
		switch item.Type {
		case ItemWeapon:
			if !haveWeapon || item.MaxHit > best.weapon.MaxHit {
				best.weapon, haveWeapon = item, true
			}
		case ItemShield:
			if !haveShield || item.MaxDef > best.shield.MaxDef {
				best.shield, haveShield = item, true
			}
		case ItemArmor:
			if !haveArmor || item.MaxDef > best.armor.MaxDef {
				best.armor, haveArmor = item, true
			}
		case ItemHelmet:
			if !haveHelmet || item.MaxDef > best.helmet.MaxDef {
				best.helmet, haveHelmet = item, true
			}
		case ItemRing:
			// Rings carry no stat this converter parses, so Value is the only
			// signal available for "which one is better."
			if !haveRing || item.Value > best.ring.Value {
				best.ring, haveRing = item, true
			}
		case ItemFood:
			if !haveFood || item.Restores > food.Restores {
				food, haveFood = item, true
			}
		case ItemDrink:
			if !haveDrink || item.Restores > drink.Restores {
				drink, haveDrink = item, true
			}
		case ItemPotion:
			t := item.PotionType
			if t <= 0 || t >= len(potions) || t == PotionBlack {
				continue // the joke potion is not something you spawn with
			}
			if potions[t].ID == 0 || item.Value > potions[t].Value {
				potions[t] = item
			}
		}
	}

	const potionStack, consumableStack = 20, 20
	slot := 0
	next := func() int { s := slot; slot++; return s }
	out := bestLoadout{}
	add := func(item Item, amount int, equipped bool) {
		if item.ID == 0 {
			return
		}
		out.slots = append(out.slots, protocol.InventorySlot{
			Slot: next(), ItemID: item.ID, Amount: amount, Equipped: equipped,
		})
	}

	add(best.weapon, 1, true)
	add(best.shield, 1, true)
	add(best.armor, 1, true)
	add(best.helmet, 1, true)
	add(best.ring, 1, true)
	for t, item := range potions {
		if t != 0 {
			add(item, potionStack, false)
		}
	}
	add(food, consumableStack, false)
	add(drink, consumableStack, false)

	return out
}

func (b bestLoadout) inventory() []protocol.InventorySlot {
	return append([]protocol.InventorySlot(nil), b.slots...)
}
