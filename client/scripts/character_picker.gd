extends Control
## The pre-game character picker: choose a class and a race, then play.
##
## Battle royale means nobody grinds into a build — see server/internal/world
## /balance.go's maxLevel — so this is the only character-creation step there
## is. Shown full-screen before the world/HUD/net ever connect to anything;
## main.gd frees this node once confirmed and only then dials the server.

signal confirmed(class_id: int, race_id: int)

const COLOR_BG := Color("15100a")
const COLOR_PANEL := Color("2b2118")
const COLOR_INSET := Color("15100a")
const COLOR_SLOT_EDGE := Color("4a3c2c")
const COLOR_TEXT := Color("ddd0b4")
const COLOR_TEXT_DIM := Color("8a7c63")
const COLOR_ACCENT := Color("d9b45b")

const LIST_WIDTH := 300
const LIST_HEIGHT := 380

var _class_list: ItemList
var _race_list: ItemList


func _ready() -> void:
	set_anchors_preset(Control.PRESET_FULL_RECT)

	var bg := ColorRect.new()
	bg.color = COLOR_BG
	bg.set_anchors_preset(Control.PRESET_FULL_RECT)
	bg.mouse_filter = Control.MOUSE_FILTER_IGNORE
	add_child(bg)

	# One vertical stack — title, then the two lists side by side, then the
	# play button — each centred via alignment rather than by a hand-picked
	# pixel offset. The first two attempts at this both drifted content off
	# the left edge of the window: a Container in Godot resizes itself to fit
	# its children regardless of the offset_left/offset_right asked for, so
	# anchoring "root" to a fixed width centred on the viewport let its real,
	# content-driven width silently grow past that and push everything
	# rightward off-window. Spanning the FULL viewport width removes the
	# possibility of that miscalculation entirely — there is no offset left to
	# get wrong — and ALIGNMENT_CENTER on each row centres its children within
	# that guaranteed-correct width.
	var root := VBoxContainer.new()
	root.set_anchors_preset(Control.PRESET_TOP_WIDE)
	root.offset_top = 60
	root.alignment = BoxContainer.ALIGNMENT_CENTER
	root.add_theme_constant_override("separation", 24)
	add_child(root)

	var title := Label.new()
	title.text = "ELEGÍ TU PERSONAJE"
	title.horizontal_alignment = HORIZONTAL_ALIGNMENT_CENTER
	title.add_theme_font_size_override("font_size", 28)
	title.add_theme_color_override("font_color", COLOR_ACCENT)
	root.add_child(title)

	var columns := HBoxContainer.new()
	columns.alignment = BoxContainer.ALIGNMENT_CENTER
	columns.add_theme_constant_override("separation", 32)
	root.add_child(columns)

	_class_list = _build_column(columns, "CLASE", AOData.CLASS_NAMES)
	_race_list = _build_column(columns, "RAZA", AOData.RACE_NAMES)
	# Nobody has to make a choice to start — both lists default to their first
	# entry, so JUGAR is always immediately clickable.
	_class_list.select(0)
	_race_list.select(0)

	var play_button := Button.new()
	play_button.text = "JUGAR"
	play_button.focus_mode = Control.FOCUS_NONE
	play_button.custom_minimum_size = Vector2(200, 44)
	play_button.size_flags_horizontal = Control.SIZE_SHRINK_CENTER
	play_button.add_theme_font_size_override("font_size", 16)
	play_button.add_theme_color_override("font_color", COLOR_TEXT)
	play_button.add_theme_stylebox_override("normal", _flat(COLOR_PANEL, COLOR_ACCENT))
	play_button.add_theme_stylebox_override("hover", _flat(COLOR_PANEL.lightened(0.1), COLOR_ACCENT))
	play_button.add_theme_stylebox_override("pressed", _flat(COLOR_INSET, COLOR_ACCENT))
	play_button.pressed.connect(_on_play_pressed)
	root.add_child(play_button)


func _build_column(parent: HBoxContainer, caption: String, names: Array) -> ItemList:
	var column := VBoxContainer.new()
	column.add_theme_constant_override("separation", 8)
	parent.add_child(column)

	var label := Label.new()
	label.text = caption
	label.horizontal_alignment = HORIZONTAL_ALIGNMENT_CENTER
	label.size_flags_horizontal = Control.SIZE_EXPAND_FILL
	label.add_theme_font_size_override("font_size", 14)
	label.add_theme_color_override("font_color", COLOR_TEXT_DIM)
	column.add_child(label)

	var list := ItemList.new()
	list.custom_minimum_size = Vector2(LIST_WIDTH, LIST_HEIGHT)
	# Mouse-only, same reasoning as the in-game spell list: a focused ItemList
	# eats arrow keys for its own navigation, which would fight the picker's
	# own use of arrows for nothing gained here.
	list.focus_mode = Control.FOCUS_NONE
	list.add_theme_font_size_override("font_size", 15)
	list.add_theme_color_override("font_color", COLOR_TEXT)
	list.add_theme_color_override("font_selected_color", Color.BLACK)
	list.add_theme_stylebox_override("panel", _flat(COLOR_INSET, COLOR_SLOT_EDGE))
	list.add_theme_stylebox_override("selected", _flat(COLOR_ACCENT, COLOR_ACCENT))
	list.add_theme_stylebox_override("selected_focus", _flat(COLOR_ACCENT, COLOR_ACCENT))
	for entry_name in names:
		list.add_item(entry_name)
	column.add_child(list)
	return list


func _on_play_pressed() -> void:
	var class_id: int = maxi(_class_list.get_selected_items()[0] if _class_list.get_selected_items() else 0, 0)
	var race_id: int = maxi(_race_list.get_selected_items()[0] if _race_list.get_selected_items() else 0, 0)
	confirmed.emit(class_id, race_id)


func _flat(fill: Color, edge: Color) -> StyleBoxFlat:
	var box := StyleBoxFlat.new()
	box.bg_color = fill
	box.border_color = edge
	box.set_border_width_all(1)
	box.set_corner_radius_all(2)
	return box
