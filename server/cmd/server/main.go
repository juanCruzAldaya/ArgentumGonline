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
		addr      = flag.String("addr", ":8080", "address to listen on")
		tickRate  = flag.Int("tick", 20, "simulation ticks per second")
		mapW      = flag.Int("map-width", 100, "demo map width in tiles")
		mapH      = flag.Int("map-height", 100, "demo map height in tiles")
		seed      = flag.Int64("seed", 1, "demo map generation seed")
		debug     = flag.Bool("debug", false, "enable debug logging")
		webDir    = flag.String("web-dir", "", "directory holding the exported web client; empty disables it")
		mapFile   = flag.String("map-file", "", "converted Argentum map to play on; empty uses the generated demo arena")
		itemsFile = flag.String("items-file", "", "converted obj.dat; without it every weapon and armour is unknown")
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
	mapNumber, mapName := 0, ""
	if *mapFile != "" {
		loaded, number, name, err := world.LoadMap(*mapFile)
		if err != nil {
			log.Error("no se pudo cargar el mapa", "err", err)
			os.Exit(1)
		}
		grid, mapNumber, mapName = loaded, number, name
		log.Info("mapa cargado", "number", number, "name", name, "size", [2]int{grid.W, grid.H})
	}

	w := world.New(grid, protocol.JSONCodec{}, *tickRate, log)
	w.SetMap(mapNumber, mapName)

	if *itemsFile != "" {
		items, err := world.LoadItems(*itemsFile)
		if err != nil {
			log.Error("no se pudieron cargar los items", "err", err)
			os.Exit(1)
		}
		w.SetItems(items)
		log.Info("items cargados", "count", len(items))
	}

	go w.Run(ctx)

	srv := &transport.WSServer{
		Addr:      *addr,
		Handler:   w.HandleConn,
		Logger:    log,
		StaticDir: *webDir,
	}
	if err := srv.ListenAndServe(ctx); err != nil {
		log.Error("server failed", "err", err)
		os.Exit(1)
	}
	log.Info("server stopped")
}
