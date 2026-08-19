// Command bot runs headless clients that wander the map.
//
// With -pass they register an account and sign in first, so a swarm can be
// pointed at a server started with -accounts. Without it they go straight to
// the join, which is all a server without accounts wants.
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
		password = flag.String("pass", "", "contraseña con la que registrarse en un servidor que pide cuenta; vacío asume un servidor sin cuentas")
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
			if err := runBot(ctx, *url, name, *password, *interval, log); err != nil && ctx.Err() == nil {
				log.Error("bot stopped", "name", name, "err", err)
			}
		}(i)
	}

	log.Info("bots running", "count", *count, "url", *url)
	wg.Wait()
}

func runBot(ctx context.Context, url, name, password string, interval time.Duration, log *slog.Logger) error {
	dialCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	conn, err := transport.DialWS(dialCtx, url)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	defer conn.Close()

	codec := protocol.JSONCodec{}
	rng := rand.New(rand.NewSource(time.Now().UnixNano() + int64(len(name))))

	// A server with accounts speaks first and refuses anything before a login,
	// so a swarm without -pass could never reach the production configuration —
	// exactly the one worth load testing. Registering rather than requiring the
	// accounts to exist keeps the swarm a one-liner; a name already taken falls
	// through to signing in, which is what happens on every run after the first.
	if password != "" {
		if err := signIn(conn, codec, name, password); err != nil {
			return err
		}
	}

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

	// And take a place in the queue. On the default one-player lobby the join
	// above already did it and this is a no-op; against a server started with
	// -lobby-min it is the difference between a swarm that plays and a swarm
	// that sits in the waiting room watching each other.
	queue, err := codec.Encode(protocol.TypeQueue, protocol.Queue{Join: true})
	if err != nil {
		return err
	}
	if err := conn.Send(queue); err != nil {
		return fmt.Errorf("queue: %w", err)
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
	var seq uint32

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
			seq++
			move, err := codec.Encode(protocol.TypeMove, protocol.Move{Dir: dir, Seq: seq})
			if err != nil {
				return err
			}
			if err := conn.Send(move); err != nil {
				return fmt.Errorf("send move: %w", err)
			}
		}
	}
}

// signIn registers this bot's account and signs in, tolerating the name already
// existing from an earlier run.
//
// The email is required by the server and is deliberately obvious junk on a
// .invalid domain, which RFC 2606 reserves precisely so that nothing anybody
// writes can ever reach a real mailbox.
func signIn(conn transport.Conn, codec protocol.JSONCodec, name, password string) error {
	send := func(register bool) error {
		login := protocol.Login{Name: name, Password: password, Register: register}
		if register {
			login.Email = name + "@bots.invalid"
		}
		frame, err := codec.Encode(protocol.TypeLogin, login)
		if err != nil {
			return err
		}
		return conn.Send(frame)
	}

	if err := send(true); err != nil {
		return fmt.Errorf("registro: %w", err)
	}

	// The server answers a login with an account card on success and an error
	// on failure, and keeps the connection open either way so a typo does not
	// cost a reconnect. Two attempts is all this needs: register, then sign in.
	for attempts := 0; attempts < 2; attempts++ {
		frame, err := conn.Recv()
		if err != nil {
			return fmt.Errorf("login: %w", err)
		}
		typ, _, err := codec.DecodeEnvelope(frame)
		if err != nil {
			continue
		}
		switch typ {
		case protocol.TypeAccount:
			return nil
		case protocol.TypeError:
			// Almost certainly "ese nombre ya está tomado" from a previous run.
			if err := send(false); err != nil {
				return fmt.Errorf("login: %w", err)
			}
		}
	}
	return fmt.Errorf("login rechazado para %s", name)
}
