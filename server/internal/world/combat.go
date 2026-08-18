package world

import (
	"math"

	"juegito/server/internal/protocol"
)

// Melee combat, ported from SistemaCombate.bas.
//
// The formulas are Argentum's, copied rather than reinvented — twenty years of
// balance is not worth re-deriving. What is deliberately different is what
// happens at zero health: Argentum revives you, a battle royale does not.

const (
	// attackCooldownTicks gates swings. At 20 Hz this is one attack every
	// 1.2 seconds, matching classic Argentum's melee interval.
	attackCooldownTicks = 24

	// Argentum clamps every hit roll into this band, so even a hopeless
	// matchup lands one in ten and even a lopsided one misses one in ten.
	minHitChance = 10.0
	maxHitChance = 90.0

	// Bare-handed damage range, used when nothing is equipped.
	fistMinDamage = 1
	fistMaxDamage = 3
)

// AttackResult is what one swing did, for the log and the wire.
type AttackResult struct {
	Attacker EntityID
	Victim   EntityID
	Hit      bool
	Blocked  bool
	Damage   int
	Killed   bool
}

// poderEvasion is PoderEvasion from SistemaCombate.bas.
func (w *World) poderEvasion(p *Player) float64 {
	mods := classModifiers[p.Class]
	tacticas := float64(p.Skills.Tacticas)
	agility := float64(w.effectiveAttributes(p).Agilidad)
	base := (tacticas + tacticas/33*agility) * mods.Evasion
	return base + 2.5*math.Max(float64(p.Vitals.Level-12), 0)
}

// poderEvasionEscudo is PoderEvasionEscudo: half the defence skill, scaled by
// how well the class handles a shield.
func (w *World) poderEvasionEscudo(p *Player) float64 {
	return float64(p.Skills.Defensa) * classModifiers[p.Class].Escudo / 2
}

// poderAtaque is PoderAtaqueArma, or PoderAtaqueWrestling bare-handed.
//
// The weapon branch steps agility in at skill 31, 61 and 91: a beginner swings
// on skill alone, an expert brings three times their agility to bear.
func (w *World) poderAtaque(p *Player) float64 {
	mods := classModifiers[p.Class]
	agility := float64(w.effectiveAttributes(p).Agilidad)

	var power float64
	if _, armed := w.equippedWeapon(p); armed {
		skill := float64(p.Skills.Armas)
		switch {
		case skill < 31:
			power = skill * mods.AtaqueArmas
		case skill < 61:
			power = (skill + agility) * mods.AtaqueArmas
		case skill < 91:
			power = (skill + 2*agility) * mods.AtaqueArmas
		default:
			power = (skill + 3*agility) * mods.AtaqueArmas
		}
	} else {
		power = float64(p.Skills.Wrestling) * mods.AtaqueWrestling
	}
	return power + 2.5*math.Max(float64(p.Vitals.Level-12), 0)
}

// hitChance is the ProbExito line of UsuarioImpacto, clamped exactly as the
// original clamps it.
func (w *World) hitChance(attacker, victim *Player) float64 {
	evasion := w.poderEvasion(victim)
	if _, shielded := w.equippedShield(victim); shielded {
		evasion += w.poderEvasionEscudo(victim)
	}
	chance := 50 + (w.poderAtaque(attacker)-evasion)*0.4
	if victim.Meditating {
		// SistemaCombate.bas cuts a meditating target's miss chance by a
		// quarter: sitting still to regen mana costs evasion, not just the
		// ability to fight back.
		chance = 100 - (100-chance)*0.75
	}
	return math.Max(minHitChance, math.Min(maxHitChance, chance))
}

// blockChance is the shield rejection roll a miss falls through to: how much of
// the victim's combat training sits in defence rather than tactics.
func (w *World) blockChance(victim *Player) float64 {
	total := float64(max(victim.Skills.Defensa+victim.Skills.Tacticas, 1))
	chance := 100 * float64(victim.Skills.Defensa) / total
	return math.Max(minHitChance, math.Min(maxHitChance, chance))
}

