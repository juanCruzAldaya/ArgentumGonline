extends Control
## The classic Argentum side panel plus the readouts a battle royale needs.
##
## main.tscn defines structure and position; everything cosmetic is built here,
## so the palette and the slot counts live in one readable place instead of
## being spread across a .tscn nobody wants to hand-edit.

const COLOR_PANEL := Color("2a2118")
const COLOR_PANEL_EDGE := Color("120e09")
const COLOR_SLOT := Color("1b150f")
const COLOR_SLOT_EDGE := Color("3d3125")
const COLOR_TEXT := Color("d9cdb4")
const COLOR_TEXT_DIM := Color("8a7c63")
const COLOR_ACCENT := Color("d9b45b")
const COLOR_HP := Color("a63232")
const COLOR_MANA := Color("2f5fa8")
const COLOR_STAMINA := Color("c2a33a")
const COLOR_OVERLAY := Color(0.07, 0.06, 0.04, 0.72)

const INVENTORY_SLOTS := 20
const SLOT_SIZE := 38

## Spell names are cosmetic until casting exists server-side. They are drawn
## dimmed so the panel can be judged at full size without implying the list
## does anything yet.
const PLACEHOLDER_SPELLS := [
	"Dardo Mágico",
	"Curar Heridas Leves",
	"Inmovilizar",
	"Remover Parálisis",
	"Tormenta de Fuego",
	"Apocalipsis",
]

@onready var _alive: Label = $TopBar/Alive
@onready var _zone: Label = $TopBar/Zone
@onready var _log: RichTextLabel = $Console/Log
@onready var _char_name: Label = $SidePanel/CharName
@onready var _char_class: Label = $SidePanel/CharClass

var _bars: Dictionary = {}


func _ready() -> void:
	_style_panels()
	_style_top_bar()
	_style_side_panel()
	_build_bars()
	_build_inventory()
	_build_spells()


func set_character(player_name: String, level: int) -> void:
	_char_name.text = player_name
	# No class selection exists yet, so saying so is more honest than inventing
	# one the server would later contradict.
	_char_class.text = "sin clase  ·  nivel %d" % level


func set_vitals(vitals: Dictionary) -> void:
	_apply_bar("hp", int(vitals.get("hp", 0)), int(vitals.get("maxHp", 1)))
	_apply_bar("mana", int(vitals.get("mana", 0)), int(vitals.get("maxMana", 1)))
	_apply_bar("sta", int(vitals.get("sta", 0)), int(vitals.get("maxSta", 1)))


func set_alive(count: int) -> void:
	_alive.text = "◈  VIVOS  %d" % count


func set_zone(text: String) -> void:
	_zone.text = text


func log_line(text: String, color: Color = COLOR_TEXT) -> void:
	_log.append_text("[color=#%s]%s[/color]\n" % [color.to_html(false), text])


func _apply_bar(key: String, value: int, maximum: int) -> void:
	var entry: Dictionary = _bars[key]
	var bar: ProgressBar = entry["bar"]
	bar.max_value = maxi(maximum, 1)
	bar.value = value
	entry["label"].text = "%s   %d / %d" % [entry["caption"], value, maximum]


func _build_bars() -> void:
	var container: VBoxContainer = $SidePanel/Bars
	container.add_theme_constant_override("separation", 8)
	_add_bar(container, "hp", "VIDA", COLOR_HP)
	_add_bar(container, "mana", "MANÁ", COLOR_MANA)
	_add_bar(container, "sta", "ENERGÍA", COLOR_STAMINA)


func _add_bar(parent: VBoxContainer, key: String, caption: String, color: Color) -> void:
	var row := VBoxContainer.new()
	row.add_theme_constant_override("separation", 2)

	var label := Label.new()
	label.text = caption
	label.add_theme_color_override("font_color", COLOR_TEXT_DIM)
	label.add_theme_font_size_override("font_size", 11)

	var bar := ProgressBar.new()
	bar.custom_minimum_size = Vector2(0, 15)
	bar.show_percentage = false
	bar.add_theme_stylebox_override("background", _flat(COLOR_SLOT, COLOR_SLOT_EDGE))
	bar.add_theme_stylebox_override("fill", _flat(color, color.darkened(0.4)))

	row.add_child(label)
	row.add_child(bar)
	parent.add_child(row)

	_bars[key] = {"bar": bar, "label": label, "caption": caption}


func _build_inventory() -> void:
	var grid: GridContainer = $SidePanel/InvGrid
	grid.add_theme_constant_override("h_separation", 5)
	grid.add_theme_constant_override("v_separation", 5)

	for i in INVENTORY_SLOTS:
		var slot := Panel.new()
		slot.custom_minimum_size = Vector2(SLOT_SIZE, SLOT_SIZE)
		slot.mouse_filter = Control.MOUSE_FILTER_IGNORE
		slot.add_theme_stylebox_override("panel", _flat(COLOR_SLOT, COLOR_SLOT_EDGE))
		grid.add_child(slot)


func _build_spells() -> void:
	var list: VBoxContainer = $SidePanel/SpellList
	list.add_theme_constant_override("separation", 3)

	for i in PLACEHOLDER_SPELLS.size():
		var entry := Label.new()
		entry.text = "%d.  %s" % [i + 1, PLACEHOLDER_SPELLS[i]]
		entry.add_theme_color_override("font_color", COLOR_TEXT_DIM)
		entry.add_theme_font_size_override("font_size", 12)
		list.add_child(entry)


func _style_panels() -> void:
	for panel in [$SidePanel, $Console]:
		panel.add_theme_stylebox_override("panel", _flat(COLOR_PANEL, COLOR_PANEL_EDGE, 0))

	_log.add_theme_color_override("default_color", COLOR_TEXT)
	_log.add_theme_font_size_override("normal_font_size", 13)


## The battle royale bar floats over the game rather than taking panel space, so
## it is translucent — it must be readable without hiding the tiles beneath it.
func _style_top_bar() -> void:
	$TopBar.add_theme_stylebox_override("panel", _flat(COLOR_OVERLAY, Color(0, 0, 0, 0), 0))

	_alive.add_theme_color_override("font_color", COLOR_ACCENT)
	_alive.add_theme_font_size_override("font_size", 15)
	_zone.add_theme_color_override("font_color", COLOR_TEXT_DIM)
	_zone.add_theme_font_size_override("font_size", 15)

	set_alive(0)
	# The shrinking zone does not exist yet; the slot is reserved, not faked.
	set_zone("zona  —")


func _style_side_panel() -> void:
	$SidePanel/Portrait.add_theme_stylebox_override(
		"panel", _flat(COLOR_SLOT, COLOR_ACCENT.darkened(0.4), 3)
	)

	_char_name.add_theme_color_override("font_color", COLOR_TEXT)
	_char_name.add_theme_font_size_override("font_size", 16)
	_char_class.add_theme_color_override("font_color", COLOR_TEXT_DIM)
	_char_class.add_theme_font_size_override("font_size", 12)

	for title in [$SidePanel/InvTitle, $SidePanel/SpellTitle]:
		title.add_theme_color_override("font_color", COLOR_ACCENT)
		title.add_theme_font_size_override("font_size", 12)
	$SidePanel/InvTitle.text = "INVENTARIO"
	$SidePanel/SpellTitle.text = "HECHIZOS"


func _flat(fill: Color, edge: Color, radius: int = 2) -> StyleBoxFlat:
	var box := StyleBoxFlat.new()
	box.bg_color = fill
	box.border_color = edge
	box.set_border_width_all(1)
	box.set_corner_radius_all(radius)
	return box
