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
	// TypeShoot is the ranged half of TypeAttack. Separate because the two
	// answer different questions: melee hits the tile you face and needs no
	// target, an arrow crosses the screen and must name one.
	TypeShoot    MsgType = "shoot"
	TypeCast     MsgType = "cast"
	TypeUse      MsgType = "use"
	TypeHide     MsgType = "hide"
	TypeMeditate MsgType = "meditate"
	TypePickup   MsgType = "pickup"
	TypeDrop     MsgType = "drop"
	TypeSwap     MsgType = "swap"
	// TypeSwapSpell reorders the spell book, the same drag-and-drop Argentum
	// gives its own spell list. Separate from TypeSwap because the two lists
	// are separate server state and a bag index is not a spell index.
	TypeSwapSpell MsgType = "swapSpell"
	TypeLogin     MsgType = "login"
	TypeTalk      MsgType = "talk"
	TypePing      MsgType = "ping"
	// TypeQueue steps in or out of the queue for the next match. Naming a
	// character with a Join is what takes the place in the line to begin with,
	// so this only ever has to carry the stepping-out case and the
	// stepping-back-in after it.
	TypeQueue MsgType = "queue"

	// Server -> client.
	TypeWelcome   MsgType = "welcome"
	TypeSnapshot  MsgType = "snapshot"
	TypeLoadout   MsgType = "loadout"
	TypeCombat    MsgType = "combat"
	TypeSpell     MsgType = "spell"
	TypeUseResult MsgType = "useResult"
	TypeSpeech    MsgType = "speech"
	// TypeProjectile is an arrow or a thrown blade in flight, sent to everyone
	// who can see it — not only to the two people it concerns.
	TypeProjectile MsgType = "projectile"
	TypeOutcome    MsgType = "outcome"
	TypeAccount    MsgType = "account"
	TypeHello      MsgType = "hello"
	TypeLobby      MsgType = "lobby"
	TypePong       MsgType = "pong"
	TypeError      MsgType = "error"
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
//
// Seq is the sender's own monotonic counter, one per input attempt (a step or
// a mid-cadence turn), never reset for the life of the connection. It exists
// so Snapshot.AckSeq can tell a predicting client exactly which of its inputs
// the server has already answered — see AckSeq for why that matters.
type Move struct {
	Dir Heading `json:"dir"`
	Seq uint32  `json:"seq"`
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
	Meditating  bool `json:"meditating,omitempty"`
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
	// Meditating drives the meditation aura FX everyone in view sees, not just
	// the meditator's own client — Argentum plays it as a looping CreateFX on
	// the character, not a one-shot cast animation, so it rides the standing
	// snapshot state the same way Paralyzed/Immobilized do rather than a
	// one-shot event like SpellEvent.
	Meditating bool `json:"md,omitempty"`

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
	Alive int `json:"alive"`
	// AckSeq is the highest Move.Seq this snapshot's own recipient has had
	// applied so far — never meaningful for anyone else, which is why it rides
	// on a message that is already built fresh per player rather than needing
	// a field of its own.
	//
	// This is what lets client prediction reconcile precisely instead of
	// guessing: without it, a predicting client sees only "the server's
	// position disagrees with mine" and cannot tell a step the server simply
	// hasn't seen yet from one it genuinely rejected. Comparing raw positions
	// for that used to need a multi-snapshot vote (see the client's old
	// DESYNC_SNAPSHOTS), which broke under fast, sustained input: the predicted
	// tile is *always* ahead of the last-acked one while a key is held, so the
	// vote fired anyway and the player got yanked backward mid-stride. With
	// AckSeq the client drops exactly the inputs this number covers and
	// replays only what is genuinely still in flight.
	AckSeq uint32  `json:"ack"`
	Self   *Vitals `json:"self,omitempty"`
	// Entities is everyone inside the viewport, including the player itself.
	Entities []EntityState `json:"e"`
	// Ground is every item stack lying on the map inside the viewport.
	Ground []GroundItem `json:"g,omitempty"`
	// Zone is the shrinking circle, or nil when the match has none. Sent to
	// everybody in full: unlike positions, where the ring is going is public
	// information — the whole mechanic is that everyone can see it coming.
	Zone *Zone `json:"z,omitempty"`
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

// Shoot fires the equipped bow, or throws the equipped blade, at somebody.
//
// Unlike Attack this names a target, for the same reason Cast does: the shot
// crosses the screen, so the tile in front of the shooter says nothing about
// who it is aimed at. The server re-checks that there is a projectile weapon
// equipped, that there is ammunition behind it, and that the target is close
// enough to be seen — the same viewport bound every other reach in this
// protocol is held to.
type Shoot struct {
	Target uint32 `json:"target"`
}

// Projectile is one arrow or thrown blade crossing the ground between two
// tiles, for anybody whose viewport it passes through.
//
// It is its own message rather than a flag on CombatEvent because the two have
// different audiences, which is the whole point: the damage is between the two
// people involved, and the arrow is a thing anybody nearby can watch happen.
// Seeing where a shot came from is how a ranged fight tells everyone else
// there is a fight — the same logic as the magic words over a caster's head.
//
// Tiles rather than entity ids alone, because the shooter is not always in the
// receiver's snapshot: the arrow has to be drawn along the ground it actually
// crossed. The ids ride along so a client that does have both entities can
// anchor the line to where it is drawing them right now, mid-step, instead of
// to the tile they were standing on when the server resolved the shot.
type Projectile struct {
	FromID uint32 `json:"a"`
	ToID   uint32 `json:"v"`
	FromX  int    `json:"x"`
	FromY  int    `json:"y"`
	ToX    int    `json:"tx"`
	ToY    int    `json:"ty"`
	// ItemID is what is flying — the arrow, or the blade itself when it is a
	// thrown weapon. The client already ships obj.dat, so only the number
	// travels and it draws the real sprite.
	ItemID int `json:"i"`
}

// Meditate toggles meditation on or off — F6 in the original, one key with no
// payload either way. What it does is server state (Vitals.Meditating and
// EntityState.Meditating carry that out on every snapshot); this message is
// only ever the request to flip it.
type Meditate struct{}

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

// Talk is a player saying something out loud.
type Talk struct {
	Text string `json:"text"`
}

// Speech is somebody's words, shown over their head to everyone who can see
// them.
//
// It is deliberately the same message for chat and for a spell's incantation,
// because in Argentum they are the same thing: DecirPalabrasMagicas sends the
// spell's PalabrasMagicas to everyone in the area anchored to the caster, and
// the client draws it with Dialogos.CreateDialog exactly as it draws a chat
// line. One sign per character, so a new one replaces the old — which is what
// makes saying anything a way to wipe the incantation off your own head.
//
// The consequence is the point: casting announces where you are. Argentum goes
// further and drops your Ocultar outright (modHechizos.bas), which this does
// too.
type Speech struct {
	EntityID uint32 `json:"id"`
	// X and Y are where the sign hangs. They travel with the message because
	// the speaker is not always in the receiver's snapshot: an invisible
	// caster is absent from it by design, and drawing their words in empty air
	// over their real tile is precisely the tell. Bounded by the viewport like
	// everything else, so this reveals nothing about anyone you could not
	// already have walked into.
	X    int    `json:"x"`
	Y    int    `json:"y"`
	Text string `json:"text"`
	// Spell marks an incantation rather than something the player typed, so
	// the client can colour it the way the original does (a light green).
	Spell bool `json:"spell,omitempty"`
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
	// Zone marks damage taken from the shrinking ring rather than from a
	// player, so the client can say "the zone is killing you" instead of
	// naming an attacker that does not exist.
	Zone bool `json:"zone,omitempty"`
	// Ranged says an arrow or a thrown blade did this rather than a swing, so
	// the console can word it as a shot. The arrow itself is a Projectile,
	// which goes to a wider audience — see there.
	Ranged bool `json:"rng,omitempty"`
	// Failed is why the shot never happened: no bow, no arrows, out of range.
	// Only ever sent to whoever tried, and only for ranged attacks — a melee
	// swing at an empty tile is its own answer, while an archer whose quiver
	// ran out is owed a sentence rather than a key that stopped working.
	Failed string `json:"failed,omitempty"`
}

// Zone is the shrinking circle of safe ground.
//
// Positions are floats over a tile grid on purpose: the ring moves continuously
// during a contraction, and rounding it to tiles would make it jump a whole
// square at a time and make "am I inside" flicker for anyone standing on the
// edge.
type Zone struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Radius float64 `json:"r"`
	// The circle being closed toward, drawn ahead of time so players can see
	// where to run. Radius is zero once there is no further contraction.
	NextX      float64 `json:"nx,omitempty"`
	NextY      float64 `json:"ny,omitempty"`
	NextRadius float64 `json:"nr,omitempty"`
	// Seconds until the current phase ends: until the ring starts moving, or
	// until it stops.
	Seconds   float64 `json:"t,omitempty"`
	Stage     int     `json:"st"`
	Shrinking bool    `json:"s,omitempty"`
}

