extends Control
## El panel de configurar teclas, con el arte horneado igual que el original.
##
## En AO clásico esto es frmCustomKeys, y no está armado como lo armaría un
## motor de juegos: el panel es una sola imagen con los textos ya pintados, y
## encima se posicionan los controles vivos, que caen en los agujeros que el
## arte dibuja. Es el mismo criterio que el login y el panel lateral de este
## juego (OPERACION §3), y la consecuencia es la misma: **cada offset de acá es
## una medición sobre el PNG**, no un número a ojo. Si cambia el arte, hay que
## volver a medir.
##
## Se abre con F1, y con Ctrl+0 —el atajo del original— fuera del browser: ahí
## Ctrl+0 es el zoom de la página y Chrome no lo suelta.
##
## Lo que se edita es una copia. Nada toca el InputMap hasta GUARDAR, así que
## salir con Escape deja todo como estaba: el original aplica en vivo y no te
## deja arrepentirte.

const ART := "res://assets/ao/ui/keys_bg.png"

## El espacio en el que están medidos todos los rects de abajo: el PNG horneado.
const PANEL_BASE := Vector2(1339, 898)
## Aire arriba y abajo. Más chico, panel más grande.
const PANEL_MARGIN := 24.0

## Un casillero entero, del bisel de arriba al de abajo. El hueco liso de
## adentro son los 34px del medio; el botón ocupa el casillero completo y su
## texto queda centrado en el hueco.
const BOX_SIZE := Vector2(279, 52)

## Dónde arranca el casillero de cada acción. Medido sobre keys_bg.png por el
## perfil de luminancia de cada columna, no a ojo: dos columnas, x=359 y x=998,
## renglones cada 62px salvo los dos de OTRAS TECLAS, que el arte separa 65.
const BOXES := {
	&"mover_norte": Vector2(359, 165),
	&"mover_sur": Vector2(359, 227),
	&"mover_oeste": Vector2(359, 289),
	&"mover_este": Vector2(359, 351),
	&"hablar": Vector2(359, 487),

	&"agarrar": Vector2(998, 165),
	&"equipar": Vector2(998, 227),
	&"ocultarse": Vector2(998, 289),
	&"tirar": Vector2(998, 351),
	&"usar": Vector2(998, 413),
	&"atacar": Vector2(998, 476),
	&"meditar": Vector2(998, 621),
	&"mapa": Vector2(998, 686),
}

## Las tres placas de abajo, que el arte ya dibuja con su texto.
const BUTTONS := {
	"guardar": Rect2(300, 808, 190, 58),
	"defecto": Rect2(579, 808, 194, 58),
	"cancelar": Rect2(861, 808, 183, 58),
}

## El hueco libre de la columna izquierda, debajo de HABLAR: es donde entra el
## renglón que dice qué está pasando sin taparle nada al arte.
const STATUS_RECT := Rect2(90, 560, 560, 40)

const COLOR_SCRIM := Color(0.02, 0.02, 0.03, 0.55)
const COLOR_TEXT := Color("e8dcc0")
const COLOR_MUTED := Color(0.72, 0.66, 0.54)
const COLOR_LISTENING := Color("f0c869")
const COLOR_CLASH := Color(0.90, 0.42, 0.36)

var _pending: Dictionary = {}
var _capturing: StringName = &""

var _art: TextureRect
var _buttons: Dictionary = {}
var _status: Label
var _scale := 1.0


func _ready() -> void:
	visible = false
	set_anchors_preset(Control.PRESET_FULL_RECT)
	mouse_filter = Control.MOUSE_FILTER_STOP
	_build()
	get_viewport().size_changed.connect(_layout)


func open() -> void:
	# La copia se saca al abrir: entre una apertura y la siguiente el jugador
	# pudo haber cambiado cosas.
	_pending = KeyBindings.snapshot()
	_capturing = &""
	_layout()
	_refresh()
	_say("Clic en una tecla para cambiarla.", COLOR_MUTED)
	visible = true


func close() -> void:
	_capturing = &""
	visible = false


