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
	TypeJoin   MsgType = "join"
	TypeMove   MsgType = "move"
	TypeAttack MsgType = "attack"
	TypeCast   MsgType = "cast"
	TypeUse    MsgType = "use"
	TypeHide   MsgType = "hide"
	TypePickup MsgType = "pickup"
	TypeDrop   MsgType = "drop"
	TypeSwap   MsgType = "swap"
	// TypeSwapSpell reorders the spell book, the same drag-and-drop Argentum
	// gives its own spell list. Separate from TypeSwap because the two lists
	// are separate server state and a bag index is not a spell index.
	TypeSwapSpell MsgType = "swapSpell"
	TypePing      MsgType = "ping"

	// Server -> client.
	TypeWelcome   MsgType = "welcome"
	TypeSnapshot  MsgType = "snapshot"
	TypeLoadout   MsgType = "loadout"
	TypeCombat    MsgType = "combat"
	TypeSpell     MsgType = "spell"
	TypeUseResult MsgType = "useResult"
	TypePong      MsgType = "pong"
	TypeError     MsgType = "error"
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
//
// Class and Race are chosen by the player before connecting — see the client's
// character picker — and validated server-side against the real class/race
// count, so a modified client sending an out-of-range id just gets clamped
// rather than crashing anything.
type Join struct {
	Name  string `json:"name"`
	Class int    `json:"class"`
	Race  int    `json:"race"`
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
	EntityID uint32 `json:"id"`
	TickRate int    `json:"tickRate"`
	// MapNumber is the Argentum map the match runs on, so the client knows which
	// bundled tile data to load. Zero means the generated demo arena.
	MapNumber int    `json:"map"`
	MapName   string `json:"mapName,omitempty"`
	MapWidth  int    `json:"w"`
	MapHeight int    `json:"h"`
	// Blocked is a base64 row-major bitset, one bit per tile, LSB first.
	Blocked string `json:"blocked"`
	ViewW   int    `json:"vw"`
	ViewH   int    `json:"vh"`
	SpawnX  int    `json:"sx"`
	SpawnY  int    `json:"sy"`
	// WalkSpeed is tiles per second, so the client's movement interpolation is
	// driven by the server's real cadence instead of a second copy of it. The
	// client used to hardcode 5.0 to match a 4-tick cooldown; the moment the
	// cooldown became tunable, a hardcoded twin would have the sprite arriving
	// early and stuttering at the tile edge.
	WalkSpeed float64 `json:"walkSpeed"`
	// SpellSlots is how many slots the spell book holds. The client draws that
	// many and the server refuses a reorder outside the range, so the two
	// cannot disagree about the size of the list being dragged around.
	SpellSlots int `json:"spellSlots"`
}

