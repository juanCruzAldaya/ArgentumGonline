package world

// Argentum encodes every stat-affecting spell the same way: a "Sube<X>" switch
// where 1 raises the stat and 2 lowers it. AffectsHP reuses this as
// effectHeals/effectDamages; attribute buffs reuse the identical 1/2 encoding
// under their own names since "heals" would be a confusing word for a
// strength buff.
const (
	attributeBuff   = 1
	attributeDebuff = 2
)

// Status effect durations.
//
// Argentum stores these as a counter decremented by elapsed real time on a
// 40ms pulse, not by ticks of our simulation. The pulse is no longer inferred:
// mMainLoop.bas declares GAME_TIMER_INTERVAL = 40 and computes
// DeltaTick = (GetTickCount - LastGameTick) / GAME_TIMER_INTERVAL, so a
// "duration" is milliseconds/40. That puts the originals at:
//
//	IntervaloParalizado 500  ->  20s of paralysis
//	DuracionEfecto     1200  ->  48s of buff
//	DuracionEfecto      700  ->  28s of debuff
//	obj.dat's potions  1000  ->  40s
//
// One more thing the source does that this does not: those all share a single
// flags.DuracionEfecto per character, and when it lapses General.bas restores
// every attribute from backup at once. So in Argentum refreshing strength also
// extends agility. Here the two carry separate deadlines, which is a
// simplification and not a port.
//
// More importantly: those durations were tuned for an MMO where a fight is one
// event in a session that runs for hours. A 20-second stun or a 48-second buff
// is a much bigger fraction of a 15-minute battle royale match. These are
// deliberately shorter than the original for that reason — retune freely,
// there is nothing sacred about the numbers below.
const (
	// paralysisDurationTicks covers both Paralizar and Inmovilizar, which in
	// the source share one counter and are mutually exclusive.
	paralysisDurationTicks    = 6 * 20  // 6s at the server's 20Hz tick rate
	invisibilityDurationTicks = 12 * 20 // 12s: an escape tool, not a stun
	buffDurationTicks         = 30 * 20 // 30s
	debuffDurationTicks       = 20 * 20 // 20s

	// hideCooldownTicks paces Ocultarse. There is no mana cost gating it the
	// way there is for the spell, so this is what stops it being spammed.
	hideCooldownTicks = 3 * 20 // 3s
)

func (p *Player) paralyzed(tick uint64) bool   { return tick < p.ParalyzedUntil }
func (p *Player) immobilized(tick uint64) bool { return tick < p.ImmobilizedUntil }
func (p *Player) invisible(tick uint64) bool   { return tick < p.InvisibleUntil }

// canMove is false under either paralysis or immobilize: both root you in
// place. They differ in whether you can still act — see canAct.
func (p *Player) canMove(tick uint64) bool {
	return !p.paralyzed(tick) && !p.immobilized(tick)
}

// revealHidden clears invisibility regardless of which source set it. Used
// wherever an action breaks stealth — see hide, and the reveal-on-attack /
// reveal-on-cast calls in combat.go and spells.go.
func (p *Player) revealHidden() {
	p.InvisibleUntil = 0
	p.HiddenBySkill = false
}

// hide is Ocultarse: DoOcultarse in the source rolls a success chance against
// the Ocultarse skill, which this project does not model as its own stat —
// every character already spawns at the skill cap everything else uses (see
// balance.go's maxLevel), so a roll against a skill that is always maxed has
// only one outcome, and it is simpler to just always succeed once the
// cooldown and "not already hidden" gate (the source's own gate, so a fresh
// Ocultarse can't refresh a spell-granted invisibility into a skill-based one)
// clear.
//
// Any class may attempt this — the source's class check lives in the move
// handler (only Ladron/Bandido keep walking while hidden), not here.
//
// Blocked by paralysis: this one is a design call rather than a sourced fact
// — DoOcultarse's own gate list wasn't pinned down against Paralizado — but
// it keeps the rule consistent with everything else physical (move, melee):
// paralysis stops your body, not your mind, so it blocks this the same way
// it blocks a step or a swing. Immobilize does not: crouching in place needs
// no footwork.
func (w *World) hide(p *Player) {
	if p.Dead || p.paralyzed(w.tick) || p.invisible(w.tick) {
		return
	}
	if w.tick-p.lastHideTick < hideCooldownTicks {
		return
	}
	p.lastHideTick = w.tick
	p.InvisibleUntil = w.tick + invisibilityDurationTicks
	p.HiddenBySkill = true
}

// canAct governs melee: Argentum's HandleWalk rejects movement outright while
// Paralizado, but PuedeLanzar never checks it, so a paralyzed caster can still
// throw spells — the tension of "stunned but not helpless" is the point of the
// spell. Immobilize is weaker still: only your feet are rooted, so melee is
// unaffected by it. Casting is therefore never blocked by either status; only
// melee reads this.
func (p *Player) canAct(tick uint64) bool {
	return !p.paralyzed(tick)
}

// effectiveAttributes applies any live buff/debuff on top of the character's
// base attributes. Combat formulas read this instead of Attributes directly.
func (w *World) effectiveAttributes(p *Player) Attributes {
	a := p.Attributes
	a.Agilidad += liveDelta(w.tick, p.AgilityDelta, p.AgilityUntil)
	a.Fuerza += liveDelta(w.tick, p.StrengthDelta, p.StrengthUntil)
	return a
}

