extends Control
## Argentum's character panel, plus the readouts a battle royale needs.
##
## main.tscn defines structure and position; everything cosmetic is built here,
## so the palette and the slot counts live in one readable place instead of
## being spread across a .tscn nobody wants to hand-edit.

const COLOR_INSET := Color("15100a")
## Pixel-sampled from the real AO Frost art (VentanaPrincipal.jpg): the flat
## grey of an equip-slot plate and the copper trim around it — see
## PANEL_BORDER_PATH's comment for where that file lives and how it was
## measured. The big panels wear the real texture directly; slots, buttons
## and bars are too small for that same texture to read at their scale (see
## _textured_box), so they get its colours instead of its pixels.
const COLOR_SLOT := Color("1a1a1a")
const COLOR_SLOT_EDGE := Color("4e3d2d")
const COLOR_TEXT := Color("ddd0b4")
const COLOR_TEXT_DIM := Color("8a7c63")
const COLOR_ACCENT := Color("d9b45b")
const COLOR_OVERLAY := Color(0.07, 0.06, 0.04, 0.72)
# Argentum marks a selected inventory item with a red tile under the icon;
# this is that same "you have this selected" language applied to a border.
const COLOR_SELECTED := Color(0.90, 0.35, 0.30)

## Argentum's own bar colours, in its own order.
const COLOR_EXP := Color("8c2f2f")
const COLOR_HP := Color("2f8c3a")
const COLOR_MANA := Color("2f5fa8")
const COLOR_STAMINA := Color("c2a33a")
const COLOR_HUNGER := Color("a86a2f")
const COLOR_THIRST := Color("2f97a8")

const INVENTORY_SLOTS := 30
## Must match the server's world.SpellSlots. The Welcome carries the server's
## own figure and set_spell_slots checks it, so a mismatch is reported rather
## than silently truncating someone's book.
const SPELL_SLOTS := 30
## How many spell rows fit the artwork's hole at a legible size. The rest are
## reached by scrolling — 262px of panel over 30 rows would be 8px a row.
const SPELL_ROWS_VISIBLE := 15.0
## The black area the artwork leaves for whichever tab is showing. All three
## tabs share it — inventory, spell book and stats — so it is measured once
## here and the slot sizes are derived from it rather than hardcoded.
const PANEL_HOLE := Vector2(354, 262)

## Which item types are worn rather than consumed. Only the right-click menu
## reads this, to word itself "Equipar/Quitar" instead of "Usar" — the server
## is still the one that decides what actually happens to a slot.
const EQUIPMENT_TYPES := [
	AOData.TYPE_WEAPON, AOData.TYPE_SHIELD, AOData.TYPE_ARMOR, AOData.TYPE_HELMET, AOData.TYPE_RING,
]

## Raised when the player picks a spell and presses LANZAR. The HUD does not
## know what a target is; main.gd takes it from here into targeting mode.
signal cast_requested(spell_id: int)

## Raised when the player clicks a bag or equipment slot. server_slot is the
## server's own InventorySlot.Slot number for whatever is displayed there —
## not the visual grid position, which does not necessarily match once
## equipped items and a partially-filled bag are laid out.
signal item_used(server_slot: int)

## Raised when the player drags one bag slot onto another. Equip-row slots
## never raise this — see InventorySlot's `draggable` note.
signal swap_requested(from_slot: int, to_slot: int)

## Raised when the player drags one spell slot onto another. Separate from
## swap_requested because the bag and the spell book are separate server state
## and a bag index is not a spell index — the server has two handlers, and one
## signal carrying both would have to be told apart somewhere anyway.
signal spell_swap_requested(from_slot: int, to_slot: int)

## Raised by the right-click context menu's "Tirar" — the same drop Argentum's
## own mnuTirar sends, just reached without needing to stand and press a key.
signal drop_requested(server_slot: int)

## The side panel buttons Argentum shows. None are wired yet; they are here so
## the panel can be judged at its real density. Trimmed to what a single BR
## match without persistence could ever actually use — Amigos/Grupo/Clanes/
## Quests all presuppose state that outlives a match (a friends list, a
## guild, a quest log), which the genre-fork decision already ruled out.
## Estadísticas moved to its own tab (see TabStats) rather than staying a dead
## button here — a whole attribute readout doesn't fit a single click.
const PANEL_BUTTONS := [
	"Mapa", "Retos", "Opciones"
]

## Argentum's own attribute order, and the Vitals wire key each one rides in
## on — see protocol.Vitals' Fuerza/Agilidad/Inteligencia/Carisma/Constitucion.
const ATTRIBUTE_ROWS := [
	["str", "Fuerza"], ["agi", "Agilidad"], ["int", "Inteligencia"],
	["cha", "Carisma"], ["con", "Constitución"],
]

