package world

import (
	"strings"
	"unicode"

	"juegito/server/internal/protocol"
	"juegito/server/internal/transport"
)

const maxNameLen = 16

// HandleConn drives one client connection for its whole lifetime: it performs
// the join handshake, then pumps inbound frames into the world until the peer
// disconnects. It runs on the connection's own goroutine and touches no world
// state directly.
func (w *World) HandleConn(conn transport.Conn) {
	defer conn.Close()

	// The server speaks first, once, so the client knows which handshake it is
	// in before it draws anything.
	w.sendHello(conn)

	name, ok := w.signIn(conn)
	if !ok {
		return
	}

	join, ok := w.awaitJoin(conn)
	if !ok {
		return
	}
	// With accounts on, the name is the one the server authenticated, never the
	// one in the join — otherwise the record is of whoever the client felt like
	// claiming to be that match.
	if name == "" {
		name = sanitizeName(join.Name)
	}

	id, err := w.Join(name, Class(join.Class), Race(join.Race), conn)
	if err != nil {
		return
	}
	w.setAccount(id, name)
	defer w.Leave(id)

	for {
		frame, err := conn.Recv()
		if err != nil {
			return
		}
		typ, payload, err := w.codec.DecodeEnvelope(frame)
		if err != nil {
			continue
		}

		switch typ {
		case protocol.TypePing:
			// Answered right here rather than queued: a pong needs no world
			// state, so making it wait for a tick would only add jitter to the
			// latency number the client is trying to measure.
			var ping protocol.Ping
			if w.codec.DecodePayload(payload, &ping) == nil {
				if pong, err := w.codec.Encode(protocol.TypePong, protocol.Pong{T: ping.T}); err == nil {
					_ = conn.Send(pong)
				}
			}
		default:
			w.Submit(id, typ, payload)
		}
	}
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

// awaitJoin waits for the join that says which character to play.
func (w *World) awaitJoin(conn transport.Conn) (protocol.Join, bool) {
	for {
		frame, err := conn.Recv()
		if err != nil {
			return protocol.Join{}, false
		}
		typ, payload, err := w.codec.DecodeEnvelope(frame)
		if err != nil || typ != protocol.TypeJoin {
			w.sendError(conn, "falta el join con clase y raza")
			continue
		}
		var join protocol.Join
		if err := w.codec.DecodePayload(payload, &join); err != nil {
			w.sendError(conn, "join ilegible")
			continue
		}
		return join, true
	}
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
