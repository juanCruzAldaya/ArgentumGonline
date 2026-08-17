package world

import "testing"

func TestSpellBookIsFixedLengthWithEmptyTail(t *testing.T) {
	// Guerrero has no mana, so it gets none of the heavy spells and its book
	// is exactly startingSpells followed by empty slots.
	book := spellBook(Guerrero)
	if len(book) != SpellSlots {
		t.Fatalf("book has %d slots, want %d", len(book), SpellSlots)
	}
	for i, id := range startingSpells {
		if book[i] != id {
			t.Errorf("slot %d = %d, want %d", i, book[i], id)
		}
	}
	for i := len(startingSpells); i < SpellSlots; i++ {
		if book[i] != 0 {
			t.Errorf("slot %d = %d, want 0 (empty)", i, book[i])
		}
	}
}

// Each player rearranges their own book, so they must not share a backing
// array — one player's drag would otherwise reshuffle everyone else's list.
func TestEachPlayerGetsTheirOwnSpellBook(t *testing.T) {
	w := newTestWorld(t, 20, 20)
	a, _ := place(t, w, "a", 5, 5)
	b, _ := place(t, w, "b", 6, 5)

	w.swapSpells(a, 0, 29)

	if b.Spells[0] == a.Spells[0] && b.Spells[29] == a.Spells[29] {
		t.Error("both players' books moved together: they share a backing array")
	}
	if b.Spells[0] != startingSpells[0] {
		t.Errorf("player b's slot 0 = %d, want %d untouched", b.Spells[0], startingSpells[0])
	}
}

func TestSwapSpellsExchangesTwoSlots(t *testing.T) {
	w := newTestWorld(t, 20, 20)
	p, _ := place(t, w, "wachin", 5, 5)
	first, third := p.Spells[0], p.Spells[2]

	w.swapSpells(p, 0, 2)

	if p.Spells[0] != third || p.Spells[2] != first {
		t.Errorf("book = %v..., want %d and %d exchanged", p.Spells[:4], first, third)
	}
}

// Dragging into the empty tail is the normal way to spread a book out, so it
// has to work rather than being rejected as "nothing there".
func TestSwapSpellsIntoAnEmptySlot(t *testing.T) {
	w := newTestWorld(t, 20, 20)
	p, _ := place(t, w, "wachin", 5, 5)
	moved := p.Spells[0]

	w.swapSpells(p, 0, SpellSlots-1)

	if p.Spells[SpellSlots-1] != moved {
		t.Errorf("last slot = %d, want %d", p.Spells[SpellSlots-1], moved)
	}
	if p.Spells[0] != 0 {
		t.Errorf("slot 0 = %d, want 0: the empty slot should have come back the other way", p.Spells[0])
	}
}

// Every client-named index is bounds-checked against the server's own
// SpellSlots rather than trusted.
func TestSwapSpellsRejectsOutOfRangeIndices(t *testing.T) {
	w := newTestWorld(t, 20, 20)
	p, _ := place(t, w, "wachin", 5, 5)
	before := append([]int(nil), p.Spells...)

	for _, pair := range [][2]int{{-1, 0}, {0, -1}, {0, SpellSlots}, {SpellSlots, 0}, {999, 1000}} {
		w.swapSpells(p, pair[0], pair[1])
	}

	for i := range before {
		if p.Spells[i] != before[i] {
			t.Fatalf("an out-of-range swap changed the book at slot %d", i)
		}
	}
}

// The 0 that marks an empty slot must never be castable, or a client could
// name it and walk straight past the "do you know this spell" gate.
func TestTheEmptySlotMarkerIsNotCastable(t *testing.T) {
	w := newTestWorld(t, 20, 20)
	p, _ := place(t, w, "wachin", 5, 5)

	if knowsSpell(p, 0) {
		t.Error("spell id 0 reported as known")
	}
}
