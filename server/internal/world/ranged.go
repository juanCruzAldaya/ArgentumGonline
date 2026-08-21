package world

import (
	"math"

	"juegito/server/internal/protocol"
)

// Combate a distancia: arcos, flechas y cuchillas arrojadizas.
//
// Ported from LanzarProyectil and the projectile arms of UsuarioAtacaUsuario
// and CalcularDano (SistemaCombate.bas). The data was already here — obj.dat
// declares Proyectil and Municiones, the starting kits already hand a Cazador a
// bow and a quiver — and until now the bow was a stick you hit people with,
// which is a strange thing for the one class whose whole identity is the bow.
//
// Two weapons take this path and they are not the same thing. A bow declares
// both Proyectil and Municiones, and spends an arrow per shot. Cuchillas
// declare only Proyectil: they are the throw itself, so what the shot spends is
// the weapon. LanzarProyectil branches on Municiones for exactly this, and so
// does spendAmmo below.
//
// What is deliberately not ported is the stamina cost. The source takes 1..10
// energy per shot, and this game does not meter energy at all — see the same
// decision, with its reasons, in spells.go. Turning it back on for archery
// alone would make the bow the one weapon with a resource nothing refills.

// poderAtaqueProyectil is PoderAtaqueProyectil: the same four skill bands as
// PoderAtaqueArma, reading the Proyectiles skill and the class's own projectile
// column instead.
func (w *World) poderAtaqueProyectil(p *Player) float64 {
	mods := classModifiers[p.Class]
	agility := float64(w.effectiveAttributes(p).Agilidad)
	skill := float64(p.Skills.Proyectiles)

	var power float64
	switch {
	case skill < 31:
		power = skill * mods.AtaqueProyectiles
	case skill < 61:
		power = (skill + agility) * mods.AtaqueProyectiles
	case skill < 91:
		power = (skill + 2*agility) * mods.AtaqueProyectiles
	default:
		power = (skill + 3*agility) * mods.AtaqueProyectiles
	}
	return power + 2.5*math.Max(float64(p.Vitals.Level-12), 0)
}

// calcShotDamage is CalcularDano's projectile arm.
//
// The shape is the melee one — see calcDamage — with two changes the source
// makes and this follows: the arrow's own roll is added to the weapon's, and
// the class multiplier is DanoProyectiles rather than DanoArmas. A thrown blade
// has no ammunition, so it is only the weapon's roll.
func (w *World) calcShotDamage(p *Player, weapon Item, ammo Item, loaded bool) int {
	minHit, maxHit := weapon.MinHit, weapon.MaxHit
	if maxHit < minHit {
		maxHit = minHit
	}
	roll := float64(w.randRange(minHit, maxHit))
	if loaded {
		high := ammo.MaxHit
		if high < ammo.MinHit {
			high = ammo.MinHit
		}
		roll += float64(w.randRange(ammo.MinHit, high))
		// The strength bonus keeps reading the weapon's own ceiling, not the
		// arrow's: it is the draw of the bow that strength helps with.
	}

	strength := float64(w.effectiveAttributes(p).Fuerza)
	strengthBonus := float64(maxHit) / 5 * math.Max(strength-15, 0)
	bodyRoll := float64(w.randRange(fistMinDamage, fistMaxDamage))

	damage := (3*roll + strengthBonus + bodyRoll) * classModifiers[p.Class].DanoProyectiles
	return max(int(damage), 1)
}

// equippedAmmo is the equipped quiver, if there is one.
//
// Ammunition is equipment here — see equipmentTypes — which is the source's own
// MunicionEqpSlot: arrows are worn, not consumed by hand, and equipping a
// better arrow takes the old one off exactly as a better sword does.
func (w *World) equippedAmmo(p *Player) (Item, int, bool) {
	for i, slot := range p.Inventory {
		if !slot.Equipped || slot.Amount < 1 {
			continue
		}
		if item, ok := w.items[slot.ItemID]; ok && item.Type == ItemArrow {
			return item, i, true
		}
	}
	return Item{}, -1, false
}

// equippedWeaponSlot is the equipped weapon and where it sits in the bag. The
// index matters here and nowhere else: a thrown blade is spent by the shot, so
// the shot has to be able to find the slot it came out of.
func (w *World) equippedWeaponSlot(p *Player) (Item, int, bool) {
	for i, slot := range p.Inventory {
		if !slot.Equipped {
			continue
		}
		if item, ok := w.items[slot.ItemID]; ok && item.Type == ItemWeapon {
			return item, i, true
		}
	}
	return Item{}, -1, false
}

// aimProjectile answers "usar" on a projectile weapon by asking who to shoot.
//
// This is UsarInvItem's otWeapon branch, and it is the whole input model for
// ranged combat in Argentum: the bow is equipped like any weapon, and then it
// is *used* — the same double-click a potion takes — and what using it does is
// put the crosshair up. The attack key never comes into it, which is also why
// the bow keeps swinging for its own damage if you press it.
//
// The unequipped case is the source's own sentence, kept word for word: a bow
// in the bag is not a bow in your hands.
func (w *World) aimProjectile(p *Player, idx int, item Item) {
	if !p.Inventory[idx].Equipped {
		w.sendTo(p, protocol.TypeUseResult, protocol.UseResult{
			ItemName: item.Name,
			Failed:   "Antes de usar el arco deberías equipártelo.",
		})
		return
	}
	// No ammunition check here, deliberately: the source arms the crosshair and
	// lets LanzarProyectil be the one to say "no tienes municiones", and so
	// does shoot() below. Refusing here would answer a question the player has
	// not asked yet — they might be about to pick arrows up.
	w.sendTo(p, protocol.TypeUseResult, protocol.UseResult{ItemName: item.Name, Aim: true})
}

