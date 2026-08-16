extends Control
## Argentum's character panel, plus the readouts a battle royale needs.
##
## main.tscn defines structure and position; everything cosmetic is built here,
## so the palette and the slot counts live in one readable place instead of
## being spread across a .tscn nobody wants to hand-edit.

const COLOR_PANEL := Color("2b2118")
const COLOR_PANEL_EDGE := Color("6b5330")
const COLOR_INSET := Color("15100a")
const COLOR_SLOT := Color("1d1710")
const COLOR_SLOT_EDGE := Color("4a3c2c")
const COLOR_TEXT := Color("ddd0b4")
const COLOR_TEXT_DIM := Color("8a7c63")
const COLOR_ACCENT := Color("d9b45b")
const COLOR_OVERLAY := Color(0.07, 0.06, 0.04, 0.72)

## Argentum's own bar colours, in its own order.
const COLOR_EXP := Color("8c2f2f")
const COLOR_HP := Color("2f8c3a")
const COLOR_MANA := Color("2f5fa8")
const COLOR_STAMINA := Color("c2a33a")
const COLOR_HUNGER := Color("a86a2f")
const COLOR_THIRST := Color("2f97a8")

const INVENTORY_SLOTS := 30
const EQUIP_SLOTS := 5
const SLOT_SIZE := 36

## Raised when the player picks a spell and presses LANZAR. The HUD does not
## know what a target is; main.gd takes it from here into targeting mode.
signal cast_requested(spell_id: int)

## The side panel buttons Argentum shows. None are wired yet; they are here so
## the panel can be judged at its real density.
const PANEL_BUTTONS := [
	"Mapa", "Amigos", "Grupo", "Retos", "Estadísticas", "Opciones", "Clanes", "Quests"
]

@onready var _alive: Label = $TopBar/Alive
@onready var _zone: Label = $TopBar/Zone
@onready var _log: RichTextLabel = $Console/Log
@onready var _char_name: Label = $SidePanel/CharName
@onready var _level_bar: ProgressBar = $SidePanel/LevelBar
@onready var _level_text: Label = $SidePanel/LevelText
@onready var _tab_inv: Button = $SidePanel/TabInv
@onready var _tab_spells: Button = $SidePanel/TabSpells
@onready var _inv_grid: GridContainer = $SidePanel/InvGrid
@onready var _spell_list: ItemList = $SidePanel/SpellList
@onready var _spell_info: Label = $SidePanel/SpellInfo
@onready var _spell_buttons: HBoxContainer = $SidePanel/SpellButtons

var _bars: Dictionary = {}
var _sprites := AOSprites.new()
var _data := AOData.new()
var _spell_ids: Array[int] = []
var _cast_button: Button


func _ready() -> void:
	_sprites.load_bundle()
	_data.load_data()

	_style_panels()
	_style_top_bar()
	_style_character_header()
	_build_tabs()
	_build_equipment()
	_build_inventory()
	_build_spells()
	_build_bars()
	_build_buttons()
	_show_inventory(true)


## set_loadout fills the bag and the spell list from what the server says the
## player is carrying and knows.
func set_loadout(loadout: Dictionary) -> void:
	_fill_inventory(loadout.get("inv", []))
	_fill_spells(loadout.get("spells", []))


func _fill_inventory(slots: Array) -> void:
	for child in _inv_grid.get_children():
		_clear_slot(child)
	for child in $SidePanel/EquipRow.get_children():
		_clear_slot(child)

	for entry in slots:
		var index := int(entry.get("s", -1))
		var item := _data.item(int(entry.get("i", 0)))
		if item.is_empty():
			continue

		# Equipped things go in the row above the bag, the way Argentum splits
		# what you are wearing from what you are merely carrying.
		var parent: Node = $SidePanel/EquipRow if bool(entry.get("e", false)) else _inv_grid
		var slot := _next_free_slot(parent) if bool(entry.get("e", false)) else _slot_at(index)
		if slot == null:
			continue
		_fill_slot(slot, item, int(entry.get("n", 1)))


func _slot_at(index: int) -> Panel:
	if index < 0 or index >= _inv_grid.get_child_count():
		return null
	return _inv_grid.get_child(index) as Panel


func _next_free_slot(parent: Node) -> Panel:
	for child in parent.get_children():
		if child.get_child_count() == 0:
			return child as Panel
	return null


func _clear_slot(slot: Node) -> void:
	for child in slot.get_children():
		child.queue_free()