// calcDamage is CalcularDano's user-versus-user branch:
//
//	(3 * weaponRoll + (weaponMax / 5) * max(0, strength - 15) + bodyRoll) * classMod
//
// The weapon dominates, strength above 15 adds a slice of the weapon's ceiling,
// and the class multiplier decides whether a swing lands like a warrior's.
func (w *World) calcDamage(p *Player) int {
	mods := classModifiers[p.Class]

	weapon, armed := w.equippedWeapon(p)
	minHit, maxHit := fistMinDamage, fistMaxDamage
	modifier := mods.DanoWrestling
	if armed {
		minHit, maxHit = weapon.MinHit, weapon.MaxHit
		modifier = mods.DanoArmas
	}
	if maxHit < minHit {
		maxHit = minHit
	}

	weaponRoll := float64(w.randRange(minHit, maxHit))
	strength := float64(w.effectiveAttributes(p).Fuerza)
	strengthBonus := float64(maxHit) / 5 * math.Max(strength-15, 0)
	bodyRoll := float64(w.randRange(fistMinDamage, fistMaxDamage))

	damage := (3*weaponRoll + strengthBonus + bodyRoll) * modifier
	return max(int(damage), 1)
}

// armorAbsorption is how much the victim's armour and helmet soak up.
func (w *World) armorAbsorption(p *Player) int {
	total := 0
	for _, item := range w.equippedDefence(p) {
		high := item.MaxDef
		if high < item.MinDef {
			high = item.MinDef
		}
		total += w.randRange(item.MinDef, high)
	}
	return total
}

// resolveAttack runs one swing. It is called from the world goroutine, so it
// mutates both players directly.
func (w *World) resolveAttack(attacker, victim *Player) AttackResult {
	result := AttackResult{Attacker: attacker.ID, Victim: victim.ID}

	if w.randRange(1, 100) > int(w.hitChance(attacker, victim)) {
		// A miss can still be a shield block, which is worth reporting
		// separately: it tells the victim their shield is doing something.
		if _, shielded := w.equippedShield(victim); shielded {
			result.Blocked = w.randRange(1, 100) <= int(w.blockChance(victim))
		}
		return result
	}

	result.Hit = true
	damage := w.calcDamage(attacker) - w.armorAbsorption(victim)
	result.Damage = max(damage, 1)

	victim.Vitals.HP -= result.Damage
	if victim.Vitals.HP <= 0 {
		victim.Vitals.HP = 0
		result.Killed = true
	}
	return result
}

// attack swings at whoever stands on the tile the player faces.
//
// The target comes from the attacker's own heading rather than from anything
// the client sends: a modified client can ask to swing, but it cannot choose
// who it reaches.
func (w *World) attack(p *Player) {
	// Immobilize roots the feet, not the arms: melee still works. Paralysis
	// stops everything, which is what canAct checks. Meditating blocks a swing
	// outright too — HandleAttack's own gate, separate from canAct because a
	// meditating player is not paralyzed, just unwilling to break concentration.
	if p.Dead || !p.canAct(w.tick) || p.Meditating {
		return
	}
	if w.tick-p.lastAttackTick < attackCooldownTicks {
		return
	}
	// IntervaloMagiaGolpe: a swing is also gated by how recently you cast, so
	// magic and melee cannot be fired together. See the interval block in
	// spells.go for why the two crossed intervals matter.
	if w.tick-p.lastCastTick < castToAttackTicks {
		return
	}
	p.lastAttackTick = w.tick

	// Swinging reveals whoever was hidden, spell or skill alike.
	if p.invisible(w.tick) {
		p.revealHidden()
	}

	dx, dy := p.Heading.Delta()
	victimID, ok := w.occupied[tileKey{p.X + dx, p.Y + dy}]
	if !ok {
		return
	}
	victim, ok := w.players[victimID]
	if !ok || victim.Dead || victim.ID == p.ID {
		return
	}

	result := w.resolveAttack(p, victim)
	if result.Hit && victim.Meditating {
		// Any landed hit breaks concentration — SistemaCombate's own
		// unconditional cancel for a player victim, unlike the damage-threshold
		// roll it uses against an NPC attacker (this game has no NPCs yet).
		victim.stopMeditating()
	}
	w.reportCombat(p, victim, result)

	if result.Killed {
		w.kill(victim, p)
		w.log.Info("player killed", "victim", victim.Name, "by", p.Name, "alive", w.aliveCount())
	}
}

