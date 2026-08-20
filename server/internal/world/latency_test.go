package world

import (
	"testing"
	"time"
)

// A fixed clock: every test decides for itself how long ago the last ping went
// out, because that is what separates "still travelling" from "lost".
var latencyBase = time.Date(2026, 8, 20, 23, 0, 0, 0, time.UTC)

// settled is a moment long enough after the last ping that nothing can still be
// in flight.
func settled() time.Time { return latencyBase.Add(10 * time.Second) }

func TestLatencySummaryReportsPercentiles(t *testing.T) {
	var lat latencyTracker
	// Ten samples, one of them a spike. A mean would report 58 ms and describe
	// a round trip that never happened; the point of the percentiles is that
	// p50 stays where the connection actually lives.
	for _, ms := range []int64{40, 41, 42, 43, 44, 45, 46, 47, 48, 400} {
		lat.ping(latencyBase)
		lat.sample(ms)
	}

	p50, p95, max, got, lost := lat.summary(settled())
	if got != 10 || lost != 0 {
		t.Fatalf("muestras=%d sin_respuesta=%d, esperaba 10 y 0", got, lost)
	}
	if p50 != 44 {
		t.Errorf("p50=%d, esperaba 44", p50)
	}
	// El p95 trunca hacia abajo, asi que en una ventana corta es "lo peor sin
	// contar el pico" y no el pico. Es a proposito, y es la razon por la que el
	// max viaja al lado: con quince muestras por ventana, un p95 redondeado
	// hacia arriba seria el maximo siempre y sobraria una de las dos columnas.
	if p95 != 48 {
		t.Errorf("p95=%d, esperaba 48", p95)
	}
	if max != 400 {
		t.Errorf("max=%d, esperaba que el pico lo reporte el max", max)
	}
}

// A ping that never came back is not a fast round trip. Send drops frames
// rather than blocking when a client stops draining, so the pings lost are
// exactly the moment worth seeing, and averaging them away as if they had been
// answered instantly would hide it.
func TestLatencySummaryCountsUnanswered(t *testing.T) {
	var lat latencyTracker
	for range 5 {
		lat.ping(latencyBase)
	}
	lat.sample(30)
	lat.sample(31)

	if _, _, _, got, lost := lat.summary(settled()); got != 2 || lost != 3 {
		t.Fatalf("muestras=%d sin_respuesta=%d, esperaba 2 y 3", got, lost)
	}
}

// The reporting interval is a multiple of the ping interval, so the two timers
// fire together and the last ping is always mid-flight when the window closes.
// Counting it as lost reported 7% packet loss on a connection that had none.
func TestLatencyDoesNotCountThePingStillInFlight(t *testing.T) {
	var lat latencyTracker
	for range 3 {
		lat.ping(latencyBase)
		lat.sample(40)
	}
	lat.ping(latencyBase.Add(30 * time.Second)) // el que acaba de salir

	_, _, _, got, lost := lat.summary(latencyBase.Add(30 * time.Second))
	if got != 3 || lost != 0 {
		t.Fatalf("muestras=%d sin_respuesta=%d, esperaba 3 y 0", got, lost)
	}

	// Y se lo lleva la ventana siguiente, que es la que puede decir si llego.
	if _, _, _, _, lost := lat.summary(settled().Add(time.Minute)); lost != 1 {
		t.Fatalf("sin_respuesta=%d en la ventana siguiente, esperaba 1", lost)
	}
}

// The client echoes a timestamp this server wrote, so a client that echoes an
// old one — or an invented one — is reporting a latency of its own choosing.
func TestLatencyDiscardsImpossibleSamples(t *testing.T) {
	var lat latencyTracker
	for range 3 {
		lat.ping(latencyBase)
	}
	lat.sample(-1)
	lat.sample(discardAbove + 1)
	lat.sample(50)

	p50, _, max, got, _ := lat.summary(settled())
	if got != 1 || p50 != 50 || max != 50 {
		t.Fatalf("muestras=%d p50=%d max=%d, esperaba que solo entre la de 50 ms", got, p50, max)
	}
}

// Each line covers the stretch since the last one: a connection that goes bad
// ten minutes in has to show it on the next line instead of being averaged
// back into the good half hour before it.
func TestLatencySummaryEmptiesItsWindow(t *testing.T) {
	var lat latencyTracker
	lat.ping(latencyBase)
	lat.sample(40)
	lat.ping(latencyBase)
	lat.sample(42)
	lat.summary(settled())

	if _, _, _, got, lost := lat.summary(settled()); got != 0 || lost != 0 {
		t.Fatalf("la ventana no se vacio: muestras=%d sin_respuesta=%d", got, lost)
	}
}
