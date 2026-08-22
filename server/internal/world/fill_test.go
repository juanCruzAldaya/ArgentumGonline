package world

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"juegito/server/internal/protocol"
	"juegito/server/internal/transport"
)

// waitFor espera a que un contador llegue a want. launchBot larga el bot en su
// propia goroutine, igual que en producción, así que leer el contador apenas
// vuelve fillTick lee una carrera y no un resultado.
func waitFor(t *testing.T, got func() int32, want int32, what string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if got() >= want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("%s: quedó en %d, esperaba %d", what, got(), want)
}

// fakeFiller cuenta bots largados y los deja hablando por su punta del pipe sin
// hacer nada. Alcanza para lo que estos tests miran: cuántos se largan y cuándo
// se los echa. Lo que un bot decide una vez adentro es cosa de internal/bot.
type fakeFiller struct {
	launched atomic.Int32
	stopped  atomic.Int32
}

func (f *fakeFiller) spawn(ctx context.Context, name string, conn transport.Conn) {
	f.launched.Add(1)
	go func() {
		<-ctx.Done()
		f.stopped.Add(1)
		_ = conn.Close()
	}()
}

func fillWorld(t *testing.T, to int) (*World, *fakeFiller) {
	t.Helper()
	w := newTestWorld(t, 40, 40)
	f := &fakeFiller{}
	w.SetFill(to, f.spawn)
	// El relleno sólo actúa en el campamento, que es donde arranca un mundo
	// nuevo. Explícito para que el test no dependa de ese default.
	w.match.phase = matchLobby
	return w, f
}

// Sin nadie de verdad esperando no se larga un solo bot. El relleno acompaña a
// una persona; si no hay ninguna, un servidor que se rellena a sí mismo nunca
// dejaría dormir a la máquina.
func TestNobodyRealMeansNoBots(t *testing.T) {
	w, f := fillWorld(t, 40)

	for range 20 {
		w.fillTick()
	}

	if n := f.launched.Load(); n != 0 {
		t.Errorf("largó %d bots con el campamento vacío", n)
	}
}

// Con una persona en la cola, se completa hasta el objetivo — y no se pasa.
// Los ya largados cuentan aunque todavía no tengan asiento: sin eso, cada tick
// vería la cola corta y largaría de nuevo.
func TestFillsUpToTheTargetAndStopsThere(t *testing.T) {
	w, f := fillWorld(t, 10)
	queueHuman(w, 1, "wachin")
	runCountdown(w, 200)

	waitFor(t, f.launched.Load, 9, "bots largados")
	if n := f.launched.Load(); n != 9 {
		t.Errorf("largó %d bots, esperaba 9 para completar 10 con la persona", n)
	}
}

// La sala se llena de a poco, repartida a lo largo de la cuenta regresiva, y no
// de golpe. Aparecer los treinta y nueve en un segundo delata lo que son, y le
// saca sentido a la espera: si a los diez segundos ya están todos, los dos
// minutos no están esperando a nadie.
func TestTheRoomFillsGraduallyOverTheCountdown(t *testing.T) {
	w, f := fillWorld(t, 40)
	queueHuman(w, 1, "wachin")

	// Arranca el reloj: 100 ticks de espera.
	w.lobby.waitTicks = 100
	w.lobby.counting = true
	w.lobby.startAt = w.tick + 100

	// A mitad de camino tiene que haber aproximadamente la mitad, no todos.
	for range 50 {
		w.fillTick()
		w.tick++
	}
	half := int(f.launched.Load())
	if half >= 39 {
		t.Fatalf("a mitad de la cuenta ya había largado %d de 39: no es progresivo", half)
	}
	if half < 10 {
		t.Errorf("a mitad de la cuenta sólo largó %d de 39: la rampa va demasiado lenta", half)
	}

	// Y al final, todos.
	for range 60 {
		w.fillTick()
		w.tick++
	}
	waitFor(t, f.launched.Load, 39, "bots al final de la cuenta")
}

// El total es siempre cuarenta: las personas ocupan lugar. Con treinta y nueve
// esperando entra un bot, y con cuarenta no entra ninguno.
func TestPeopleTakeTheBotsPlaces(t *testing.T) {
	for _, c := range []struct{ humans, wantBots int }{{1, 39}, {30, 10}, {39, 1}, {40, 0}} {
		w, f := fillWorld(t, 40)
		for i := range c.humans {
			queueHuman(w, seatID(i+1), "persona")
		}
		runCountdown(w, 300)

		if c.wantBots > 0 {
			waitFor(t, f.launched.Load, int32(c.wantBots), "bots largados")
		}
		if n := int(f.launched.Load()); n != c.wantBots {
			t.Errorf("con %d personas largó %d bots, esperaba %d", c.humans, n, c.wantBots)
		}
	}
}

// Antes de que el reloj arranque, el relleno sólo hace lo mínimo para
// dispararlo. Una persona sola nunca llega a los dos que pide el lobby, así que
// sin esto no habría cuenta regresiva sobre la cual repartir nada.
func TestOneBotStartsTheClockForALonePlayer(t *testing.T) {
	w, f := fillWorld(t, 40)
	w.lobby.minPlayers = 2
	queueHuman(w, 1, "wachin")

	for range 10 {
		w.fillTick()
	}
	waitFor(t, f.launched.Load, 1, "el bot que dispara el reloj")
	if n := f.launched.Load(); n != 1 {
		t.Errorf("largó %d bots antes de que arrancara la cuenta, esperaba 1", n)
	}
}

// Mirar el campamento sin apretar entrar no pide una partida, así que no la
// llena.
func TestWatchingWithoutQueueingDoesNotFill(t *testing.T) {
	w, f := fillWorld(t, 40)
	w.lobby.seats[1] = &seat{id: 1, name: "mirando"} // sentado, sin encolar

	for range 30 {
		w.fillTick()
		w.tick++
	}

	if n := f.launched.Load(); n != 0 {
		t.Errorf("largó %d bots para alguien que sólo estaba mirando", n)
	}
}

