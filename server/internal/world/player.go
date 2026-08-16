package world

import (
	"juegito/server/internal/protocol"
	"juegito/server/internal/transport"
)

// EntityID identifies one entity for the lifetime of a match. IDs are never
// reused, so a stale reference on a client resolves to nothing rather than to
// the wrong player.
type EntityID uint32

// Player is one connected client's presence in the world.
//
// Every field is owned by the world goroutine and must only be touched from
// there — conn is the single exception, since it is safe for concurrent use.
type Player struct {
	ID      EntityID
	Name    string
	X, Y    int
	Heading protocol.Heading

	// Body and Head are Argentum asset numbers, assigned at join.
	Body int
	Head int

	// Class, Race, Attributes and Skills are what combat reads. See balance.go.
	Class      Class
	Race       Race
	Attributes Attributes
	Skills     Skills

	// Dead means eliminated. Argentum revives you; a battle royale does not,
	// so this is terminal for the match.
	Dead bool

	lastAttackTick uint64

	// Vitals are this player's own numbers, sent back only to them.
	Vitals protocol.Vitals

	// Inventory and Spells are also private to this player.
	Inventory []protocol.InventorySlot
	Spells    []int

	conn transport.Conn

	// lastMoveTick gates walking speed; see moveCooldownTicks.
	lastMoveTick uint64

	// consecutiveDrops counts snapshots the client failed to accept in a row.
	// A client that is merely stuttering recovers; one that never drains gets
	// disconnected rather than being allowed to lag forever.
	consecutiveDrops int
}
