package world

import (
	"math"

	"juegito/server/internal/protocol"
)

// The shrinking zone: the mechanic that turns a map into a match.
//
// It is a circle of safe ground that closes in stages. Each stage waits, then
// contracts toward a smaller circle drawn somewhere inside the current one, so
// where the fight ends up is different every match and nobody can camp a spot
// they picked before the round started. Standing outside costs health, and the
// cost climbs stage by stage: early on the ring is a nudge, late on it is fatal
// to ignore.
//
// The circle is deliberately not aligned to anything — it is a real circle over
// a tile grid, compared by squared distance so no square root runs per player
// per tick.

// Zone timings, in ticks. At 20 Hz a second is 20 ticks.
const (
	// zoneGraceTicks is how long the wall sits on the edges of the map before
	// it moves at all. A full minute: time to land somewhere, find a weapon and
	// work out where you are, with the ring visible along the borders the whole
	// time so nobody is surprised by it later.
	zoneGraceTicks = 60 * 20

	// zoneHoldTicks and zoneShrinkTicks are the *first* stage's durations. Every
	// stage after it is shorter — see stageDurations.
	zoneHoldTicks   = 50 * 20
	zoneShrinkTicks = 40 * 20
)

// zoneLateFactor is how much of the first stage's timing the last stage gets.
//
// The ring speeding up as the match goes on is what stops the endgame from
// dragging: early on the circle is enormous and crossing it is most of the
// work, so the walk-there window has to be generous. By the last stages the
// whole safe area fits on a couple of screens, and the same window would be
// two minutes of standing around waiting for a wall that has nowhere to go.
const zoneLateFactor = 0.35

// stageDurations is how long stage n waits and how long its contraction takes.
// Both shrink linearly from the full duration at stage 0 to zoneLateFactor of
// it at the last stage.
func (z *zone) stageDurations(stage int) (hold, shrink uint64) {
	t := 0.0
	if zoneStages > 1 {
		t = float64(stage) / float64(zoneStages-1)
		t = math.Min(math.Max(t, 0), 1)
	}
	scale := 1 + (zoneLateFactor-1)*t
	hold = uint64(float64(z.holdTicks) * scale)
	shrink = uint64(float64(z.shrinkTicks) * scale)
	if hold < 1 {
		hold = 1
	}
	if shrink < 1 {
		shrink = 1
	}
	return
}

// zoneStages is how many times the circle contracts. After the last one the
// zone holds at its final size rather than closing to a point: a circle you
// cannot stand in makes the last two players die to the ring instead of to
// each other.
const zoneStages = 12

// zoneShrinkFactor is how much of the radius survives each stage.
//
// Twelve stages of 0.778 take the starting 430-tile radius through roughly
// 334, 260, 202, 157, 122, 95, 74, 58, 45, 35, 27 and down to 21.
//
// Twelve small steps rather than six big ones: a contraction you can watch
// happen and walk away from beats a jump that relocates the safe area. The
// first is still the largest and is the one that brings the wall in from
// beyond the corners.
const zoneShrinkFactor = 0.778

// zoneFinalRadius is where the ring stops: 21 tiles of radius, so a little
// over 40 across. That is about two and a half screens of the classic 17-tile
// viewport — small enough to force a fight, big enough to have one in, with
// room to circle somebody rather than just bumping into them.
const zoneFinalRadius = 21.0

// zoneDamagePerSecond is what standing outside costs, by stage. The first
// entries are survivable for a long while; the last ones are not. Indexed by
// stage, clamped at the end.
// Nothing during the grace period, then a climb that starts as a nudge and
// ends as a death sentence. Twelve stages, so the last entries are what the
// final circles cost.
var zoneDamagePerSecond = [...]float64{0, 1, 2, 3, 4, 6, 8, 11, 14, 18, 23, 29, 36}

// zoneDamageIntervalTicks is how often the ring bites. Once a second rather
// than every tick, so the damage numbers a player sees are readable.
const zoneDamageIntervalTicks = 20

type zonePhase int

const (
	zoneIdle zonePhase = iota
	zoneWaiting
	zoneShrinking
	zoneDone
)

type zone struct {
	// armed is "configured and waiting for a player"; enabled is "running".
	armed   bool
	enabled bool
	phase   zonePhase
	stage   int

	// The circle that is safe right now.
	x, y, radius float64
	// The circle being moved toward. Only meaningful while waiting or
	// shrinking.
	nextX, nextY, nextRadius float64
	// Where the current contraction started, so progress interpolates from the
	// real starting circle rather than from wherever it happens to be.
	fromX, fromY, fromRadius float64

	// Where the last circle is meant to end up, drawn once when the match
	// starts. See pickDestination for why the ring aims instead of wandering.
	destX, destY float64

	phaseEndsAt uint64
	lastBiteAt  uint64

	// The three durations, in ticks. Held per world rather than read from the
	// constants so a test — or a playtest that does not want to wait ten
	// minutes to see the last circle — can run the whole cycle in seconds.
	graceTicks, holdTicks, shrinkTicks uint64
}

