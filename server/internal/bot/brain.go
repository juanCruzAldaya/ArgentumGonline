package bot

import (
	"math/rand"
	"sync"
	"time"

	"juegito/server/internal/protocol"
)

// Los objetos y hechizos que el bot sabe usar, por su id de Argentum.
//
// Están acá y no leídos de los datos porque el bot no tiene la tabla: el
// servidor le manda su inventario por número de item y su libro por número de
// hechizo, y quién es cada uno vive en obj.dat y Hechizos.dat del lado del
// conversor. Son los mismos ids que world.startingInventory y
// world.startingSpells reparten, así que si ese kit cambia, esto también.
const (
	itemRedPotion  = 38 // Poción Roja: vida
	itemBluePotion = 37 // Poción Azul: maná

	spellParalyze     = 9  // Paralizar, 450 de maná
	spellFreeMovement = 10 // Devolver Movilidad, 300: la cura de la de arriba
	spellHealLight    = 3  // Curar Heridas Leves, 10
	spellHealSerious  = 5  // Curar Heridas Graves, 40
)

// Los hechizos de ataque, del más fuerte al más barato. El bot tira el mejor
// que pueda pagar, que es lo que hace cualquiera con la barra llena.
//
// El orden importa y es el de daño, no el de id. Los cuatro de abajo los tiene
// todo el mundo (world.startingSpells); los tres de arriba los reparte
// world.heavySpellsFor por clase, así que un Guerrero no va a lanzar ninguno —
// y cinco de las doce clases no tienen maná en absoluto y pelean sólo a los
// golpes. Esa variedad sale sola de que el bot elige su clase al azar.
//
// El alcance no se chequea acá: el servidor deja lanzar a cualquiera dentro
// del viewport, que es más lejos de lo que el bot ve (Temper.sight), así que
// todo lo que alcanza a ver lo alcanza a golpear.
var damageSpells = []int{
	25, // Apocalipsis,        1000 de maná, 85-100
	23, // Descarga Eléctrica,  460, 55-85
	15, // Tormenta de Fuego,   250, 45-55
	8,  // Proyectil Mágico,     45, 30-35
	7,  // Flecha Eléctrica,     40, 12-20
	6,  // Flecha Mágica,        20, 6-12
	2,  // Dardo Mágico,         10, 2-5
}

// El bot que pelea, y hasta dónde llega su cabeza.
//
// El objetivo no es un buen jugador: es un rival creíble. Un bot que juega
// perfecto no sirve para lo que hace falta — mide el bot en vez de medir el
// juego, y probar balance contra él sería pelear contra un reloj. Así que todo
// lo de acá está construido alrededor de que se equivoque de maneras en que se
// equivoca una persona: tarda en reaccionar, erra el clic, se distrae, y toma
// la poción un segundo después de cuando le convenía.
//
// Lo que sabe hacer: pegar de cerca, perseguir lo que ve, tomar la roja cuando
// le baja la vida y la azul cuando le baja el maná, paralizar al que tiene
// enfrente, sacarse la parálisis de encima si le alcanza el maná, y tirar el
// hechizo de ataque más fuerte que pueda pagar — desde donde lo vea, porque el
// servidor deja lanzar a todo el viewport.
//
// Lo que deliberadamente NO hace, y conviene que siga siendo así:
//
//   - No junta loot ni abre cofres: pelea con lo que trae puesto.
//   - No mira la zona. Se muere en el anillo si le toca, como un novato.
//   - No huye nunca. Cura y sigue peleando, no retrocede.
//   - No coordina con otros bots ni concentra fuego.
//   - No usa el arco ni los objetos arrojadizos.
//   - No gestiona el maná: tira lo más caro que pueda y después se queda seco,
//     que es una de las formas más comunes de jugar mal.
type Temper struct {
	// sight es a cuántos tiles se da cuenta de que hay alguien. Corto a
	// propósito: el viewport del servidor es de 17x13, así que un bot con la
	// vista completa perseguiría desde mucho más lejos de lo que alguien
	// mirando la pantalla registraría. Cero apaga la pelea entera y deja el
	// paseo de siempre, que es el modo de prueba de carga.
	Sight int
	// sloppy es cada cuánto se le va la atención, de 0 a 1, y cada cuánto le
	// erra al clic de un hechizo — que en el original es literalmente eso, un
	// clic sobre alguien, y errarlo es de las cosas más humanas que hay.
	//
	// La atención se tira UNA VEZ por decisión y no una vez por turno, y esa
	// diferencia es todo. El loop corre cinco veces por segundo: tirando el
	// dado en cada vuelta, un 0,35 despista al bot casi dos veces por segundo
	// y lo deja caminando en zigzag, sin alcanzar nunca a nadie que se mueva.
	// Eso no es un jugador distraído, es uno borracho — y el síntoma en
	// pantalla fue justamente "no atacan". Una persona decide a quién va a
	// perseguir, va, y recién después vuelve a pensar.
	Sloppy float64
	// focus es cuánto le dura una decisión antes de volver a pensarla, y con
	// ella el despiste cuando le toca. Es lo que convierte la torpeza en
	// "se distrajo un segundo" en vez de en un temblor.
	Focus time.Duration
	// react es cuánto tarda como mínimo entre una acción deliberada y la
	// siguiente: tomar una poción, lanzar un hechizo. El servidor ya tiene sus
	// propios intervalos, pero jugar AL LÍMITE de esos intervalos es
	// exactamente lo que ninguna persona hace, y un bot que lo hiciera sería
	// injugablemente rápido aunque no supiera nada.
	React time.Duration
	// hurt y drained son en qué fracción de vida y de maná se acuerda de tomar
	// algo. No son umbrales óptimos: son "uh, se me está por acabar".
	Hurt, Drained float64
}

