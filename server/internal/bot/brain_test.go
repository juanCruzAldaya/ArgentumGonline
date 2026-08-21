package bot

import (
	"math/rand"
	"testing"
	"time"

	"juegito/server/internal/protocol"
)

// sharp es un bot sin distracción y sin espera, para que los tests fallen por
// la regla que están mirando y no por la tirada ni por el reloj.
var sharp = Temper{Sight: 5, Sloppy: 0, Hurt: 0.5, Drained: 0.3}

// t0 es un instante cualquiera, posterior a un nextAction en cero, así que
// "puede actuar" está siempre habilitado salvo que el test diga otra cosa.
var t0 = time.Date(2026, 8, 21, 20, 0, 0, 0, time.UTC)

func mindAt(x, y int, others ...protocol.EntityState) *Brain {
	b := &Brain{self: 1}
	b.Observe(protocol.Snapshot{
		Entities: append([]protocol.EntityState{{ID: 1, X: x, Y: y}}, others...),
	})
	return b
}

func TestSwingsOnlyAtSomebodyAdjacent(t *testing.T) {
	rng := rand.New(rand.NewSource(1))

	adjacent := mindAt(10, 10, protocol.EntityState{ID: 2, X: 11, Y: 10})
	if act := adjacent.Decide(sharp, rng, protocol.North, t0); !act.swing {
		t.Error("no pegó a alguien pegado al lado")
	} else if act.dir != protocol.East {
		t.Errorf("mira al %v, tenía que mirar al este: girar es apuntar", act.dir)
	}

	// A dos tiles se acerca pero no pega al aire.
	near := mindAt(10, 10, protocol.EntityState{ID: 2, X: 12, Y: 10})
	act := near.Decide(sharp, rng, protocol.North, t0)
	if act.swing {
		t.Error("pegó a dos tiles de distancia")
	}
	if act.dir != protocol.East {
		t.Errorf("dir = %v, esperaba este: tenía que acercarse", act.dir)
	}
}

// La vista corta es lo que lo hace un rival y no un misil. Alguien lejos, aun
// dentro del viewport que el servidor manda, no le interesa.
func TestIgnoresAnybodyBeyondItsSight(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	b := mindAt(10, 10, protocol.EntityState{ID: 2, X: 10 + sharp.Sight + 1, Y: 10})

	if act := b.Decide(sharp, rng, protocol.North, t0); act.dir != protocol.North || act.swing {
		t.Errorf("persiguió a alguien fuera de su vista: %+v", act)
	}
}

// Un cadáver no es un rival. El servidor los sigue mandando para que el cliente
// los dibuje, y sin este filtro los bots se quedan pegándole a un muerto.
func TestDeadPlayersAreNotTargets(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	b := mindAt(10, 10,
		protocol.EntityState{ID: 2, X: 11, Y: 10, Dead: true},
		protocol.EntityState{ID: 3, X: 10, Y: 13},
	)

	act := b.Decide(sharp, rng, protocol.North, t0)
	if act.swing {
		t.Error("le pegó a un cadáver")
	}
	if act.dir != protocol.South {
		t.Errorf("dir = %v, esperaba sur: tenía que ir por el vivo", act.dir)
	}
}

// Con -sight 0 el bot es el paseante de siempre, que es el modo de prueba de
// carga: nada de lo de acá puede cambiar lo que ese swarm mide.
func TestSightZeroKeepsTheOldWanderer(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	b := mindAt(10, 10, protocol.EntityState{ID: 2, X: 11, Y: 10})

	act := b.Decide(Temper{Sight: 0}, rng, protocol.West, t0)
	if act.swing || act.dir != protocol.West {
		t.Errorf("con la pelea apagada hizo %+v", act)
	}
}

// Un muerto no pelea: sigue paseando hasta que la partida lo saque.
func TestADeadBotDoesNotFight(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	b := &Brain{self: 1}
	b.Observe(protocol.Snapshot{Entities: []protocol.EntityState{
		{ID: 1, X: 10, Y: 10, Dead: true},
		{ID: 2, X: 11, Y: 10},
	}})

	if act := b.Decide(sharp, rng, protocol.North, t0); act.swing {
		t.Error("un bot muerto siguió pegando")
	}
}

