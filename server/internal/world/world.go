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

	// moveCooldownTicks is how many ticks must pass between steps. At the
	// default 20 Hz that is 5 tiles per second, which matches classic AO walk
	// speed. Facing changes are free — you turn instantly, you walk on a
	// cadence.
	moveCooldownTicks = 4

	// maxConsecutiveDrops is how many snapshots a client may miss in a row
	// before it is disconnected.
	maxConsecutiveDrops = 30

	// spawnAttempts bounds the random search for a free spawn tile before
	// falling back to a linear scan.
	spawnAttempts = 200
)

// Starting vitals. Every player is identical for now: classes and races decide
// these in classic AO, and that table comes over from the VB6 source once class
// selection exists.
var startingVitals = protocol.Vitals{
	Level: 1,
	HP:    100, MaxHP: 100,
	Mana: 100, MaxMana: 100,
	Stamina: 100, MaxStamina: 100,
}

// ErrWorldClosed is returned when the simulation has stopped.
var ErrWorldClosed = errors.New("world: closed")

type tileKey struct{ X, Y int }

type joinReq struct {
	name string
	conn transport.Conn
	resp chan EntityID
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

	tickRate int
	tick     uint64

	players map[EntityID]*Player
	// occupied indexes players by tile so a move only has to look at one entry
	// instead of scanning everyone.
	occupied map[tileKey]EntityID

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
		joinCh:   make(chan joinReq),
		leaveCh:  make(chan EntityID),
		cmdCh:    make(chan command, 1024),
		done:     make(chan struct{}),
	}
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
func (w *World) Join(name string, conn transport.Conn) (EntityID, error) {
	resp := make(chan EntityID, 1)
	select {
	case w.joinCh <- joinReq{name: name, conn: conn, resp: resp}:
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
	default:
		w.log.Debug("ignoring unknown command", "id", cmd.id, "type", cmd.typ)
	}
}

func (w *World) movePlayer(p *Player, dir protocol.Heading) {
	if !dir.Valid() {
		return
	}
	p.Heading = dir

	if w.tick-p.lastMoveTick < moveCooldownTicks {
		return
	}

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
	p.lastMoveTick = w.tick
}

func (w *World) broadcast() {
	alive := len(w.players)
	for _, p := range w.players {
		// Vitals go out every tick even though they change rarely. At this
		// scale the extra bytes are noise, and the alternative — tracking what
		// each client last saw — is a cache to keep correct for no gain yet.
		vitals := p.Vitals
		w.sendTo(p, protocol.TypeSnapshot, protocol.Snapshot{
			Tick:     w.tick,
			Alive:    alive,
			Self:     &vitals,
			Entities: w.viewportOf(p),
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
		out = append(out, protocol.EntityState{
			ID:      uint32(other.ID),
			X:       other.X,
			Y:       other.Y,
			Heading: other.Heading,
			Name:    other.Name,
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
	p := &Player{ID: id, Name: req.name, X: x, Y: y, Vitals: startingVitals, conn: req.conn}
	w.players[id] = p
	w.occupied[tileKey{x, y}] = id

	w.sendTo(p, protocol.TypeWelcome, protocol.Welcome{
		EntityID:  uint32(id),
		TickRate:  w.tickRate,
		MapWidth:  w.grid.W,
		MapHeight: w.grid.H,
		Blocked:   base64.StdEncoding.EncodeToString(w.grid.PackedBitset()),
		ViewW:     ViewportW,
		ViewH:     ViewportH,
		SpawnX:    x,
		SpawnY:    y,
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