func _build() -> void:
	var scrim := ColorRect.new()
	scrim.color = COLOR_SCRIM
	scrim.set_anchors_preset(Control.PRESET_FULL_RECT)
	scrim.mouse_filter = Control.MOUSE_FILTER_IGNORE
	add_child(scrim)

	_art = TextureRect.new()
	_art.texture = load(ART)
	# El proyecto tiene el filtro en nearest, que es lo correcto para los
	# sprites del mundo y lo incorrecto acá: esto es arte pintado y la escala no
	# es entera, así que en nearest los biseles se convierten en una escalera.
	_art.texture_filter = CanvasItem.TEXTURE_FILTER_LINEAR
	_art.mouse_filter = Control.MOUSE_FILTER_IGNORE
	add_child(_art)

	for action in BOXES:
		var button := Button.new()
		button.flat = true # el casillero ya está pintado en el arte
		button.focus_mode = Control.FOCUS_NONE
		button.pressed.connect(_start_capture.bind(action))
		_art.add_child(button)
		_buttons[action] = button

	for name in BUTTONS:
		var plate := Button.new()
		plate.flat = true
		plate.focus_mode = Control.FOCUS_NONE
		_art.add_child(plate)
		_buttons[name] = plate
	_buttons["guardar"].pressed.connect(_on_save)
	_buttons["defecto"].pressed.connect(_on_defaults)
	_buttons["cancelar"].pressed.connect(close)

	_status = Label.new()
	_status.horizontal_alignment = HORIZONTAL_ALIGNMENT_CENTER
	_art.add_child(_status)


## Un solo número decide todo: arte y controles no se pueden desalinear porque
## las posiciones salen de multiplicar los rects medidos por la misma escala.
func _layout() -> void:
	var screen := get_viewport_rect().size
	_scale = minf(
		(screen.y - PANEL_MARGIN * 2.0) / PANEL_BASE.y,
		(screen.x - PANEL_MARGIN * 2.0) / PANEL_BASE.x
	)
	var panel := PANEL_BASE * _scale
	_art.size = panel
	_art.position = ((screen - panel) / 2.0).floor()

	for action in BOXES:
		_place(_buttons[action], Rect2(BOXES[action], BOX_SIZE))
	for name in BUTTONS:
		_place(_buttons[name], BUTTONS[name])

	_place(_status, STATUS_RECT)
	_status.add_theme_font_size_override("font_size", int(15 * _scale))


func _place(control: Control, rect: Rect2) -> void:
	control.position = (rect.position * _scale).floor()
	control.size = (rect.size * _scale).floor()
	if control is Button:
		control.add_theme_font_size_override("font_size", int(16 * _scale))


func _start_capture(action: StringName) -> void:
	_capturing = action
	_refresh()
	_say("Apretá una tecla. Escape cancela.", COLOR_MUTED)


## Toda la captura pasa por acá y no por _gui_input, porque el jugador puede
## apretar cualquier cosa —incluidas teclas que ningún control quiere— y porque
## marcar el evento como consumido es lo que evita que la tecla que está
## eligiendo camine al personaje al mismo tiempo.
func _input(event: InputEvent) -> void:
	if not visible or _capturing == &"":
		return
	if not (event is InputEventKey and event.pressed and not event.echo):
		return
	get_viewport().set_input_as_handled()

	var key: int = (event as InputEventKey).physical_keycode
	if key == KEY_ESCAPE:
		# Escape cancela en vez de asignarse. Es lo único en lo que nos
		# apartamos del original, y es a favor: sin una salida, una captura mal
		# empezada te deja encerrado en el panel.
		_capturing = &""
		_say("Cancelado.", COLOR_MUTED)
		_refresh()
		return

	var clash := _who_uses(key, _capturing)
	if clash != &"":
		# El clásico borra el casillero del otro y hace Beep; el moderno pinta
		# rojo. Acá se rechaza y se dice quién la tiene: un beep en el browser
		# no existe, y borrarle la tecla a otra acción en silencio es peor que
		# no cambiar nada.
		_say("Esa tecla ya la usa: %s" % _label_of(clash), COLOR_CLASH)
		_capturing = &""
		_refresh()
		return

	_pending[_capturing] = key
	_capturing = &""
	_refresh()
	_say("Listo. GUARDAR para que quede.", COLOR_TEXT)


func _who_uses(key: int, except_action: StringName) -> StringName:
	for action in _pending:
		if action != except_action and int(_pending[action]) == key:
			return action
	return &""


func _label_of(action: StringName) -> String:
	for entry in KeyBindings.ACTIONS:
		if entry["action"] == action:
			return String(entry["label"])
	return String(action)


func _refresh() -> void:
	for action in BOXES:
		var button: Button = _buttons[action]
		if action == _capturing:
			button.text = "..."
			button.add_theme_color_override("font_color", COLOR_LISTENING)
		else:
			button.text = KeyBindings.key_name(int(_pending.get(action, KEY_NONE)))
			button.add_theme_color_override("font_color", COLOR_TEXT)
		button.add_theme_color_override("font_hover_color", COLOR_LISTENING)


func _say(text: String, color: Color) -> void:
	_status.text = text
	_status.add_theme_color_override("font_color", color)


func _on_save() -> void:
	KeyBindings.apply(_pending)
	close()


func _on_defaults() -> void:
	_pending = KeyBindings.defaults_snapshot()
	_capturing = &""
	_refresh()
	_say("Teclas de fábrica. GUARDAR para que quede.", COLOR_TEXT)
