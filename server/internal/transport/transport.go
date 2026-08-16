// Package transport carries opaque frames between the server and its clients.
//
// Nothing in here knows what a frame means. That separation is the point: the
// vertical slice runs on WebSocket because it crosses NAT and corporate
// firewalls without ceremony, and moving to raw UDP later means adding a Conn
// implementation rather than touching the game.
package transport

import "errors"

// ErrBackpressure is returned by Send when the peer's queue is full. It is not
// a fatal error on its own — the caller decides whether to drop the frame or
// drop the client.
var ErrBackpressure = errors.New("transport: send queue full")

// Conn is one client connection.
//
// Send must never block: the world tick calls it once per player per tick, and
// a single stalled peer would otherwise drag down the whole simulation.
type Conn interface {
	Send(frame []byte) error
	Recv() ([]byte, error)
	Close() error
	RemoteAddr() string
}

// Handler runs one connection for its entire lifetime. It is called on its own
// goroutine and the connection is closed when it returns.
type Handler func(Conn)
