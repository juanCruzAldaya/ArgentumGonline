package world

import (
	"io"
	"sync"
	"testing"
	"time"

	"juegito/server/internal/protocol"
)

// liveConn is a connection that stays open after its script runs out, the way a
// real client does. scriptedConn answers EOF the moment its inbox empties,
// which is exactly right for the handshake tests and exactly wrong here: a seat
// waiting in the queue has said everything it is going to say and is still
// connected, and a conn that hangs up instead would never be there when the
// match it is waiting for starts.
type liveConn struct {
	mu     sync.Mutex
	sent   [][]byte
	inbox  chan []byte
	closed chan struct{}
	once   sync.Once
}

func newLiveConn() *liveConn {
	return &liveConn{inbox: make(chan []byte, 16), closed: make(chan struct{})}
}

func (c *liveConn) push(t *testing.T, typ protocol.MsgType, payload any) {
	t.Helper()
	frame, err := protocol.JSONCodec{}.Encode(typ, payload)
	if err != nil {
		t.Fatalf("encode %s: %v", typ, err)
	}
	c.inbox <- frame
}

func (c *liveConn) Recv() ([]byte, error) {
	select {
	case frame := <-c.inbox:
		return frame, nil
	case <-c.closed:
		return nil, io.EOF
	}
}

func (c *liveConn) Send(frame []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sent = append(c.sent, append([]byte(nil), frame...))
	return nil
}

func (c *liveConn) Close() error {
	c.once.Do(func() { close(c.closed) })
	return nil
}

func (c *liveConn) RemoteAddr() string { return "live" }

// got reports whether a frame of this type has arrived, and decodes the most
// recent one into into.
//
// Only the most recent, and that is the whole point: decoding every matching
// frame into the same struct looks equivalent and is not. Half these fields are
// omitempty, and encoding/json leaves a field absent from the payload at
// whatever the struct already held — so a Counting that was once true stays
// true through every later frame that says nothing about it, and the test can
// never see it turn off.
func (c *liveConn) got(typ protocol.MsgType, into any) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	codec := protocol.JSONCodec{}
	var last []byte
	for _, frame := range c.sent {
		gotType, payload, err := codec.DecodeEnvelope(frame)
		if err != nil || gotType != typ {
			continue
		}
		last = payload
	}
	if last == nil {
		return false
	}
	if into != nil {
		_ = codec.DecodePayload(last, into)
	}
	return true
}

// eventually polls until cond holds or the deadline passes. The lobby answers
// on the world's tick, so a test that asserts immediately after sending is
// asserting against a world that has not run yet.
func eventually(t *testing.T, why string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("no pasó nunca: %s", why)
}

