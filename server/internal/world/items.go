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
	ID     int    `json:"id"`
	Name   string `json:"name"`
	Grh    int    `json:"grh"`
	Type   int    `json:"type"`
	MinHit int    `json:"minHit"`
	MaxHit int    `json:"maxHit"`
	MinDef int    `json:"minDef"`
	MaxDef int    `json:"maxDef"`
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

// SetItems installs the item table and derives the spawn loadout for every
// class from it. It must be called before Run, since the world goroutine
// reads both afterwards. Precomputing all twelve here — rather than
// recomputing per join — keeps Join itself cheap; there are only twelve
// classes, so the up-front cost is trivial.
func (w *World) SetItems(items map[int]Item) {
	w.items = items
	w.loadouts = make(map[Class]bestLoadout, len(allClasses))
	for _, class := range allClasses {
		w.loadouts[class] = computeBestLoadout(items, class)
	}
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
