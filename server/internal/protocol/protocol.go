// Package protocol defines the wire messages exchanged between the game server
// and its clients.
//
// The codec is deliberately pluggable and JSON is the default: during bring-up
// the traffic stays readable in browser devtools and Wireshark, which matters a
// lot more than bytes on the wire at this stage. Swapping in a binary codec
// later touches only this package.
package protocol

import (
	"encoding/json"
	"fmt"
)

// MsgType tags every frame so the receiver knows how to decode the payload.
type MsgType string

const (
	// Client -> server.
	TypeJoin MsgType = "join"
	TypeMove MsgType = "move"
	TypePing MsgType = "ping"

	// Server -> client.
	TypeWelcome  MsgType = "welcome"
	TypeSnapshot MsgType = "snapshot"
	TypePong     MsgType = "pong"
	TypeError    MsgType = "error"
)

// Heading is a facing direction on the tile grid. Classic Argentum Online only
// ever moves along the four cardinal directions, never diagonally.
type Heading uint8

const (
	North Heading = iota
	East
	South
	West
)

// Valid reports whether h is one of the four cardinal directions. Client input
// is untrusted, so every inbound heading goes through this.
func (h Heading) Valid() bool { return h <= West }

// Delta returns the tile offset produced by stepping once in direction h.
// Y grows downward, matching how the map rows are stored and drawn.
func (h Heading) Delta() (dx, dy int) {
	switch h {
	case North:
		return 0, -1
	case East:
		return 1, 0
	case South:
		return 0, 1
	case West:
		return -1, 0
	}
	return 0, 0
}

// Envelope wraps every frame: a type tag plus the still-encoded payload.
type Envelope struct {
	Type MsgType         `json:"t"`
	Data json.RawMessage `json:"d,omitempty"`
}

// Join is the handshake frame; it must be the first thing a client sends.
type Join struct {
	Name string `json:"name"`
}

// Move asks to step one tile in the given direction. The server rate limits it,
// so a client that spams this simply gets its extra requests ignored.
type Move struct {
	Dir Heading `json:"dir"`
}

// Ping carries a client-chosen timestamp that Pong echoes back, letting the
// client measure round-trip time without the server keeping any clock state.
type Ping struct {
	T int64 `json:"t"`
}

// Pong echoes a Ping's timestamp.
type Pong struct {
	T int64 `json:"t"`
}

// Welcome is the first frame the server sends. It carries everything the client
// needs to draw the world, including the collision map, which never changes
// during a match and so is sent exactly once.
type Welcome struct {
	EntityID  uint32 `json:"id"`
	TickRate  int    `json:"tickRate"`
	MapWidth  int    `json:"w"`
	MapHeight int    `json:"h"`
	// Blocked is a base64 row-major bitset, one bit per tile, LSB first.
	Blocked string `json:"blocked"`
	ViewW   int    `json:"vw"`
	ViewH   int    `json:"vh"`
	SpawnX  int    `json:"sx"`
	SpawnY  int    `json:"sy"`
}

// Vitals are a player's own numbers, sent only to that player — nobody else's
// HP belongs in your snapshot, and leaking it would be an aimbot's best friend.
//
// Combat does not exist yet, so these currently only ever hold their starting
// values. The field is here now so the HUD is driven by the server from the
// start rather than by placeholders it would later have to unlearn.
type Vitals struct {
	Level      int `json:"lvl"`
	HP         int `json:"hp"`
	MaxHP      int `json:"maxHp"`
	Mana       int `json:"mana"`
	MaxMana    int `json:"maxMana"`
	Stamina    int `json:"sta"`
	MaxStamina int `json:"maxSta"`
}

// EntityState is one entity as seen from some player's viewport.
//
// Body and Head are Argentum's own asset numbers. Appearance is server-assigned
// so that everyone renders the same character the same way, and so a modified
// client cannot make itself invisible by claiming a body that draws nothing.
type EntityState struct {
	ID      uint32  `json:"id"`
	X       int     `json:"x"`
	Y       int     `json:"y"`
	Heading Heading `json:"h"`
	Body    int     `json:"b"`
	Head    int     `json:"hd"`
	Name    string  `json:"n,omitempty"`
}

// Snapshot is the per-tick view sent to a single player: only the entities
// inside that player's viewport, never the whole world.
type Snapshot struct {
	Tick uint64 `json:"tick"`
	// Alive is the whole-match player count. It is deliberately global rather
	// than viewport-scoped — knowing how many are left is the core battle
	// royale readout, and it reveals no position.
	Alive int     `json:"alive"`
	Self  *Vitals `json:"self,omitempty"`
	// Entities is everyone inside the viewport, including the player itself.
	Entities []EntityState `json:"e"`
}

// Error reports a protocol-level problem before the connection is dropped.
type Error struct {
	Reason string `json:"reason"`
}

// Codec encodes and decodes frames. Keeping this an interface is what makes the
// JSON-now/binary-later swap a one-file change.
type Codec interface {
	Encode(t MsgType, payload any) ([]byte, error)
	DecodeEnvelope(frame []byte) (MsgType, []byte, error)
	DecodePayload(raw []byte, into any) error
}

// JSONCodec is the default Codec.
type JSONCodec struct{}

func (JSONCodec) Encode(t MsgType, payload any) ([]byte, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode %s payload: %w", t, err)
	}
	return json.Marshal(Envelope{Type: t, Data: raw})
}

func (JSONCodec) DecodeEnvelope(frame []byte) (MsgType, []byte, error) {
	var env Envelope
	if err := json.Unmarshal(frame, &env); err != nil {
		return "", nil, fmt.Errorf("decode envelope: %w", err)
	}
	if env.Type == "" {
		return "", nil, fmt.Errorf("decode envelope: missing type")
	}
	return env.Type, env.Data, nil
}

func (JSONCodec) DecodePayload(raw []byte, into any) error {
	if len(raw) == 0 {
		return fmt.Errorf("decode payload: empty")
	}
	return json.Unmarshal(raw, into)
}
