package world

import (
	"sort"

	"juegito/server/internal/protocol"
)

// The match: a beginning, an end, and somebody who won it.
//
// Until this existed the simulation was a world with a wall in it. The zone
// closed, held at its final size, and the last two players stood in a circle
// with nothing to decide between them — the genre's one rule, last one
// standing, was the single thing the server did not know about.
//
// The state is deliberately small and lives on the world goroutine like
// everything else, so "is the match over" is a field read rather than three
// scans of the player map from three different places.

type matchPhase int

const (
	// matchIdle is a world nobody has joined yet: the zone is armed but
	// stopped, and there is nothing to win.
	matchIdle matchPhase = iota
	matchRunning
	matchOver
)

// matchMinPlayers is how many have to have been in a match at once before it
// can be won.
//
// Without it, one person connecting to a test server is instantly the last one
// alive and the match ends before the client has drawn a frame. Two is the
// smallest number for which "last one standing" means anything, and it leaves
// walking a world alone to look at it working exactly as it did.
const matchMinPlayers = 2

type match struct {
	phase     matchPhase
	startedAt uint64
	endedAt   uint64

	// peak is the most players this match has held at once. Placement is
	// reported against it rather than against who is connected now, because
	// "5th of 40" is the sentence a player wants, and len(players) has already
	// shrunk by the time anybody reads it.
	peak int

	winnerID   EntityID
	winnerName string

	// restartTicks is how long a decided match sits before the next one
	// begins; 0 leaves it standing.
	restartTicks uint64
	restartAt    uint64
}

// SetMatchRestart sets how long the world waits after a match is decided
// before starting the next one. Seconds, because the caller is a CLI flag.
//
// Zero leaves the finished match standing, which is what a server hosting a
// single match should do — the restart exists so a session can chain matches
// without everyone reconnecting, not because a match ending is a problem to
// recover from.
//
// It must be called before Run, like SetMap: the world goroutine reads it.
func (w *World) SetMatchRestart(seconds int) {
	if seconds <= 0 {
		w.match.restartTicks = 0
		return
	}
	w.match.restartTicks = uint64(seconds * w.tickRate)
}

// startMatchIfIdle begins the match the first time somebody is there to play
// it — the same rule the zone follows, and for the same reason: a server that
// booted an hour before anyone connected should not have run an hour of match.
func (w *World) startMatchIfIdle() {
	if w.match.phase != matchIdle {
		return
	}
	w.match.phase = matchRunning
	w.match.startedAt = w.tick
	w.log.Info("partida empezada", "tick", w.tick)
}

// decidable reports whether this match is one that can have a last player
// standing at all.
//
// Two things stop it. A match that never held two people cannot be won by one
// of them. And -respawn means a death is not an elimination — the ghost that
// just left the count is coming back — so a server running that flag has no
// last player standing to find, which is exactly what asking for respawn in a
// battle royale means. Getting this wrong ended the match on the first kill of
// every playtest.
func (w *World) decidable() bool {
	return w.respawnDelayTicks == 0 && w.match.peak >= matchMinPlayers
}

// matchTick decides the match and, when asked to, starts the next one.
func (w *World) matchTick() {
	switch w.match.phase {
	case matchRunning:
		if !w.decidable() || w.aliveCount() > 1 {
			return
		}
		w.endMatch(w.lastAlive())
	case matchOver:
		if w.match.restartTicks == 0 || w.tick < w.match.restartAt {
			return
		}
		w.resetMatch()
	}
}

// lastAlive is the only player still standing, or nil when that is not a
// number that describes the situation — nobody left, which the ring can do to
// the last two inside one second, or more than one.
func (w *World) lastAlive() *Player {
	var found *Player
	for _, p := range w.players {
		if p.Dead {
			continue
		}
		if found != nil {
			return nil
		}
		found = p
	}
	return found
}

// endMatch settles the match and tells everyone still connected how it went.
func (w *World) endMatch(winner *Player) {
	w.match.phase = matchOver
	w.match.endedAt = w.tick
	w.match.restartAt = w.tick + w.match.restartTicks

	name := ""
	if winner != nil {
		w.match.winnerID = winner.ID
		w.match.winnerName = winner.Name
		name = winner.Name
		// The winner never died, so nothing has given them a placement yet.
		winner.Placement = 1
	}

	// Everyone, not only the winner: the players already eliminated got their
	// own card when they died and are still watching, and who won is the half
	// of it they could not have known then.
	for _, p := range w.playersInOrder() {
		w.sendTo(p, protocol.TypeOutcome, w.outcomeFor(p))
	}

	w.log.Info("partida terminada",
		"ganador", name, "jugadores", w.match.peak,
		"duracion_s", (w.tick-w.match.startedAt)/uint64(w.tickRate))
}