func _fill_slot(slot: Panel, item: Dictionary, amount: int) -> void:
	var rect: Rect2 = _sprites.grh_rect(int(item.get("grh", 0)), 0.0)
	if rect.size.x > 0.0:
		var atlas := AtlasTexture.new()
		atlas.atlas = _sprites.atlas
		atlas.region = rect

		var icon := TextureRect.new()
		icon.texture = atlas
		icon.set_anchors_preset(Control.PRESET_FULL_RECT)
		icon.expand_mode = TextureRect.EXPAND_IGNORE_SIZE
		icon.stretch_mode = TextureRect.STRETCH_KEEP_ASPECT_CENTERED
		icon.mouse_filter = Control.MOUSE_FILTER_IGNORE
		slot.add_child(icon)

	slot.tooltip_text = str(item.get("name", ""))

	if amount > 1:
		var label := Label.new()
		label.text = str(amount)
		label.set_anchors_preset(Control.PRESET_FULL_RECT)
		label.horizontal_alignment = HORIZONTAL_ALIGNMENT_RIGHT
		label.vertical_alignment = VERTICAL_ALIGNMENT_BOTTOM
		label.add_theme_font_size_override("font_size", 10)
		label.add_theme_color_override("font_color", COLOR_TEXT)
		label.add_theme_color_override("font_shadow_color", Color(0, 0, 0, 0.9))
		label.add_theme_constant_override("shadow_offset_x", 1)
		label.add_theme_constant_override("shadow_offset_y", 1)
		label.mouse_filter = Control.MOUSE_FILTER_IGNORE
		slot.add_child(label)


func _fill_spells(spells: Array) -> void:
	_spell_list.clear()
	_spell_ids.clear()
	for id in spells:
		var spell_id := int(id)
		var spell := _data.spell(spell_id)
		if spell.is_empty():
			continue
		_spell_ids.append(spell_id)
		_spell_list.add_item(str(spell.get("name", "hechizo %d" % spell_id)))
	_on_spell_selected(-1)


func set_character(player_name: String) -> void:
	_char_name.text = player_name


func set_vitals(vitals: Dictionary) -> void:
	_apply_bar("hp", int(vitals.get("hp", 0)), int(vitals.get("maxHp", 1)))
	_apply_bar("mana", int(vitals.get("mana", 0)), int(vitals.get("maxMana", 1)))
	_apply_bar("sta", int(vitals.get("sta", 0)), int(vitals.get("maxSta", 1)))
	_apply_bar("hun", int(vitals.get("hun", 0)), int(vitals.get("maxHun", 1)))
	_apply_bar("thi", int(vitals.get("thi", 0)), int(vitals.get("maxThi", 1)))

	var level := int(vitals.get("lvl", 1))
	var exp := int(vitals.get("exp", 0))
	var max_exp: int = maxi(int(vitals.get("maxExp", 1)), 1)
	_level_bar.max_value = max_exp
	_level_bar.value = exp
	_level_text.text = "Nivel %d    %d / %d" % [level, exp, max_exp]


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
	entry["label"].text = "%s  %d/%d" % [entry["caption"], value, maximum]


## Inventory and spells share one region of the panel, exactly as in Argentum:
## the tabs swap which of the two is visible rather than shrinking both.
func _build_tabs() -> void:
	_tab_inv.text = "INVENTARIO"
	_tab_spells.text = "HECHIZOS"
	for tab: Button in [_tab_inv, _tab_spells]:
		tab.add_theme_font_size_override("font_size", 12)
		tab.add_theme_color_override("font_color", COLOR_TEXT_DIM)
		tab.add_theme_color_override("font_hover_color", COLOR_TEXT)
	_tab_inv.pressed.connect(_show_inventory.bind(true))
	_tab_spells.pressed.connect(_show_inventory.bind(false))


func _show_inventory(inventory: bool) -> void:
	_inv_grid.visible = inventory
	$SidePanel/EquipRow.visible = inventory
	_spell_list.visible = not inventory
	_spell_info.visible = not inventory
	_spell_buttons.visible = not inventory

	# The active tab is the lit one, so the panel reads at a glance.
	_tab_inv.add_theme_stylebox_override(
		"normal", _flat(COLOR_SLOT if inventory else COLOR_INSET, COLOR_ACCENT if inventory else COLOR_SLOT_EDGE)
	)
	_tab_spells.add_theme_stylebox_override(
		"normal", _flat(COLOR_INSET if inventory else COLOR_SLOT, COLOR_SLOT_EDGE if inventory else COLOR_ACCENT)
	)
	_tab_inv.add_theme_color_override("font_color", COLOR_ACCENT if inventory else COLOR_TEXT_DIM)
	_tab_spells.add_theme_color_override("font_color", COLOR_TEXT_DIM if inventory else COLOR_ACCENT)


