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

## The name plaque. Sized against its own face (446x65 on the baked panel)
## rather than against the rest of the panel's captions: it is the one feature
## with room to be large, and the line under it is deliberately less than half
## of it so the pair reads as a heading and its subtitle.
const NAME_FONT_SIZE := 26
const CLASS_FONT_SIZE := 12

const INVENTORY_SLOTS := 30
## Must match the server's world.SpellSlots. The Welcome carries the server's
## own figure and set_spell_slots checks it, so a mismatch is reported rather
## than silently truncating someone's book.
const SPELL_SLOTS := 30
## How many spell rows are on screen at once. Fifteen rows in this hole came
## out 18px tall, which is a thin thing to hit with a mouse mid-fight and a
## thinner thing to grab for a reorder drag; ten rows make each one 26px. The
## rest are reached by scrolling, and a drag near either edge scrolls on its
## own — see _process.
const SPELL_ROWS_VISIBLE := 10.0
## Gap between spell rows. Two pixels is what makes ten rows of 26 land on
## exactly the hole's 278.
const SPELL_SEPARATION := 2
## How close to the top or bottom edge of the spell list a dragged spell has to
## be for the list to start scrolling under it, and how fast it then goes. A
## band of 24px is about one row: any less and you have to hunt for it with a
## spell already in hand.
const DRAG_SCROLL_BAND := 24.0
const DRAG_SCROLL_SPEED := 420.0

## The bone-framed hole runs x 82..456 and y 276..582 on the baked panel, and
## both tabs share it. Neither number is repeated here: the slot sizes come off
## the InvGrid/SpellList rects in main.tscn, which are that hole inset by a
## couple of pixels, so the art is measured in one place and the layout follows
## it. See _hole_of.

## Which item types are worn rather than consumed. Only the right-click menu
## reads this, to word itself "Equipar/Quitar" instead of "Usar" — the server
## is still the one that decides what actually happens to a slot.
const EQUIPMENT_TYPES := [
	AOData.TYPE_WEAPON, AOData.TYPE_SHIELD, AOData.TYPE_ARMOR, AOData.TYPE_HELMET, AOData.TYPE_RING,
]

## Raised when the player picks a spell and presses LANZAR. The HUD does not
## know what a target is; main.gd takes it from here into targeting mode.
signal cast_requested(spell_id: int)

## Raised to consume what is in a bag slot — a potion, food, a drink, and
## whatever throwables come later. Never equipment: see _on_slot_gui_input.
## server_slot is the server's own InventorySlot.Slot number for whatever is
## displayed there, not the visual grid position.
signal item_used(server_slot: int)

## Raised to put on or take off what is in a bag slot. Split from item_used
## because the two are different intentions and the panel already knows which
## one a slot can accept — sending one message and letting the item's type pick
## the branch is how pressing E on a potion used to drink it.
signal item_equipped(server_slot: int)

## Raised when the player drags one bag slot onto another. Equip-row slots
## never raise this — see InventorySlot's `draggable` note.
signal swap_requested(from_slot: int, to_slot: int)

## Raised when the player drags one spell slot onto another. Separate from
## swap_requested because the bag and the spell book are separate server state
## and a bag index is not a spell index — the server has two handlers, and one
## signal carrying both would have to be told apart somewhere anyway.
signal spell_swap_requested(from_slot: int, to_slot: int)

## Raised by SALIR or the X in the panel's title bar. The HUD does not own the
## socket, so it says what happened and main.gd decides that leaving the match
## means closing the connection and then the client.
signal quit_requested

## Raised by the right-click context menu's "Tirar" — the same drop Argentum's
## own mnuTirar sends, just reached without needing to stand and press a key.
signal drop_requested(server_slot: int)

## The side panel buttons Argentum shows. None are wired yet; they are here so
## the panel can be judged at its real density. Trimmed to what a single BR
## match without persistence could ever actually use — Amigos/Grupo/Clanes/
## Quests all presuppose state that outlives a match (a friends list, a
## guild, a quest log), which the genre-fork decision already ruled out.
## Estadísticas is gone entirely, tab and button alike: everyone spawns at the
## cap, so the five attributes only move under a buff that already announces
## itself in the console, and the panel reads better with two tabs than three.
const PANEL_BUTTONS := [
	"Mapa", "Retos", "Opciones"
]