## Where each vitals bar's coloured fill sits on the background image, and
## which Vitals wire keys drive it. Measured off the source PNG by detecting
## its saturated regions, then scaled by 525/1426 — see main.tscn's header.
## The artwork already draws the trough and its bone frame; only the fill is
## a live control, so these rects are the *fill* area, not the whole widget.
const BAR_X := 59
const BAR_W := 169
const BAR_H := 13
## Only the two bars that decide a fight.
##
## Energía, Hambre and Sed are gone from the panel, not from the protocol —
## Vitals still carries all five and the server still owns them. They came out
## because none of the three tells you anything: hunger and thirst have never
## drained, and energy is deliberately never spent (see the note in the
## server's cast). Three bars pinned at 100/100 are furniture, and they were
## crowding the two numbers a player actually reads mid-fight.
##
## Their y offsets stay recorded here so putting one back is uncommenting a
## line rather than re-measuring the artwork: sta 813, hun 849, thi 884.
const BAR_ROWS := [
	["hp",   742, "SALUD", Color("c8383a")],
	["mana", 778, "MANÁ",  Color("4a63b0")],
]

## The six action-button plates the artwork draws down the right side. We only
## have three things worth putting there, so the rest stay visibly disabled
## rather than being hidden — the plates are painted into the background and
## cannot be removed anyway.
const ACTION_X := 326
const ACTION_W := 162
const ACTION_H := 25
const ACTION_YS := [716, 748, 782, 813, 847, 878]

@onready var _alive: Label = $TopBar/Alive
@onready var _zone: Label = $TopBar/Zone
@onready var _log: RichTextLabel = $Console/Log
@onready var _char_name: Label = $SidePanel/CharName
@onready var _char_class: Label = $SidePanel/CharClass
@onready var _tab_inv: Button = $SidePanel/TabInv
@onready var _tab_spells: Button = $SidePanel/TabSpells
@onready var _tab_stats: Button = $SidePanel/TabStats
@onready var _inv_grid: GridContainer = $SidePanel/InvGrid
@onready var _spell_scroll: ScrollContainer = $SidePanel/SpellList
@onready var _spell_grid: GridContainer = $SidePanel/SpellList/SpellGrid
@onready var _spell_info: Label = $SidePanel/SpellInfo
@onready var _stats_panel: VBoxContainer = $SidePanel/StatsPanel
@onready var _potion_red: Label = $SidePanel/PotionRed
@onready var _potion_blue: Label = $SidePanel/PotionBlue
@onready var _alive_box: Label = $SidePanel/AliveBox
@onready var _coords: Label = $SidePanel/Coords

var _bars: Dictionary = {}
var _attribute_labels: Dictionary = {}
var _sprites := AOSprites.new()
var _data := AOData.new()
## The spell book as the server last sent it: SPELL_SLOTS entries, 0 for empty.
## Index into this is the slot number both sides agree on, which is what a
## reorder drag names.
var _spell_ids: Array[int] = []
var _cast_button: Button
var _info_button: Button
## The server slot number currently selected in the inventory, or -1 for none.
## Single click sets this; double click acts on it. See _on_slot_gui_input.
var _selected_slot := -1

## The spell slot the player last picked, remembered across refills.
##
## The server resends the whole loadout on every inventory change — picking up
## a potion mid-fight is enough — and the spell panel is rebuilt from it each
## time. Holding the selection in the rebuilt widgets meant that every pickup
## silently deselected your spell, so the next LANZAR did nothing and you had
## to go find it in the list again. The player's choice is the player's, not
## the widget's, so it lives out here and survives the rebuild.
var _selected_spell_slot := -1

## Argentum's own right-click menu on an inventory item (mnuObj in the
## source): Equipar/Quitar or Usar depending on what the slot holds, plus
## Tirar always. One shared PopupMenu rather than one per slot — it only ever
## targets whichever slot was last right-clicked, tracked here.
var _context_menu: PopupMenu
var _context_slot := -1


func _ready() -> void:
	_sprites.load_bundle()
	_data.load_data()

	_style_panels()
	_style_top_bar()
	_style_character_header()
	_build_tabs()
	_build_inventory()
	_build_spells()
	_build_stats()
	_build_bars()
	_build_buttons()
	_build_context_menu()
	_show_tab(&"inv")


## set_loadout fills the bag and the spell list from what the server says the
## player is carrying and knows.
func set_loadout(loadout: Dictionary) -> void:
	_fill_inventory(loadout.get("inv", []))
	_fill_spells(loadout.get("spells", []))
	_update_potion_counts(loadout.get("inv", []))


## The artwork paints two potion quick-slots with a count box beside each.
## Argentum's own red/blue potions are the obvious thing to put there, so the
## boxes show how many of each the bag actually holds rather than being
## decorative — the icons beside them are part of the background image.
func _update_potion_counts(slots: Array) -> void:
	var red := 0
	var blue := 0
	for entry in slots:
		var item := _data.item(int(entry.get("i", 0)))
		if item.is_empty() or int(item.get("type", -1)) != AOData.TYPE_POTION:
			continue
		match int(item.get("potionType", 0)):
			AOData.POTION_HEALTH: red += int(entry.get("n", 0))
			AOData.POTION_MANA:   blue += int(entry.get("n", 0))
	_potion_red.text = str(red)
	_potion_blue.text = str(blue)


