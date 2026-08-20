package world

import (
	"sort"
	"time"
)

// latencyTracker turns the pongs coming back from one connection into the
// number a player would call "el lag".
//
// The protocol could measure this from the first day: the client sends a ping
// and session.go echoes it back, outside the tick, precisely so the number is
// not polluted by the wait for the next one. Nothing ever called it — the
// client has a send_ping() that no code path reaches — so the measurement
// existed and the value lived nowhere.
//
// Turning the direction around is what puts it in the log: the server pings,
// the client echoes, and the side keeping the record is the side that already
// writes to the log. That also means one player can be told what another
// player's latency is, which is the whole point when the two of them are in
// different cities and only one has a terminal.
//
// # What the number includes
//
// The round trip, plus one frame of the client. GDScript polls the socket from
// _process, so the echo waits for the next drawn frame before it leaves —
// measured against a local server, where the network is zero, this reads 15 ms
// rather than 0. That is not an error to subtract away: a player's input waits
// for the same frame, so it is part of what the game feels like. But it does
// mean a reading is bounded below by the client's frame time, and that two
// players on identical connections will differ if one of them runs at 30 fps
// and the other at 60.
//
// To separate the two, compare against cmd/probe, which answers immediately and
// therefore reports the network alone.
type latencyTracker struct {
	samples []int64 // round trips, in milliseconds
	sent    int
	lastAt  time.Time
}

// discardAbove drops a round trip too large to have come from a live
// connection. The timestamp travels to the client and back untouched, so a
// client that echoes an old one — or a made-up one — reports a latency of its
// own choosing. Nothing here is worth cheating for, but a single bogus sample
// would drag a percentile somewhere unbelievable and cost more time to explain
// than the guard costs to write.
const discardAbove = 60_000

// inFlightGrace is how recently a ping has to have left for its silence to mean
// "still travelling" rather than "lost".
//
// Without it every window reported one ping missing, because the reporting
// interval is a multiple of the ping interval: the two timers fire together,
// and the ping that just left is counted as unanswered before it could
// possibly have arrived. One in fifteen reads as 7% packet loss, which is the
// difference between a metric somebody trusts and one they have to be talked
// out of every time they look at it.
const inFlightGrace = 500 * time.Millisecond

func (t *latencyTracker) ping(at time.Time) {
	t.sent++
	t.lastAt = at
}

func (t *latencyTracker) sample(ms int64) {
	if ms < 0 || ms > discardAbove {
		return
	}
	t.samples = append(t.samples, ms)
}

// summary reports the window and empties it, so each log line covers the
// stretch since the last one rather than the whole session. A player whose
// connection went bad ten minutes in should show it on the next line, not be
// averaged back into the good half hour before it.
//
// The percentiles are what matter and the mean is not here at all: latency is
// not symmetric, and one 400 ms spike moves a mean that then describes nothing
// that actually happened. p95 is the closest thing to "the worst it felt".
func (t *latencyTracker) summary(now time.Time) (p50, p95, max int64, got, lost int) {
	inFlight := 0
	if now.Sub(t.lastAt) < inFlightGrace {
		inFlight = 1
	}

	got = len(t.samples)
	lost = t.sent - got - inFlight
	if lost < 0 {
		lost = 0
	}
	// Carried rather than dropped: the ping still travelling belongs to the
	// next window, and it is that window's job to say whether it ever arrived.
	t.sent = inFlight

	if got == 0 {
		return 0, 0, 0, 0, lost
	}

	sorted := append([]int64(nil), t.samples...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	t.samples = t.samples[:0]

	at := func(q float64) int64 {
		i := int(q * float64(len(sorted)-1))
		return sorted[i]
	}
	return at(0.5), at(0.95), sorted[len(sorted)-1], got, lost
}