func DefaultTemper() Temper {
	return Temper{
		Sight:   7,
		Sloppy:  0.2,
		Focus:   1200 * time.Millisecond,
		React:   900 * time.Millisecond,
		Hurt:    0.5,
		Drained: 0.3,
	}
}

// Brain es lo que el bot sabe del mundo: la última foto que le mandaron, más
// su propio inventario y libro de hechizos.
//
// Vive detrás de un mutex porque la goroutine que lee el socket lo escribe y el
// loop de decisiones lo lee. Es la única pieza compartida del bot, y por eso la
// única con candado.
type Brain struct {
	mu     sync.Mutex
	self   uint32
	known  bool
	x, y   int
	dead   bool
	vitals protocol.Vitals
	others []protocol.EntityState

	// El inventario y el libro llegan por su propio mensaje, no en cada
	// snapshot: una mochila cambia cuando agarrás algo, no veinte veces por
	// segundo. Así que se guardan hasta el próximo Loadout.
	inventory []protocol.InventorySlot
	spells    []int

	// nextAction es cuándo puede volver a hacer algo deliberado. Es el reloj de
	// la lentitud humana, y es aparte de los cooldowns del servidor.
	nextAction time.Time

	// rethinkAt es cuándo vuelve a decidir si persigue o se despista, y
	// distracted es qué salió la última vez. Juntos son lo que hace que una
	// decisión dure lo que dura una decisión.
	rethinkAt  time.Time
	distracted bool
}

func (b *Brain) SetSelf(id uint32) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.self = id
}

// setLoadout guarda la mochila y el libro que el servidor acaba de mandar.
func (b *Brain) SetLoadout(l protocol.Loadout) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.inventory = append(b.inventory[:0], l.Inventory...)
	b.spells = append(b.spells[:0], l.Spells...)
}

// observe guarda lo que se ve en el snapshot. Copia las entidades en vez de
// quedarse con el slice porque el que lo decodificó lo va a reusar.
func (b *Brain) Observe(snap protocol.Snapshot) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if snap.Self != nil {
		b.vitals = *snap.Self
	}
	b.others = b.others[:0]
	for _, e := range snap.Entities {
		if e.ID == b.self {
			b.x, b.y, b.dead, b.known = e.X, e.Y, e.Dead, true
			continue
		}
		// Un cadáver no es un rival. El servidor los sigue mandando para que el
		// cliente los dibuje, así que sin esto los bots se quedarían pegándole
		// a un muerto mientras alguien vivo les pasa por al lado.
		if e.Dead {
			continue
		}
		b.others = append(b.others, e)
	}
}

// intent es lo que el bot decidió hacer este turno. El movimiento va siempre
// (aunque sea el paseo); lo demás es opcional y como mucho una cosa por turno,
// que es lo que lo mantiene a velocidad de persona.
type intent struct {
	dir   protocol.Heading
	swing bool
	// drink es el slot de la mochila a usar, o -1.
	drink int
	// cast es el hechizo a lanzar, o 0. target puede ser el propio bot.
	cast   int
	target uint32
}

func noAction(dir protocol.Heading) intent { return intent{dir: dir, drink: -1} }

