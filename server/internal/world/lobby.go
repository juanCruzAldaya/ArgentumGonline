package world

import (
	"sort"

	"juegito/server/internal/protocol"
	"juegito/server/internal/transport"
)

// The lobby: where a connection waits for a match to have room for it.
//
// Until this existed a match began the moment the first person connected, which
// is the only thing a server can do when "the match" and "the process" are the
// same object. It made the first player's experience the wrong one — you walked
// alone through a world that had already started closing — and it made a full
// match impossible to arrange, because the only way in was to arrive before
// anybody else did.
//
// The important decision here is that a waiting connection is **not a Player**.
// It would have been less code to add a flag to Player and skip it everywhere,
// and that is exactly the shape of bug this project has already paid for once:
// aliveCount counting somebody it should not is what ended every match on its
// first kill (see match.go's decidable). A seat has no tile, no body and no
// vitals, so there is nothing for the simulation to accidentally include.
//
// Like everything else, the lobby is owned by the world goroutine. Connections
// reach it through channels and never touch it directly.

// seatID identifies one waiting connection. It is deliberately a different
// space from EntityID: a seat is not an entity, and a seat that gets let into
// the match is issued an EntityID then, by addPlayer, like any other join.
type seatID uint32

// seat is one connection waiting to play.
//
// A seat exists from the moment somebody signs in, before they have a character
// — that is the whole point of the lobby coming straight after the login. The
// character arrives later, when they commit to queueing, and until it does this
// seat can watch the queue but cannot be counted into a match.
type seat struct {
	id      seatID
	name    string
	account string
	conn    transport.Conn

	// class and race are unset until a Join names them, which is what picking a
	// character on the way into the queue sends.
	class   Class
	race    Race
	hasChar bool

	// queued is whether this seat wants in on the next match. A seat that is
	// not queued still gets lobby updates — it is looking at the screen — but
	// is not counted toward starting anything.
	queued bool

	// started delivers the EntityID once this seat is let into the match.
	// Buffered so the world goroutine can hand it over without ever waiting
	// for a connection goroutine to be listening.
	started chan EntityID

	// playing is the entity this seat became, or 0 while it is only waiting.
	// The seat outlives the match on purpose: it is what the player comes back
	// to when the match ends, which is why the lobby does not simply forget a
	// seat the moment it lets it in.
	playing EntityID

	// card is the result this seat's player was handed when they were
	// eliminated, and carded says there is one.
	//
	// It exists because the two halves of an Outcome are now separated by the
	// player leaving the world: the placement is decided at the moment of
	// death, the winner only when the match is called, and by then the dead
	// have been back at the camp for minutes. Keeping the first half on the
	// seat is what lets endMatch finish the sentence for somebody who is no
	// longer a player. See deathexit.go.
	card   protocol.Outcome
	carded bool
}

// lobbyDefaultMin is how many queued players it takes to start a match.
//
// One, on purpose. It is the number that reproduces the behaviour this server
// had before the lobby existed — connect and play — so every bot, test and
// local run keeps working unchanged, and a real queue is something a host turns
// on with -lobby-min rather than something everybody pays for.
const lobbyDefaultMin = 1

type lobby struct {
	seats    map[seatID]*seat
	nextSeat uint32

	// minPlayers is how many queued seats start a match, and waitTicks is how
	// long the countdown runs once that many are there. The wait exists so the
	// queue filling up is not a starting pistol: somebody who joins one second
	// after the threshold should get into that match, not the next one.
	minPlayers int
	waitTicks  uint64

	// startAt is the tick the countdown fires on, meaningful only while
	// counting. The countdown is cancelled outright if the queue drops back
	// under the threshold, which is why this is not simply a decrementing
	// number: it has to be re-derived from the clock when it restarts.
	startAt  uint64
	counting bool
}

