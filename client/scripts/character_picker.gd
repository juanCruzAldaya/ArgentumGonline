extends Control
## The pre-game login/character-creation screen: nickname, class and race,
## then play — or leave.
##
## Battle royale means nobody grinds into a build — see server/internal/world
## /balance.go's maxLevel — so this is the only character-creation step there
## is. Shown full-screen before the world/HUD/net ever connect to anything;
## main.gd frees this node once confirmed and only then dials the server.
##
## The background is real art (login_bg.png, 855x756), not primitives —
## StyleBoxFlat has no bevel, texture or carved-bone look, and reproducing that
## by hand was tried and abandoned once already for the in-game panel (see
## DIFICULTADES.md §2). Every field below sits inside a hole the art already
## draws — nickname trough, class/race dropdown troughs, the two button
## plates — measured off the source PNG the same way main.tscn's SidePanel
## offsets were, not eyeballed.

signal confirmed(player_name: String, class_id: int, race_id: int)

const BG_PATH := "res://assets/ao/ui/login_bg.png"

## Native size of login_bg.png. Every rect and font size below is expressed in
## this space and multiplied by _scale on the way out, so the art and the live
## controls are physically incapable of drifting apart: there is exactly one
## number that decides how big the panel is.
const PANEL_BASE := Vector2(855, 756)
## Breathing room left above and below the panel once it is scaled to the
## screen. Smaller number, bigger panel.
const PANEL_MARGIN := 32.0

const COLOR_BG := Color("0b0805")
const COLOR_TROUGH := Color("0c0a08")
const COLOR_TEXT := Color("ddd0b4")
const COLOR_ACCENT := Color("d9b45b")
const COLOR_PLACEHOLDER := Color("9c8f78")

## Field rects, measured off login_bg.png (855x756) by profiling the luminance
## across each trough's bevel rather than by eye — the trough interiors and
## the wood around them are within a few levels of each other, so a plain
## dark-pixel threshold finds the whole panel, not the hole. These are the
## interiors: a control sized to one of them lands inside the bevel instead of
## spilling over it, which is what the first pass got wrong.
const NICKNAME_RECT := Rect2(112, 220, 608, 44)
const CLASE_RECT := Rect2(112, 336, 608, 38)
const RAZA_RECT := Rect2(112, 444, 608, 38)
## The two button plates, measured to the outer edge of their metal frame, so
## the hover tint stops exactly where the plate does.
const CREAR_RECT := Rect2(279, 684, 263, 46)
const SALIR_RECT := Rect2(660, 689, 141, 36)

const NICKNAME_MAX_LENGTH := 16

## Set by main.gd before this node enters the tree, so the field starts
## pre-filled with whatever --name=/JUEGITO_NAME/random default it already
## resolved — editable here rather than silently used, since asking for a
## nickname is the point.
var default_nickname := ""

## How much the panel is blown up from PANEL_BASE. Resolved once in _ready:
## the stretch mode is canvas_items, so the visible rect stays 1613x962 no
## matter how the OS window is resized, and this never needs recomputing.
var _scale := 1.0

var _nickname: LineEdit
var _class_option: OptionButton
var _race_option: OptionButton
var _play_button: Button