## The equipped row sits apart from the bag in Argentum: weapon, shield, helmet,
## armour, ring.
func _build_equipment() -> void:
	var row: HBoxContainer = $SidePanel/EquipRow
	row.add_theme_constant_override("separation", 6)
	for i in EQUIP_SLOTS:
		row.add_child(_make_slot(SLOT_SIZE + 2, COLOR_ACCENT.darkened(0.45)))


func _build_inventory() -> void:
	_inv_grid.add_theme_constant_override("h_separation", 4)
	_inv_grid.add_theme_constant_override("v_separation", 4)
	for i in INVENTORY_SLOTS:
		_inv_grid.add_child(_make_slot(SLOT_SIZE, COLOR_SLOT_EDGE))


func _make_slot(side: int, edge: Color) -> Panel:
	var slot := Panel.new()
	slot.custom_minimum_size = Vector2(side, side)
	slot.mouse_filter = Control.MOUSE_FILTER_IGNORE
	slot.add_theme_stylebox_override("panel", _flat(COLOR_SLOT, edge))
	return slot


## Spells follow Argentum's interaction: pick one from the list, press LANZAR,
## then click a target in the world. The list is a real selection rather than a
## row of labels, because selecting is half of how casting works.
func _build_spells() -> void:
	_spell_list.add_theme_font_size_override("font_size", 12)
	_spell_list.add_theme_color_override("font_color", COLOR_TEXT)
	_spell_list.add_theme_color_override("font_selected_color", Color.BLACK)
	_spell_list.add_theme_stylebox_override("panel", _flat(COLOR_INSET, COLOR_SLOT_EDGE))
	_spell_list.add_theme_stylebox_override("selected", _flat(COLOR_ACCENT, COLOR_ACCENT))
	_spell_list.add_theme_stylebox_override("selected_focus", _flat(COLOR_ACCENT, COLOR_ACCENT))
	_spell_list.item_selected.connect(_on_spell_selected)
	_spell_list.item_activated.connect(_on_spell_activated)

	_spell_info.add_theme_color_override("font_color", COLOR_TEXT_DIM)
	_spell_info.add_theme_font_size_override("font_size", 11)

	_spell_buttons.add_theme_constant_override("separation", 6)
	_cast_button = _make_panel_button("LANZAR")
	_cast_button.pressed.connect(_on_cast_pressed)
	_spell_buttons.add_child(_cast_button)

	var info := _make_panel_button("INFO")
	info.pressed.connect(_on_info_pressed)
	_spell_buttons.add_child(info)


func spell_name(id: int) -> String:
	var spell := _data.spell(id)
	return "hechizo %d" % id if spell.is_empty() else str(spell.get("name", ""))


func _selected_spell_id() -> int:
	var selected := _spell_list.get_selected_items()
	if selected.is_empty():
		return 0
	return _spell_ids[selected[0]]


func _on_spell_selected(_index: int) -> void:
	var id := _selected_spell_id()
	if id == 0:
		_spell_info.text = "elegí un hechizo"
		_cast_button.disabled = true
		return
	_spell_info.text = _data.spell_summary(id)
	_cast_button.disabled = false


## Double clicking a spell casts it, which is the shortcut every AO player ends
## up using instead of reaching for the button.
func _on_spell_activated(_index: int) -> void:
	_on_cast_pressed()


func _on_cast_pressed() -> void:
	var id := _selected_spell_id()
	if id > 0:
		cast_requested.emit(id)


func _on_info_pressed() -> void:
	var id := _selected_spell_id()
	if id == 0:
		return
	var spell := _data.spell(id)
	log_line(str(spell.get("name", "")), COLOR_ACCENT)
	var words := str(spell.get("words", ""))
	if words != "":
		log_line("  \"%s\"" % words, COLOR_TEXT_DIM)
	var desc := str(spell.get("desc", ""))
	if desc != "":
		log_line("  " + desc, COLOR_TEXT_DIM)


func _make_panel_button(caption: String) -> Button:
	var button := Button.new()
	button.text = caption
	button.custom_minimum_size = Vector2(118, 26)
	button.add_theme_font_size_override("font_size", 11)
	button.add_theme_color_override("font_color", COLOR_TEXT)
	button.add_theme_color_override("font_disabled_color", COLOR_TEXT_DIM.darkened(0.3))
	button.add_theme_stylebox_override("normal", _flat(COLOR_SLOT, COLOR_SLOT_EDGE))
	button.add_theme_stylebox_override("hover", _flat(COLOR_SLOT.lightened(0.1), COLOR_ACCENT))
	button.add_theme_stylebox_override("pressed", _flat(COLOR_INSET, COLOR_ACCENT))
	button.add_theme_stylebox_override("disabled", _flat(COLOR_INSET, COLOR_SLOT_EDGE))
	return button


