extends Control
## Signing in, and the career that comes back when it works.
##
## Two states in one screen because they are two halves of one moment: you type
## a name and a password, and what you get for it is your record. Splitting them
## into separate scenes would mean a screen whose only content is "ok" before
## the one you actually wanted.
##
## It is shown before the character picker and before the world exists, and only
## on a server that asked for it — see main.gd, which reads the hello. A server
## without accounts never draws this at all.

## Asks the server to sign in or sign up, depending on new_account.
signal login_submitted(account: String, password: String, new_account: bool)
signal play_requested
signal quit_requested

const COLOR_BG := Color("0b0805")
const COLOR_PANEL := Color("14100c")
const COLOR_TROUGH := Color("0c0a08")
const COLOR_TEXT := Color("ddd0b4")
const COLOR_ACCENT := Color("d9b45b")
const COLOR_MUTED := Color("9c8f78")
const COLOR_ERROR := Color("e06c5a")
const COLOR_WIN := Color("9ee08a")

const PANEL := Vector2(520, 470)

var _min_password := 6

var _name_field: LineEdit
var _pass_field: LineEdit
var _message: Label
var _enter: Button
var _create: Button
var _play: Button
var _title: Label
var _name_label: Label
var _pass_label: Label
var _card: Control

var _account: Dictionary = {}


func _ready() -> void:
	# set_anchors_and_offsets_preset, not set_anchors_preset. The second
	# argument of the latter is keep_offsets and it defaults to TRUE: called
	# after add_child, when the node is already in the tree at 0x0, Godot
	# preserves that rect — anchors go full-screen and the offsets are
	# compensated so the size does not change. The node stays 0x0 forever and
	# the whole screen draws into a small block in the top-left corner.
	#
	# This is DIFICULTADES §13, which cost two sessions the first time it
	# happened, to character_picker.gd, on this exact line. It happened again
	# here because the idiom is the same one; the note is now on the line
	# itself so the next person copying this file copies the fix with it.
	set_anchors_and_offsets_preset(Control.PRESET_FULL_RECT)
	mouse_filter = Control.MOUSE_FILTER_STOP
	_build()
	# And the layout is redone whenever the size actually arrives. Even with
	# the preset right, _ready runs before the first layout pass, so the size
	# read below is still 0 on this frame — positioning once here would put
	# everything at a corner of a rectangle that does not exist yet.
	resized.connect(_layout)
	_layout()


## configure takes the server's hello, which carries the password floor so the
## client can refuse a short one without spending a round trip on it.
func configure(hello: Dictionary) -> void:
	_min_password = int(hello.get("minpass", _min_password))


func _panel_rect() -> Rect2:
	return Rect2(((size - PANEL) * 0.5).floor(), PANEL)


## _build only creates the controls. Where they go is _layout's problem, and it
## is a separate problem because the answer changes: the window can be resized,
## and the size is not known yet on the frame this runs.
func _build() -> void:
	_title = _label("CUENTA", 22, COLOR_ACCENT)

	_name_label = _label("Nombre", 12, COLOR_MUTED)
	_name_field = _field(false)
	_name_field.placeholder_text = "3 a 16 letras o números"

	_pass_label = _label("Contraseña", 12, COLOR_MUTED)
	_pass_field = _field(true)
	_pass_field.placeholder_text = "al menos %d caracteres" % _min_password

	_message = _label("", 12, COLOR_ERROR)
	_message.autowrap_mode = TextServer.AUTOWRAP_WORD_SMART

	_enter = _button("Entrar")
	_enter.pressed.connect(func() -> void: _submit(false))

	_create = _button("Crear cuenta")
	_create.pressed.connect(func() -> void: _submit(true))

	# The career, hidden until there is one. It is a flag rather than a node
	# with children: everything in it is drawn in _draw.
	_card = Control.new()
	_card.visible = false
	_card.mouse_filter = Control.MOUSE_FILTER_IGNORE
	add_child(_card)

	_play = _button("Jugar")
	_play.visible = false
	_play.pressed.connect(func() -> void: play_requested.emit())

	# Enter submits, which is what anyone typing a password expects.
	_name_field.text_submitted.connect(func(_t: String) -> void: _pass_field.grab_focus())
	_pass_field.text_submitted.connect(func(_t: String) -> void: _submit(false))
	_name_field.grab_focus()


