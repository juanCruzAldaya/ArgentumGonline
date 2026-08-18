// Command server runs the authoritative game server.
//
// It is headless: no rendering, no engine, just the simulation and a socket.
// Godot is only ever a client.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"juegito/server/internal/protocol"
	"juegito/server/internal/transport"
	"juegito/server/internal/world"
)

// The three data files the game is actually played with. They used to default
// to "", which started a server on a generated empty arena with no items and
// no spells — a state that looks like a broken game rather than like a missing
// flag, and that shipped to production once. The defaults now point at the
// files that live in server/maps, and a missing one is a hard error instead of
// a silent downgrade.
const (
	defaultMapFile    = "maps/map1.json"
	defaultItemsFile  = "maps/items.json"
	defaultSpellsFile = "maps/spells.json"
)

func main() {
	var (
		addr       = flag.String("addr", ":8080", "address to listen on")
		tickRate   = flag.Int("tick", 20, "simulation ticks per second")
		mapW       = flag.Int("map-width", 100, "generated arena width in tiles; only used with -map-file=\"\"")
		mapH       = flag.Int("map-height", 100, "generated arena height in tiles; only used with -map-file=\"\"")
		seed       = flag.Int64("seed", 1, "generated arena seed; only used with -map-file=\"\"")
		debug      = flag.Bool("debug", false, "enable debug logging")
		webDir     = flag.String("web-dir", "", "directory holding the exported web client; empty disables it")
		mapFile    = flag.String("map-file", defaultMapFile, "converted Argentum map to play on; -map-file=\"\" plays the generated demo arena instead")
		itemsFile  = flag.String("items-file", defaultItemsFile, "converted obj.dat; -items-file=\"\" leaves every weapon and armour unknown")
		spellsFile = flag.String("spells-file", defaultSpellsFile, "converted Hechizos.dat; -spells-file=\"\" leaves nothing castable")
		respawn    = flag.Int("respawn", 5, "seconds a dead player stays a ghost before coming back in the middle of the map; 0 is the genre's own rule, elimination")
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
		loaded, number, name, err := world.LoadMap(dataFile(log, "-map-file", *mapFile))
		if err != nil {
			log.Error("no se pudo cargar el mapa", "err", err)
			os.Exit(1)
		}
		grid, mapNumber, mapName = loaded, number, name
		log.Info("mapa cargado", "number", number, "name", name, "size", [2]int{grid.W, grid.H})
	}

	w := world.New(grid, protocol.JSONCodec{}, *tickRate, log)
	w.SetMap(mapNumber, mapName)
	// A playtest affordance, not the game's rule: a battle royale eliminates
	// you. It defaults on because testing a fight otherwise means restarting
	// the client after every death; -respawn 0 gives permadeath back.
	w.SetRespawnDelay(*respawn)
	if *respawn > 0 {
		log.Info("respawn habilitado", "segundos", *respawn)
	}

	itemCount, spellCount := 0, 0
	if *itemsFile != "" {
		items, err := world.LoadItems(dataFile(log, "-items-file", *itemsFile))
		if err != nil {
			log.Error("no se pudieron cargar los items", "err", err)
			os.Exit(1)
		}
		w.SetItems(items)
		itemCount = len(items)
		log.Info("items cargados", "count", itemCount)
	}

	if *spellsFile != "" {
		spells, err := world.LoadSpells(dataFile(log, "-spells-file", *spellsFile))
		if err != nil {
			log.Error("no se pudieron cargar los hechizos", "err", err)
			os.Exit(1)
		}
		w.SetSpells(spells)
		spellCount = len(spells)
		log.Info("hechizos cargados", "count", spellCount)
	}

	status := gameStatus(mapName, itemCount, spellCount)
	if strings.HasPrefix(status, "degradado") {
		// Reachable only by asking for it: every data flag now defaults to a
		// real file and a missing one exits above. Say so anyway, because the
		// symptom on screen ("no conozco los hechizos, el mapa no renderiza")
		// never pointed back at the flag that caused it.
		log.Warn("arrancando sin datos de juego", "estado", status)
	}

	go w.Run(ctx)

	srv := &transport.WSServer{
		Addr:      *addr,
		Handler:   w.HandleConn,
		Logger:    log,
		StaticDir: *webDir,
		Health:    func() string { return status },
	}
	if err := srv.ListenAndServe(ctx); err != nil {
		log.Error("server failed", "err", err)
		os.Exit(1)
	}
	log.Info("server stopped")
}

// dataFile resolves one of the data paths, exiting with instructions if it is
// not there. Running from server/ is the documented way and makes the relative
// defaults resolve on their own, but a built binary sitting next to its own
// maps/ directory is just as reasonable, so try that before giving up.
func dataFile(log *slog.Logger, flagName, path string) string {
	if _, err := os.Stat(path); err == nil {
		return path
	}
	if !filepath.IsAbs(path) {
		if exe, err := os.Executable(); err == nil {
			alt := filepath.Join(filepath.Dir(exe), path)
			if _, err := os.Stat(alt); err == nil {
				return alt
			}
		}
	}
	log.Error("falta un archivo de datos del juego", "flag", flagName, "path", path)
	log.Error(`corré el servidor desde server/ (go run ./cmd/server) para que los defaults resuelvan, ` +
		`o pasá una ruta con ` + flagName + `. Para la arena generada de prueba, sin items ni hechizos: ` + flagName + `=""`)
	os.Exit(1)
	return ""
}

// gameStatus is the body of /healthz. "ok" only ever meant "the process is
// up", which is exactly the question that could not tell a real server from
// one on an empty arena — so it now carries what loaded.
func gameStatus(mapName string, items, spells int) string {
	var missing []string
	if mapName == "" {
		missing = append(missing, "sin mapa (arena generada)")
	}
	if items == 0 {
		missing = append(missing, "sin items")
	}
	if spells == 0 {
		missing = append(missing, "sin hechizos")
	}
	if len(missing) > 0 {
		return "degradado: " + strings.Join(missing, ", ")
	}
	return fmt.Sprintf("ok map=%q items=%d spells=%d", mapName, items, spells)
}