func _build_bars() -> void:
	var container: VBoxContainer = $SidePanel/Bars
	container.add_theme_constant_override("separation", 7)
	_add_bar(container, "hp", "SALUD", COLOR_HP)
	_add_bar(container, "mana", "MANÁ", COLOR_MANA)
	_add_bar(container, "sta", "ENERGÍA", COLOR_STAMINA)
	_add_bar(container, "hun", "HAMBRE", COLOR_HUNGER)
	_add_bar(container, "thi", "SED", COLOR_THIRST)


func _add_bar(parent: VBoxContainer, key: String, caption: String, color: Color) -> void:
	var bar := ProgressBar.new()
	bar.custom_minimum_size = Vector2(0, 20)
	bar.show_percentage = false
	bar.add_theme_stylebox_override("background", _flat(COLOR_INSET, COLOR_SLOT_EDGE))
	bar.add_theme_stylebox_override("fill", _flat(color, color.darkened(0.4)))

	# The caption rides on top of the bar the way Argentum draws it, rather than
	# above it, which keeps five bars inside the panel without crowding.
	var label := Label.new()
	label.set_anchors_preset(Control.PRESET_FULL_RECT)
	label.horizontal_alignment = HORIZONTAL_ALIGNMENT_CENTER
	label.vertical_alignment = VERTICAL_ALIGNMENT_CENTER
	label.add_theme_color_override("font_color", COLOR_TEXT)
	label.add_theme_color_override("font_shadow_color", Color(0, 0, 0, 0.9))
	label.add_theme_constant_override("shadow_offset_x", 1)
	label.add_theme_constant_override("shadow_offset_y", 1)
	label.add_theme_font_size_override("font_size", 11)
	label.mouse_filter = Control.MOUSE_FILTER_IGNORE
	bar.add_child(label)

	parent.add_child(bar)
	_bars[key] = {"bar": bar, "label": label, "caption": caption}


func _build_buttons() -> void:
	var grid: GridContainer = $SidePanel/Buttons
	grid.add_theme_constant_override("h_separation", 6)
	grid.add_theme_constant_override("v_separation", 6)
	for caption in PANEL_BUTTONS:
		var button := Button.new()
		button.text = caption
		button.custom_minimum_size = Vector2(118, 26)
		button.disabled = true
		button.add_theme_font_size_override("font_size", 11)
		button.add_theme_color_override("font_disabled_color", COLOR_TEXT_DIM.darkened(0.2))
		button.add_theme_stylebox_override("disabled", _flat(COLOR_SLOT, COLOR_SLOT_EDGE))
		grid.add_child(button)


func _style_panels() -> void:
	for panel in [$SidePanel, $Console, $MinimapFrame]:
		panel.add_theme_stylebox_override("panel", _flat(COLOR_PANEL, COLOR_PANEL_EDGE, 0, 2))

	_log.add_theme_color_override("default_color", COLOR_TEXT)
	_log.add_theme_font_size_override("normal_font_size", 13)


## The battle royale bar floats over the game rather than taking panel space, so
## it is translucent — it must be readable without hiding the tiles beneath it.
func _style_top_bar() -> void:
	$TopBar.add_theme_stylebox_override("panel", _flat(COLOR_OVERLAY, Color(0, 0, 0, 0), 0, 0))

	_alive.add_theme_color_override("font_color", COLOR_ACCENT)
	_alive.add_theme_font_size_override("font_size", 15)
	_zone.add_theme_color_override("font_color", COLOR_TEXT_DIM)
	_zone.add_theme_font_size_override("font_size", 15)

	set_alive(0)
	# The shrinking zone does not exist yet; the slot is reserved, not faked.
	set_zone("zona  —")


func _style_character_header() -> void:
	_char_name.add_theme_color_override("font_color", COLOR_ACCENT)
	_char_name.add_theme_font_size_override("font_size", 17)

	_level_bar.show_percentage = false
	_level_bar.add_theme_stylebox_override("background", _flat(COLOR_INSET, COLOR_SLOT_EDGE))
	_level_bar.add_theme_stylebox_override("fill", _flat(COLOR_EXP, COLOR_EXP.darkened(0.4)))

	_level_text.add_theme_color_override("font_color", COLOR_TEXT)
	_level_text.add_theme_color_override("font_shadow_color", Color(0, 0, 0, 0.9))
	_level_text.add_theme_constant_override("shadow_offset_x", 1)
	_level_text.add_theme_constant_override("shadow_offset_y", 1)
	_level_text.add_theme_font_size_override("font_size", 11)


func _flat(fill: Color, edge: Color, radius: int = 2, border: int = 1) -> StyleBoxFlat:
	var box := StyleBoxFlat.new()
	box.bg_color = fill
	box.border_color = edge
	box.set_border_width_all(border)
	box.set_corner_radius_all(radius)
	return box
