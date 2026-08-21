extends Node
## Las teclas del juego: catálogo, defaults y lo que el jugador haya cambiado.
##
## Autoload, porque tiene que estar listo antes de que main.gd lea la primera
## tecla, y porque el panel y el juego miran exactamente la misma tabla.
##
## Los nombres y las teclas por defecto salen del Argentum original —
## clsCustomKeys.cls del linaje clásico (Alkon), enum eKeyType — así que
## alguien que jugó AO encuentra lo que espera: Ctrl pega, A agarra, O oculta,
## F6 medita. La columna `ao` deja anotado de cuál constante vino cada una.
##
## Acá están solamente las acciones que el servidor sabe hacer. El original
## tiene 27 y varias —robar, domar, seguro de PK, música, macros— no tienen
## sistema detrás todavía: una tecla configurable que no hace nada es peor que
## no tenerla. Cuando aparezca el sistema, agregar la fila acá alcanza.

## Las acciones registradas en el InputMap al arrancar, en el orden en que el
## panel las dibuja. `column` es 0 para la columna de movimiento y 1 para la de
## acciones, como las dos columnas de frmCustomKeys.
##
## `keys` puede traer más de una tecla: las flechas se acompañan de WASD, que es
## lo que el juego tenía hardcodeado. El oeste es el único sin alternativa a
## propósito — A es Agarrar, y caminar al oeste con A dispararía un intento de
## agarrar por cada paso.
##
## `group` y las etiquetas son las del panel original
## (Graficos/Interfaces/VentanaConfigurarTeclas_spanish.jpg): ahí los 27
## casilleros están repartidos en cinco recuadros con título — Movimiento,
## Acciones, Hablar, Opciones Personales y Otras Teclas. Faltan los dos grupos
## que son todos de sistemas que no existen todavía.
const ACTIONS: Array[Dictionary] = [
	{"action": &"mover_norte", "label": "Arriba", "ao": "mKeyUp", "keys": [KEY_UP, KEY_W], "group": "Movimiento"},
	{"action": &"mover_sur", "label": "Abajo", "ao": "mKeyDown", "keys": [KEY_DOWN, KEY_S], "group": "Movimiento"},
	{"action": &"mover_oeste", "label": "Izquierda", "ao": "mKeyLeft", "keys": [KEY_LEFT], "group": "Movimiento"},
	{"action": &"mover_este", "label": "Derecha", "ao": "mKeyRight", "keys": [KEY_RIGHT, KEY_D], "group": "Movimiento"},

	{"action": &"agarrar", "label": "Agarrar", "ao": "mKeyGetObject", "keys": [KEY_A], "group": "Acciones"},
	{"action": &"equipar", "label": "Equipar", "ao": "mKeyEquipObject", "keys": [KEY_E], "group": "Acciones"},
	{"action": &"ocultarse", "label": "Ocultar", "ao": "mKeyHide", "keys": [KEY_O], "group": "Acciones"},
	{"action": &"tirar", "label": "Tirar", "ao": "mKeyDropObject", "keys": [KEY_T], "group": "Acciones"},
	{"action": &"usar", "label": "Usar", "ao": "mKeyUseObject", "keys": [KEY_U], "group": "Acciones"},
	{"action": &"atacar", "label": "Atacar", "ao": "mKeyAttack", "keys": [KEY_CTRL], "group": "Acciones"},

	{"action": &"hablar", "label": "Hablar a Todos", "ao": "mKeyTalk", "keys": [KEY_ENTER, KEY_KP_ENTER], "group": "Hablar"},

	{"action": &"meditar", "label": "Meditar", "ao": "mKeyMeditate", "keys": [KEY_F6], "group": "Otras Teclas"},
	{"action": &"mapa", "label": "Mapa", "ao": "", "keys": [KEY_M], "group": "Otras Teclas"},
]

## El orden de los recuadros y en qué columna cae cada uno, como en el original:
## Movimiento, Acciones y Hablar a la izquierda; Otras Teclas a la derecha.
const GROUPS: Array[Dictionary] = [
	{"name": "Movimiento", "column": 0},
	{"name": "Acciones", "column": 0},
	{"name": "Hablar", "column": 0},
	{"name": "Otras Teclas", "column": 1},
]

const CONFIG_PATH := "user://teclas.cfg"
const CONFIG_SECTION := "teclas"

## Lo que el jugador cambió se guarda como una sola tecla por acción, aunque el
## default traiga dos. El original asigna una tecla por acción y el panel
## muestra una sola casilla; conservar en silencio un alt que la pantalla no
## muestra es prometer algo que no se ve.
var _defaults: Dictionary = {}


func _ready() -> void:
	_install_defaults()
	_load_overrides()


func _install_defaults() -> void:
	for entry in ACTIONS:
		var action: StringName = entry["action"]
		_defaults[action] = (entry["keys"] as Array).duplicate()
		if not InputMap.has_action(action):
			InputMap.add_action(action)
		_bind(action, entry["keys"])


