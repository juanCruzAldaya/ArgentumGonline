package world

import (
	"sort"

	"juegito/server/internal/protocol"
)

// Respawn: a ghost that comes back, which battle royale says it should not.
//
// Elimination is the genre's rule and the rest of the code still assumes it —
// aliveCount, the ghost body, the inventory scattered on death. This is a
// playtest affordance on top of that: with a delay configured, a corpse stands
// around for a few seconds and then re-enters the match in the middle of the
// map with a fresh kit, so testing combat does not mean restarting the client
// after every fight. SetRespawnDelay(0) restores permadeath, and that is the
// default — only the server binary's own flag turns it on.

// SetRespawnDelay sets how long a dead player stays a ghost before coming
// back. Seconds, not ticks, because the caller is a CLI flag; zero (the
// default) means never — death is elimination, as the genre wants.
//
// It must be called before Run, like SetMap: the world goroutine reads it.
func (w *World) SetRespawnDelay(seconds int) {
	if seconds <= 0 {
		w.respawnDelayTicks = 0
		return
	}
	w.respawnDelayTicks = uint64(seconds * w.tickRate)
}

// respawnDue brings back every ghost whose timer has run out. Called once per
// tick, before the snapshot goes out, so a player never sees a frame in which
// they are alive at their old tile.
func (w *World) respawnDue() {
	if w.respawnDelayTicks == 0 {
		return
	}

	// Sorted rather than straight map iteration: two players coming back on
	// the same tick would otherwise take the centre tile in whatever order Go
	// felt like, and a world that is deterministic everywhere else should not
	// have a coin flip here.
	var due []EntityID
	for id, p := range w.players {
		if p.Dead && p.respawnAt != 0 && w.tick >= p.respawnAt {
			due = append(due, id)
		}
	}
	if len(due) == 0 {
		return
	}
	sort.Slice(due, func(i, j int) bool { return due[i] < due[j] })

	for _, id := range due {
		w.respawn(w.players[id])
	}
}

// respawn puts a ghost back in the match as if they had just joined: middle of
// the map, full vitals, a fresh starting kit, and a body that is no longer a
// corpse. What it deliberately keeps is the Kills counter — the score is
// theirs, and a test session is more useful if it survives dying.
func (w *World) respawn(p *Player) {
	x, y := w.centreSpawn()
	w.revive(p, x, y)

	w.sendTo(p, protocol.TypeLoadout, protocol.Loadout{
		Inventory: p.Inventory,
		Spells:    p.Spells,
	})

	w.log.Info("player respawned", "id", p.ID, "name", p.Name, "pos", [2]int{x, y}, "alive", w.aliveCount())
}

// revive puts a player back on their feet at a given tile, alive and kitted.
//
// Split out of respawn because a match restart needs exactly the same work on
// a player who never died: fresh vitals, fresh bag, a body that is not a
// corpse, and every cooldown cleared. The caller supplies the tile because the
// two want different ones — a respawn goes to the middle of the map so a ghost
// comes back somewhere findable, while a new match scatters everybody the way
// a join does.
//
// The caller owns the old tile: revive claims the new one but does not release
// what the player was standing on, since after a death that has already been
// freed and after a restart it has not.
func (w *World) revive(p *Player, x, y int) {
	// Out of the corpse index first, while p still holds the tile it fell on —
	// after the assignment below there is no way left to find the entry.
	w.removeCorpse(p)

	p.Dead = false
	p.diedAt = 0
	p.respawnAt = 0
	p.X, p.Y = x, y
	p.Heading = protocol.South
	w.occupied[tileKey{x, y}] = p.ID

	// kill() overwrote both with the corpse's, so they have to be reassigned
	// rather than restored — same roll a join does, which is exactly the "as
	// if they logged in again" this is meant to feel like.
	p.Body = availableBodies[w.rng.Intn(len(availableBodies))]
	p.Head = availableHeads[w.rng.Intn(len(availableHeads))]

	p.Vitals = vitalsFor(p.Class, p.Race)
	p.Inventory = w.startingInventoryFor(p.Class, p.Race)
	p.Spells = spellBook(p.Class)

	// Every cooldown and every leftover state, cleared. A ghost cannot act, so
	// none of these can be mid-flight — but coming back with someone else's
	// leftover meditation or a step still on cooldown is the kind of thing
	// that only shows up as "the respawn feels wrong" three sessions later.
	p.Meditating = false
	p.meditateStartTick = 0
	p.lastAttackTick, p.lastCastTick, p.lastUseTick, p.lastHideTick = 0, 0, 0, 0
	p.moveReadyAt, p.turnReadyAt = 0, 0

	// kill() already clears these on the way out, so a ghost arrives here
	// clean. A player who never died does not: a match restart has to take
	// away the Celeridad they were still running on, or the new match starts
	// with somebody buffed by the old one.
	p.revealHidden()
	p.ParalyzedUntil, p.ImmobilizedUntil = 0, 0
	p.AgilityDelta, p.AgilityUntil = 0, 0
	p.StrengthDelta, p.StrengthUntil = 0, 0
}

// centreSpawn is the free tile nearest the middle of the map, searched in
// growing rings. Argentum's own spawn is a random walkable tile (freeSpawn),
// which is right for a match start and wrong here: coming back should put you
// somewhere findable, not scattered across a hundred tiles of Ullathorpe.
func (w *World) centreSpawn() (int, int) {
	cx, cy := w.grid.W/2, w.grid.H/2
	if w.tileFree(cx, cy) {
		return cx, cy
	}

	maxRadius := max(w.grid.W, w.grid.H)
	for r := 1; r <= maxRadius; r++ {
		for dy := -r; dy <= r; dy++ {
			for dx := -r; dx <= r; dx++ {
				// Only the ring itself: the inside was covered by smaller r.
				if abs(dx) != r && abs(dy) != r {
					continue
				}
				x, y := cx+dx, cy+dy
				if x < 0 || y < 0 || x >= w.grid.W || y >= w.grid.H {
					continue
				}
				if w.tileFree(x, y) {
					return x, y
				}
			}
		}
	}
	// The centre of a map with no free tile anywhere is a map bug, not a
	// runtime condition — same fallback freeSpawn takes.
	w.log.Error("no free spawn tile near the centre, falling back to a random one")
	return w.freeSpawn()
}

// abs, because Go's builtin min/max never grew an integer sibling.
func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
