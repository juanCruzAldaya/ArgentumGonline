package world

import (
	"testing"

	"juegito/server/internal/protocol"
)

// Ids de la tabla de abajo, con los nombres que tienen en obj.dat.
const (
	testBow    = 10 // arco: proyectil + municiones
	testArrow  = 11 // flecha
	testBlade  = 12 // cuchilla: proyectil sin municiones, se tira y se va
	testSword  = 13 // espada: ni proyectil ni nada que ver
	testShield = 14
)

func rangedWorld(t *testing.T) *World {
	t.Helper()
	w := newTestWorld(t, 60, 60)
	w.SetItems(map[int]Item{
		testBow:    {ID: testBow, Name: "Arco", Type: ItemWeapon, MinHit: 1, MaxHit: 4, Projectile: true, NeedsAmmo: true},
		testArrow:  {ID: testArrow, Name: "Flecha", Type: ItemArrow, MinHit: 1, MaxHit: 3},
		testBlade:  {ID: testBlade, Name: "Cuchillas", Type: ItemWeapon, MinHit: 7, MaxHit: 16, Projectile: true},
		testSword:  {ID: testSword, Name: "Espada", Type: ItemWeapon, MinHit: 5, MaxHit: 10},
		testShield: {ID: testShield, Name: "Escudo", Type: ItemShield, MinDef: 1, MaxDef: 2},
	})
	w.tick = 1000 // pasado el enfriamiento inicial
	return w
}

// archer arma un tirador con arco y carcaj puestos.
func archer(t *testing.T, w *World, name string, x, y int, arrows int) (*Player, *fakeConn) {
	t.Helper()
	p, conn := place(t, w, name, x, y)
	p.Class = Cazador
	p.Attributes = rolledAttributes(Humano)
	p.Skills = startingSkills
	p.Vitals = vitalsFor(Cazador, Humano)
	p.Inventory = []protocol.InventorySlot{
		{Slot: 0, ItemID: testBow, Amount: 1, Equipped: true},
		{Slot: 1, ItemID: testArrow, Amount: arrows, Equipped: true},
	}
	return p, conn
}

func target(t *testing.T, w *World, name string, x, y int) *Player {
	t.Helper()
	p, _ := place(t, w, name, x, y)
	p.Class = Guerrero
	p.Attributes = rolledAttributes(Humano)
	p.Skills = startingSkills
	p.Vitals = vitalsFor(Guerrero, Humano)
	p.Inventory = nil
	return p
}

// lastCombat es el último CombatEvent que recibió una conexión.
func lastCombat(t *testing.T, w *World, conn *fakeConn) protocol.CombatEvent {
	t.Helper()
	var event protocol.CombatEvent
	if err := w.codec.DecodePayload(conn.lastOfType(t, protocol.TypeCombat), &event); err != nil {
		t.Fatalf("decode combat: %v", err)
	}
	return event
}

// El disparo llega a donde el cuerpo a cuerpo no. Es toda la feature: el arco
// era, hasta ahora, un palo con el que se pegaba de cerca.
func TestAShotReachesAcrossTheViewport(t *testing.T) {
	w := rangedWorld(t)
	shooter, _ := archer(t, w, "cazador", 10, 10, 10)
	victim := target(t, w, "victima", 16, 10)

	full := victim.Vitals.HP
	hit := false
	// El acierto es una tirada, así que se dispara hasta que entre una: lo que
	// se prueba es que puede llegar, no que llega siempre.
	for i := 0; i < 40 && !hit; i++ {
		w.tick += attackCooldownTicks
		w.shoot(shooter, victim.ID)
		hit = victim.Vitals.HP < full
	}
	if !hit {
		t.Error("cuarenta flechas a seis tiles y ninguna llegó")
	}
}

// Y no llega más lejos que la vista. Es el mismo límite que tienen los
// hechizos: disparar a alguien que el servidor nunca mandó sería actuar sobre
// información que el cliente no tiene.
func TestAShotStopsAtTheViewport(t *testing.T) {
	w := rangedWorld(t)
	shooter, conn := archer(t, w, "cazador", 10, 10, 10)
	victim := target(t, w, "lejano", 10+ViewportW, 10)

	full := victim.Vitals.HP
	w.shoot(shooter, victim.ID)

	if victim.Vitals.HP != full {
		t.Error("le pegó a alguien fuera de la pantalla")
	}
	if got := lastCombat(t, w, conn).Failed; got == "" {
		t.Error("no le dijo por qué no salió el tiro")
	}
}

