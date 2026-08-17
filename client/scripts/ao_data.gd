class_name AOData
extends RefCounted
## Item and spell definitions, converted from Argentum's obj.dat and
## Hechizos.dat by tools/aoconv.
##
## Both ship with the client and are addressed by id over the wire, so a bag
## full of loot costs a handful of numbers rather than a list of names.

const ITEMS_PATH := "res://assets/ao/items.json"
const SPELLS_PATH := "res://assets/ao/spells.json"

## Argentum object types, from the table at the top of obj.dat.
const TYPE_FOOD := 1
const TYPE_WEAPON := 2
const TYPE_ARMOR := 3
const TYPE_POTION := 11
const TYPE_DRINK := 13
const TYPE_SHIELD := 16
const TYPE_HELMET := 17
const TYPE_RING := 18

## obj.dat's ePocionType, in the server's own order (see items.go). The HUD
## reads these to count red/blue potions for the two quick-slot boxes the
## panel artwork draws.
const POTION_AGILITY := 1
const POTION_STRENGTH := 2
const POTION_HEALTH := 3
const POTION_MANA := 4
const POTION_CURE_POISON := 5
const POTION_BLACK := 6

## Class and race names, in the exact order the server's Class/Race enums
## define them (see server/internal/world/balance.go's allClasses/allRaces).
## The character picker sends the index, not the name, so this order is the
## actual contract between client and server — reordering either side without
## the other silently reassigns everyone's class.
const CLASS_NAMES := [
	"Guerrero", "Cazador", "Paladín", "Bandido", "Asesino", "Pirata",
	"Ladrón", "Clérigo", "Bardo", "Mago", "Druida", "Trabajador",
]
const RACE_NAMES := ["Humano", "Elfo", "Elfo Oscuro", "Enano", "Gnomo"]

var _items: Dictionary = {}
var _spells: Dictionary = {}
var _loaded := false


func load_data() -> bool:
	if _loaded:
		return true
	_items = _read(ITEMS_PATH)
	_spells = _read(SPELLS_PATH)
	_loaded = not _items.is_empty() or not _spells.is_empty()
	return _loaded


func item(id: int) -> Dictionary:
	var entry: Variant = _items.get(str(id))
	return entry if typeof(entry) == TYPE_DICTIONARY else {}


func spell(id: int) -> Dictionary:
	var entry: Variant = _spells.get(str(id))
	return entry if typeof(entry) == TYPE_DICTIONARY else {}


## spell_summary is the one-line form the spell list shows: what it costs and,
## for the offensive ones, what it does.
func spell_summary(id: int) -> String:
	var s := spell(id)
	if s.is_empty():
		return "hechizo %d" % id

	var parts: Array[String] = []
	var mana := int(s.get("mana", 0))
	if mana > 0:
		parts.append("%d maná" % mana)

	var affects := int(s.get("affectsHp", 0))
	var low := int(s.get("minHp", 0))
	var high := int(s.get("maxHp", 0))
	if affects == 2 and high > 0:
		parts.append("%d-%d daño" % [low, high])
	elif affects == 1 and high > 0:
		parts.append("cura %d-%d" % [low, high])
	elif bool(s.get("paralyzes", false)):
		parts.append("paraliza")
	elif bool(s.get("immobilizes", false)):
		parts.append("inmoviliza")
	elif bool(s.get("removesParalysis", false)):
		parts.append("remueve parálisis")
	elif bool(s.get("invisibility", false)):
		parts.append("invisibilidad")

	return "" if parts.is_empty() else "  ·  ".join(parts)


func _read(path: String) -> Dictionary:
	if not FileAccess.file_exists(path):
		push_warning("falta %s; corré tools/aoconv" % path)
		return {}
	var parsed: Variant = JSON.parse_string(FileAccess.get_file_as_string(path))
	if typeof(parsed) != TYPE_DICTIONARY:
		push_error("%s ilegible" % path)
		return {}
	return parsed
