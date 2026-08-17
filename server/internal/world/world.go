// Package world holds the authoritative simulation.
//
// The whole world lives on a single goroutine that owns every field of World.
// Connections hand it work over channels and never touch state directly, so
// there is not a mutex anywhere in here and tick order is deterministic.
package world

import (
	"context"
	"encoding/base64"
	"errors"
	"log/slog"
	"math/rand"
	"sort"
	"time"

	"juegito/server/internal/protocol"
	"juegito/server/internal/transport"
)

const (
	// ViewportW and ViewportH mirror the classic Argentum Online render window.
	// The client never draws more than this, which makes the viewport the
	// natural unit of interest management: a player is only ever told about
	// entities inside it, so a 50-player match still sends small snapshots.
	ViewportW = 17
	ViewportH = 13

	// Walk speed. Argentum's cadence is 5 tiles per second, which at the
	// default 20 Hz is exactly 4 ticks per step; walkSpeedPercent trims that
	// to taste and is the one number to turn when it needs retuning. Facing
	// changes stay free — you turn instantly, you walk on a cadence.
	//
	// The cooldown is carried in thousandths of a tick rather than in whole
	// ticks because whole ticks cannot express the trim. 4 ticks is 5 tiles/s
	// and 5 ticks is 4 tiles/s, so the nearest integer move away from 100% is
	// a 20% cut — there is nothing in between. At 90% the honest figure is
	// 4.444 ticks, and rounding it to either neighbour would quietly deliver
	// 0% or -20% instead of what was asked for.
	baseMoveCooldownMilliticks = 4000
	walkSpeedPercent           = 90
	moveCooldownMilliticks     = baseMoveCooldownMilliticks * 100 / walkSpeedPercent

	// turnCooldownMilliticks is INT_CHANGE_HEADING from the client's
	// Declares.bas: 300ms, which at 20Hz is 6 ticks. It only gates turning in
	// place; a turn that comes with a step rides the step's own cadence.
	turnCooldownMilliticks = 6 * 1000

	// maxConsecutiveDrops is how many snapshots a client may miss in a row
	// before it is disconnected.
	maxConsecutiveDrops = 30

	// spawnAttempts bounds the random search for a free spawn tile before
	// falling back to a linear scan.
	spawnAttempts = 200
)

// Appearances that the client has assets bundled for. Argentum defines
// hundreds; these are the ones tools/aoconv currently packs into the atlas, and
// the two lists must stay in step with that bundle.
var (
	// Body 8 and head 500 are bundled but deliberately absent here: they are
	// Argentum's corpse (ghostBody/ghostHead in combat.go), not appearances a
	// living player should be handed. Body 8 was in this pool until the ghost
	// went in, which meant a share of players spawned already looking dead —
	// and with no sideways walk frames, since a corpse has none.
	availableBodies = []int{1, 2, 3, 4, 5, 6, 7}
	availableHeads  = []int{1, 2, 3, 4, 5, 6, 7, 8}
)

// Starting vitals. Every player is identical for now: classes and races decide
// these in classic AO, and that table comes over from the VB6 source once class
// selection exists.
// The classic Argentum newbie kit, by obj.dat id: a sword, both potions,
// something to eat and something to drink. Equipment and consumables do
// nothing yet — this is what the inventory panel is drawing.
var startingInventory = []protocol.InventorySlot{
	{Slot: 0, ItemID: 2, Amount: 1, Equipped: true}, // Espada Larga
	{Slot: 1, ItemID: 38, Amount: 100},              // Poción Roja
	{Slot: 2, ItemID: 37, Amount: 100},              // Poción Azul
	{Slot: 3, ItemID: 1, Amount: 20},                // Manzana Roja
	{Slot: 4, ItemID: 43, Amount: 20},               // Botella de Agua
}

// Known spells, by Hechizos.dat id: 1-12 plus, by id, the specific
// status-effect spells 14 Invisibilidad, 18 Celeridad, 19 Torpeza, 20 Fuerza,
// 21 Debilidad.
var startingSpells = []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 14, 18, 19, 20, 21}

