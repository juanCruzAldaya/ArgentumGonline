package world

import (
	"math"
	"math/rand"
	"testing"

	"juegito/server/internal/protocol"
)

// matchWorld is a world with a match running and enough item table behind it
// that a restart has something to scatter.
func matchWorld(t *testing.T) *World {
	t.Helper()
	w := newTestWorld(t, 60, 60)
	w.rng = rand.New(rand.NewSource(1))
	w.SetItems(lootItems())
	return w
}

// outcomeOf reads the last outcome card a connection was sent.
func outcomeOf(t *testing.T, w *World, conn *fakeConn) protocol.Outcome {
	t.Helper()
	var out protocol.Outcome
	if err := w.codec.DecodePayload(conn.lastOfType(t, protocol.TypeOutcome), &out); err != nil {
		t.Fatalf("decode outcome: %v", err)
	}
	return out
}

// The whole point: somebody wins.
func TestMatchEndsWithTheLastPlayerStanding(t *testing.T) {
	w := matchWorld(t)
	a, connA := place(t, w, "a", 5, 5)
	b, _ := place(t, w, "b", 7, 7)
	c, _ := place(t, w, "c", 9, 9)

	w.kill(b, a, "")
	w.kill(c, a, "")
	w.matchTick()

	if w.match.phase != matchOver {
		t.Fatalf("fase = %v, se esperaba matchOver", w.match.phase)
	}
	if w.match.winnerID != a.ID {
		t.Errorf("ganador = %d, se esperaba %d", w.match.winnerID, a.ID)
	}

	out := outcomeOf(t, w, connA)
	if !out.Won {
		t.Error("el ganador no recibió Won")
	}
	if out.Placement != 1 || out.Players != 3 {
		t.Errorf("puesto %d de %d, se esperaba 1 de 3", out.Placement, out.Players)
	}
	if out.Kills != 2 {
		t.Errorf("bajas = %d, se esperaban 2", out.Kills)
	}
}

// A match that only ever held one person cannot be won by them. Without this,
// a lone player on a test server is the last one alive from the first tick.
func TestOnePlayerNeverEndsTheMatch(t *testing.T) {
	w := matchWorld(t)
	solo, conn := place(t, w, "solo", 5, 5)

	for i := 0; i < 5; i++ {
		w.matchTick()
	}
	if w.match.phase != matchRunning {
		t.Fatalf("fase = %v con un solo jugador vivo, se esperaba matchRunning", w.match.phase)
	}

	// Not even when they die: nobody is left to have beaten them.
	w.kill(solo, nil, "")
	w.matchTick()
	if w.match.phase != matchRunning {
		t.Errorf("fase = %v, una partida de uno no se decide", w.match.phase)
	}
	if hasType(t, conn, protocol.TypeOutcome) {
		t.Error("se mandó un resultado en una partida que nunca tuvo dos jugadores")
	}
}

// Placement is decided when you die, counting the dead player: last of five is
// 5th. Getting this off by one is the difference between "you came 4th" and a
// player who watched four others still standing.
func TestPlacementIsFixedAtTheMomentOfDeath(t *testing.T) {
	w := matchWorld(t)
	a, _ := place(t, w, "a", 5, 5)
	b, _ := place(t, w, "b", 6, 6)
	c, _ := place(t, w, "c", 7, 7)
	d, _ := place(t, w, "d", 8, 8)

	w.kill(d, a, "")
	w.kill(c, a, "")
	w.kill(b, a, "")
	w.matchTick()

	for _, want := range []struct {
		p     *Player
		place int
	}{{d, 4}, {c, 3}, {b, 2}, {a, 1}} {
		if want.p.Placement != want.place {
			t.Errorf("%s quedó %d, se esperaba %d", want.p.Name, want.p.Placement, want.place)
		}
	}
}