// ArmZone readies the shrinking zone. Called once, before Run.
//
// It does not start the clock: the ring begins when the first player joins, not
// when the process does. A server that boots an hour before anybody connects
// would otherwise have closed the circle to its final radius by the time the
// first person picked a character, which is exactly what it looked like — you
// spawned with the wall already in the middle of the map.
//
// speed scales every duration: 1 is the real pacing, 10 runs a whole match's
// worth of contractions in about a minute, which is the only practical way to
// watch the last stages without playing to them.
func (w *World) ArmZone(speed float64) {
	if speed <= 0 {
		speed = 1
	}
	scale := func(ticks uint64) uint64 {
		out := uint64(float64(ticks) / speed)
		if out < 1 {
			return 1
		}
		return out
	}
	w.zone.graceTicks = scale(zoneGraceTicks)
	w.zone.holdTicks = scale(zoneHoldTicks)
	w.zone.shrinkTicks = scale(zoneShrinkTicks)
	w.zone.armed = true
}

// startIfArmed begins the countdown the first time somebody is there to see it.
func (w *World) startIfArmed() {
	if w.zone.armed && !w.zone.enabled {
		w.startZone()
	}
}

// startZone sizes the first circle to cover the whole playable area.
//
// The playable area is not the grid: a composed world carries a ring of ocean
// that is blocked on every tile, and a circle sized to the grid would spend its
// first stages contracting over water nobody could have stood on anyway. The
// bounds of what is actually walkable are measured instead.
func (w *World) startZone() {
	minX, minY, maxX, maxY, any := w.walkableBounds()
	if !any {
		w.log.Warn("no hay terreno caminable, la zona queda apagada")
		return
	}

	cx := float64(minX+maxX) / 2
	cy := float64(minY+maxY) / 2
	// The circle reaches the corners: every walkable tile starts safe.
	//
	// The map is a square and the zone is a circle, so the two only agree at
	// four points. Sizing the circle to touch the middle of each side — which
	// looks right on paper — leaves the four corners outside from the first
	// second, and those corners are a lot of ground: about 21% of a square.
	// Starting a match with a fifth of the map already lit up blue reads as
	// "the zone is half closed", not as "the zone has not started".
	//
	// So the radius goes to the furthest corner instead. Nothing walkable is
	// outside at the start, and the first contraction — the longest of the six
	// — is what sweeps the wall in from beyond the edges and across the map.
	dx := float64(maxX) - cx
	dy := float64(maxY) - cy
	grace, hold, shrink := w.zone.graceTicks, w.zone.holdTicks, w.zone.shrinkTicks
	if grace == 0 {
		grace, hold, shrink = zoneGraceTicks, zoneHoldTicks, zoneShrinkTicks
	}
	destX, destY := w.pickDestination()

	w.zone = zone{
		// Kept, not dropped: armed means "this world is configured to have a
		// ring", which stays true for as long as the process lives. Losing it
		// here made a restarted match silently zoneless, and made startIfArmed
		// a one-shot despite its name.
		armed:       true,
		enabled:     true,
		phase:       zoneWaiting,
		x:           cx,
		y:           cy,
		radius:      math.Hypot(dx, dy),
		phaseEndsAt: w.tick + grace,
		graceTicks:  grace,
		holdTicks:   hold,
		shrinkTicks: shrink,
		destX:       destX,
		destY:       destY,
	}
	w.planNextCircle()
	w.log.Info("zona activa",
		"centro", [2]int{int(cx), int(cy)}, "radio", int(w.zone.radius),
		"destino", [2]int{int(destX), int(destY)},
		"etapas", zoneStages, "gracia_s", grace/uint64(w.tickRate))
}

// walkableBounds is the bounding box of every tile a player could stand on.
func (w *World) walkableBounds() (minX, minY, maxX, maxY int, any bool) {
	minX, minY = w.grid.W, w.grid.H
	for y := 0; y < w.grid.H; y++ {
		for x := 0; x < w.grid.W; x++ {
			if w.grid.Blocked(x, y) {
				continue
			}
			any = true
			if x < minX {
				minX = x
			}
			if x > maxX {
				maxX = x
			}
			if y < minY {
				minY = y
			}
			if y > maxY {
				maxY = y
			}
		}
	}
	return
}

