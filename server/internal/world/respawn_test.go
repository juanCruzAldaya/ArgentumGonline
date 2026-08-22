package world

import (
	"testing"
)

// Permadeath is still the default. Everything else in this file is about the
// playtest flag, so this is the one that guards the genre's own rule.
func TestWithoutTheFlagDeathIsStillForever(t *testing.T) {
	w := combatWorld(t)
	victim, _ := place(t, w, "victima", 5, 5)

	w.kill(victim, nil, "")
	if victim.respawnAt != 0 {
		t.Fatalf("respawnAt = %d, want 0: no flag, no respawn", victim.respawnAt)
	}

	for i := 0; i < 10*w.tickRate; i++ {
		w.tick++
		w.respawnDue()
	}
	if !victim.Dead {
		t.Error("the victim came back with respawn disabled")
	}
}

func TestRespawnWaitsItsDelayAndComesBackInTheMiddle(t *testing.T) {
	w := combatWorld(t)
	w.SetRespawnDelay(5)
	victim, _ := place(t, w, "victima", 5, 5)
	victim.Kills = 3

	w.kill(victim, nil, "")
	if want := w.tick + uint64(5*w.tickRate); victim.respawnAt != want {
		t.Fatalf("respawnAt = %d, want %d", victim.respawnAt, want)
	}

	// One tick short of the deadline: still a ghost.
	for w.tick < victim.respawnAt-1 {
		w.tick++
		w.respawnDue()
	}
	if !victim.Dead {
		t.Fatal("came back early, before the delay was up")
	}

	w.tick++
	w.respawnDue()

	if victim.Dead {
		t.Fatal("still dead after the delay elapsed")
	}
	if victim.X != w.grid.W/2 || victim.Y != w.grid.H/2 {
		t.Errorf("respawned at %d,%d, want the middle of the map %d,%d",
			victim.X, victim.Y, w.grid.W/2, w.grid.H/2)
	}
	if id, taken := w.occupied[tileKey{victim.X, victim.Y}]; !taken || id != victim.ID {
		t.Error("the respawned player does not occupy their new tile")
	}
	if victim.Body == ghostBody || victim.Head == ghostHead {
		t.Errorf("came back wearing the corpse: body/head = %d/%d", victim.Body, victim.Head)
	}
	if victim.Vitals.HP != victim.Vitals.MaxHP || victim.Vitals.HP == 0 {
		t.Errorf("HP = %d/%d, want full", victim.Vitals.HP, victim.Vitals.MaxHP)
	}
	if len(victim.Inventory) == 0 {
		t.Error("came back with an empty bag instead of a fresh kit")
	}
	if victim.Kills != 3 {
		t.Errorf("kills = %d, want 3: dying does not erase the score", victim.Kills)
	}
	if victim.respawnAt != 0 {
		t.Error("respawnAt was left armed, so the next tick would respawn them again")
	}
}

// The middle tile can be a wall — half of Ullathorpe is — so the search has to
// give the nearest free tile rather than dropping someone inside a building.
func TestCentreSpawnFallsOutwardWhenTheMiddleIsBlocked(t *testing.T) {
	w := newTestWorld(t, 21, 21)
	cx, cy := w.grid.W/2, w.grid.H/2
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			w.grid.SetBlocked(cx+dx, cy+dy, true)
		}
	}

	x, y := w.centreSpawn()

	if w.grid.Blocked(x, y) {
		t.Fatalf("centreSpawn returned a blocked tile %d,%d", x, y)
	}
	if d := max(abs(x-cx), abs(y-cy)); d != 2 {
		t.Errorf("spawned %d tiles from the centre, want the first free ring, 2", d)
	}
}

// Two ghosts on the same tick cannot land on the same tile, and which of them
// gets the exact centre has to be decided by id rather than by map iteration
// order — everything else in this world is deterministic.
func TestTwoRespawnsOnTheSameTickDoNotShareATile(t *testing.T) {
	w := combatWorld(t)
	w.SetRespawnDelay(1)
	first, _ := place(t, w, "uno", 5, 5)
	second, _ := place(t, w, "dos", 7, 7)

	w.kill(first, nil, "")
	w.kill(second, nil, "")
	for i := 0; i <= w.tickRate; i++ {
		w.tick++
		w.respawnDue()
	}

	if first.Dead || second.Dead {
		t.Fatal("one of the two never came back")
	}
	if first.X == second.X && first.Y == second.Y {
		t.Fatalf("both respawned on %d,%d", first.X, first.Y)
	}
	if first.ID > second.ID {
		first, second = second, first
	}
	if first.X != w.grid.W/2 || first.Y != w.grid.H/2 {
		t.Error("the lower id did not get the centre tile: the order is not deterministic")
	}
}
