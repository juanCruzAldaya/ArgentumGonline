extends Node2D
## Wires the network client to the view, the HUD and the minimap, and turns held
## keys into move commands.

const DEFAULT_URL := "ws://127.0.0.1:8080/ws"

## How often held keys produce a move command. The server enforces the real walk
## cadence, so this only has to be fast enough that input never feels dropped.
const INPUT_INTERVAL := 0.05

@onready var _net: Node = $Net
@onready var _view: Node2D = $WorldView
@onready var _hud: Control = $UI/Screen
@onready var _minimap: Control = $UI/Screen/MinimapFrame/Minimap

var _url := DEFAULT_URL
var _player_name := ""
var _local_id := 0
var _time_since_input := 0.0
var _connected := false
## Spell awaiting a target. Argentum casts in two steps — pick the spell, then
## pick who it lands on — and the second click is what this holds open.
var _targeting_spell := 0
var _warned_casting := false
## Entities seen in the previous snapshot, so the console can report who walked
## into and out of view. Diffing here keeps the server free of an event stream
## it does not need yet.
var _seen: Dictionary = {}


func _ready() -> void:
	randomize()
	_url = _resolve("server", "JUEGITO_SERVER", _default_server_url())
	_player_name = _resolve("name", "JUEGITO_NAME", "wachin%02d" % (randi() % 100))

	_net.server_connected.connect(_on_connected)
	_net.server_disconnected.connect(_on_disconnected)
	_net.welcomed.connect(_on_welcomed)
	_net.snapshot_received.connect(_on_snapshot)
	_net.loadout_received.connect(_hud.set_loadout)
	_net.combat_received.connect(_on_combat)
	_hud.cast_requested.connect(_on_cast_requested)

	_hud.set_character(_player_name)
	_hud.log_line("conectando a %s ..." % _url, _hud.COLOR_TEXT_DIM)
	_net.connect_to_server(_url, _player_name)


func _process(delta: float) -> void:
	_time_since_input += delta
	if not _connected or _time_since_input < INPUT_INTERVAL:
		return

	# Ctrl swings, as in Argentum. The server enforces the real cadence, so
	# holding it down simply gets the extra requests ignored.
	if Input.is_key_pressed(KEY_CTRL):
		_time_since_input = 0.0
		_net.send_attack()
		return

	var dir := _pressed_direction()
	if dir >= 0:
		_time_since_input = 0.0
		_net.send_move(dir)


## Combat lines are worded the way Argentum words them, from the point of view
## of whoever is reading them.
func _on_combat(event: Dictionary) -> void:
	var mine: bool = bool(event.get("mine", false))
	var attacker := str(event.get("an", "alguien"))
	var victim := str(event.get("vn", "alguien"))
	var damage := int(event.get("dmg", 0))

	if not bool(event.get("hit", false)):
		if bool(event.get("blocked", false)):
			if mine:
				_hud.log_line("%s rechazó tu ataque con su escudo." % victim, _hud.COLOR_TEXT_DIM)
			else:
				_hud.log_line("Rechazaste el ataque de %s con tu escudo." % attacker, _hud.COLOR_MANA)
		elif mine:
			_hud.log_line("¡Has fallado el golpe!", _hud.COLOR_TEXT_DIM)
		else:
			_hud.log_line("¡%s ha fallado el golpe!" % attacker, _hud.COLOR_TEXT_DIM)
		return

	if mine:
		_hud.log_line("Le has quitado %d puntos de vida a %s." % [damage, victim], _hud.COLOR_HP)
	else:
		_hud.log_line("%s te ha quitado %d puntos de vida." % [attacker, damage], _hud.COLOR_EXP)

	if bool(event.get("killed", false)):
		if mine:
			_hud.log_line("¡Has matado a %s!" % victim, _hud.COLOR_ACCENT)
		else:
			_hud.log_line("¡%s te ha matado!" % attacker, _hud.COLOR_EXP)


func _on_cast_requested(spell_id: int) -> void:
	_targeting_spell = spell_id
	_view.targeting = true
	Input.set_default_cursor_shape(Input.CURSOR_CROSS)
	_hud.log_line("Elegí un objetivo. Click derecho o Esc para cancelar.", _hud.COLOR_ACCENT)


func _stop_targeting() -> void:
	_targeting_spell = 0
	_view.targeting = false
	Input.set_default_cursor_shape(Input.CURSOR_ARROW)


