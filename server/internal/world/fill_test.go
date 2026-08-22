package world

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

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

// Con una persona sentada, la cola se completa hasta el objetivo — y no se
// pasa. Los ya largados cuentan aunque todavía no tengan asiento: sin eso, cada
// tick vería la cola corta y largaría de nuevo.
func TestFillsUpToTheTargetAndStopsThere(t *testing.T) {
	w, f := fillWorld(t, 10)
	w.lobby.seats[1] = &seat{id: 1, name: "wachin", queued: true}

	for range 60 {
		w.fillTick()
	}
	waitFor(t, f.launched.Load, 9, "bots largados")

	if n := f.launched.Load(); n != 9 {
		t.Errorf("largó %d bots, esperaba 9 para completar 10 con la persona", n)
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
	w.lobby.seats[1] = &seat{id: 1, name: "wachin", queued: true}
	for range 60 {
		w.fillTick()
	}
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
	w.lobby.seats[1] = &seat{id: 1, name: "wachin", queued: true}
	for range 60 {
		w.fillTick()
	}
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
