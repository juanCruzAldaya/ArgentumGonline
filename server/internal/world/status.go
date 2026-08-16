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
)

func (p *Player) paralyzed(tick uint64) bool   { return tick < p.ParalyzedUntil }
func (p *Player) immobilized(tick uint64) bool { return tick < p.ImmobilizedUntil }
func (p *Player) invisible(tick uint64) bool   { return tick < p.InvisibleUntil }

// canMove is false under either paralysis or immobilize: both root you in
// place. They differ in whether you can still act — see canAct.
func (p *Player) canMove(tick uint64) bool {
	return !p.paralyzed(tick) && !p.immobilized(tick)
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