func _fill_inventory(slots: Array) -> void:
	for child in _inv_grid.get_children():
		_clear_slot(child)

	for entry in slots:
		var server_slot := int(entry.get("s", -1))
		var item := _data.item(int(entry.get("i", 0)))
		if item.is_empty():
			continue

		# Everything lives in the bag now, worn or not, at the server's own slot
		# number. Equipped items are marked with a badge in place (see
		# _fill_slot) instead of being moved into a separate row.
		#
		# That row is gone on purpose. It was a second home for an item, which
		# meant equipping something *moved* it on screen, and keeping the two
		# views agreed on who owned which slot was the source of the reordering
		# glitch that the fixed-column-per-type table was written to paper over.
		# One item, one place, and the whole class of problem disappears along
		# with the table.
		var equipped: bool = entry.get("e", false)
		var slot := _slot_at(server_slot)
		if slot == null:
			continue
		_fill_slot(slot, item, int(entry.get("n", 1)), server_slot, equipped)


func _slot_at(index: int) -> Panel:
	if index < 0 or index >= _inv_grid.get_child_count():
		return null
	return _inv_grid.get_child(index) as Panel


func _clear_slot(slot: Control) -> void:
	for child in slot.get_children():
		# remove_child before queue_free, not queue_free alone. queue_free is
		# deferred to the end of the frame, and _fill_inventory clears every
		# slot and refills them all within the same call — so with a bare
		# queue_free the old icon is still a child when the new one is added,
		# and the slot draws both stacked. That is the "I drink a green potion
		# and a blue one appears" bug: nothing was mixed up, the previous
		# icon simply had not been detached yet.
		slot.remove_child(child)
		child.queue_free()
	# -1 means "nothing here": a click on an empty slot has no server slot
	# number to report, and _on_slot_gui_input relies on this to ignore it.
	slot.set_meta(&"server_slot", -1)
	slot.set_meta(&"item_type", -1)
	slot.set_meta(&"equipped", false)
	_restyle_slot(slot)


func _fill_slot(slot: Panel, item: Dictionary, amount: int, server_slot: int, equipped: bool) -> void:
	slot.set_meta(&"server_slot", server_slot)
	# What the right-click context menu needs to word itself — see
	# _open_context_menu. Cheaper to remember here than to look the item back
	# up by id when the menu is actually opened.
	slot.set_meta(&"item_type", int(item.get("type", -1)))
	slot.set_meta(&"equipped", equipped)
	_restyle_slot(slot)
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

	if equipped:
		# A small E in the corner is the whole "you are wearing this" signal now
		# that there is no separate row to move it to. Bottom-right, where the
		# count would go — no collision in practice, since gear does not stack
		# and the count only draws above 1.
		var badge := Label.new()
		badge.text = "E"
		badge.set_anchors_preset(Control.PRESET_FULL_RECT)
		badge.horizontal_alignment = HORIZONTAL_ALIGNMENT_RIGHT
		badge.vertical_alignment = VERTICAL_ALIGNMENT_BOTTOM
		badge.add_theme_font_size_override("font_size", 10)
		badge.add_theme_color_override("font_color", COLOR_ACCENT)
		badge.add_theme_color_override("font_shadow_color", Color(0, 0, 0, 0.95))
		badge.add_theme_constant_override("shadow_offset_x", 1)
		badge.add_theme_constant_override("shadow_offset_y", 1)
		badge.mouse_filter = Control.MOUSE_FILTER_IGNORE
		slot.add_child(badge)

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


## The book arrives as a fixed SPELL_SLOTS-long list with 0 for an empty slot,
## so slot number is just the index — no lookup, and a drag can name an empty
## slot as its destination the same way the bag grid can.
##
## Unlike the old ItemList this never clears the selection: see
## _selected_spell_slot for why a refill wiping it was a real bug rather than
## tidiness.
func _fill_spells(spells: Array) -> void:
	_spell_ids.clear()
	for i in SPELL_SLOTS:
		var spell_id := 0 if i >= spells.size() else int(spells[i])
		_spell_ids.append(spell_id)
		_fill_spell_slot(_spell_grid.get_child(i) as Panel, i, spell_id)
	_restyle_spell_slots()
	_refresh_spell_info()


