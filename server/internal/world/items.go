package world

import (
	"encoding/json"
	"fmt"
	"os"
)

// Argentum object types, from the table at the top of obj.dat.
const (
	ItemFood   = 1
	ItemWeapon = 2
	ItemArmor  = 3
	ItemPotion = 11
	ItemDrink  = 13
	ItemShield = 16
	ItemHelmet = 17
	ItemRing   = 18
	// ItemArrow is otFlechas. Nothing fires them — ranged combat does not
	// exist yet — but a bow without them is a stick, so the classes whose kit
	// is a bow carry a quiver anyway. See computeStartingKit.
	ItemArrow = 32
)

// Argentum potion types (ePocionType), which decide what an otPociones item
// actually does — ObjType alone only says "this is a potion."
const (
	PotionAgility    = 1
	PotionStrength   = 2
	PotionHealth     = 3
	PotionMana       = 4
	PotionCurePoison = 5
	PotionBlack      = 6 // the joke item: kills whoever drinks it
)

// Item is one obj.dat entry, as converted by tools/aoconv. The server reads the
// same file the client does, so the numbers combat runs on and the icons the
// player sees can never describe different objects.
type Item struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Grh  int    `json:"grh"`

	// Appearance, straight from obj.dat via tools/aoconv. Body is NumRopaje
	// and is only ever populated for armour; Anim is the weapon/shield/helmet
	// animation index; DwarfAnim is a weapon's short-race variant. See
	// appearance.go for how they combine, and the converter for why Body is
	// not read for shields and helmets.
	Body      int `json:"body,omitempty"`
	Anim      int `json:"anim,omitempty"`
	DwarfAnim int `json:"dwarfAnim,omitempty"`

	Type   int `json:"type"`
	MinHit int `json:"minHit"`
	MaxHit int `json:"maxHit"`
	MinDef int `json:"minDef"`
	MaxDef int `json:"maxDef"`
	// StaffPower is real obj.dat data (modHechizos.bas gates NeedStaff spells
	// on a Mago's equipped weapon meeting this), carried over for when spell
	// casting enforces it. Nothing reads it yet — casting does not check it,
	// and it does not distinguish newbie-tier weapons from one another; every
	// newbie weapon, including the newbie staff, is StaffPower 0 in the real
	// data. See loadout.go for how the starting kit actually picks a weapon.
	StaffPower int `json:"staffPower,omitempty"`
	// Restores is the flat hunger (food) or thirst (drink) a UseOnce/Bebidas
	// item gives back.
	Restores int `json:"restores"`
	Value    int `json:"value"`

	// Potion fields. TipoPocion selects the effect; MinModificador/
	// MaxModificador is the roll a Salud/Agilidad/Fuerza potion uses (Mana
	// potions ignore these and use a formula instead — ported faithfully in
	// useItem, not guessed).
	//
	// DuracionEfecto is parsed but deliberately not used: it is the source's
	// buff duration in the same real-time pulse units as the spell durations
	// documented in status.go, and stacking a second inferred pulse-to-tick
	// conversion on top of the first felt like compounding one guess with
	// another for no real gain. Potion buffs share buffDurationTicks/
	// debuffDurationTicks with spell buffs instead — one duration to reason
	// about and retune.
	PotionType     int `json:"potionType,omitempty"`
	MinModificador int `json:"minMod,omitempty"`
	MaxModificador int `json:"maxMod,omitempty"`
	DuracionEfecto int `json:"potionDuration,omitempty"`

	// Projectile is obj.dat's Proyectil and NeedsAmmo its Municiones. Not the
	// same flag: Cuchillas are thrown and gone (Proyectil, no Municiones)
	// while a bow declares both, which is what decides who gets arrows.
	Projectile bool `json:"projectile,omitempty"`
	NeedsAmmo  bool `json:"needsAmmo,omitempty"`

	// Newbie is obj.dat's own starter-tier flag, and it agrees exactly with
	// the "(Newbie)" name suffix — 29 items, both ways, no disagreements.
	Newbie bool `json:"newbie,omitempty"`

	// The cut armour comes in. Equipping armour *is* changing your body here
	// (see appearance.go), so the wrong one does not look wrong, it looks like
	// somebody else. DwarfArmor is RazaEnana, which covers Enano and Gnomo
	// both, the same way DwarfAnim does for weapons.
	DwarfArmor  bool `json:"dwarfArmor,omitempty"`
	DrowArmor   bool `json:"drowArmor,omitempty"`
	FemaleArmor bool `json:"femaleArmor,omitempty"`

	// Sold says a merchant NPC in the original stocks this item. It is the
	// line between the gear a character starts with and the gear worth
	// crossing the map for — see loadout.go.
	Sold bool `json:"sold,omitempty"`

	// ForbiddenClasses is obj.dat's CP1..CP12: a deny list, not an allow list.
	// See classForbidsUse.
	ForbiddenClasses []string `json:"forbiddenClasses,omitempty"`
}

// aoClassNames are the literal uppercase tokens obj.dat's CP fields use —
// the source's own eClass enum names, ASCII, no accents. This is the one
// place that maps them to our Class ids; everything else reads Class.
var aoClassNames = [...]string{
	"GUERRERO", "CAZADOR", "PALADIN", "BANDIDO", "ASESINO", "PIRATA",
	"LADRON", "CLERIGO", "BARDO", "MAGO", "DRUIDA", "TRABAJADOR",
}

