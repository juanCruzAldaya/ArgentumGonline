package world

import (
	"encoding/json"
	"fmt"
	"os"

	"juegito/server/internal/protocol"
)

// Spell effect switch, as Argentum encodes it in Hechizos.dat: 1 raises the
// stat, 2 lowers it.
const (
	effectHeals   = 1
	effectDamages = 2
)

// Spell targets, from the comment block at the top of Hechizos.dat.
const (
	targetUser    = 1
	targetNPC     = 2
	targetBoth    = 3
	targetTerrain = 4
)

// The four intervals Argentum paces a fight with, straight out of the original
// Server.ini's [INTERVALOS] block. They are in milliseconds there and in ticks
// here; at 20 Hz a tick is 50 ms.
//
// The two crossed ones are the interesting half. Casting does not only delay
// the next spell, it delays your next swing, and swinging delays your next
// spell — which is what stops magic and melee from being two independent
// buttons mashed at once, and is the reason AO's PvP has the rhythm it does.
// modNuevoTimer.bas is where the original enforces them.
const (
	// IntervaloLanzaHechizo, 1400 ms.
	castCooldownTicks = 28
	// IntervaloMagiaGolpe, 1000 ms: cast, then wait before you can hit.
	castToAttackTicks = 20
	// IntervaloGolpeMagia, 1000 ms: hit, then wait before you can cast.
	attackToCastTicks = 20
)

// Spell is one Hechizos.dat entry, as converted by tools/aoconv.
type Spell struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Words    string `json:"words"`
	Type     int    `json:"type"`
	Target   int    `json:"target"`
	MinSkill int    `json:"minSkill"`
	Mana     int    `json:"mana"`
	Stamina  int    `json:"sta"`

	AffectsHP int `json:"affectsHp"`
	MinHP     int `json:"minHp"`
	MaxHP     int `json:"maxHp"`

	// AffectsAgility/AffectsStrength: 1 buffs, 2 debuffs.
	AffectsAgility  int `json:"affectsAg"`
	MinAgility      int `json:"minAg"`
	MaxAgility      int `json:"maxAg"`
	AffectsStrength int `json:"affectsFu"`
	MinStrength     int `json:"minFu"`
	MaxStrength     int `json:"maxFu"`

	Paralyzes        bool `json:"paralyzes"`
	Immobilizes      bool `json:"immobilizes"`
	RemovesParalysis bool `json:"removesParalysis"`
	Invisibility     bool `json:"invisibility"`
}

// LoadSpells reads the converted Hechizos.dat.
func LoadSpells(path string) (map[int]Spell, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var byID map[int]Spell
	if err := json.Unmarshal(raw, &byID); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return byID, nil
}

// SetSpells installs the spell table. Call before Run.
func (w *World) SetSpells(spells map[int]Spell) { w.spells = spells }

// SpellSlots is how many positions the spell book holds. Argentum's own spell
// window is a fixed list you arrange yourself, gaps and all, rather than a
// list that closes up behind whatever you happen to know — so Spells is a
// fixed-length slice with 0 meaning "empty slot", and the order in it is the
// player's, not the server's.
const SpellSlots = 30

// knowsSpell reports whether the player has this spell in their book. The
// client shows only what it was told, but a modified one can name anything.
func knowsSpell(p *Player, id int) bool {
	if id == 0 {
		return false // the empty-slot marker is not a castable spell
	}
	for _, known := range p.Spells {
		if known == id {
			return true
		}
	}
	return false
}

// swapSpells is the spell window's drag-and-drop, the counterpart to
// swapSlots. The book is a fixed set of positions, so unlike the bag there is
// no "landing on nothing" case to special-case: every index in range holds
// something, even if that something is the 0 that means empty, and swapping
// two of them is the whole operation.
//
// Both indices are bounds-checked against the server's own SpellSlots rather
// than trusted, for the same reason every other client-named index is.
func (w *World) swapSpells(p *Player, from, to int) {
	if p.Dead || from == to {
		return
	}
	if from < 0 || from >= len(p.Spells) || to < 0 || to >= len(p.Spells) {
		return
	}
	p.Spells[from], p.Spells[to] = p.Spells[to], p.Spells[from]
	w.sendLoadout(p)
}