## Where each vitals bar's coloured fill sits on the background image, and
## which Vitals wire keys drive it. Measured by connected-component labelling
## on the baked 525x962 panel: five identical troughs, 174x14, stacked down the
## bone-framed box on the left. The artwork draws the trough and its outline;
## only the fill is a live control, so these rects are the *fill* area.
const BAR_X := 61
const BAR_W := 174
const BAR_H := 14
## Big enough to read, small enough that the reading fits inside those 14px
## along with the caption — the trough is the only place on this panel with
## room for the number.
const BAR_FONT_SIZE := 10
## All five, because this artwork paints five troughs.
##
## Energía, Hambre and Sed had been dropped from the old panel: none of them
## ever moves — hunger and thirst have never drained and energy is never spent
## — so three bars pinned at 100/100 were furniture crowding the two numbers
## that decide a fight. That was the right call for a panel with room for two.
## Here the art reserves five slots in a bone frame, and three unlit troughs
## read as a broken panel rather than as a deliberate omission, so they are
## filled again. The colours are the artwork's own outlines, sampled off the
## PNG, so a full bar matches the trough it sits in.
const BAR_ROWS := [
	["hp",   750, "SALUD",  Color("c8383a")],
	["mana", 786, "MANÁ",   Color("5a76ad")],
	["sta",  821, "ENERGÍA", Color("f7ca5e")],
	["hun",  856, "HAMBRE", Color("688d54")],
	["thi",  892, "SED",    Color("6c9559")],
]

## The six action-button plates the artwork draws down the right side. We only
## have three things worth putting there, so the rest stay visibly disabled
## rather than being hidden — the plates are painted into the background and
## cannot be removed anyway.
## The six plates sit between the icon pair the artwork stamps at each end,
## so the caption uses the face between them (x 354..479) rather than the whole
## plate — text over a painted icon reads as a mistake.
const ACTION_X := 354
const ACTION_W := 126
const ACTION_H := 26
const ACTION_YS := [723, 756, 788, 821, 854, 887]

@onready var _alive: Label = $TopBar/Alive
@onready var _zone: Label = $TopBar/Zone
@onready var _log: RichTextLabel = $Console/Log
@onready var _char_name: Label = $SidePanel/CharName
@onready var _char_class: Label = $SidePanel/CharClass
@onready var _tab_inv: Button = $SidePanel/TabInv
@onready var _tab_spells: Button = $SidePanel/TabSpells
@onready var _inv_grid: GridContainer = $SidePanel/InvGrid
@onready var _spell_scroll: ScrollContainer = $SidePanel/SpellList
@onready var _spell_grid: GridContainer = $SidePanel/SpellList/SpellGrid
@onready var _spell_info: Label = $SidePanel/SpellInfo
## The bone scrollbar the artwork paints down the right of the spell list. It
## is a real VScrollBar of our own rather than the ScrollContainer's, because
## the container puts its own bar wherever its rect ends and the painted rail
## has to be met exactly — arrows included. See _build_spell_scrollbar.
@onready var _spell_bar: VScrollBar = $SidePanel/SpellScroll
## The painted rail, cut out of the background and made a node of its own.
## It stays part of the artwork in every other respect — nothing about it
## moves — but it had to stop being *painted*, because it is furniture that
## belongs to the spell book alone: on the inventory tab it scrolled nothing
## and still took 26px out of the grid's width. Now it appears with the book
## and leaves with it. Same rule as always, one step further: what does not
## move stays in the background, unless it does not always belong there.
@onready var _scroll_rail: TextureRect = $SidePanel/ScrollRail
## The two boxes beside the potions the artwork paints. Argentum's yellow
## potion is agility and its green one is strength, so each box carries the
## attribute its own bottle buffs — read together with the potions they sit
## next to, they answer "am I doped, or do I need to be?" without opening
## anything. They are live values, not bag counts: a buff or a debuff moves
## them, which is the whole point of putting them here.
@onready var _agility_box: Label = $SidePanel/AgilityBox
@onready var _strength_box: Label = $SidePanel/StrengthBox
## The inset plate beside the painted treasure chest. A chest next to a number
## reads as loot, and the only score a match without an economy keeps is how
## many people you put down — so it carries the kill count.
@onready var _kills_box: Label = $SidePanel/KillsBox
@onready var _coords: Label = $SidePanel/Coords
## The two plates in the panel's title bar. The left one had a little tree
## baked into it that meant nothing here and is painted out of the art now;
## the right one is the X the artwork already draws. Both do the same thing —
## leave the match — so the X gets the word next to it rather than a tooltip
## nobody hovers for.
## Where the source of whatever server you are playing on lives. This is not
## a credit line, it is a licence term: the AGPL is copyleft over the network,
## and §13 says anyone who interacts with the program remotely has to be
## offered its complete source. A player of the web build never sees the
## README, so the offer has to be on screen — and the empty half of the title
## bar is the one place on this panel that costs nothing to give it.
@onready var _source_link: Label = $SidePanel/SourceLink
@onready var _quit_button: Button = $SidePanel/QuitButton
@onready var _close_button: Button = $SidePanel/CloseButton
## The footer's four readouts, in the order the artwork stamps their icons:
## helmet, armour, shield, weapon. Empty when nothing is worn in that slot —
## an empty black box is the artwork's own way of saying "nothing here", and it
## reads better than a zero that looks like a real, terrible stat.
@onready var _gear_boxes := {
	AOData.TYPE_HELMET: $Footer/Helmet,
	AOData.TYPE_ARMOR: $Footer/Armor,
	AOData.TYPE_SHIELD: $Footer/Shield,
	AOData.TYPE_WEAPON: $Footer/Weapon,
}

