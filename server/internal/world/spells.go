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

// castCooldownTicks paces casting. At 20 Hz this is one spell per second.
const castCooldownTicks = 20

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

// knowsSpell reports whether the player has this spell in their book. The
// client shows only what it was told, but a modified one can name anything.
func knowsSpell(p *Player, id int) bool {
	for _, known := range p.Spells {
		if known == id {
			return true
		}
	}
	return false
}

// cast resolves one spell against one target.
//
// Unlike melee, casting does take a target id — Argentum spells reach across
// the screen. So the server re-checks everything the client could lie about:
// that the spell is known, that the victim exists and is close enough to see,
// and that the caster can actually pay for it.
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

	spell, ok := w.spells[spellID]
	if !ok || !knowsSpell(caster, spellID) {
		w.spellFailed(caster, "No conocés ese hechizo.")
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
	if !w.withinViewport(caster, victim) {
		w.spellFailed(caster, "Está demasiado lejos.")
		return
	}
	if caster.Vitals.Mana < spell.Mana {
		w.spellFailed(caster, "No tenés suficiente maná.")
		return
	}
	if caster.Vitals.Stamina < spell.Stamina {
		w.spellFailed(caster, "No tenés suficiente energía.")
		return
	}

	caster.lastCastTick = w.tick
	caster.Vitals.Mana -= spell.Mana
	caster.Vitals.Stamina -= spell.Stamina

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
		victim.Vitals.HP = 0
		victim.Dead = true
		event.Killed = true
		delete(w.occupied, tileKey{victim.X, victim.Y})
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
