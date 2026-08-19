extends Control
## Registro de cuenta: usuario, correo, contraseña y su confirmación.
##
## Es la pantalla que dibuja register_bg.png, y como todo el arte de UI de este
## proyecto, los controles vivos caen adentro de los agujeros que el arte ya
## trae en vez de reconstruir el marco con primitivas (DIFICULTADES.md §2).
## Cada rect de acá abajo es una medición sobre el PNG, no un número a ojo: si
## cambia la imagen, hay que volver a medir.
##
## Las canaletas del arte venían con asteriscos y un cursor pintados adentro:
## contenido de ejemplo del mockup, no parte del marco. Se sacaron del PNG en
## vez de taparse con un relleno opaco, que es lo que se probó primero y se veía
## como un rectángulo plano encima de la madera. Por eso los campos de acá no
## dibujan fondo: la textura de la canaleta es la del arte.

## Pide crear la cuenta. El correo viaja porque el servidor lo exige al
## registrar; entrar con una cuenta que ya existe no lo pide ni lo mira.
signal register_submitted(account: String, email: String, password: String)
## VOLVER: la pantalla de inicio otra vez.
signal back_requested

const BG_PATH := "res://assets/ao/ui/register_bg.png"

## Igual que start_screen: el arte se horneó al alto en el que se muestra, así
## que estos números son píxeles de pantalla.
const PANEL_BASE := Vector2(992, 898)
const PANEL_MARGIN := 32.0

const COLOR_BG := Color("0b0805")
const COLOR_TEXT := Color("ddd0b4")
const COLOR_ACCENT := Color("d9b45b")
const COLOR_PLACEHOLDER := Color("7d7264")
const COLOR_MUTED := Color("9c8f78")
const COLOR_ERROR := Color("e06c5a")

## Las cuatro canaletas, medidas perfilando la luminancia al cruzar el bisel:
## el bisel es una banda clara de 6 a 10 px y el interior arranca justo después
## (OPERACION §3). Son los interiores, así que un control de este tamaño cae
## adentro del bisel en vez de desbordarlo.
const USUARIO_RECT := Rect2(429, 257, 416, 44)
const CORREO_RECT := Rect2(429, 325, 416, 44)
const PASS_RECT := Rect2(429, 395, 416, 44)
const CONFIRM_RECT := Rect2(429, 465, 416, 44)
## El cuadradito de los términos, al borde exterior de su marco, y la etiqueta
## horneada de al lado, que también tiene que recibir el click.
const TERMINOS_RECT := Rect2(125, 533, 31, 31)
const ETIQUETA_RECT := Rect2(163, 533, 620, 31)
## Las dos placas, al borde exterior del marco de metal.
const REGISTRAR_RECT := Rect2(326, 807, 320, 60)
const VOLVER_RECT := Rect2(777, 817, 157, 40)
## La tabla vacía entre el checkbox y el botón. El arte deja ahí 240 px de
## madera sin nada, que es exactamente donde tienen que ir lo que guardamos y
## lo que salga mal.
const AVISO_RECT := Rect2(125, 592, 720, 64)
const MENSAJE_RECT := Rect2(125, 676, 720, 90)

## Espejo de las reglas del servidor (internal/account). Están acá para poder
## decir que no sin gastar un viaje de ida y vuelta, no para reemplazarlas: el
## servidor vuelve a validar todo, porque este lado no es de fiar.
const NAME_MIN := 3
const NAME_MAX := 16
const EMAIL_MAX := 254

var _min_password := 6

var _scale := 1.0
var _usuario: LineEdit
var _correo: LineEdit
var _pass: LineEdit
var _confirm: LineEdit
var _terminos: Button
var _tilde: Line2D
var _mensaje: Label
var _registrar: Button


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

	_usuario = _field(panel, USUARIO_RECT, false, "3 a 16 letras o números", NAME_MAX)
	_correo = _field(panel, CORREO_RECT, false, "vos@ejemplo.com", EMAIL_MAX)
	_pass = _field(panel, PASS_RECT, true, "al menos %d caracteres" % _min_password, 0)
	_confirm = _field(panel, CONFIRM_RECT, true, "de nuevo, igual", 0)

	_build_terminos(panel)

	# Lo que la casilla de al lado dice que aceptás. Sin esta línea el checkbox
	# pide aceptar un documento que no existe: no hay términos ni política
	# escritos en ninguna parte, y esto es todo lo que el servidor hace con tus
	# datos. El correo se guarda en claro en un archivo que no se reescribe
	# nunca — ver account.Store.Register del lado del servidor.
	var aviso := _text(
		panel, AVISO_RECT,
		"Guardamos tu nombre, tu correo y el resultado de tus partidas en este servidor, "
		+ "en un archivo que no se borra. Nadie te va a escribir: no hay nada acá que mande "
		+ "correo. El juego es AGPL-3.0 y el código es público.",
		12, COLOR_MUTED
	)
	aviso.autowrap_mode = TextServer.AUTOWRAP_WORD_SMART

	_mensaje = _text(panel, MENSAJE_RECT, "", 15, COLOR_ERROR)
	_mensaje.autowrap_mode = TextServer.AUTOWRAP_WORD_SMART
	_mensaje.horizontal_alignment = HORIZONTAL_ALIGNMENT_CENTER

	_registrar = _plate(panel, REGISTRAR_RECT, _submit)
	_plate(panel, VOLVER_RECT, func() -> void: back_requested.emit())

	# Enter baja al campo siguiente y, en el último, manda. Es lo que espera
	# cualquiera que esté tipeando un formulario de cuatro campos.
	_usuario.text_submitted.connect(func(_t: String) -> void: _correo.grab_focus())
	_correo.text_submitted.connect(func(_t: String) -> void: _pass.grab_focus())
	_pass.text_submitted.connect(func(_t: String) -> void: _confirm.grab_focus())
	_confirm.text_submitted.connect(func(_t: String) -> void: _submit())
	_usuario.grab_focus()


