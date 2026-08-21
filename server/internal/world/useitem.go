package world

import "juegito/server/internal/protocol"

// useCooldownTicks paces potions the way IntervaloPermiteGolpeUsar does in the
// source: you cannot chug them back to back.
const useCooldownTicks = 6 // 0.3s at 20Hz

// useItem is EquiparInvItem plus the otPociones/otUseOnce/otBebidas branches of
// the inventory click handler, folded into one entry point because that is
// what a single inventory click is in the source: which branch runs depends
// on the item's own ObjType, never on what the client claims to be doing.
func (w *World) useItem(p *Player, slotIndex int, action protocol.UseAction) {
	if p.Dead {
		w.sendTo(p, protocol.TypeUseResult, protocol.UseResult{Failed: "Estás muerto."})
		return
	}
	// HandleUseItem's own gate: meditating blocks the inventory entirely, not
	// just equipping — the source's error comment says the client should have
	// already caught this, but the server holds the line either way.
	if p.Meditating {
		w.sendTo(p, protocol.TypeUseResult, protocol.UseResult{Failed: "No podés hacer eso mientras meditás."})
		return
	}

	slot, idx := findSlot(p.Inventory, slotIndex)
	if idx < 0 {
		return // no such slot; a stale client action, not worth answering
	}
	item, ok := w.items[slot.ItemID]
	if !ok {
		return
	}

	// Un arma de proyectiles no se usa como se usa una poción: **usarla es
	// apuntar**. Es literal del original — UsarInvItem, rama otWeapon:
	//
	//	If ObjData(ObjIndex).proyectil = 1 Then
	//	    If .Invent.Object(Slot).Equipped = 0 Then
	//	        "Antes de usar el arco deberias equipartelo."
	//	    Call WriteWorkRequestTarget(UserIndex, Proyectiles)
	//
	// Va antes del switch de abajo justamente porque ese switch contestaría
	// "eso no se consume, se equipa", que para un arco es la respuesta a otra
	// pregunta. Equipar sigue siendo E, y sigue siendo lo primero que hay que
	// hacer: acá se pide puntería, no el arma.
	if item.Type == ItemWeapon && item.Projectile && action != protocol.UseEquip {
		w.aimProjectile(p, idx, item)
		return
	}

	// An explicit action that does not match the slot is refused and said out
	// loud. The whole point of separating E from U is that neither one ever
	// quietly does the other's job — a mistyped equip must not drink anything.
	equipment := equipmentTypes[item.Type]
	switch {
	case action == protocol.UseEquip && !equipment:
		w.sendTo(p, protocol.TypeUseResult, protocol.UseResult{Failed: "Eso no se equipa."})
		return
	case action == protocol.UseUseUp && equipment:
		w.sendTo(p, protocol.TypeUseResult, protocol.UseResult{Failed: "Eso no se consume, se equipa."})
		return
	}

	if equipment {
		w.equip(p, idx, item)
		return
	}

	if w.tick-p.lastUseTick < useCooldownTicks {
		return
	}
	p.lastUseTick = w.tick

	switch item.Type {
	case ItemPotion:
		w.drinkPotion(p, idx, item)
	case ItemFood:
		w.consumeFlat(p, idx, item, &p.Vitals.Hunger, p.Vitals.MaxHunger, "hambre")
	case ItemDrink:
		w.consumeFlat(p, idx, item, &p.Vitals.Thirst, p.Vitals.MaxThirst, "sed")
	}
}

// swapSlots is the inventory window's own drag-and-drop: reorder two bag
// positions without touching what's equipped or how much of anything is
// carried. Landing on an empty slot just relabels the moved item's own Slot
// field rather than needing a partner to trade with — there's nothing there
// to swap with.
func (w *World) swapSlots(p *Player, from, to int) {
	if p.Dead || from == to {
		return
	}
	_, fi := findSlot(p.Inventory, from)
	if fi < 0 {
		return // nothing at the source; a stale client action
	}
	if _, ti := findSlot(p.Inventory, to); ti >= 0 {
		p.Inventory[fi].Slot, p.Inventory[ti].Slot = to, from
	} else {
		p.Inventory[fi].Slot = to
	}
	w.sendLoadout(p)
}

func findSlot(inv []protocol.InventorySlot, slotIndex int) (protocol.InventorySlot, int) {
	for i, s := range inv {
		if s.Slot == slotIndex {
			return s, i
		}
	}
	return protocol.InventorySlot{}, -1
}

// freeSlotNumber is the lowest slot index not already occupied. Inventories
// here are unbounded and consumeStack removes slots from the middle rather
// than leaving zeroed placeholders, so "lowest free index" — scanned fresh
// each time — is simpler than a counter that would need seeding from
// whatever the starting kit already used.
func freeSlotNumber(inv []protocol.InventorySlot) int {
	used := make(map[int]bool, len(inv))
	for _, s := range inv {
		used[s.Slot] = true
	}
	for i := 0; ; i++ {
		if !used[i] {
			return i
		}
	}
}