var _bars: Dictionary = {}
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
## Whether that slot holds something worn rather than consumed, remembered
## when the menu opens so the single "act on it" entry knows which of the two
## messages it is.
var _context_equipment := false


func _ready() -> void:
	_sprites.load_bundle()
	_data.load_data()

	_style_panels()
	_style_top_bar()
	_style_character_header()
	_build_tabs()
	_build_inventory()
	_build_spells()
	_build_spell_scrollbar()
	_build_bars()
	_build_buttons()
	_build_footer()
	_build_context_menu()
	_build_titlebar()
	_show_tab(&"inv")


## set_loadout fills the bag and the spell list from what the server says the
## player is carrying and knows.
func set_loadout(loadout: Dictionary) -> void:
	_fill_inventory(loadout.get("inv", []))
	_fill_spells(loadout.get("spells", []))
	_update_gear_readout(loadout.get("inv", []))


## The bar across the bottom of the world view: what you are actually wearing,
## in numbers. Argentum makes you open the inventory and hover an item to learn
## that, which is a fine trade in an MMO and a bad one in a match that lasts
## minutes — the whole question when you pick something off the floor is
## whether it beats what you have on.
func _build_footer() -> void:
	for box: Label in _gear_boxes.values():
		box.add_theme_color_override("font_color", COLOR_TEXT)
		box.add_theme_color_override("font_shadow_color", Color(0, 0, 0, 0.95))
		box.add_theme_constant_override("shadow_offset_x", 1)
		box.add_theme_constant_override("shadow_offset_y", 1)
		box.add_theme_font_size_override("font_size", 14)


## Reads the equipped items straight out of the bag the server just sent. No
## new protocol: the client already has obj.dat's own MinHit/MaxHit and
## MinDef/MaxDef for every item id, and the loadout already says which slots
## are worn, so the numbers are the item table's rather than a second opinion
## computed somewhere else.
func _update_gear_readout(slots: Array) -> void:
	var text := {}
	for entry in slots:
		if not bool(entry.get("e", false)):
			continue
		var item := _data.item(int(entry.get("i", 0)))
		if item.is_empty():
			continue
		var type := int(item.get("type", -1))
		if not _gear_boxes.has(type):
			continue
		# The weapon reports what it deals and the rest what they stop, which
		# is the same pair of fields under two names in obj.dat.
		if type == AOData.TYPE_WEAPON:
			text[type] = _stat_range(item, "minHit", "maxHit")
		else:
			text[type] = _stat_range(item, "minDef", "maxDef")

	for type: int in _gear_boxes:
		_gear_boxes[type].text = str(text.get(type, ""))


## "5-10", or "10" when the range is a single number.
##
## An item with no defence at all still reports its 0. Only an *empty* slot
## leaves the box black — newbie clothing is exactly that case (the Túnica has
## no MinDef or MaxDef in obj.dat and the Armadura de Cuero has MaxDef 1 and no
## MinDef), and blanking it made wearing something look identical to wearing
## nothing. "0" is information; a black box in a slot you filled is a bug.
func _stat_range(item: Dictionary, low_key: String, high_key: String) -> String:
	var low := int(item.get(low_key, 0))
	var high := int(item.get(high_key, 0))
	if low == high:
		return str(high)
	return "%d-%d" % [low, high]


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
	var item_grh := int(item.get("grh", 0))
	var rect: Rect2 = _sprites.grh_rect(item_grh, 0.0)
	if rect.size.x > 0.0:
		var atlas := AtlasTexture.new()
		atlas.atlas = _sprites.texture_for(item_grh)
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
	# Upper case, like the tabs and every other caption the panel carries. The
	# plaque is the biggest single feature on it and a lower-case name sitting
	# in it read as text that had landed there rather than as an engraving.
	_char_name.text = player_name.to_upper()
	# The plaque's face is 446px wide and the label is pinned to it, so a long
	# name would spill past the frame. Shrinking the font to fit keeps it inside
	# without truncating anyone's name — a name is the one string on this panel
	# that should never come back as "WACHINCI…".
	_fit_label(_char_name, NAME_FONT_SIZE, 12)


