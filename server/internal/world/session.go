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

	first, err := conn.Recv()
	if err != nil {
		return
	}
	typ, payload, err := w.codec.DecodeEnvelope(first)
	if err != nil || typ != protocol.TypeJoin {
		w.sendError(conn, "first message must be a join")
		return
	}
	var join protocol.Join
	if err := w.codec.DecodePayload(payload, &join); err != nil {
		w.sendError(conn, "malformed join")
		return
	}

	id, err := w.Join(sanitizeName(join.Name), Class(join.Class), Race(join.Race), conn)
	if err != nil {
		return
	}
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
