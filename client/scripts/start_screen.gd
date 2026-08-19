extends Control
## La pantalla de inicio: entrar, crear una cuenta, o irse.
##
## Es la primera cosa que ve alguien en un servidor con cuentas, y existe
## porque el arte lo pide: "INICIAR SESIÓN" y "CREAR CUENTA" son dos placas
## distintas, así que son dos caminos distintos. Antes las dos cosas vivían en
## un formulario solo con dos botones, que obligaba a adivinar cuál de los dos
## era el que querías.
##
## El fondo es arte de verdad (start_bg.png), no primitivas, y cada control
## vivo cae adentro de un agujero que el arte ya dibuja — mismo criterio que
## character_picker.gd, y por la misma razón: un StyleBoxFlat no tiene bisel,
## ni madera, ni hueso tallado. Ver DIFICULTADES.md §2.

## Las tres salidas. El padre decide qué pantalla viene después de cada una.
signal sign_in_requested
signal register_requested
signal quit_requested

const BG_PATH := "res://assets/ao/ui/start_bg.png"

## Tamaño nativo de start_bg.png, que además es el tamaño en el que se muestra:
## el arte se horneó a 898 de alto, que es 962 (el viewport) menos los 32 de
## aire de arriba y abajo. Así los rects de acá abajo son píxeles de pantalla y
## no queda ningún factor de escala que equivocar.
const PANEL_BASE := Vector2(992, 898)
const PANEL_MARGIN := 32.0

const COLOR_BG := Color("0b0805")

## Las tres placas, medidas sobre el PNG por componentes conexas sobre la
## máscara de rojo (R-B > 22) que OPERACION §3 usa para las placas de botón: el
## granate se despega de la madera mucho más limpio que el brillo. Van al borde
## exterior del marco de metal, así que el tinte del hover termina exactamente
## donde termina la placa.
const ENTRAR_RECT := Rect2(301, 372, 399, 79)
const CREAR_RECT := Rect2(301, 509, 399, 80)
const SALIR_RECT := Rect2(777, 817, 157, 40)

var _scale := 1.0


## configure toma el hello del servidor. Esta pantalla no usa nada de lo que
## trae — no hay ningún campo que validar acá — pero la tiene igual para que
## main.gd construya las tres pantallas del flujo de la misma manera.
func configure(_hello: Dictionary) -> void:
	pass


func _ready() -> void:
	# set_anchors_and_offsets_preset, no set_anchors_preset: el segundo
	# argumento del segundo es keep_offsets y viene en true, así que llamado
	# acá — con el nodo ya en el árbol y en 0x0 — Godot conserva ese rect y la
	# pantalla entera se dibuja en un bloque chiquito en la esquina. Es
	# DIFICULTADES §13, que ya pasó dos veces en dos archivos distintos.
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

	var entrar := _plate(panel, ENTRAR_RECT, func() -> void: sign_in_requested.emit())
	_plate(panel, CREAR_RECT, func() -> void: register_requested.emit())
	_plate(panel, SALIR_RECT, func() -> void: quit_requested.emit())
	entrar.grab_focus()


## Un botón invisible encima de la placa que el arte ya dibuja. No lleva texto
## propio: "INICIAR SESIÓN" ya está letreado en el PNG, y ponerle una etiqueta
## encima lo duplicaría. El hover y el pressed son un tinte, no un stylebox
## nuevo, así que el bisel y el brillo horneados siguen viéndose.
func _plate(panel: Control, rect: Rect2, on_pressed: Callable) -> Button:
	var button := Button.new()
	button.set_position(rect.position * _scale)
	button.set_size(rect.size * _scale)
	button.add_theme_stylebox_override("normal", StyleBoxEmpty.new())
	button.add_theme_stylebox_override("hover", _tint(Color(1, 0.9, 0.6, 0.12)))
	button.add_theme_stylebox_override("pressed", _tint(Color(0, 0, 0, 0.28)))
	button.add_theme_stylebox_override("focus", StyleBoxEmpty.new())
	button.pressed.connect(on_pressed)
	panel.add_child(button)
	return button


func _tint(fill: Color) -> StyleBoxFlat:
	var box := StyleBoxFlat.new()
	box.bg_color = fill
	box.set_corner_radius_all(int(round(6 * _scale)))
	return box
