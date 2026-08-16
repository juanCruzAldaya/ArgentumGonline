package world

import "juegito/server/internal/protocol"

// useCooldownTicks paces potions the way IntervaloPermiteGolpeUsar does in the
// source: you cannot chug them back to back.
const useCooldownTicks = 6 // 0.3s at 20Hz

// useItem is EquiparInvItem plus the otPociones/otUseOnce/otBebidas branches of
// the inventory click handler, folded into one entry point because that is
// what a single inventory click is in the source: which branch runs depends
// on the item's own ObjType, never on what the client claims to be doing.
func (w *World) useItem(p *Player, slotIndex int) {
	if p.Dead {
		w.sendTo(p, protocol.TypeUseResult, protocol.UseResult{Failed: "Estás muerto."})
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

	if equipmentTypes[item.Type] {
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

func findSlot(inv []protocol.InventorySlot, slotIndex int) (protocol.InventorySlot, int) {
	for i, s := range inv {
		if s.Slot == slotIndex {
			return s, i
		}
	}
	return protocol.InventorySlot{}, -1
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
	result := protocol.UseResult{ItemName: item.Name}

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

	case PotionAgility:
		result.AgilityDelta = w.applyAgility(p, Spell{MinAgility: item.MinModificador, MaxAgility: item.MaxModificador}, attributeBuff)

	case PotionStrength:
		result.StrengthDelta = w.applyStrength(p, Spell{MinStrength: item.MinModificador, MaxStrength: item.MaxModificador}, attributeBuff)

	case PotionCurePoison:
		// Poison is not modelled yet (see README) — nothing currently
		// inflicts it, so this is always a no-op. Still consumes the potion,
		// same as the source.
		result.CuredPoison = false

	case PotionBlack:
		// The joke item: an instant, unconditional kill. classic AO gated
		// this to non-GM accounts; there is no GM concept in a match, so it
		// simply always fires.
		p.Vitals.HP = 0
		p.Dead = true
		delete(w.occupied, tileKey{p.X, p.Y})
		result.Died = true
		w.log.Info("player drank the black potion", "who", p.Name, "alive", w.aliveCount())
	}

	consumeStack(p, idx)
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

	result := protocol.UseResult{ItemName: item.Name}
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
