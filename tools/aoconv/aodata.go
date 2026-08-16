package main

import (
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
	Grh    int `json:"grh"`
	Type   int `json:"type"`
	MinHit int `json:"minHit,omitempty"`
	MaxHit int `json:"maxHit,omitempty"`
	MinDef int `json:"minDef,omitempty"`
	MaxDef int `json:"maxDef,omitempty"`
	// Restores is how much food or drink an item gives back.
	Restores int `json:"restores,omitempty"`
	Value    int `json:"value,omitempty"`
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
			ID:     id,
			Name:   section["NAME"],
			Grh:    sectionInt(section, "GrhIndex"),
			Type:   objType,
			MinHit: sectionInt(section, "MinHit"),
			MaxHit: sectionInt(section, "MaxHit"),
			MinDef: sectionInt(section, "MinDef"),
			MaxDef: sectionInt(section, "MaxDef"),
			Value:  sectionInt(section, "Valor"),
		}
		// Food and drink use different keys for the same idea.
		if v := sectionInt(section, "MinHAM"); v > 0 {
			item.Restores = v
		} else if v := sectionInt(section, "MinSed"); v > 0 {
			item.Restores = v
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
