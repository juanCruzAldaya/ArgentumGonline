package world

import (
	"encoding/base64"
	"io"
	"log/slog"
	"sync"
	"testing"

	"juegito/server/internal/protocol"
	"juegito/server/internal/transport"
)

// fakeConn records what the world sent so tests can assert on the wire format
// rather than on internal state.
type fakeConn struct {
	mu     sync.Mutex
	frames [][]byte
	full   bool // when set, Send always reports backpressure
	closed bool
}

func (f *fakeConn) Send(frame []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.full {
		return transport.ErrBackpressure
	}
	f.frames = append(f.frames, append([]byte(nil), frame...))
	return nil
}

func (f *fakeConn) Recv() ([]byte, error) { return nil, io.EOF }

func (f *fakeConn) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
	return nil
}

func (f *fakeConn) RemoteAddr() string { return "fake" }

func (f *fakeConn) lastOfType(t *testing.T, typ protocol.MsgType) []byte {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()

	codec := protocol.JSONCodec{}
	for i := len(f.frames) - 1; i >= 0; i-- {
		got, payload, err := codec.DecodeEnvelope(f.frames[i])
		if err != nil {
			t.Fatalf("undecodable frame: %v", err)
		}
		if got == typ {
			return payload
		}
	}
	t.Fatalf("no %q frame among %d sent", typ, len(f.frames))
	return nil
}

func newTestWorld(t *testing.T, w, h int) *World {
	t.Helper()
	grid := NewGrid(w, h)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return New(grid, protocol.JSONCodec{}, 20, log)
}

// place drops a player at an exact tile, since spawn selection is random and
// these tests care about movement rules rather than about where you start.
func place(t *testing.T, w *World, name string, x, y int) (*Player, *fakeConn) {
	t.Helper()
	conn := &fakeConn{}
	id := w.addPlayer(joinReq{name: name, conn: conn})
	p := w.players[id]

	delete(w.occupied, tileKey{p.X, p.Y})
	p.X, p.Y = x, y
	w.occupied[tileKey{x, y}] = id
	return p, conn
}

func TestWelcomeCarriesTheCollisionMap(t *testing.T) {
	w := newTestWorld(t, 20, 20)
	w.grid.SetBlocked(3, 4, true)

	_, conn := place(t, w, "wachin", 5, 5)

	var welcome protocol.Welcome
	if err := w.codec.DecodePayload(conn.lastOfType(t, protocol.TypeWelcome), &welcome); err != nil {
		t.Fatalf("decode welcome: %v", err)
	}

	if welcome.MapWidth != 20 || welcome.MapHeight != 20 {
		t.Errorf("map dims = %dx%d, want 20x20", welcome.MapWidth, welcome.MapHeight)
	}
	if welcome.ViewW != ViewportW || welcome.ViewH != ViewportH {
		t.Errorf("viewport = %dx%d, want %dx%d", welcome.ViewW, welcome.ViewH, ViewportW, ViewportH)
	}

	bits, err := base64.StdEncoding.DecodeString(welcome.Blocked)
	if err != nil {
		t.Fatalf("decode blocked bitset: %v", err)
	}
	idx := 4*20 + 3
	if bits[idx/8]&(1<<(idx%8)) == 0 {
		t.Error("blocked tile (3,4) did not survive the bitset round trip")
	}
}

func TestMoveIsBlockedByWalls(t *testing.T) {
	w := newTestWorld(t, 20, 20)
	w.tick = 100 // past the initial cooldown
	p, _ := place(t, w, "wachin", 5, 5)
	w.grid.SetBlocked(5, 4, true)

	w.movePlayer(p, protocol.North)

	if p.X != 5 || p.Y != 5 {
		t.Errorf("position = (%d,%d), want (5,5): walked into a wall", p.X, p.Y)
	}
	if p.Heading != protocol.North {
		t.Error("heading should update even when the step is refused")
	}
}

func TestMoveIsBlockedByAnotherPlayer(t *testing.T) {
	w := newTestWorld(t, 20, 20)
	w.tick = 100
	p, _ := place(t, w, "wachin", 5, 5)
	place(t, w, "otro", 6, 5)

	w.movePlayer(p, protocol.East)

	if p.X != 5 || p.Y != 5 {
		t.Errorf("position = (%d,%d), want (5,5): walked through another player", p.X, p.Y)
	}
}