## Reemplaza las teclas de una acción. Physical keycode y no keycode: con un
## teclado latinoamericano el keycode se mueve con el layout, y lo que uno
## espera de WASD es la posición física de la tecla, no la letra impresa.
func _bind(action: StringName, keys: Array) -> void:
	InputMap.action_erase_events(action)
	for key in keys:
		var event := InputEventKey.new()
		event.physical_keycode = key
		InputMap.action_add_event(action, event)


## La tecla que el panel muestra para una acción: la primera, que es la que el
## jugador ve escrita en el original.
func current_key(action: StringName) -> int:
	for event in InputMap.action_get_events(action):
		if event is InputEventKey:
			return event.physical_keycode
	return KEY_NONE


func snapshot() -> Dictionary:
	var out := {}
	for entry in ACTIONS:
		out[entry["action"]] = current_key(entry["action"])
	return out


func defaults_snapshot() -> Dictionary:
	var out := {}
	for entry in ACTIONS:
		out[entry["action"]] = (_defaults[entry["action"]] as Array)[0]
	return out


## Aplica un mapa completo de acción -> tecla, el que el panel venía editando
## aparte. Guardar en disco es parte de aplicar: si el juego ya te cambió las
## teclas, la próxima sesión tiene que arrancar igual.
func apply(bindings: Dictionary, persist := true) -> void:
	for action in bindings:
		if not InputMap.has_action(action):
			continue
		_bind_choice(action, int(bindings[action]))
	if persist:
		save()


## Deja una acción en la tecla elegida — o en sus teclas de fábrica, si la
## elegida es justamente la de fábrica.
##
## Esa distinción es la que salva las alternativas. El panel muestra una tecla
## por acción, así que guardar convierte cada acción en esa única tecla, y
## apretar GUARDAR sin haber tocado nada te sacaba el WASD que acompaña a las
## flechas. Nadie va a un panel de teclas a perder teclas que no tocó.
func _bind_choice(action: StringName, key: int) -> void:
	var defaults: Array = _defaults.get(action, [])
	if not defaults.is_empty() and key == int(defaults[0]):
		_bind(action, defaults)
	else:
		_bind(action, [key])


## Vuelve a las teclas de fábrica y borra el archivo, para que no quede un
## override viejo esperando a la próxima sesión. Esto es lo que el botón "POR
## DEFECTO" del original dice que hace y no hace: allá abre otra pantalla.
func restore_defaults() -> void:
	for entry in ACTIONS:
		_bind(entry["action"], entry["keys"])
	DirAccess.remove_absolute(ProjectSettings.globalize_path(CONFIG_PATH))
	var config := ConfigFile.new()
	config.save(CONFIG_PATH)


func save() -> void:
	var config := ConfigFile.new()
	for entry in ACTIONS:
		var action: StringName = entry["action"]
		config.set_value(CONFIG_SECTION, String(action), current_key(action))
	# En web user:// vive sobre IndexedDB y el flush es asíncrono, así que esto
	# se llama al apretar GUARDAR y no al cerrar la pestaña: un guardado que
	# arranca mientras la página se está yendo puede no llegar nunca.
	var err := config.save(CONFIG_PATH)
	if err != OK:
		push_warning("no se pudieron guardar las teclas: %d" % err)


## Carga lo que el jugador haya cambiado, tecla por tecla y nunca todo o nada.
##
## Una acción que no está en el archivo se queda con su default, y una que el
## archivo nombra pero el juego no conoce se ignora: así un teclas.cfg viejo,
## editado a mano o de una versión con otras acciones sigue sirviendo en vez de
## dejar al jugador sin poder atacar. Es el fallback por clave del linaje
## moderno (GetBind), que el clásico no tiene.
func _load_overrides() -> void:
	var config := ConfigFile.new()
	if config.load(CONFIG_PATH) != OK:
		return # sin archivo, o ilegible: defaults y a jugar

	var taken := {}
	for entry in ACTIONS:
		var action: StringName = entry["action"]
		var key := int(config.get_value(CONFIG_SECTION, String(action), KEY_NONE))
		if key == KEY_NONE:
			taken[current_key(action)] = action
			continue
		# Un duplicado no puede entrar: gana el primero del catálogo y el otro
		# se queda con su default. El panel ya no deja crearlos, pero el archivo
		# es texto y se edita a mano.
		if taken.has(key):
			push_warning("tecla repetida en %s: %s ya la usa" % [action, taken[key]])
			taken[current_key(action)] = action
			continue
		_bind_choice(action, key)
		taken[key] = action


## El nombre que se dibuja en la casilla. Traduce de posición física a la letra
## que el jugador tiene impresa en SU teclado, que es el ReadableName del
## original.
static func key_name(physical_keycode: int) -> String:
	if physical_keycode == KEY_NONE:
		return "—"
	# No todos los display servers saben traducir: headless no, y no doy por
	# sentado que el de web sí. Cuando no puede, el nombre de la posición
	# física es una respuesta peor pero correcta — una casilla vacía en el
	# panel no lo es.
	var keycode := DisplayServer.keyboard_get_keycode_from_physical(physical_keycode)
	if keycode == KEY_NONE:
		keycode = physical_keycode
	return OS.get_keycode_string(keycode)