// pickDestination chooses where the last circle is meant to end up.
//
// The ring used to have no destination at all: every stage stepped off in a
// random direction, bounded by how much the radius had shrunk. That is a random
// walk, and a random walk starting at the centre of the map *stays near the
// centre of the map* — the steps cancel out. Twelve stages of it typically
// drifted about 150 tiles on a 760-tile world, so the endgame was always
// roughly the middle and the map's own places never got their turn.
//
// Drawing the endpoint up front and aiming at it makes the last arena land
// anywhere: a corner, a forest, the edge of the water. Where the fight happens
// becomes something the match decides, which is the whole reason the circle
// moves at all.
//
// The candidate has to be somewhere a fight can actually happen, so it is
// scored on how much of the final arena is walkable ground rather than just on
// being walkable itself: the centre of a lake passes the naive test and gives
// the last two players a puddle.
func (w *World) pickDestination() (float64, float64) {
	minX, minY, maxX, maxY, any := w.walkableBounds()
	cx, cy := float64(minX+maxX)/2, float64(minY+maxY)/2
	if !any {
		return cx, cy
	}

	const attempts = 60
	const wantWalkable = 0.7

	bestX, bestY, best := cx, cy, -1.0
	for i := 0; i < attempts; i++ {
		x := float64(minX) + w.rng.Float64()*float64(maxX-minX)
		y := float64(minY) + w.rng.Float64()*float64(maxY-minY)
		score := w.walkableFraction(x, y, zoneFinalRadius)
		if score > best {
			bestX, bestY, best = x, y, score
		}
		if score >= wantWalkable {
			return x, y
		}
	}
	// Nothing cleared the bar: take the best seen rather than falling back to
	// the centre, which is the one answer this whole function exists to avoid.
	return bestX, bestY
}

// walkableFraction is how much of the disc around a point is standable, on a
// coarse sample. Coarse on purpose: this runs sixty times at match start and
// the answer only has to tell a field from a lake.
func (w *World) walkableFraction(cx, cy, radius float64) float64 {
	const step = 3.0
	total, free := 0, 0
	for dy := -radius; dy <= radius; dy += step {
		for dx := -radius; dx <= radius; dx += step {
			if dx*dx+dy*dy > radius*radius {
				continue
			}
			x, y := int(cx+dx), int(cy+dy)
			if x < 0 || y < 0 || x >= w.grid.W || y >= w.grid.H {
				total++
				continue
			}
			total++
			if !w.grid.Blocked(x, y) {
				free++
			}
		}
	}
	if total == 0 {
		return 0
	}
	return float64(free) / float64(total)
}

// planNextCircle draws the circle for the coming stage: smaller, and centred
// inside the current one so it never leaves the safe ground it is contracting
// from.
func (w *World) planNextCircle() {
	z := &w.zone
	if z.stage >= zoneStages || z.radius <= zoneFinalRadius {
		z.phase = zoneDone
		z.nextRadius = 0
		return
	}

	next := z.radius * zoneShrinkFactor
	if next < zoneFinalRadius {
		next = zoneFinalRadius
	}

	// slack is how far the centre may move: exactly the difference in radii,
	// which is the condition for the smaller circle to sit wholly inside the
	// larger one. A circle that poked outside would strand players who had
	// correctly walked into the safe area, so this bound is not negotiable and
	// everything below has to fit inside it.
	slack := z.radius - next

	// Aim at the destination, going as far as the slack allows. Early stages
	// have the most slack and cover the most ground, which is also when the
	// circle is huge and moving it costs the players the least.
	dx, dy := z.destX-z.x, z.destY-z.y
	dist := math.Hypot(dx, dy)
	move := math.Min(dist, slack)
	if dist > 0 {
		dx, dy = dx/dist*move, dy/dist*move
	} else {
		dx, dy = 0, 0
	}

	// Whatever slack is left over is spent on a random wobble, so the path is
	// not a straight line anyone can extrapolate two stages ahead. It is only
	// ever the remainder, so aiming always wins over wandering — the opposite
	// of the old behaviour, where wandering was all there was.
	if spare := slack - move; spare > 0 {
		angle := w.rng.Float64() * 2 * math.Pi
		wobble := spare * math.Sqrt(w.rng.Float64())
		dx += math.Cos(angle) * wobble
		dy += math.Sin(angle) * wobble
	}

	z.nextX = z.x + dx
	z.nextY = z.y + dy
	z.nextRadius = next
}