// The three heavy attack spells, by Hechizos.dat id, with the mana each costs
// in that same file. They are handed out by what a class can actually pay for
// rather than to everyone, because a spell you can never afford is a dead row
// in the book — the same reason the mana formulas were worth porting exactly.
//
// The figures to compare against are manaFor's, which for a Humano are: Mago
// 2524, Clérigo/Bardo/Druida 1810, Paladín/Asesino 930, Bandido 622, everyone
// else 0.
const (
	spellFireStorm  = 15 // Tormenta de Fuego, 250 mana, 45-55 damage
	spellLightning  = 23 // Descarga Eléctrica, 460 mana, 55-85 damage
	spellApocalypse = 25 // Apocalipsis, 1000 mana, 85-100 damage
)

// heavySpellsFor is which of the three a class gets.
//
// Apocalipsis at 1000 mana is a Mago spell and reads as one: a Clérigo could
// pay for it exactly once and then be empty for the rest of the match, which
// is not a spell so much as a single button. Descarga Eléctrica at 460 is
// affordable two to four times over by everyone from Paladín up, and Tormenta
// de Fuego at 250 by anyone with mana at all — including the Bandido, whose
// 622 is the lowest non-zero pool in the game.
//
// The five classes with no mana get none of them, which is not a special case
// here: they get the whole spell list already and can cast none of it.
func heavySpellsFor(class Class) []int {
	switch class {
	case Mago:
		return []int{spellFireStorm, spellLightning, spellApocalypse}
	case Clerigo, Druida, Bardo:
		return []int{spellFireStorm, spellLightning}
	case Paladin, Asesino:
		return []int{spellFireStorm, spellLightning}
	case Bandido:
		return []int{spellFireStorm}
	default:
		return nil
	}
}

// spellBook lays the starting spells into a fixed SpellSlots-long book, the
// trailing slots empty. Every player gets their own copy: the order is theirs
// to rearrange (see swapSpells), so a shared backing array would have one
// player's drag reshuffle everyone else's list.
func spellBook(class Class) []int {
	book := make([]int, SpellSlots)
	n := copy(book, startingSpells)
	for _, id := range heavySpellsFor(class) {
		if n >= len(book) {
			break // the book is full; better a missing spell than a panic
		}
		book[n] = id
		n++
	}
	return book
}

var startingVitals = protocol.Vitals{
	Level: 1,
	Exp:   0, MaxExp: 300,
	HP: 100, MaxHP: 100,
	Mana: 100, MaxMana: 100,
	Stamina: 100, MaxStamina: 100,
	Hunger: 100, MaxHunger: 100,
	Thirst: 100, MaxThirst: 100,
}

// ErrWorldClosed is returned when the simulation has stopped.
var ErrWorldClosed = errors.New("world: closed")

type tileKey struct{ X, Y int }

type joinReq struct {
	name  string
	class Class
	race  Race
	conn  transport.Conn
	resp  chan EntityID
}

type command struct {
	id      EntityID
	typ     protocol.MsgType
	payload []byte
}

// World is the authoritative game state for one match.
type World struct {
	grid  *Grid
	codec protocol.Codec
	log   *slog.Logger
	rng   *rand.Rand

	// mapNumber and mapName describe which Argentum map is loaded, so the
	// client can pick the matching tile data it already ships with.
	mapNumber int
	mapName   string

	// items and spells are the converted obj.dat and Hechizos.dat.
	items  map[int]Item
	spells map[int]Spell
	// startingKits is derived from items by SetItems: per (class, race) pair,
	// the minimal newbie-tier loadout every player spawns with. Nil map (no
	// items loaded) falls back to startingInventory below.
	startingKits map[startingKitKey]startingKit

	tickRate int
	tick     uint64

	players map[EntityID]*Player
	// occupied indexes players by tile so a move only has to look at one entry
	// instead of scanning everyone.
	occupied map[tileKey]EntityID
	// ground indexes item stacks lying on the map by tile — the loot a match
	// scatters at start (see loot.go) plus whatever drops on death or by hand.
	// Argentum's own map format holds one object per tile, so this does too.
	ground map[tileKey]groundStack

	joinCh  chan joinReq
	leaveCh chan EntityID
	cmdCh   chan command
	done    chan struct{}

	nextID uint32
	// pending buffers commands that arrived between ticks so they are applied
	// in a defined order rather than racing the simulation.
	pending []command
}