// Hello is the only message the server sends before being spoken to.
//
// It exists because the client cannot otherwise know which handshake it is in:
// a server with accounts wants a Login first, one without wants a Join, and
// guessing wrong means either an error frame the user sees or a login screen on
// a server that has no idea what an account is. One field, sent on connect,
// settles it.
type Hello struct {
	// Accounts is whether this server requires signing in.
	Accounts bool `json:"accounts,omitempty"`
	// MinPassword is the server's own floor, so the client can say no before
	// spending a round trip on it.
	MinPassword int `json:"minpass,omitempty"`
}

// Login is the first message on a server that has accounts, and it replaces
// Join's name: from here on the name is something the server knows rather than
// something the client asserts, which is the whole point of keeping a record.
//
// One message for both sign-in and sign-up, told apart by a flag, because they
// are the same form with the same two fields and a client that had to guess
// which one it was on would guess wrong for somebody.
type Login struct {
	Name     string `json:"name"`
	Password string `json:"pass"`
	// Email is collected when registering and ignored otherwise, so signing in
	// never asks for it. It is stored in the clear in the account log, which is
	// append-only and never rewritten — see account.Store.Register for what
	// that means and when it should stop being true.
	Email string `json:"email,omitempty"`
	// Register asks for the account to be created. It fails if the name is
	// taken rather than falling through to a sign-in attempt, so a typo in an
	// existing name never silently becomes "wrong password".
	Register bool `json:"new,omitempty"`
}