// Cada tiro gasta una flecha, entre o no entre. El original saca la munición
// por cualquier disparo que haya salido, y esa es toda la economía del arco.
func TestEveryShotSpendsAnArrow(t *testing.T) {
	w := rangedWorld(t)
	shooter, _ := archer(t, w, "cazador", 10, 10, 3)
	victim := target(t, w, "victima", 14, 10)

	for i := 0; i < 3; i++ {
		w.tick += attackCooldownTicks
		w.shoot(shooter, victim.ID)
	}

	if _, _, loaded := w.equippedAmmo(shooter); loaded {
		t.Fatalf("quedaron flechas después de tres tiros: %v", shooter.Inventory)
	}
	// Y el carcaj vacío desaparece del inventario en vez de quedar en cero,
	// que es lo que dejaría disparar para siempre.
	for _, slot := range shooter.Inventory {
		if slot.ItemID == testArrow {
			t.Errorf("el carcaj vacío quedó en el bolso con %d", slot.Amount)
		}
	}
}

// Sin flechas no se dispara, y se dice. Un arco sin munición es invisible desde
// donde está sentado el jugador: la tecla que deja de hacer algo sería
// indistinguible de un bug.
func TestNoArrowsNoShot(t *testing.T) {
	w := rangedWorld(t)
	shooter, conn := archer(t, w, "cazador", 10, 10, 1)
	victim := target(t, w, "victima", 14, 10)
	shooter.Inventory = shooter.Inventory[:1] // se queda solo con el arco

	full := victim.Vitals.HP
	w.shoot(shooter, victim.ID)

	if victim.Vitals.HP != full {
		t.Error("disparó sin flechas")
	}
	event := lastCombat(t, w, conn)
	if event.Failed == "" {
		t.Error("no avisó que no tenía flechas")
	}
	if !event.Ranged || !event.Mine {
		t.Errorf("el aviso no vino marcado como tiro propio: %+v", event)
	}
}

// Una cuchilla es el tiro mismo: no lleva munición, así que lo que gasta es el
// arma. Es la rama que LanzarProyectil elige con Municiones, no con Proyectil.
func TestAThrownBladeSpendsItself(t *testing.T) {
	w := rangedWorld(t)
	thrower, _ := place(t, w, "pirata", 10, 10)
	thrower.Class = Pirata
	thrower.Attributes = rolledAttributes(Humano)
	thrower.Skills = startingSkills
	thrower.Vitals = vitalsFor(Pirata, Humano)
	thrower.Inventory = []protocol.InventorySlot{
		{Slot: 0, ItemID: testBlade, Amount: 2, Equipped: true},
	}
	victim := target(t, w, "victima", 13, 10)

	w.shoot(thrower, victim.ID)

	if thrower.Inventory[0].Amount != 1 {
		t.Errorf("quedan %d cuchillas después de tirar una, esperaba 1", thrower.Inventory[0].Amount)
	}
}

// Con una espada en la mano no hay nada que disparar.
func TestASwordCannotBeShot(t *testing.T) {
	w := rangedWorld(t)
	shooter, conn := archer(t, w, "guerrero", 10, 10, 10)
	shooter.Inventory = []protocol.InventorySlot{
		{Slot: 0, ItemID: testSword, Amount: 1, Equipped: true},
	}
	victim := target(t, w, "victima", 14, 10)

	full := victim.Vitals.HP
	w.shoot(shooter, victim.ID)

	if victim.Vitals.HP != full {
		t.Error("una espada disparó")
	}
	if lastCombat(t, w, conn).Failed == "" {
		t.Error("no dijo que no tenía con qué disparar")
	}
}

