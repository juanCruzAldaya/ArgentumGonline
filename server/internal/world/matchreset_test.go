package world

import "testing"

// queuedSeat arma un asiento ya sentado y en la cola, sin pasar por la
// goroutine del mundo. Los tests de abajo necesitan empezar una partida desde
// el lobby en un mundo que avanzan tick por tick con step().
func queuedSeat(w *World, name string) *seat {
	w.lobby.nextSeat++
	s := &seat{
		id:      seatID(w.lobby.nextSeat),
		name:    name,
		conn:    &fakeConn{},
		hasChar: true,
		queued:  true,
		started: make(chan EntityID, 1),
	}
	if w.lobby.seats == nil {
		w.lobby.seats = map[seatID]*seat{}
	}
	w.lobby.seats[s.id] = s
	return s
}

// La partida que se queda vacía tiene que devolver el lobby.
//
// Con un solo jugador la partida no es "decidible" —no puede haber un último
// en pie— y eso hacía que no terminara nunca: el que entraba solo, jugaba y
// cerraba el cliente dejaba el servidor en "partida en curso, 0 jugando",
// rechazando a todos los que llegaran después hasta que alguien lo reiniciara.
func TestAnEmptyMatchGivesTheLobbyBack(t *testing.T) {
	w := newTestWorld(t, 40, 40)
	p, _ := place(t, w, "solo", 10, 10)
	w.startMatchIfIdle()
	if w.match.phase != matchRunning {
		t.Fatalf("la partida no arrancó: phase=%v", w.match.phase)
	}
	if w.decidable() {
		t.Fatal("con un jugador la partida no tendría que ser decidible")
	}

	w.removePlayer(p.ID)
	w.step()

	if w.match.phase != matchLobby {
		t.Fatalf("phase=%v con cero jugadores, esperaba volver al lobby", w.match.phase)
	}
}

// Una partida que arranca desde el lobby es tan nueva como una que arranca por
// reinicio, y era la única que no se comportaba así: heredaba el anillo de la
// anterior, ya cerrado hasta donde hubiera llegado y sin los 60 s de gracia.
func TestASecondMatchFromTheLobbyReopensTheRing(t *testing.T) {
	w := zoneWorld(t)
	place(t, w, "uno", 300, 300)
	if !w.zone.enabled {
		t.Fatal("la primera partida no abrió la zona")
	}
	opening := w.zone.radius

	// Se la deja cerrar hasta pasar la primera etapa.
	for i := 0; i < zoneGraceTicks+zoneHoldTicks+zoneShrinkTicks+10; i++ {
		w.step()
	}
	if w.zone.stage < 1 {
		t.Fatalf("la zona no llegó a cerrar una etapa: stage=%d", w.zone.stage)
	}
	closed := w.zone.radius

	queuedSeat(w, "dos")
	w.startMatchFromLobby()

	if w.zone.stage != 0 {
		t.Errorf("la partida nueva arrancó en la etapa %d de la anterior", w.zone.stage)
	}
	if w.zone.radius <= closed {
		t.Errorf("el radio quedó en %.0f, el de la partida anterior; esperaba %.0f", w.zone.radius, opening)
	}
	if w.zone.phase != zoneWaiting {
		t.Error("la partida nueva no arrancó con la gracia de la zona")
	}
}

// El piso también se rehace: si no, la segunda partida se juega sobre lo que
// quedó tirado en la primera.
func TestASecondMatchFromTheLobbyRebuildsTheFloor(t *testing.T) {
	w := zoneWorld(t)
	place(t, w, "uno", 300, 300)

	clear(w.ground)
	queuedSeat(w, "dos")
	w.startMatchFromLobby()

	if len(w.ground) == 0 {
		t.Error("la partida nueva arrancó con el piso vacío")
	}
}

// Y el pico de jugadores cuenta esta partida, no la anterior: es el número que
// sale en el log y en la tarjeta del final.
func TestPeakCountsThisMatchOnly(t *testing.T) {
	w := zoneWorld(t)
	place(t, w, "uno", 300, 300)
	place(t, w, "dos", 302, 300)
	place(t, w, "tres", 304, 300)
	if w.match.peak != 3 {
		t.Fatalf("peak=%d en la primera partida, esperaba 3", w.match.peak)
	}
	for _, p := range w.playersInOrder() {
		w.removePlayer(p.ID)
	}

	s := queuedSeat(w, "cuatro")
	w.startMatchFromLobby()
	<-s.started

	if w.match.peak != 1 {
		t.Errorf("peak=%d en la partida nueva, esperaba 1", w.match.peak)
	}
}