func TestMoveCooldownLimitsWalkSpeed(t *testing.T) {
	w := newTestWorld(t, 20, 20)
	w.tick = 100
	p, _ := place(t, w, "wachin", 5, 5)

	w.movePlayer(p, protocol.East)
	if p.X != 6 {
		t.Fatalf("first step did not happen: x = %d, want 6", p.X)
	}

	// Same tick: the cooldown must refuse a second step.
	w.movePlayer(p, protocol.East)
	if p.X != 6 {
		t.Errorf("x = %d, want 6: cooldown let a second step through", p.X)
	}

	// One tick short of the cooldown: still refused.
	w.tick += moveCooldownMilliticks/1000 - 1
	w.movePlayer(p, protocol.East)
	if p.X != 6 {
		t.Errorf("x = %d, want 6: a step went through before the cooldown elapsed", p.X)
	}
	w.tick++
	w.movePlayer(p, protocol.East)
	if p.X != 7 {
		t.Errorf("x = %d, want 7: step refused after the cooldown elapsed", p.X)
	}
}

// Every step has to take the same number of ticks as every other one.
//
// This asserted the *average* before, and the average was exactly what hid the
// bug. The world clock only advances in whole ticks, so a fractional cooldown
// does not buy fractional steps: at 4.444 the steps landed 5, 4, 5, 4 ticks
// apart while the client interpolated all of them over the one average the
// Welcome reports. That is a 28 ms hitch on every other step, forever, and the
// old test was green through all of it — 4.444 was the mean of a sequence that
// contained no 4.444.
//
// So the assertion is on the gaps themselves, not on what they average out to.
func TestWalkCadenceIsIdenticalEveryStep(t *testing.T) {
	w := newTestWorld(t, 200, 20)
	w.tick = 100
	p, _ := place(t, w, "wachin", 2, 5)

	const steps = 45
	var at []uint64
	for len(at) < steps {
		before := p.X
		w.movePlayer(p, protocol.East)
		if p.X != before {
			at = append(at, w.tick)
			continue
		}
		w.tick++
	}

	want := at[1] - at[0]
	for i := 2; i < len(at); i++ {
		if got := at[i] - at[i-1]; got != want {
			t.Fatalf("el paso %d tardó %d ticks y el anterior %d: la cadencia no es pareja",
				i, got, want)
		}
	}

	if want != moveCooldownMilliticks/1000 {
		t.Errorf("cadencia = %d ticks, se esperaban %d", want, moveCooldownMilliticks/1000)
	}

	// And it has to be Argentum's own 5 tiles a second, which is what makes it
	// a whole number of ticks in the first place.
	if tilesPerSecond := 20 / float64(want); tilesPerSecond != 5 {
		t.Errorf("velocidad = %.2f tiles/s, se esperaban 5", tilesPerSecond)
	}
}

// And the client has to be told exactly that cadence, or it interpolates over
// a different span than the server steps on -- which is the same hitch arriving
// from the other side.
func TestWelcomeReportsTheExactWalkSpeed(t *testing.T) {
	w := newTestWorld(t, 20, 20)
	_, conn := place(t, w, "wachin", 5, 5)

	var welcome protocol.Welcome
	if err := w.codec.DecodePayload(conn.lastOfType(t, protocol.TypeWelcome), &welcome); err != nil {
		t.Fatalf("decode welcome: %v", err)
	}

	// The client derives its step animation as 1000/walkSpeed milliseconds. It
	// has to come out a whole number of ticks, or every step is animated over
	// a span the server never takes.
	stepMs := 1000.0 / welcome.WalkSpeed
	if stepMs != float64(moveCooldownMilliticks/1000)*(1000.0/float64(w.tickRate)) {
		t.Errorf("el cliente interpolaría %.1f ms por paso y el servidor tarda %d ticks",
			stepMs, moveCooldownMilliticks/1000)
	}
}

// Idling must not bank steps: a player who stood still for ten seconds should
// start walking at the normal cadence, not sprint off with a stored-up burst.
func TestIdlingDoesNotBankSteps(t *testing.T) {
	w := newTestWorld(t, 60, 20)
	w.tick = 100
	p, _ := place(t, w, "wachin", 2, 5)

	w.movePlayer(p, protocol.East)
	w.tick += 200 // ten seconds of standing around

	x := p.X
	w.movePlayer(p, protocol.East)
	if p.X != x+1 {
		t.Fatalf("the first step after idling did not happen")
	}
	w.movePlayer(p, protocol.East)
	if p.X != x+1 {
		t.Errorf("x moved %d tiles in one tick: idling banked credit", p.X-x)
	}
}

