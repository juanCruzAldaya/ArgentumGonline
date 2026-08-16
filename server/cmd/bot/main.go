// Command bot runs headless clients that wander the map.
//
// Two people cannot fill a 50-player match by hand, so the load test has to be
// synthetic. Bots speak exactly the protocol a real client speaks — there is no
// server-side "bot" concept — which means anything that breaks under bots would
// have broken under players too.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"math/rand"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"juegito/server/internal/protocol"
	"juegito/server/internal/transport"
)

func main() {
	var (
		url      = flag.String("url", "ws://127.0.0.1:8080/ws", "server websocket URL")
		count    = flag.Int("n", 10, "number of bots to run")
		prefix   = flag.String("prefix", "bot", "display name prefix")
		interval = flag.Duration("move-interval", 200*time.Millisecond, "time between move attempts")
		stagger  = flag.Duration("stagger", 50*time.Millisecond, "delay between bot connections")
		debug    = flag.Bool("debug", false, "enable debug logging")
	)
	flag.Parse()

	level := slog.LevelInfo
	if *debug {
		level = slog.LevelDebug
	}
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level}))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var wg sync.WaitGroup
	for i := 0; i < *count; i++ {
		// Connecting all at once would measure the accept path rather than the
		// steady state we actually care about.
		select {
		case <-ctx.Done():
			return
		case <-time.After(*stagger):
		}

		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			name := fmt.Sprintf("%s%02d", *prefix, n)
			if err := runBot(ctx, *url, name, *interval, log); err != nil && ctx.Err() == nil {
				log.Error("bot stopped", "name", name, "err", err)
			}
		}(i)
	}

	log.Info("bots running", "count", *count, "url", *url)
	wg.Wait()
}

func runBot(ctx context.Context, url, name string, interval time.Duration, log *slog.Logger) error {
	dialCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	conn, err := transport.DialWS(dialCtx, url)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	defer conn.Close()

	codec := protocol.JSONCodec{}
	rng := rand.New(rand.NewSource(time.Now().UnixNano() + int64(len(name))))

	// Bots pick their own class and race before joining, the same as a real
	// player's character picker would. The server has no "randomize this for
	// me" path — a Join always names a real selection — so an all-Guerrero,
	// all-Humano bot swarm would be this loop's fault, not the server's.
	// classCount/raceCount must stay in step with world.allClasses/allRaces;
	// cmd/bot deliberately doesn't import internal/world just to read two
	// slice lengths.
	const classCount, raceCount = 12, 5
	join, err := codec.Encode(protocol.TypeJoin, protocol.Join{
		Name:  name,
		Class: rng.Intn(classCount),
		Race:  rng.Intn(raceCount),
	})
	if err != nil {
		return err
	}
	if err := conn.Send(join); err != nil {
		return fmt.Errorf("join: %w", err)
	}

	// Snapshots must be drained or the server's send queue fills and the bot
	// gets disconnected as a slow client — exactly as a real client would.
	go func() {
		<-ctx.Done()
		_ = conn.Close()
	}()
	go func() {
		for {
			frame, err := conn.Recv()
			if err != nil {
				return
			}
			if typ, _, err := codec.DecodeEnvelope(frame); err == nil {
				log.Debug("bot received", "name", name, "type", typ)
			}
		}
	}()

	dir := protocol.Heading(rng.Intn(4))

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			// Mostly hold a heading, occasionally turn: a pure random walk
			// barely leaves its spawn tile and would not exercise the viewport
			// entering and leaving that snapshots are built around.
			if rng.Intn(4) == 0 {
				dir = protocol.Heading(rng.Intn(4))
			}
			move, err := codec.Encode(protocol.TypeMove, protocol.Move{Dir: dir})
			if err != nil {
				return err
			}
			if err := conn.Send(move); err != nil {
				return fmt.Errorf("send move: %w", err)
			}
		}
	}
}
