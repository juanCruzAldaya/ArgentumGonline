package world

import (
	"strings"
	"testing"

	"juegito/server/internal/protocol"
)

// hasType reports whether the connection was ever sent a frame of this type.
// Unlike lastOfType it does not fail the test when there is none, which is what
// the "nobody outside the viewport heard it" assertions need.
func hasType(t *testing.T, f *fakeConn, typ protocol.MsgType) bool {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()

	codec := protocol.JSONCodec{}
	for _, frame := range f.frames {
		got, _, err := codec.DecodeEnvelope(frame)
		if err != nil {
			t.Fatalf("undecodable frame: %v", err)
		}
		if got == typ {
			return true
		}
	}
	return false
}

func decodeSpeech(t *testing.T, w *World, conn *fakeConn) protocol.Speech {
	t.Helper()
	var speech protocol.Speech
	if err := w.codec.DecodePayload(conn.lastOfType(t, protocol.TypeSpeech), &speech); err != nil {
		t.Fatalf("decode speech: %v", err)
	}
	return speech
}

func TestSpeechReachesEveryoneInTheViewport(t *testing.T) {
	w := statusWorld(t)
	speaker, speakerConn := place(t, w, "wachin", 40, 40)
	_, nearConn := place(t, w, "vecino", 42, 41)

	w.say(speaker, "hola")

	for name, conn := range map[string]*fakeConn{"el que habla": speakerConn, "el vecino": nearConn} {
		speech := decodeSpeech(t, w, conn)
		if speech.Text != "hola" {
			t.Errorf("%s recibió %q, esperaba %q", name, speech.Text, "hola")
		}
		if speech.EntityID != uint32(speaker.ID) {
			t.Errorf("%s recibió el cartel del id %d, esperaba %d", name, speech.EntityID, speaker.ID)
		}
		if speech.X != 40 || speech.Y != 40 {
			t.Errorf("%s recibió la posición (%d,%d), esperaba (40,40)", name, speech.X, speech.Y)
		}
	}
}

// Speech obeys the same interest management as everything else: it is a way to
// give away where you are, not a way to find somebody you could not already
// have walked into.
func TestSpeechStopsAtTheViewportEdge(t *testing.T) {
	w := statusWorld(t)
	speaker, _ := place(t, w, "wachin", 40, 40)
	_, farConn := place(t, w, "lejano", 40+ViewportW, 40)

	w.say(speaker, "hola")

	if hasType(t, farConn, protocol.TypeSpeech) {
		t.Error("alguien fuera del viewport escuchó lo que se dijo")
	}
}

// The counterplay to casting being loud: one sign per character means saying
// anything replaces it, and an empty line is a real move rather than an ignored
// one. Players use a single space for exactly this.
func TestEmptySpeechClearsTheSign(t *testing.T) {
	w := statusWorld(t)
	speaker, conn := place(t, w, "wachin", 40, 40)

	w.say(speaker, "algo")
	w.say(speaker, "   ")

	speech := decodeSpeech(t, w, conn)
	if speech.Text != "" {
		t.Errorf("un renglón en blanco mandó %q, esperaba vaciar el cartel", speech.Text)
	}
}

func TestSpeechIsTrimmedAndBounded(t *testing.T) {
	w := statusWorld(t)
	speaker, conn := place(t, w, "wachin", 40, 40)

	w.say(speaker, "  hola\nmundo\ttodo  "+strings.Repeat("x", 200))

	speech := decodeSpeech(t, w, conn)
	if strings.ContainsAny(speech.Text, "\n\r\t") {
		t.Errorf("quedaron caracteres de control en %q", speech.Text)
	}
	if n := len([]rune(speech.Text)); n > maxSpeechRunes {
		t.Errorf("el texto quedó en %d runas, el tope es %d", n, maxSpeechRunes)
	}
}

