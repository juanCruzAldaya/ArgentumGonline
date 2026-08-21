package world

// Relleno de partida: completar la cola con bots hasta que haya gente
// suficiente para que se sienta un battle royale.
//
// Es lo que hacen los battle royale comerciales cuando no hay cola para armar
// una partida entera, y acá el problema es más agudo todavía: un juego que se
// comparte mandando un link empieza siempre con una persona sola mirando un
// campamento vacío. Cuarenta jugadores en el mapa es la diferencia entre "esto
// es un juego" y "esto es una demo".
//
// Tres reglas lo definen, y las tres son sobre no mentirle a nadie más de lo
// necesario:
//
//   - **Sin nadie de verdad, no hay bots.** El relleno existe para acompañar a
//     una persona, no para jugar solo. Además la máquina de Fly duerme cuando
//     no queda ninguna conexión, y un servidor que se rellena a sí mismo no se
//     dormiría nunca: serían cuarenta bots peleando toda la noche a cuenta de
//     alguien.
//   - **No tienen cuenta.** Entran por serve() con un nombre en la mano,
//     salteando el login. No se registran, no escriben en el log de cuentas y
//     no tienen carrera — que es exactamente el problema que hoy ensucia el
//     archivo de producción cada vez que alguien corre cmd/bot contra Fly.
//   - **El mundo no sabe cuáles son.** Hablan el mismo protocolo por un
//     transport.Pipe, y no hay una sola rama "if bot" en la simulación. Lo
//     único que los distingue es que el log los nombra, para que las bajas y
//     los puestos se puedan leer sin confundirlos con personas.

import (
	"context"

	"juegito/server/internal/transport"
)

// SetFill configura cuántos jugadores tiene que haber en la cola, contando
// bots, y quién sabe crear uno.
//
// El spawner se inyecta desde cmd/server en vez de que este paquete importe
// internal/bot, y no es ceremonia: el bot ya importa el protocolo y el
// transporte, así que la dependencia al revés sería un ciclo. Que la
// simulación no pueda ni nombrar al paquete de los bots es la versión
// estructural de la promesa de arriba.
//
// Como SetMap, se llama antes de Run: lo lee la goroutine del mundo.
func (w *World) SetFill(to int, spawn func(ctx context.Context, name string, conn transport.Conn)) {
	w.fillTo = to
	w.spawnBot = spawn
}

// fillPerTick limita cuántos bots se largan por tick. Cuarenta goroutines de
// golpe, cada una mandando su join y su queue en el mismo instante, mediría el
// camino de entrada en vez de la partida — el mismo motivo por el que cmd/bot
// escalona sus conexiones. A dos por tick, una partida se llena en un segundo.
const fillPerTick = 2

// fillTick decide si hace falta más gente y larga lo que falte. Corre una vez
// por tick desde lobbyTick.
func (w *World) fillTick() {
	if w.fillTo <= 0 || w.spawnBot == nil {
		return
	}
	// Sólo se rellena el lobby. Una partida en curso no admite a nadie más —
	// entrar tarde te deja en un anillo ya cerrado, sin nada que caminar — y
	// esa regla no tiene por qué ser distinta para un bot.
	if w.match.phase != matchLobby {
		return
	}

	if w.humansSeated() == 0 {
		w.dismissBots()
		return
	}

	// Los ya largados cuentan aunque todavía no tengan asiento: su entrada
	// viaja por el mismo canal que la de cualquiera y se aplica en el tick
	// siguiente. Sin contarlos, cada tick volvería a ver la cola corta y
	// largaría otros dos, y para cuando el primero apareciera habría cuarenta
	// de más. botsPending baja en addSeat, donde el asiento realmente existe.
	missing := w.fillTo - w.queuedCount() - w.botsPending
	for range min(missing, fillPerTick) {
		w.launchBot()
	}
}

// humansSeated es cuánta gente de verdad está mirando el campamento. Un asiento
// sin bot detrás es una persona: los bots son los únicos que entran sin cuenta.
func (w *World) humansSeated() int {
	n := 0
	for _, s := range w.lobby.seats {
		if !s.bot {
			n++
		}
	}
	return n
}

// launchBot crea un par de conexiones en memoria, le da una punta al mundo y la
// otra al bot. Desde acá para abajo es una conexión como cualquier otra.
func (w *World) launchBot() {
	// Un solo contexto para toda la tanda, y no uno por bot: se van todos
	// juntos o no se va ninguno, así que una lista de cancels sería una lista
	// que hay que mantener limpia cada vez que uno muere por su cuenta.
	if w.botCtx == nil {
		w.botCtx, w.botStop = context.WithCancel(context.Background())
	}

	name := w.botName()
	server, client := transport.Pipe()
	w.botsPending++
	w.botsLive++

	go w.serve(server, name)
	go w.spawnBot(w.botCtx, name, client)
}

// dismissBots echa a todos los bots. Se llama cuando no queda nadie de verdad:
// sin una persona a la que acompañar no hay a quién rellenarle la partida, y
// dejarlos conectados mantendría la máquina despierta para siempre.
func (w *World) dismissBots() {
	if w.botStop == nil {
		return
	}
	w.log.Info("relleno: no queda nadie de verdad, se van los bots", "bots", w.botsLive)
	w.botStop()
	w.botCtx, w.botStop = nil, nil
	w.botsPending, w.botsLive = 0, 0
}

// botName saca un nombre de la lista de abajo, con un número si hace falta
// desempatar. Que parezcan jugadores no es cosmético: un relleno llamado bot07
// anuncia que la partida está vacía, que es exactamente lo que el relleno
// existe para evitar.
func (w *World) botName() string {
	base := botNames[w.rng.Intn(len(botNames))]
	name := base
	for attempt := 1; w.nameTaken(name); attempt++ {
		name = base + suffixes[w.rng.Intn(len(suffixes))]
		if attempt > 8 {
			// Después de ocho intentos la lista está saturada y seguir tirando
			// es un bucle que se ve como un cuelgue. Un número es feo pero
			// siempre termina.
			name = base + string(rune('0'+w.rng.Intn(10))) + string(rune('0'+w.rng.Intn(10)))
			break
		}
	}
	return name
}

// nameTaken mira asientos y jugadores: los dos espacios donde un nombre puede
// estar en uso ahora mismo.
func (w *World) nameTaken(name string) bool {
	for _, s := range w.lobby.seats {
		if s.name == name {
			return true
		}
	}
	for _, p := range w.players {
		if p.Name == name {
			return true
		}
	}
	return false
}

// Nombres de Argentum, del tipo que se ve en un servidor real: apócopes,
// nombres propios castellanos y los de fantasía que la gente elige de verdad.
// La mezcla importa — cuarenta nombres del mismo estilo se leen como una lista
// generada, que es justo lo que son.
var botNames = []string{
	"Rakhar", "Morgul", "Zeppelin", "Kaelen", "Tormenta", "Nahuel", "Drako",
	"Valkyria", "Sombra", "Elandir", "Bruno", "Kira", "Thorne", "Milagros",
	"Ragnar", "Lucero", "Ezequiel", "Vandal", "Aluminé", "Sauron", "Camila",
	"Grimlock", "Facundo", "Zoe", "Mordred", "Ciro", "Nerea", "Balrog",
	"Santino", "Ithil", "Rocío", "Khaos", "Julián", "Freya", "Malak",
	"Agustina", "Verne", "Dorian", "Renzo", "Lyra", "Ulric", "Bautista",
}

var suffixes = []string{"x", "sz", "ok", "ia", "el", "ar", "us", "yn"}