## configure toma el hello del servidor, que trae el piso de la contraseña.
func configure(hello: Dictionary) -> void:
	_min_password = int(hello.get("minpass", _min_password))
	if _pass != null:
		_pass.placeholder_text = "al menos %d caracteres" % _min_password


func _place(control: Control, rect: Rect2) -> void:
	control.set_position(rect.position * _scale)
	control.set_size(rect.size * _scale)


## Los cuerpos de letra también son números del espacio del arte, y se redondean
## a un tamaño de fuente real en vez de aplicarse como transformación, así que
## los glifos salen nítidos y no rasterizados chiquitos y después estirados.
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
	field.add_theme_stylebox_override("normal", _trough())
	field.add_theme_stylebox_override("focus", _trough())
	# Sin fondo propio, el borde del foco de Godot sería lo único que se dibuje
	# encima del bisel horneado, y no coincide con él.
	field.add_theme_constant_override("outline_size", 0)
	field.add_theme_color_override("font_color", COLOR_TEXT)
	field.add_theme_color_override("font_placeholder_color", COLOR_PLACEHOLDER)
	field.add_theme_color_override("caret_color", COLOR_ACCENT)
	field.add_theme_font_size_override("font_size", _font(17))
	field.add_theme_constant_override("minimum_character_width", 0)
	field.text_changed.connect(func(_t: String) -> void: _clear_message())
	panel.add_child(field)
	return field


## El cuadrado de los términos. El arte dibuja el marco vacío, así que el
## control no aporta caja: aporta la tilde, que es un Line2D y no un glifo,
## porque la fuente de fallback no tiene por qué traer un carácter de tilde.
func _build_terminos(panel: Control) -> void:
	_terminos = Button.new()
	_place(_terminos, TERMINOS_RECT)
	_terminos.toggle_mode = true
	_terminos.add_theme_stylebox_override("normal", StyleBoxEmpty.new())
	_terminos.add_theme_stylebox_override("hover", _tint(Color(1, 0.9, 0.6, 0.14)))
	_terminos.add_theme_stylebox_override("pressed", StyleBoxEmpty.new())
	_terminos.add_theme_stylebox_override("focus", StyleBoxEmpty.new())
	panel.add_child(_terminos)

	_tilde = Line2D.new()
	_tilde.width = maxf(2.0, 3.0 * _scale)
	_tilde.default_color = COLOR_ACCENT
	_tilde.antialiased = true
	_tilde.position = TERMINOS_RECT.position * _scale
	for point in [Vector2(8, 16), Vector2(13, 23), Vector2(24, 9)]:
		_tilde.add_point(point * _scale)
	_tilde.visible = false
	panel.add_child(_tilde)

	_terminos.toggled.connect(_on_terminos_toggled)

	# La etiqueta de al lado está horneada en el arte, así que no hay un Label
	# al que pedirle que también reciba el click: este botón invisible la cubre
	# y hace lo mismo que el cuadradito.
	var etiqueta := Button.new()
	_place(etiqueta, ETIQUETA_RECT)
	etiqueta.focus_mode = Control.FOCUS_NONE
	etiqueta.add_theme_stylebox_override("normal", StyleBoxEmpty.new())
	etiqueta.add_theme_stylebox_override("hover", StyleBoxEmpty.new())
	etiqueta.add_theme_stylebox_override("pressed", StyleBoxEmpty.new())
	etiqueta.pressed.connect(func() -> void:
		_terminos.button_pressed = not _terminos.button_pressed
	)
	panel.add_child(etiqueta)


func _on_terminos_toggled(on: bool) -> void:
	_tilde.visible = on
	_clear_message()


func _text(panel: Control, rect: Rect2, content: String, size_px: int, color: Color) -> Label:
	var label := Label.new()
	_place(label, rect)
	label.text = content
	label.add_theme_font_size_override("font_size", _font(size_px))
	label.add_theme_color_override("font_color", color)
	label.mouse_filter = Control.MOUSE_FILTER_IGNORE
	panel.add_child(label)
	return label