// SetLobby configures the queue. Seconds, because the caller is a CLI flag.
//
// It must be called before Run, like SetMap: the world goroutine reads it.
func (w *World) SetLobby(minPlayers, waitSeconds int) {
	if minPlayers < 1 {
		minPlayers = 1
	}
	w.lobby.minPlayers = minPlayers
	if waitSeconds <= 0 {
		w.lobby.waitTicks = 0
		return
	}
	w.lobby.waitTicks = uint64(waitSeconds * w.tickRate)
}

// EnterLobby seats a connection. It is called as soon as the player is known —
// right after the sign-in, or right after the hello on a server without
// accounts — because the lobby is what you land on when you enter your account,
// not something you reach by way of a character.
//
// The seat starts without a character and out of the queue, so somebody can see
// how many are already waiting before deciding to be one of them.
func (w *World) EnterLobby(name, account string, conn transport.Conn) (*seat, error) {
	resp := make(chan *seat, 1)
	select {
	case w.seatCh <- seatReq{name: name, account: account, conn: conn, resp: resp}:
	case <-w.done:
		return nil, ErrWorldClosed
	}

	select {
	case s := <-resp:
		return s, nil
	case <-w.done:
		return nil, ErrWorldClosed
	}
}

// SeatCharacter names the character this seat will play and takes its place in
// the queue. It is what a Join means now.
//
// It returns the EntityID when the match could be started immediately, which is
// what keeps a one-player lobby indistinguishable from the server that had no
// lobby at all: with -lobby-min 1 the queue is deep enough the instant the
// character lands, so the match starts inside this call and the caller walks
// straight into the world without waiting on a tick. That matters more than it
// looks — a connection whose script ends the moment it has sent its join (which
// is every handshake test in this package) would otherwise be torn down by its
// own EOF before the world's next tick got round to starting anything.
func (w *World) SeatCharacter(id seatID, name string, class Class, race Race) EntityID {
	resp := make(chan *seat, 1)
	select {
	case w.charCh <- charReq{seat: id, name: name, class: class, race: race, resp: resp}:
	case <-w.done:
		return 0
	}

	select {
	case s := <-resp:
		if s == nil {
			return 0
		}
		select {
		case entity := <-s.started:
			return entity
		default:
			return 0
		}
	case <-w.done:
		return 0
	}
}

// LeaveLobby gives up a seat. Safe to call for a seat that is already gone, and
// safe to call for one that has since been let into the match — the seat is
// removed either way, and the player it became is torn down by Leave.
func (w *World) LeaveLobby(id seatID) {
	select {
	case w.unseatCh <- id:
	case <-w.done:
	}
}

// SetQueued steps a seat in or out of the queue.
func (w *World) SetQueued(id seatID, join bool) {
	select {
	case w.queueCh <- queueReq{seat: id, join: join}:
	case <-w.done:
	}
}

type seatReq struct {
	name    string
	account string
	conn    transport.Conn
	resp    chan *seat
}

type charReq struct {
	seat  seatID
	name  string
	class Class
	race  Race
	resp  chan *seat
}

type queueReq struct {
	seat seatID
	join bool
}

// addSeat registers a waiting connection. World goroutine only.
func (w *World) addSeat(req seatReq) *seat {
	w.lobby.nextSeat++
	s := &seat{
		id:      seatID(w.lobby.nextSeat),
		name:    req.name,
		account: req.account,
		conn:    req.conn,
		started: make(chan EntityID, 1),
	}
	w.lobby.seats[s.id] = s

	w.log.Info("asiento en el lobby",
		"seat", s.id, "cuenta", s.account,
		"sentados", len(w.lobby.seats), "addr", req.conn.RemoteAddr())
	return s
}

// seatCharacter records the character and queues the seat. World goroutine only.
func (w *World) seatCharacter(req charReq) *seat {
	s, ok := w.lobby.seats[req.seat]
	if !ok {
		return nil
	}
	// On a server without accounts the name is whatever the join claimed, which
	// is the behaviour that predates the lobby. With an account behind the
	// connection the authenticated name wins and the join cannot touch it.
	if s.account == "" && req.name != "" {
		s.name = req.name
	}
	s.class = req.class
	s.race = req.race
	s.hasChar = true
	s.queued = true

	// Evaluated right here rather than left to the next tick. See SeatCharacter
	// for why that is load-bearing and not just an optimisation.
	w.lobbyCheck()
	return s
}

