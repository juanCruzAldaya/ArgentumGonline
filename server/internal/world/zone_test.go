package world

import (
	"juegito/server/internal/protocol"
	"math"
	"testing"
)

// zoneWorld is a world with the ring running.
//
// The same 760x760 a composed world is, not the 40x40 the other suites use.
//
// The size is load-bearing twice over. The final radius is an absolute number
// of tiles — tuned so the last circle is an arena you can fight in — so on a
// small map the ring is already at its floor before the first contraction. And
// how many stages actually run depends on how far the starting radius is from
// that floor, which means a smaller test map quietly finishes early and the
// pacing assertions stop meaning anything.
func zoneWorld(t *testing.T) *World {
	t.Helper()
	w := newTestWorld(t, 760, 760)
	w.SetItems(map[int]Item{
		38: {ID: 38, Name: "Poción Roja", Type: ItemPotion},
	})
	w.tick = 1000
	w.ArmZone(1)
	w.startIfArmed()
	if !w.zone.enabled {
		t.Fatal("la zona no arrancó")
	}
	return w
}

// Every walkable tile starts inside. A circle that only touches the middle of
// each side leaves the corners out, and on a square map that is a fifth of the
// ground lit up blue before the match has started.
func TestZoneStartsCoveringEveryWalkableTile(t *testing.T) {
	w := zoneWorld(t)
	z := &w.zone

	outside := 0
	for y := 0; y < w.grid.H; y++ {
		for x := 0; x < w.grid.W; x++ {
			if w.grid.Blocked(x, y) {
				continue
			}
			if math.Hypot(float64(x)-z.x, float64(y)-z.y) > z.radius {
				outside++
			}
		}
	}
	if outside > 0 {
		t.Errorf("%d tiles caminables arrancaron fuera del círculo", outside)
	}
}

// And it gets there in the number of stages the pacing is built around, rather
// than bottoming out early or still being huge at the end.
func TestZoneReachesItsFinalSize(t *testing.T) {
	w := zoneWorld(t)
	// Run until it finishes rather than to a computed deadline: each phase
	// transition costs a tick of its own, and a test that has to model that is
	// testing the loop instead of the pacing. The cap is a generous multiple of
	// what the durations add up to, so a zone that stalls still fails.
	cap := w.zone.graceTicks
	for stage := 0; stage < zoneStages; stage++ {
		hold, shrink := w.zone.stageDurations(stage)
		cap += hold + shrink
	}
	cap = w.tick + cap*2
	for w.tick < cap && w.zone.stage < zoneStages {
		w.tick++
		w.zoneTick()
	}
	if w.zone.stage != zoneStages {
		t.Errorf("quedó en la etapa %d de %d", w.zone.stage, zoneStages)
	}
	if r := w.zone.radius; r > zoneFinalRadius*1.5 || r < zoneFinalRadius*0.5 {
		t.Errorf("radio final %.1f, esperaba algo cerca de %.1f", r, zoneFinalRadius)
	}
}

// The grace period is genuinely safe, corners included. Anything else kills
// whoever spawned in one before they knew the ring existed.
func TestGracePeriodDoesNoDamage(t *testing.T) {
	w := zoneWorld(t)
	z := &w.zone
	z.x, z.y, z.radius = 40, 40, 5

	victim, _ := place(t, w, "en la esquina", 40, 60)
	before := victim.Vitals.HP

	for i := 0; i < zoneDamageIntervalTicks*3; i++ {
		w.tick++
		w.zoneTick()
	}
	if victim.Vitals.HP != before {
		t.Errorf("la zona lastimó durante la gracia: %d -> %d", before, victim.Vitals.HP)
	}
}

// A smaller circle that poked outside the current one would strand players who
// had correctly walked into the safe area.
func TestNextCircleAlwaysFitsInsideTheCurrentOne(t *testing.T) {
	w := zoneWorld(t)

	for stage := 0; stage < zoneStages; stage++ {
		z := &w.zone
		if z.nextRadius <= 0 {
			break
		}
		gap := math.Hypot(z.nextX-z.x, z.nextY-z.y)
		if gap+z.nextRadius > z.radius+1e-9 {
			t.Fatalf("etapa %d: el círculo siguiente se sale (centro a %.2f, radio %.2f, actual %.2f)",
				stage, gap, z.nextRadius, z.radius)
		}
		// Jump straight to the end of this stage.
		z.phase = zoneShrinking
		z.fromX, z.fromY, z.fromRadius = z.x, z.y, z.radius
		z.phaseEndsAt = w.tick
		w.zoneTick()
	}
}

func TestZoneLandsExactlyOnTheNextCircle(t *testing.T) {
	w := zoneWorld(t)
	z := &w.zone
	wantX, wantY, wantR := z.nextX, z.nextY, z.nextRadius

	w.tick = z.phaseEndsAt
	w.zoneTick() // waiting -> shrinking
	w.tick += z.shrinkTicks
	w.zoneTick() // shrinking -> waiting, snapped to the planned circle

	if z.x != wantX || z.y != wantY || z.radius != wantR {
		t.Errorf("terminó en (%.3f,%.3f) r=%.3f, esperaba (%.3f,%.3f) r=%.3f",
			z.x, z.y, z.radius, wantX, wantY, wantR)
	}
	if z.stage != 1 {
		t.Errorf("etapa = %d, esperaba 1", z.stage)
	}
}