// An eliminated player is told immediately rather than at the end of the
// match: the half that is already decided is theirs to see while they are
// looking at their own corpse.
func TestEliminationTellsThePlayerBeforeTheMatchIsDecided(t *testing.T) {
	w := matchWorld(t)
	a, _ := place(t, w, "a", 5, 5)
	victim, conn := place(t, w, "victim", 6, 6)
	place(t, w, "c", 7, 7)

	w.tick = 200 // ten seconds in, at 20 Hz
	w.kill(victim, a, "")

	out := outcomeOf(t, w, conn)
	if out.Placement != 3 {
		t.Errorf("puesto = %d, se esperaba 3", out.Placement)
	}
	if out.Winner != "" || out.Won {
		t.Errorf("la carta de eliminación no puede nombrar un ganador: %+v", out)
	}
	if out.Seconds != 10 {
		t.Errorf("sobrevivió %.1fs, se esperaban 10", out.Seconds)
	}
	if w.match.phase != matchRunning {
		t.Error("la partida terminó con dos vivos")
	}
}

// Time survived counts from your own join. Somebody who connected nine minutes
// into a match did not survive nine minutes.
func TestSecondsSurvivedCountsFromTheOwnJoin(t *testing.T) {
	w := matchWorld(t)
	place(t, w, "early", 5, 5)

	w.tick = 1200 // a minute into the match
	late, conn := place(t, w, "late", 6, 6)
	w.tick = 1400 // ten seconds later

	w.kill(late, nil, "")
	if out := outcomeOf(t, w, conn); out.Seconds != 10 {
		t.Errorf("sobrevivió %.1fs, se esperaban 10 desde que entró", out.Seconds)
	}
}

// With -respawn on, a death is not an elimination, so the match cannot be
// decided by one. This is what keeps the playtest affordance from silently
// ending every match at the first kill.
func TestRespawnKeepsTheMatchRunning(t *testing.T) {
	w := matchWorld(t)
	w.SetRespawnDelay(1)
	a, _ := place(t, w, "a", 5, 5)
	b, _ := place(t, w, "b", 6, 6)

	w.kill(b, a, "")

	// The window that used to end the match: a full second where the ghost is
	// down and the count really does say one. Nothing may be decided in it.
	for i := 0; i < 20; i++ {
		w.step()
		if w.match.phase != matchRunning {
			t.Fatalf("la partida se decidió en el tick %d de un muerto que vuelve", i)
		}
	}
	w.step()
	if b.Dead {
		t.Fatal("el muerto no volvió: el test ya no prueba lo que dice")
	}
	if w.match.phase != matchRunning {
		t.Errorf("fase = %v, se esperaba matchRunning", w.match.phase)
	}
}

// The ring can take the last two inside the same second. Nobody won that.
func TestNobodyWinsWhenTheLastTwoDieTogether(t *testing.T) {
	w := matchWorld(t)
	a, connA := place(t, w, "a", 5, 5)
	b, _ := place(t, w, "b", 6, 6)

	w.kill(a, nil, "")
	w.kill(b, nil, "")
	w.matchTick()

	if w.match.phase != matchOver {
		t.Fatalf("fase = %v, se esperaba matchOver", w.match.phase)
	}
	if w.match.winnerName != "" {
		t.Errorf("ganador = %q, no debería haber", w.match.winnerName)
	}
	if out := outcomeOf(t, w, connA); out.Won {
		t.Error("alguien ganó una partida que no ganó nadie")
	}
}

// A decided match waits the configured time before the next one, so the card
// can be read.
func TestRestartWaitsItsTime(t *testing.T) {
	w := matchWorld(t)
	w.SetMatchRestart(3) // 60 ticks at 20 Hz
	a, _ := place(t, w, "a", 5, 5)
	b, _ := place(t, w, "b", 6, 6)

	w.kill(b, a, "")
	w.matchTick()
	if w.match.phase != matchOver {
		t.Fatalf("fase = %v, se esperaba matchOver", w.match.phase)
	}

	w.tick += 59
	w.matchTick()
	if w.match.phase != matchOver {
		t.Error("la partida se reinició antes de tiempo")
	}

	w.tick++
	w.matchTick()
	if w.match.phase != matchRunning {
		t.Errorf("fase = %v, se esperaba que arrancara la siguiente", w.match.phase)
	}
}