// Dispararse a uno mismo es un misclick, y un misclick no cuesta nada: ni la
// flecha, ni la vida. Es la misma regla a la que llegó el guard de los
// hechizos ofensivos.
func TestYouCannotShootYourself(t *testing.T) {
	w := rangedWorld(t)
	shooter, _ := archer(t, w, "cazador", 10, 10, 5)

	full := shooter.Vitals.HP
	w.shoot(shooter, shooter.ID)

	if shooter.Vitals.HP != full {
		t.Error("se disparó a sí mismo")
	}
	ammo, _, _ := w.equippedAmmo(shooter)
	if ammo.ID != testArrow || shooter.Inventory[1].Amount != 5 {
		t.Errorf("el misclick costó una flecha: %+v", shooter.Inventory)
	}
}

// El tiro comparte el enfriamiento del golpe. Si tuviera uno propio, alternar
// arco y espada sería atacar al doble de velocidad.
func TestShootingSharesTheMeleeCooldown(t *testing.T) {
	w := rangedWorld(t)
	shooter, _ := archer(t, w, "cazador", 10, 10, 10)
	victim := target(t, w, "victima", 14, 10)

	w.shoot(shooter, victim.ID)
	before := shooter.Inventory[1].Amount
	w.shoot(shooter, victim.ID) // en el mismo tick: no sale

	if shooter.Inventory[1].Amount != before {
		t.Error("el segundo tiro salió sin esperar el enfriamiento")
	}
}

// La flecha la ve todo el que la ve pasar, no solo los dos que se están
// peleando. Ver de dónde salió un tiro es cómo una pelea a distancia le avisa
// al resto que hay una pelea — el mismo razonamiento que las palabras mágicas
// sobre la cabeza del que lanza.
func TestTheArrowIsVisibleToTheNeighbours(t *testing.T) {
	w := rangedWorld(t)
	shooter, _ := archer(t, w, "cazador", 10, 10, 5)
	victim := target(t, w, "victima", 14, 10)
	_, nearConn := place(t, w, "mirón", 12, 12)
	_, farConn := place(t, w, "lejos", 10+3*ViewportW, 10)

	w.shoot(shooter, victim.ID)

	var shot protocol.Projectile
	if err := w.codec.DecodePayload(nearConn.lastOfType(t, protocol.TypeProjectile), &shot); err != nil {
		t.Fatalf("decode projectile: %v", err)
	}
	if shot.ItemID != testArrow {
		t.Errorf("voló el objeto %d, esperaba la flecha %d", shot.ItemID, testArrow)
	}
	if shot.FromX != 10 || shot.ToX != 14 {
		t.Errorf("la flecha fue de %d a %d en x, esperaba de 10 a 14", shot.FromX, shot.ToX)
	}
	if farConn.countOfType(t, protocol.TypeProjectile) != 0 {
		t.Error("una flecha a tres pantallas de distancia le llegó a alguien que no podía verla")
	}
	_ = shooter
}

// Disparar delata: el que estaba oculto deja de estarlo, igual que al pegar y
// al lanzar. Si no, el arco sería la única forma de atacar sin aparecer.
func TestShootingRevealsYou(t *testing.T) {
	w := rangedWorld(t)
	shooter, _ := archer(t, w, "cazador", 10, 10, 5)
	victim := target(t, w, "victima", 14, 10)
	shooter.InvisibleUntil = w.tick + 1000
	shooter.HiddenBySkill = true

	w.shoot(shooter, victim.ID)

	if shooter.invisible(w.tick) {
		t.Error("disparó y siguió oculto")
	}
}

// Un muerto no dispara.
func TestTheDeadDoNotShoot(t *testing.T) {
	w := rangedWorld(t)
	shooter, _ := archer(t, w, "cazador", 10, 10, 5)
	victim := target(t, w, "victima", 14, 10)
	shooter.Dead = true

	w.shoot(shooter, victim.ID)

	if shooter.Inventory[1].Amount != 5 {
		t.Error("un fantasma gastó una flecha")
	}
}

