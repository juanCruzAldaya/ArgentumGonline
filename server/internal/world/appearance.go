package world

// Appearance: what a character looks like, derived from what they are wearing.
//
// This is Argentum's own model, not an invention. Equipping armour does not
// put a layer on top of you — it *replaces your body*: obj.dat's NumRopaje
// becomes Char.body (InvUsuario.bas:1395). Weapon, shield and helmet are
// separate animation indices drawn over that body. Take the armour off and the
// body reverts to a per-race naked body (DarCuerpoDesnudo, General.bas:45-114),
// never to whatever it was before.

// nakedBodies is DarCuerpoDesnudo's table, male column — this game has no
// gender, so the female bodies (39/259/40/260/60) go unused. The source's own
// Select Case lists its arms in a different order than the eRaza enum, so
// these are read off the values, not the arm order.
var nakedBodies = map[Race]int{
	Humano: 21,
	Elfo:   210,
	Drow:   32,
	Gnomo:  222,
	Enano:  53,
}

// noneAnim is the source's "nothing equipped" sentinel for weapon, shield and
// helmet alike: NingunArma, NingunEscudo and NingunCasco are all **2**, not 0
// (Declares.bas:251-255).
//
// It works because index 2 is deliberately empty in the asset files — Armas.dat
// has no [ARMA2], Escudos.dat has no [ESC2] and says so in a comment on its
// first line, and Cascos.ini's [HEAD2] is four zeroes. Porting this as 0 would
// break twice over: unequipping would ask for index 0, and any item that
// legitimately has Anim=2 would go invisible.
const noneAnim = 2

// Appearance is the six numbers that fully describe how a character is drawn.
// Everything else — race, class, which item ids are equipped — stays server
// side. This is exactly the set the original puts on the wire in
// CharacterCreate/CharacterChange.
type Appearance struct {
	Body   int
	Head   int
	Weapon int
	Shield int
	Helmet int
}

// appearanceOf recomputes a player's look from their equipped items.
//
// Recomputed from scratch on every change rather than mutated in place. The
// original mutates, and pays for it: its helmet-unequip branch has an `Or`
// where the armour path has an `And` (InvUsuario.bas:984 against :966), so a
// mimicked player gets their helmet silently cleared. Deriving the whole thing
// from the inventory makes that class of bug unrepresentable.
func (w *World) appearanceOf(p *Player) Appearance {
	if p.Dead {
		return Appearance{Body: ghostBody, Head: ghostHead, Weapon: noneAnim, Shield: noneAnim, Helmet: noneAnim}
	}

	look := Appearance{
		Body:   nakedBodies[p.Race],
		Head:   p.Head,
		Weapon: noneAnim,
		Shield: noneAnim,
		Helmet: noneAnim,
	}

	for _, slot := range p.Inventory {
		if !slot.Equipped {
			continue
		}
		item, ok := w.items[slot.ItemID]
		if !ok {
			continue
		}
		switch item.Type {
		case ItemArmor:
			// Only armour turns NumRopaje into a body. Shields and helmets
			// carry a leftover NumRopaje=2 in obj.dat — 13 of 13 shields and
			// 20 of 21 helmets — and reading it for them would drop everyone
			// into body 2 the moment they raised a shield. The converter
			// already refuses to export it for those types; this is the second
			// half of the same guard.
			if item.Body > 0 {
				look.Body = item.Body
			}
		case ItemWeapon:
			look.Weapon = weaponAnimFor(item, p.Race)
		case ItemShield:
			if item.Anim > 0 {
				look.Shield = item.Anim
			}
		case ItemHelmet:
			if item.Anim > 0 {
				look.Helmet = item.Anim
			}
		}
	}
	return look
}

// weaponAnimFor is GetWeaponAnim (Modulo_UsUaRiOs.bas:344-370): weapons — and
// only weapons — have an alternate animation for the short races. The obj.dat
// field is named RazaEnanaAnim but the check covers Gnomo as well as Enano, so
// the name is about body shape rather than about race.
//
// One weapon in the whole file has no Anim at all, which yields 0 — neither a
// valid index nor the sentinel. Treat it as "nothing", explicitly, rather than
// letting a 0 reach the client.
func weaponAnimFor(item Item, race Race) int {
	if item.DwarfAnim > 0 && (race == Enano || race == Gnomo) {
		return item.DwarfAnim
	}
	if item.Anim > 0 {
		return item.Anim
	}
	return noneAnim
}
