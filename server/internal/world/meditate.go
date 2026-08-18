package world

// Meditation, F6 in the original: stand still and mana trickles back, ported
// from HandleMeditate and DoMeditar (Protocol.bas, Trabajo.bas).

const (
	// meditateRampTicks is TIEMPO_INICIOMEDITAR: 2s of "concentrating" before
	// regen actually starts, so tapping F6 mid-fight is not a free heal.
	meditateRampTicks = 2 * 20 // 2s at 20Hz

	// meditateTickInterval and meditatePercent stand in for DoMeditar's RNG
	// roll against the Meditar skill (Suerte, 5 to 35 depending on skill) that
	// decides how often a Balance.dat PorcentajeRecuperoMana-sized chunk of
	// mana lands. This project has no Meditar skill to roll against — every
	// character already spawns at the stat cap everything else uses (see
	// balance.go's maxLevel) — so like hide()'s always-succeeds roll, a fixed
	// cadence stands in for a roll that would always land the same way.
	// Retunable; nothing sacred about either number.
	meditateTickInterval = 20 // once a second
	meditatePercent      = 6  // Balance.dat: PorcentajeRecuperoMana
)

// toggleMeditate is HandleMeditate, minus the mount check (no mounts here) and
// the admin instant-refill branch (no privilege levels here).
func (w *World) toggleMeditate(p *Player) {
	if p.Dead || p.Vitals.MaxMana == 0 {
		return
	}
	p.Meditating = !p.Meditating
	if p.Meditating {
		p.meditateStartTick = w.tick
	}
}

// stopMeditating cancels meditation from any of the source's own interrupts:
// HandleWalk moving (movePlayer) or SistemaCombate landing a hit (attack).
func (p *Player) stopMeditating() {
	p.Meditating = false
}

// meditateTick is DoMeditar, run once a world tick for every meditating
// player: wait out the ramp, then grant mana on the fixed cadence above until
// it caps out, which ends meditation the same way the source's "Has terminado
// de meditar" branch does.
func (w *World) meditateTick() {
	for _, p := range w.players {
		if !p.Meditating || p.Dead {
			continue
		}
		elapsed := w.tick - p.meditateStartTick
		if elapsed < meditateRampTicks || elapsed%meditateTickInterval != 0 {
			continue
		}
		if p.Vitals.Mana >= p.Vitals.MaxMana {
			p.Meditating = false
			continue
		}
		gain := p.Vitals.MaxMana * meditatePercent / 100
		if gain <= 0 {
			gain = 1
		}
		p.Vitals.Mana = min(p.Vitals.Mana+gain, p.Vitals.MaxMana)
	}
}