// Zero means the finished match stays finished, which is what a server hosting
// one match wants.
func TestRestartOffLeavesTheMatchStanding(t *testing.T) {
	w := matchWorld(t)
	a, _ := place(t, w, "a", 5, 5)
	b, _ := place(t, w, "b", 6, 6)

	w.kill(b, a, "")
	w.matchTick()
	w.tick += 10000
	w.matchTick()

	if w.match.phase != matchOver {
		t.Errorf("fase = %v, sin -match-restart la partida no vuelve a arrancar", w.match.phase)
	}
}

// A restart is a new match on the same world: everyone alive, scores back to
// zero, and a floor that was laid again rather than topped up.
func TestRestartPutsEveryoneBackAndRelaysTheFloor(t *testing.T) {
	w := matchWorld(t)
	w.SetMatchRestart(1)
	a, connA := place(t, w, "a", 5, 5)
	b, _ := place(t, w, "b", 6, 6)

	// Empty the floor so the reseed is the only thing that could refill it.
	clear(w.ground)

	w.kill(b, a, "")
	w.matchTick()
	w.tick += 20
	w.matchTick()

	if w.match.phase != matchRunning {
		t.Fatalf("fase = %v, se esperaba matchRunning", w.match.phase)
	}
	for _, p := range []*Player{a, b} {
		if p.Dead {
			t.Errorf("%s sigue muerto después del reinicio", p.Name)
		}
		if p.Kills != 0 || p.Placement != 0 || p.diedAt != 0 {
			t.Errorf("%s arrastra estado de la partida anterior: bajas=%d puesto=%d muerto_en=%d",
				p.Name, p.Kills, p.Placement, p.diedAt)
		}
		if p.joinedAt != w.tick {
			t.Errorf("%s no reinició su reloj de supervivencia", p.Name)
		}
	}
	if len(w.ground) == 0 {
		t.Error("el piso quedó vacío: el loot no se volvió a sembrar")
	}

	// The client is told where it now stands, through the same message a join
	// uses — otherwise it keeps predicting from the tile it held last match.
	var welcome protocol.Welcome
	if err := w.codec.DecodePayload(connA.lastOfType(t, protocol.TypeWelcome), &welcome); err != nil {
		t.Fatalf("decode welcome: %v", err)
	}
	if welcome.SpawnX != a.X || welcome.SpawnY != a.Y {
		t.Errorf("el Welcome del reinicio dice %d,%d y el jugador está en %d,%d",
			welcome.SpawnX, welcome.SpawnY, a.X, a.Y)
	}
}

// Two players cannot come back on the same tile, and the tile somebody is
// about to leave must not be treated as taken.
func TestRestartGivesEveryoneTheirOwnTile(t *testing.T) {
	w := matchWorld(t)
	w.SetMatchRestart(1)
	a, _ := place(t, w, "a", 5, 5)
	b, _ := place(t, w, "b", 6, 6)
	c, _ := place(t, w, "c", 7, 7)

	w.kill(b, a, "")
	w.kill(c, a, "")
	w.matchTick()
	w.tick += 20
	w.matchTick()

	seen := map[tileKey]string{}
	for _, p := range []*Player{a, b, c} {
		key := tileKey{p.X, p.Y}
		if other, taken := seen[key]; taken {
			t.Fatalf("%s y %s comparten el tile %v", other, p.Name, key)
		}
		seen[key] = p.Name
		if id, ok := w.occupied[key]; !ok || id != p.ID {
			t.Errorf("%s no quedó indexado en su tile %v", p.Name, key)
		}
	}
	if len(w.occupied) != 3 {
		t.Errorf("occupied tiene %d entradas para 3 jugadores: quedaron tiles fantasma", len(w.occupied))
	}
}