## class_id/race_id are the same indices the character picker sent the server
## at join — see AOData.CLASS_NAMES/RACE_NAMES for why that index doubles as
## the name lookup without another round trip to the server.
func set_identity(class_id: int, race_id: int) -> void:
	var class_name_: String = _data.CLASS_NAMES[class_id] if class_id >= 0 and class_id < _data.CLASS_NAMES.size() else "?"
	var race_name: String = _data.RACE_NAMES[race_id] if race_id >= 0 and race_id < _data.RACE_NAMES.size() else "?"
	_char_class.text = ("%s · %s" % [class_name_, race_name]).to_upper()


func set_vitals(vitals: Dictionary) -> void:
	_apply_bar("hp", int(vitals.get("hp", 0)), int(vitals.get("maxHp", 1)))
	_apply_bar("mana", int(vitals.get("mana", 0)), int(vitals.get("maxMana", 1)))
	_apply_bar("sta", int(vitals.get("sta", 0)), int(vitals.get("maxSta", 1)))
	_apply_bar("hun", int(vitals.get("hun", 0)), int(vitals.get("maxHun", 1)))
	_apply_bar("thi", int(vitals.get("thi", 0)), int(vitals.get("maxThi", 1)))
	# Sent fresh every tick precisely because a spell can move them.
	_agility_box.text = str(int(vitals.get("agi", 0)))
	_strength_box.text = str(int(vitals.get("str", 0)))


## The count lives in the bar over the viewport and nowhere else. It had a
## second home in the panel's top inset for a while and that was one readout
## too many: the same number twice on one screen reads as two numbers.
func set_alive(count: int) -> void:
	_alive.text = "◈  VIVOS  %d" % count


## Own kills, read from the local player's own entity in the snapshot — the
## same counter the click-to-inspect line reports for anyone else.
func set_kills(count: int) -> void:
	_kills_box.text = "BAJAS  %d" % count


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
	var fill: ColorRect = entry["fill"]
	var ratio := clampf(float(value) / float(maxi(maximum, 1)), 0.0, 1.0)
	fill.size = Vector2(roundf(BAR_W * ratio), BAR_H)
	entry["label"].text = "%s  %d/%d" % [entry["caption"], value, maximum]


## Inventory and spells share one region of the panel, exactly as in Argentum:
## the tabs swap which one is visible rather than shrinking both.
func _build_tabs() -> void:
	_tab_inv.text = "INVENTARIO"
	_tab_spells.text = "HECHIZOS"
	for tab: Button in [_tab_inv, _tab_spells]:
		tab.focus_mode = Control.FOCUS_NONE
		tab.add_theme_font_size_override("font_size", 12)
		tab.add_theme_color_override("font_color", COLOR_TEXT_DIM)
		tab.add_theme_color_override("font_hover_color", COLOR_TEXT)
	_tab_inv.pressed.connect(_show_tab.bind(&"inv"))
	_tab_spells.pressed.connect(_show_tab.bind(&"spells"))


func _show_tab(tab: StringName) -> void:
	var showing_inv := tab == &"inv"
	var showing_spells := tab == &"spells"

	_inv_grid.visible = showing_inv
	_spell_scroll.visible = showing_spells
	_spell_info.visible = showing_spells
	# The rail belongs to the book, so it leaves with it — that is the whole
	# reason it stopped being painted into the background.
	_scroll_rail.visible = showing_spells
	_sync_spell_scrollbar()
	# LANZAR/INFO are painted into the background image, so they cannot be
	# hidden with the spell list — they just go dead outside the spell tab.
	_cast_button.disabled = not showing_spells
	_info_button.disabled = not showing_spells

	# The active tab is the lit one, so the panel reads at a glance.
	_style_tab(_tab_inv, showing_inv)
	_style_tab(_tab_spells, showing_spells)


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
	var hole := _hole_of(_inv_grid)
	var columns := _inv_grid.columns
	var rows := int(ceil(float(INVENTORY_SLOTS) / columns))
	var side := int(
		min(
			(hole.x - (columns - 1) * SEPARATION) / float(columns),
			(hole.y - (rows - 1) * SEPARATION) / float(rows),
		)
	)

	# Square slots almost never divide the hole exactly, and the leftover used
	# to pile up on the right and the bottom — the grid sat in a corner of its
	# own frame. Splitting it four ways costs nothing and is the difference
	# between a grid that fills the hole and one that fits in it.
	var used := Vector2(
		columns * side + (columns - 1) * SEPARATION, rows * side + (rows - 1) * SEPARATION
	)
	var slack := ((hole - used) * 0.5).floor()
	_inv_grid.offset_left += slack.x
	_inv_grid.offset_top += slack.y
	_inv_grid.offset_right = _inv_grid.offset_left + used.x
	_inv_grid.offset_bottom = _inv_grid.offset_top + used.y

	for i in INVENTORY_SLOTS:
		var slot := _make_slot(side, COLOR_SLOT_EDGE)
		# grid_index is this Panel's own fixed position (0..29), matching the
		# server's own Slot numbering one-to-one — unlike server_slot, it never
		# changes, so a drag dropped on an empty slot still has a real "to".
		slot.set_meta(&"draggable", true)
		slot.set_meta(&"grid_index", i)
		_inv_grid.add_child(slot)


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


