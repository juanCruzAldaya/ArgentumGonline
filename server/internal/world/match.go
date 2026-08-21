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
	// matchLobby is a world with no match under way: the zone is armed but
	// stopped, and whoever is connected is waiting in the queue rather than
	// standing on the map. It is the phase a server boots into and the one it
	// comes back to after every match.
	matchLobby matchPhase = iota
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
	if w.match.phase != matchLobby {
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
		// A match with nobody in it ends, decidable or not.
		//
		// decidable() answers "can this match have a last player standing?",
		// and for a match that never held two people the answer is no — but it
		// does not follow that the match should keep running, empty, forever.
		// That is what it did: one person entering alone, playing, and closing
		// their client left the server saying "partida en curso, 0 jugando"
		// and refusing everybody who came afterwards, with no way out short of
		// a restart. There is no winner to announce and nobody to send a card
		// to, so this goes straight back to the lobby instead of through
		// endMatch.
		if len(w.players) == 0 {
			w.log.Info("partida vacía, vuelve el lobby", "tick", w.tick)
			w.match.phase = matchLobby
			w.lobbyCheck()
			return
		}
		if !w.decidable() || w.aliveCount() > 1 {
			return
		}
		w.endMatch(w.lastAlive())
	case matchOver:
		if w.match.restartTicks == 0 || w.tick < w.match.restartAt {
			return
		}
		// Anybody who arrived through a seat goes back to the lobby and has to
		// queue again for the next match. Whoever is left after that came in
		// through a direct Join — the load bot, and every test in this package
		// — and gets the in-place restart that predates the lobby, because
		// there is no waiting room for them to be sent back to.
		w.returnToLobby()
		if len(w.players) == 0 {
			w.match.phase = matchLobby
			w.lobbyCheck()
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
		out := w.outcomeFor(p)
		w.sendTo(p, protocol.TypeOutcome, out)
		// Only the survivor is filed here. Everybody else was filed the moment
		// they were eliminated, and recording them again would give each of
		// them two rows for one match.
		if winner != nil && p.ID == winner.ID {
			w.recordOutcome(p, out)
		}
	}

	// And everybody who was eliminated and has already gone back to the camp,
	// which since deathexit.go is most of the field. They got the card with
	// their placement on it when they died; this is the same card with the
	// winner filled in, which is the half they could not have known then.
	for _, s := range w.seatsInOrder() {
		if !s.carded || s.playing != 0 {
			continue
		}
		card := s.card
		card.Winner = name
		if frame, err := w.codec.Encode(protocol.TypeOutcome, card); err == nil {
			_ = s.conn.Send(frame)
		}
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
	out := w.outcomeFor(p)
	w.sendTo(p, protocol.TypeOutcome, out)
	// Kept on the seat as well, so the half of the card that only exists once
	// the match is decided can still reach somebody who has since walked back
	// to the camp. See endMatch.
	if s := w.seatOf(p.ID); s != nil {
		s.card, s.carded = out, true
	}
	// Filed here rather than at the end of the match: the placement is already
	// final, and a player who closes the client on their own corpse still
	// played the match.
	w.recordOutcome(p, out)
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