func TestMoveVacatesTheOldTile(t *testing.T) {
	w := newTestWorld(t, 20, 20)
	w.tick = 100
	p, _ := place(t, w, "wachin", 5, 5)

	w.movePlayer(p, protocol.East)

	if _, stale := w.occupied[tileKey{5, 5}]; stale {
		t.Error("old tile still marked occupied: the map would leak phantom blockers")
	}
	if got := w.occupied[tileKey{6, 5}]; got != p.ID {
		t.Errorf("new tile occupied by %d, want %d", got, p.ID)
	}
}

func TestSnapshotOnlyIncludesTheViewport(t *testing.T) {
	w := newTestWorld(t, 100, 100)
	const halfW = ViewportW / 2

	p, conn := place(t, w, "wachin", 50, 50)
	far, _ := place(t, w, "lejano", 50+halfW+1, 50)

	w.step()
	var snap protocol.Snapshot
	if err := w.codec.DecodePayload(conn.lastOfType(t, protocol.TypeSnapshot), &snap); err != nil {
		t.Fatalf("decode snapshot: %v", err)
	}
	if len(snap.Entities) != 1 || snap.Entities[0].ID != uint32(p.ID) {
		t.Fatalf("snapshot = %+v, want only the local player", snap.Entities)
	}

	// Step the far player one tile closer, onto the viewport edge.
	delete(w.occupied, tileKey{far.X, far.Y})
	far.X = 50 + halfW
	w.occupied[tileKey{far.X, far.Y}] = far.ID

	w.step()
	if err := w.codec.DecodePayload(conn.lastOfType(t, protocol.TypeSnapshot), &snap); err != nil {
		t.Fatalf("decode snapshot: %v", err)
	}
	if len(snap.Entities) != 2 {
		t.Errorf("snapshot has %d entities, want 2: viewport edge should be visible", len(snap.Entities))
	}
}

func TestSnapshotCarriesAliveCountAndOwnVitalsOnly(t *testing.T) {
	w := newTestWorld(t, 100, 100)

	me, conn := place(t, w, "wachin", 50, 50)
	place(t, w, "vecino", 51, 50)
	// Far enough to be outside the viewport, so it must not appear in entities
	// while still counting towards alive.
	place(t, w, "lejano", 90, 90)

	w.step()

	var snap protocol.Snapshot
	if err := w.codec.DecodePayload(conn.lastOfType(t, protocol.TypeSnapshot), &snap); err != nil {
		t.Fatalf("decode snapshot: %v", err)
	}

	if snap.Alive != 3 {
		t.Errorf("alive = %d, want 3: the count is match-wide, not viewport-scoped", snap.Alive)
	}
	if len(snap.Entities) != 2 {
		t.Errorf("entities = %d, want 2: the far player must stay out of the viewport", len(snap.Entities))
	}
	if snap.Self == nil {
		t.Fatal("snapshot carried no vitals for the local player")
	}
	// Max health comes from the class's MODVIDA column times maxLevel — every
	// match starts characters at the level cap — so it is checked against
	// that formula rather than against a fixed number.
	wantMax := int(classModifiers[me.Class].Vida * maxLevel)
	if snap.Self.MaxHP != wantMax {
		t.Errorf("maxHp = %d, want %d for %s", snap.Self.MaxHP, wantMax, me.Class)
	}
	if snap.Self.HP != snap.Self.MaxHP {
		t.Errorf("hp = %d/%d, want to start at full health", snap.Self.HP, snap.Self.MaxHP)
	}
}

func TestSlowClientIsDisconnected(t *testing.T) {
	w := newTestWorld(t, 20, 20)
	p, conn := place(t, w, "lento", 5, 5)

	conn.mu.Lock()
	conn.full = true
	conn.mu.Unlock()

	for i := 0; i < maxConsecutiveDrops; i++ {
		w.sendTo(p, protocol.TypeSnapshot, protocol.Snapshot{Tick: uint64(i)})
	}

	conn.mu.Lock()
	defer conn.mu.Unlock()
	if !conn.closed {
		t.Errorf("client survived %d dropped snapshots, want disconnect", maxConsecutiveDrops)
	}
}

func TestLeaveFreesTheTile(t *testing.T) {
	w := newTestWorld(t, 20, 20)
	p, _ := place(t, w, "wachin", 5, 5)

	w.removePlayer(p.ID)

	if _, stale := w.occupied[tileKey{5, 5}]; stale {
		t.Error("tile still occupied after the player left")
	}
	if len(w.players) != 0 {
		t.Errorf("players = %d, want 0", len(w.players))
	}
}