## A single click only SELECTS an item, drawn with a highlighted border — the
## same idea as Argentum's red tile under the icon. A double-click **consumes**
## it, and only consumes it: a sword, a shield, a helmet does not answer a
## double-click at all.
##
## That is a deliberate departure from the original, where one click branches on
## the item's type and equips or drinks accordingly. Equipping and consuming are
## opposite in consequence — one is undone with another click, the other is
## gone — and a bag where every icon looks alike under the same gesture is a bag
## where a mis-click costs a potion. So the destructive half gets the gesture
## and the reversible half gets its own keys: E, or the right-click menu.
##
## Clicking an empty slot — or any slot, while nothing is selected — clears the
## selection, same as clicking empty space in the original.
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
		if EQUIPMENT_TYPES.has(int(slot.get_meta(&"item_type", -1))):
			# Said out loud rather than ignored. A gesture that does nothing
			# reads as a broken panel; a gesture that says where the action
			# actually lives teaches the two keys once.
			log_line("Eso no se consume: se equipa con E o con el botón derecho.", COLOR_TEXT_DIM)
			_select_slot(server_slot)
			return
		item_used.emit(server_slot)
		_select_slot(-1)
		return

	_select_slot(server_slot)


## Argentum's own object menu is three lines: whichever of Usar/Equipar or
## Quitar applies, plus Tirar. clsGrapchicalInventory picks the verb the same
## way — by the item's own type and whether it's already worn — never by
## asking the item itself for a caption. Here the verb picks the message too,
## so the menu is the one place equipment can be toggled with the mouse.
func _build_context_menu() -> void:
	_context_menu = PopupMenu.new()
	_context_menu.id_pressed.connect(_on_context_menu_id_pressed)
	add_child(_context_menu)


func _open_context_menu(slot: Panel, at: Vector2) -> void:
	_context_slot = slot.get_meta(&"server_slot", -1)
	var item_type: int = slot.get_meta(&"item_type", -1)
	var equipped: bool = slot.get_meta(&"equipped", false)

	_context_equipment = EQUIPMENT_TYPES.has(item_type)
	_context_menu.clear()
	if _context_equipment:
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
			if _context_equipment:
				item_equipped.emit(_context_slot)
			else:
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
	_spell_grid.add_theme_constant_override("h_separation", SPELL_SEPARATION)
	_spell_grid.add_theme_constant_override("v_separation", SPELL_SEPARATION)

	# One column, the way Argentum's own spell window reads. Thirty rows in the
	# hole the artwork leaves is 9px a row, which no font fits, so the book
	# scrolls: SPELL_ROWS_VISIBLE rows are on screen and the wheel moves
	# through the rest. A second column was tried first and rejected — it fit
	# without scrolling but stopped looking like a spell list.
	#
	# Sized off the container's own rect. A row now takes the full width of it,
	# because the bone bar no longer stands inside the list stealing a column
	# of it — the rail is its own node beside the list (see _scroll_rail), so
	# the list ends where the rail begins and every pixel up to there is a
	# click target.
	var hole := _hole_of(_spell_scroll)
	var rows := float(SPELL_SLOTS) / _spell_grid.columns
	var visible_rows := minf(rows, SPELL_ROWS_VISIBLE)
	var slot_w := int((hole.x - (_spell_grid.columns - 1) * SPELL_SEPARATION)
		/ float(_spell_grid.columns))
	var slot_h := int((hole.y - (visible_rows - 1) * SPELL_SEPARATION) / visible_rows)

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
		_style_plate_button(b)
		b.add_theme_font_size_override("font_size", 13)
	_cast_button.text = "LANZAR"
	_info_button.text = "INFO"
	_cast_button.pressed.connect(_on_cast_pressed)
	_info_button.pressed.connect(_on_info_pressed)