// The whole point of the mechanic: casting announces where you are, to
// everybody who can see the tile, whether or not they can see you.
func TestCastingBroadcastsTheIncantation(t *testing.T) {
	w := statusWorld(t)
	caster, _ := place(t, w, "mago", 40, 40)
	victim, _ := place(t, w, "victima", 41, 40)
	_, witnessConn := place(t, w, "testigo", 43, 41)

	w.spells[spellParalyze] = Spell{
		ID: spellParalyze, Name: "Paralizar", Words: "OHL VAR PARALIZANT",
		Target: targetBoth, Mana: 10, Paralyzes: true,
	}
	castKnown(w, caster, spellParalyze)
	w.cast(caster, spellParalyze, victim.ID)

	speech := decodeSpeech(t, w, witnessConn)
	if speech.Text != "OHL VAR PARALIZANT" {
		t.Errorf("el testigo escuchó %q, esperaba las palabras mágicas", speech.Text)
	}
	if !speech.Spell {
		t.Error("el cartel no vino marcado como hechizo, así que se dibujaría como charla")
	}
	if speech.EntityID != uint32(caster.ID) {
		t.Errorf("el cartel quedó anclado al id %d, esperaba el del lanzador %d", speech.EntityID, caster.ID)
	}
}

// modHechizos.bas: casting drops Ocultar outright. Being invisible does not
// silence the words either — that is what makes an invisible caster's sign hang
// in empty air over their real tile.
func TestCastingWhileHiddenRevealsAndStillSpeaks(t *testing.T) {
	w := statusWorld(t)
	caster, _ := place(t, w, "ladron", 40, 40)
	victim, _ := place(t, w, "victima", 41, 40)
	_, witnessConn := place(t, w, "testigo", 42, 40)

	w.spells[spellParalyze] = Spell{
		ID: spellParalyze, Name: "Paralizar", Words: "OHL VAR PARALIZANT",
		Target: targetBoth, Mana: 10, Paralyzes: true,
	}
	w.hide(caster)
	if !caster.invisible(w.tick) {
		t.Fatal("precondición: el lanzador tenía que estar oculto")
	}

	castKnown(w, caster, spellParalyze)
	w.cast(caster, spellParalyze, victim.ID)

	if caster.invisible(w.tick) {
		t.Error("lanzar no sacó el ocultamiento")
	}
	speech := decodeSpeech(t, w, witnessConn)
	if speech.Text != "OHL VAR PARALIZANT" || speech.X != 40 || speech.Y != 40 {
		t.Errorf("el testigo recibió %q en (%d,%d), esperaba las palabras sobre el tile real del lanzador",
			speech.Text, speech.X, speech.Y)
	}
}

// The two crossed intervals from Server.ini are what stop magic and melee from
// being two buttons pressed at once.
func TestCastingDelaysTheNextSwing(t *testing.T) {
	w := statusWorld(t)
	attacker, _ := place(t, w, "mago", 40, 40)
	victim, _ := place(t, w, "victima", 41, 40)
	attacker.Heading = protocol.East

	castKnown(w, attacker, spellParalyze)
	w.cast(attacker, spellParalyze, victim.ID)

	// Asserted on the cooldown the swing consumes rather than on damage: a
	// swing that is allowed through can still miss on evasion, and a test that
	// reads the victim's HP would fail on the dice instead of on the rule.
	attacker.lastAttackTick = 0
	w.tick += castToAttackTicks - 1
	w.attack(attacker)
	if attacker.lastAttackTick != 0 {
		t.Error("se pudo golpear dentro del intervalo magia-golpe")
	}

	w.tick += 2
	w.attack(attacker)
	if attacker.lastAttackTick == 0 {
		t.Error("pasado el intervalo magia-golpe el golpe siguió bloqueado")
	}
}

func TestSwingingDelaysTheNextCast(t *testing.T) {
	w := statusWorld(t)
	attacker, _ := place(t, w, "guerrero", 40, 40)
	victim, _ := place(t, w, "victima", 41, 40)
	attacker.Heading = protocol.East

	w.attack(attacker)
	castKnown(w, attacker, spellParalyze)
	attacker.lastCastTick = 0

	w.tick += attackToCastTicks - 1
	w.cast(attacker, spellParalyze, victim.ID)
	if attacker.lastCastTick != 0 {
		t.Error("se pudo lanzar dentro del intervalo golpe-magia")
	}

	w.tick += 2
	w.cast(attacker, spellParalyze, victim.ID)
	if attacker.lastCastTick == 0 {
		t.Error("pasado el intervalo golpe-magia el hechizo siguió bloqueado")
	}
}