// removeSeat drops a waiting connection. World goroutine only.
func (w *World) removeSeat(id seatID) {
	s, ok := w.lobby.seats[id]
	if !ok {
		return
	}
	delete(w.lobby.seats, id)
	if s.queued {
		// Somebody leaving can take the queue back under the threshold, which
		// has to stop the countdown rather than let it fire on a lobby that no
		// longer has enough people in it.
		w.lobbyCheck()
	}
}

// setQueued steps a seat in or out of the queue. World goroutine only.
func (w *World) setQueued(req queueReq) {
	s, ok := w.lobby.seats[req.seat]
	if !ok || s.queued == req.join {
		return
	}
	// Nothing to queue without a character. Stepping back in is only ever
	// possible for somebody who already picked one, so this cannot lock
	// anybody out of a queue they were in.
	if req.join && !s.hasChar {
		return
	}
	s.queued = req.join
	w.lobbyCheck()
}

// queuedCount is how many seats want in on the next match.
func (w *World) queuedCount() int {
	n := 0
	for _, s := range w.lobby.seats {
		if s.queued {
			n++
		}
	}
	return n
}

// seatsInOrder is every seat, by id, for the same reason playersInOrder exists:
// Go randomises map iteration and who gets let into a full match should not be
// decided by a coin flip.
func (w *World) seatsInOrder() []*seat {
	out := make([]*seat, 0, len(w.lobby.seats))
	for _, s := range w.lobby.seats {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].id < out[j].id })
	return out
}

// lobbyCheck re-decides whether a match should be starting, and is called from
// everywhere the answer can change: a seat arriving, leaving, or changing its
// mind, and every tick while a countdown runs.
//
// It deliberately does nothing at all while a match is running. A queue that
// filled up mid-match waits for that one to finish; letting people in late
// would drop them into a closed ring with nothing to walk to.
func (w *World) lobbyCheck() {
	if w.match.phase != matchLobby {
		w.lobby.counting = false
		return
	}

	if w.queuedCount() < w.lobby.minPlayers {
		w.lobby.counting = false
		return
	}

	if w.lobby.waitTicks == 0 {
		w.startMatchFromLobby()
		return
	}

	if !w.lobby.counting {
		w.lobby.counting = true
		w.lobby.startAt = w.tick + w.lobby.waitTicks
		w.log.Info("cuenta regresiva del lobby",
			"en_cola", w.queuedCount(), "segundos", w.lobby.waitTicks/uint64(w.tickRate))
		return
	}

	if w.tick >= w.lobby.startAt {
		w.startMatchFromLobby()
	}
}