// shoot fires at somebody named by the client.
//
// Naming a target is the one thing melee never does, and it is checked rather
// than trusted: the target has to be inside the shooter's own viewport, which
// is the same bound spells are held to and the same window the snapshots are
// cut from. A modified client cannot shoot at somebody the server never told it
// about.
func (w *World) shoot(p *Player, targetID EntityID) {
	// The same three gates a swing goes through, for the same reasons: see
	// attack(). Immobilized still shoots — it roots the feet, not the arms.
	if p.Dead || !p.canAct(w.tick) || p.Meditating {
		return
	}
	// And the same two intervals. An arrow is an attack: it shares the melee
	// cooldown rather than getting one of its own, so switching between the bow
	// and the blade cannot be used to attack twice as often.
	if w.tick-p.lastAttackTick < attackCooldownTicks {
		return
	}
	if w.tick-p.lastCastTick < castToAttackTicks {
		return
	}

	weapon, weaponSlot, armed := w.equippedWeaponSlot(p)
	if !armed || !weapon.Projectile {
		w.shotFailed(p, "No tenés un arco ni una cuchilla equipada.")
		return
	}

	ammo, ammoSlot, loaded := w.equippedAmmo(p)
	if weapon.NeedsAmmo && !loaded {
		w.shotFailed(p, "No tenés flechas equipadas.")
		return
	}

	victim, ok := w.players[targetID]
	switch {
	case !ok || victim.Dead:
		w.shotFailed(p, "Ahí no hay a quién tirarle.")
		return
	case victim.ID == p.ID:
		// The same rule the self-target guard in spells.go arrived at, and for
		// the same reason: an offensive thing aimed at yourself is a misclick,
		// and a misclick should cost nothing. Checked before the arrow is spent.
		w.shotFailed(p, "No podés dispararte a vos mismo.")
		return
	case !w.withinViewport(p, victim):
		w.shotFailed(p, "Está demasiado lejos.")
		return
	}

	p.lastAttackTick = w.tick

	// Shooting reveals whoever was hidden, exactly as swinging and casting do.
	// It happens before the arrow goes out, so the shot is never drawn leaving
	// a character nobody can see.
	if p.invisible(w.tick) {
		p.revealHidden()
	}

	// The arrow first, then what it did. Everyone in view gets to watch it
	// cross; only the two people involved are told how it landed.
	flying := weapon.ID
	if loaded {
		flying = ammo.ID
	}
	w.broadcastProjectile(p, victim, flying)

	result := w.resolveHit(p, victim, w.poderAtaqueProyectil(p), func() int {
		return w.calcShotDamage(p, weapon, ammo, loaded)
	})
	if result.Hit && victim.Meditating {
		victim.stopMeditating()
	}
	w.reportCombat(p, victim, result, true)

	// Spent whether or not it hit: the source takes the arrow away for any shot
	// that was actually let go, including one that misses, and that is the
	// whole economy of the bow.
	if weapon.NeedsAmmo {
		w.spendAmmo(p, ammoSlot)
	} else {
		w.spendAmmo(p, weaponSlot)
	}

	if result.Killed {
		w.kill(victim, p)
		w.log.Info("player shot dead",
			"victim", victim.Name, "by", p.Name, "arma", weapon.Name, "alive", w.aliveCount())
	}
}

// spendAmmo takes one unit off a slot and, when that empties it, takes the slot
// with it.
//
// An emptied slot is removed rather than left at zero because everything else
// in this inventory works that way — see consumeStack, which this deliberately
// mirrors — and because an equipped quiver holding nothing would keep answering
// equippedAmmo and let somebody shoot forever.
func (w *World) spendAmmo(p *Player, idx int) {
	if idx < 0 || idx >= len(p.Inventory) {
		return
	}
	p.Inventory[idx].Amount--
	if p.Inventory[idx].Amount <= 0 {
		p.Inventory = append(p.Inventory[:idx], p.Inventory[idx+1:]...)
	}
	w.sendLoadout(p)
}

// broadcastProjectile puts one arrow in the air for everybody who can see it.
//
// The viewport of the person watching, not of the shooter: an arrow that
// crosses into your screen is a thing you should see even if whoever loosed it
// is off the edge of it — which is precisely the interesting case, since it is
// how you learn there is an archer over there at all.
func (w *World) broadcastProjectile(from, to *Player, itemID int) {
	shot := protocol.Projectile{
		FromID: uint32(from.ID),
		ToID:   uint32(to.ID),
		FromX:  from.X, FromY: from.Y,
		ToX: to.X, ToY: to.Y,
		ItemID: itemID,
	}
	for _, other := range w.players {
		if w.withinViewport(other, from) || w.withinViewport(other, to) {
			w.sendTo(other, protocol.TypeProjectile, shot)
		}
	}
}

// shotFailed tells the shooter why nothing happened.
//
// Melee has no equivalent and does not want one: swinging at an empty tile is
// its own answer, and the player can see the tile. A quiver that ran out is
// invisible from where they are sitting, so the key simply stopping would be
// indistinguishable from a bug.
func (w *World) shotFailed(p *Player, reason string) {
	w.sendTo(p, protocol.TypeCombat, protocol.CombatEvent{
		AttackerID:   uint32(p.ID),
		AttackerName: p.Name,
		Ranged:       true,
		Mine:         true,
		Failed:       reason,
	})
}
