package world

import "juegito/server/internal/protocol"

// Los cofres.
//
// Un cofre es un objeto más tirado en el piso, del mismo tipo que Argentum le
// da a los contenedores (ObjType 7), y eso es a propósito: el piso ya viaja por
// viewport, el cliente ya dibuja lo que hay en un tile por su grh, y agarrar ya
// es una tecla. Nada de esto necesitó protocolo nuevo ni un gráfico nuevo — el
// grh 503, "Cofre Cerrado", ya estaba en el atlas.
//
// Lo que cambia es qué pasa cuando lo agarrás: en vez de entrar a la mochila,
// se abre y desparrama equipo que **tu clase puede usar**. Ahí está la
// mecánica: el loot del piso es lo que hay, y un cofre es lo que te sirve. Un
// mago que cruza medio mapa hasta un cofre sabe que no le va a tocar una
// espada que no puede levantar.

const (
	// chestTiles es cada cuántos tiles libres se pone un cofre. Los cofres son
	// el premio grande, así que son mucho más raros que el loot suelto, que va
	// uno cada groundLootTiles.
	chestTiles = 2600

	// chestDrops es cuántas piezas larga un cofre.
	chestDrops = 3
)

// ChestItemID es el "Cofre Cerrado" de obj.dat (OBJ11, grh 503). Se conserva
// el id original en vez de inventar uno: es el mismo objeto, y el día que el
// conversor traiga los contenedores no hay que reconciliar nada.
const ChestItemID = 11

// isChest dice si lo que hay en un tile es un cofre y no loot común.
func (w *World) isChest(itemID int) bool {
	item, ok := w.items[itemID]
	return ok && item.Type == ItemChest
}

// chestLootTable es todo el equipo que esta clase y esta raza pueden usar,
// ponderado como el loot del piso: 1/(1+poder), así lo fuerte sigue siendo
// raro y un cofre no es un pasaporte al mejor martillo del juego.
//
// Solo equipo. Las pociones y la comida se encuentran sueltas por todos lados
// —dos mil y pico por mundo— y meterlas acá haría que el premio grande fuera,
// una de cada tres veces, una manzana.
func chestLootTable(items map[int]Item, class Class, race Race) []lootEntry {
	var table []lootEntry
	for _, item := range items {
		if item.Newbie {
			continue
		}
		var power int
		switch item.Type {
		case ItemWeapon:
			power = item.MaxHit
		case ItemArmor, ItemShield, ItemHelmet:
			power = item.MaxDef
		case ItemRing:
			power = 0
		default:
			continue
		}
		// La misma regla que el servidor ya usa para equipar: la lista CP de
		// obj.dat es de clases prohibidas, y la armadura tiene forma de raza.
		if classForbidsUse(item, class) || !fitsRace(item, race) {
			continue
		}
		table = append(table, lootEntry{Item: item, Weight: 1.0 / float64(1+power)})
	}
	return table
}

// spawnChests siembra los cofres, drenando el mismo pool de tiles libres que
// el resto del loot para que nunca compartan un tile.
func (w *World) spawnChests(pool []tileKey, count int) []tileKey {
	if _, ok := w.items[ChestItemID]; !ok {
		// Sin el objeto en la tabla no hay cofres, y no es motivo para no
		// arrancar: es un servidor con un items.json viejo.
		w.log.Warn("no hay cofres: falta el objeto en items.json", "id", ChestItemID)
		return pool
	}
	placed := 0
	for placed < count && len(pool) > 0 {
		key := pool[len(pool)-1]
		pool = pool[:len(pool)-1]
		w.ground[key] = groundStack{ItemID: ChestItemID, Amount: 1}
		placed++
	}
	w.log.Info("cofres esparcidos", "pedido", count, "colocado", placed)
	return pool
}

// openChest reemplaza el cofre por lo que tenía adentro.
//
// El cofre desaparece: no queda un "Cofre Abierto" ocupando el tile, porque un
// tile lleva un solo objeto y ese objeto tiene que ser el premio. El primero
// que llega se lo lleva, como todo lo demás en el piso.
//
// Lo que sale cae al piso y no a la mochila, y esa es la decisión que hace que
// un cofre sea un lugar y no un botón: tres piezas desparramadas alrededor son
// varios segundos agachado juntándolas, a la vista de cualquiera que venga
// llegando.
func (w *World) openChest(p *Player, key tileKey) {
	delete(w.ground, key)

	table := chestLootTable(w.items, p.Class, p.Race)
	if len(table) == 0 {
		w.log.Warn("cofre vacío: ninguna pieza le sirve a la clase",
			"clase", p.Class, "raza", p.Race)
		return
	}

	// El tile del cofre primero — el que lo abrió está parado ahí y esa pieza
	// la levanta sin moverse — y después los de alrededor.
	//
	// Se piden más lugares de los que hacen falta porque varios se descartan:
	// freeTilesNear mira el mapa y no el piso, así que devuelve tiles que ya
	// tienen algo tirado, y devuelve también el del propio cofre, que ya está
	// en la lista. Un tile lleva un solo objeto, así que el ocupado se saltea
	// sin excepciones — con una excepción para el tile del cofre, la segunda
	// pieza se escribía encima de la primera y el cofre largaba una menos.
	spots := append([]tileKey{key}, w.freeTilesNear(key.X, key.Y, chestDrops*3)...)

	dropped := 0
	for _, spot := range spots {
		if dropped >= chestDrops {
			break
		}
		if _, taken := w.ground[spot]; taken {
			continue
		}
		item := w.pickWeighted(table)
		if item.ID == 0 {
			continue
		}
		w.ground[spot] = groundStack{ItemID: item.ID, Amount: 1}
		dropped++
	}

	w.log.Info("cofre abierto", "quien", p.Name, "clase", p.Class,
		"piezas", dropped, "en", [2]int{key.X, key.Y})

	// El que lo abrió se entera por texto además de por lo que ve: las piezas
	// caen alrededor y alguna puede quedarle fuera de la pantalla.
	w.sendTo(p, protocol.TypeUseResult, protocol.UseResult{
		ItemName: w.items[ChestItemID].Name,
		Opened:   true,
		Dropped:  dropped,
	})
}