## Dresses our scrollbar in the artwork own bone and hands it the list.
##
## The panel painted a complete scrollbar — rail, two arrow plates, and a bone
## with an iron ring for a grabber — and none of it did anything, while Godot
## drew its own grey bar beside it. The bone was cut out first
## (scroll_grabber.png) because it moves; the rail and the arrow plates
## followed (scroll_rail.png) because they belong to the spell book rather than
## to the panel, and painted into the background they went on taking 26px out
## of the inventory grid to scroll a tab that has nothing to scroll. So the
## live bar sits on a rail that is itself a node now, at the exact coordinates
## the art had it: idle, the two are indistinguishable from the painting they
## replaced.
func _build_spell_scrollbar() -> void:
	var bone: Texture2D = load("res://assets/ao/ui/scroll_grabber.png")
	_spell_bar.add_theme_stylebox_override("grabber", _bone_box(bone, Color(1, 1, 1)))
	_spell_bar.add_theme_stylebox_override("grabber_highlight", _bone_box(bone, Color(1.15, 1.13, 1.08)))
	_spell_bar.add_theme_stylebox_override("grabber_pressed", _bone_box(bone, Color(0.82, 0.80, 0.78)))
	for state in ["scroll", "scroll_focus"]:
		_spell_bar.add_theme_stylebox_override(state, StyleBoxEmpty.new())

	# The arrows are painted; these invisible icons are what keeps Godot own
	# increment/decrement buttons sitting on top of them, at the right size, so
	# clicking a painted arrow actually scrolls.
	var blank: Texture2D = load("res://assets/ao/ui/scroll_arrow.png")
	for icon in ["increment", "increment_highlight", "increment_pressed",
			"decrement", "decrement_highlight", "decrement_pressed"]:
		_spell_bar.add_theme_icon_override(icon, blank)

	# The list scrolls itself with the wheel and keeps its own bar hidden; the
	# two are kept in step in both directions rather than one driving the other,
	# so the wheel moves the bone and the bone moves the list.
	var inner := _spell_scroll.get_v_scroll_bar()
	inner.changed.connect(_sync_spell_scrollbar)
	inner.value_changed.connect(func(v: float) -> void: _spell_bar.set_value_no_signal(v))
	_spell_bar.value_changed.connect(func(v: float) -> void: _spell_scroll.scroll_vertical = int(v))
	_sync_spell_scrollbar()


## Scrolls the spell book while a spell is being dragged over either end of
## it. Without this the book can only be rearranged among the ten rows that
## happen to be on screen: a drag holds the mouse button, so there is no way to
## reach for the wheel, and Godot's ScrollContainer does not scroll for a drag
## by itself. Moving a spell from the last row to the first is the whole point
## of a book you arrange to taste, so the list has to come to meet you.
func _process(delta: float) -> void:
	if not _spell_scroll.visible or not get_viewport().gui_is_dragging():
		return
	var rect := Rect2(_spell_scroll.global_position, _spell_scroll.size)
	var mouse := get_global_mouse_position()
	if not rect.has_point(mouse):
		return
	var direction := 0.0
	if mouse.y < rect.position.y + DRAG_SCROLL_BAND:
		direction = -1.0
	elif mouse.y > rect.end.y - DRAG_SCROLL_BAND:
		direction = 1.0
	if direction != 0.0:
		_spell_scroll.scroll_vertical += int(round(direction * DRAG_SCROLL_SPEED * delta))


func _sync_spell_scrollbar() -> void:
	var inner := _spell_scroll.get_v_scroll_bar()
	_spell_bar.min_value = inner.min_value
	_spell_bar.max_value = inner.max_value
	_spell_bar.page = inner.page
	_spell_bar.step = inner.step
	_spell_bar.set_value_no_signal(inner.value)
	# Hidden when the book fits, like any scrollbar, and hidden along with the
	# list when another tab is showing — it belongs to the list, not the panel.
	_spell_bar.visible = _spell_scroll.visible and inner.max_value > inner.page


## The bone as a 9-slice. The iron ring sits 38px down a 103px bone, so it has
## to fall inside the fixed top band: the only strip allowed to stretch is the
## plain shaft between the ring and the bottom knuckle. Slicing it anywhere
## else smears the ring as the grabber grows.
func _bone_box(bone: Texture2D, tint: Color) -> StyleBoxTexture:
	var box := StyleBoxTexture.new()
	box.texture = bone
	box.texture_margin_top = 70
	box.texture_margin_bottom = 25
	box.modulate_color = tint
	return box


