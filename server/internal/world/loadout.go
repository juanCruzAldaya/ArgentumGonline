package world

import (
	"juegito/server/internal/protocol"
)

// startingKit is what every player spawns with: the basic gear of their class,
// not the best gear in the game.
//
// Argentum has no such thing to port. AddItemsToNewUser (TCP.bas) hands a
// level-1 character a Daga and newbie rags because everything after that is
// earned over weeks, and this game deleted that whole axis — everyone spawns at
// the cap, so newbie rags are not a starting point any more, they are just
// being underdressed for the rest of the match. What replaces it has to come
// out of the data, and the data does answer it:
//
//   - **A shop sells it.** obj.dat is a catalogue of every object the engine
//     knows, GM tools and donor trophies included — the Espada Mata Dragones
//     NatOs hits for a flat 1000 and forbids no class from carrying it, so
//     anything that ranks equipment by its numbers alone hands that one sword
//     to all twelve classes. Item.Sold (the converter's loadSold) says a
//     merchant NPC stocks it, which is exactly the ordinary gear of the world:
//     what a smith forges, what a dragon drops and what a GM spawns all stay
//     out — and stay out on the floor, where finding one is worth the walk.
//   - **The class is not barred from it**, which is ClasePuedeUsarItem's own
//     CP list, already ported as classForbidsUse.
//   - **Of what is left, the most class-specific piece.** The longer an item's
//     CP list, the fewer classes it was built for: Arco de Cazador bars ten of
//     twelve, Hacha Larga de Guerra bars eight, and a plain Espada Corta bars
//     nobody. Ranking by that first and by the numbers only after is what puts
//     a bow in a Cazador's hands instead of the axe that would out-damage it,
//     which is the whole point of a kit that says what you are.
//
// Nothing here is a per-class table. Twelve classes by five races is sixty
// kits, and every one of them falls out of obj.dat's own class lists, so a data
// update carries the kit with it instead of leaving a hardcoded id pointing at
// something that moved.
type startingKit struct {
	slots []protocol.InventorySlot
}

// potionStack is how many of each potion type a player spawns holding,
// consumableStack the same for food and water, and arrowStack is a quiver.
const (
	potionStack     = 3000
	consumableStack = 10
	arrowStack      = 500
)

// Consumables are the deliberate exception to "basic gear", and they are not
// the original's numbers either. AddItemsToNewUser gives 200 red potions and
// 200 blue; this kit gives potionStack of every usable kind. Argentum meters
// potions because they are a gold sink in an economy that outlives any one
// fight — a battle royale has no economy and no next session, so rationing them
// only means a match is decided by who ran dry rather than who fought better.
// Potions are ammunition here: effectively unlimited on spawn, and lying
// everywhere (spawnGroundPotions).
//
// kitPotionTypes is every ePocionType handed out, in bag order — health and
// mana first, because those are the two anyone reaches for. PotionBlack is
// absent for the same reason computeLootTable drops it: it kills whoever drinks
// it, and 3000 of them is a joke that ends the match rather than one that
// lands. PotionCurePoison is absent because nothing inflicts poison yet, so it
// would be 3000 bottles of nothing.
//
// These are the ordinary potions of the world — the same 36-39 block a shop
// sells — and not the newbie duplicates, which is a difference you can feel:
// the newbie red heals 10 and the real one heals 30. The newbie tier is gone
// from this game entirely, kit and floor alike; see pickConsumable.
var kitPotionTypes = []int{PotionHealth, PotionMana, PotionAgility, PotionStrength}

// gearSlots are the four worn slots a kit fills, and the pair of obj.dat fields
// each one is judged by. Rings are not here: obj.dat gives them DefensaMagica
// and the Impide* immunities, none of which this server models, so a ring would
// be a slot that does nothing.
var gearSlots = []struct {
	itemType int
	stat     statPair
}{
	{ItemWeapon, weaponStat},
	{ItemArmor, defenceStat},
	{ItemShield, defenceStat},
	{ItemHelmet, defenceStat},
}

// statPair reads the min/max pair a slot is ranked by. Some rows in the real
// data have the two inverted — Ropaje sombrio is MinDef 50, MaxDef 40 — so the
// pair is normalised by statValue rather than trusted.
type statPair func(Item) (int, int)

func weaponStat(i Item) (int, int)  { return i.MinHit, i.MaxHit }
func defenceStat(i Item) (int, int) { return i.MinDef, i.MaxDef }

// potionStat is the roll a Salud/Agilidad/Fuerza potion restores. Mana potions
// leave both fields at zero and use a formula instead (see drinkPotion), so
// they tie here and fall through to the id — which is fine, since a shop only
// stocks one blue potion once the newbie one is out.
func potionStat(i Item) (int, int) { return i.MinModificador, i.MaxModificador }

func restoresStat(i Item) (int, int) { return i.Restores, i.Restores }

// fitsRace answers whether a character of this race can wear this armour
// without turning into somebody else. Only armour is cut per race: weapons have
// DwarfAnim for the same reason but the server picks that at draw time
// (appearance.go), and shields and helmets have one cut for everyone.
func fitsRace(item Item, race Race) bool {
	if item.Type != ItemArmor {
		return true
	}
	// No gender in this game, so the women's cut is simply a different body
	// and never the right one.
	if item.FemaleArmor {
		return false
	}
	if item.DrowArmor && race != Drow {
		return false
	}
	return item.DwarfArmor == (race == Enano || race == Gnomo)
}

