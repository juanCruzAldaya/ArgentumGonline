package transport

import (
	"errors"
	"sync"
)

// Pipe es un par de Conn conectados entre sí, sin red en el medio.
//
// Existe para los bots que rellenan una partida: corren dentro del proceso del
// servidor, pero hablan el mismo protocolo por el mismo tipo de conexión que
// alguien conectado desde su casa. Eso es lo que mantiene la promesa que hace
// OPERACION §1 — "del lado del servidor no existe el concepto de bot" — con un
// relleno que vive adentro: World.HandleConn recibe un transport.Conn y no
// tiene forma de saber cuál de los dos le tocó.
//
// Lo que NO imita es la red: acá no hay latencia, ni jitter, ni frames
// perdidos. Un bot de relleno juega con ping cero, que es una ventaja real
// sobre la persona que está jugando contra él — ver el comentario de latencia
// en internal/bot sobre por qué eso se compensa del lado del bot y no acá.
func Pipe() (Conn, Conn) {
	a, b := make(chan []byte, pipeBuffer), make(chan []byte, pipeBuffer)
	done := make(chan struct{})
	var once sync.Once
	closeBoth := func() { once.Do(func() { close(done) }) }

	return &pipeConn{out: a, in: b, done: done, shut: closeBoth, who: "bot"},
		&pipeConn{out: b, in: a, done: done, shut: closeBoth, who: "bot"}
}

// pipeBuffer es cuántos frames aguanta cada dirección antes de que Send
// devuelva ErrBackpressure. El mismo trato que un cliente lento por red: el
// mundo descarta su frame en vez de esperarlo, y si acumula suficientes lo
// desconecta. Un bot que no drena tiene que morir como moriría una persona con
// la conexión tapada, no frenar la simulación de todos.
const pipeBuffer = 32

type pipeConn struct {
	out  chan []byte
	in   chan []byte
	done chan struct{}
	shut func()
	who  string
}

func (p *pipeConn) Send(frame []byte) error {
	// Copiar es obligatorio y no una precaución: el que llama reusa su buffer
	// entre frames, y sin la copia el receptor leería lo que ya fue pisado.
	// Por red esto pasa gratis porque el frame se serializa al socket.
	cp := append([]byte(nil), frame...)
	select {
	case <-p.done:
		return ErrClosed
	case p.out <- cp:
		return nil
	default:
		return ErrBackpressure
	}
}

func (p *pipeConn) Recv() ([]byte, error) {
	select {
	case <-p.done:
		return nil, ErrClosed
	case frame := <-p.in:
		return frame, nil
	}
}

// Close cierra las dos puntas: no hay media conexión que sobreviva a la otra,
// igual que un socket.
func (p *pipeConn) Close() error {
	p.shut()
	return nil
}

func (p *pipeConn) RemoteAddr() string { return p.who }

// ErrClosed es lo que devuelven Send y Recv sobre una conexión ya cerrada.
var ErrClosed = errors.New("transport: conexión cerrada")