func _ready() -> void:
	# set_anchors_preset() keeps the current rect by default — keep_offsets
	# defaults to *true* — and by the time _ready runs this node is already in
	# the tree at 0x0, so that call pinned the picker to zero size and only the
	# panel's bottom-right quadrant ever reached the screen. See
	# DIFICULTADES.md §13. The offsets have to be reset along with the anchors.
	set_anchors_and_offsets_preset(Control.PRESET_FULL_RECT)

	var screen := get_viewport_rect().size
	_scale = maxf(1.0, minf(
		(screen.y - PANEL_MARGIN * 2.0) / PANEL_BASE.y,
		(screen.x - PANEL_MARGIN * 2.0) / PANEL_BASE.x,
	))
	var panel_size := PANEL_BASE * _scale

	var bg := ColorRect.new()
	bg.color = COLOR_BG
	bg.set_anchors_preset(Control.PRESET_FULL_RECT)
	bg.mouse_filter = Control.MOUSE_FILTER_IGNORE
	add_child(bg)

	var panel := Control.new()
	panel.set_anchors_preset(Control.PRESET_CENTER)
	panel.offset_left = -panel_size.x / 2.0
	panel.offset_right = panel_size.x / 2.0
	panel.offset_top = -panel_size.y / 2.0
	panel.offset_bottom = panel_size.y / 2.0
	add_child(panel)

	var art := TextureRect.new()
	art.texture = load(BG_PATH)
	art.set_anchors_preset(Control.PRESET_FULL_RECT)
	art.mouse_filter = Control.MOUSE_FILTER_IGNORE
	# The project default is nearest filtering, which is right for the world's
	# pixel art and wrong here: login_bg.png is painted, not pixel art, and
	# _scale is not an integer — nearest turns its bevels into a staircase.
	art.texture_filter = CanvasItem.TEXTURE_FILTER_LINEAR
	panel.add_child(art)

	_nickname = _build_nickname(panel)
	_class_option = _build_dropdown(panel, CLASE_RECT, "[Seleccionar Clase]", AOData.CLASS_NAMES)
	_race_option = _build_dropdown(panel, RAZA_RECT, "[Seleccionar Raza]", AOData.RACE_NAMES)
	_play_button = _build_action_button(panel, CREAR_RECT, _on_play_pressed)
	_build_action_button(panel, SALIR_RECT, _on_exit_pressed)

	_class_option.item_selected.connect(func(_i: int) -> void: _revalidate())
	_race_option.item_selected.connect(func(_i: int) -> void: _revalidate())
	_nickname.text_changed.connect(func(_t: String) -> void: _revalidate())
	_nickname.text_submitted.connect(func(_t: String) -> void: _try_play())
	_revalidate()


## Places a control inside one of the measured rects, converting from the art's
## own 855x756 space into the scaled panel.
func _place(control: Control, rect: Rect2) -> void:
	control.set_position(rect.position * _scale)
	control.set_size(rect.size * _scale)


## Font sizes are art-space numbers too, and get rounded into a real font size
## rather than applied as a transform, so the glyphs stay crisp instead of
## being rasterized small and then stretched.
func _font(base: int) -> int:
	return int(round(base * _scale))


func _build_nickname(panel: Control) -> LineEdit:
	var field := LineEdit.new()
	_place(field, NICKNAME_RECT)
	field.text = default_nickname
	field.max_length = NICKNAME_MAX_LENGTH
	field.caret_blink = true
	field.add_theme_stylebox_override("normal", _flat(COLOR_TROUGH))
	field.add_theme_stylebox_override("focus", _flat(COLOR_TROUGH))
	field.add_theme_color_override("font_color", COLOR_TEXT)
	field.add_theme_color_override("caret_color", COLOR_ACCENT)
	field.add_theme_font_size_override("font_size", _font(18))
	field.add_theme_constant_override("minimum_character_width", 0)
	panel.add_child(field)
	field.grab_focus()
	field.caret_column = field.text.length()
	return field