// Vitals are a player's own numbers, sent only to that player — nobody else's
// HP belongs in your snapshot, and leaking it would be an aimbot's best friend.
//
// Combat does not exist yet, so these currently only ever hold their starting
// values. The field is here now so the HUD is driven by the server from the
// start rather than by placeholders it would later have to unlearn.
type Vitals struct {
	Level int `json:"lvl"`
	// Exp and MaxExp drive the level bar. Nothing awards experience yet.
	Exp    int `json:"exp"`
	MaxExp int `json:"maxExp"`

	HP         int `json:"hp"`
	MaxHP      int `json:"maxHp"`
	Mana       int `json:"mana"`
	MaxMana    int `json:"maxMana"`
	Stamina    int `json:"sta"`
	MaxStamina int `json:"maxSta"`
	// Hunger and Thirst are core Argentum vitals and the HUD shows them, so
	// they are real server state rather than decoration. Nothing drains them
	// yet — that is a design call about whether a battle royale wants upkeep.
	Hunger    int `json:"hun"`
	MaxHunger int `json:"maxHun"`
	Thirst    int `json:"thi"`
	MaxThirst int `json:"maxThi"`

	// Attributes drive the Estadísticas panel. Sent fresh every tick rather
	// than once at join, the same tradeoff as the status booleans below —
	// Fuerza/Agilidad move under a temporary buff or debuff (see
	// World.effectiveAttributes), so a value read once at spawn would go
	// stale the moment a spell landed.
	Fuerza       int `json:"str"`
	Agilidad     int `json:"agi"`
	Inteligencia int `json:"int"`
	Carisma      int `json:"cha"`
	Constitucion int `json:"con"`

	// Status effects, computed fresh each tick rather than tracked as
	// standing state — see World.broadcast.
	Paralyzed   bool `json:"paralyzed,omitempty"`
	Immobilized bool `json:"immobilized,omitempty"`
	Invisible   bool `json:"invisible,omitempty"`
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
	// Weapon, Shield and Helmet are the worn-equipment animation indices, the
	// same three Argentum puts in CharacterCreate. Body above already carries
	// the armour: in Argentum equipping armour replaces your body rather than
	// layering over it, so there is no separate "armor" field here and there
	// is not meant to be one.
	//
	// 2 means nothing equipped, not 0 — the source's NingunArma/NingunEscudo/
	// NingunCasco are all 2. Sent unconditionally rather than omitempty for
	// exactly that reason: 0 would be a different, wrong thing to say.
	Weapon int `json:"wp"`
	Shield int `json:"sh"`
	Helmet int `json:"hm"`
	// Dead marks an eliminated player, so the client can draw a body rather
	// than a character standing around.
	Dead bool `json:"d,omitempty"`
	// Paralyzed/Immobilized are shown for whoever is visible — Argentum shows
	// a stunned enemy's state, which matters tactically. Invisible has no
	// equivalent field here: an invisible player is simply absent from
	// everyone's Entities except their own.
	Paralyzed   bool `json:"pz,omitempty"`
	Immobilized bool `json:"im,omitempty"`

	// Clan, Desc and Kills are what clicking a character reports, the way
	// Argentum answers a click with "Nombre <Clan> <Descripción>". They ride
	// along in the snapshot rather than answering a separate request because
	// the viewport already limits who is in it: a client can only ever read
	// these for characters it can see, which is the same rule that protects
	// positions, so a lookup message would buy nothing but a round trip.
	//
	// Clan is always empty today — there is no guild system — and the client
	// omits the bracket when it is, exactly as the original does for a
	// clanless character.
	Clan  string `json:"cl,omitempty"`
	Desc  string `json:"ds,omitempty"`
	Kills int    `json:"k,omitempty"`
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
	// Ground is every item stack lying on the map inside the viewport.
	Ground []GroundItem `json:"g,omitempty"`
}

// InventorySlot is one bag slot. ItemID indexes Argentum's obj.dat, which the
// client already ships, so only the number travels.
type InventorySlot struct {
	Slot     int  `json:"s"`
	ItemID   int  `json:"i"`
	Amount   int  `json:"n"`
	Equipped bool `json:"e,omitempty"`
}

// Loadout is what a player carries and knows.
//
// It rides its own message rather than the snapshot: a bag changes when someone
// picks something up, not twenty times a second.
type Loadout struct {
	Inventory []InventorySlot `json:"inv"`
	Spells    []int           `json:"spells"`
}

// Attack swings at whoever is standing on the tile the player faces. There is
// no target field on purpose: melee in Argentum hits the square in front of
// you, so a client cannot name someone across the map as its victim.
type Attack struct{}

// Cast asks to throw a spell at somebody.
//
// Unlike Attack this does name a target, because Argentum spells reach across
// the screen. The server re-checks that the spell is known, that the target is
// close enough to see and that the caster can pay for it.
type Cast struct {
	SpellID int    `json:"spell"`
	Target  uint32 `json:"target"`
}

