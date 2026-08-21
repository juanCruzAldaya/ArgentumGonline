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
// ~40ms AI pulse (General.bas's EfectoParalisisUser takes a DeltaTick of
// elapsed pulses, not ticks of our simulation). IntervaloParalizado=500 pulses
// and DuracionEfecto=1200/700 pulses work out to roughly 20s of paralysis and
// 48s/28s of buff/debuff — but that arithmetic rests on an inferred pulse
// rate, not a constant grepped straight out of the source, so treat it as a
// well-reasoned estimate rather than a verified figure.
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
	if w.tick < p.AgilityUntil {
		a.Agilidad += p.AgilityDelta
	}
	if w.tick < p.StrengthUntil {
		a.Fuerza += p.StrengthDelta
	}
	return a
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

// applyAgility/applyStrength roll and store a new buff or debuff, overwriting
// whatever was already running rather than stacking with it — casting Fuerza
// twice in a row does not double up, exactly as in the source.
//
// A buff is capped at the character's own base attribute (so the effective
// total tops out at double); a debuff is capped so it can never floor the
// attribute below 1.
// applyAgility rolls spell.MinAgility..MaxAgility and stores it as a buff
// (attributeBuff) or debuff (attributeDebuff) on p.AgilityDelta.
func (w *World) applyAgility(p *Player, spell Spell, effect int) (delta int) {
	roll := w.randRange(spell.MinAgility, spell.MaxAgility)
	delta = clampAttributeDelta(effect, roll, p.Attributes.Agilidad)
	p.AgilityDelta = delta
	p.AgilityUntil = w.tick + durationFor(effect)
	return delta
}

func (w *World) applyStrength(p *Player, spell Spell, effect int) (delta int) {
	roll := w.randRange(spell.MinStrength, spell.MaxStrength)
	delta = clampAttributeDelta(effect, roll, p.Attributes.Fuerza)
	p.StrengthDelta = delta
	p.StrengthUntil = w.tick + durationFor(effect)
	return delta
}

// drinkAgility and drinkStrength apply a green or yellow potion. That is not
// the same operation as casting the equivalent spell, even though both land on
// the same field, and treating it as the same one is what capped everybody
// short of their race's ceiling.
//
// The source's potion ADDS to whatever the character is carrying right now,
// and tops out at double the base attribute (InvUsuario.bas):
//
//	.Stats.UserAtributos(Agilidad) = MinimoInt(
//	    .Stats.UserAtributos(Agilidad) + RandomNumber(obj.MinModificador, obj.MaxModificador),
//	    .Stats.UserAtributosBackUP(Agilidad) * 2)
//
// A spell overwrites instead — casting Fuerza twice does not stack — and both
// paths used to go through applyAgility. So every potion after the first threw
// away the one before it, and the real ceiling was one roll above base rather
// than double it: a human topped out at 27 strength (21 base + a 6 roll) when
// the rule allows 42.
func (w *World) drinkAgility(p *Player, minMod, maxMod int) (gained int) {
	before := p.AgilityDelta
	p.AgilityDelta = w.accumulateBuff(before, p.AgilityUntil, w.randRange(minMod, maxMod), p.Attributes.Agilidad)
	p.AgilityUntil = w.tick + buffDurationTicks
	return p.AgilityDelta - before
}

func (w *World) drinkStrength(p *Player, minMod, maxMod int) (gained int) {
	before := p.StrengthDelta
	p.StrengthDelta = w.accumulateBuff(before, p.StrengthUntil, w.randRange(minMod, maxMod), p.Attributes.Fuerza)
	p.StrengthUntil = w.tick + buffDurationTicks
	return p.StrengthDelta - before
}

// accumulateBuff adds a fresh roll onto a buff that is still running, bounded
// so the effective attribute never passes double its base.
//
// A buff that has already expired counts as zero rather than as its last
// value: the character is back at base by then, and carrying the old delta
// forward would let somebody stack up to the ceiling, let it lapse, and then
// jump straight back to the top on a single potion.
func (w *World) accumulateBuff(delta int, until uint64, roll, base int) int {
	if w.tick >= until {
		delta = 0
	}
	if delta+roll > base {
		return base
	}
	return delta + roll
}

func durationFor(effect int) uint64 {
	if effect == attributeBuff {
		return buffDurationTicks
	}
	return debuffDurationTicks
}

// clampAttributeDelta bounds a rolled buff/debuff against the target's own
// base value: a buff cannot push the effective total past double the base, a
// debuff cannot push it below 1.
func clampAttributeDelta(effect, roll, base int) int {
	if effect == attributeBuff {
		if roll > base {
			return base
		}
		return roll
	}
	// debuff
	if roll >= base {
		return -(base - 1)
	}
	return -roll
}