func _fill_spell_slot(slot: Panel, index: int, spell_id: int) -> void:
	var label := slot.get_child(0) as Label
	# server_slot is what InventorySlot._get_drag_data reads to name the source
	# of a drag, and it refuses to start one when the value is negative — which
	# is how an empty slot ends up being a drop target with nothing to pick up.
	# For the book the value is just the slot index, since the book has no
	# separate server-side numbering the way bag slots do.
	if spell_id == 0:
		label.text = ""
		slot.tooltip_text = ""
		slot.set_meta(&"spell_id", 0)
		slot.set_meta(&"server_slot", -1)
		return
	label.text = spell_name(spell_id)
	slot.tooltip_text = _data.spell_summary(spell_id)
	slot.set_meta(&"spell_id", spell_id)
	slot.set_meta(&"server_slot", index)


func set_character(player_name: String) -> void:
	_char_name.text = player_name
	# The plate the artwork paints for the name is 312px wide and the label is
	# pinned to it, so a long name used to spill past the frame. Shrinking the
	# font to fit keeps it inside without truncating anyone's name — a name is
	# the one string on this panel that should never come back as "wachinci…".
	_fit_label(_char_name, 20, 11)


## class_id/race_id are the same indices the character picker sent the server
## at join — see AOData.CLASS_NAMES/RACE_NAMES for why that index doubles as
## the name lookup without another round trip to the server.
func set_identity(class_id: int, race_id: int) -> void:
	var class_name_: String = _data.CLASS_NAMES[class_id] if class_id >= 0 and class_id < _data.CLASS_NAMES.size() else "?"
	var race_name: String = _data.RACE_NAMES[race_id] if race_id >= 0 and race_id < _data.RACE_NAMES.size() else "?"
	_char_class.text = "%s · %s" % [class_name_, race_name]


func set_vitals(vitals: Dictionary) -> void:
	_apply_bar("hp", int(vitals.get("hp", 0)), int(vitals.get("maxHp", 1)))
	_apply_bar("mana", int(vitals.get("mana", 0)), int(vitals.get("maxMana", 1)))
	# sta/hun/thi still arrive every tick and are still real server state; the
	# panel just no longer draws them. See BAR_ROWS.

	for row in ATTRIBUTE_ROWS:
		var key: String = row[0]
		if _attribute_labels.has(key):
			_attribute_labels[key].text = "%s: %d" % [row[1], int(vitals.get(key, 0))]


## The panel artwork has a wide inset box where a gold counter used to live.
## There is no currency in a battle royale, so it carries the one number that
## actually matters instead: how many players are still alive.
func set_alive(count: int) -> void:
	_alive.text = "◈  VIVOS  %d" % count
	_alive_box.text = "VIVOS  %d" % count


func set_zone(text: String) -> void:
	_zone.text = text
	_coords.text = text


func log_line(text: String, color: Color = COLOR_TEXT) -> void:
	_log.append_text("[color=#%s]%s[/color]\n" % [color.to_html(false), text])


## Steps a label's font size down from max_size until its text fits the width
## it was given, stopping at min_size. Used for anything player-supplied, where
## the string's length is not ours to choose.
func _fit_label(label: Label, max_size: int, min_size: int) -> void:
	var font := label.get_theme_font("font")
	var width := label.size.x
	if width <= 0.0:
		width = label.offset_right - label.offset_left
	for size in range(max_size, min_size - 1, -1):
		if font.get_string_size(label.text, HORIZONTAL_ALIGNMENT_LEFT, -1, size).x <= width:
			label.add_theme_font_size_override("font_size", size)
			return
	label.add_theme_font_size_override("font_size", min_size)


func _apply_bar(key: String, value: int, maximum: int) -> void:
	# A vital whose bar was taken out of BAR_ROWS is simply not drawn, rather
	# than crashing whoever still reports it.
	if not _bars.has(key):
		return
	var entry: Dictionary = _bars[key]
	var bar: ProgressBar = entry["bar"]
	bar.max_value = maxi(maximum, 1)
	bar.value = value
	entry["label"].text = "%s  %d/%d" % [entry["caption"], value, maximum]


## Inventory, spells and stats share one region of the panel, exactly as in
## Argentum: the tabs swap which one is visible rather than shrinking all three.
func _build_tabs() -> void:
	_tab_inv.text = "INVENTARIO"
	_tab_spells.text = "HECHIZOS"
	_tab_stats.text = "STATS"
	for tab: Button in [_tab_inv, _tab_spells, _tab_stats]:
		tab.focus_mode = Control.FOCUS_NONE
		tab.add_theme_font_size_override("font_size", 12)
		tab.add_theme_color_override("font_color", COLOR_TEXT_DIM)
		tab.add_theme_color_override("font_hover_color", COLOR_TEXT)
	_tab_inv.pressed.connect(_show_tab.bind(&"inv"))
	_tab_spells.pressed.connect(_show_tab.bind(&"spells"))
	_tab_stats.pressed.connect(_show_tab.bind(&"stats"))


