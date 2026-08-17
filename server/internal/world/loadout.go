package world

import (
	"strings"

	"juegito/server/internal/protocol"
)

// startingKit is what every player spawns with. Gear stays deliberately poor —
// the original's own AddItemsToNewUser (TCP.bas) hands a brand new character a
// Daga and race-appropriate newbie clothes, and the interesting equipment is
// what you find on the ground (see loot.go), not what you already have.
//
// Consumables are the deliberate exception, and they are not the original's
// numbers. AddItemsToNewUser gives 200 red potions and 200 blue; this kit gives
// potionStack of every usable kind. Argentum meters potions because they are a
// gold sink in an economy that outlives any one fight — a battle royale has no
// economy and no next session, so rationing them only means a match is decided
// by who ran dry rather than who fought better. Potions are treated as ammo
// here: effectively unlimited on spawn, and lying everywhere (spawnGroundPotions).
type startingKit struct {
	slots []protocol.InventorySlot
}

// potionStack is how many of each potion type a player spawns holding, and
// consumableStack the same for food and water. See the note on startingKit for
// why these are not AddItemsToNewUser's 200/100/50.
const (
	potionStack     = 3000
	consumableStack = 10
)

// kitPotionTypes is every ePocionType the kit hands out, in bag order — health
// and mana first because those are the two the HUD's quick-slot counters read.
// PotionBlack is not here for the same reason computeLootTable drops it: it
// kills whoever drinks it, and spawning with 3000 of them is a joke that ends
// the match rather than one that lands. PotionCurePoison is absent because no
// newbie-tier cure-poison potion exists in obj.dat, not by choice — if one
// appears in a data update, adding it here is all it takes.
var kitPotionTypes = []int{PotionHealth, PotionMana, PotionAgility, PotionStrength}

// newbieTag is how the converted data marks starter-tier items — obj.dat's own
// naming convention ("Daga (Newbie)", "Vestimentas Comunes (Newbie)"), not
// something this project invented. Matching on it is more robust than hoping
// specific item ids line up with a particular fork's numbering, which the
// upstream data does not guarantee.
const newbieTag = "(Newbie)"

func computeStartingKit(items map[int]Item, class Class, race Race) startingKit {
	var weapon, armor Item
	haveWeapon, haveArmor := false, false

	// One newbie potion per ePocionType, keyed by that type. Picking them by
	// type rather than "the first potion, then the next different one" matters
	// twice over: it is deterministic (Go randomises map iteration order, so
	// the old second-potion pick genuinely varied between server starts), and
	// it is what lets the kit hand out every kind instead of two.
	potions := make(map[int]Item)
	var food, drink Item
	haveFood, haveDrink := false, false

	// Race-appropriate newbie armor comes in two cosmetic variants in the
	// source, split the same way DarCuerpoDesnudo splits naked bodies:
	// Humano/Elfo/Drow share one look, Enano/Gnomo another.
	armorHint := "(H/E/EO)"
	if race == Enano || race == Gnomo {
		armorHint = "(E/G)"
	}

	for _, item := range items {
		if !strings.Contains(item.Name, newbieTag) || classForbidsUse(item, class) {
			continue
		}
		switch item.Type {
		case ItemWeapon:
			// Among the newbie weapons a class may use, prefer the most
			// class-restrictive one: Daga (Newbie) carries no CP list at
			// all — everyone may use it, which is exactly why it is the
			// generic fallback — while class-flavored weapons each name a
			// long list of classes forbidden from them (Baston de Mago
			// (Newbie) bars 11 of 12). The longer that list, the more this
			// weapon was built for whichever few classes it still allows,
			// so a Mago naturally ends up with the staff and a Guerrero —
			// barred from it by that same CP list — falls back to the
			// dagger, with no separate per-class table to maintain.
			//
			// (obj.dat does carry a real StaffPower field that the source's
			// NeedStaff spell gate reads, but the newbie staff itself is
			// StaffPower 0 like everything else newbie-tier, so it cannot
			// be what tells this pick apart — verified against the actual
			// obj.dat entry, not assumed.)
			if !haveWeapon || len(item.ForbiddenClasses) > len(weapon.ForbiddenClasses) {
				weapon, haveWeapon = item, true
			}
		case ItemArmor:
			// Same "which of several allowed newbie items is actually the
			// better pick" problem as the weapon above: a race group has
			// both a real armor (Armadura de Cuero, MaxDef 1) and a plain
			// robe (Tunica, MaxDef 0, unrestricted). Picking whichever one
			// map iteration order happens to hit first — Go map order is
			// randomized — would spawn a Guerrero in a robe roughly half
			// the time. Preferring higher MaxDef always keeps the real
			// armor for anyone allowed to wear it, and correctly leaves the
			// robe to Mago/Bardo/Druida, who are the ones CP-barred from
			// Armadura de Cuero.
			if (armorHint == "" || strings.Contains(item.Name, armorHint)) &&
				(!haveArmor || item.MaxDef > armor.MaxDef) {
				armor, haveArmor = item, true
			}
		case ItemPotion:
			// obj.dat carries two entries for some newbie potions — a legacy
			// low-id pair (461 Roja, 462 Verde, both priced at 0 and reachable
			// from nothing in the source) alongside the live 855-858 block that
			// AddItemsToNewUser actually hands out. Highest id per type picks
			// the live one in every case, which is why the tiebreak is not
			// arbitrary despite looking like one.
			if item.PotionType == PotionBlack {
				break
			}
			if cur, seen := potions[item.PotionType]; !seen || item.ID > cur.ID {
				potions[item.PotionType] = item
			}
		case ItemFood:
			if !haveFood {
				food, haveFood = item, true
			}
		case ItemDrink:
			if !haveDrink {
				drink, haveDrink = item, true
			}
		}
	}
	// The armor-hint filter can legitimately come up empty (e.g. a class
	// barred from every race-matching newbie robe); take whatever newbie
	// armor is allowed rather than spawn with none.
	if !haveArmor {
		for _, item := range items {
			if item.Type == ItemArmor && strings.Contains(item.Name, newbieTag) && !classForbidsUse(item, class) {
				armor, haveArmor = item, true
				break
			}
		}
	}

	slot := 0
	next := func() int { s := slot; slot++; return s }
	out := startingKit{}
	add := func(item Item, amount int, equipped bool) {
		if item.ID == 0 {
			return
		}
		out.slots = append(out.slots, protocol.InventorySlot{
			Slot: next(), ItemID: item.ID, Amount: amount, Equipped: equipped,
		})
	}

	add(weapon, 1, true)
	add(armor, 1, true)
	for _, potionType := range kitPotionTypes {
		add(potions[potionType], potionStack, false)
	}
	add(food, consumableStack, false)
	add(drink, consumableStack, false)

	return out
}

func (k startingKit) inventory() []protocol.InventorySlot {
	return append([]protocol.InventorySlot(nil), k.slots...)
}
