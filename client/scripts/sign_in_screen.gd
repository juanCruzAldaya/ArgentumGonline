extends Control
## Entrar con una cuenta que ya existe.
##
## Reemplaza al panel que esta pantalla tenía dibujado a mano con StyleBoxFlat,
## que era la única de las cuatro sin arte. Registrarse es otra pantalla
## (register_screen.gd) porque el arte lo pide así: son dos placas distintas en
## la pantalla de inicio, y por lo tanto dos caminos distintos.
##
## Lo que sigue a entrar es el campamento, directo. No hay una pantalla de ficha
## en el medio: la carrera se muestra en la columna derecha del lobby, que es
## donde el arte la puso.

signal sign_in_submitted(account: String, password: String)
## Volver a la pantalla de inicio.
signal back_requested

const BG_PATH := "res://assets/ao/ui/signin_bg.png"

## Este arte es más ancho que los otros tres (1065 contra 992) porque la fuente
## venía con otra proporción. El alto es el mismo, que es lo que decide: se
## horneó a los 898 en los que se muestra.
const PANEL_BASE := Vector2(1065, 898)
const PANEL_MARGIN := 32.0

const COLOR_BG := Color("0b0805")
const COLOR_TEXT := Color("ddd0b4")
const COLOR_ACCENT := Color("d9b45b")
const COLOR_PLACEHOLDER := Color("8d8175")
const COLOR_MUTED := Color("9c8f78")
const COLOR_ERROR := Color("e06c5a")

## Las dos canaletas, medidas sobre el PNG. Son los interiores: un control de
## este tamaño cae adentro del bisel en vez de desbordarlo.
const NOMBRE_RECT := Rect2(262, 422, 542, 43)
const CLAVE_RECT := Rect2(262, 534, 542, 43)
## Las dos placas, al borde exterior de su marco de metal.
const ENTRAR_RECT := Rect2(277, 625, 199, 51)
const VOLVER_RECT := Rect2(591, 625, 200, 51)
## Entre la contraseña y los botones, que es donde se mira cuando algo falla.
const MENSAJE_RECT := Rect2(257, 583, 552, 38)

var _scale := 1.0
var _nombre: LineEdit
var _clave: LineEdit
var _mensaje: Label
var _entrar: Button


## configure toma el hello del servidor. Nada de acá usa el piso de contraseña
## que trae: elegir una es problema del registro, no de entrar. Está igual para
## que main.gd construya las pantallas del flujo todas de la misma forma.
func configure(_hello: Dictionary) -> void:
	pass


func _ready() -> void:
	# Ver DIFICULTADES §13: set_anchors_preset conserva el rect actual, que acá
	# todavía es 0x0, y toda la pantalla termina dibujada en la esquina.
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
	# El default del proyecto es nearest, correcto para los sprites del mundo y
	# equivocado acá: esto es arte pintado y _scale no tiene por qué ser entero.
	art.texture_filter = CanvasItem.TEXTURE_FILTER_LINEAR
	panel.add_child(art)

	# Las mismas palabras que el arte traía pintadas adentro de las canaletas.
	# Se sacaron del PNG y vuelven como placeholder vivo: horneadas, el texto
	# que escribís se dibujaría encima del ejemplo.
	_nombre = _field(panel, NOMBRE_RECT, false, "3 a 16 letras o números", 16)
	_clave = _field(panel, CLAVE_RECT, true, "tu contraseña", 0)

	_mensaje = _text(panel, MENSAJE_RECT, "", 13, COLOR_ERROR)
	_mensaje.horizontal_alignment = HORIZONTAL_ALIGNMENT_CENTER
	_mensaje.autowrap_mode = TextServer.AUTOWRAP_WORD_SMART

	_entrar = _plate(panel, ENTRAR_RECT, _submit)
	_plate(panel, VOLVER_RECT, func() -> void: back_requested.emit())

	_nombre.text_submitted.connect(func(_t: String) -> void: _clave.grab_focus())
	_clave.text_submitted.connect(func(_t: String) -> void: _submit())
	_nombre.grab_focus()