func _unhandled_input(event: InputEvent) -> void:
	if _targeting_spell == 0:
		return

	var cancel: bool = event is InputEventKey and event.pressed and event.keycode == KEY_ESCAPE
	if event is InputEventMouseButton and event.pressed and event.button_index == MOUSE_BUTTON_RIGHT:
		cancel = true
	if cancel:
		_stop_targeting()
		_hud.log_line("Cancelado.", _hud.COLOR_TEXT_DIM)
		return

	if not (event is InputEventMouseButton and event.pressed and event.button_index == MOUSE_BUTTON_LEFT):
		return

	var target: int = _view.entity_at(_view.to_local(event.position))
	if target == 0:
		_hud.log_line("No hay nadie ahí.", _hud.COLOR_TEXT_DIM)
		return

	var spell_name := str(_hud.spell_name(_targeting_spell))
	_net.send_cast(_targeting_spell, target)
	_stop_targeting()
	_hud.log_line("Lanzás %s sobre %s." % [spell_name, _view.entity_name(target)], _hud.COLOR_ACCENT)

	# The flow is wired end to end, but the server does not resolve spells yet.
	# Saying so once beats letting it look like nothing happened.
	if not _warned_casting:
		_warned_casting = true
		_hud.log_line("(el servidor todavía no resuelve hechizos)", _hud.COLOR_TEXT_DIM)


## Raw key checks rather than input actions, so the project needs no input map
## and the controls read the same as the server's heading constants.
func _pressed_direction() -> int:
	if Input.is_key_pressed(KEY_UP) or Input.is_key_pressed(KEY_W):
		return 0  # north
	if Input.is_key_pressed(KEY_RIGHT) or Input.is_key_pressed(KEY_D):
		return 1  # east
	if Input.is_key_pressed(KEY_DOWN) or Input.is_key_pressed(KEY_S):
		return 2  # south
	if Input.is_key_pressed(KEY_LEFT) or Input.is_key_pressed(KEY_A):
		return 3  # west
	return -1


## In the browser the page is served by the same Go process that speaks the
## protocol, so its own origin is the right default and the player configures
## nothing at all.
func _default_server_url() -> String:
	if OS.has_feature("web"):
		return str(
			JavaScriptBridge.eval(
				"(location.protocol === 'https:' ? 'wss://' : 'ws://') + location.host + '/ws'", true
			)
		)
	return DEFAULT_URL


## Web builds have no command line and no environment, so configuration arrives
## as query string parameters instead: ?server=...&name=...
func _resolve(key: String, env_var: String, fallback: String) -> String:
	if OS.has_feature("web"):
		var param := str(
			JavaScriptBridge.eval(
				"new URLSearchParams(location.search).get('%s') || ''" % key, true
			)
		)
		return param if param != "" else fallback

	var prefix := "--%s=" % key
	for arg in OS.get_cmdline_user_args() + OS.get_cmdline_args():
		if arg.begins_with(prefix):
			return arg.substr(prefix.length())
	if OS.has_environment(env_var):
		return OS.get_environment(env_var)
	return fallback


func _on_connected() -> void:
	_connected = true
	_hud.log_line("conectado como %s" % _player_name, _hud.COLOR_ACCENT)


func _on_disconnected() -> void:
	_connected = false
	_seen.clear()
	_hud.log_line("desconectado de %s" % _url, _hud.COLOR_HP)


func _on_welcomed(welcome: Dictionary) -> void:
	_local_id = int(welcome.get("id", 0))
	_view.configure(welcome)
	_minimap.configure(welcome)

	var map_name := str(welcome.get("mapName", ""))
	if map_name != "":
		_hud.log_line("Has llegado a %s." % map_name, _hud.COLOR_ACCENT)
	_hud.log_line(
		(
			"mapa %dx%d  ·  %d Hz  ·  spawn en (%d, %d)"
			% [
				int(welcome.get("w", 0)),
				int(welcome.get("h", 0)),
				int(welcome.get("tickRate", 0)),
				int(welcome.get("sx", 0)),
				int(welcome.get("sy", 0)),
			]
		),
		_hud.COLOR_TEXT_DIM
	)


func _on_snapshot(snapshot: Dictionary) -> void:
	var entities: Array = snapshot.get("e", [])

	_view.set_entities(entities)
	_minimap.set_entities(entities)
	_hud.set_alive(int(snapshot.get("alive", 0)))

	var vitals: Variant = snapshot.get("self")
	if typeof(vitals) == TYPE_DICTIONARY:
		_hud.set_vitals(vitals)

	_report_arrivals_and_departures(entities)


func _report_arrivals_and_departures(entities: Array) -> void:
	var current: Dictionary = {}
	for entity in entities:
		var id := int(entity.get("id", 0))
		if id != _local_id:
			current[id] = str(entity.get("n", "alguien"))

	for id: int in current:
		if not _seen.has(id):
			_hud.log_line("%s aparece en pantalla" % current[id], _hud.COLOR_TEXT)
	for id: int in _seen:
		if not current.has(id):
			_hud.log_line("%s sale de pantalla" % _seen[id], _hud.COLOR_TEXT_DIM)

	_seen = current