func TestZoneHurtsOnlyWhoIsOutside(t *testing.T) {
	w := zoneWorld(t)
	z := &w.zone
	// A circle small enough that the two players below sit on either side of
	// its edge, without waiting for eight contractions.
	z.x, z.y, z.radius = 40, 40, 5

	inside, _ := place(t, w, "adentro", 40, 42)
	outside, _ := place(t, w, "afuera", 40, 60)
	inHP, outHP := inside.Vitals.HP, outside.Vitals.HP

	z.stage = 1 // past the grace period, where the ring starts billing
	z.lastBiteAt = 0
	w.tick += zoneDamageIntervalTicks
	w.zoneTick()

	if inside.Vitals.HP != inHP {
		t.Errorf("el de adentro perdió vida: %d -> %d", inHP, inside.Vitals.HP)
	}
	if outside.Vitals.HP >= outHP {
		t.Errorf("el de afuera no perdió vida: %d -> %d", outHP, outside.Vitals.HP)
	}
}

// The ring bites on its own interval, not every tick, so the numbers stay
// readable and the damage matches the per-second figures.
func TestZoneBitesOnItsInterval(t *testing.T) {
	w := zoneWorld(t)
	z := &w.zone
	z.x, z.y, z.radius = 40, 40, 5

	victim, _ := place(t, w, "afuera", 40, 60)
	z.stage = 1 // past the grace period
	z.lastBiteAt = w.tick
	before := victim.Vitals.HP

	for i := 0; i < zoneDamageIntervalTicks-1; i++ {
		w.tick++
		w.zoneTick()
	}
	if victim.Vitals.HP != before {
		t.Errorf("mordió antes de tiempo: %d -> %d", before, victim.Vitals.HP)
	}

	w.tick++
	w.zoneTick()
	if victim.Vitals.HP == before {
		t.Error("no mordió al cumplirse el intervalo")
	}
}

// Dying to the ring has to be a death like any other: it drops what you were
// carrying and starts the respawn clock, rather than quietly zeroing the bar.
func TestZoneCanKill(t *testing.T) {
	w := zoneWorld(t)
	z := &w.zone
	z.x, z.y, z.radius = 40, 40, 5
	z.stage = len(zoneDamagePerSecond) - 1 // the deadliest tier

	victim, _ := place(t, w, "afuera", 40, 60)
	victim.Vitals.HP = 1
	z.phase = zoneShrinking
	victim.Inventory = []protocol.InventorySlot{{Slot: 0, ItemID: 38, Amount: 3}}

	z.lastBiteAt = 0
	w.tick += zoneDamageIntervalTicks
	w.zoneTick()

	if !victim.Dead {
		t.Fatalf("la zona no lo mató, quedó en %d de vida", victim.Vitals.HP)
	}
	if victim.Vitals.HP != 0 {
		t.Errorf("murió con %d de vida en vez de 0", victim.Vitals.HP)
	}
	// Not "the ground is non-empty" — SetItems scatters loot across the map at
	// load, so that would pass without the corpse dropping anything. What has
	// to be true is that this player's own stack is now lying somewhere.
	found := false
	for _, stack := range w.ground {
		if stack.ItemID == 38 && stack.Amount == 3 {
			found = true
			break
		}
	}
	if !found {
		t.Error("no soltó lo que llevaba al morir por la zona")
	}
}

// Each stage is quicker than the one before it, and costs more to sit out.
func TestLaterStagesAreFasterAndHurtMore(t *testing.T) {
	w := zoneWorld(t)

	firstHold, firstShrink := w.zone.stageDurations(0)
	lastHold, lastShrink := w.zone.stageDurations(zoneStages - 1)
	if lastHold >= firstHold || lastShrink >= firstShrink {
		t.Errorf("las etapas no se aceleran: primera %d/%d, última %d/%d",
			firstHold, firstShrink, lastHold, lastShrink)
	}

	for i := 1; i < len(zoneDamagePerSecond); i++ {
		if zoneDamagePerSecond[i] <= zoneDamagePerSecond[i-1] {
			t.Errorf("el daño no sube en la etapa %d: %.0f tras %.0f",
				i, zoneDamagePerSecond[i], zoneDamagePerSecond[i-1])
		}
	}
	if len(zoneDamagePerSecond) <= zoneStages {
		t.Errorf("la tabla de daño tiene %d entradas para %d etapas: las últimas quedan aplanadas",
			len(zoneDamagePerSecond), zoneStages)
	}
}

func TestZoneStateStopsOfferingANextCircleAtTheEnd(t *testing.T) {
	w := zoneWorld(t)

	// Driven by the clock rather than by poking the phase, so this exercises
	// the transitions the way a match does.
	deadline := w.tick + w.zone.graceTicks
	for stage := 0; stage < zoneStages+2; stage++ {
		hold, shrink := w.zone.stageDurations(stage)
		deadline += hold + shrink
	}
	for w.tick < deadline {
		w.tick++
		w.zoneTick()
	}

	state := w.zoneState()
	if state == nil {
		t.Fatal("la zona dejó de reportarse")
	}
	if state.NextRadius != 0 {
		t.Errorf("sigue ofreciendo un círculo siguiente de radio %.2f", state.NextRadius)
	}
	if state.Radius < zoneFinalRadius-1e-9 {
		t.Errorf("el radio final quedó en %.2f, por debajo del piso de %.2f", state.Radius, zoneFinalRadius)
	}
}