## LANZAR and INFO, pressable as whole plates.
##
## They used to be bare text over a plate that was only painted, so the plate
## never reacted and only the letters lit up — the click target was the whole
## button but nothing said so. Now the plate itself is a texture cut from the
## panel art (button_plate.png) and drawn by the button in its own four states,
## on the exact rect the painted plate occupies, so the two line up pixel for
## pixel while idle.
##
## Pressing shifts the drawn plate two pixels down (expand margins, negative at
## the top and positive at the bottom, which moves the box without resizing it)
## and darkens it. The sliver of the painted plate that the shift uncovers at
## the top is what sells it: the plate reads as sinking into its own socket,
## and the shadow is the artwork own top bevel rather than a drawn gradient.
## Releasing puts every pixel back where it was.
func _style_plate_button(b: Button) -> void:
	var plate: Texture2D = load("res://assets/ao/ui/button_plate.png")
	b.focus_mode = Control.FOCUS_NONE
	b.add_theme_stylebox_override("normal", _plate_box(plate, Color(1, 1, 1), 0))
	b.add_theme_stylebox_override("hover", _plate_box(plate, Color(1.14, 1.10, 1.06), 0))
	b.add_theme_stylebox_override("pressed", _plate_box(plate, Color(0.70, 0.66, 0.66), 2))
	b.add_theme_stylebox_override("focus", StyleBoxEmpty.new())
	b.add_theme_stylebox_override("disabled", _plate_box(plate, Color(0.62, 0.60, 0.62), 0))
	b.add_theme_color_override("font_color", COLOR_TEXT)
	b.add_theme_color_override("font_hover_color", COLOR_ACCENT)
	b.add_theme_color_override("font_pressed_color", COLOR_ACCENT)
	b.add_theme_color_override("font_disabled_color", COLOR_TEXT_DIM.darkened(0.35))
	b.add_theme_color_override("font_shadow_color", Color(0, 0, 0, 0.95))
	b.add_theme_constant_override("shadow_offset_x", 1)
	b.add_theme_constant_override("shadow_offset_y", 1)


## One plate state. sink is how many pixels the plate (and its caption with it)
## drops when pressed.
func _plate_box(plate: Texture2D, tint: Color, sink: int) -> StyleBoxTexture:
	var box := StyleBoxTexture.new()
	box.texture = plate
	# 9-slice: the chamfered corners and the studded frame keep their size, the
	# middle stretches. The two plates differ by two pixels of width and this is
	# what lets one texture serve both.
	box.texture_margin_left = 16
	box.texture_margin_right = 16
	box.texture_margin_top = 12
	box.texture_margin_bottom = 12
	box.modulate_color = tint
	box.expand_margin_top = -sink
	box.expand_margin_bottom = sink
	# The caption rides down with the plate instead of floating over it. These
	# stay well under the plate's 39px: content margins feed the button's
	# minimum size, and a minimum taller than the plate stretches the texture
	# past the painted one it has to sit on — the button came out 42px with the
	# margins matched to the 9-slice.
	box.content_margin_top = 6 + sink
	box.content_margin_bottom = 6 - sink
	box.content_margin_left = 16
	box.content_margin_right = 16
	return box


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

		# Not a ProgressBar. Control.size clamps to the combined minimum size,
		# and a stock ProgressBar's minimum is tall enough for the "100%" it
		# can draw — 27px against a 14px trough — so the fill came out a bar
		# and a half taller than the canaleta the artwork paints and covered
		# its own coloured outline. Turning show_percentage off does not lower
		# that minimum. A plain Control with clip_contents cannot exceed the
		# trough whatever the theme thinks, which is the property actually
		# wanted here: the fill loads and empties *inside* the frame.
		var trough := Control.new()
		trough.position = Vector2(BAR_X, y)
		trough.custom_minimum_size = Vector2(BAR_W, BAR_H)
		trough.size = Vector2(BAR_W, BAR_H)
		trough.clip_contents = true
		trough.mouse_filter = Control.MOUSE_FILTER_IGNORE
		container.add_child(trough)

		var fill := ColorRect.new()
		fill.color = color
		fill.position = Vector2.ZERO
		fill.size = Vector2(BAR_W, BAR_H)
		fill.mouse_filter = Control.MOUSE_FILTER_IGNORE
		trough.add_child(fill)

		# The reading rides inside the same 14px, centred over the fill: the
		# trough is the only place on this panel with room for it, and a number
		# floating outside its bar belongs to nothing.
		var label := Label.new()
		label.position = Vector2.ZERO
		label.size = Vector2(BAR_W, BAR_H)
		label.horizontal_alignment = HORIZONTAL_ALIGNMENT_CENTER
		label.vertical_alignment = VERTICAL_ALIGNMENT_CENTER
		label.add_theme_color_override("font_color", Color.WHITE)
		label.add_theme_color_override("font_shadow_color", Color(0, 0, 0, 0.95))
		label.add_theme_constant_override("shadow_offset_x", 1)
		label.add_theme_constant_override("shadow_offset_y", 1)
		label.add_theme_font_size_override("font_size", BAR_FONT_SIZE)
		label.clip_text = true
		label.mouse_filter = Control.MOUSE_FILTER_IGNORE
		trough.add_child(label)
		# After add_child, and the offsets go with the anchors: the preset that
		# keeps its offsets is what pinned a whole screen at 0x0 once
		# (DIFICULTADES §13).
		label.set_anchors_and_offsets_preset(Control.PRESET_FULL_RECT)

		_bars[key] = {"fill": fill, "label": label, "caption": caption}


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