func New(grid *Grid, codec protocol.Codec, tickRate int, log *slog.Logger) *World {
	return &World{
		grid:     grid,
		codec:    codec,
		log:      log,
		rng:      rand.New(rand.NewSource(time.Now().UnixNano())),
		tickRate: tickRate,
		players:  make(map[EntityID]*Player),
		occupied: make(map[tileKey]EntityID),
		ground:   make(map[tileKey]groundStack),
		joinCh:   make(chan joinReq),
		leaveCh:  make(chan EntityID),
		cmdCh:    make(chan command, 1024),
		done:     make(chan struct{}),
	}
}

// SetMap records which Argentum map this world was built from. It must be
// called before Run, since it is read from the world goroutine afterwards.
func (w *World) SetMap(number int, name string) {
	w.mapNumber = number
	w.mapName = name
}

// Run drives the simulation until ctx is cancelled. It must be called exactly
// once, and everything it touches belongs to this goroutine.
func (w *World) Run(ctx context.Context) {
	defer close(w.done)

	ticker := time.NewTicker(time.Second / time.Duration(w.tickRate))
	defer ticker.Stop()

	w.log.Info("world running", "tickRate", w.tickRate, "map", [2]int{w.grid.W, w.grid.H})

	for {
		select {
		case <-ctx.Done():
			w.log.Info("world stopping", "tick", w.tick, "players", len(w.players))
			return
		case req := <-w.joinCh:
			req.resp <- w.addPlayer(req)
		case id := <-w.leaveCh:
			w.removePlayer(id)
		case cmd := <-w.cmdCh:
			w.pending = append(w.pending, cmd)
		case <-ticker.C:
			w.step()
		}
	}
}

// Join registers a client and returns its entity ID.
// Join registers a client. class/race are the player's own choice from the
// character picker — both real players and the bot client always send a real
// selection, so 0 (Guerrero/Humano) is just as legitimate a pick as any other
// and is not treated as "unset." Only a genuinely out-of-range value — from a
// modified client, say — falls back to a random pick in addPlayer.
func (w *World) Join(name string, class Class, race Race, conn transport.Conn) (EntityID, error) {
	resp := make(chan EntityID, 1)
	select {
	case w.joinCh <- joinReq{name: name, class: class, race: race, conn: conn, resp: resp}:
	case <-w.done:
		return 0, ErrWorldClosed
	}
	select {
	case id := <-resp:
		return id, nil
	case <-w.done:
		return 0, ErrWorldClosed
	}
}

// Leave removes a client. It is safe to call for an ID that is already gone.
func (w *World) Leave(id EntityID) {
	select {
	case w.leaveCh <- id:
	case <-w.done:
	}
}

// Submit queues a client command for the next tick. It drops the command rather
// than blocking a connection goroutine if the world is saturated.
func (w *World) Submit(id EntityID, typ protocol.MsgType, payload []byte) {
	select {
	case w.cmdCh <- command{id: id, typ: typ, payload: payload}:
	case <-w.done:
	default:
		w.log.Warn("command queue full, dropping", "id", id, "type", typ)
	}
}

func (w *World) step() {
	w.tick++

	for _, cmd := range w.pending {
		w.apply(cmd)
	}
	w.pending = w.pending[:0]

	w.broadcast()
}

