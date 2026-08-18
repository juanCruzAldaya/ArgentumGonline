// Package account holds who the players are and how their matches went.
//
// It is the first thing in this server that outlives a process. Everything else
// is deliberately ephemeral — nobody levels up, a match is minutes long, and
// there was nothing worth saving. A career is different: "you have won three of
// forty" is a sentence about the past, and it needs one.
//
// The store is an append-only log, not a database, and that is a considered
// choice rather than a shortcut. What is written here is a registration and a
// finished match — two facts that are never edited and never deleted — which is
// exactly the shape a log is good at, and the simplest durable structure there
// is. The alternative was SQLite, which would have cost nine transitive
// dependencies in a repo that has one on purpose and builds static for
// distroless. Everything here is behind Store, so swapping the backing for a
// real database later is replacing one file.
package account

import (
	"errors"
	"strings"
	"time"
)

// Errors a caller is expected to handle rather than log.
var (
	ErrNameTaken    = errors.New("ese nombre ya está tomado")
	ErrNoSuchUser   = errors.New("no existe esa cuenta")
	ErrBadPassword  = errors.New("contraseña incorrecta")
	ErrBadName      = errors.New("el nombre tiene que ser de 3 a 16 letras o números")
	ErrShortPass    = errors.New("la contraseña tiene que tener al menos 6 caracteres")
	ErrStoreClosed  = errors.New("account: store cerrado")
	ErrNotRecording = errors.New("account: sin almacenamiento configurado")
)

// MinPasswordLen is the floor. Deliberately low: this guards a scoreboard in a
// game, and a rule strict enough to be annoying pushes people to reuse a real
// password, which is the outcome that would actually matter.
const MinPasswordLen = 6

// Match is one finished match from one player's side. It is exactly what the
// outcome card already carries, which is why recording a career costs so little
// — the numbers were being computed and shown already, and were then thrown
// away.
type Match struct {
	PlayedAt  time.Time `json:"at"`
	Placement int       `json:"place"`
	Players   int       `json:"of"`
	Kills     int       `json:"kills"`
	Seconds   float64   `json:"secs"`
	Won       bool      `json:"won,omitempty"`
	Map       string    `json:"map,omitempty"`
}

// Profile is a career, as the account screen shows it.
type Profile struct {
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"since"`

	Matches int `json:"matches"`
	Wins    int `json:"wins"`
	Kills   int `json:"kills"`
	// Best is the highest placement ever reached, 1 being a win. Zero means no
	// match finished yet, which the client shows as a dash rather than as a
	// suspiciously good "0th".
	Best int `json:"best"`
	// Seconds is time survived across every match, which is the stat that says
	// "I play a lot" without saying "I win a lot".
	Seconds float64 `json:"secs"`
	// Recent is the last few matches, newest first.
	Recent []Match `json:"recent"`
}

// recentKept is how much history a profile carries. Enough to see a streak,
// short enough that the answer fits in one message and one screen.
const recentKept = 10

// validName is the same rule the character name has always followed, applied
// where it now means something: this string is an identity rather than a label
// over somebody's head.
//
// Letters and digits only, no spaces. Names are compared case-insensitively —
// "Wachin" and "wachin" are the same account — so that nobody can register the
// visually identical name of somebody who already plays here.
func validName(name string) bool {
	if len(name) < 3 || len(name) > 16 {
		return false
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		default:
			return false
		}
	}
	return true
}

func foldName(name string) string { return strings.ToLower(strings.TrimSpace(name)) }
