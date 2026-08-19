extends Control
## El campamento: la cola para la próxima partida, y tu carrera al lado.
##
## Es la pantalla que existe porque antes no había ninguna. Una partida empezaba
## cuando arrancaba el proceso del servidor, así que el primero en conectarse
## caminaba solo por un mundo que ya se estaba cerrando, y juntar gente para una
## partida llena era imposible: la única forma de entrar era llegar antes que
## los demás.
##
## Sobre el arte (lobby_bg.png) se sacaron del PNG las partes que describían un
## juego que todavía no existe — tipo de partida, tiempo estimado, K/D, rango,
## historial completo, inventario, tienda y social. La regla es la del resto del
## proyecto: si no hay nada atrás, no se dibuja. El JPG original queda entero
## para el día que sí lo haya.

## Pide entrar o salir de la cola. El servidor decide cuándo eso alcanza para
## empezar una partida; esta pantalla solo dice qué quiere el jugador.
signal queue_toggled(join: bool)
signal quit_requested

const BG_PATH := "res://assets/ao/ui/lobby_bg.png"

const PANEL_BASE := Vector2(992, 898)
const PANEL_MARGIN := 32.0

const COLOR_BG := Color("0b0805")
const COLOR_TEXT := Color("ddd0b4")
const COLOR_ACCENT := Color("d9b45b")
const COLOR_MUTED := Color("9c8f78")
const COLOR_WIN := Color("9ee08a")

## La placa de UNIRSE A LA COLA, medida al borde exterior de su marco.
const COLA_RECT := Rect2(129, 335, 183, 74)
## Debajo de la placa: qué hace tocarla ahora. Hace falta porque la placa está
## letreada en el arte y su etiqueta no puede cambiar — el único control de este
## proyecto cuyo significado tiene dos estados.
const COLA_HINT_RECT := Rect2(104, 414, 237, 44)
## Donde el arte dice ESTADO:, debajo del rótulo horneado.
const ESTADO_RECT := Rect2(104, 548, 237, 58)
## Las cuatro filas del perfil, y la lista de últimas partidas.
const PERFIL_RECT := Rect2(683, 328, 214, 120)
const ULTIMAS_RECT := Rect2(683, 566, 214, 188)
const SALIR_RECT := Rect2(779, 817, 155, 39)

var _scale := 1.0
var _account: Dictionary = {}
var _state: Dictionary = {}
var _queued := false

var _estado: Label
var _hint: Label
var _cola: Button
var _ink: Control


func configure(_hello: Dictionary) -> void:
	pass


func _ready() -> void:
	# Ver DIFICULTADES §13.
	set_anchors_and_offsets_preset(Control.PRESET_FULL_RECT)
	mouse_filter = Control.MOUSE_FILTER_STOP

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
	art.texture_filter = CanvasItem.TEXTURE_FILTER_LINEAR
	panel.add_child(art)

	# El dibujo del perfil y de la lista va en un nodo propio encima del arte,
	# para que _draw tenga el mismo origen que los rects medidos sobre el PNG.
	var ink := Control.new()
	ink.set_anchors_preset(Control.PRESET_FULL_RECT)
	ink.mouse_filter = Control.MOUSE_FILTER_IGNORE
	ink.draw.connect(_draw_career.bind(ink))
	panel.add_child(ink)
	_ink = ink

	_cola = _plate(panel, COLA_RECT, _on_cola_pressed)
	_plate(panel, SALIR_RECT, func() -> void: quit_requested.emit())

	_hint = _text(panel, COLA_HINT_RECT, "", 12, COLOR_MUTED)
	_hint.horizontal_alignment = HORIZONTAL_ALIGNMENT_CENTER
	_hint.autowrap_mode = TextServer.AUTOWRAP_WORD_SMART

	_estado = _text(panel, ESTADO_RECT, "", 14, COLOR_TEXT)
	_estado.horizontal_alignment = HORIZONTAL_ALIGNMENT_CENTER
	_estado.autowrap_mode = TextServer.AUTOWRAP_WORD_SMART

	_refresh()


## set_account guarda la carrera que ya venía llegando al entrar. La pantalla la
## dibuja entera: el arte de las cuatro filas del perfil se horneó afuera
## justamente porque los números son de acá.
func set_account(account: Dictionary) -> void:
	_account = account
	if _ink != null:
		_ink.queue_redraw()


## set_lobby es el estado de la cola, que llega una vez por tick mientras se
## espera. Deja de llegar al entrar a la partida, y vuelve al terminar.
func set_lobby(state: Dictionary) -> void:
	_state = state
	_queued = bool(state.get("mine", false))
	_refresh()


func _on_cola_pressed() -> void:
	# El servidor manda: esto pide, y el próximo estado que llegue confirma.
	queue_toggled.emit(not _queued)


