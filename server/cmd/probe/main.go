// probe conecta como un jugador más y mide lo que un jugador siente: cuánto
// tarda el servidor en contestarle, con qué regularidad le llegan los
// snapshots, y cuántos bytes le cuesta todo eso.
//
// Existe porque las tres preguntas que más se hacen sobre este servidor —
// "¿cuánto lag hay?", "¿cuánto sale tenerlo prendido?" y "¿aguanta 50
// jugadores?" — se venían respondiendo estimando. Los números de OPERACION §7
// salen de acá.
//
//	go run ./cmd/probe -url wss://juegito.fly.dev/ws
package main

import (
	"context"
	"flag"
	"fmt"
	"math"
	"math/rand"
	"sort"
	"time"

	"juegito/server/internal/protocol"
	"juegito/server/internal/transport"
)

func main() {
	url := flag.String("url", "ws://127.0.0.1:8080/ws", "servidor al que medirle")
	window := flag.Duration("for", 30*time.Second, "cuánto medir")
	every := flag.Duration("ping-every", 250*time.Millisecond, "cada cuánto mandar un ping")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), *window+30*time.Second)
	defer cancel()

	dialCtx, dialCancel := context.WithTimeout(ctx, 20*time.Second)
	defer dialCancel()
	conn, err := transport.DialWS(dialCtx, *url)
	if err != nil {
		fmt.Println("dial:", err)
		return
	}
	defer conn.Close()

	codec := protocol.JSONCodec{}
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	join, err := codec.Encode(protocol.TypeJoin, protocol.Join{
		Name: "probe", Class: rng.Intn(12), Race: rng.Intn(5),
	})
	if err != nil {
		fmt.Println("encode:", err)
		return
	}
	if err := conn.Send(join); err != nil {
		fmt.Println("join:", err)
		return
	}

	var rtts, gaps []float64
	var total, snapshotBytes, welcomeBytes int
	var snapshots int
	inFlight := map[int64]time.Time{}
	var lastSnapshot time.Time

	start := time.Now()
	deadline := start.Add(*window)
	nextPing := start

	for time.Now().Before(deadline) {
		// El ping usa el propio protocolo: session.go devuelve el timestamp
		// tal cual, sin que el servidor guarde estado de reloj, así que la
		// resta se hace enteramente de este lado.
		if now := time.Now(); now.After(nextPing) {
			payload, err := codec.Encode(protocol.TypePing, protocol.Ping{T: now.UnixNano()})
			if err == nil && conn.Send(payload) == nil {
				inFlight[now.UnixNano()] = now
			}
			nextPing = now.Add(*every)
		}

		frame, err := conn.Recv()
		if err != nil {
			fmt.Println("recv:", err)
			break
		}
		total += len(frame)

		kind, raw, err := codec.DecodeEnvelope(frame)
		if err != nil {
			continue
		}
		switch kind {
		case protocol.TypePong:
			var pong protocol.Pong
			if codec.DecodePayload(raw, &pong) == nil {
				if at, ok := inFlight[pong.T]; ok {
					rtts = append(rtts, ms(time.Since(at)))
					delete(inFlight, pong.T)
				}
			}
		case protocol.TypeWelcome:
			welcomeBytes += len(frame)
		case protocol.TypeSnapshot:
			snapshots++
			snapshotBytes += len(frame)
			if now := time.Now(); !lastSnapshot.IsZero() {
				gaps = append(gaps, ms(now.Sub(lastSnapshot)))
				lastSnapshot = now
			} else {
				lastSnapshot = now
			}
		}
	}

	elapsed := time.Since(start).Seconds()
	fmt.Printf("contra %s, %.0fs\n\n", *url, elapsed)
	report("ping/pong (ida y vuelta)", rtts)
	report("llegada de snapshots    ", gaps)

	fmt.Printf("\nvolumen   %d bytes en %.0fs = %.1f KB/s = %.0f MB/hora por jugador\n",
		total, elapsed, float64(total)/elapsed/1024, float64(total)/elapsed*3600/1024/1024)
	fmt.Printf("          welcome %d bytes (una sola vez), %d snapshots de %d bytes promedio\n",
		welcomeBytes, snapshots, snapshotBytes/max(snapshots, 1))

	if len(rtts) > 0 {
		// El mundo aplica los comandos encolados en el tick siguiente, así que
		// uno cae en algún punto de [0, 50ms): 25 en promedio. Eso es latencia
		// que ninguna mejora de red saca.
		const tickMs = 1000.0 / 20
		fmt.Printf("\nlo que siente el jugador: %.0f ms de red + %.0f ms de espera de tick = %.0f ms\n",
			percentile(rtts, 50), tickMs/2, percentile(rtts, 50)+tickMs/2)
	}
}

func ms(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }

// report imprime la distribución, no el promedio: en una conexión con jitter
// la mediana miente y el desvío es el número que explica la sensación.
func report(label string, xs []float64) {
	if len(xs) == 0 {
		fmt.Printf("%s  sin datos\n", label)
		return
	}
	sum := 0.0
	for _, x := range xs {
		sum += x
	}
	mean := sum / float64(len(xs))
	varsum := 0.0
	for _, x := range xs {
		varsum += (x - mean) * (x - mean)
	}
	fmt.Printf("%s  n=%-4d min %6.1f  mediana %6.1f  p95 %6.1f  max %6.1f  desvío %5.1f ms\n",
		label, len(xs), percentile(xs, 0), percentile(xs, 50), percentile(xs, 95),
		percentile(xs, 100), math.Sqrt(varsum/float64(len(xs))))
}

func percentile(xs []float64, p int) float64 {
	s := append([]float64(nil), xs...)
	sort.Float64s(s)
	switch {
	case p <= 0:
		return s[0]
	case p >= 100:
		return s[len(s)-1]
	}
	return s[p*(len(s)-1)/100]
}