## The two plates in the title bar, both of them the way out of the match.
##
## The left one used to carry a little tree — a button from the template's own
## world with nothing behind it here — so the tree was painted out of the art
## and the plate now carries the word. The right one keeps the X the artwork
## draws and takes no caption of its own: an icon and its label side by side,
## rather than a symbol you have to hover to understand.
##
## Neither draws a plate of its own, since both are already painted (see
## _style_over_art). The X has no text to light up on hover, so it gets the
## only feedback available to a control that must not cover its own artwork:
## a translucent wash over it, dark when held. The caption on SALIR lights up
## the same way every other painted-plate caption on this panel does.
func _build_titlebar() -> void:
	_source_link.text = "código fuente · AGPL-3.0
github.com/juanCruzAldaya/ArgentumGonline"
	_source_link.add_theme_font_size_override("font_size", 9)
	_source_link.add_theme_color_override("font_color", COLOR_TEXT_DIM)
	_source_link.add_theme_color_override("font_shadow_color", Color(0, 0, 0, 0.95))
	_source_link.add_theme_constant_override("shadow_offset_x", 1)
	_source_link.add_theme_constant_override("shadow_offset_y", 1)
	_source_link.add_theme_constant_override("line_spacing", 0)

	_quit_button.text = "SALIR"
	_close_button.text = ""
	for b: Button in [_quit_button, _close_button]:
		_style_over_art(b)
		b.add_theme_font_size_override("font_size", 14)
		b.add_theme_stylebox_override("hover", _flat(Color(1, 1, 1, 0.09), Color(0, 0, 0, 0), 3, 0))
		b.add_theme_stylebox_override("pressed", _flat(Color(0, 0, 0, 0.30), Color(0, 0, 0, 0), 3, 0))
		b.tooltip_text = "Salir de la partida"
		b.pressed.connect(quit_requested.emit)


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
	for l: Label in [_char_name, _char_class, _agility_box, _strength_box, _kills_box, _coords]:
		l.add_theme_color_override("font_shadow_color", Color(0, 0, 0, 0.95))
		l.add_theme_constant_override("shadow_offset_x", 1)
		l.add_theme_constant_override("shadow_offset_y", 1)

	# The plaque is a lit plate on dark wood, so the name is the one place on
	# the panel bright enough to want the near-white; the line under it steps
	# down in both size and warmth so the two read as one engraving rather than
	# as two labels that happened to land on the same plate.
	_char_name.add_theme_color_override("font_color", Color("f0e6da"))
	_char_name.add_theme_font_size_override("font_size", NAME_FONT_SIZE)
	_char_class.add_theme_color_override("font_color", Color("a89a86"))
	_char_class.add_theme_font_size_override("font_size", CLASS_FONT_SIZE)

	for l: Label in [_agility_box, _strength_box, _kills_box, _coords]:
		l.add_theme_color_override("font_color", COLOR_TEXT)
		l.add_theme_font_size_override("font_size", 12)
	_kills_box.add_theme_color_override("font_color", COLOR_ACCENT)
	# The two attribute boxes are numbers to compare at a glance mid-fight, so
	# they get the accent and a size the small plates can still hold.
	for l: Label in [_agility_box, _strength_box]:
		l.add_theme_color_override("font_color", COLOR_ACCENT)
		l.add_theme_font_size_override("font_size", 15)


## The size a control was given in main.tscn, read off its offsets rather than
## its size — at _ready a container has not necessarily been laid out yet, but
## the offsets are whatever the scene file says the moment it loads, and those
## offsets *are* the measurement taken off the artwork.
func _hole_of(c: Control) -> Vector2:
	return Vector2(c.offset_right - c.offset_left, c.offset_bottom - c.offset_top)


func _flat(fill: Color, edge: Color, radius: int = 2, border: int = 1) -> StyleBoxFlat:
	var box := StyleBoxFlat.new()
	box.bg_color = fill
	box.border_color = edge
	box.set_border_width_all(border)
	box.set_corner_radius_all(radius)
	return box