func _show_tab(tab: StringName) -> void:
	var showing_inv := tab == &"inv"
	var showing_spells := tab == &"spells"
	var showing_stats := tab == &"stats"

	_inv_grid.visible = showing_inv
	_spell_scroll.visible = showing_spells
	_spell_info.visible = showing_spells
	_stats_panel.visible = showing_stats
	# LANZAR/INFO are painted into the background image, so they cannot be
	# hidden with the spell list — they just go dead outside the spell tab.
	_cast_button.disabled = not showing_spells
	_info_button.disabled = not showing_spells

	# The active tab is the lit one, so the panel reads at a glance.
	_style_tab(_tab_inv, showing_inv)
	_style_tab(_tab_spells, showing_spells)
	_style_tab(_tab_stats, showing_stats)


func _style_tab(tab: Button, active: bool) -> void:
	tab.add_theme_stylebox_override(
		"normal", _flat(COLOR_SLOT if active else COLOR_INSET, COLOR_ACCENT if active else COLOR_SLOT_EDGE)
	)
	tab.add_theme_color_override("font_color", COLOR_ACCENT if active else COLOR_TEXT_DIM)


func _build_inventory() -> void:
	const SEPARATION := 4
	_inv_grid.add_theme_constant_override("h_separation", SEPARATION)
	_inv_grid.add_theme_constant_override("v_separation", SEPARATION)

	# Slots are sized to fill the hole the artwork leaves rather than being a
	# fixed 36px that left a visible margin of dead black inside the frame.
	# Derived from the container so the two cannot drift: change the rect in
	# main.tscn and the slots follow.
	var columns := _inv_grid.columns
	var rows := int(ceil(float(INVENTORY_SLOTS) / columns))
	var side := int(
		min(
			(PANEL_HOLE.x - (columns - 1) * SEPARATION) / float(columns),
			(PANEL_HOLE.y - (rows - 1) * SEPARATION) / float(rows),
		)
	)

	for i in INVENTORY_SLOTS:
		var slot := _make_slot(side, COLOR_SLOT_EDGE)
		# grid_index is this Panel's own fixed position (0..29), matching the
		# server's own Slot numbering one-to-one — unlike server_slot, it never
		# changes, so a drag dropped on an empty slot still has a real "to".
		slot.set_meta(&"draggable", true)
		slot.set_meta(&"grid_index", i)
		_inv_grid.add_child(slot)


## Argentum's Estadísticas window, minus the skills half — every character
## here already spawns with every skill at its cap (see the server's
## startingSkills), so a skills list would read "100" five times over and say
## nothing. Attributes still vary by race and move under buffs, so they're
## the part actually worth a window.
func _build_stats() -> void:
	_stats_panel.add_theme_constant_override("separation", 10)

	var rows := VBoxContainer.new()
	rows.add_theme_constant_override("separation", 8)
	_stats_panel.add_child(rows)

	for row in ATTRIBUTE_ROWS:
		var key: String = row[0]
		var caption: String = row[1]

		var panel := Panel.new()
		panel.custom_minimum_size = Vector2(0, 30)
		panel.add_theme_stylebox_override("panel", _flat(COLOR_SLOT, COLOR_SLOT_EDGE, 4))
		rows.add_child(panel)

		var label := Label.new()
		label.text = "%s: 0" % caption
		label.set_anchors_preset(Control.PRESET_FULL_RECT)
		label.offset_left = 10
		label.add_theme_font_size_override("font_size", 13)
		label.add_theme_color_override("font_color", COLOR_TEXT)
		label.vertical_alignment = VERTICAL_ALIGNMENT_CENTER
		panel.add_child(label)

		_attribute_labels[key] = label


## One slot panel. The bag, the equip row and the spell book all use it; only
## the first two want the bag's click-and-drag behaviour, which is what
## wire_bag selects.
##
## The alternative — always wiring the bag handlers and letting the spell book
## disconnect them — does not work: _on_slot_gui_input is connected with a
## .bind(slot), and a bound Callable only matches on disconnect if it carries
## the identical binding, so it is easy to write a disconnect that silently
## fails to match and then errors at runtime. Choosing up front avoids the
## whole question.
func _make_slot(side: int, edge: Color, wire_bag := true) -> InventorySlot:
	var slot := InventorySlot.new()
	slot.custom_minimum_size = Vector2(side, side)
	# STOP (not the default IGNORE) so the slot receives clicks; FOCUS_NONE so
	# it never grabs keyboard focus the way the spell list used to — see the
	# note on keyboard focus in _build_spells.
	slot.mouse_filter = Control.MOUSE_FILTER_STOP
	slot.focus_mode = Control.FOCUS_NONE
	slot.set_meta(&"server_slot", -1)
	# The slot's own colour when it's NOT the current selection — differs
	# between the bag and the equip row, so it has to be remembered per slot
	# rather than assumed, whenever _restyle_slot needs to put it back.
	slot.set_meta(&"base_edge", edge)
	slot.add_theme_stylebox_override("panel", _flat(COLOR_SLOT, edge))
	if wire_bag:
		slot.gui_input.connect(_on_slot_gui_input.bind(slot))
		slot.drag_dropped.connect(swap_requested.emit)
	return slot