// Queue steps in or out of the line for the next match.
type Queue struct {
	// Join is true to take a place in the queue, false to give it up.
	Join bool `json:"join"`
}

// LobbyState is the waiting room, sent to everybody sitting in it.
//
// It is only ever sent to somebody who is *not* playing: once the match starts
// the world's own snapshot takes over and says everything there is to say. A
// client that stops receiving these and starts receiving a Welcome has been let
// in, and one that starts receiving them again has been sent back — which is
// the whole of both transitions, with no message of their own needed.
type LobbyState struct {
	// Queued is how many are waiting, and Needed is how many it takes to
	// start. Both are whole-lobby numbers rather than viewport-scoped ones:
	// nobody has a position yet, so there is nothing here to leak.
	Queued int `json:"q"`
	Needed int `json:"need"`
	// Mine is whether this particular recipient is one of the queued.
	Mine bool `json:"mine,omitempty"`
	// Seconds is the countdown to the match starting. It only runs once the
	// queue is deep enough, and it is cancelled if somebody leaves and takes
	// it back under — so a client showing it has to be ready for it to stop.
	Seconds float64 `json:"t,omitempty"`
	// Counting says the countdown above is live, which a zero Seconds cannot:
	// the last tick before the match starts is legitimately zero.
	Counting bool `json:"c,omitempty"`
	// Running says a match is already under way, so this queue is for the next
	// one. Waiting out a match in progress is the normal case on a busy
	// server, and a lobby that only ever said "faltan 3" would be lying about
	// what it is waiting for.
	Running bool `json:"run,omitempty"`
	// Playing is how many are in the match currently under way, so the lobby
	// can say what it is waiting on rather than only that it is waiting.
	Playing int `json:"play,omitempty"`
}

// MatchRow is one finished match in a career.
type MatchRow struct {
	At    int64   `json:"at"`
	Place int     `json:"place"`
	Of    int     `json:"of"`
	Kills int     `json:"kills"`
	Secs  float64 `json:"secs"`
	Won   bool    `json:"won,omitempty"`
	Map   string  `json:"map,omitempty"`
}

// Account is a career, sent once the login succeeds and again whenever it
// changes. It is what the account screen draws.
type Account struct {
	Name  string `json:"name"`
	Since int64  `json:"since"`

	Matches int `json:"matches"`
	Wins    int `json:"wins"`
	Kills   int `json:"kills"`
	// Best is the highest placement ever reached, 1 being a win. Zero means no
	// match has finished yet, and the client draws a dash rather than a
	// suspiciously good "0th".
	Best    int     `json:"best"`
	Seconds float64 `json:"secs"`
	// Recent is the last few matches, newest first.
	Recent []MatchRow `json:"recent,omitempty"`
}

// Outcome is how the match ended for one player, and it is the only message
// that is about the match rather than about the world.
//
// It arrives twice for anyone who does not win: once the moment they are
// eliminated, carrying the half that is already decided — where they placed,
// how many they took with them, how long they lasted — and again when the
// match is called, with the winner filled in. The two are the same message
// because they answer the same question, and a client that draws the second
// one over the first needs no extra logic to do it.
type Outcome struct {
	// Placement is where this player finished, 1 being the winner. It is fixed
	// at the moment of death: with five alive, the fifth is whoever just died.
	Placement int `json:"place"`
	// Players is how many the match held at its fullest, so the placement has
	// something to be out of. Not the current connection count, which has
	// already shrunk by the time anyone reads the card.
	Players int `json:"of"`
	Kills   int `json:"kills"`
	// Seconds is time survived, measured from this player's own join: somebody
	// who connected nine minutes in did not survive nine minutes.
	Seconds float64 `json:"secs"`
	Won     bool    `json:"won,omitempty"`
	// Winner is empty in the message sent at the moment of elimination —
	// nobody has won yet — and set in the one sent when the match is called.
	Winner string `json:"winner,omitempty"`
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

	// Aim is the answer to using an equipped projectile weapon: it is not
	// consumed and it is not equipped, it asks the player who to shoot at.
	//
	// This is WriteWorkRequestTarget(Proyectiles) in the source, and using the
	// bow really is how Argentum arms a shot — UsarInvItem's otWeapon branch
	// checks Proyectil and answers with it, which is why the crosshair belongs
	// to the inventory gesture and not to the attack key.
	Aim bool `json:"aim,omitempty"`

	// Opened and Dropped are a chest: it is not consumed and it is not
	// equipped, it turns into gear on the floor. Dropped says how many pieces,
	// because some of them land outside the viewport and the player would
	// otherwise only ever see the ones at their feet.
	Opened  bool `json:"opened,omitempty"`
	Dropped int  `json:"dropped,omitempty"`

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