// equip is EquiparInvItem's weapon/shield/armour/helmet/ring branches, which
// all follow the same shape: clicking what you're wearing takes it off;
// clicking something else takes off whatever else of that type you had on
// first, since only one of each slot can be worn at once.
func (w *World) equip(p *Player, idx int, item Item) {
	if p.Inventory[idx].Equipped {
		p.Inventory[idx].Equipped = false
		w.sendLoadout(p)
		w.sendTo(p, protocol.TypeUseResult, protocol.UseResult{ItemName: item.Name, Unequipped: true})
		return
	}

	// Every class starts with gear it can already use (see loadout.go), but
	// ground loot is unfiltered — this is the same ClasePuedeUsarItem gate
	// classic AO runs in EquiparInvItem, and it only matters now that a class
	// can pick up something it was never handed at spawn.
	if classForbidsUse(item, p.Class) {
		w.sendTo(p, protocol.TypeUseResult, protocol.UseResult{ItemName: item.Name, Failed: "Tu clase no puede usar este objeto."})
		return
	}

	for i := range p.Inventory {
		if i == idx {
			continue
		}
		if other, ok := w.items[p.Inventory[i].ItemID]; ok && other.Type == item.Type {
			p.Inventory[i].Equipped = false
		}
	}
	p.Inventory[idx].Equipped = true
	w.sendLoadout(p)
	w.sendTo(p, protocol.TypeUseResult, protocol.UseResult{ItemName: item.Name, Equipped: true})
}

// drinkPotion is the otPociones Select Case in the source, one arm per
// ePocionType. Every arm ends by removing one unit of the potion, which
// consumeStack does for whichever arm ran.
func (w *World) drinkPotion(p *Player, idx int, item Item) {
	// Consumed va desde el vamos y no al final: es lo que hace que el trago se
	// oiga (el SND_BEBER del original, ver el audio.gd del cliente), y sin él
	// la poción se tomaba en silencio. El campo estaba en el protocolo desde
	// que existe UseResult y nadie lo había llenado nunca — el cliente no tenía
	// para qué mirarlo hasta que hubo sonido.
	result := protocol.UseResult{ItemName: item.Name, Consumed: true}

	switch item.PotionType {
	case PotionHealth: // Roja: instant, no duration
		healed := w.randRange(item.MinModificador, item.MaxModificador)
		if room := p.Vitals.MaxHP - p.Vitals.HP; healed > room {
			healed = room
		}
		p.Vitals.HP += healed
		result.HealedHP = healed

	case PotionMana:
		// The source ignores the item's own MinModificador/MaxModificador for
		// mana and uses a formula instead: Porcentaje(MaxMana,4) + ELV/2 +
		// 40/ELV. Ported as written, quirk included — ELV is always positive
		// here since every match starts characters at maxLevel.
		restored := p.Vitals.MaxMana*4/100 + p.Vitals.Level/2 + 40/p.Vitals.Level
		if room := p.Vitals.MaxMana - p.Vitals.Mana; restored > room {
			restored = room
		}
		p.Vitals.Mana += restored
		result.RestoredMana = restored

	// Potions accumulate towards double the base attribute; spells overwrite.
	// See drinkAgility for why these no longer borrow the spell path.
	case PotionAgility:
		result.AgilityDelta = w.drinkAgility(p, item.MinModificador, item.MaxModificador)

	case PotionStrength:
		result.StrengthDelta = w.drinkStrength(p, item.MinModificador, item.MaxModificador)

	case PotionCurePoison:
		// Poison is not modelled yet (see README) — nothing currently
		// inflicts it, so this is always a no-op. Still consumes the potion,
		// same as the source.
		result.CuredPoison = false

	case PotionBlack:
		// The joke item: an instant, unconditional kill. classic AO gated
		// this to non-GM accounts; there is no GM concept in a match, so it
		// simply always fires.
		result.Died = true
		w.log.Info("player drank the black potion", "who", p.Name, "alive", w.aliveCount())
	}

	// Consume the bottle before dying, not after: kill() scatters the bag and
	// nils it, and consumeStack indexing into that afterward would panic.
	// Nobody is credited with the kill — you did this to yourself.
	consumeStack(p, idx)
	if result.Died {
		w.kill(p, nil)
	}
	w.sendLoadout(p)
	w.sendTo(p, protocol.TypeUseResult, result)
}

// consumeFlat is the otUseOnce (food) / otBebidas (drink) branch: a flat
// restore to one vital, no roll.
func (w *World) consumeFlat(p *Player, idx int, item Item, vital *int, max int, label string) {
	restored := item.Restores
	if room := max - *vital; restored > room {
		restored = room
	}
	*vital += restored

	result := protocol.UseResult{ItemName: item.Name, Consumed: true}
	if label == "hambre" {
		result.RestoredHunger = restored
	} else {
		result.RestoredThirst = restored
	}

	consumeStack(p, idx)
	w.sendLoadout(p)
	w.sendTo(p, protocol.TypeUseResult, result)
}

// consumeStack removes one unit from a stack, dropping the slot entirely once
// it hits zero — QuitarUserInvItem(Userindex, Slot, 1) in the source.
func consumeStack(p *Player, idx int) {
	p.Inventory[idx].Amount--
	if p.Inventory[idx].Amount <= 0 {
		p.Inventory = append(p.Inventory[:idx], p.Inventory[idx+1:]...)
	}
}

func (w *World) sendLoadout(p *Player) {
	w.sendTo(p, protocol.TypeLoadout, protocol.Loadout{Inventory: p.Inventory, Spells: p.Spells})
}