// betterFor reports whether a beats b as a kit piece: more class-specific
// first, then the stronger of the two.
func betterFor(a, b Item, stat statPair) bool {
	if len(a.ForbiddenClasses) != len(b.ForbiddenClasses) {
		return len(a.ForbiddenClasses) > len(b.ForbiddenClasses)
	}
	// StaffPower before raw damage, and only ever as a tiebreak: three mage
	// staffs bar the same eleven classes and differ in little else, and this
	// is the field the source's own NeedStaff gate reads.
	if a.StaffPower != b.StaffPower {
		return a.StaffPower > b.StaffPower
	}
	if v, w := statValue(a, stat), statValue(b, stat); v != w {
		return v > w
	}
	// Two items that tie on everything still have to be told apart, and it has
	// to be by something stable: the candidates arrive from a map, and Go
	// randomises that iteration order, so "whichever came first" would hand out
	// a different kit on every server start.
	return a.ID < b.ID
}

// statValue is the midpoint of a min/max pair, kept doubled to stay in
// integers, with the inverted rows straightened out first.
func statValue(item Item, stat statPair) int {
	low, high := stat(item)
	if high < low {
		low, high = high, low
	}
	return low + high
}

// pickGear is the whole selection rule in one place: of everything a shop
// sells that this class and race can actually wear, the most class-specific
// piece. allow is an extra filter for the one caller that needs it.
func pickGear(items map[int]Item, itemType int, class Class, race Race, stat statPair, allow func(Item) bool) (Item, bool) {
	var best Item
	found := false
	for _, item := range items {
		if item.Type != itemType || !item.Sold || item.Newbie {
			continue
		}
		if classForbidsUse(item, class) || !fitsRace(item, race) {
			continue
		}
		if allow != nil && !allow(item) {
			continue
		}
		if !found || betterFor(item, best, stat) {
			best, found = item, true
		}
	}
	return best, found
}

func computeStartingKit(items map[int]Item, class Class, race Race) startingKit {
	out := startingKit{}
	slot := 0
	next := func() int { s := slot; slot++; return s }
	add := func(item Item, amount int, equipped bool) {
		if item.ID == 0 {
			return
		}
		out.slots = append(out.slots, protocol.InventorySlot{
			Slot: next(), ItemID: item.ID, Amount: amount, Equipped: equipped,
		})
	}

	var weapon Item
	for _, gear := range gearSlots {
		item, ok := pickGear(items, gear.itemType, class, race, gear.stat, nil)
		if !ok {
			// A class with no candidate for a slot goes without it, and that
			// is the right answer rather than a hole to patch: no shop sells a
			// Mago or a Druida a shield, because their CP lists bar them from
			// all thirteen shields in the game.
			continue
		}
		if gear.itemType == ItemWeapon {
			weapon = item
		}
		add(item, 1, true)
	}

	// A bow is the one kit piece that arrives incomplete. Municiones — not
	// Proyectil — is what says so: Cuchillas are thrown and gone, while a bow
	// declares both and is a stick without a quiver behind it.
	if weapon.NeedsAmmo {
		if arrows, ok := pickGear(items, ItemArrow, class, race, weaponStat, nil); ok {
			add(arrows, arrowStack, false)
		}
		// And a sidearm, carried rather than worn. Ranged combat is not
		// implemented yet, so a bow currently swings like a club for its own
		// 6-11 — handing a Cazador only that would make the one class whose kit
		// *is* its identity the one class that cannot fight. The sidearm comes
		// out of the same rule with projectiles excluded, so it is still that
		// class's own weapon and not a generic fallback.
		if melee, ok := pickGear(items, ItemWeapon, class, race, weaponStat,
			func(i Item) bool { return !i.Projectile }); ok {
			add(melee, 1, false)
		}
	}

	// Consumables run through the same filter as the gear — sold in a shop,
	// not newbie — so there is one rule in this file and not two. Potions are
	// taken one per ePocionType rather than "the first potion, then the next
	// different one": Go randomises map iteration order, so a pick that leaned
	// on it genuinely varied between server starts.
	for _, potionType := range kitPotionTypes {
		if potion, ok := pickConsumable(items, ItemPotion, class, potionStat,
			func(i Item) bool { return i.PotionType == potionType }); ok {
			add(potion, potionStack, false)
		}
	}
	if food, ok := pickConsumable(items, ItemFood, class, restoresStat, nil); ok {
		add(food, consumableStack, false)
	}
	if drink, ok := pickConsumable(items, ItemDrink, class, restoresStat, nil); ok {
		add(drink, consumableStack, false)
	}

	return out
}

// pickConsumable is pickGear for things you swallow. Race never gates a bottle,
// and no consumable in the real data carries a CP list at all, so the
// specificity half of the ranking is always a tie here and the strongest one
// wins — which is the right answer for something that is treated as
// ammunition: of the four potions a shop stocks per colour, take the one that
// heals 30 rather than the newbie one that heals 10.
func pickConsumable(items map[int]Item, itemType int, class Class, stat statPair, allow func(Item) bool) (Item, bool) {
	return pickGear(items, itemType, class, Humano, stat, allow)
}

func (k startingKit) inventory() []protocol.InventorySlot {
	return append([]protocol.InventorySlot(nil), k.slots...)
}