// decide elige qué hacer este turno, en orden de urgencia.
//
// Devuelve siempre una dirección: hasta cuando pega, mandar el movimiento hacia
// el objetivo es lo que lo deja mirándolo. El golpe de Argentum pega al tile de
// enfrente y no nombra a nadie (protocol.Attack no tiene campos), así que girar
// ES apuntar. El servidor gira igual si el paso se rechaza por cadencia o
// porque el tile está ocupado — que es justamente el caso cuando el objetivo
// está ahí parado.
func (b *Brain) Decide(t Temper, rng *rand.Rand, wander protocol.Heading, now time.Time) intent {
	b.mu.Lock()
	defer b.mu.Unlock()

	if t.Sight <= 0 || !b.known || b.dead {
		return noAction(wander)
	}

	// Lo deliberado tiene su propio reloj. Sin esto el bot encadena poción,
	// hechizo y golpe en el mismo instante en que cada cooldown vence, que es
	// jugar mejor de lo que juega nadie.
	deliberate := now.After(b.nextAction)
	act := noAction(wander)

	// 1. Sacarse la parálisis de encima. Es lo único que se atiende antes que
	//    la vida: paralizado no podés caminar ni pegar, así que cualquier otra
	//    decisión es sobre un jugador que no puede ejecutarla. Lanzar sí se
	//    puede — el PuedeLanzar del original nunca chequea parálisis, y esa
	//    rareza portada es justamente lo que hace que este hechizo tenga
	//    sentido tenerlo.
	if deliberate && b.vitals.Paralyzed && b.canCast(spellFreeMovement) {
		b.nextAction = now.Add(t.React)
		return intent{dir: wander, drink: -1, cast: spellFreeMovement, target: b.self}
	}

	// 2. Curarse. Primero la poción, que es lo que hace todo el mundo; el
	//    hechizo sólo si no le quedan.
	if deliberate && b.vitals.MaxHP > 0 && frac(b.vitals.HP, b.vitals.MaxHP) < t.Hurt {
		if slot := b.slotOf(itemRedPotion); slot >= 0 {
			b.nextAction = now.Add(t.React)
			return intent{dir: wander, drink: slot, cast: 0}
		}
		if spell := b.bestHeal(); spell != 0 {
			b.nextAction = now.Add(t.React)
			return intent{dir: wander, drink: -1, cast: spell, target: b.self}
		}
	}

	// 3. Reponer maná, para poder seguir haciendo lo de arriba.
	if deliberate && b.vitals.MaxMana > 0 && frac(b.vitals.Mana, b.vitals.MaxMana) < t.Drained {
		if slot := b.slotOf(itemBluePotion); slot >= 0 {
			b.nextAction = now.Add(t.React)
			return intent{dir: wander, drink: slot, cast: 0}
		}
	}

	target, dist := b.nearest()
	if target == nil || dist > t.Sight {
		return act
	}

	// La atención se re-tira cada tanto, no cada turno. Ver el comentario de
	// Temper.Sloppy: el dado por turno es lo que los volvía inofensivos.
	if now.After(b.rethinkAt) {
		b.rethinkAt = now.Add(t.Focus)
		b.distracted = rng.Float64() < t.Sloppy
	}
	// Salvo pegado a alguien: ahí no hay despiste que valga. Nadie se pone a
	// mirar el techo con un tipo pegándole en la cara, y era el otro motivo de
	// que los golpes no salieran.
	if b.distracted && dist > 1 {
		return act
	}

	act.dir = headingToward(b.x, b.y, target.X, target.Y)

	// 4. Los hechizos, en el orden en que los usa cualquiera: primero
	//    paralizar —que es lo que decide la pelea— y después pegarle con lo
	//    más fuerte que tenga al que ya no se puede mover.
	//
	//    Acá es donde erra el clic: en el original apuntar un hechizo es
	//    literalmente clickear a alguien, y errarle es de las cosas más humanas
	//    que pasan en una pelea. Un clic errado gasta el maná y el intervalo
	//    igual, exactamente como al jugador que le erró.
	if deliberate {
		spell := 0
		if b.canCast(spellParalyze) && !target.Paralyzed {
			spell = spellParalyze
		} else {
			spell = b.bestDamage()
		}
		if spell != 0 {
			b.nextAction = now.Add(t.React)
			act.cast = spell
			act.target = target.ID
			if rng.Float64() < t.Sloppy {
				act.target = b.missedClick(rng, target.ID)
			}
			return act
		}
	}

	// 5. Y si no, pegar. Sólo cuando está pegado: ni predice hacia dónde va el
	//    otro ni se adelanta al tile al que va a llegar.
	act.swing = dist == 1
	return act
}

