package world

import (
	"testing"

	"juegito/server/internal/protocol"
)

// chestWorld es un mundo con cofres y con equipo de dos clases distintas, para
// poder comprobar que lo que sale de un cofre depende de quién lo abre.
func chestWorld(t *testing.T) *World {
	t.Helper()
	w := newTestWorld(t, 40, 40)
	w.SetItems(map[int]Item{
		ChestItemID: {ID: ChestItemID, Name: "Cofre Cerrado", Type: ItemChest},

		// Prohibida para el mago, que es la mitad de la prueba.
		100: {ID: 100, Name: "Espada Larga", Type: ItemWeapon, MinHit: 5, MaxHit: 10,
			ForbiddenClasses: []string{"MAGO"}},
		101: {ID: 101, Name: "Daga", Type: ItemWeapon, MinHit: 1, MaxHit: 4},
		102: {ID: 102, Name: "Túnica", Type: ItemArmor, MinDef: 1, MaxDef: 3},

		// Nada de esto tiene que salir de un cofre: las pociones se encuentran
		// sueltas por todo el mapa y el tier newbie no existe en este juego.
		200: {ID: 200, Name: "Poción Roja", Type: ItemPotion, PotionType: PotionHealth},
		201: {ID: 201, Name: "Daga (Newbie)", Type: ItemWeapon, MaxHit: 2, Newbie: true},
	})
	return w
}

// Un cofre no se levanta: se abre, y en su lugar queda equipo.
func TestOpeningAChestLeavesGearInstead(t *testing.T) {
	w := chestWorld(t)
	p, _ := place(t, w, "wachin", 10, 10)
	// El piso arranca sembrado de loot: contar lo que cayó del cofre exige que
	// no haya nada más.
	clear(w.ground)
	w.ground[tileKey{10, 10}] = groundStack{ItemID: ChestItemID, Amount: 1}
	before := len(p.Inventory)

	w.pickup(p)

	if len(p.Inventory) != before {
		t.Error("el cofre entró a la mochila en vez de abrirse")
	}
	if stack, ok := w.ground[tileKey{10, 10}]; ok && stack.ItemID == ChestItemID {
		t.Error("el cofre sigue cerrado en el piso")
	}
	dropped := 0
	for _, stack := range w.ground {
		if stack.ItemID != ChestItemID {
			dropped++
		}
	}
	if dropped != chestDrops {
		t.Errorf("cayeron %d piezas, esperaba %d", dropped, chestDrops)
	}
}

// Lo que sale es para la clase del que lo abrió: es la razón de ser del cofre
// contra el loot suelto, que es lo que hay y no lo que te sirve.
func TestAChestNeverDropsWhatTheClassCannotUse(t *testing.T) {
	w := chestWorld(t)
	p, _ := place(t, w, "mago", 10, 10)
	p.Class = Mago

	// Muchas aperturas, porque la tabla se sortea: una sola no probaría nada.
	for i := 0; i < 60; i++ {
		clear(w.ground)
		w.ground[tileKey{10, 10}] = groundStack{ItemID: ChestItemID, Amount: 1}
		w.pickup(p)
		for at, stack := range w.ground {
			if stack.ItemID == 100 {
				t.Fatalf("le salió al mago una Espada Larga en %v", at)
			}
			if item := w.items[stack.ItemID]; item.Newbie || item.Type == ItemPotion {
				t.Fatalf("salió algo que no es equipo: %s", item.Name)
			}
		}
	}
}

// El que lo abre se entera: parte de lo que cae puede quedarle fuera de la
// pantalla, así que el mensaje dice cuántas piezas fueron.
func TestOpeningAChestTellsThePlayer(t *testing.T) {
	w := chestWorld(t)
	p, conn := place(t, w, "wachin", 10, 10)
	w.ground[tileKey{10, 10}] = groundStack{ItemID: ChestItemID, Amount: 1}

	w.pickup(p)

	var result protocol.UseResult
	if err := w.codec.DecodePayload(conn.lastOfType(t, protocol.TypeUseResult), &result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !result.Opened {
		t.Error("no se avisó que era un cofre")
	}
	if result.Dropped != chestDrops {
		t.Errorf("informó %d piezas, esperaba %d", result.Dropped, chestDrops)
	}
}

// Un cofre en un rincón sin lugar alrededor tiene que largar lo que entre, no
// tragarse el premio ni pisar lo que ya estaba en el piso.
func TestAChestInACornerStillDropsWhatFits(t *testing.T) {
	w := chestWorld(t)
	p, _ := place(t, w, "wachin", 1, 1)
	for y := 0; y < 40; y++ {
		for x := 0; x < 40; x++ {
			if x > 2 || y > 2 {
				w.grid.SetBlocked(x, y, true)
			}
		}
	}
	w.ground[tileKey{1, 1}] = groundStack{ItemID: ChestItemID, Amount: 1}
	w.ground[tileKey{2, 2}] = groundStack{ItemID: 102, Amount: 1} // ya ocupado

	w.pickup(p)

	if stack, ok := w.ground[tileKey{2, 2}]; !ok || stack.ItemID != 102 {
		t.Error("el cofre pisó lo que ya había en el piso")
	}
	if _, ok := w.ground[tileKey{1, 1}]; !ok {
		t.Error("no cayó nada en el tile del propio cofre")
	}
}

// La clase que tiene todo prohibido no rompe nada: el cofre se abre, no larga
// nada y el servidor sigue en pie.
func TestAChestWithNothingForYouOpensAnyway(t *testing.T) {
	w := newTestWorld(t, 20, 20)
	w.SetItems(map[int]Item{
		ChestItemID: {ID: ChestItemID, Name: "Cofre Cerrado", Type: ItemChest},
		100: {ID: 100, Name: "Espada Larga", Type: ItemWeapon, MaxHit: 10,
			ForbiddenClasses: []string{"MAGO"}},
	})
	p, _ := place(t, w, "mago", 5, 5)
	p.Class = Mago
	w.ground[tileKey{5, 5}] = groundStack{ItemID: ChestItemID, Amount: 1}

	w.pickup(p)

	if _, ok := w.ground[tileKey{5, 5}]; ok {
		t.Error("el cofre quedó en el piso")
	}
}