// Argentum's ghost, from Declares.bas: a dead character is redrawn as body
// iCuerpoMuerto with head iCabezaMuerto, weapon, shield and helmet cleared,
// facing south (General.bas does all five together). Body 8 really is the
// corpse in the original index — its Cuerpos.ini record has Walk2 and Walk4 at
// zero, because a corpse has no sideways walk frames — which is also why the
// live spawn pool in world.go stops at 7.
const (
	ghostBody = 8   // iCuerpoMuerto
	ghostHead = 500 // iCabezaMuerto
)

// kill is the one place a player stops being alive, whether a sword, a spell or
// the black potion did it. All three used to inline the same four steps and had
// already drifted apart — the potion path forgot to scatter until after the
// bottle was consumed, and none of them turned the body into a ghost.
//
// killer may be nil: the black potion has no one to credit.
func (w *World) kill(victim, killer *Player) {
	if victim.Dead {
		return
	}
	// Before the flag goes up, so aliveCount still counts the victim: last of
	// five is 5th, not 4th. See match.go.
	w.eliminate(victim)
	victim.Dead = true
	victim.Vitals.HP = 0
	// Zero unless a respawn delay was configured, in which case the ghost is
	// on a clock. See respawn.go.
	if w.respawnDelayTicks > 0 {
		victim.respawnAt = w.tick + w.respawnDelayTicks
	}

	// The ghost stays on the map but stops blocking, so a corpse cannot wall
	// off a doorway for the rest of the match. It moves from the collision
	// index to the corpse one, which is what keeps it findable by tile for the
	// snapshot — see viewportOf.
	delete(w.occupied, tileKey{victim.X, victim.Y})
	w.addCorpse(victim)
	victim.Body, victim.Head = ghostBody, ghostHead
	victim.Heading = protocol.South

	// A ghost is not hidden, paralyzed or buffed — death clears the lot rather
	// than leaving a corpse that is still counting down someone's Celeridad.
	victim.revealHidden()
	victim.ParalyzedUntil, victim.ImmobilizedUntil = 0, 0
	victim.AgilityDelta, victim.AgilityUntil = 0, 0
	victim.StrengthDelta, victim.StrengthUntil = 0, 0

	w.scatterInventory(victim)

	if killer != nil && killer.ID != victim.ID {
		killer.Kills++
	}
}

// reportCombat tells both sides what happened, each from their own side of it.
func (w *World) reportCombat(attacker, victim *Player, result AttackResult) {
	event := protocol.CombatEvent{
		AttackerID:   uint32(attacker.ID),
		AttackerName: attacker.Name,
		VictimID:     uint32(victim.ID),
		VictimName:   victim.Name,
		Hit:          result.Hit,
		Blocked:      result.Blocked,
		Damage:       result.Damage,
		Killed:       result.Killed,
	}

	event.Mine = true
	w.sendTo(attacker, protocol.TypeCombat, event)
	event.Mine = false
	w.sendTo(victim, protocol.TypeCombat, event)
}

// aliveCount is how many players are still in the match.
func (w *World) aliveCount() int {
	alive := 0
	for _, p := range w.players {
		if !p.Dead {
			alive++
		}
	}
	return alive
}

// randRange returns an integer in [low, high], matching RandomNumber.
func (w *World) randRange(low, high int) int {
	if high <= low {
		return low
	}
	return low + w.rng.Intn(high-low+1)
}