## Mirrors the real client exactly: a single click only SELECTS an item (drawn
## with a highlighted border, same idea as Argentum's red-tile-under-the-icon),
## and a second, real double-click is what actually equips or consumes it.
## Clicking an empty slot — or any slot, while nothing is selected — clears the
## selection, same as clicking empty space in the original. Right-click opens
## the same mnuObj context menu Argentum has (Usar/Equipar, Tirar) without
## needing either of those.
func _on_slot_gui_input(event: InputEvent, slot: Panel) -> void:
	if not (event is InputEventMouseButton and event.pressed):
		return
	var server_slot: int = slot.get_meta(&"server_slot", -1)

	if event.button_index == MOUSE_BUTTON_RIGHT:
		if server_slot >= 0:
			_open_context_menu(slot, event.global_position)
		return

	if event.button_index != MOUSE_BUTTON_LEFT:
		return

	if event.double_click and server_slot >= 0:
		item_used.emit(server_slot)
		_select_slot(-1)
		return

	_select_slot(server_slot)


## Argentum's own object menu is three lines: whichever of Usar/Equipar or
## Quitar applies, plus Tirar. clsGrapchicalInventory picks the verb the same
## way — by the item's own type and whether it's already worn — never by
## asking the item itself for a caption.
func _build_context_menu() -> void:
	_context_menu = PopupMenu.new()
	_context_menu.id_pressed.connect(_on_context_menu_id_pressed)
	add_child(_context_menu)


func _open_context_menu(slot: Panel, at: Vector2) -> void:
	_context_slot = slot.get_meta(&"server_slot", -1)
	var item_type: int = slot.get_meta(&"item_type", -1)
	var equipped: bool = slot.get_meta(&"equipped", false)

	_context_menu.clear()
	if EQUIPMENT_TYPES.has(item_type):
		_context_menu.add_item("Quitar" if equipped else "Equipar", 0)
	else:
		_context_menu.add_item("Usar", 0)
	_context_menu.add_item("Tirar", 1)
	_context_menu.position = Vector2i(at)
	_context_menu.popup()


func _on_context_menu_id_pressed(id: int) -> void:
	if _context_slot < 0:
		return
	match id:
		0:
			item_used.emit(_context_slot)
		1:
			drop_requested.emit(_context_slot)
	_select_slot(-1)


## The server slot currently selected in the bag, or -1 for none — what the
## U key acts on, same item a real double-click would.
func selected_slot() -> int:
	return _selected_slot


func _select_slot(server_slot: int) -> void:
	if _selected_slot == server_slot:
		return
	_selected_slot = server_slot
	for slot in _inv_grid.get_children():
		_restyle_slot(slot)


## Applies the highlighted style if this slot holds the current selection,
## otherwise its own resting colour — called both when the selection changes
## and every time a slot is (re)filled, since set_loadout rebuilds the whole
## grid after every equip/use and the highlight has to survive that rebuild
## for whichever slot the player is still looking at.
func _restyle_slot(slot: Control) -> void:
	var server_slot: int = slot.get_meta(&"server_slot", -1)
	if server_slot >= 0 and server_slot == _selected_slot:
		slot.add_theme_stylebox_override("panel", _flat(COLOR_SLOT, COLOR_SELECTED))
	else:
		slot.add_theme_stylebox_override("panel", _flat(COLOR_SLOT, slot.get_meta(&"base_edge", COLOR_SLOT_EDGE)))