// never checks that cond stays false for long enough to mean something. Used
// for "the match did not start", which is only interesting if it keeps not
// starting for more ticks than the start path needs.
func never(t *testing.T, why string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(300 * time.Millisecond)
	for time.Now().Before(deadline) {
		if cond() {
			t.Fatalf("pasó y no tenía que pasar: %s", why)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// sitConn connects and stops there: a seat in the lobby with no character, the
// state somebody is in from the moment they sign in until they pick one. It
// cannot be counted into a match, which is the thing worth testing.
func sitConn(t *testing.T, w *World) *liveConn {
	t.Helper()
	conn := newLiveConn()
	go w.HandleConn(conn)
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// seatConn nombra un personaje y pide entrar a la cola: los dos gestos que hace
// alguien en el campamento. Son dos mensajes y no uno porque son dos decisiones
// — elegir quién sos no es lo mismo que decir que querés jugar; ver
// seatCharacter.
func seatConn(t *testing.T, w *World, name string) *liveConn {
	t.Helper()
	conn := sitConn(t, w)
	conn.push(t, protocol.TypeJoin, protocol.Join{Name: name})
	conn.push(t, protocol.TypeQueue, protocol.Queue{Join: true})
	return conn
}

// One person is not a match. With the minimum at two, the first to arrive waits
// instead of walking into a world alone — which is the whole reason the lobby
// exists, and the thing the server could not do when a match began with the
// process.
func TestLobbyWaitsForTheMinimum(t *testing.T) {
	w := newTestWorld(t, 40, 40)
	w.SetLobby(2, 0)
	go w.Run(t.Context())

	uno := seatConn(t, w, "uno")
	eventually(t, "el primero recibe el estado del lobby", func() bool {
		var state protocol.LobbyState
		return uno.got(protocol.TypeLobby, &state) && state.Needed == 2
	})
	never(t, "el primero entró al mundo solo", func() bool {
		return uno.got(protocol.TypeWelcome, nil)
	})

	dos := seatConn(t, w, "dos")
	eventually(t, "los dos entran cuando se completa la cola", func() bool {
		return uno.got(protocol.TypeWelcome, nil) && dos.got(protocol.TypeWelcome, nil)
	})
}

// The default is one, and it has to behave exactly like the server that had no
// lobby: connect and play. Every bot and every other test in this package
// depends on it.
func TestLobbyOfOneStartsImmediately(t *testing.T) {
	w := newTestWorld(t, 40, 40)
	go w.Run(t.Context())

	solo := seatConn(t, w, "solo")
	eventually(t, "entra solo, sin esperar a nadie", func() bool {
		return solo.got(protocol.TypeWelcome, nil)
	})
}

// Being connected is not the same as wanting to play. Somebody looking at the
// lobby without taking a place in the line must not make up the numbers for a
// match they never asked to be in.
func TestWatchingWithoutQueueingDoesNotCount(t *testing.T) {
	w := newTestWorld(t, 40, 40)
	w.SetLobby(2, 0)
	go w.Run(t.Context())

	uno := seatConn(t, w, "uno")
	eventually(t, "el primero está en la cola", func() bool {
		var state protocol.LobbyState
		return uno.got(protocol.TypeLobby, &state) && state.Queued == 1
	})

	// El segundo se sienta a mirar sin elegir personaje: hay dos conectados y
	// uno solo esperando, que no alcanza.
	dos := sitConn(t, w)
	never(t, "arrancó con uno solo en la cola", func() bool {
		return uno.got(protocol.TypeWelcome, nil)
	})
	// Y pedir entrar sí la arranca, que es lo que prueba que mirar sin jugar no
	// rompió la cola sino que simplemente no contaba.
	dos.push(t, protocol.TypeJoin, protocol.Join{Name: "dos"})
	dos.push(t, protocol.TypeQueue, protocol.Queue{Join: true})
	eventually(t, "pide entrar y arranca", func() bool {
		return uno.got(protocol.TypeWelcome, nil) && dos.got(protocol.TypeWelcome, nil)
	})
}

// The countdown is what stops the queue filling up from being a starting
// pistol. It also has to be cancellable, because somebody leaving during it
// takes the lobby back under the minimum.
func TestCountdownRunsAndCancels(t *testing.T) {
	w := newTestWorld(t, 40, 40)
	w.SetLobby(2, 3) // 3 s: largo comparado con los 50 ms del tick
	go w.Run(t.Context())

	uno := seatConn(t, w, "uno")
	dos := seatConn(t, w, "dos")

	eventually(t, "la cuenta regresiva arranca al llegar al mínimo", func() bool {
		var state protocol.LobbyState
		return uno.got(protocol.TypeLobby, &state) && state.Counting
	})
	never(t, "arrancó antes de que terminara la cuenta", func() bool {
		return uno.got(protocol.TypeWelcome, nil)
	})

	dos.push(t, protocol.TypeQueue, protocol.Queue{Join: false})
	eventually(t, "la cuenta se cancela al bajar del mínimo", func() bool {
		var state protocol.LobbyState
		return uno.got(protocol.TypeLobby, &state) && !state.Counting && state.Queued == 1
	})
}

// A seat is what survives a match; the player is not. When the match ends,
// everybody who came in through the lobby goes back to it — and comes back
// un-queued, because being put straight back in line takes the decision away
// from somebody who has just been handed a result to read.
//
// Driven synchronously, like the rest of match_test.go: the world's state
// belongs to its own goroutine, so a test that both runs it and reads it is
// testing something other than what it says.
func TestMatchEndReturnsSeatsToTheLobby(t *testing.T) {
	w := matchWorld(t)
	w.SetLobby(2, 0)
	w.SetMatchRestart(1)

	first := w.addSeat(seatReq{conn: &fakeConn{}})
	second := w.addSeat(seatReq{conn: &fakeConn{}})
	if w.match.phase != matchLobby {
		t.Fatalf("sentarse no encola: la fase es %v, se esperaba matchLobby", w.match.phase)
	}

	w.seatCharacter(charReq{seat: first.id, name: "uno"})
	w.setQueued(queueReq{seat: first.id, join: true})
	if w.match.phase != matchLobby {
		t.Fatalf("con uno solo en la cola la fase es %v, se esperaba matchLobby", w.match.phase)
	}
	w.seatCharacter(charReq{seat: second.id, name: "dos"})
	w.setQueued(queueReq{seat: second.id, join: true})
	if w.match.phase != matchRunning {
		t.Fatalf("con dos en la cola la fase es %v, se esperaba matchRunning", w.match.phase)
	}

	uno := w.players[<-first.started]
	dos := w.players[<-second.started]
	if uno == nil || dos == nil {
		t.Fatal("el lobby no entregó las dos entidades")
	}
	if first.playing == 0 || second.playing == 0 {
		t.Error("los asientos no quedaron marcados como jugando")
	}

	w.kill(dos, uno)
	w.matchTick()
	if w.match.phase != matchOver {
		t.Fatalf("fase = %v, se esperaba matchOver", w.match.phase)
	}

	// Adelantar el reloj hasta el reinicio, que es lo que dispara la vuelta.
	w.tick = w.match.restartAt
	w.matchTick()

	if w.match.phase != matchLobby {
		t.Errorf("fase = %v, se esperaba volver a matchLobby", w.match.phase)
	}
	if len(w.players) != 0 {
		t.Errorf("quedaron %d jugadores en el mundo, se esperaba ninguno", len(w.players))
	}
	if first.playing != 0 || second.playing != 0 {
		t.Error("los asientos siguen marcados como jugando")
	}
	if first.queued || second.queued {
		t.Error("volvieron al lobby ya encolados: la decisión de jugar otra es de ellos")
	}
	// Y los asientos siguen ahí: es la conexión la que sobrevive a la partida.
	if len(w.lobby.seats) != 2 {
		t.Errorf("quedaron %d asientos, se esperaban 2", len(w.lobby.seats))
	}
}

// A direct Join has no seat, so it has no lobby to go back to. That path is the
// load-testing bot and every other test in this package, and for it the old
// in-place restart is the only behaviour that means anything.
func TestPlayersWithoutASeatStillRestartInPlace(t *testing.T) {
	w := matchWorld(t)
	w.SetMatchRestart(1)

	a, _ := place(t, w, "a", 5, 5)
	b, _ := place(t, w, "b", 7, 7)

	w.kill(b, a)
	w.matchTick()
	if w.match.phase != matchOver {
		t.Fatalf("fase = %v, se esperaba matchOver", w.match.phase)
	}

	w.tick = w.match.restartAt
	w.matchTick()

	if w.match.phase != matchRunning {
		t.Errorf("fase = %v, sin asientos la partida se reinicia en el lugar", w.match.phase)
	}
	if len(w.players) != 2 {
		t.Errorf("quedaron %d jugadores, se esperaban los 2 de siempre", len(w.players))
	}
}

// Entrar a la cuenta es llegar al campamento: el estado del lobby tiene que
// llegar antes de que exista un personaje, porque es la pantalla donde se
// decide jugar y todavía no se eligió nada.
func TestLobbyArrivesBeforeAnyCharacter(t *testing.T) {
	w := newTestWorld(t, 40, 40)
	w.SetLobby(2, 0)
	go w.Run(t.Context())

	mirando := sitConn(t, w)
	eventually(t, "el lobby llega sin haber mandado un join", func() bool {
		var state protocol.LobbyState
		return mirando.got(protocol.TypeLobby, &state) && state.Needed == 2 && !state.Mine
	})
	never(t, "entró al mundo sin personaje", func() bool {
		return mirando.got(protocol.TypeWelcome, nil)
	})

	// Elegir personaje tampoco lo encola: sigue mirando, ahora con un personaje
	// elegido. Son dos gestos distintos.
	mirando.push(t, protocol.TypeJoin, protocol.Join{Name: "wachin"})
	never(t, "el join solo lo encoló", func() bool {
		var state protocol.LobbyState
		return mirando.got(protocol.TypeLobby, &state) && state.Mine
	})

	// Y recién el "quiero jugar" lo mete en la cola.
	mirando.push(t, protocol.TypeQueue, protocol.Queue{Join: true})
	eventually(t, "pedir entrar lo encola", func() bool {
		var state protocol.LobbyState
		return mirando.got(protocol.TypeLobby, &state) && state.Mine && state.Queued == 1
	})
}