// La torpeza tiene que notarse de verdad, o el flag es decorativo. Con sloppy
// alto la mayoría de los turnos son cualquier cosa; con 0, ninguno.
func TestSloppinessActuallyDistracts(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	const turns = 400

	count := func(sloppy float64) int {
		n := 0
		for range turns {
			b := mindAt(10, 10, protocol.EntityState{ID: 2, X: 12, Y: 10})
			// El paseo apunta al oeste, o sea al revés del objetivo: cualquier
			// turno que salga al oeste es un turno distraído.
			if b.Decide(Temper{Sight: 5, Sloppy: sloppy}, rng, protocol.West, t0).dir == protocol.West {
				n++
			}
		}
		return n
	}

	if n := count(0); n != 0 {
		t.Errorf("con sloppy=0 se distrajo %d de %d veces", n, turns)
	}
	if n := count(0.8); n < turns/2 {
		t.Errorf("con sloppy=0.8 se distrajo sólo %d de %d veces", n, turns)
	}
}

func TestHeadingTowardTakesTheLongerAxisFirst(t *testing.T) {
	cases := []struct {
		dx, dy int
		want   protocol.Heading
	}{
		{3, 1, protocol.East},
		{-3, 1, protocol.West},
		{1, 3, protocol.South},
		{1, -3, protocol.North},
	}
	for _, c := range cases {
		if got := headingToward(10, 10, 10+c.dx, 10+c.dy); got != c.want {
			t.Errorf("desde (0,0) hacia (%d,%d): %v, esperaba %v", c.dx, c.dy, got, c.want)
		}
	}
}

// mindFighting arma un bot con vitals, mochila y libro, que es lo que hace
// falta para todo lo que no sea caminar y pegar.
func mindFighting(hp, mana int, bag []protocol.InventorySlot, spells []int, others ...protocol.EntityState) *Brain {
	b := &Brain{self: 1}
	b.SetLoadout(protocol.Loadout{Inventory: bag, Spells: spells})
	b.Observe(protocol.Snapshot{
		Self:     &protocol.Vitals{HP: hp, MaxHP: 100, Mana: mana, MaxMana: 1000},
		Entities: append([]protocol.EntityState{{ID: 1, X: 10, Y: 10}}, others...),
	})
	return b
}

var fullBag = []protocol.InventorySlot{
	{Slot: 1, ItemID: itemRedPotion, Amount: 100},
	{Slot: 2, ItemID: itemBluePotion, Amount: 100},
}

func TestDrinksTheRedPotionWhenHurt(t *testing.T) {
	rng := rand.New(rand.NewSource(1))

	hurt := mindFighting(30, 1000, fullBag, nil, protocol.EntityState{ID: 2, X: 11, Y: 10})
	if act := hurt.Decide(sharp, rng, protocol.North, t0); act.drink != 1 {
		t.Errorf("con 30 de 100 de vida no tomó la roja: drink=%d", act.drink)
	}

	// Entero, no toma nada y se dedica a pelear.
	whole := mindFighting(100, 1000, fullBag, nil, protocol.EntityState{ID: 2, X: 11, Y: 10})
	if act := whole.Decide(sharp, rng, protocol.North, t0); act.drink != -1 {
		t.Errorf("tomó una poción estando entero: drink=%d", act.drink)
	}
}

func TestDrinksTheBluePotionWhenDrained(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	b := mindFighting(100, 100, fullBag, nil, protocol.EntityState{ID: 2, X: 11, Y: 10})

	if act := b.Decide(sharp, rng, protocol.North, t0); act.drink != 2 {
		t.Errorf("con 100 de 1000 de maná no tomó la azul: drink=%d", act.drink)
	}
}

// Sacarse la parálisis va antes que todo: paralizado no podés caminar ni pegar,
// así que cualquier otra decisión es sobre un jugador que no puede ejecutarla.
func TestBreaksParalysisBeforeAnythingElse(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	b := &Brain{self: 1}
	b.SetLoadout(protocol.Loadout{Inventory: fullBag, Spells: []int{spellFreeMovement}})
	b.Observe(protocol.Snapshot{
		Self:     &protocol.Vitals{HP: 10, MaxHP: 100, Mana: 1000, MaxMana: 1000, Paralyzed: true},
		Entities: []protocol.EntityState{{ID: 1, X: 10, Y: 10}, {ID: 2, X: 11, Y: 10}},
	})

	act := b.Decide(sharp, rng, protocol.North, t0)
	if act.cast != spellFreeMovement {
		t.Fatalf("paralizado y con 10 de vida no se soltó: cast=%d drink=%d", act.cast, act.drink)
	}
	if act.target != 1 {
		t.Errorf("se lo lanzó a %d en vez de a sí mismo", act.target)
	}
}

