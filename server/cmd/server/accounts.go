package main

import (
	"log/slog"

	"juegito/server/internal/account"
	"juegito/server/internal/protocol"
	"juegito/server/internal/world"
)

// accountBridge is the composition root's translation layer: it turns the
// store's types into the wire's, and turns a blocking disk write into one that
// the world goroutine can call.
//
// The world knows nothing about files or hashes and the store knows nothing
// about the protocol; this is the only place that knows both, which is why the
// coupling lives here and not in either of them.
type accountBridge struct {
	store *account.Store
	log   *slog.Logger
	// writes carries finished matches to the goroutine that files them.
	writes chan pendingMatch
}

type pendingMatch struct {
	name  string
	match account.Match
}

// recordQueue is how many finished matches can be waiting to be written.
//
// Generous on purpose: the burst that matters is the closing circle taking a
// crowd within a second or two of each other, and every one of those is
// somebody's placement. Deep enough that it never fills in a real match, and
// bounded so a stuck disk costs memory instead of costing the world its tick.
const recordQueue = 4096

func newAccountBridge(store *account.Store, log *slog.Logger) *accountBridge {
	b := &accountBridge{
		store:  store,
		log:    log,
		writes: make(chan pendingMatch, recordQueue),
	}
	go b.run()
	return b
}

// run files matches off the world goroutine.
//
// This is the whole reason the queue exists. Writing a match is an append plus
// an fsync, a couple of milliseconds, and the world has 50 of those per tick
// for everything it does. One death is nothing; the ring killing forty people
// in the same second is a stutter every one of them would feel.
func (b *accountBridge) run() {
	for pending := range b.writes {
		if err := b.store.Record(pending.name, pending.match); err != nil {
			b.log.Error("no se pudo registrar la partida",
				"cuenta", pending.name, "err", err)
		}
	}
}

func (b *accountBridge) Register(name, password string) error {
	return b.store.Register(name, password)
}

func (b *accountBridge) Authenticate(name, password string) (string, error) {
	return b.store.Authenticate(name, password)
}

func (b *accountBridge) Profile(name string) (protocol.Account, error) {
	p, err := b.store.Profile(name)
	if err != nil {
		return protocol.Account{}, err
	}

	recent := make([]protocol.MatchRow, 0, len(p.Recent))
	for _, m := range p.Recent {
		recent = append(recent, protocol.MatchRow{
			At:    m.PlayedAt.Unix(),
			Place: m.Placement,
			Of:    m.Players,
			Kills: m.Kills,
			Secs:  m.Seconds,
			Won:   m.Won,
			Map:   m.Map,
		})
	}

	return protocol.Account{
		Name:    p.Name,
		Since:   p.CreatedAt.Unix(),
		Matches: p.Matches,
		Wins:    p.Wins,
		Kills:   p.Kills,
		Best:    p.Best,
		Seconds: p.Seconds,
		Recent:  recent,
	}, nil
}

// Record hands the match to the writer without waiting for it.
//
// A full queue drops the record and says so. Losing a row is bad; making every
// player on the server wait on a disk that has stopped answering is worse, and
// the log line is what turns the first into something anybody notices.
func (b *accountBridge) Record(name string, outcome protocol.Outcome, mapName string) {
	select {
	case b.writes <- pendingMatch{
		name: name,
		match: account.Match{
			Placement: outcome.Placement,
			Players:   outcome.Players,
			Kills:     outcome.Kills,
			Seconds:   outcome.Seconds,
			Won:       outcome.Won,
			Map:       mapName,
		},
	}:
	default:
		b.log.Warn("cola de registro llena, se pierde una partida", "cuenta", name)
	}
}

// Close drains what is queued and shuts the store.
func (b *accountBridge) Close() error {
	close(b.writes)
	return b.store.Close()
}

// compile-time proof that the bridge is what the world asked for.
var _ world.Accounts = (*accountBridge)(nil)