// cast resolves one spell against one target.
//
// Unlike melee, casting does take a target id — Argentum spells reach across
// the screen. So the server re-checks everything the client could lie about:
// that the spell is known, that the victim exists and is close enough to see,
// and that the caster can actually pay for it.
// offensive says whether a spell is something done TO somebody rather than FOR
// them, which is what the self-target rule turns on: casting on yourself stays
// legal, because healing and buffing yourself is most of what a support spell
// is for.
//
// Damage, paralysis and immobilisation are hostile outright. Agility and
// strength count only when they debuff, since those same two fields carry the
// buffs a player puts on themselves on purpose.
func offensive(s Spell) bool {
	return s.AffectsHP == effectDamages ||
		s.Paralyzes || s.Immobilizes ||
		s.AffectsAgility == attributeDebuff ||
		s.AffectsStrength == attributeDebuff
}

func (w *World) cast(caster *Player, spellID int, targetID EntityID) {
	// Paralysis does not stop casting in the source — only movement and melee
	// do — so only death gates it here.
	if caster.Dead {
		w.spellFailed(caster, "Estás muerto.")
		return
	}
	if w.tick-caster.lastCastTick < castCooldownTicks {
		return
	}
	if w.tick-caster.lastAttackTick < attackToCastTicks {
		return
	}

	spell, ok := w.spells[spellID]
	if !ok || !knowsSpell(caster, spellID) {
		w.spellFailed(caster, "No conocés ese hechizo.")
		return
	}
	// PuedeLanzar's actual gate: not enough Magia skill for this spell yet.
	if caster.Skills.Magia < spell.MinSkill {
		w.spellFailed(caster, "No tenés suficientes puntos de magia para lanzar este hechizo.")
		return
	}
	if spell.Target != targetUser && spell.Target != targetBoth {
		w.spellFailed(caster, "Ese hechizo no se lanza sobre personas.")
		return
	}

	victim, ok := w.players[targetID]
	if !ok || victim.Dead {
		w.spellFailed(caster, "No hay nadie ahí.")
		return
	}
	// An invisible target is simply not there as far as anyone else's client
	// is concerned, and the check is repeated here rather than trusted to the
	// client: a modified one could still send an id it inferred some other way.
	if victim.ID != caster.ID && victim.invisible(w.tick) {
		w.spellFailed(caster, "No hay nadie ahí.")
		return
	}
	// Nobody attacks themselves. Casting on yourself stays legal — healing and
	// buffing yourself is most of what a support spell is for — so this gates
	// on what the spell does rather than on who it is aimed at.
	//
	// It sits before the mana is spent, so a misclick costs nothing.
	if victim.ID == caster.ID && offensive(spell) {
		w.spellFailed(caster, "No podés atacarte a vos mismo.")
		return
	}
	if !w.withinViewport(caster, victim) {
		w.spellFailed(caster, "Está demasiado lejos.")
		return
	}
	if caster.Vitals.Mana < spell.Mana {
		w.spellFailed(caster, "No tenés suficiente maná.")
		return
	}
	// Stamina is deliberately not a gate and deliberately not spent. Argentum
	// meters energy because it is an MMO where you fight for hours; this game
	// is a battle royale where a match is minutes, and an energy floor there
	// only ever decides a fight by who ran out rather than who played better —
	// the same reasoning that made potions effectively unlimited (see
	// startingKit). Spell.Stamina is still parsed from Hechizos.dat, so
	// turning the cost back on is deleting this comment and subtracting it.
	caster.lastCastTick = w.tick
	caster.Vitals.Mana -= spell.Mana

	// Casting reveals whoever was hidden, spell or skill alike — including a
	// caster who is about to re-hide themselves with Invisibilidad below,
	// which nets out as a no-op rather than a bug.
	if caster.invisible(w.tick) {
		caster.revealHidden()
	}

	// The incantation, over the caster's head, for everyone who can see the
	// tile — DecirPalabrasMagicas in modHechizos.bas. This is the tell: it
	// fires before any of the spell's effects are worked out, so it lands even
	// when the cast goes on to fail, and it carries from an invisible caster
	// as readily as from a visible one.
	if spell.Words != "" {
		w.broadcastSpeech(caster, spell.Words, true)
	}

	event := protocol.SpellEvent{
		CasterID:   uint32(caster.ID),
		CasterName: caster.Name,
		VictimID:   uint32(victim.ID),
		VictimName: victim.Name,
		SpellID:    spellID,
		SpellName:  spell.Name,
		Words:      spell.Words,
	}

	// A spell can carry more than one effect at once in the data (Dardo
	// Mágico Paralizante style combos exist in later AO versions), so these
	// are independent ifs rather than a switch on any single field.
	switch spell.AffectsHP {
	case effectDamages:
		w.applySpellDamage(caster, victim, spell, &event)
	case effectHeals:
		w.applySpellHeal(caster, victim, spell, &event)
	}

	if spell.Paralyzes {
		victim.applyParalysis(w.tick + paralysisDurationTicks)
		event.Paralyzed = true
	} else if spell.Immobilizes {
		victim.applyImmobilize(w.tick + paralysisDurationTicks)
		event.Immobilized = true
	}
	if spell.RemovesParalysis {
		victim.removeParalysis()
		event.RemovedParalysis = true
	}
	if spell.Invisibility {
		victim.InvisibleUntil = w.tick + invisibilityDurationTicks
		// Explicitly not HiddenBySkill: the spell form does not break on
		// movement the way Ocultarse does. Without clearing this, a target
		// still carrying an old Ocultarse flag from earlier in the match
		// would incorrectly keep the movement-breaks-it rule.
		victim.HiddenBySkill = false
		event.MadeInvisible = true
	}
	if spell.AffectsAgility != 0 {
		event.AgilityDelta = w.applyAgility(victim, spell, spell.AffectsAgility)
	}
	if spell.AffectsStrength != 0 {
		event.StrengthDelta = w.applyStrength(victim, spell, spell.AffectsStrength)
	}

	event.Mine = true
	w.sendTo(caster, protocol.TypeSpell, event)
	if victim.ID != caster.ID {
		event.Mine = false
		w.sendTo(victim, protocol.TypeSpell, event)
	}
}