func _place(control: Control, rect: Rect2) -> void:
	control.set_position(rect.position * _scale)
	control.set_size(rect.size * _scale)


func _font(base: int) -> int:
	return int(round(base * _scale))


func _field(panel: Control, rect: Rect2, secret: bool, placeholder: String, max_length: int) -> LineEdit:
	var field := LineEdit.new()
	_place(field, rect)
	field.secret = secret
	field.placeholder_text = placeholder
	if max_length > 0:
		field.max_length = max_length
	field.caret_blink = true
	# Sin fondo propio: la canaleta que se ve es la que dibuja el arte.
	field.add_theme_stylebox_override("normal", _trough())
	field.add_theme_stylebox_override("focus", _trough())
	field.add_theme_color_override("font_color", COLOR_TEXT)
	field.add_theme_color_override("font_placeholder_color", COLOR_PLACEHOLDER)
	field.add_theme_color_override("caret_color", COLOR_ACCENT)
	field.add_theme_font_size_override("font_size", _font(19))
	field.add_theme_constant_override("minimum_character_width", 0)
	field.text_changed.connect(func(_t: String) -> void: _clear_message())
	panel.add_child(field)
	return field


func _text(panel: Control, rect: Rect2, content: String, size_px: int, color: Color) -> Label:
	var label := Label.new()
	_place(label, rect)
	label.text = content
	label.add_theme_font_size_override("font_size", _font(size_px))
	label.add_theme_color_override("font_color", color)
	label.mouse_filter = Control.MOUSE_FILTER_IGNORE
	panel.add_child(label)
	return label


## La placa ya viene letreada en el arte, así que el botón no dibuja texto
## propio y sólo tinta el hover.
func _plate(panel: Control, rect: Rect2, on_pressed: Callable) -> Button:
	var button := Button.new()
	_place(button, rect)
	button.focus_mode = Control.FOCUS_NONE
	button.add_theme_stylebox_override("normal", StyleBoxEmpty.new())
	button.add_theme_stylebox_override("hover", _tint(Color(1, 0.9, 0.6, 0.12)))
	button.add_theme_stylebox_override("pressed", _tint(Color(0, 0, 0, 0.28)))
	button.add_theme_stylebox_override("disabled", _tint(Color(0, 0, 0, 0.45)))
	button.pressed.connect(on_pressed)
	panel.add_child(button)
	return button


func _trough() -> StyleBoxEmpty:
	var box := StyleBoxEmpty.new()
	box.set_content_margin_all(round(6 * _scale))
	box.content_margin_left = round(14 * _scale)
	return box


func _tint(fill: Color) -> StyleBoxFlat:
	var box := StyleBoxFlat.new()
	box.bg_color = fill
	box.set_corner_radius_all(int(round(6 * _scale)))
	return box


## _submit revisa lo que puede antes de gastar un viaje al servidor.
##
## Lo que no revisa es el largo de la contraseña: acá no se está eligiendo una,
## se está escribiendo una que ya existe, y rechazar por corta la de alguien que
## se registró cuando el piso era otro sería negarle su propia cuenta.
func _submit() -> void:
	var account := _nombre.text.strip_edges()
	var password := _clave.text

	if account.length() < 3 or account.length() > 16:
		_fail("El nombre tiene que tener entre 3 y 16 caracteres.")
		return
	if password.is_empty():
		_fail("Falta la contraseña.")
		return

	_mensaje.add_theme_color_override("font_color", COLOR_MUTED)
	_mensaje.text = "Entrando..."
	_entrar.disabled = true
	sign_in_submitted.emit(account, password)


func _fail(reason: String) -> void:
	_entrar.disabled = false
	_mensaje.add_theme_color_override("font_color", COLOR_ERROR)
	_mensaje.text = reason


func _clear_message() -> void:
	if _mensaje.get_theme_color("font_color") == COLOR_ERROR:
		_mensaje.text = ""


## rejected es la palabra del servidor, mostrada como vino: es el lado que sabe
## si la cuenta no existe o la contraseña está mal.
func rejected(reason: String) -> void:
	_fail(reason)
	_clave.grab_focus()
	_clave.select_all()