func _refresh() -> void:
	if _estado == null:
		return

	var queued := int(_state.get("q", 0))
	var needed := int(_state.get("need", 0))
	var running := bool(_state.get("run", false))
	var playing := int(_state.get("play", 0))

	if running:
		# Esperar a que termine la que está jugándose es el caso normal en un
		# servidor con gente. Una cola que solo supiera decir "faltan 3" estaría
		# mintiendo sobre qué está esperando.
		_estado.text = "PARTIDA EN CURSO\n%d jugando" % playing
		_estado.add_theme_color_override("font_color", COLOR_MUTED)
	elif bool(_state.get("c", false)):
		_estado.text = "EMPIEZA EN %d\n%d en la cola" % [ceili(float(_state.get("t", 0.0))), queued]
		_estado.add_theme_color_override("font_color", COLOR_ACCENT)
	elif _queued:
		_estado.text = "BUSCANDO...\n%d de %d" % [queued, needed]
		_estado.add_theme_color_override("font_color", COLOR_TEXT)
	else:
		_estado.text = "%d en la cola" % queued
		_estado.add_theme_color_override("font_color", COLOR_MUTED)

	if running:
		_hint.text = "podés encolarte para la próxima"
	elif _queued:
		_hint.text = "tocá de nuevo para salir de la cola"
	else:
		_hint.text = ""

	# La placa se apaga un poco mientras estás adentro, que es lo único que
	# puede decir "ya estás" sin poder cambiar lo que dice el arte.
	_cola.modulate = Color(0.72, 0.72, 0.72) if _queued else Color.WHITE


func _draw_career(ink: Control) -> void:
	var font := ThemeDB.fallback_font
	var at := PERFIL_RECT.position * _scale
	var wide := PERFIL_RECT.size.x * _scale
	var row := 30.0 * _scale

	var best := int(_account.get("best", 0))
	var stats := [
		["VICTORIAS", str(int(_account.get("wins", 0))), int(_account.get("wins", 0)) > 0],
		["PARTIDAS", str(int(_account.get("matches", 0))), false],
		["BAJAS", str(int(_account.get("kills", 0))), false],
		["MEJOR", "—" if best <= 0 else ("#%d" % best), best == 1],
	]
	for i in stats.size():
		var label: String = stats[i][0]
		var value: String = stats[i][1]
		var good: bool = stats[i][2]
		var y := at.y + row * (float(i) + 0.8)
		ink.draw_string(
			font, Vector2(at.x, y), label,
			HORIZONTAL_ALIGNMENT_LEFT, -1, int(round(14 * _scale)), COLOR_MUTED
		)
		var size := font.get_string_size(value, HORIZONTAL_ALIGNMENT_LEFT, -1, int(round(16 * _scale)))
		ink.draw_string(
			font, Vector2(at.x + wide - size.x, y), value,
			HORIZONTAL_ALIGNMENT_LEFT, -1, int(round(16 * _scale)),
			COLOR_WIN if good else COLOR_TEXT
		)

	# Las últimas partidas, la más nueva arriba.
	var list := ULTIMAS_RECT.position * _scale
	ink.draw_string(
		font, Vector2(list.x, list.y + 12 * _scale), "ÚLTIMAS PARTIDAS",
		HORIZONTAL_ALIGNMENT_LEFT, -1, int(round(12 * _scale)), COLOR_MUTED
	)

	var recent: Array = _account.get("recent", [])
	if recent.is_empty():
		ink.draw_string(
			font, Vector2(list.x, list.y + 42 * _scale), "todavía ninguna",
			HORIZONTAL_ALIGNMENT_LEFT, -1, int(round(13 * _scale)), COLOR_MUTED
		)
		return

	var y := list.y + 42 * _scale
	for i in mini(recent.size(), 5):
		var m: Dictionary = recent[i]
		var won: bool = bool(m.get("won", false))
		ink.draw_string(
			font, Vector2(list.x, y),
			"#%d de %d" % [int(m.get("place", 0)), int(m.get("of", 0))],
			HORIZONTAL_ALIGNMENT_LEFT, -1, int(round(14 * _scale)),
			COLOR_WIN if won else COLOR_TEXT
		)
		var kills := int(m.get("kills", 0))
		ink.draw_string(
			font, Vector2(list.x, y + 15 * _scale),
			"%d %s · %s" % [kills, "baja" if kills == 1 else "bajas", str(m.get("map", "—"))],
			HORIZONTAL_ALIGNMENT_LEFT, -1, int(round(11 * _scale)), COLOR_MUTED
		)
		y += 34 * _scale


func _text(panel: Control, rect: Rect2, content: String, size_px: int, color: Color) -> Label:
	var label := Label.new()
	label.set_position(rect.position * _scale)
	label.set_size(rect.size * _scale)
	label.text = content
	label.add_theme_font_size_override("font_size", int(round(size_px * _scale)))
	label.add_theme_color_override("font_color", color)
	label.mouse_filter = Control.MOUSE_FILTER_IGNORE
	panel.add_child(label)
	return label


func _plate(panel: Control, rect: Rect2, on_pressed: Callable) -> Button:
	var button := Button.new()
	button.set_position(rect.position * _scale)
	button.set_size(rect.size * _scale)
	button.focus_mode = Control.FOCUS_NONE
	button.add_theme_stylebox_override("normal", StyleBoxEmpty.new())
	button.add_theme_stylebox_override("hover", _tint(Color(1, 0.9, 0.6, 0.12)))
	button.add_theme_stylebox_override("pressed", _tint(Color(0, 0, 0, 0.28)))
	button.pressed.connect(on_pressed)
	panel.add_child(button)
	return button


func _tint(fill: Color) -> StyleBoxFlat:
	var box := StyleBoxFlat.new()
	box.bg_color = fill
	box.set_corner_radius_all(int(round(6 * _scale)))
	return box
