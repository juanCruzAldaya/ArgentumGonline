// Command bot corre clientes headless contra un servidor por la red.
//
// Con -pass se registran y entran a una cuenta antes de jugar, así el swarm se
// puede apuntar a un servidor arrancado con -accounts. Sin eso van directo al
// join, que es todo lo que quiere un servidor sin cuentas.
//
// Dos personas no llenan una partida de cincuenta, así que la prueba de carga
// tiene que ser sintética. Hablan exactamente el protocolo que habla un cliente
// real — del lado del servidor no existe el concepto de "bot" — así que lo que
// se rompe con bots se hubiera roto con jugadores.
//
// El cerebro y el loop viven en internal/bot, compartidos con el relleno que el
// servidor corre adentro suyo (-fill). Un solo bot, dos formas de largarlo: por
// la red desde acá, o por un transport.Pipe desde el propio servidor.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"juegito/server/internal/bot"
	"juegito/server/internal/protocol"
	"juegito/server/internal/transport"
)

func main() {
	def := bot.DefaultTemper()
	var (
		url      = flag.String("url", "ws://127.0.0.1:8080/ws", "server websocket URL")
		count    = flag.Int("n", 10, "number of bots to run")
		prefix   = flag.String("prefix", "bot", "display name prefix")
		interval = flag.Duration("move-interval", 200*time.Millisecond, "time between move attempts")
		stagger  = flag.Duration("stagger", 50*time.Millisecond, "delay between bot connections")
		password = flag.String("pass", "", "contraseña con la que registrarse en un servidor que pide cuenta; vacío asume un servidor sin cuentas")
		debug    = flag.Bool("debug", false, "enable debug logging")
		sight    = flag.Int("sight", def.Sight, "a cuántos tiles un bot se da cuenta de que hay alguien; 0 los deja paseando sin pelear")
		sloppy   = flag.Float64("sloppy", def.Sloppy, "con qué frecuencia hace cualquier cosa en vez de lo que le conviene, y con qué frecuencia le erra al clic de un hechizo, 0 a 1")
		focus    = flag.Duration("focus", def.Focus, "cuánto le dura una decisión antes de volver a pensarla")
		react    = flag.Duration("react", def.React, "cuánto tarda como mínimo entre una acción deliberada y la siguiente")
		hurt     = flag.Float64("hurt", def.Hurt, "con qué fracción de vida se acuerda de tomar una roja")
		drained  = flag.Float64("drained", def.Drained, "con qué fracción de maná se acuerda de tomar una azul")
	)
	flag.Parse()

	level := slog.LevelInfo
	if *debug {
		level = slog.LevelDebug
	}
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level}))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	temper := bot.NewTemper(*sight, *sloppy, *focus, *react, *hurt, *drained)

	var wg sync.WaitGroup
	for i := range *count {
		// Conectarlos todos de golpe mediría el camino de accept en vez del
		// estado estacionario, que es lo que interesa.
		select {
		case <-ctx.Done():
			return
		case <-time.After(*stagger):
		}

		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			name := fmt.Sprintf("%s%02d", *prefix, n)
			if err := dialAndPlay(ctx, *url, name, *password, *interval, temper); err != nil && ctx.Err() == nil {
				log.Error("bot stopped", "name", name, "err", err)
			}
		}(i)
	}

	log.Info("bots running", "count", *count, "url", *url)
	wg.Wait()
}

func dialAndPlay(ctx context.Context, url, name, password string, interval time.Duration, t bot.Temper) error {
	dialCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	conn, err := transport.DialWS(dialCtx, url)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	defer conn.Close()

	// Un servidor con cuentas habla primero y rechaza cualquier cosa antes de
	// un login, así que un swarm sin -pass nunca podría alcanzar la
	// configuración de producción — justamente la que vale la pena medir.
	// Registrarse en vez de exigir que las cuentas ya existan mantiene al swarm
	// en una línea; un nombre ya tomado cae en el ingreso, que es lo que pasa
	// en toda corrida después de la primera.
	if password != "" {
		if err := bot.SignIn(conn, protocol.JSONCodec{}, name, password); err != nil {
			return err
		}
	}

	return bot.Play(ctx, conn, bot.Config{
		Name:     name,
		Seed:     time.Now().UnixNano() + int64(len(name)),
		Interval: interval,
		Temper:   t,
	})
}
