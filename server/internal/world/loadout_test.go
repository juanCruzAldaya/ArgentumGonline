package world

import (
	"fmt"
	"strings"
	"testing"
)

// realItems loads the converted obj.dat the server actually ships. These tests
// are about a selection rule reading real data — a hand-built fixture would
// only prove the rule runs, not that it picks a bow for a Cazador.
func realItems(t *testing.T) map[int]Item {
	t.Helper()
	items, err := LoadItems("../../maps/items.json")
	if err != nil {
		t.Skipf("sin items.json convertido: %v", err)
	}
	return items
}

func kitByType(items map[int]Item, kit startingKit) map[int]Item {
	out := map[int]Item{}
	for _, slot := range kit.slots {
		if !slot.Equipped {
			continue
		}
		out[items[slot.ItemID].Type] = items[slot.ItemID]
	}
	return out
}

// TestStartingKitIsClassGear is the whole promise of the kit in one place:
// everyone is dressed, in gear their own class is allowed to wear, of the tier
// a shop sells rather than the tier a GM hands out.
func TestStartingKitIsClassGear(t *testing.T) {
	items := realItems(t)
	for _, class := range allClasses {
		for _, race := range allRaces {
			kit := computeStartingKit(items, class, race)
			worn := kitByType(items, kit)

			for _, required := range []int{ItemWeapon, ItemArmor} {
				if _, ok := worn[required]; !ok {
					t.Errorf("%s %s: sin objeto equipado de tipo %d", className[class], raceName[race], required)
				}
			}
			for _, item := range worn {
				switch {
				case classForbidsUse(item, class):
					t.Errorf("%s: %q está prohibido para esa clase", className[class], item.Name)
				case !item.Sold:
					t.Errorf("%s: %q no lo vende ningún mercader del original", className[class], item.Name)
				case item.Newbie:
					t.Errorf("%s: %q es equipo newbie", className[class], item.Name)
				case !fitsRace(item, race):
					t.Errorf("%s %s: %q es de otro corte", className[class], raceName[race], item.Name)
				}
			}
		}
	}
}

// TestArmorMatchesRaceCut guards the one mistake that does not look like a
// mistake: armour *is* the body here, so the wrong cut renders somebody else's
// character rather than a wrongly-armoured one.
func TestArmorMatchesRaceCut(t *testing.T) {
	items := realItems(t)
	for _, race := range allRaces {
		short := race == Enano || race == Gnomo
		for _, class := range allClasses {
			armor, ok := kitByType(items, computeStartingKit(items, class, race))[ItemArmor]
			if !ok {
				continue
			}
			if armor.DwarfArmor != short {
				t.Errorf("%s %s: %q tiene corte enano=%v", className[class], raceName[race], armor.Name, armor.DwarfArmor)
			}
			if armor.FemaleArmor {
				t.Errorf("%s %s: %q es el corte de mujer", className[class], raceName[race], armor.Name)
			}
		}
	}
}

// TestHunterGetsBowAndArrows pins the case the whole "most class-specific
// first" tiebreak exists for. Ranked by damage alone a Cazador would take the
// Hacha Larga de Guerra (10-18) over the Arco de Cazador (6-11) and stop being
// a hunter; ranked by how few classes an item allows, the bow wins. The quiver
// and the sidearm come with it — the first because a bow without arrows is a
// stick, the second because nothing fires them yet.
func TestHunterGetsBowAndArrows(t *testing.T) {
	items := realItems(t)
	kit := computeStartingKit(items, Cazador, Humano)

	var bow, arrows, sidearm bool
	for _, slot := range kit.slots {
		item := items[slot.ItemID]
		switch {
		case item.Type == ItemWeapon && slot.Equipped:
			bow = item.Projectile && item.NeedsAmmo
		case item.Type == ItemArrow:
			arrows = slot.Amount == arrowStack
		case item.Type == ItemWeapon && !slot.Equipped:
			sidearm = !item.Projectile
		}
	}
	if !bow {
		t.Error("el Cazador no arranca con un arco equipado")
	}
	if !arrows {
		t.Error("el Cazador no arranca con flechas")
	}
	if !sidearm {
		t.Error("el Cazador no arranca con un arma cuerpo a cuerpo de repuesto")
	}
}

// TestMagesCarryNoShield is the negative case: a slot with no candidate stays
// empty instead of being filled with something the class cannot use. Every one
// of the game's shields names Mago and Druida in its CP list.
func TestMagesCarryNoShield(t *testing.T) {
	items := realItems(t)
	for _, class := range []Class{Mago, Druida} {
		if _, ok := kitByType(items, computeStartingKit(items, class, Humano))[ItemShield]; ok {
			t.Errorf("%s arranca con escudo", className[class])
		}
	}
}

// TestKitTable is not an assertion, it is the table itself: sixty kits printed
// so a balance change is read rather than guessed at. `go test -run TestKitTable -v`.
func TestKitTable(t *testing.T) {
	items := realItems(t)
	var b strings.Builder
	fmt.Fprintf(&b, "\n%-11s %-30s %-30s %-24s %-22s\n", "CLASE", "ARMA", "ARMADURA (Humano)", "CASCO", "ESCUDO")
	for _, class := range allClasses {
		worn := kitByType(items, computeStartingKit(items, class, Humano))
		show := func(t int, low, high func(Item) int) string {
			item, ok := worn[t]
			if !ok {
				return "—"
			}
			return fmt.Sprintf("%s %d-%d", item.Name, low(item), high(item))
		}
		hit := func(i Item) int { return i.MinHit }
		hitMax := func(i Item) int { return i.MaxHit }
		def := func(i Item) int { return i.MinDef }
		defMax := func(i Item) int { return i.MaxDef }
		fmt.Fprintf(&b, "%-11s %-30s %-30s %-24s %-22s\n", className[class],
			show(ItemWeapon, hit, hitMax), show(ItemArmor, def, defMax),
			show(ItemHelmet, def, defMax), show(ItemShield, def, defMax))
	}
	t.Log(b.String())
}