func (w *World) apply(cmd command) {
	p, ok := w.players[cmd.id]
	if !ok {
		return
	}

	switch cmd.typ {
	case protocol.TypeMove:
		var m protocol.Move
		if err := w.codec.DecodePayload(cmd.payload, &m); err != nil {
			return
		}
		w.movePlayer(p, m.Dir)
	case protocol.TypeAttack:
		w.attack(p)
	case protocol.TypeCast:
		var c protocol.Cast
		if err := w.codec.DecodePayload(cmd.payload, &c); err != nil {
			return
		}
		w.cast(p, c.SpellID, EntityID(c.Target))
	case protocol.TypeUse:
		var u protocol.Use
		if err := w.codec.DecodePayload(cmd.payload, &u); err != nil {
			return
		}
		w.useItem(p, u.Slot, u.Action)
	case protocol.TypeHide:
		w.hide(p)
	case protocol.TypePickup:
		w.pickup(p)
	case protocol.TypeDrop:
		var d protocol.Drop
		if err := w.codec.DecodePayload(cmd.payload, &d); err != nil {
			return
		}
		w.dropItem(p, d.Slot)
	case protocol.TypeSwap:
		var s protocol.Swap
		if err := w.codec.DecodePayload(cmd.payload, &s); err != nil {
			return
		}
		w.swapSlots(p, s.From, s.To)
	case protocol.TypeSwapSpell:
		var s protocol.Swap
		if err := w.codec.DecodePayload(cmd.payload, &s); err != nil {
			return
		}
		w.swapSpells(p, s.From, s.To)
	default:
		w.log.Debug("ignoring unknown command", "id", cmd.id, "type", cmd.typ)
	}
}

// turn changes facing without taking a step, on its own interval.
//
// This is the source's INT_CHANGE_HEADING (Declares.bas), 300ms, and the
// client only reaches it when the move itself is illegal — blocked, rooted, or
// still mid-step. A legal step carries its own turn along with it and never
// comes through here, which is why walking around a corner feels immediate
// while pivoting against a wall does not.
func (w *World) turn(p *Player, dir protocol.Heading) {
	if p.Heading == dir {
		return
	}
	now := w.tick * 1000
	if now < p.turnReadyAt {
		return
	}
	p.Heading = dir
	p.turnReadyAt = now + turnCooldownMilliticks
}

func (w *World) movePlayer(p *Player, dir protocol.Heading) {
	if !dir.Valid() || p.Dead {
		return
	}
	// Paralysis and immobilize both root you — HandleWalk in the source
	// rejects the move outright under either, though a rooted player can
	// still turn to face a new direction and swing.
	if !p.canMove(w.tick) {
		// A rooted player can still turn to face a new direction and swing,
		// which is what the source does — on the turn timer, since there is no
		// step to carry the turn along with.
		w.turn(p, dir)
		return
	}

	// The step clock runs in milliticks so a fractional cadence is expressible;
	// see moveCooldownMilliticks. The remainder has to survive from one step to
	// the next, which is why moveReadyAt is advanced by the cooldown rather
	// than reset to now — resetting would round every step up to a whole tick
	// and collapse 4.444 back into a flat 5.
	now := w.tick * 1000
	if now < p.moveReadyAt {
		// Mid-step. The step is refused, but a turn is not: it goes on the
		// turn timer instead.
		//
		// Heading used to be assigned before this gate, on every command that
		// arrived. The client polls the movement keys once per frame, so at
		// 60fps that was up to 60 turns a second while a single step was still
		// being drawn — the character spun in place while sliding sideways.
		// Then it was quantised to the step cadence, which overcorrected: the
		// source gives turning its own, separate interval and only falls back
		// to it when the move itself is refused.
		w.turn(p, dir)
		return
	}
	// Standing still must not bank credit toward a burst of fast steps, so the
	// carry is capped at a single cooldown. A player who just idled for ten
	// seconds starts walking at the normal cadence, not with ten free steps.
	if now-p.moveReadyAt > moveCooldownMilliticks {
		p.moveReadyAt = now
	}

	// Past the gate, so this is a step boundary: turn now, and keep the turn
	// even if the step below is refused. Walking into a wall in Argentum faces
	// you at the wall.
	p.Heading = dir

	dx, dy := dir.Delta()
	nx, ny := p.X+dx, p.Y+dy
	if w.grid.Blocked(nx, ny) {
		return
	}
	if _, taken := w.occupied[tileKey{nx, ny}]; taken {
		return
	}

	delete(w.occupied, tileKey{p.X, p.Y})
	p.X, p.Y = nx, ny
	w.occupied[tileKey{nx, ny}] = p.ID
	p.moveReadyAt += moveCooldownMilliticks

	// Moving reveals a player hidden via Ocultarse, unless their class is one
	// of the two the source lets keep walking while hidden. This does not
	// apply to Invisibilidad: the spell shares the same InvisibleUntil field,
	// but only HiddenBySkill carries this rule, matching what the source
	// actually ties to movement (Protocol.bas's walk handler, not the spell).
	if p.invisible(w.tick) && p.HiddenBySkill && p.Class != Ladron && p.Class != Bandido {
		p.revealHidden()
	}
}