// SpellEvent narrates one cast. Failed carries the reason when nothing happened,
// so the player learns why instead of watching the mana vanish.
type SpellEvent struct {
	CasterID   uint32 `json:"c"`
	CasterName string `json:"cn,omitempty"`
	VictimID   uint32 `json:"v,omitempty"`
	VictimName string `json:"vn,omitempty"`
	SpellID    int    `json:"s,omitempty"`
	SpellName  string `json:"sn,omitempty"`
	Words      string `json:"w,omitempty"`
	Damage     int    `json:"dmg,omitempty"`
	Healed     int    `json:"heal,omitempty"`
	Killed     bool   `json:"killed,omitempty"`

	// Status outcomes. AgilityDelta/StrengthDelta are signed and never zero
	// when present — a positive value is Celeridad/Fuerza, negative is
	// Torpeza/Debilidad — so the client can tell buff from debuff without a
	// separate flag.
	Paralyzed        bool `json:"paralyzed,omitempty"`
	Immobilized      bool `json:"immobilized,omitempty"`
	RemovedParalysis bool `json:"removedParalysis,omitempty"`
	MadeInvisible    bool `json:"invisible,omitempty"`
	AgilityDelta     int  `json:"agDelta,omitempty"`
	StrengthDelta    int  `json:"fuDelta,omitempty"`

	Failed string `json:"failed,omitempty"`
	Mine   bool   `json:"mine"`
}

// CombatEvent narrates one swing to both people involved.
type CombatEvent struct {
	AttackerID   uint32 `json:"a"`
	AttackerName string `json:"an"`
	VictimID     uint32 `json:"v"`
	VictimName   string `json:"vn"`
	Hit          bool   `json:"hit"`
	Blocked      bool   `json:"blocked,omitempty"`
	Damage       int    `json:"dmg,omitempty"`
	Killed       bool   `json:"killed,omitempty"`
	// Mine tells the client whether it was the one swinging, so it can word
	// the line without having to compare ids itself.
	Mine bool `json:"mine"`
}

// Drop asks to place one inventory slot's whole stack on the ground at the
// player's own tile. Pickup has no payload — Agarrar in the source always
// takes from the tile you're standing on, never a tile you point at.
type Drop struct {
	Slot int `json:"slot"`
}

// GroundItem is one item stack lying on the map, inside a player's viewport —
// the same interest-management rule as Entities, so a modified client cannot
// learn about loot it hasn't actually seen.
type GroundItem struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	ItemID int `json:"i"`
	Amount int `json:"n"`
}

// Swap asks to reorder two bag slots — Argentum's own drag-and-drop within
// the inventory window. Landing on an empty slot is just as valid as landing
// on an occupied one; either way the server decides what actually moves
// where, never the client.
type Swap struct {
	From int `json:"from"`
	To   int `json:"to"`
}

// Use asks to act on one bag slot.
//
// Argentum overloads a single "use item" click and lets the item's own ObjType
// pick the branch, which is what Action "" still does — it is what a
// double-click sends. The two explicit actions exist because overloading is
// only pleasant when you meant either outcome: pressing a key expecting to put
// a sword on and instead drinking the potion that shares that habit is not the
// item type being clever, it is the input being ambiguous. E asks to equip and
// U asks to consume, and asking the wrong one of a slot is answered with a
// message rather than with the other thing happening.
type Use struct {
	Slot   int       `json:"slot"`
	Action UseAction `json:"a,omitempty"`
}

// UseAction narrows what a Use means. Empty is the original's own overloaded
// click and stays the default, so an older client is unaffected.
type UseAction string

const (
	UseAuto  UseAction = ""
	UseEquip UseAction = "equip"
	UseUseUp UseAction = "use"
)

// UseResult narrates what a Use actually did. Exactly one of the outcome
// fields is meaningful per call: equipment toggles Equipped/Unequipped,
// consumables set the rest.
type UseResult struct {
	ItemName string `json:"item"`

	Equipped   bool `json:"equipped,omitempty"`
	Unequipped bool `json:"unequipped,omitempty"`

	Consumed       bool `json:"consumed,omitempty"`
	HealedHP       int  `json:"healHp,omitempty"`
	RestoredMana   int  `json:"restoredMana,omitempty"`
	RestoredHunger int  `json:"restoredHunger,omitempty"`
	RestoredThirst int  `json:"restoredThirst,omitempty"`
	AgilityDelta   int  `json:"agDelta,omitempty"`
	StrengthDelta  int  `json:"fuDelta,omitempty"`
	CuredPoison    bool `json:"curedPoison,omitempty"`
	// Died is the Poción Negra joke item: a coin-flip's worth of "why would
	// anyone drink this" that classic AO players did anyway.
	Died bool `json:"died,omitempty"`

	Failed string `json:"failed,omitempty"`
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
