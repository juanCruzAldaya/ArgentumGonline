package main

import (
	"fmt"
	"strconv"
	"strings"
)

// Argentum object types, from the table at the top of obj.dat.
const (
	ObjFood   = 1
	ObjWeapon = 2
	ObjArmor  = 3
	ObjTree   = 4
	ObjMoney  = 5
	ObjPotion = 11
	ObjDrink  = 13
	ObjShield = 16
	ObjHelmet = 17
	ObjRing   = 18
)

// nakedBodies is DarCuerpoDesnudo (General.bas:45-114): the body a character
// wears with no armour on, one per race. The source's table is per race AND
// gender; these are the male column, since this game has no gender. Read the
// values off here rather than off the source's Select Case, whose arm order
// (Humano/Drow/Elfo/Gnomo/Enano) is not the enum order.
//
// Keyed by Argentum's eRaza: 1 Humano, 2 Elfo, 3 Drow, 4 Gnomo, 5 Enano.
var nakedBodies = map[int]int{1: 21, 2: 210, 3: 32, 4: 222, 5: 53}

// ghostBody is iCuerpoMuerto (Declares.bas:560) — what a dead character is
// redrawn as, together with head iCabezaMuerto 500.
const ghostBody = 8

// carriableTypes are the object types worth putting in a battle royale
// inventory. Argentum also defines trees, doors, signs, forums and campfires as
// objects; those are scenery the map places, not loot a player carries.
var carriableTypes = map[int]bool{
	ObjFood: true, ObjWeapon: true, ObjArmor: true, ObjPotion: true,
	ObjDrink: true, ObjShield: true, ObjHelmet: true, ObjRing: true,
}

// Item is one entry of obj.dat, trimmed to what the client and the combat code
// actually need.
type Item struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	// Grh is the inventory icon.
	Grh int `json:"grh"`

	// Body is what this item makes the wearer's body look like: obj.dat's
	// NumRopaje, which the source loads into ObjData.Ropaje and assigns
	// straight to Char.body when armour is equipped (InvUsuario.bas:1395).
	//
	// Read only for armour. FileIO.bas:1110 loads NumRopaje for *every* object
	// type because that line sits outside the ObjType switch, and 13 of 13
	// shields and 20 of 21 helmets carry a leftover NumRopaje=2 — copy-paste
	// of the "none" sentinel. The server never reads it for them, and a
	// converter that did would turn a character's body into 2 the moment they
	// put on a shield.
	Body int `json:"body,omitempty"`

	// Anim is the weapon/shield/helmet animation index, obj.dat's own `Anim`
	// key for all three. Which table it indexes depends on the item's type:
	// weapons into Armas.dat, shields into Escudos.dat, helmets into
	// Cascos.ini. The three are separate tables and the numbers are not
	// interchangeable.
	Anim int `json:"anim,omitempty"`

	// DwarfAnim is RazaEnanaAnim: an alternate weapon animation used when the
	// wielder is an Enano *or a Gnomo* (GetWeaponAnim, Modulo_UsUaRiOs.bas:358
	// — the field name says Enana but the check covers both short races).
	// Weapons only; shields and helmets have no such variant.
	DwarfAnim int `json:"dwarfAnim,omitempty"`
	Type      int `json:"type"`
	MinHit    int `json:"minHit,omitempty"`
	MaxHit    int `json:"maxHit,omitempty"`
	MinDef    int `json:"minDef,omitempty"`
	MaxDef    int `json:"maxDef,omitempty"`
	// StaffPower is real obj.dat data — modHechizos.bas gates NeedStaff
	// spells on a Mago's equipped weapon meeting this — carried over for
	// when spell casting enforces it server-side. Every newbie-tier weapon,
	// including the newbie staff, is StaffPower 0 in the real data, so it
	// does not help tell newbie weapons apart from one another.
	StaffPower int `json:"staffPower,omitempty"`
	// Restores is how much food or drink an item gives back.
	Restores int `json:"restores,omitempty"`
	Value    int `json:"value,omitempty"`

	// Potion fields — see ePocionType in the source: 1 Agilidad, 2 Fuerza,
	// 3 Salud, 4 Mana, 5 CuraVeneno, 6 Negra (a joke item that kills you).
	// MinModificador/MaxModificador is the roll a Salud/Agilidad/Fuerza potion
	// uses; Mana potions ignore both and use a formula instead (ported into
	// the server's useItem, not here — this converter just carries the data).
	PotionType     int `json:"potionType,omitempty"`
	MinModificador int `json:"minMod,omitempty"`
	MaxModificador int `json:"maxMod,omitempty"`
	DuracionEfecto int `json:"potionDuration,omitempty"`

	// ForbiddenClasses is obj.dat's CP1..CP12 fields: "Clase Prohibida", a
	// DENY list, not an allow list — most weapons/armour/shields/helmets/rings
	// name the classes barred from them (Espada Larga: MAGO, DRUIDA, PIRATA,
	// BARDO) rather than the ones permitted. Consumables never carry one.
	// Stored as the raw uppercase class tokens from the source, so the server
	// (which owns the canonical Class enum) resolves them, not this converter.
	ForbiddenClasses []string `json:"forbiddenClasses,omitempty"`
}

