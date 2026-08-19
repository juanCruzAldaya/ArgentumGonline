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
	"math/rand"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"juegito/server/internal/account"
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

	// defaultWorlds is the pattern the composed worlds are written under by
	// tools/aoconv -worlds. A match runs on one of them, drawn at startup, so
	// that two servers started from the same image do not always play the same
	// world. It takes precedence over -map-file when any file matches.
	defaultWorlds = "maps/map1[0-9][0-9][0-9].json"
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
		respawn    = flag.Int("respawn", 0, "seconds a dead player stays a ghost before coming back in the middle of the map; 0, the default, is the genre's own rule — and a match with respawn on is never decided")
		worlds     = flag.String("worlds", defaultWorlds, "glob of composed worlds to draw this match's map from; -worlds=\"\" falls back to -map-file")
		worldSeed  = flag.Int64("world-seed", 0, "pick the world deterministically; 0 draws from the clock")
		zone       = flag.Bool("zone", true, "shrink the safe circle over the match; -zone=false leaves the whole map playable")
		zoneSpeed  = flag.Float64("zone-speed", 1, "multiplier on every zone duration; 10 runs a whole match of contractions in about a minute")
		accounts   = flag.String("accounts", "", "archivo de cuentas; vacio deja el servidor sin cuentas y confia en el nombre del join")
		restart    = flag.Int("match-restart", 20, "seconds between a match being decided and the next one starting; 0 leaves the finished match standing")
		lobbyMin   = flag.Int("lobby-min", 1, "cuantos en la cola hacen falta para empezar una partida; 1, el default, arranca apenas entra alguien, que es como se comportaba el servidor antes de que existiera el lobby")
		lobbyWait  = flag.Int("lobby-wait", 0, "segundos de cuenta regresiva una vez que la cola llego al minimo; 0 arranca en el acto")
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

	// A match plays one world. Which one is drawn here, once, before anybody
	// connects: the simulation only ever knows about a single grid, so the
	// choice has to be made before it exists.
	chosen := *mapFile
	chosenFlag := "-map-file"
	if *worlds != "" {
		matches, err := filepath.Glob(*worlds)
		if err != nil {
			log.Error("patrón de mundos inválido", "worlds", *worlds, "err", err)
			os.Exit(1)
		}
		if len(matches) > 0 {
			sort.Strings(matches)
			seed := *worldSeed
			if seed == 0 {
				seed = time.Now().UnixNano()
			}
			chosen = matches[rand.New(rand.NewSource(seed)).Intn(len(matches))]
			chosenFlag = "-worlds"
			log.Info("mundo sorteado", "de", len(matches), "archivo", filepath.Base(chosen))
		}
	}

	if chosen != "" {
		loaded, number, name, err := world.LoadMap(dataFile(log, chosenFlag, chosen))
		if err != nil {
			log.Error("no se pudo cargar el mapa", "err", err)
			os.Exit(1)
		}
		grid, mapNumber, mapName = loaded, number, name
		log.Info("mapa cargado", "number", number, "name", name, "size", [2]int{grid.W, grid.H})
	}

	w := world.New(grid, protocol.JSONCodec{}, *tickRate, log)
	w.SetMap(mapNumber, mapName)
	if *zone {
		w.ArmZone(*zoneSpeed)
	}
	w.SetMatchRestart(*restart)
	w.SetLobby(*lobbyMin, *lobbyWait)

	// Accounts are opt-in. Without the flag this is the server it always was:
	// you are whatever name you typed, and nothing outlives the process. With
	// it, the name is checked against a password and every finished match is
	// filed against it.
	if *accounts != "" {
		store, err := account.Open(*accounts)
		if err != nil {
			log.Error("no se pudo abrir el archivo de cuentas", "path", *accounts, "err", err)
			os.Exit(1)
		}
		bridge := newAccountBridge(store, log)
		defer bridge.Close()
		w.SetAccounts(bridge)
		log.Info("cuentas habilitadas", "archivo", *accounts, "registradas", store.Count())
	}

	// Respawn used to default on, because dying otherwise meant restarting the
	// client to test the next fight. -match-restart is the honest version of
	// that convenience: the match ends, everyone is put back, and the next one
	// starts on the same connections. So the default goes back to the genre's
	// rule — and it has to, because a match where death is not elimination is
	// a match that never gets decided.
	w.SetRespawnDelay(*respawn)
	if *respawn > 0 {
		log.Warn("respawn habilitado: la muerte no elimina, asi que la partida no se va a decidir",
			"segundos", *respawn)
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
