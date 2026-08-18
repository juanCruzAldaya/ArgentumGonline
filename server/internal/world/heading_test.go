package world

import (
	"testing"

	"juegito/server/internal/protocol"
)

// A step refused mid-cadence does not take the turn down with it: the source
// meters turning separately (INT_CHANGE_HEADING), and only walking is on the
// walk cadence. What must not happen is the step.
func TestAMidStepTurnDoesNotMoveYou(t *testing.T) {
	w := newTestWorld(t, 20, 20)
	w.tick = 100
	p, _ := place(t, w, "wachin", 5, 5)

	w.movePlayer(p, protocol.East)
	if p.Heading != protocol.East || p.X != 6 {
		t.Fatalf("first step: heading %v at x=%d, want East at 6", p.Heading, p.X)
	}

	w.movePlayer(p, protocol.North)
	if p.Y != 5 {
		t.Errorf("y = %d, want 5: a step went through mid-cadence", p.Y)
	}
	if p.Heading != protocol.North {
		t.Errorf("heading = %v, want North: turning has its own timer", p.Heading)
	}
}

// Turning in place is metered by its own interval, so holding a direction
// against a wall does not spin the character at framerate.
func TestTurningInPlaceIsRateLimited(t *testing.T) {
	w := newTestWorld(t, 20, 20)
	w.tick = 100
	p, _ := place(t, w, "wachin", 5, 5)
	p.ParalyzedUntil = w.tick + 1000 // rooted, so every command is a turn
	// Facing something other than the first direction asked for: turning to
	// where you already face is a no-op and deliberately does not spend the
	// interval, so starting on North would make the first call free.
	p.Heading = protocol.South

	w.movePlayer(p, protocol.North)
	if p.Heading != protocol.North {
		t.Fatalf("first turn did not land: %v", p.Heading)
	}
	w.movePlayer(p, protocol.East)
	if p.Heading != protocol.North {
		t.Errorf("heading = %v, want North: a second turn beat the interval", p.Heading)
	}

	w.tick += turnCooldownMilliticks / 1000
	w.movePlayer(p, protocol.East)
	if p.Heading != protocol.East {
		t.Errorf("heading = %v, want East once the turn interval elapsed", p.Heading)
	}
}

// Once the cooldown has elapsed the turn lands, so this is a delay of one step
// and never a refusal to turn at all.
func TestTurningLandsOnTheNextStepBoundary(t *testing.T) {
	w := newTestWorld(t, 20, 20)
	w.tick = 100
	p, _ := place(t, w, "wachin", 5, 5)

	w.movePlayer(p, protocol.East)
	w.tick += moveCooldownMilliticks/1000 + 1

	w.movePlayer(p, protocol.North)
	if p.Heading != protocol.North {
		t.Errorf("heading = %v, want North once the step finished", p.Heading)
	}
	if p.Y != 4 {
		t.Errorf("y = %d, want 4", p.Y)
	}
}

// Walking into a wall faces you at the wall — the turn is kept even though the
// step is refused, which is what the source does and what makes attacking a
// wall-adjacent target work.
func TestTurningIntoAWallStillTurnsYou(t *testing.T) {
	w := newTestWorld(t, 20, 20)
	w.tick = 100
	p, _ := place(t, w, "wachin", 5, 5)
	w.grid.SetBlocked(5, 4, true)

	w.movePlayer(p, protocol.North)

	if p.Heading != protocol.North {
		t.Errorf("heading = %v, want North: blocked should still turn", p.Heading)
	}
	if p.X != 5 || p.Y != 5 {
		t.Errorf("moved to (%d,%d), want to stay at (5,5)", p.X, p.Y)
	}
}

// A rooted player has no step in flight to protect, and the source lets them
// turn freely, so paralysis must not inherit the mid-step rule.
func TestAParalyzedPlayerCanStillTurn(t *testing.T) {
	w := newTestWorld(t, 20, 20)
	w.tick = 100
	p, _ := place(t, w, "wachin", 5, 5)
	p.Heading = protocol.South
	p.ParalyzedUntil = w.tick + 100

	w.movePlayer(p, protocol.North)

	if p.Heading != protocol.North {
		t.Errorf("heading = %v, want North: a paralyzed player may still turn", p.Heading)
	}
	if p.X != 5 || p.Y != 5 {
		t.Errorf("a paralyzed player moved to (%d,%d)", p.X, p.Y)
	}
}

// Spamming a direction faster than the cadence must not accelerate anything —
// the same guarantee the move cooldown always gave, now covering facing too.
func TestSpammingTurnsDoesNotOutrunTheCadence(t *testing.T) {
	w := newTestWorld(t, 40, 20)
	w.tick = 100
	p, _ := place(t, w, "wachin", 5, 5)

	turns := []protocol.Heading{protocol.East, protocol.North, protocol.West, protocol.South}
	start := p.X
	for i := 0; i < 60; i++ {
		w.movePlayer(p, turns[i%len(turns)])
	}

	// Sixty commands inside one tick buy exactly one step.
	if moved := abs(p.X - start); moved > 1 {
		t.Errorf("moved %d tiles from 60 commands in one tick", moved)
	}
}