func TestSanitizeName(t *testing.T) {
	cases := map[string]string{
		"":                     "anon",
		"   ":                  "anon",
		"  wachin  ":           "wachin",
		"un-nombre-larguisimo": "un-nombre-largui", // truncated to maxNameLen
		"mal\x00formado":       "malformado",
	}
	for in, want := range cases {
		if got := sanitizeName(in); got != want {
			t.Errorf("sanitizeName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestHeadingDeltas(t *testing.T) {
	cases := []struct {
		h      protocol.Heading
		dx, dy int
	}{
		{protocol.North, 0, -1},
		{protocol.East, 1, 0},
		{protocol.South, 0, 1},
		{protocol.West, -1, 0},
	}
	for _, c := range cases {
		dx, dy := c.h.Delta()
		if dx != c.dx || dy != c.dy {
			t.Errorf("heading %d delta = (%d,%d), want (%d,%d)", c.h, dx, dy, c.dx, c.dy)
		}
	}
}

// The dead have to stay visible.
//
// They leave the collision index the moment they fall -- a corpse must not wall
// off a doorway -- so a viewport built by walking tiles would lose them unless
// something else keeps them findable. That something is the corpse index, and
// this is the test that says so: before it existed, switching viewportOf from
// scanning players to scanning tiles made every body on the map vanish.
func TestGhostsStayInTheViewport(t *testing.T) {
	w := newTestWorld(t, 40, 40)
	watcher, _ := place(t, w, "vivo", 10, 10)
	victim, _ := place(t, w, "muerto", 11, 10)

	w.kill(victim, watcher, "")

	if _, blocks := w.occupied[tileKey{11, 10}]; blocks {
		t.Error("el cadáver sigue bloqueando su tile")
	}

	var seen *protocol.EntityState
	for i, e := range w.viewportOf(watcher) {
		if e.ID == uint32(victim.ID) {
			seen = &w.viewportOf(watcher)[i]
			break
		}
	}
	if seen == nil {
		t.Fatal("el muerto desapareció del viewport")
	}
	if !seen.Dead {
		t.Error("el muerto viaja como vivo")
	}
}

// Coming back has to take the body out of the corpse index, or the player is
// drawn twice: once alive where they are, once dead where they fell.
func TestReviveClearsTheCorpseIndex(t *testing.T) {
	w := newTestWorld(t, 40, 40)
	watcher, _ := place(t, w, "vivo", 10, 10)
	victim, _ := place(t, w, "muerto", 11, 10)

	w.kill(victim, watcher, "")
	w.revive(victim, 12, 10)

	if len(w.corpses) != 0 {
		t.Errorf("quedaron %d tiles con cadáver después de revivir", len(w.corpses))
	}

	seen := 0
	for _, e := range w.viewportOf(watcher) {
		if e.ID == uint32(victim.ID) {
			seen++
		}
	}
	if seen != 1 {
		t.Errorf("el revivido aparece %d veces en el viewport, se esperaba 1", seen)
	}
}

// And leaving while dead must not leave the id behind: the corpse index would
// keep naming a player the world no longer has.
func TestLeavingWhileDeadClearsTheCorpseIndex(t *testing.T) {
	w := newTestWorld(t, 40, 40)
	watcher, _ := place(t, w, "vivo", 10, 10)
	victim, _ := place(t, w, "muerto", 11, 10)

	w.kill(victim, watcher, "")
	w.removePlayer(victim.ID)

	if len(w.corpses) != 0 {
		t.Errorf("quedaron %d tiles con cadáver después de que el jugador se fuera", len(w.corpses))
	}
	for _, e := range w.viewportOf(watcher) {
		if e.ID == uint32(victim.ID) {
			t.Error("el que se fue sigue en el viewport")
		}
	}
}

// Several bodies can share a tile, because they do not block. The living index
// holds one id per tile and would silently keep only the last.
func TestSeveralCorpsesShareATile(t *testing.T) {
	w := newTestWorld(t, 40, 40)
	watcher, _ := place(t, w, "vivo", 10, 10)

	var dead []*Player
	for i := 0; i < 3; i++ {
		p, _ := place(t, w, "muerto", 12, 10)
		w.kill(p, watcher, "")
		dead = append(dead, p)
	}

	seen := map[uint32]bool{}
	for _, e := range w.viewportOf(watcher) {
		seen[e.ID] = true
	}
	for _, p := range dead {
		if !seen[uint32(p.ID)] {
			t.Errorf("%d se perdió: los cadáveres se pisan entre sí en el tile", p.ID)
		}
	}
}