## _layout puts everything where the panel currently is. Called on every resize,
## so the screen follows the window instead of being placed once against a size
## that was still zero.
func _layout() -> void:
	if _title == null:
		return
	var at := _panel_rect()
	var wide := PANEL.x - 56

	_title.position = at.position + Vector2(28, 24)
	_title.size = Vector2(wide, 30)
	_name_label.position = at.position + Vector2(28, 78)
	_name_field.position = at.position + Vector2(28, 98)
	_name_field.size = Vector2(wide, 34)
	_pass_label.position = at.position + Vector2(28, 144)
	_pass_field.position = at.position + Vector2(28, 164)
	_pass_field.size = Vector2(wide, 34)
	_message.position = at.position + Vector2(28, 206)
	_message.size = Vector2(wide, 40)
	_enter.position = at.position + Vector2(28, 252)
	_enter.size = Vector2(200, 38)
	_create.position = at.position + Vector2(PANEL.x - 228, 252)
	_create.size = Vector2(200, 38)
	_play.position = at.position + Vector2((PANEL.x - 220) * 0.5, PANEL.y - 66)
	_play.size = Vector2(220, 42)
	queue_redraw()


func _label(text: String, size_px: int, color: Color) -> Label:
	var label := Label.new()
	label.text = text
	label.add_theme_font_size_override("font_size", size_px)
	label.add_theme_color_override("font_color", color)
	add_child(label)
	return label


func _field(secret: bool) -> LineEdit:
	var field := LineEdit.new()
	field.secret = secret
	field.add_theme_color_override("font_color", COLOR_TEXT)
	field.add_theme_color_override("font_placeholder_color", COLOR_MUTED)
	var box := StyleBoxFlat.new()
	box.bg_color = COLOR_TROUGH
	box.border_color = COLOR_ACCENT * Color(1, 1, 1, 0.35)
	box.set_border_width_all(1)
	box.content_margin_left = 8
	field.add_theme_stylebox_override("normal", box)
	field.add_theme_stylebox_override("focus", box)
	add_child(field)
	return field


func _button(text: String) -> Button:
	var button := Button.new()
	button.text = text
	button.add_theme_color_override("font_color", COLOR_TEXT)
	button.add_theme_font_size_override("font_size", 14)
	var box := StyleBoxFlat.new()
	box.bg_color = COLOR_PANEL
	box.border_color = COLOR_ACCENT * Color(1, 1, 1, 0.5)
	box.set_border_width_all(1)
	button.add_theme_stylebox_override("normal", box)
	button.add_theme_stylebox_override("hover", box)
	button.add_theme_stylebox_override("pressed", box)
	add_child(button)
	return button


## _submit checks what it can locally before asking the server. A password the
## server is going to refuse for being short costs a round trip and reads as a
## server problem; refusing it here reads as a form.
func _submit(new_account: bool) -> void:
	var account := _name_field.text.strip_edges()
	var password := _pass_field.text

	if account.length() < 3 or account.length() > 16:
		_fail("El nombre tiene que tener entre 3 y 16 caracteres.")
		return
	if new_account and password.length() < _min_password:
		_fail("La contraseña necesita al menos %d caracteres." % _min_password)
		return
	if password.is_empty():
		_fail("Falta la contraseña.")
		return

	_message.add_theme_color_override("font_color", COLOR_MUTED)
	_message.text = "Creando la cuenta..." if new_account else "Entrando..."
	_set_busy(true)
	login_submitted.emit(account, password, new_account)


func _fail(reason: String) -> void:
	_set_busy(false)
	_message.add_theme_color_override("font_color", COLOR_ERROR)
	_message.text = reason


## rejected is the server's own wording, shown as it came: it is the side that
## knows whether the name is taken or the password is wrong, and inventing a
## friendlier sentence here would risk saying something untrue.
func rejected(reason: String) -> void:
	_fail(reason)
	_pass_field.grab_focus()
	_pass_field.select_all()