## Spells follow Argentum's interaction: pick one from the book, then click a
## target in the world. The book is SPELL_SLOTS fixed positions rather than a
## list that closes up behind what you know — same as the original's own spell
## window, and the reason a slot can be empty and still be a drop target.
##
## It used to be an ItemList. That had to go for two reasons: a focused
## ItemList takes over the arrow keys the game walks with, and its rows are
## not drop targets, so there was nowhere to drag a spell to.
func _build_spells() -> void:
	_spell_grid.add_theme_constant_override("h_separation", 4)
	_spell_grid.add_theme_constant_override("v_separation", 1)

	# One column, the way Argentum's own spell window reads. Thirty rows in the
	# 354 x 262 hole the artwork leaves is 8px a row, which no font fits, so the
	# book scrolls: SPELL_ROWS_VISIBLE rows are on screen and the wheel moves
	# through the rest. A second column was tried first and rejected — it fit
	# without scrolling but stopped looking like a spell list.
	var rows := float(SPELL_SLOTS) / _spell_grid.columns
	var slot_w := int((354 - 4) / float(_spell_grid.columns))
	var slot_h := int(262 / min(rows, SPELL_ROWS_VISIBLE)) - 1

	for i in SPELL_SLOTS:
		# A spell slot is not the bag's select-then-double-click-to-use: it is
		# one click to arm, so it takes its own handlers rather than the bag's.
		var slot := _make_slot(0, COLOR_SLOT_EDGE, false)
		slot.custom_minimum_size = Vector2(slot_w, slot_h)
		# Same split the bag grid uses: grid_index is this panel's fixed
		# position and never changes, so an empty slot is still a valid drop
		# destination, while draggable/server_slot decide whether anything can
		# be picked up FROM it. draggable stays true for every slot precisely
		# because InventorySlot._can_drop_data reads it — turning it off on the
		# empty ones would make the holes unfillable, which is the opposite of
		# what a spell book is arranged for.
		slot.set_meta(&"grid_index", i)
		slot.set_meta(&"draggable", true)
		slot.set_meta(&"spell_id", 0)
		slot.gui_input.connect(_on_spell_slot_gui_input.bind(slot))
		slot.drag_dropped.connect(spell_swap_requested.emit)

		var label := Label.new()
		label.set_anchors_preset(Control.PRESET_FULL_RECT)
		label.offset_left = 4
		label.vertical_alignment = VERTICAL_ALIGNMENT_CENTER
		label.add_theme_font_size_override("font_size", 11)
		label.add_theme_color_override("font_color", COLOR_TEXT)
		label.clip_text = true
		label.mouse_filter = Control.MOUSE_FILTER_IGNORE
		slot.add_child(label)

		_spell_grid.add_child(slot)

	_spell_info.add_theme_color_override("font_color", COLOR_TEXT_DIM)
	_spell_info.add_theme_font_size_override("font_size", 11)

	# LANZAR/INFO live in the scene now rather than being built here: the
	# background image already paints their plates, so their position is part
	# of the layout and belongs in main.tscn with every other measured offset.
	_cast_button = $SidePanel/CastButton
	_info_button = $SidePanel/InfoButton
	for b: Button in [_cast_button, _info_button]:
		_style_over_art(b)
		b.add_theme_font_size_override("font_size", 13)
	_cast_button.text = "LANZAR"
	_info_button.text = "INFO"
	_cast_button.pressed.connect(_on_cast_pressed)
	_info_button.pressed.connect(_on_info_pressed)


## Controls that sit on top of the painted artwork keep their text and their
## click target but draw no box of their own — the plate underneath is already
## in the image, and a StyleBox on top would just cover it up.
func _style_over_art(c: Control) -> void:
	c.focus_mode = Control.FOCUS_NONE
	var clear := StyleBoxEmpty.new()
	for state in ["normal", "hover", "pressed", "disabled", "focus"]:
		c.add_theme_stylebox_override(state, clear)
	c.add_theme_color_override("font_color", COLOR_TEXT)
	c.add_theme_color_override("font_hover_color", COLOR_ACCENT)
	c.add_theme_color_override("font_pressed_color", COLOR_ACCENT)
	c.add_theme_color_override("font_disabled_color", COLOR_TEXT_DIM.darkened(0.35))
	c.add_theme_color_override("font_shadow_color", Color(0, 0, 0, 0.95))
	c.add_theme_constant_override("shadow_offset_x", 1)
	c.add_theme_constant_override("shadow_offset_y", 1)


## The converted obj.dat row for one item id, or empty if it is unknown. The
## HUD already owns the AOData table every panel reads from, so this is the
## lookup the rest of the client borrows rather than loading a second copy.
func item_data(id: int) -> Dictionary:
	return _data.item(id)


## Checks the server's own slot count against the one this panel was built
## with. A mismatch would silently truncate whatever did not fit, which is the
## kind of thing that looks like a lost spell rather than a version skew.
func set_spell_slots(slots: int) -> void:
	if slots > 0 and slots != SPELL_SLOTS:
		log_line(
			"El servidor usa %d slots de hechizos y el cliente %d." % [slots, SPELL_SLOTS], COLOR_EXP
		)


func spell_name(id: int) -> String:
	var spell := _data.spell(id)
	return "hechizo %d" % id if spell.is_empty() else str(spell.get("name", ""))


func _selected_spell_id() -> int:
	if _selected_spell_slot < 0 or _selected_spell_slot >= _spell_ids.size():
		return 0
	return _spell_ids[_selected_spell_slot]


## One left click on a spell both selects it and arms the crosshair, which is
## the whole interaction: pick the spell, then one left click in the world
## spends it. LANZAR still works and does the same thing, for anyone who
## reaches for the button.
##
## A reorder drag starts from the same slots, so the selection is only moved on
## a real click — Godot delivers the button-down that begins a drag through
## here too, and arming the crosshair on it would leave a stray target cursor
## every time the book got rearranged.
func _on_spell_slot_gui_input(event: InputEvent, slot: Panel) -> void:
	if not (event is InputEventMouseButton and event.pressed):
		return
	if event.button_index != MOUSE_BUTTON_LEFT:
		return
	var spell_id: int = slot.get_meta(&"spell_id", 0)
	if spell_id == 0:
		return

	_selected_spell_slot = slot.get_meta(&"grid_index", -1)
	_restyle_spell_slots()
	_refresh_spell_info()
	cast_requested.emit(spell_id)


