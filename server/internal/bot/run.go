package bot

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"time"

	"juegito/server/internal/protocol"
	"juegito/server/internal/transport"
)

// Play corre un bot sobre una conexión ya establecida, hasta que el contexto se
// cancele o la conexión muera.
//
// No sabe si del otro lado hay un websocket o un transport.Pipe, y esa es toda
// la idea: el mismo bot rellena una partida desde adentro del servidor y hace
// pruebas de carga desde afuera. Lo único que cambia es quién le pasa la Conn.
//
// El login es cosa del que llama: un swarm externo contra un servidor con
// cuentas tiene que registrarse (ver SignIn), y un bot de relleno entra sin
// cuenta porque no la necesita — no hay carrera que guardarle a alguien que no
// existe.
func Play(ctx context.Context, conn transport.Conn, cfg Config) error {
	codec := protocol.JSONCodec{}
	rng := rand.New(rand.NewSource(cfg.Seed))

	// Los bots eligen clase y raza antes de entrar, igual que lo haría el
	// picker de una persona. El servidor no tiene un camino de "elegime uno":
	// un Join nombra siempre una selección concreta, así que un swarm de puros
	// Guerreros Humanos sería culpa de acá y no del servidor.
	//
	// De esa tirada sale sola la variedad del grupo: cinco de las doce clases
	// no tienen maná, así que una parte del relleno pelea sólo a los golpes
	// mientras otra tira Apocalipsis.
	const classCount, raceCount = 12, 5
	if err := send(conn, codec, protocol.TypeJoin, protocol.Join{
		Name:  cfg.Name,
		Class: rng.Intn(classCount),
		Race:  rng.Intn(raceCount),
	}); err != nil {
		return fmt.Errorf("join: %w", err)
	}

	// Y a la cola. Con el lobby en uno el Join de arriba ya alcanzó y esto no
	// hace nada; contra un servidor con -lobby-min es la diferencia entre un
	// swarm que juega y uno que mira el campamento — que es exactamente el
	// "sentado no es lo mismo que estar en la cola" de la sesión del 21.
	if err := send(conn, codec, protocol.TypeQueue, protocol.Queue{Join: true}); err != nil {
		return fmt.Errorf("queue: %w", err)
	}

	mind := &Brain{}
	go readLoop(ctx, conn, codec, mind, cfg)

	dir := protocol.Heading(rng.Intn(4))
	var seq uint32

	ticker := time.NewTicker(cfg.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			// El paseo: mantiene el rumbo y dobla de a ratos. Un random walk
			// puro casi no se despega del spawn, y entonces no ejercita el
			// entrar y salir del viewport sobre el que están armados los
			// snapshots. Es lo que hace un bot sin nadie a la vista.
			if rng.Intn(4) == 0 {
				dir = protocol.Heading(rng.Intn(4))
			}

			act := mind.Decide(cfg.Temper, rng, dir, time.Now())
			seq++
			if err := send(conn, codec, protocol.TypeMove, protocol.Move{Dir: act.dir, Seq: seq}); err != nil {
				return err
			}
			if act.drink >= 0 {
				// UseUseUp y no UseAuto: el clic sin calificar del original
				// equipa o consume según lo que haya en el slot, y una poción
				// no se equipa nunca. Decirlo explícito evita que un cambio de
				// mochila convierta un trago en un cambio de arma.
				if err := send(conn, codec, protocol.TypeUse, protocol.Use{Slot: act.drink, Action: protocol.UseUseUp}); err != nil {
					return err
				}
			}
			if act.cast != 0 {
				if err := send(conn, codec, protocol.TypeCast, protocol.Cast{SpellID: act.cast, Target: act.target}); err != nil {
					return err
				}
			}
			if act.swing {
				// Después del movimiento y en el mismo turno: el servidor
				// aplica los comandos en orden dentro del tick, así que para
				// cuando llega el golpe ya está mirando al objetivo. Al revés
				// pegaría hacia donde venía caminando.
				//
				// Va sin llevar la cuenta del cooldown: el servidor lo descarta
				// solo si todavía no toca, y duplicar acá su reloj sería una
				// segunda copia de una regla que ya vive de aquel lado.
				if err := send(conn, codec, protocol.TypeAttack, protocol.Attack{}); err != nil {
					return err
				}
			}
		}
	}
}

// readLoop drena todo lo que manda el servidor y le da al cerebro lo que
// necesita. Drenar no es opcional: un cliente que no lee llena la cola de envío
// del servidor y termina desconectado por lento, exactamente como una persona
// con la conexión tapada.
func readLoop(ctx context.Context, conn transport.Conn, codec protocol.JSONCodec, mind *Brain, cfg Config) {
	go func() {
		<-ctx.Done()
		_ = conn.Close()
	}()
	for {
		frame, err := conn.Recv()
		if err != nil {
			return
		}
		typ, payload, err := codec.DecodeEnvelope(frame)
		if err != nil {
			continue
		}
		// Con -debug, qué le llega. Es la sonda con la que se contesta "¿el
		// servidor lo está mandando o el cliente no lo está mostrando?", que es
		// la primera bifurcación de cualquier bug de mensaje que no aparece.
		if cfg.Log != nil {
			cfg.Log.Debug("bot recibió", "nombre", cfg.Name, "tipo", typ)
		}
		switch typ {
		case protocol.TypePing:
			// El servidor mide latencia pinguean­do y cronometrando el eco, así
			// que un bot mudo parecería una conexión que nunca contesta:
			// llenar una partida de bots llenaría el log de pérdida fantasma, y
			// la configuración que más interesa medir —bajo carga— sería la
			// única que no se puede.
			var ping protocol.Ping
			if codec.DecodePayload(payload, &ping) == nil {
				_ = send(conn, codec, protocol.TypePong, protocol.Pong{T: ping.T})
			}
		case protocol.TypeWelcome:
			var welcome protocol.Welcome
			if codec.DecodePayload(payload, &welcome) == nil {
				mind.SetSelf(welcome.EntityID)
			}
		case protocol.TypeLoadout:
			var loadout protocol.Loadout
			if codec.DecodePayload(payload, &loadout) == nil {
				mind.SetLoadout(loadout)
			}
		case protocol.TypeSnapshot:
			var snap protocol.Snapshot
			if codec.DecodePayload(payload, &snap) == nil {
				mind.Observe(snap)
			}
		}
	}
}

func send(conn transport.Conn, codec protocol.JSONCodec, typ protocol.MsgType, payload any) error {
	frame, err := codec.Encode(typ, payload)
	if err != nil {
		return err
	}
	if err := conn.Send(frame); err != nil {
		return fmt.Errorf("enviar %s: %w", typ, err)
	}
	return nil
}

// Config es todo lo que hace falta para correr un bot.
type Config struct {
	Name string
	Seed int64
	// Interval es cada cuánto piensa. Doscientos milisegundos es la cadencia
	// del paso: pensar más seguido no compra nada porque el servidor no lo
	// dejaría avanzar igual.
	Interval time.Duration
	Temper   Temper
	// Log es opcional: con él puesto y en nivel debug, el bot dice qué mensajes
	// recibe. El relleno interno lo deja en nil — treinta y nueve bots narrando
	// cada snapshot es exactamente el ruido que hubo que sacar del log de
	// producción.
	Log *slog.Logger
}