func _set_busy(busy: bool) -> void:
	_enter.disabled = busy
	_create.disabled = busy


## show_account swaps the form for the career.
func show_account(account: Dictionary) -> void:
	_account = account
	_set_busy(false)
	_message.text = ""

	for control in [_name_field, _pass_field, _enter, _create]:
		control.visible = false
	_title.text = str(account.get("name", "")).to_upper()
	_card.visible = true
	_play.visible = true
	_play.grab_focus()
	queue_redraw()


func _seconds(total: float) -> String:
	var whole := int(maxf(total, 0.0))
	if whole >= 3600:
		return "%dh %dm" % [whole / 3600, (whole % 3600) / 60]
	return "%dm %02ds" % [whole / 60, whole % 60]


func _draw() -> void:
	draw_rect(Rect2(Vector2.ZERO, size), COLOR_BG)
	var at := _panel_rect()
	draw_rect(at, COLOR_PANEL)
	draw_rect(at, COLOR_ACCENT * Color(1, 1, 1, 0.55), false, 2.0)

	if not _card.visible:
		return

	var font := ThemeDB.fallback_font
	var matches := int(_account.get("matches", 0))
	var wins := int(_account.get("wins", 0))
	var best := int(_account.get("best", 0))

	# The four numbers that answer "how am I doing", in a row.
	var stats := [
		["PARTIDAS", str(matches)],
		["VICTORIAS", str(wins)],
		["BAJAS", str(int(_account.get("kills", 0)))],
		["MEJOR", "—" if best <= 0 else ("#%d" % best)],
	]
	var col := (PANEL.x - 56) / float(stats.size())
	for i in stats.size():
		var centre := at.position.x + 28 + col * (float(i) + 0.5)
		var label: String = stats[i][0]
		var value: String = stats[i][1]
		var color: Color = COLOR_WIN if (i == 1 and wins > 0) else COLOR_TEXT
		_centred(font, label, Vector2(centre, at.position.y + 90), 11, COLOR_MUTED)
		_centred(font, value, Vector2(centre, at.position.y + 122), 26, color)

	_centred(
		font,
		"sobreviviste %s en total" % _seconds(float(_account.get("secs", 0.0))),
		Vector2(at.position.x + PANEL.x * 0.5, at.position.y + 152),
		12,
		COLOR_MUTED
	)

	# The history, newest first.
	var recent: Array = _account.get("recent", [])
	var top := at.position.y + 190
	if recent.is_empty():
		_centred(
			font, "Todavía no jugaste ninguna partida.",
			Vector2(at.position.x + PANEL.x * 0.5, top + 20), 13, COLOR_MUTED
		)
		return

	draw_string(
		font, Vector2(at.position.x + 28, top), "ÚLTIMAS PARTIDAS",
		HORIZONTAL_ALIGNMENT_LEFT, -1, 11, COLOR_MUTED
	)
	var row := top + 24.0
	for i in mini(recent.size(), 6):
		var m: Dictionary = recent[i]
		var place := int(m.get("place", 0))
		var won: bool = bool(m.get("won", false))
		var line := "#%d de %d" % [place, int(m.get("of", 0))]
		var detail := "%d bajas · %s · %s" % [
			int(m.get("kills", 0)), _seconds(float(m.get("secs", 0.0))), str(m.get("map", "—"))
		]
		draw_string(
			font, Vector2(at.position.x + 28, row), line,
			HORIZONTAL_ALIGNMENT_LEFT, -1, 14, COLOR_WIN if won else COLOR_TEXT
		)
		draw_string(
			font, Vector2(at.position.x + 130, row), detail,
			HORIZONTAL_ALIGNMENT_LEFT, -1, 12, COLOR_MUTED
		)
		row += 22.0


func _centred(font: Font, text: String, at: Vector2, size_px: int, color: Color) -> void:
	var half := font.get_string_size(text, HORIZONTAL_ALIGNMENT_LEFT, -1, size_px).x * 0.5
	draw_string(font, Vector2(at.x - half, at.y), text, HORIZONTAL_ALIGNMENT_LEFT, -1, size_px, color)