## The art already bakes "[Seleccionar Clase]" / "[Seleccionar Raza]" into the
## trough, so the placeholder is a real disabled item (id -1) rather than a
## label trick — get_selected_id() then doubles as the validity check, and the
## returned id is the class/race id itself, no index math needed. The opaque
## trough fill is what hides the baked copy underneath; drop it and the two
## texts print on top of each other.
func _build_dropdown(panel: Control, rect: Rect2, placeholder: String, names: Array) -> OptionButton:
	var option := OptionButton.new()
	_place(option, rect)
	option.alignment = HORIZONTAL_ALIGNMENT_LEFT
	option.add_item(placeholder, -1)
	option.set_item_disabled(0, true)
	for i in names.size():
		option.add_item(names[i], i)
	option.select(0)

	var blank := ImageTexture.create_from_image(Image.create(1, 1, false, Image.FORMAT_RGBA8))
	option.add_theme_icon_override("arrow", blank)
	option.add_theme_constant_override("h_separation", 0)
	option.add_theme_stylebox_override("normal", _flat(COLOR_TROUGH))
	option.add_theme_stylebox_override("hover", _flat(COLOR_TROUGH.lightened(0.06)))
	option.add_theme_stylebox_override("pressed", _flat(COLOR_TROUGH))
	option.add_theme_stylebox_override("disabled", _flat(COLOR_TROUGH))
	option.add_theme_color_override("font_color", COLOR_TEXT)
	option.add_theme_color_override("font_disabled_color", COLOR_PLACEHOLDER)
	option.add_theme_font_size_override("font_size", _font(16))

	var popup := option.get_popup()
	popup.add_theme_stylebox_override("panel", _flat(Color("120d09"), 8))
	popup.add_theme_color_override("font_color", COLOR_TEXT)
	popup.add_theme_color_override("font_hover_color", Color.BLACK)
	popup.add_theme_stylebox_override("hover", _flat(COLOR_ACCENT, 8))
	popup.add_theme_font_size_override("font_size", _font(16))

	panel.add_child(option)
	return option


## Buttons stay visually transparent in their normal state — CREAR PERSONAJE
## and SALIR are already lettered into the art, so drawing our own label on
## top would double them up. Hover/pressed/disabled get a subtle tint instead
## of a swapped stylebox, so the baked bevel and glow keep showing through.
func _build_action_button(panel: Control, rect: Rect2, on_pressed: Callable) -> Button:
	var button := Button.new()
	_place(button, rect)
	button.focus_mode = Control.FOCUS_NONE
	button.add_theme_stylebox_override("normal", StyleBoxEmpty.new())
	button.add_theme_stylebox_override("hover", _flat(Color(1, 0.9, 0.6, 0.12), 6, 0))
	button.add_theme_stylebox_override("pressed", _flat(Color(0, 0, 0, 0.28), 6, 0))
	button.add_theme_stylebox_override("disabled", _flat(Color(0, 0, 0, 0.55), 6, 0))
	button.pressed.connect(on_pressed)
	panel.add_child(button)
	return button


## CREAR PERSONAJE stays disabled until all three fields are actually chosen —
## the art's own placeholder text ("[Seleccionar Clase]") is a real prompt,
## not decoration, so this honors it instead of defaulting to item 0 the way
## the previous picker's always-ready ItemLists did.
func _revalidate() -> void:
	_play_button.disabled = not _is_valid()


func _is_valid() -> bool:
	return (
		_nickname.text.strip_edges() != ""
		and _class_option.get_selected_id() >= 0
		and _race_option.get_selected_id() >= 0
	)


func _try_play() -> void:
	if _is_valid():
		_on_play_pressed()
	else:
		_nickname.grab_focus()


func _on_play_pressed() -> void:
	if not _is_valid():
		return
	confirmed.emit(_nickname.text.strip_edges(), _class_option.get_selected_id(), _race_option.get_selected_id())


## Native quits outright. The web export cannot close a tab it did not open
## itself — no browser allows a page to do that — so window.close() is
## attempted anyway (it works in embedded/kiosk contexts) and silently no-ops
## everywhere else, which is the best any client-side script can do here.
func _on_exit_pressed() -> void:
	if OS.has_feature("web"):
		JavaScriptBridge.eval("window.close()", true)
	else:
		get_tree().quit()


## Radius and padding are art-space numbers like everything else. pad_left is
## where the text starts inside a trough: 14 puts it on the same column the
## art's own baked placeholder sits at. The buttons pass 0 — they draw no text
## of their own to inset.
func _flat(fill: Color, radius: int = 4, pad_left: int = 14) -> StyleBoxFlat:
	var box := StyleBoxFlat.new()
	box.bg_color = fill
	box.set_corner_radius_all(int(round(radius * _scale)))
	box.set_content_margin_all(round(6 * _scale))
	box.content_margin_left = round(pad_left * _scale)
	return box