func TestParalyzesSomebodyInRange(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	b := mindFighting(100, 1000, fullBag, []int{spellParalyze}, protocol.EntityState{ID: 2, X: 11, Y: 10})

	act := b.Decide(sharp, rng, protocol.North, t0)
	if act.cast != spellParalyze || act.target != 2 {
		t.Errorf("no paralizó al que tenía al lado: cast=%d target=%d", act.cast, act.target)
	}
}

// No se gasta el maná en alguien que ya está paralizado.
func TestDoesNotReparalyzeAParalyzedTarget(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	b := mindFighting(100, 1000, fullBag, []int{spellParalyze},
		protocol.EntityState{ID: 2, X: 11, Y: 10, Paralyzed: true})

	act := b.Decide(sharp, rng, protocol.North, t0)
	if act.cast != 0 {
		t.Errorf("volvió a paralizar a alguien ya paralizado")
	}
	if !act.swing {
		t.Error("teniéndolo al lado y sin nada que lanzar, tenía que pegarle")
	}
}

// Sin maná no hay hechizo: se arregla a los golpes, como cualquiera.
func TestWithoutManaItJustSwings(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	b := mindFighting(100, 0, nil, []int{spellParalyze}, protocol.EntityState{ID: 2, X: 11, Y: 10})

	act := b.Decide(sharp, rng, protocol.North, t0)
	if act.cast != 0 {
		t.Errorf("lanzó un hechizo de 450 con 0 de maná")
	}
	if !act.swing {
		t.Error("no pegó, que era lo único que le quedaba")
	}
}

// El reloj humano: una acción deliberada por vez, no una cadena instantánea.
func TestDeliberateActionsAreRateLimited(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	slow := sharp
	slow.React = time.Second

	b := mindFighting(30, 1000, fullBag, []int{spellParalyze}, protocol.EntityState{ID: 2, X: 11, Y: 10})

	if act := b.Decide(slow, rng, protocol.North, t0); act.drink != 1 {
		t.Fatalf("no tomó la primera poción: %+v", act)
	}
	// Medio segundo después todavía está "ocupado": pega, pero no encadena
	// otra acción deliberada.
	if act := b.Decide(slow, rng, protocol.North, t0.Add(500*time.Millisecond)); act.drink != -1 || act.cast != 0 {
		t.Errorf("encadenó otra acción a los 500ms de una que tarda 1s: %+v", act)
	}
	// Pasado el segundo, vuelve a decidir.
	if act := b.Decide(slow, rng, protocol.North, t0.Add(1500*time.Millisecond)); act.drink == -1 && act.cast == 0 {
		t.Error("pasado el tiempo de reacción no volvió a actuar")
	}
}

// Errarle al clic es de las cosas más humanas que hay, y tiene que pasar de
// verdad: un clic errado se va al vacío o le pega a otro, pero no al que
// apuntabas.
func TestMissedClicksHitSomebodyElseOrNobody(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	clumsy := Temper{Sight: 5, Sloppy: 0.9, Hurt: 0.5, Drained: 0.3}

	missed, hit := 0, 0
	for range 300 {
		b := mindFighting(100, 1000, fullBag, []int{spellParalyze},
			protocol.EntityState{ID: 2, X: 11, Y: 10},
			protocol.EntityState{ID: 3, X: 10, Y: 11},
		)
		act := b.Decide(clumsy, rng, protocol.North, t0)
		if act.cast != spellParalyze {
			continue
		}
		if act.target == 2 {
			hit++
		} else {
			missed++
		}
	}
	if missed == 0 {
		t.Error("con sloppy=0.9 nunca le erró a un clic")
	}
	if hit == 0 {
		t.Error("con sloppy=0.9 nunca le acertó a ninguno: erra siempre, que tampoco es humano")
	}
}

// Pegado a alguien no hay despiste que valga: nadie se pone a mirar el techo
// con un tipo pegándole en la cara. Junto con el dado por decisión, esto es lo
// que arregló el "no atacan mucho".
func TestNeverGetsDistractedWhileToeToToe(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	alwaysDistracted := Temper{Sight: 5, Sloppy: 1, Focus: time.Second, Hurt: 0.5, Drained: 0.3}

	for i := range 50 {
		b := mindAt(10, 10, protocol.EntityState{ID: 2, X: 11, Y: 10})
		act := b.Decide(alwaysDistracted, rng, protocol.North, t0.Add(time.Duration(i)*time.Second))
		if !act.swing {
			t.Fatalf("con sloppy=1 y el rival al lado, no pegó en el intento %d", i)
		}
	}
}