// classForbidsUse is ClasePuedeUsarItem's actual check (InvUsuario.bas):
// EquiparInvItem rejects equipping only when the wearer's class appears in
// the item's own CP1..CP12 list. 213 of 363 carriable equippables (59%) name
// at least one forbidden class in the real data; consumables never do.
func classForbidsUse(item Item, class Class) bool {
	if int(class) < 0 || int(class) >= len(aoClassNames) {
		return false
	}
	name := aoClassNames[class]
	for _, forbidden := range item.ForbiddenClasses {
		if forbidden == name {
			return true
		}
	}
	return false
}

// LoadItems reads the converted obj.dat.
func LoadItems(path string) (map[int]Item, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var byID map[int]Item
	if err := json.Unmarshal(raw, &byID); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return byID, nil
}

// startingKitKey identifies one (class, race) starting kit — the newbie
// armour varies by race as well as the weapon by class, so the cache is
// keyed on both.
type startingKitKey struct {
	class Class
	race  Race
}

// Ground spawn densities, both expressed as "one stack per N walkable tiles".
//
// Walkable, not raw W*H, which is what groundLootTiles used to divide. Half of
// Ullathorpe is wall, so the old figure delivered half the density it claimed,
// and on a denser map it would have delivered less still. groundLootTiles is
// 30 rather than 60 purely to absorb that correction: 4962 walkable tiles / 30
// is 165 gear spawns, which is what 100*100/60 was already producing there.
// Gear on the map is unchanged; only the arithmetic behind it now means
// something on a map that isn't half open ground.
//
// Both numbers were an order of magnitude denser while the game was played on
// one 100x100 map. Potions in particular sat at one stack of 25 every four
// walkable tiles — deliberately, so they read as ammunition rather than as a
// find. On Ullathorpe that was ~1220 stacks and felt right: the map is small
// enough that sparse loot means walking past empty ground for the whole match.
//
// A composed world is 820x820 with ~330.000 walkable tiles, and the same
// per-tile rate produced 79.483 stacks and 1,98 million loose potions. Nothing
// broke — the density per tile is what it always was — but it stopped being a
// battle royale: a refill was never more than a couple of steps away, so
// running out was not a thing that could happen, and neither was choosing to
// fight over a supply. Finding something has to be an event.
//
// The rates now: gear every 220 walkable tiles (~1.500 in a world), potions
// every 110 (~3.000 stacks of 5, so ~15.000 potions). At Argentum's 5 tiles a
// second that is a potion roughly every twenty seconds of walking and a piece
// of gear every forty — often enough to reward exploring, rare enough that
// what you are carrying is a decision.
//
// minGroundLoot is a floor for small maps, so a tiny arena still has gear.
const (
	groundLootTiles   = 220
	minGroundLoot     = 30
	groundPotionTiles = 110
	groundPotionStack = 5
)

// SetItems installs the item table and the starting kit for every class/race
// pair, then scatters ground loot across the map. Must be called after New
// (which sets the grid loot is scattered onto) and before Run. Precomputing
// all 60 starting-kit combinations here — rather than per join — keeps Join
// itself cheap; there are only twelve classes and five races.
func (w *World) SetItems(items map[int]Item) {
	w.items = items
	w.startingKits = make(map[startingKitKey]startingKit, len(allClasses)*len(allRaces))
	for _, class := range allClasses {
		for _, race := range allRaces {
			w.startingKits[startingKitKey{class, race}] = computeStartingKit(items, class, race)
		}
	}

	// One shuffled pool of free tiles, drained by both passes, so gear and
	// potions can never be assigned the same tile — Argentum's map format
	// carries one object per tile and groundStack keeps that constraint.
	pool := w.freeTilePool()

	gear := len(pool) / groundLootTiles
	if gear < minGroundLoot {
		gear = minGroundLoot
	}
	pool = w.spawnGroundLoot(pool, gear)
	w.spawnGroundPotions(pool, len(pool)/groundPotionTiles)
}

// equippedOfType returns the equipped item of a given type, if any.
func (w *World) equippedOfType(p *Player, itemType int) (Item, bool) {
	for _, slot := range p.Inventory {
		if !slot.Equipped {
			continue
		}
		if item, ok := w.items[slot.ItemID]; ok && item.Type == itemType {
			return item, true
		}
	}
	return Item{}, false
}

func (w *World) equippedWeapon(p *Player) (Item, bool) {
	return w.equippedOfType(p, ItemWeapon)
}

func (w *World) equippedShield(p *Player) (Item, bool) {
	return w.equippedOfType(p, ItemShield)
}

// equippedDefence is everything that soaks damage. Argentum rolls armour and
// helmet separately against the body part it hit; this sums them, which is the
// same idea without the hit-location table.
func (w *World) equippedDefence(p *Player) []Item {
	var out []Item
	for _, itemType := range []int{ItemArmor, ItemHelmet} {
		if item, ok := w.equippedOfType(p, itemType); ok {
			out = append(out, item)
		}
	}
	return out
}

// equipmentTypes toggle Equipped when used; everything else is consumed.
var equipmentTypes = map[int]bool{
	ItemWeapon: true, ItemShield: true, ItemArmor: true, ItemHelmet: true, ItemRing: true,
}