// El daño del tiro suma la flecha al arco: es la línea que CalcularDano agrega
// en su rama de proyectiles, y es lo que hace que la flecha importe.
func TestTheArrowAddsItsOwnDamage(t *testing.T) {
	w := rangedWorld(t)
	shooter, _ := archer(t, w, "cazador", 10, 10, 5)

	bow := w.items[testBow]
	arrow := w.items[testArrow]
	// El piso del daño con munición tiene que superar al techo sin ella: con
	// arco 1-4 y flecha 1-3 no hay solapamiento posible si la flecha suma.
	loaded, bare := 0, 0
	for i := 0; i < 200; i++ {
		loaded += w.calcShotDamage(shooter, bow, arrow, true)
		bare += w.calcShotDamage(shooter, bow, Item{}, false)
	}
	if loaded <= bare {
		t.Errorf("con flecha pegó %d y sin flecha %d en 200 tiros", loaded, bare)
	}
}

// Y el Cazador es el que mejor tira. No es una opinión: son las columnas
// [MODATAQUEPROYECTILES] y [MODDANOPROYECTILES] de Balance.dat, donde es la
// única clase que llega a 1,0 y a 1,1.
func TestTheHunterIsTheBestArcher(t *testing.T) {
	w := rangedWorld(t)
	hunter, _ := archer(t, w, "cazador", 10, 10, 5)
	mage, _ := archer(t, w, "mago", 12, 10, 5)
	mage.Class = Mago
	mage.Attributes = hunter.Attributes
	mage.Vitals = hunter.Vitals

	if w.poderAtaqueProyectil(hunter) <= w.poderAtaqueProyectil(mage) {
		t.Error("el mago apunta igual o mejor que el cazador")
	}
	if classModifiers[Cazador].DanoProyectiles <= classModifiers[Cazador].DanoArmas {
		t.Error("el cazador pega más fuerte con el arma blanca que con el arco")
	}
}

// Usar un arco equipado no lo desequipa ni lo consume: pide puntería. Es la
// rama otWeapon de UsarInvItem, y es el modelo de input entero del original —
// el arco se equipa como cualquier arma y después se **usa**, con el mismo
// doble clic que una poción, y lo que hace al usarse es levantar la mira.
func TestUsingAnEquippedBowAsksForATarget(t *testing.T) {
	w := rangedWorld(t)
	shooter, conn := archer(t, w, "cazador", 10, 10, 5)

	w.useItem(shooter, 0, protocol.UseAuto)

	var result protocol.UseResult
	if err := w.codec.DecodePayload(conn.lastOfType(t, protocol.TypeUseResult), &result); err != nil {
		t.Fatalf("decode useResult: %v", err)
	}
	if !result.Aim {
		t.Errorf("usar el arco no pidió objetivo: %+v", result)
	}
	if result.Unequipped || result.Consumed {
		t.Errorf("usar el arco lo desequipó o lo consumió: %+v", result)
	}
	if !shooter.Inventory[0].Equipped {
		t.Error("el arco quedó desequipado")
	}
}

// Y con el arco en la mochila contesta lo que contesta el original, que es una
// instrucción y no un silencio.
func TestUsingABowInTheBagSaysToEquipItFirst(t *testing.T) {
	w := rangedWorld(t)
	shooter, conn := archer(t, w, "cazador", 10, 10, 5)
	shooter.Inventory[0].Equipped = false

	w.useItem(shooter, 0, protocol.UseAuto)

	var result protocol.UseResult
	if err := w.codec.DecodePayload(conn.lastOfType(t, protocol.TypeUseResult), &result); err != nil {
		t.Fatalf("decode useResult: %v", err)
	}
	if result.Aim {
		t.Error("pidió objetivo con el arco sin equipar")
	}
	if result.Failed == "" {
		t.Error("no dijo que hay que equiparlo primero")
	}
}

// La tecla E sigue equipando y desequipando el arco: apuntar es usar, equipar
// es equipar, y una no puede comerse a la otra.
func TestEquipStillTogglesTheBow(t *testing.T) {
	w := rangedWorld(t)
	shooter, conn := archer(t, w, "cazador", 10, 10, 5)

	w.useItem(shooter, 0, protocol.UseEquip)

	if shooter.Inventory[0].Equipped {
		t.Error("E no desequipó el arco")
	}
	var result protocol.UseResult
	if err := w.codec.DecodePayload(conn.lastOfType(t, protocol.TypeUseResult), &result); err != nil {
		t.Fatalf("decode useResult: %v", err)
	}
	if !result.Unequipped || result.Aim {
		t.Errorf("E contestó otra cosa: %+v", result)
	}
}