// liveDelta is the modifier actually in force right now. A buff or debuff whose
// timer has run out counts as zero even though the field still holds its last
// value — nothing clears it on expiry, the deadline is what decides.
//
// Every caller that stacks onto an existing modifier, and every caller that
// reports how much one changed, has to start from this rather than from the raw
// field. Reading the field directly is what made a potion taken after the
// previous buff lapsed report a *negative* delta: the sip raised agility from
// 21 to 26 and the client announced "tu agilidad ha disminuido", because the
// change was measured against a bonus the character no longer had.
func liveDelta(tick uint64, delta int, until uint64) int {
	if tick >= until {
		return 0
	}
	return delta
}

// applyParalysis and applyImmobilize mirror the source: casting either clears
// the other, since a character is never both at once.
func (p *Player) applyParalysis(untilTick uint64) {
	p.ParalyzedUntil = untilTick
	p.ImmobilizedUntil = 0
}

func (p *Player) applyImmobilize(untilTick uint64) {
	p.ImmobilizedUntil = untilTick
	p.ParalyzedUntil = 0
}

func (p *Player) removeParalysis() {
	p.ParalyzedUntil = 0
	p.ImmobilizedUntil = 0
}

// applyAgility/applyStrength are Celeridad/Torpeza and Fuerza/Debilidad. They
// roll spell.Min..Max and add it onto whatever modifier is already running —
// they do not replace it.
//
// This used to overwrite, with a comment claiming that was what the source did.
// It is not. modHechizos.bas adds, in the same shape the potion does:
//
//	.Stats.UserAtributos(Agilidad) = .Stats.UserAtributos(Agilidad) + dano
//
// A spell and a potion are one operation in Argentum, which is why casting
// Celeridad repeatedly is how you climb to the ceiling rather than a way to
// re-roll a single +5. Overwriting capped every buff at one roll above base no
// matter how many you cast — the same bug the potions had before they stopped
// sharing this path, arrived at from the opposite direction.
//
// Refreshing also resets the deadline, so a debuff landing on a buffed target
// shortens what is left of the buff to the debuff's own (shorter) duration.
// That falls out of the source's single shared timer rather than being aimed
// at, and it is the one piece of that design that survives here — juegito
// keeps agility and strength on separate deadlines, where Argentum runs both
// off one flags.DuracionEfecto and restores every attribute together when it
// lapses.
func (w *World) applyAgility(p *Player, spell Spell, effect int) (gained int) {
	roll := signedRoll(w.randRange(spell.MinAgility, spell.MaxAgility), effect)
	return w.addAgility(p, roll, durationFor(effect))
}

func (w *World) applyStrength(p *Player, spell Spell, effect int) (gained int) {
	roll := signedRoll(w.randRange(spell.MinStrength, spell.MaxStrength), effect)
	return w.addStrength(p, roll, durationFor(effect))
}

// addAgility/addStrength are the one operation every buff and debuff goes
// through, spell or potion. They return how much THIS application moved the
// attribute, which is not the same as the total modifier now standing — the
// client shows it as "tu agilidad ha aumentado" next to the character.
func (w *World) addAgility(p *Player, roll int, duration uint64) (gained int) {
	before := liveDelta(w.tick, p.AgilityDelta, p.AgilityUntil)
	p.AgilityDelta = clampAttributeDelta(before+roll, p.Attributes.Agilidad)
	p.AgilityUntil = w.tick + duration
	return p.AgilityDelta - before
}

func (w *World) addStrength(p *Player, roll int, duration uint64) (gained int) {
	before := liveDelta(w.tick, p.StrengthDelta, p.StrengthUntil)
	p.StrengthDelta = clampAttributeDelta(before+roll, p.Attributes.Fuerza)
	p.StrengthUntil = w.tick + duration
	return p.StrengthDelta - before
}

func signedRoll(roll, effect int) int {
	if effect == attributeDebuff {
		return -roll
	}
	return roll
}

// drinkAgility and drinkStrength apply a green or yellow potion — the same
// accumulate-and-clamp the spells go through, which is what the source does
// too (InvUsuario.bas):
//
//	.Stats.UserAtributos(Agilidad) = MinimoInt(
//	    .Stats.UserAtributos(Agilidad) + RandomNumber(obj.MinModificador, obj.MaxModificador),
//	    .Stats.UserAtributosBackUP(Agilidad) * 2)
//
// A potion is always a buff — there is no potion that lowers an attribute —
// so the roll goes in positive and the duration is the buff's.
func (w *World) drinkAgility(p *Player, minMod, maxMod int) (gained int) {
	return w.addAgility(p, w.randRange(minMod, maxMod), buffDurationTicks)
}

func (w *World) drinkStrength(p *Player, minMod, maxMod int) (gained int) {
	return w.addStrength(p, w.randRange(minMod, maxMod), buffDurationTicks)
}

func durationFor(effect int) uint64 {
	if effect == attributeBuff {
		return buffDurationTicks
	}
	return debuffDurationTicks
}

// clampAttributeDelta bounds an accumulated modifier against the target's own
// base value: it cannot push the effective total past double the base, nor
// floor the attribute below 1.
//
// Argentum clamps the same two ends with global constants instead —
// MinimoInt(MAXATRIBUTOS, base*2) and MINATRIBUTOS, which are 40 and 6
// (Declares.bas:422,424). Those are deliberately not ported: every character
// here spawns at the cap, so a flat 40 would bind on some races and not
// others, and a floor of 6 would make Torpeza and Debilidad much gentler than
// the rest of the balance assumes.
func clampAttributeDelta(delta, base int) int {
	if delta > base {
		return base
	}
	if delta < -(base - 1) {
		return -(base - 1)
	}
	return delta
}