// queueHuman sienta a una persona y la pone en la cola.
func queueHuman(w *World, id seatID, name string) {
	w.lobby.seats[id] = &seat{id: id, name: name, queued: true, hasChar: true}
}

// runCountdown corre la cuenta regresiva entera, que es cuando el relleno
// reparte sus bots.
func runCountdown(w *World, ticks uint64) {
	w.lobby.waitTicks = ticks
	w.lobby.counting = true
	w.lobby.startAt = w.tick + ticks
	for range ticks + 5 {
		w.fillTick()
		w.tick++
	}
}

// El que se va a mitad de partida se lleva el relleno con él.
//
// Sin esto los bots siguen peleando entre ellos hasta que quede uno —hasta un
// cuarto de hora— y la máquina no puede dormir mientras tanto, porque dormir
// depende de que no quede ninguna conexión abierta. Se vio en producción: la
// persona cerró el cliente y el log siguió con players=39.
func TestBotsGoHomeWhenTheLastHumanLeavesMidMatch(t *testing.T) {
	w, f := fillWorld(t, 10)
	queueHuman(w, 1, "wachin")
	runCountdown(w, 200)
	waitFor(t, f.launched.Load, 9, "bots largados")

	// Arranca la partida y la persona cierra el cliente: su asiento se va, los
	// de los bots quedan.
	w.match.phase = matchRunning
	delete(w.lobby.seats, 1)
	w.matchTick()
	waitFor(t, f.stopped.Load, 9, "bots echados")

	if f.stopped.Load() == 0 {
		t.Error("los bots siguieron jugando solos después de que se fue la última persona")
	}
	if w.botsLive != 0 {
		t.Errorf("botsLive = %d después de echarlos", w.botsLive)
	}
}

// Mientras quede una persona jugando, el relleno se queda: es su partida.
func TestBotsStayWhileSomebodyIsStillPlaying(t *testing.T) {
	w, f := fillWorld(t, 10)
	queueHuman(w, 1, "wachin")
	runCountdown(w, 200)
	waitFor(t, f.launched.Load, 9, "bots largados")

	// La partida corre y la persona sigue en su asiento. Un jugador en el mundo
	// para que no se dispare el "partida vacía", que es otro camino.
	w.match.phase = matchRunning
	place(t, w, "wachin", 5, 5)
	w.matchTick()

	if n := f.stopped.Load(); n != 0 {
		t.Errorf("echó %d bots con la persona todavía adentro", n)
	}
}

// Los nombres tienen que parecer de jugador. Uno llamado bot07 anuncia que la
// partida está vacía, que es lo que el relleno existe para evitar.
func TestBotNamesAreUniqueAndLookLikePlayers(t *testing.T) {
	w, _ := fillWorld(t, 40)

	seen := map[string]bool{}
	for i := range 40 {
		name := w.botName()
		if name == "" {
			t.Fatal("nombre vacío")
		}
		if seen[name] {
			t.Errorf("nombre repetido en el intento %d: %q", i, name)
		}
		seen[name] = true
		// Ocuparlo, que es lo que pasa cuando el bot se sienta.
		w.lobby.seats[seatID(i+1)] = &seat{id: seatID(i + 1), name: name, bot: true}
	}
}

// El feed de bajas va a TODA la partida, no sólo a los dos involucrados.
//
// Es lo contrario de reportCombat, y a propósito: el daño es entre dos, pero
// una baja es del estado de la partida. Con cuarenta jugadores en 760x760 te
// cruzás con poca gente, y sin esto una partida entera puede parecer vacía.
func TestAKillIsAnnouncedToEverybody(t *testing.T) {
	w := matchWorld(t)
	killer, killerConn := place(t, w, "Rakhar", 5, 5)
	victim, _ := place(t, w, "Nahuel", 6, 5)
	_, bystanderConn := place(t, w, "mirón", 40, 40) // lejísimos, fuera del viewport

	w.kill(victim, killer, "")

	// Al que no vio nada le llega igual: eso es lo que prueba que no va por
	// viewport.
	var event protocol.KillEvent
	if err := w.codec.DecodePayload(bystanderConn.lastOfType(t, protocol.TypeKill), &event); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if event.KillerName != "Rakhar" || event.VictimName != "Nahuel" {
		t.Errorf("el aviso dijo %q mató a %q", event.KillerName, event.VictimName)
	}
	if event.Alive != 2 {
		t.Errorf("vivos = %d, esperaba 2", event.Alive)
	}
	// Y al que mató también: lastOfType corta el test si no hay ninguno.
	killerConn.lastOfType(t, protocol.TypeKill)
}

// Una muerte sin asesino se anuncia igual, diciendo de qué fue. Sin esto, el
// que se lo come el anillo desaparece de la partida sin que nadie se entere.
func TestZoneAndSelfInflictedDeathsSayWhy(t *testing.T) {
	for _, cause := range []protocol.KillCause{protocol.CauseZone, protocol.CauseSelf} {
		w := matchWorld(t)
		victim, _ := place(t, w, "Lyra", 5, 5)
		_, conn := place(t, w, "testigo", 6, 5)

		w.kill(victim, nil, cause)

		var event protocol.KillEvent
		if err := w.codec.DecodePayload(conn.lastOfType(t, protocol.TypeKill), &event); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if event.KillerName != "" {
			t.Errorf("le adjudicó la muerte a %q", event.KillerName)
		}
		if event.Cause != cause {
			t.Errorf("causa %q, esperaba %q", event.Cause, cause)
		}
	}
}