## Igual que en la pantalla de inicio: la placa ya viene letreada en el arte,
## así que el botón no dibuja texto propio y solo tinta el hover.
func _plate(panel: Control, rect: Rect2, on_pressed: Callable) -> Button:
	var button := Button.new()
	_place(button, rect)
	button.focus_mode = Control.FOCUS_NONE
	button.add_theme_stylebox_override("normal", StyleBoxEmpty.new())
	button.add_theme_stylebox_override("hover", _tint(Color(1, 0.9, 0.6, 0.12)))
	button.add_theme_stylebox_override("pressed", _tint(Color(0, 0, 0, 0.28)))
	button.add_theme_stylebox_override("disabled", _tint(Color(0, 0, 0, 0.5)))
	button.pressed.connect(on_pressed)
	panel.add_child(button)
	return button


## Una canaleta sin fondo: lo único que aporta es el margen que mete el texto
## adentro del bisel que el arte ya dibuja. El fondo es el arte.
func _trough() -> StyleBoxEmpty:
	var box := StyleBoxEmpty.new()
	box.set_content_margin_all(round(6 * _scale))
	box.content_margin_left = round(12 * _scale)
	return box


func _tint(fill: Color) -> StyleBoxFlat:
	var box := StyleBoxFlat.new()
	box.bg_color = fill
	box.set_corner_radius_all(int(round(6 * _scale)))
	return box


## _submit revisa todo lo que se puede revisar de este lado antes de gastar un
## viaje al servidor. Un error que el servidor va a devolver igual se lee como
## un problema del servidor; rechazarlo acá se lee como un formulario.
##
## El botón nunca se apaga por estar incompleto: con cuatro campos y una
## casilla, una placa deshabilitada y sin explicación no dice cuál de las cinco
## cosas falta.
func _submit() -> void:
	var account := _usuario.text.strip_edges()
	var email := _correo.text.strip_edges()
	var password := _pass.text

	if account.length() < NAME_MIN or account.length() > NAME_MAX:
		_fail("El usuario tiene que tener entre %d y %d caracteres." % [NAME_MIN, NAME_MAX], _usuario)
		return
	if not _is_alphanumeric(account):
		_fail("El usuario va sin espacios ni acentos: solo letras y números.", _usuario)
		return
	if not _looks_like_email(email):
		_fail("Ese correo no parece un correo.", _correo)
		return
	if password.length() < _min_password:
		_fail("La contraseña necesita al menos %d caracteres." % _min_password, _pass)
		return
	if _confirm.text != password:
		_fail("Las dos contraseñas no coinciden.", _confirm)
		return
	if not _terminos.button_pressed:
		_fail("Falta tildar la casilla de abajo de los campos.", null)
		return

	_mensaje.add_theme_color_override("font_color", COLOR_MUTED)
	_mensaje.text = "Creando la cuenta..."
	_registrar.disabled = true
	register_submitted.emit(account, email, password)


## Espejo de validName del servidor. Sin acentos y sin eñe a propósito: un
## nombre acá es una identidad que se compara, no una etiqueta arriba de la
## cabeza.
func _is_alphanumeric(text: String) -> bool:
	for i in text.length():
		var c := text[i]
		if not ((c >= "a" and c <= "z") or (c >= "A" and c <= "Z") or (c >= "0" and c <= "9")):
			return false
	return true


## Una comprobación de forma, no de existencia: el servidor la vuelve a hacer
## con net/mail, y ni siquiera él puede saber si la casilla existe, porque nada
## en este proyecto manda un correo.
func _looks_like_email(email: String) -> bool:
	if email.is_empty() or email.length() > EMAIL_MAX:
		return false
	if email.count("@") != 1:
		return false
	var at := email.find("@")
	if at <= 0 or at == email.length() - 1:
		return false
	var domain := email.substr(at + 1)
	return domain.contains(".") and not domain.begins_with(".") and not domain.ends_with(".")


func _fail(reason: String, focus: LineEdit) -> void:
	_registrar.disabled = false
	_mensaje.add_theme_color_override("font_color", COLOR_ERROR)
	_mensaje.text = reason
	if focus != null:
		focus.grab_focus()
		focus.select_all()


## Escribir en cualquier campo borra el error de la vuelta anterior, pero no el
## "Creando la cuenta...": ese lo tiene que pisar la respuesta del servidor.
func _clear_message() -> void:
	if _mensaje.get_theme_color("font_color") == COLOR_ERROR:
		_mensaje.text = ""


## rejected es la palabra del servidor, mostrada como vino: es el lado que sabe
## si el nombre está tomado o si el correo no le gustó, y escribir acá una frase
## más amable arriesga decir algo que no es cierto.
func rejected(reason: String) -> void:
	_fail(reason, null)