// eliminate records where a player finished and tells them straight away.
//
// Placement is decided at the moment of death and never revisited: with five
// people alive, the fifth is the one who just died. Working it out at the end
// instead would need a history nobody keeps, and would leave a player staring
// at their own corpse with no idea how they had done.
//
// Called from kill() before the victim is marked dead, so aliveCount still
// counts them.
func (w *World) eliminate(p *Player) {
	p.diedAt = w.tick
	p.Placement = w.aliveCount()
	// No card for a death that is not an elimination: with -respawn on, this
	// player is standing up again in a few seconds, and an end-of-match screen
	// over every skirmish is the opposite of what that flag is for.
	if w.match.phase != matchRunning || !w.decidable() {
		return
	}
	w.sendTo(p, protocol.TypeOutcome, w.outcomeFor(p))
}

// outcomeFor is one player's own view of how the match went.
func (w *World) outcomeFor(p *Player) protocol.Outcome {
	players := w.match.peak
	if players < 1 {
		players = 1
	}
	place := p.Placement
	if place < 1 {
		// Still standing when the match was called: joint first with whoever
		// else survived, which only happens when the ring took the last two
		// together and there is no winner at all.
		place = 1
	}
	return protocol.Outcome{
		Placement: place,
		Players:   players,
		Kills:     p.Kills,
		Seconds:   w.secondsSurvived(p),
		Won:       w.match.winnerID != 0 && p.ID == w.match.winnerID,
		Winner:    w.match.winnerName,
	}
}

// secondsSurvived is how long a player lasted: from joining to dying, or to
// now if they are still standing.
//
// Measured from their own join rather than from the match's start, because
// somebody who connected nine minutes in did not survive nine minutes.
func (w *World) secondsSurvived(p *Player) float64 {
	until := w.tick
	if p.diedAt > 0 {
		until = p.diedAt
	}
	if until < p.joinedAt {
		return 0
	}
	return float64(until-p.joinedAt) / float64(w.tickRate)
}

// resetMatch starts a fresh match without restarting the process: same world,
// same connections, everything else back to where a match begins.
//
// Chaining matches used to mean restarting the server and having everybody
// reconnect, which is enough friction that a second match rarely happened — so
// the thing most worth playtesting, a match from start to finish, was the thing
// least often played.
func (w *World) resetMatch() {
	w.match = match{
		phase:        matchRunning,
		startedAt:    w.tick,
		peak:         len(w.players),
		restartTicks: w.match.restartTicks,
	}

	// The floor is rebuilt, not topped up: leaving the last match's leftovers
	// would compound with every restart until the map was carpeted in potions.
	clear(w.ground)
	w.scatterMatchLoot()

	// The ring goes first, and the order is load-bearing: freeSpawn draws
	// inside the safe circle, so reviving anybody before the zone is back at
	// its opening radius would pack the whole match into the last one's final
	// arena instead of spreading it over the map.
	if w.zone.armed {
		w.startZone()
	}

	// Every tile is released before any is claimed, so the second player placed
	// cannot be denied a tile the first one is about to leave.
	players := w.playersInOrder()
	for _, p := range players {
		delete(w.occupied, tileKey{p.X, p.Y})
	}
	for _, p := range players {
		x, y := w.freeSpawn()
		w.revive(p, x, y)
		p.Kills = 0
		p.Placement = 0
		p.diedAt = 0
		p.joinedAt = w.tick

		// A fresh Welcome rather than a message of its own: a new match puts
		// the player somewhere else on the map, and the spawn tile is what
		// stops the client predicting from the old one. It is the same message
		// a join sends, so the client resets through the path it already has
		// instead of through a second one that could drift from it.
		w.sendWelcome(p)
		w.sendTo(p, protocol.TypeLoadout, protocol.Loadout{
			Inventory: p.Inventory,
			Spells:    p.Spells,
		})
	}

	w.log.Info("partida reiniciada", "jugadores", len(w.players))
}

// playersInOrder is every player, sorted by id.
//
// Go randomises map iteration, and a world that is deterministic everywhere
// else should not decide who gets the better spawn — or who hears the match
// ended first — on a coin flip.
func (w *World) playersInOrder() []*Player {
	out := make([]*Player, 0, len(w.players))
	for _, p := range w.players {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