// Spell is one entry of Hechizos.dat.
//
// Argentum encodes effects as a "Sube<X>" switch plus a min/max range: 1 raises
// the stat, 2 lowers it. Dardo Mágico for instance is SubeHP=2 with MinHP=2 and
// MaxHP=5, which is 2 to 5 points of damage.
type Spell struct {
	ID     int    `json:"id"`
	Name   string `json:"name"`
	Desc   string `json:"desc,omitempty"`
	Words  string `json:"words,omitempty"`
	Type   int    `json:"type"`
	Target int    `json:"target"`

	MinSkill int `json:"minSkill"`
	Mana     int `json:"mana"`
	Stamina  int `json:"sta"`
	FXGrh    int `json:"fx,omitempty"`

	AffectsHP int `json:"affectsHp,omitempty"` // 1 heals, 2 damages
	MinHP     int `json:"minHp,omitempty"`
	MaxHP     int `json:"maxHp,omitempty"`

	// AffectsAgility/AffectsStrength: 1 buffs, 2 debuffs. Argentum rolls
	// Min..Max and caps a buff at double the target's own base attribute, a
	// debuff at a floor of 1 — see MODATRIBUTOS handling ported into combat.go.
	AffectsAgility  int `json:"affectsAg,omitempty"`
	MinAgility      int `json:"minAg,omitempty"`
	MaxAgility      int `json:"maxAg,omitempty"`
	AffectsStrength int `json:"affectsFu,omitempty"`
	MinStrength     int `json:"minFu,omitempty"`
	MaxStrength     int `json:"maxFu,omitempty"`

	Paralyzes        bool `json:"paralyzes,omitempty"`
	Immobilizes      bool `json:"immobilizes,omitempty"`
	RemovesParalysis bool `json:"removesParalysis,omitempty"`
	Invisibility     bool `json:"invisibility,omitempty"`
}

// iniSections parses one of Argentum's .dat files into section -> key -> value.
//
// Section headers carry trailing comments in some files ("[OBJ46]'Nieve 1"), and
// iniLines already strips those along with the VB comment markers.
func iniSections(path string) (map[string]map[string]string, error) {
	lines, err := iniLines(path)
	if err != nil {
		return nil, err
	}

	out := map[string]map[string]string{}
	current := ""
	for _, line := range lines {
		if strings.HasPrefix(line, "[") {
			if end := strings.Index(line, "]"); end > 0 {
				current = strings.ToUpper(line[1:end])
				out[current] = map[string]string{}
			}
			continue
		}
		if current == "" {
			continue
		}
		if key, value, ok := strings.Cut(line, "="); ok {
			out[current][strings.ToUpper(strings.TrimSpace(key))] = strings.TrimSpace(value)
		}
	}
	return out, nil
}

func sectionInt(section map[string]string, key string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(section[strings.ToUpper(key)]))
	return n
}

func sectionBool(section map[string]string, key string) bool {
	return sectionInt(section, key) != 0
}

