package world

import (
	"strings"
	"time"
	"unicode"

	"juegito/server/internal/protocol"
	"juegito/server/internal/transport"
)

const maxNameLen = 16

const (
	// pingEvery is the gap between the server's own pings. Two seconds is
	// cheap — a ping is a couple of dozen bytes against the 16 KB/s a player
	// already costs in snapshots — and still fills a reporting window with
	// enough samples for a p95 to mean something.
	pingEvery = 2 * time.Second

	// latencyEvery is how often the summary reaches the log. Per sample would
	// be one line every two seconds per player, which buries every other line
	// in the log; per window it is one line a player a half minute, and the
	// shape of a connection going bad still shows up in the next one.
	latencyEvery = 30 * time.Second
)

// HandleConn drives one client connection for its whole lifetime: it performs
// the join handshake, then pumps inbound frames into the world until the peer
// disconnects. It runs on the connection's own goroutine and touches no world
// state directly.
func (w *World) HandleConn(conn transport.Conn) {
	defer conn.Close()

	// The server speaks first, once, so the client knows which handshake it is
	// in before it draws anything.
	w.sendHello(conn)

	account, ok := w.signIn(conn)
	if !ok {
		return
	}

	// The lobby is what you land on the moment you are through the door, before
	// there is a character to play — signing in and arriving at the camp are the
	// same step. The seat exists from here on, and the Join that names a
	// character comes later, from the lobby screen, on the way into the queue.
	s, err := w.EnterLobby(account, account, conn)
	if err != nil {
		return
	}
	var id EntityID
	defer w.LeaveLobby(s.id)
	defer func() {
		if id != 0 {
			w.Leave(id)
		}
	}()

	// Reading moves to its own goroutine so this one can wait on two things at
	// once: the next frame, and the match starting. A connection sitting in the
	// queue still has to be able to say it is leaving, and a blocking Recv
	// cannot hear the world at the same time.
	frames := w.readFrames(conn)

	// Latency is measured from here, on the connection's own goroutine, rather
	// than from the world loop: a ping that queued behind a tick would be
	// timing the tick as much as the network. See latency.go for why the
	// server pings the client and not the other way round.
	pings := time.NewTicker(pingEvery)
	defer pings.Stop()
	reports := time.NewTicker(latencyEvery)
	defer reports.Stop()

	var lat latencyTracker
	who := account
	if who == "" {
		// No accounts on this server, so there is no name to file this under
		// until a character exists. The address is what tells two anonymous
		// connections apart.
		who = conn.RemoteAddr()
	}
	logLatency := func() {
		p50, p95, max, got, lost := lat.summary(time.Now())
		if got == 0 {
			return
		}
		w.log.Info("latencia", "cuenta", who,
			"p50_ms", p50, "p95_ms", p95, "max_ms", max,
			"muestras", got, "sin_respuesta", lost)
	}
	// Reported on the way out too, so a connection that drops before its first
	// window still leaves its numbers behind. A player who quits because it
	// played badly is exactly the one whose latency is worth having.
	defer logLatency()

	for {
		select {
		case <-w.done:
			return

		case <-pings.C:
			// A timestamp rather than a sequence number, so this side keeps no
			// state at all: it travels to the client, comes back untouched,
			// and the subtraction happens against the clock that wrote it.
			//
			// Send drops the frame instead of blocking when a client is not
			// draining, which is why the summary counts what came back against
			// what went out — a lost ping is not a fast one.
			now := time.Now()
			if frame, err := w.codec.Encode(protocol.TypePing, protocol.Ping{
				T: now.UnixMilli(),
			}); err == nil {
				_ = conn.Send(frame)
				lat.ping(now)
			}

		case <-reports.C:
			logLatency()

		case started, ok := <-s.started:
			if !ok {
				return
			}
			// A new match, and with it a new entity: the seat is the thing that
			// persists across matches, the player is not.
			id = started

		case frame, ok := <-frames:
			if !ok {
				return
			}
			typ, payload, err := w.codec.DecodeEnvelope(frame)
			if err != nil {
				continue
			}

			switch typ {
			case protocol.TypePing:
				// Answered right here rather than queued: a pong needs no world
				// state, so making it wait for a tick would only add jitter to
				// the latency number the client is trying to measure.
				var ping protocol.Ping
				if w.codec.DecodePayload(payload, &ping) == nil {
					if pong, err := w.codec.Encode(protocol.TypePong, protocol.Pong{T: ping.T}); err == nil {
						_ = conn.Send(pong)
					}
				}
			case protocol.TypePong:
				// The other direction of the same exchange, and the two do not
				// collide: a client measuring itself sends ping and reads pong,
				// this server sends ping and reads pong, and each side only
				// ever subtracts a timestamp it wrote.
				//
				// An old client never sends this, so it simply never appears in
				// the log — there is nothing to fall back to and nothing that
				// breaks.
				var pong protocol.Pong
				if w.codec.DecodePayload(payload, &pong) == nil {
					lat.sample(time.Now().UnixMilli() - pong.T)
				}

			case protocol.TypeJoin:
				// Picking a character is what commits you to the queue. With
				// accounts on, the name in it is ignored: the record has to be
				// of who signed in, not of whoever the client felt like
				// claiming to be this match.
				var join protocol.Join
				if w.codec.DecodePayload(payload, &join) != nil {
					continue
				}
				if entity := w.SeatCharacter(
					s.id, sanitizeName(join.Name), Class(join.Class), Race(join.Race),
				); entity != 0 {
					id = entity
				}
			case protocol.TypeQueue:
				var q protocol.Queue
				if w.codec.DecodePayload(payload, &q) == nil {
					w.SetQueued(s.id, q.Join)
				}
			default:
				// Dropped while waiting rather than buffered: a command aimed
				// at a player who does not exist yet has no tick to be applied
				// on, and holding it would apply it to whoever this seat
				// becomes two matches later.
				if id != 0 {
					w.Submit(id, typ, payload)
				}
			}
		}
	}
}

