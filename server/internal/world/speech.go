package world

import (
	"strings"
	"unicode/utf8"

	"juegito/server/internal/protocol"
)

// maxSpeechRunes caps one line of speech.
//
// The client wraps at 18 characters a line (MAX_LENGTH in clsDialogs.cls), so
// this is a handful of lines: long enough to say something, short enough that a
// sign over somebody's head cannot cover the screen. Counted in runes rather
// than bytes because the game is played in Spanish and an "ñ" is not two
// characters to anyone looking at it.
const maxSpeechRunes = 90

// say broadcasts what a player typed to everyone who can see them.
//
// Speech is bounded by the same viewport that bounds everything else, so it
// cannot be used to find somebody you could not already see. An invisible
// player is the deliberate exception: their words carry even though their body
// does not, which is what makes talking — or casting — give you away.
func (w *World) say(p *Player, text string) {
	if p.Dead {
		return
	}

	text = sanitizeSpeech(text)
	if text == "" {
		// Saying nothing is not a no-op: it is how you wipe the incantation
		// off your own head, since one sign per character means the empty one
		// replaces whatever was there. Argentum gets this for free from the
		// same rule and players use it deliberately; it works here for the
		// same reason rather than as a special case.
		w.broadcastSpeech(p, "", false)
		return
	}
	w.broadcastSpeech(p, text, false)
}

// sanitizeSpeech trims a line down to something safe to draw.
//
// Control characters are dropped rather than escaped: there is nothing a
// newline or a tab can mean over somebody's head, and letting them through
// would let one player push another's sign around the screen.
func sanitizeSpeech(text string) string {
	var b strings.Builder
	for _, r := range text {
		if r == '\n' || r == '\r' || r == '\t' {
			b.WriteRune(' ')
			continue
		}
		if r < 0x20 || r == 0x7f {
			continue
		}
		b.WriteRune(r)
	}
	out := strings.TrimSpace(b.String())
	if utf8.RuneCountInString(out) > maxSpeechRunes {
		runes := []rune(out)
		out = string(runes[:maxSpeechRunes])
	}
	return out
}

// broadcastSpeech puts a sign over p's head for everyone who can see them,
// including p.
func (w *World) broadcastSpeech(p *Player, text string, spell bool) {
	const halfW, halfH = ViewportW / 2, ViewportH / 2

	event := protocol.Speech{EntityID: uint32(p.ID), X: p.X, Y: p.Y, Text: text, Spell: spell}
	for _, other := range w.players {
		dx, dy := p.X-other.X, p.Y-other.Y
		if dx < -halfW || dx > halfW || dy < -halfH || dy > halfH {
			continue
		}
		w.sendTo(other, protocol.TypeSpeech, event)
	}
}
