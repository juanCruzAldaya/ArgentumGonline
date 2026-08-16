// Command server runs the authoritative game server.
//
// It is headless: no rendering, no engine, just the simulation and a socket.
// Godot is only ever a client.
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"juegito/server/internal/protocol"
	"juegito/server/internal/transport"
	"juegito/server/internal/world"
)

func main() {
	var (
		addr     = flag.String("addr", ":8080", "address to listen on")
		tickRate = flag.Int("tick", 20, "simulation ticks per second")
		mapW     = flag.Int("map-width", 100, "demo map width in tiles")
		mapH     = flag.Int("map-height", 100, "demo map height in tiles")
		seed     = flag.Int64("seed", 1, "demo map generation seed")
		debug    = flag.Bool("debug", false, "enable debug logging")
	)
	flag.Parse()

	// Honour PORT so the same binary runs unchanged on platforms that inject it.
	if port := os.Getenv("PORT"); port != "" && *addr == ":8080" {
		*addr = ":" + port
	}

	level := slog.LevelInfo
	if *debug {
		level = slog.LevelDebug
	}
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level}))

	if *tickRate <= 0 {
		log.Error("tick rate must be positive", "tick", *tickRate)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	grid := world.GenerateDemoMap(*mapW, *mapH, *seed)
	w := world.New(grid, protocol.JSONCodec{}, *tickRate, log)
	go w.Run(ctx)

	srv := &transport.WSServer{
		Addr:    *addr,
		Handler: w.HandleConn,
		Logger:  log,
	}
	if err := srv.ListenAndServe(ctx); err != nil {
		log.Error("server failed", "err", err)
		os.Exit(1)
	}
	log.Info("server stopped")
}