// readFrames pumps one connection into a channel, so a caller can select on it
// alongside anything else. The channel closes when the peer goes away.
func (w *World) readFrames(conn transport.Conn) <-chan []byte {
	frames := make(chan []byte, 32)
	go func() {
		defer close(frames)
		for {
			frame, err := conn.Recv()
			if err != nil {
				return
			}
			select {
			case frames <- frame:
			case <-w.done:
				return
			}
		}
	}()
	return frames
}

// sendHello announces what this server wants. Best effort: a client that never
// reads it and goes straight to a join still works on a server without
// accounts, which is what every bot does.
func (w *World) sendHello(conn transport.Conn) {
	hello := protocol.Hello{Accounts: w.accounts != nil}
	if hello.Accounts {
		hello.MinPassword = MinPasswordLen
	}
	if frame, err := w.codec.Encode(protocol.TypeHello, hello); err == nil {
		_ = conn.Send(frame)
	}
}

// signIn runs the account half of the handshake, and returns the authenticated
// name — or the empty string when this server has no accounts configured, which
// leaves the old behaviour of trusting the name in the join.
//
// A failed attempt answers with an error and waits for another one instead of
// dropping the connection. A wrong password is the single most common thing
// that will happen on this path, and making it cost a reconnect — with the map
// and the collision bitset downloaded again — would be punishing the typo, not
// the attacker.
func (w *World) signIn(conn transport.Conn) (string, bool) {
	if w.accounts == nil {
		return "", true
	}

	for {
		frame, err := conn.Recv()
		if err != nil {
			return "", false
		}
		typ, payload, err := w.codec.DecodeEnvelope(frame)
		if err != nil || typ != protocol.TypeLogin {
			w.sendError(conn, "este servidor pide cuenta: mandá un login")
			continue
		}

		var login protocol.Login
		if err := w.codec.DecodePayload(payload, &login); err != nil {
			w.sendError(conn, "login ilegible")
			continue
		}

		name, err := w.authenticate(login)
		if err != nil {
			w.sendError(conn, err.Error())
			continue
		}

		// The career goes out before the player picks a character, because it
		// is the thing the account screen is there to show.
		profile, err := w.accounts.Profile(name)
		if err == nil {
			if frame, err := w.codec.Encode(protocol.TypeAccount, profile); err == nil {
				_ = conn.Send(frame)
			}
		}
		return name, true
	}
}

// authenticate is sign-in or sign-up, told apart by the flag the client set.
//
// The email only means anything on the sign-up branch: signing in is answered
// by the name and the password, so an address sent alongside one is ignored
// rather than checked against what is stored. Nothing here would be improved by
// making somebody retype their address to get back in.
func (w *World) authenticate(login protocol.Login) (string, error) {
	if login.Register {
		if err := w.accounts.Register(login.Name, login.Email, login.Password); err != nil {
			return "", err
		}
	}
	return w.accounts.Authenticate(login.Name, login.Password)
}

func (w *World) sendError(conn transport.Conn, reason string) {
	if frame, err := w.codec.Encode(protocol.TypeError, protocol.Error{Reason: reason}); err == nil {
		_ = conn.Send(frame)
	}
}

// sanitizeName keeps display names short and printable so a hostile client
// cannot smuggle control characters into everyone else's screen.
func sanitizeName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.Map(func(r rune) rune {
		if unicode.IsPrint(r) {
			return r
		}
		return -1
	}, name)

	if runes := []rune(name); len(runes) > maxNameLen {
		name = string(runes[:maxNameLen])
	}
	if name == "" {
		return "anon"
	}
	return name
}