func (w *World) broadcast() {
	alive := w.aliveCount()
	for _, p := range w.players {
		// Vitals go out every tick even though they change rarely. At this
		// scale the extra bytes are noise, and the alternative — tracking what
		// each client last saw — is a cache to keep correct for no gain yet.
		// Status booleans are computed here rather than kept in sync on
		// Player.Vitals itself, the same way Alive is computed fresh above.
		vitals := p.Vitals
		vitals.Paralyzed = p.paralyzed(w.tick)
		vitals.Immobilized = p.immobilized(w.tick)
		vitals.Invisible = p.invisible(w.tick)
		attrs := w.effectiveAttributes(p)
		vitals.Fuerza = attrs.Fuerza
		vitals.Agilidad = attrs.Agilidad
		vitals.Inteligencia = attrs.Inteligencia
		vitals.Carisma = attrs.Carisma
		vitals.Constitucion = attrs.Constitucion
		w.sendTo(p, protocol.TypeSnapshot, protocol.Snapshot{
			Tick:     w.tick,
			Alive:    alive,
			Self:     &vitals,
			Entities: w.viewportOf(p),
			Ground:   w.groundItemsInView(p),
		})
	}
}

// viewportOf collects the entities p can see. This is O(players) per player,
// so O(players²) per tick — about 2500 comparisons at 50 players, which is
// nothing. A spatial index is the upgrade path once entities number in the
// hundreds, not before.
func (w *World) viewportOf(p *Player) []protocol.EntityState {
	const halfW, halfH = ViewportW / 2, ViewportH / 2

	out := make([]protocol.EntityState, 0, 8)
	for _, other := range w.players {
		dx, dy := other.X-p.X, other.Y-p.Y
		if dx < -halfW || dx > halfW || dy < -halfH || dy > halfH {
			continue
		}
		// An invisible player is absent from everyone's viewport but their
		// own — not shown-but-marked, actually absent, so a modified client
		// has nothing to read a position out of.
		if other.ID != p.ID && other.invisible(w.tick) {
			continue
		}
		// Derived fresh rather than stored, so what a character looks like can
		// never drift from what they are actually wearing.
		look := w.appearanceOf(other)
		out = append(out, protocol.EntityState{
			ID:          uint32(other.ID),
			X:           other.X,
			Y:           other.Y,
			Heading:     other.Heading,
			Body:        look.Body,
			Head:        look.Head,
			Weapon:      look.Weapon,
			Shield:      look.Shield,
			Helmet:      look.Helmet,
			Name:        other.Name,
			Dead:        other.Dead,
			Paralyzed:   other.paralyzed(w.tick),
			Immobilized: other.immobilized(w.tick),
			// Clan stays empty until a guild system exists; the field is
			// carried now so the console line has the shape Argentum gives it.
			Desc:  other.Class.String(),
			Kills: other.Kills,
		})
	}
	// Map iteration order is random in Go; sorting keeps snapshots stable so
	// they can be diffed by eye when debugging.
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (w *World) addPlayer(req joinReq) EntityID {
	w.nextID++
	id := EntityID(w.nextID)

	x, y := w.freeSpawn()
	// The client picks class and race before ever connecting; out-of-range
	// ids (a stale client, a bot that didn't bother) fall back to random
	// rather than being rejected.
	class := req.class
	if class < 0 || int(class) >= len(allClasses) {
		class = allClasses[w.rng.Intn(len(allClasses))]
	}
	race := req.race
	if race < 0 || int(race) >= len(allRaces) {
		race = allRaces[w.rng.Intn(len(allRaces))]
	}

	inventory := w.startingKits[startingKitKey{class, race}].inventory()
	if len(inventory) == 0 {
		// No item table loaded (e.g. running without -items-file): fall back
		// to the fixed newbie kit rather than spawning with an empty bag.
		inventory = append([]protocol.InventorySlot(nil), startingInventory...)
	}

	p := &Player{
		ID:         id,
		Name:       req.name,
		X:          x,
		Y:          y,
		Body:       availableBodies[w.rng.Intn(len(availableBodies))],
		Head:       availableHeads[w.rng.Intn(len(availableHeads))],
		Class:      class,
		Race:       race,
		Attributes: rolledAttributes(race),
		Skills:     startingSkills,
		Vitals:     vitalsFor(class, race),
		Inventory:  inventory,
		Spells:     spellBook(class),
		conn:       req.conn,
	}
	w.players[id] = p
	w.occupied[tileKey{x, y}] = id

	w.sendTo(p, protocol.TypeWelcome, protocol.Welcome{
		EntityID:  uint32(id),
		TickRate:  w.tickRate,
		MapNumber: w.mapNumber,
		MapName:   w.mapName,
		MapWidth:  w.grid.W,
		MapHeight: w.grid.H,
		Blocked:   base64.StdEncoding.EncodeToString(w.grid.PackedBitset()),
		ViewW:     ViewportW,
		ViewH:     ViewportH,
		SpawnX:    x,
		SpawnY:    y,
		// Tiles per second, derived from the same constant the step gate uses
		// so the client's interpolation cannot drift from the server's cadence.
		WalkSpeed:  float64(w.tickRate) * 1000 / moveCooldownMilliticks,
		SpellSlots: SpellSlots,
	})

	w.sendTo(p, protocol.TypeLoadout, protocol.Loadout{
		Inventory: p.Inventory,
		Spells:    p.Spells,
	})

	w.log.Info("player joined",
		"id", id, "name", req.name, "pos", [2]int{x, y},
		"players", len(w.players), "addr", req.conn.RemoteAddr())
	return id
}

func (w *World) removePlayer(id EntityID) {
	p, ok := w.players[id]
	if !ok {
		return
	}
	delete(w.occupied, tileKey{p.X, p.Y})
	delete(w.players, id)
	w.log.Info("player left", "id", id, "name", p.Name, "players", len(w.players))
}

// freeSpawn finds an unblocked, unoccupied tile. Random probing is fast while
// the map is mostly empty; the scan is the guarantee that it always terminates.
func (w *World) freeSpawn() (int, int) {
	for i := 0; i < spawnAttempts; i++ {
		x := w.rng.Intn(w.grid.W)
		y := w.rng.Intn(w.grid.H)
		if w.tileFree(x, y) {
			return x, y
		}
	}
	for y := 0; y < w.grid.H; y++ {
		for x := 0; x < w.grid.W; x++ {
			if w.tileFree(x, y) {
				return x, y
			}
		}
	}
	// A map with no free tile is a map bug, not a runtime condition.
	w.log.Error("no free spawn tile, placing at origin")
	return 0, 0
}

func (w *World) tileFree(x, y int) bool {
	if w.grid.Blocked(x, y) {
		return false
	}
	_, taken := w.occupied[tileKey{x, y}]
	return !taken
}

func (w *World) sendTo(p *Player, typ protocol.MsgType, payload any) {
	frame, err := w.codec.Encode(typ, payload)
	if err != nil {
		w.log.Error("encode failed", "id", p.ID, "type", typ, "err", err)
		return
	}

	switch err := p.conn.Send(frame); {
	case err == nil:
		p.consecutiveDrops = 0
	case errors.Is(err, transport.ErrBackpressure):
		p.consecutiveDrops++
		if p.consecutiveDrops >= maxConsecutiveDrops {
			w.log.Warn("disconnecting slow client", "id", p.ID, "drops", p.consecutiveDrops)
			_ = p.conn.Close()
		}
	default:
		_ = p.conn.Close()
	}
}
