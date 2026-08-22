package world

import (
	"testing"

	"juegito/server/internal/protocol"
)

// playFromLobby empieza una partida con los asientos que se le pidan y devuelve
// los asientos y sus jugadores, en orden.
//
// Pasa por el lobby a propósito: salir al campamento al morir solo existe para
// quien entró por un asiento, así que un test que armara los jugadores con
// place() estaría probando el caso contrario al que le interesa.
func playFromLobby(t *testing.T, w *World, names ...string) ([]*seat, []*Player) {
	t.Helper()
	seats := make([]*seat, 0, len(names))
	for _, name := range names {
		seats = append(seats, queuedSeat(w, name))
	}
	w.startMatchFromLobby()

	players := make([]*Player, 0, len(seats))
	for _, s := range seats {
		if s.playing == 0 {
			t.Fatalf("el asiento %q no entró a la partida", s.name)
		}
		// Vaciado como lo vacía una conexión de verdad: el canal tiene lugar
		// para uno, así que un asiento que juega una segunda partida sin que
		// nadie haya leído la primera dejaría al mundo esperándolo.
		<-s.started
		players = append(players, w.players[s.playing])
	}
	return seats, players
}

// frames cuenta cuántos mensajes de un tipo recibió una conexión.
func (f *fakeConn) countOfType(t *testing.T, typ protocol.MsgType) int {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()

	codec := protocol.JSONCodec{}
	n := 0
	for _, frame := range f.frames {
		got, _, err := codec.DecodeEnvelope(frame)
		if err != nil {
			t.Fatalf("frame ilegible: %v", err)
		}
		if got == typ {
			n++
		}
	}
	return n
}

// Morir descalifica y te saca al campamento. Antes el eliminado se quedaba de
// fantasma en el mapa hasta que la partida terminara: podía caminar y mirar,
// pero no jugar, y no tenía a dónde ir. El lobby es a dónde.
func TestAnEliminatedPlayerGoesBackToTheCamp(t *testing.T) {
	w := newTestWorld(t, 60, 60)
	w.SetDeathExit(5)
	seats, players := playFromLobby(t, w, "uno", "dos", "tres")

	victim := players[0]
	w.kill(victim, players[1], "")

	// El cuerpo se queda unos segundos: la tarjeta recién apareció y el que
	// mató está parado al lado.
	w.step()
	if _, still := w.players[victim.ID]; !still {
		t.Fatal("el eliminado salió del mundo en el mismo tick en que murió")
	}

	for i := 0; i < 5*w.tickRate+2; i++ {
		w.step()
	}

	if _, still := w.players[victim.ID]; still {
		t.Error("el eliminado sigue en el mundo después de que venciera el plazo")
	}
	if seats[0].playing != 0 {
		t.Errorf("el asiento sigue jugando la entidad %d", seats[0].playing)
	}
	if seats[0].queued {
		t.Error("volvió al campamento ya encolado; encolarse es decisión del jugador")
	}
	// Y le vuelve a llegar el estado del lobby, que es —por sí solo— cómo el
	// cliente se entera de que volvió al campamento.
	conn := seats[0].conn.(*fakeConn)
	var state protocol.LobbyState
	if err := w.codec.DecodePayload(conn.lastOfType(t, protocol.TypeLobby), &state); err != nil {
		t.Fatalf("decode lobby: %v", err)
	}
	if !state.Running {
		t.Error("el campamento no dice que hay una partida en curso")
	}
}

// Lo que dejó tirado se queda en el mapa. Salir no es llevarse el botín: el
// inventario se desparrama al morir y esa es la mitad del género.
func TestWhatTheDeadDroppedStaysOnTheMap(t *testing.T) {
	w := newTestWorld(t, 60, 60)
	w.SetDeathExit(1)
	_, players := playFromLobby(t, w, "uno", "dos")

	victim := players[0]
	victim.Inventory = []protocol.InventorySlot{{Slot: 0, ItemID: 480, Amount: 7}}
	w.kill(victim, players[1], "")

	before := len(w.ground)
	if before == 0 {
		t.Fatal("morir no desparramó nada")
	}
	for i := 0; i < w.tickRate+2; i++ {
		w.step()
	}
	if len(w.ground) < before {
		t.Errorf("el piso pasó de %d pilas a %d cuando el muerto salió", before, len(w.ground))
	}
}

// Un Join directo no tiene campamento al que volver. Es el bot de carga y todos
// los tests de este paquete: para ellos la muerte sigue siendo lo que era.
func TestADirectJoinStaysOnTheMap(t *testing.T) {
	w := newTestWorld(t, 40, 40)
	w.SetDeathExit(1)
	uno, _ := place(t, w, "uno", 10, 10)
	dos, _ := place(t, w, "dos", 12, 10)
	w.startMatchIfIdle()

	w.kill(uno, dos, "")
	for i := 0; i < 3*w.tickRate; i++ {
		w.step()
	}

	if _, still := w.players[uno.ID]; !still {
		t.Error("un jugador sin asiento salió del mundo al morir, y no hay a dónde")
	}
}