// applySpellDamage is CalcularDano's magic branch: dano + Porcentaje(dano,
// 3*ELV) — every caster level adds three percent.
func (w *World) applySpellDamage(caster, victim *Player, spell Spell, event *protocol.SpellEvent) {
	roll := float64(w.randRange(spell.MinHP, spell.MaxHP))
	damage := max(int(roll*(1+0.03*float64(caster.Vitals.Level))), 1)

	victim.Vitals.HP -= damage
	event.Damage = damage
	if victim.Vitals.HP <= 0 {
		event.Killed = true
		w.kill(victim, caster)
		w.log.Info("player killed by spell",
			"victim", victim.Name, "by", caster.Name, "spell", spell.Name, "alive", w.aliveCount())
	}
}

func (w *World) applySpellHeal(caster, victim *Player, spell Spell, event *protocol.SpellEvent) {
	roll := float64(w.randRange(spell.MinHP, spell.MaxHP))
	healed := max(int(roll*(1+0.03*float64(caster.Vitals.Level))), 1)
	if room := victim.Vitals.MaxHP - victim.Vitals.HP; healed > room {
		healed = room
	}
	victim.Vitals.HP += healed
	event.Healed = healed
}

// withinViewport keeps spell range to what the caster can actually see, which
// is the same window the snapshots use. Casting at someone off screen would
// mean acting on information the server never sent.
func (w *World) withinViewport(caster, victim *Player) bool {
	const halfW, halfH = ViewportW / 2, ViewportH / 2
	dx, dy := victim.X-caster.X, victim.Y-caster.Y
	return dx >= -halfW && dx <= halfW && dy >= -halfH && dy <= halfH
}

func (w *World) spellFailed(p *Player, reason string) {
	w.sendTo(p, protocol.TypeSpell, protocol.SpellEvent{
		CasterID: uint32(p.ID),
		Mine:     true,
		Failed:   reason,
	})
}