// Una decisión dura lo que dura una decisión. Tirar el dado en cada vuelta del
// loop —cinco veces por segundo— dejaba al bot caminando en zigzag, sin
// alcanzar nunca a nadie: en pantalla eso fue "son fáciles, no atacan".
func TestADecisionSticksForItsFocusWindow(t *testing.T) {
	rng := rand.New(rand.NewSource(2))
	t2 := Temper{Sight: 8, Sloppy: 0.5, Focus: time.Second, Hurt: 0.5, Drained: 0.3}

	// Un solo Brain, perseguido a lo largo de una ventana entera: lo que
	// decida en el primer turno tiene que sostenerlo en los siguientes.
	b := mindAt(10, 10, protocol.EntityState{ID: 2, X: 14, Y: 10})
	first := b.Decide(t2, rng, protocol.West, t0)

	for ms := 100; ms < 900; ms += 100 {
		got := b.Decide(t2, rng, protocol.West, t0.Add(time.Duration(ms)*time.Millisecond))
		if got.dir != first.dir {
			t.Fatalf("a los %dms cambió de idea (%v -> %v) dentro de la misma ventana", ms, first.dir, got.dir)
		}
	}
}

// Tira el hechizo de ataque más fuerte que pueda pagar, no el primero que
// encuentra en el libro.
func TestCastsTheStrongestDamageSpellItCanAfford(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	book := []int{2, 6, 7, 8, 15, 23, 25} // el libro completo de ataque

	cases := []struct {
		mana int
		want int
	}{
		{2000, 25}, // le alcanza para Apocalipsis
		{500, 23},  // no, pero sí para Descarga Eléctrica
		{300, 15},  // Tormenta de Fuego
		{50, 8},    // Proyectil Mágico
		{15, 2},    // apenas el Dardo
	}
	// Sin poción azul en la mochila y sin umbral de maná: con poca barra el bot
	// prioriza beber, que es correcto y es otro test — acá se mira sólo cuál de
	// los siete elige.
	focused := sharp
	focused.Drained = 0
	for _, c := range cases {
		b := mindFighting(100, c.mana, nil, book, protocol.EntityState{ID: 2, X: 14, Y: 10})
		if act := b.Decide(focused, rng, protocol.North, t0); act.cast != c.want {
			t.Errorf("con %d de maná lanzó %d, esperaba %d", c.mana, act.cast, c.want)
		}
	}
}

// Con el libro entero y maná de sobra, paralizar va primero: es lo que decide
// la pelea, y pegarle a alguien que no se puede mover es el orden en que lo
// hace cualquiera.
func TestParalysisComesBeforeDamage(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	book := []int{spellParalyze, 25, 23, 15}

	free := mindFighting(100, 2000, fullBag, book, protocol.EntityState{ID: 2, X: 13, Y: 10})
	if act := free.Decide(sharp, rng, protocol.North, t0); act.cast != spellParalyze {
		t.Errorf("no paralizó primero: lanzó %d", act.cast)
	}

	// Ya paralizado, pasa al daño.
	held := mindFighting(100, 2000, fullBag, book,
		protocol.EntityState{ID: 2, X: 13, Y: 10, Paralyzed: true})
	if act := held.Decide(sharp, rng, protocol.North, t0); act.cast != 25 {
		t.Errorf("contra alguien ya paralizado lanzó %d, esperaba el más fuerte", act.cast)
	}
}

// Los hechizos llegan a todo lo que el bot ve: el servidor deja lanzar a
// cualquiera dentro del viewport, que es más lejos que Temper.sight. Sin esto
// el bot se acercaría a golpear teniendo con qué disparar de lejos.
func TestCastsAtRangeWithoutClosingIn(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	b := mindFighting(100, 2000, fullBag, []int{15}, protocol.EntityState{ID: 2, X: 15, Y: 10})

	act := b.Decide(sharp, rng, protocol.North, t0)
	if act.cast != 15 {
		t.Errorf("a 5 tiles no lanzó nada: cast=%d", act.cast)
	}
	if act.swing {
		t.Error("intentó pegar a 5 tiles de distancia")
	}
}

// Sin maná y sin libro, es un guerrero: se acerca y pega. Cinco de las doce
// clases no tienen maná en absoluto, así que este es el caso de casi la mitad
// del swarm.
func TestAManalessBotIsJustAFighter(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	b := mindFighting(100, 0, nil, nil, protocol.EntityState{ID: 2, X: 11, Y: 10})

	act := b.Decide(sharp, rng, protocol.North, t0)
	if act.cast != 0 {
		t.Errorf("sin maná lanzó %d", act.cast)
	}
	if !act.swing {
		t.Error("no pegó, que era lo único que podía hacer")
	}
}
