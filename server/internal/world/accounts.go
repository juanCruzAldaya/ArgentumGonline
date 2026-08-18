package world

import (
	"juegito/server/internal/protocol"
)

// Accounts is everything the world needs from an account store, and no more.
//
// It is an interface here rather than an import of internal/account so the
// simulation keeps knowing nothing about files, hashes or careers: it can name
// a player and report how their match went, and something else decides what
// that means. The translation between the store's types and the wire's lives at
// the composition root, in cmd/server.
//
// Nil is a valid value and means this server has no accounts: the handshake
// goes back to trusting the name in the join, which is what every test and
// every local run does.
type Accounts interface {
	Register(name, password string) error
	Authenticate(name, password string) (string, error)
	Profile(name string) (protocol.Account, error)
	// Record files one finished match. It must not block: it is called from
	// the world goroutine, and a disk write in there is a stutter for
	// everybody connected.
	Record(name string, outcome protocol.Outcome, mapName string)
}

// MinPasswordLen is what the server tells the client to enforce before spending
// a round trip on a password it would reject anyway. It mirrors the store's own
// floor, which is the one that actually decides.
const MinPasswordLen = 6

// SetAccounts installs the store. Like SetMap, it must be called before Run.
func (w *World) SetAccounts(a Accounts) { w.accounts = a }

// setAccount remembers which account a connection authenticated as, so the
// match can be filed against it when it ends.
//
// Kept beside the player rather than on it because it is not part of the
// simulation: the world does not care who you are logged in as, only the thing
// that writes the record does.
func (w *World) setAccount(id EntityID, name string) {
	if w.accounts == nil || name == "" {
		return
	}
	w.accountNames.Store(id, name)
}

// recordOutcome files one player's finished match against their account.
//
// Called once per player per match: at elimination for everybody who died,
// which is when their placement is already final, and at the end for the one
// who is still standing. Recording in both places would file the dead twice.
func (w *World) recordOutcome(p *Player, outcome protocol.Outcome) {
	if w.accounts == nil {
		return
	}
	name, ok := w.accountNames.Load(p.ID)
	if !ok {
		return
	}
	w.accounts.Record(name.(string), outcome, w.mapName)
}