// zoneTick advances the zone and bills whoever is outside it.
func (w *World) zoneTick() {
	z := &w.zone
	if !z.enabled || z.phase == zoneIdle {
		return
	}

	switch z.phase {
	case zoneWaiting:
		if w.tick >= z.phaseEndsAt {
			z.fromX, z.fromY, z.fromRadius = z.x, z.y, z.radius
			_, shrink := z.stageDurations(z.stage)
			z.phase = zoneShrinking
			z.phaseEndsAt = w.tick + shrink
		}
	case zoneShrinking:
		// Nothing to contract toward means the ring is finished. Without this
		// the interpolation below would read a zero target radius and close the
		// circle to a point, killing whoever was correctly standing in it.
		if z.nextRadius <= 0 {
			z.phase = zoneDone
			return
		}
		_, shrink := z.stageDurations(z.stage)
		remaining := int64(z.phaseEndsAt) - int64(w.tick)
		progress := 1.0
		if remaining > 0 && shrink > 0 {
			progress = 1 - float64(remaining)/float64(shrink)
		}
		z.x = z.fromX + (z.nextX-z.fromX)*progress
		z.y = z.fromY + (z.nextY-z.fromY)*progress
		z.radius = z.fromRadius + (z.nextRadius-z.fromRadius)*progress

		if w.tick >= z.phaseEndsAt {
			z.x, z.y, z.radius = z.nextX, z.nextY, z.nextRadius
			z.stage++
			hold, _ := z.stageDurations(z.stage)
			z.phase = zoneWaiting
			z.phaseEndsAt = w.tick + hold
			w.planNextCircle()
			if z.phase != zoneDone {
				w.log.Info("zona cerrando", "etapa", z.stage, "radio", int(z.radius))
			} else {
				w.log.Info("zona en su tamaño final", "radio", int(z.radius))
			}
		}
	}

	// The grace period is genuinely safe. The starting circle already leaves
	// the corners outside, so billing during it would kill whoever happened to
	// spawn in one before they had any way to know the ring existed.
	if z.stage == 0 && z.phase == zoneWaiting {
		z.lastBiteAt = w.tick
		return
	}

	if w.tick-z.lastBiteAt >= zoneDamageIntervalTicks {
		z.lastBiteAt = w.tick
		w.biteOutsiders()
	}
}

// biteOutsiders damages everyone standing outside the circle.
func (w *World) biteOutsiders() {
	z := &w.zone
	dps := zoneDamagePerSecond[len(zoneDamagePerSecond)-1]
	if z.stage < len(zoneDamagePerSecond) {
		dps = zoneDamagePerSecond[z.stage]
	}
	damage := int(math.Round(dps))
	if damage <= 0 {
		return
	}

	r2 := z.radius * z.radius
	for _, p := range w.players {
		if p.Dead {
			continue
		}
		dx := float64(p.X) - z.x
		dy := float64(p.Y) - z.y
		if dx*dx+dy*dy <= r2 {
			continue
		}
		w.applyZoneDamage(p, damage)
	}
}

// applyZoneDamage takes health and kills, without an attacker.
//
// Death by the ring is a death like any other — it drops what you were
// carrying and starts the respawn clock — so it goes through the same path a
// killing blow does rather than quietly zeroing the vitals.
func (w *World) applyZoneDamage(p *Player, damage int) {
	p.Vitals.HP -= damage
	if p.Vitals.HP > 0 {
		w.sendTo(p, protocol.TypeCombat, protocol.CombatEvent{
			VictimID:   uint32(p.ID),
			VictimName: p.Name,
			Damage:     damage,
			Hit:        true,
			Zone:       true,
			Mine:       false,
		})
		return
	}

	p.Vitals.HP = 0
	w.kill(p, nil, protocol.CauseZone)
	w.sendTo(p, protocol.TypeCombat, protocol.CombatEvent{
		VictimID:   uint32(p.ID),
		VictimName: p.Name,
		Damage:     damage,
		Hit:        true,
		Killed:     true,
		Zone:       true,
	})
}

// zoneState is what the client needs to draw the ring and count down to the
// next contraction.
func (w *World) zoneState() *protocol.Zone {
	z := &w.zone
	if !z.enabled || z.phase == zoneIdle {
		return nil
	}
	out := &protocol.Zone{
		X:         z.x,
		Y:         z.y,
		Radius:    z.radius,
		Stage:     z.stage,
		Shrinking: z.phase == zoneShrinking,
	}
	if z.phase != zoneDone {
		out.NextX, out.NextY, out.NextRadius = z.nextX, z.nextY, z.nextRadius
		if z.phaseEndsAt > w.tick {
			out.Seconds = float64(z.phaseEndsAt-w.tick) / float64(w.tickRate)
		}
	}
	return out
}