// loadItems reads obj.dat, keeping only the types a player can carry.
func loadItems(path string) (map[int]Item, error) {
	sections, err := iniSections(path)
	if err != nil {
		return nil, err
	}

	out := map[int]Item{}
	for name, section := range sections {
		if !strings.HasPrefix(name, "OBJ") {
			continue
		}
		id, err := strconv.Atoi(strings.TrimPrefix(name, "OBJ"))
		if err != nil {
			continue
		}
		objType := sectionInt(section, "ObjType")
		if !carriableTypes[objType] {
			continue
		}

		item := Item{
			ID:             id,
			Name:           section["NAME"],
			Grh:            sectionInt(section, "GrhIndex"),
			Type:           objType,
			MinHit:         sectionInt(section, "MinHit"),
			MaxHit:         sectionInt(section, "MaxHit"),
			MinDef:         sectionInt(section, "MinDef"),
			MaxDef:         sectionInt(section, "MaxDef"),
			StaffPower:     sectionInt(section, "StaffPower"),
			Value:          sectionInt(section, "Valor"),
			PotionType:     sectionInt(section, "TipoPocion"),
			MinModificador: sectionInt(section, "MinModificador"),
			MaxModificador: sectionInt(section, "MaxModificador"),
			DuracionEfecto: sectionInt(section, "DuracionEfecto"),
		}
		// Appearance. NumRopaje is a body only for armour — see the note on
		// Item.Body for why reading it unconditionally would be a bug — and
		// Anim only means something for the three worn types.
		switch objType {
		case ObjArmor:
			item.Body = sectionInt(section, "NumRopaje")
		case ObjWeapon:
			item.Anim = sectionInt(section, "Anim")
			item.DwarfAnim = sectionInt(section, "RazaEnanaAnim")
		case ObjShield, ObjHelmet:
			item.Anim = sectionInt(section, "Anim")
		}
		// Food and drink restore different vitals through different .dat keys
		// that both end up meaning "how much this refills." Drinks are the one
		// surprise: the key on disk is MinAgu, not MinSed — FileIO.bas loads
		// GetValue("OBJ..","MinAgu") straight into the in-memory field the rest
		// of the source calls MinSed, so the name that looks right (MinSed) is
		// actually never populated. Read as MinSed originally here, this always
		// silently returned zero and every drink was inert.
		if v := sectionInt(section, "MinHam"); v > 0 {
			item.Restores = v
		} else if v := sectionInt(section, "MinAgu"); v > 0 {
			item.Restores = v
		}
		// CP1..CP12: up to twelve separate keys, each one class name, not one
		// field holding a list.
		for i := 1; i <= 12; i++ {
			if name := strings.ToUpper(strings.TrimSpace(section[fmt.Sprintf("CP%d", i)])); name != "" {
				item.ForbiddenClasses = append(item.ForbiddenClasses, name)
			}
		}
		if item.Name == "" || item.Grh == 0 {
			continue
		}
		out[id] = item
	}
	return out, nil
}

// loadSpells reads Hechizos.dat.
func loadSpells(path string) (map[int]Spell, error) {
	sections, err := iniSections(path)
	if err != nil {
		return nil, err
	}

	out := map[int]Spell{}
	for name, section := range sections {
		if !strings.HasPrefix(name, "HECHIZO") {
			continue
		}
		id, err := strconv.Atoi(strings.TrimPrefix(name, "HECHIZO"))
		if err != nil {
			continue
		}
		spell := Spell{
			ID:               id,
			Name:             section["NOMBRE"],
			Desc:             section["DESC"],
			Words:            section["PALABRASMAGICAS"],
			Type:             sectionInt(section, "Tipo"),
			Target:           sectionInt(section, "Target"),
			MinSkill:         sectionInt(section, "MinSkill"),
			Mana:             sectionInt(section, "ManaRequerido"),
			Stamina:          sectionInt(section, "StaRequerido"),
			FXGrh:            sectionInt(section, "FXgrh"),
			AffectsHP:        sectionInt(section, "SubeHP"),
			MinHP:            sectionInt(section, "MinHP"),
			MaxHP:            sectionInt(section, "MaxHP"),
			AffectsAgility:   sectionInt(section, "SubeAG"),
			MinAgility:       sectionInt(section, "MinAG"),
			MaxAgility:       sectionInt(section, "MaxAG"),
			AffectsStrength:  sectionInt(section, "SubeFU"),
			MinStrength:      sectionInt(section, "MinFU"),
			MaxStrength:      sectionInt(section, "MaxFU"),
			Paralyzes:        sectionBool(section, "Paraliza"),
			Immobilizes:      sectionBool(section, "Inmoviliza"),
			RemovesParalysis: sectionBool(section, "RemoverParalisis"),
			Invisibility:     sectionBool(section, "Invisibilidad"),
		}
		if spell.Name == "" {
			continue
		}
		out[id] = spell
	}
	return out, nil
}