// The ring has to start over too. It used to drop its own armed flag when it
// started, which left every match after the first without a zone at all.
func TestRestartStartsTheZoneOver(t *testing.T) {
	w := matchWorld(t)
	w.SetMatchRestart(1)
	w.ArmZone(1)
	a, _ := place(t, w, "a", 5, 5)
	b, _ := place(t, w, "b", 6, 6)

	if !w.zone.enabled {
		t.Fatal("la zona no arrancó con el primer jugador")
	}
	w.zone.stage = 7
	firstRadius := w.zone.radius
	w.zone.radius = 3

	w.kill(b, a, "")
	w.matchTick()
	w.tick += 20
	w.matchTick()

	if !w.zone.armed || !w.zone.enabled {
		t.Fatalf("la zona quedó apagada tras el reinicio: armed=%v enabled=%v", w.zone.armed, w.zone.enabled)
	}
	if w.zone.stage != 0 {
		t.Errorf("etapa = %d, se esperaba 0", w.zone.stage)
	}
	if w.zone.radius != firstRadius {
		t.Errorf("radio = %.1f, se esperaba el inicial %.1f", w.zone.radius, firstRadius)
	}
}

// Somebody arriving with the ring already closed has to land inside it.
//
// A uniformly random tile on a 760-map is outside a radius-44 circle 99,7% of
// the time, and outside is not a disadvantage this late -- it is a death with
// no turn taken, because the walk back is longer than the health bar.
func TestLateJoinSpawnsInsideTheZone(t *testing.T) {
	w := newTestWorld(t, 400, 400)
	w.rng = rand.New(rand.NewSource(7))
	w.ArmZone(1)
	w.startIfArmed()

	// Wind the ring down to a small circle off in one corner, so "inside" and
	// "anywhere" cannot be confused for each other.
	w.zone.x, w.zone.y, w.zone.radius = 90, 70, 25

	for i := 0; i < 40; i++ {
		// place() moves the player itself, and what is under test is where the
		// world chose to put them -- so the spawn is read off the Welcome,
		// which is also the number the client predicts from.
		_, conn := place(t, w, "tarde", 0, 0)
		var welcome protocol.Welcome
		if err := w.codec.DecodePayload(conn.lastOfType(t, protocol.TypeWelcome), &welcome); err != nil {
			t.Fatalf("decode welcome: %v", err)
		}
		x, y := welcome.SpawnX, welcome.SpawnY
		dx, dy := float64(x)-w.zone.x, float64(y)-w.zone.y
		if math.Hypot(dx, dy) > w.zone.radius {
			t.Fatalf("spawn en (%d,%d) a %.1f tiles del centro, fuera del radio %.0f",
				x, y, math.Hypot(dx, dy), w.zone.radius)
		}
	}
}

// And the same rule must not follow a restart: a new match spreads everybody
// over the map, not into the arena the last one ended in.
func TestRestartSpreadsBeyondTheOldFinalCircle(t *testing.T) {
	w := matchWorld(t)
	w.SetMatchRestart(1)
	w.ArmZone(1)
	a, _ := place(t, w, "a", 5, 5)
	b, _ := place(t, w, "b", 6, 6)

	// End the match with the ring squeezed into a corner.
	w.zone.x, w.zone.y, w.zone.radius = 10, 10, 4
	w.kill(b, a, "")
	w.matchTick()
	w.tick += 20
	w.matchTick()

	if w.zone.radius <= 4 {
		t.Fatalf("la zona no volvió a abrirse: radio %.1f", w.zone.radius)
	}
	for _, p := range []*Player{a, b} {
		if math.Hypot(float64(p.X)-10, float64(p.Y)-10) <= 4 {
			t.Errorf("%s reapareció dentro del círculo final de la partida anterior, en (%d,%d)",
				p.Name, p.X, p.Y)
		}
	}
}