// missedClick devuelve a quién le pegó el clic cuando se erró: otro que esté a
// la vista, o nadie. No es azar puro sobre todo el mapa — un clic errado cae
// cerca de donde apuntabas, que es lo que hace que a veces le pegues sin querer
// al que estaba al lado.
func (b *Brain) missedClick(rng *rand.Rand, intended uint32) uint32 {
	if len(b.others) < 2 {
		return 0 // le erró a todo: el hechizo se va al vacío
	}
	for range 3 {
		other := b.others[rng.Intn(len(b.others))]
		if other.ID != intended {
			return other.ID
		}
	}
	return 0
}

// canCast es si el hechizo está en el libro y le alcanza el maná. No mira el
// intervalo del servidor: duplicar acá su reloj sería una segunda copia de una
// regla que ya vive de aquel lado, y el bot no pierde nada con que le rebote
// un lanzamiento de vez en cuando — le pasa a cualquiera.
func (b *Brain) canCast(spell int) bool {
	cost, ok := spellCost[spell]
	if !ok || b.vitals.Mana < cost {
		return false
	}
	for _, s := range b.spells {
		if s == spell {
			return true
		}
	}
	return false
}

// spellCost es el maná de Hechizos.dat para lo único que el bot lanza. Es una
// copia chica de un dato que vive en spells.json, y está acá porque cmd/bot no
// carga los datos del juego: pedirle que lea el .json sólo para cuatro números
// le agregaría un flag de ruta y un modo de fallar al arrancar.
var spellCost = map[int]int{
	spellHealLight:    10,
	spellHealSerious:  40,
	spellFreeMovement: 300,
	spellParalyze:     450,
	25:                1000,
	23:                460,
	15:                250,
	8:                 45,
	7:                 40,
	6:                 20,
	2:                 10,
}

// bestDamage es el hechizo de ataque más fuerte que puede pagar ahora mismo.
// No reserva maná para curarse después: gastar de más y quedarse seco es una
// de las formas más comunes de jugar mal, y este bot juega así a propósito.
func (b *Brain) bestDamage() int {
	for _, spell := range damageSpells {
		if b.canCast(spell) {
			return spell
		}
	}
	return 0
}

// bestHeal elige la curación más cara que pueda pagar, que es lo que uno hace
// cuando está por morirse.
func (b *Brain) bestHeal() int {
	if b.canCast(spellHealSerious) {
		return spellHealSerious
	}
	if b.canCast(spellHealLight) {
		return spellHealLight
	}
	return 0
}

// slotOf busca un item en la mochila y devuelve su slot, o -1.
func (b *Brain) slotOf(itemID int) int {
	for _, s := range b.inventory {
		if s.ItemID == itemID && s.Amount > 0 {
			return s.Slot
		}
	}
	return -1
}

// nearest es el vivo más cerca, en distancia de tiles caminados (Manhattan,
// porque el movimiento es de a cuatro direcciones y no hay diagonales).
func (b *Brain) nearest() (*protocol.EntityState, int) {
	var best *protocol.EntityState
	bestDist := 0
	for i := range b.others {
		e := &b.others[i]
		d := abs(e.X-b.x) + abs(e.Y-b.y)
		if best == nil || d < bestDist {
			best, bestDist = e, d
		}
	}
	return best, bestDist
}

// headingToward camina primero por el eje en el que está más lejos. Es la ruta
// tonta a propósito: no rodea nada, así que una pared en el medio lo deja
// empujando contra ella hasta que el otro se mueve.
func headingToward(fromX, fromY, toX, toY int) protocol.Heading {
	dx, dy := toX-fromX, toY-fromY
	if abs(dx) >= abs(dy) {
		if dx > 0 {
			return protocol.East
		}
		return protocol.West
	}
	if dy > 0 {
		return protocol.South
	}
	return protocol.North
}

func frac(part, whole int) float64 { return float64(part) / float64(whole) }

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// NewTemper arma el carácter desde los flags, dejando DefaultTemper como el
// único lugar donde viven los valores por defecto.
func NewTemper(sight int, sloppy float64, focus, react time.Duration, hurt, drained float64) Temper {
	return Temper{Sight: sight, Sloppy: sloppy, Focus: focus, React: react, Hurt: hurt, Drained: drained}
}
