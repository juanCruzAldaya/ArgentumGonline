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
	// so this is terminal for the match. A dead player keeps playing in the
	// only sense the original allows: as a ghost. See kill().
	Dead bool

	// Kills is how many players this one has eliminated. Argentum has no such
	// counter — it has experience, which this game deleted along with levelling
	// — so this is the battle royale's own replacement for "how am I doing",
	// and it is what the click-to-inspect line reports.
	Kills int

	// Status effects. Each "Until" is an absolute world tick: the effect is
	// active exactly while w.tick < the field. Zero means never applied, which
	// is also naturally "not active" since tick 0 has already passed by the
	// time anyone can be affected. See status.go.
	ParalyzedUntil   uint64
	ImmobilizedUntil uint64
	InvisibleUntil   uint64
	// HiddenBySkill distinguishes Ocultarse from the Invisibilidad spell —
	// both set InvisibleUntil and look identical to everyone else (SetInvisible
	// in the source is the same call either way), but only Ocultarse breaks on
	// movement. See movePlayer and hide() in status.go.
	HiddenBySkill bool

	// AgilityDelta/StrengthDelta are temporary attribute modifiers from Fuerza,
	// Celeridad and their debuff counterparts. A new cast overwrites rather
	// than stacks, matching the source.
	AgilityDelta  int
	AgilityUntil  uint64
	StrengthDelta int
	StrengthUntil uint64

	lastAttackTick uint64
	lastCastTick   uint64
	lastUseTick    uint64
	lastHideTick   uint64

	// Vitals are this player's own numbers, sent back only to them.
	Vitals protocol.Vitals

	// Inventory and Spells are also private to this player.
	Inventory []protocol.InventorySlot
	Spells    []int

	conn transport.Conn

	// moveReadyAt is the milltick this player may next step on; see
	// moveCooldownMilliticks for why the clock is finer than a tick.
	moveReadyAt uint64

	// consecutiveDrops counts snapshots the client failed to accept in a row.
	// A client that is merely stuttering recovers; one that never drains gets
	// disconnected rather than being allowed to lag forever.
	consecutiveDrops int
}
