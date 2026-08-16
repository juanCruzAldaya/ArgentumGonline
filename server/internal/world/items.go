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

// Item is one obj.dat entry, as converted by tools/aoconv. The server reads the
// same file the client does, so the numbers combat runs on and the icons the
// player sees can never describe different objects.
type Item struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Grh      int    `json:"grh"`
	Type     int    `json:"type"`
	MinHit   int    `json:"minHit"`
	MaxHit   int    `json:"maxHit"`
	MinDef   int    `json:"minDef"`
	MaxDef   int    `json:"maxDef"`
	Restores int    `json:"restores"`
	Value    int    `json:"value"`
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

// SetItems installs the item table. It must be called before Run, since the
// world goroutine reads it afterwards.
func (w *World) SetItems(items map[int]Item) { w.items = items }

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