## Keeps the selection highlight on whichever slot _selected_spell_slot names.
## Called after every refill, so a reorder or a pickup redraws the highlight
## where the player left it rather than losing it.
func _restyle_spell_slots() -> void:
	for i in _spell_grid.get_child_count():
		var slot := _spell_grid.get_child(i) as Panel
		var edge := COLOR_SELECTED if i == _selected_spell_slot else COLOR_SLOT_EDGE
		slot.add_theme_stylebox_override("panel", _flat(COLOR_SLOT, edge))


func _refresh_spell_info() -> void:
	var id := _selected_spell_id()
	if id == 0:
		_spell_info.text = "elegí un hechizo"
		_cast_button.disabled = true
		return
	_spell_info.text = _data.spell_summary(id)
	_cast_button.disabled = false


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


## The five bars sit exactly on the troughs the artwork already paints, so
## each one is an absolutely-positioned fill with the value written over it —
## no background stylebox, or it would hide the carved trough underneath.
func _build_bars() -> void:
	var container: Control = $SidePanel/Bars
	for row in BAR_ROWS:
		var key: String = row[0]
		var y: int = row[1]
		var caption: String = row[2]
		var color: Color = row[3]

		var bar := ProgressBar.new()
		bar.position = Vector2(BAR_X, y)
		bar.size = Vector2(BAR_W, BAR_H)
		bar.show_percentage = false
		bar.mouse_filter = Control.MOUSE_FILTER_IGNORE
		bar.add_theme_stylebox_override("background", StyleBoxEmpty.new())
		bar.add_theme_stylebox_override("fill", _flat(color, color.darkened(0.45), 2, 0))
		container.add_child(bar)

		var label := Label.new()
		label.set_anchors_preset(Control.PRESET_FULL_RECT)
		label.horizontal_alignment = HORIZONTAL_ALIGNMENT_CENTER
		label.vertical_alignment = VERTICAL_ALIGNMENT_CENTER
		label.add_theme_color_override("font_color", Color.WHITE)
		label.add_theme_color_override("font_shadow_color", Color(0, 0, 0, 0.95))
		label.add_theme_constant_override("shadow_offset_x", 1)
		label.add_theme_constant_override("shadow_offset_y", 1)
		label.add_theme_font_size_override("font_size", 10)
		label.mouse_filter = Control.MOUSE_FILTER_IGNORE
		bar.add_child(label)

		_bars[key] = {"bar": bar, "label": label, "caption": caption}


## Six button plates are painted into the background, and we only have three
## things worth putting on them. The spares get an empty caption and stay
## disabled rather than being left out — the plate is in the image either way,
## and a blank one reads better than one labelled with a feature that does not
## exist in a match.
func _build_buttons() -> void:
	var container: Control = $SidePanel/Buttons
	for i in ACTION_YS.size():
		var button := Button.new()
		button.position = Vector2(ACTION_X, ACTION_YS[i])
		button.size = Vector2(ACTION_W, ACTION_H)
		button.text = PANEL_BUTTONS[i] if i < PANEL_BUTTONS.size() else ""
		button.disabled = true
		_style_over_art(button)
		button.add_theme_font_size_override("font_size", 11)
		container.add_child(button)


func _style_panels() -> void:
	for panel in [$Console, $MinimapFrame]:
		panel.add_theme_stylebox_override("panel", _flat(COLOR_INSET, COLOR_SLOT_EDGE, 4, 2))

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
	# main.gd fills this in from the first snapshot — see _update_zone.
	set_zone("")


## The name and class ride on the carved plaque the artwork paints, so they
## are light-on-stone rather than the panel's usual gold-on-dark.
func _style_character_header() -> void:
	for l: Label in [_char_name, _char_class, _potion_red, _potion_blue, _alive_box, _coords]:
		l.add_theme_color_override("font_shadow_color", Color(0, 0, 0, 0.95))
		l.add_theme_constant_override("shadow_offset_x", 1)
		l.add_theme_constant_override("shadow_offset_y", 1)

	_char_name.add_theme_color_override("font_color", Color("e8ddd4"))
	_char_name.add_theme_font_size_override("font_size", 20)
	_char_class.add_theme_color_override("font_color", Color("b3a79f"))
	_char_class.add_theme_font_size_override("font_size", 11)

	for l: Label in [_potion_red, _potion_blue, _alive_box, _coords]:
		l.add_theme_color_override("font_color", COLOR_TEXT)
		l.add_theme_font_size_override("font_size", 12)
	_alive_box.add_theme_color_override("font_color", COLOR_ACCENT)


func _flat(fill: Color, edge: Color, radius: int = 2, border: int = 1) -> StyleBoxFlat:
	var box := StyleBoxFlat.new()
	box.bg_color = fill
	box.border_color = edge
	box.set_border_width_all(border)
	box.set_corner_radius_all(radius)
	return box
