package world

import (
	"sort"

	"juegito/server/internal/protocol"
)

// Salir al campamento al morir.
//
// Until this existed, being eliminated left you standing on the map as a
// ghost: you could still walk around and watch, but you could not play, and
// there was nowhere to go. The lobby was the missing half — with a waiting room
// that exists, death has somewhere to send you, and the queue for the next
// match is the natural place to be while this one finishes.
//
// The exit is deliberately *not* immediate. A player who dies has just been
// handed a result card and is looking at the tile they died on and at whoever
// killed them; yanking the world out from under them in the same frame reads as
// a disconnection, not as an elimination. A few seconds of ghost is the beat
// between the two, and it is also what keeps the death legible to everybody
// else: the corpse is on the map for long enough to be seen.
//
// Only a player who came in through a seat leaves. A direct Join — the load
// bot, and every test in this package — has no waiting room to be sent back to,
// so for them death stays what it was, exactly as returnToLobby already decides
// for the end of a match.

// deathExitDefaultSeconds is how long a corpse stays on the map before its
// player is taken back to the camp.
//
// Five is short enough that nobody sits looking at their own body wondering
// whether the game hung, and long enough to read the card and see who is
// standing over you.
const deathExitDefaultSeconds = 5

// SetDeathExit sets how long an eliminated player stays a ghost before being
// returned to the lobby. Seconds, because the caller is a CLI flag.
//
// Zero leaves them on the map for the rest of the match, which is what this
// server did before the lobby existed and is still the only thing it can do for
// a connection that has no seat.
//
// It must be called before Run, like SetMap: the world goroutine reads it.
func (w *World) SetDeathExit(seconds int) {
	if seconds <= 0 {
		w.deathExitTicks = 0
		return
	}
	w.deathExitTicks = uint64(seconds * w.tickRate)
}

// markForExit puts a freshly killed player on the clock to leave.
//
// Called from kill(), which is the one place a player stops being alive. The
// two conditions are the same ones that make an exit meaningful at all: a
// server running -respawn has not eliminated anybody (the ghost is coming back,
// so there is nothing to leave), and a player with no seat has no camp to
// return to.
func (w *World) markForExit(p *Player) {
	if w.deathExitTicks == 0 || w.respawnDelayTicks > 0 {
		return
	}
	if w.seatOf(p.ID) == nil {
		return
	}
	p.exitAt = w.tick + w.deathExitTicks
}

// exitDue takes every ghost whose timer has run out back to the camp.
//
// Called once per tick from step(), after matchTick and before lobbyTick. Both
// halves of that order are load-bearing. After matchTick, because the match is
// decided by counting who is alive and by handing out the final card, and a
// player who has left the world can be sent neither — the last death has to be
// able to end the match while its victim is still in it. Before lobbyTick,
// because the very next thing that should reach this connection is the lobby
// frame that puts the camp back on their screen.
func (w *World) exitDue() {
	if w.deathExitTicks == 0 {
		return
	}

	// Sorted rather than straight map iteration, the same reason respawnDue
	// sorts: two players leaving on the same tick should not do so in whatever
	// order Go felt like, or the log of a match stops being reproducible.
	var due []EntityID
	for id, p := range w.players {
		if p.Dead && p.exitAt != 0 && w.tick >= p.exitAt {
			due = append(due, id)
		}
	}
	if len(due) == 0 {
		return
	}
	sort.Slice(due, func(i, j int) bool { return due[i] < due[j] })

	for _, id := range due {
		w.exitToLobby(w.players[id])
	}
}

// exitToLobby takes one eliminated player out of the world and gives their seat
// back.
//
// What stays behind is what they dropped: kill() scattered the inventory on the
// tile they fell on before any of this, so the loot is the map's now and leaving
// does not take it with them.
func (w *World) exitToLobby(p *Player) {
	s := w.seatOf(p.ID)
	if s == nil {
		// Their connection went away between the death and the timer. Nothing
		// to return them to, and removePlayer has already been called by Leave.
		p.exitAt = 0
		return
	}

	// The career goes out before the camp is drawn, so the profile the lobby
	// shows on the right already has the match that just ended in it. The
	// record itself was filed at the moment of elimination and is written off
	// the world goroutine — five seconds ago now, against a queue that empties
	// in milliseconds — so what this reads is the career including this match.
	// A stalled disk would leave it one row short until the next refresh, which
	// is a screen that catches up rather than a number that goes wrong.
	w.sendCareer(s)

	w.removePlayer(p.ID)
	s.playing = 0
	// Not re-queued, for the same reason returnToLobby does not: deciding to
	// play again belongs to the person who has just been handed a result.
	s.queued = false

	w.log.Info("eliminado al campamento",
		"seat", s.id, "cuenta", s.account, "nombre", s.name,
		"puesto", p.Placement, "jugando", len(w.players))
}

// seatOf finds the seat a player came in through, or nil for a direct Join.
//
// A scan rather than an index: seats are counted in tens at most, this is
// called on a death and on a match ending, and a second map from entity to seat
// would be a second thing to keep in step with the first.
func (w *World) seatOf(id EntityID) *seat {
	for _, s := range w.lobby.seats {
		if s.playing == id {
			return s
		}
	}
	return nil
}

// sendCareer pushes a fresh account profile to one seat.
//
// The career used to be sent exactly once, at sign-in, which was right when the
// only way back to the camp was to reconnect. Now that a match ends by putting
// everybody back on that screen, a profile from before the match would be
// showing somebody their own history with the last row missing.
func (w *World) sendCareer(s *seat) {
	if w.accounts == nil || s.account == "" {
		return
	}
	profile, err := w.accounts.Profile(s.account)
	if err != nil {
		return
	}
	if frame, err := w.codec.Encode(protocol.TypeAccount, profile); err == nil {
		_ = s.conn.Send(frame)
	}
}