// Con -respawn puesto, morir no es eliminación: el fantasma vuelve. Mandarlo al
// campamento sería sacarlo de una partida que todavía lo tiene adentro.
func TestRespawnKeepsTheGhost(t *testing.T) {
	w := newTestWorld(t, 60, 60)
	w.SetDeathExit(1)
	w.SetRespawnDelay(2)
	_, players := playFromLobby(t, w, "uno", "dos")

	w.kill(players[0], players[1], "")
	if players[0].exitAt != 0 {
		t.Fatalf("exitAt=%d con respawn puesto, esperaba 0", players[0].exitAt)
	}
	for i := 0; i < 3*w.tickRate; i++ {
		w.step()
	}
	if _, still := w.players[players[0].ID]; !still {
		t.Error("el fantasma se fue al campamento en un servidor con respawn")
	}
}

// Quién ganó le llega también al que ya se fue.
//
// La tarjeta llega dos veces para el que no gana: una al morir, con el puesto
// que ya está decidido, y otra cuando la partida se define, con el ganador. La
// segunda mitad ahora tiene que cruzar la puerta del campamento, porque el
// eliminado hace rato que no es un jugador.
func TestTheWinnerReachesSomebodyWhoAlreadyLeft(t *testing.T) {
	w := newTestWorld(t, 60, 60)
	w.SetDeathExit(1)
	seats, players := playFromLobby(t, w, "uno", "dos", "tres")
	conn := seats[0].conn.(*fakeConn)

	// El primero muere y se va al campamento.
	w.kill(players[0], players[2], "")
	cards := conn.countOfType(t, protocol.TypeOutcome)
	if cards != 1 {
		t.Fatalf("recibió %d tarjetas al morir, esperaba 1", cards)
	}
	for i := 0; i < w.tickRate+2; i++ {
		w.step()
	}
	if _, still := w.players[players[0].ID]; still {
		t.Fatal("el eliminado no llegó a salir")
	}

	// Y recién entonces se define la partida.
	w.kill(players[1], players[2], "")
	w.step()

	if got := conn.countOfType(t, protocol.TypeOutcome); got != 2 {
		t.Fatalf("recibió %d tarjetas en total, esperaba 2", got)
	}
	var card protocol.Outcome
	if err := w.codec.DecodePayload(conn.lastOfType(t, protocol.TypeOutcome), &card); err != nil {
		t.Fatalf("decode outcome: %v", err)
	}
	if card.Winner != "tres" {
		t.Errorf("ganador=%q en la tarjeta del que ya se había ido, esperaba \"tres\"", card.Winner)
	}
	if card.Placement != 3 {
		t.Errorf("puesto=%d, esperaba 3: era el primero de tres en caer", card.Placement)
	}
}

// La partida se decide con el muerto todavía adentro. Es el orden que exitDue
// respeta al correr después de matchTick: si el último en caer saliera en el
// mismo tick, no quedaría nadie contra quien contar al último en pie.
func TestTheLastDeathStillDecidesTheMatch(t *testing.T) {
	w := newTestWorld(t, 60, 60)
	w.SetDeathExit(5)
	_, players := playFromLobby(t, w, "uno", "dos")

	w.kill(players[0], players[1], "")
	w.step()

	if w.match.phase != matchOver {
		t.Fatalf("phase=%v después de la última muerte, esperaba que terminara", w.match.phase)
	}
	if w.match.winnerName != "dos" {
		t.Errorf("ganador=%q, esperaba \"dos\"", w.match.winnerName)
	}
}

// Y una partida nueva no arrastra la tarjeta de la anterior, que es lo que
// haría que endMatch le terminara la frase dos veces al mismo asiento.
func TestANewMatchClearsTheOldCard(t *testing.T) {
	w := newTestWorld(t, 60, 60)
	w.SetDeathExit(1)
	seats, players := playFromLobby(t, w, "uno", "dos", "tres")

	w.kill(players[0], players[2], "")
	for i := 0; i < w.tickRate+2; i++ {
		w.step()
	}
	if !seats[0].carded {
		t.Fatal("el asiento no se quedó con la tarjeta del eliminado")
	}

	seats[0].queued = true
	seats[1].queued = true
	w.match.phase = matchLobby
	w.startMatchFromLobby()

	if seats[0].carded {
		t.Error("la partida nueva arrancó con la tarjeta de la anterior colgada del asiento")
	}
}