// startMatchFromLobby lets everybody in the queue into the world at once.
//
// Everybody queued, not everybody seated: somebody looking at the lobby without
// having taken a place in the line is watching, not playing.
func (w *World) startMatchFromLobby() {
	w.lobby.counting = false

	let := make([]*seat, 0, len(w.lobby.seats))
	for _, s := range w.seatsInOrder() {
		if s.queued {
			let = append(let, s)
		}
	}
	if len(let) == 0 {
		return
	}

	// The phase flips before the first addPlayer rather than after the last.
	// addPlayer calls startMatchIfIdle, and leaving the phase alone here would
	// let the first seat through that path and start the match a second time,
	// with a startedAt one player later than the truth.
	w.match.phase = matchRunning
	w.match.startedAt = w.tick
	// Peak counts this match's players, not the last one's. addPlayer raises it
	// as each seat comes in.
	w.match.peak = 0

	// A match starting from the lobby is as new as one starting from a restart,
	// and used to be the only one that did not act like it: resetMatch rebuilds
	// the floor and reopens the ring, this path did neither. So the second
	// match on a server ran on the first one's leftovers — the same loot nobody
	// picked up, and a ring already halfway shut, which is what put players in
	// the second match on the edge of a circle that was closing on them from
	// the first tick, with none of the grace period they were owed.
	//
	// The order is load-bearing, same as in resetMatch: the ring goes first
	// because freeSpawn draws inside the safe circle, so opening it after the
	// players are placed would pack them into the last match's final arena.
	clear(w.ground)
	w.scatterMatchLoot()
	if w.zone.armed {
		w.startZone()
	}

	for _, s := range let {
		id := w.addPlayer(joinReq{name: s.name, class: s.class, race: s.race, conn: s.conn})
		if s.account != "" {
			w.setAccount(id, s.account)
		}
		// Buffered *and* non-blocking. The buffer is what lets the world hand
		// the entity over without waiting for a connection goroutine that has
		// not got round to listening yet; the select is what keeps a full
		// buffer from being the end of the world — literally.
		//
		// The channel holds one, and a seat that never read the id from its
		// last match is a seat whose reader is gone. A plain send there parks
		// the world goroutine forever: the tick stops, and the connection
		// goroutine that would clear it is itself blocked handing its seat
		// back on a channel nobody is reading any more. Found by a test that
		// hung for ten minutes rather than failing, which is exactly the shape
		// of the bug it was pointing at.
		select {
		case s.started <- id:
		default:
			w.log.Warn("el asiento no está escuchando; entra igual pero su cliente no se entera",
				"seat", s.id, "cuenta", s.account, "entidad", id)
		}
		// The seat stays in the map while its player is in the world, so that
		// the end of the match can put them back in the lobby without needing
		// a second index from player to seat.
		s.queued = false
		s.playing = id
		// Last match's result, gone: the card on screen belongs to a match
		// that is over, and endMatch must not finish that sentence twice.
		s.card, s.carded = protocol.Outcome{}, false
	}

	w.log.Info("partida empezada desde el lobby",
		"jugadores", len(let), "tick", w.tick)
}

// lobbyTick advances the countdown and tells everybody waiting where things
// stand.
func (w *World) lobbyTick() {
	if w.lobby.counting {
		w.lobbyCheck()
	}
	w.broadcastLobby()
}

// broadcastLobby sends the waiting room to everybody in it.
//
// Only to seats that are not currently playing: a player in the world is being
// told everything by the snapshot already, and a lobby frame arriving mid-match
// would be a second, contradictory account of what is going on.
func (w *World) broadcastLobby() {
	if len(w.lobby.seats) == 0 {
		return
	}

	state := protocol.LobbyState{
		Queued:  w.queuedCount(),
		Needed:  w.lobby.minPlayers,
		Running: w.match.phase == matchRunning,
		Playing: len(w.players),
	}
	if w.lobby.counting && w.lobby.startAt > w.tick {
		state.Counting = true
		state.Seconds = float64(w.lobby.startAt-w.tick) / float64(w.tickRate)
	} else if w.lobby.counting {
		state.Counting = true
	}

	for _, s := range w.lobby.seats {
		if s.playing != 0 {
			continue
		}
		mine := state
		mine.Mine = s.queued
		if frame, err := w.codec.Encode(protocol.TypeLobby, mine); err == nil {
			_ = s.conn.Send(frame)
		}
	}
}

// returnToLobby takes every player who came in through a seat back out of the
// world, so the next match is queued for rather than inherited.
//
// Players without a seat are left exactly where they are. That is not an
// oversight: a direct Join is the load-testing bot and every test in this
// package, and they have no lobby to be returned to — for them the old
// behaviour, respawning in place for the next match, is the only one that
// means anything.
func (w *World) returnToLobby() int {
	returned := 0
	for _, s := range w.seatsInOrder() {
		if s.playing == 0 {
			continue
		}
		// The camp draws this seat's career on its right-hand column, and the
		// match that just ended belongs in it. Same call the eliminated get on
		// their way out — see sendCareer.
		w.sendCareer(s)
		w.removePlayer(s.playing)
		s.playing = 0
		// Not re-queued: coming back from a match you just lost and being put
		// straight into the line for the next one takes the decision away from
		// the person who has just been handed a result to read.
		s.queued = false
		returned++
	}
	return returned
}
