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
## Fed into the TopBar's Zone label alongside the player's own tile every
## snapshot — see _on_snapshot. Empty for the generated demo arena, which has
## no name of its own.
var _map_name := ""
var _time_since_input := 0.0
var _connected := false
## Spell awaiting a target. Argentum casts in two steps — pick the spell, then
## pick who it lands on — and the second click is what this holds open.
var _targeting_spell := 0
## Entities seen in the previous snapshot, so the console can report who walked
## into and out of view. Diffing here keeps the server free of an event stream
## it does not need yet.
var _seen: Dictionary = {}

## Own status, mirrored from the server every snapshot. The server is what
## actually blocks a paralyzed move or an immobilized swing; these exist so the
## client can say why locally instead of silently swallowing the input, and so
## held keys don't spam the server with commands it is only going to drop.
var _paralyzed := false
var _immobilized := false
var _invisible := false
## Debounces the "you can't do that" line the same way Argentum's own
## UltimoMensaje flag does: say it once when the key is first denied, not once
## per _process frame for as long as it's held.
var _told_blocked := false


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
	_net.spell_received.connect(_on_spell)
	_net.use_result_received.connect(_on_use_result)
	_hud.cast_requested.connect(_on_cast_requested)
	_hud.item_used.connect(_net.send_use)
	_hud.swap_requested.connect(_net.send_swap)
	_hud.spell_swap_requested.connect(_net.send_swap_spell)
	_hud.drop_requested.connect(_net.send_drop)

	# The world and the HUD have nothing to show until a character exists, so
	# they stay hidden — and the server stays untouched — until the picker
	# confirms a class and race.
	_view.visible = false
	_hud.visible = false

	var picker := preload("res://scripts/character_picker.gd").new()
	$UI.add_child(picker)
	picker.confirmed.connect(_on_character_confirmed.bind(picker))


func _on_character_confirmed(class_id: int, race_id: int, picker: Control) -> void:
	picker.queue_free()
	_view.visible = true
	_hud.visible = true

	_hud.set_character(_player_name)
	_hud.set_identity(class_id, race_id)
	_hud.log_line("conectando a %s ..." % _url, _hud.COLOR_TEXT_DIM)
	_net.connect_to_server(_url, _player_name, class_id, race_id)


func _process(delta: float) -> void:
	_time_since_input += delta
	if not _connected or _time_since_input < INPUT_INTERVAL:
		return

	# Ctrl swings, as in Argentum. Immobilize roots the feet, not the arms —
	# only paralysis stops a swing, matching the server's canAct.
	if Input.is_key_pressed(KEY_CTRL):
		_time_since_input = 0.0
		if _paralyzed:
			_tell_blocked("No podés atacar, estás paralizado.")
			return
		_net.send_attack()
		return

	var dir := _pressed_direction()
	if dir >= 0:
		_time_since_input = 0.0
		if _paralyzed or _immobilized:
			_tell_blocked(
				"No podés moverte, estás paralizado."
				if _paralyzed
				else "No podés moverte, estás inmovilizado."
			)
			return
		_net.send_move(dir)


func _tell_blocked(text: String) -> void:
	if _told_blocked:
		return
	_told_blocked = true
	_hud.log_line(text, _hud.COLOR_TEXT_DIM)


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


func _on_spell(event: Dictionary) -> void:
	var failed := str(event.get("failed", ""))
	if failed != "":
		_hud.log_line(failed, _hud.COLOR_TEXT_DIM)
		return

	var mine: bool = bool(event.get("mine", false))
	var caster := str(event.get("cn", "alguien"))
	var victim := str(event.get("vn", "alguien"))
	var spell := str(event.get("sn", "un hechizo"))
	var words := str(event.get("w", ""))

	# The magic words are half of what a spell feels like in Argentum.
	if words != "":
		_hud.log_line("%s: ¡%s!" % [caster, words], _hud.COLOR_MANA)

	var damage := int(event.get("dmg", 0))
	var healed := int(event.get("heal", 0))
	if damage > 0:
		if mine:
			_hud.log_line("Has lanzado %s sobre %s. Le quitaste %d puntos de vida." % [spell, victim, damage], _hud.COLOR_HP)
		else:
			_hud.log_line("%s te ha lanzado %s. Te quitó %d puntos de vida." % [caster, spell, damage], _hud.COLOR_EXP)
	elif healed > 0:
		if mine:
			_hud.log_line("Has curado %d puntos de vida a %s." % [healed, victim], _hud.COLOR_HP)
		else:
			_hud.log_line("%s te ha curado %d puntos de vida." % [caster, healed], _hud.COLOR_HP)

	if bool(event.get("killed", false)):
		if mine:
			_hud.log_line("¡Has matado a %s!" % victim, _hud.COLOR_ACCENT)
		else:
			_hud.log_line("¡%s te ha matado!" % caster, _hud.COLOR_EXP)

	if bool(event.get("paralyzed", false)):
		if mine:
			_hud.log_line("Has paralizado a %s." % victim, _hud.COLOR_MANA)
		else:
			_hud.log_line("¡%s te ha paralizado!" % caster, _hud.COLOR_EXP)
	if bool(event.get("immobilized", false)):
		if mine:
			_hud.log_line("Has inmovilizado a %s." % victim, _hud.COLOR_MANA)
		else:
			_hud.log_line("¡%s te ha inmovilizado!" % caster, _hud.COLOR_EXP)
	if bool(event.get("removedParalysis", false)):
		if mine:
			_hud.log_line("Has devuelto la movilidad a %s." % victim, _hud.COLOR_HP)
		else:
			_hud.log_line("%s te ha devuelto la movilidad." % caster, _hud.COLOR_HP)
	if bool(event.get("invisible", false)):
		if not mine:
			_hud.log_line("%s te ha vuelto invisible." % caster, _hud.COLOR_MANA)
		elif caster == victim:
			_hud.log_line("Te has vuelto invisible.", _hud.COLOR_ACCENT)
		else:
			_hud.log_line("Has vuelto invisible a %s." % victim, _hud.COLOR_HP)

	var ag := int(event.get("agDelta", 0))
	if ag > 0:
		_log_attribute(mine, caster, victim, "agilidad", "aumentado", _hud.COLOR_HP)
	elif ag < 0:
		_log_attribute(mine, caster, victim, "agilidad", "reducido", _hud.COLOR_EXP)

	var fu := int(event.get("fuDelta", 0))
	if fu > 0:
		_log_attribute(mine, caster, victim, "fuerza", "aumentado", _hud.COLOR_HP)
	elif fu < 0:
		_log_attribute(mine, caster, victim, "fuerza", "reducido", _hud.COLOR_EXP)


## Buff/debuff narration shares the same "quién hizo qué a quién" shape four
## times over (agility up, agility down, strength up, strength down), so it is
## factored out rather than repeated.
func _log_attribute(mine: bool, caster: String, victim: String, stat: String, verb: String, color: Color) -> void:
	if mine and caster == victim:
		_hud.log_line("Tu %s ha %s." % [stat, verb], color)
	elif mine:
		_hud.log_line("Le has %s la %s a %s." % [verb, stat, victim], color)
	else:
		_hud.log_line("%s te ha %s la %s." % [caster, verb, stat], color)


## UseResult narration mirrors the source's own phrasing for the shape each
## outcome takes ("Has bebido...", "Te has equipado...") rather than the
## attacker/victim framing combat and spells use — using an item only ever
## affects yourself.
func _on_use_result(result: Dictionary) -> void:
	var failed := str(result.get("failed", ""))
	if failed != "":
		_hud.log_line(failed, _hud.COLOR_TEXT_DIM)
		return

	var item_name := str(result.get("item", "el objeto"))

	if bool(result.get("equipped", false)):
		_hud.log_line("Te has equipado: %s." % item_name, _hud.COLOR_ACCENT)
		return
	if bool(result.get("unequipped", false)):
		_hud.log_line("Te has quitado: %s." % item_name, _hud.COLOR_TEXT_DIM)
		return

	if bool(result.get("died", false)):
		_hud.log_line("Sientes un gran mareo y pierdes el conocimiento.", _hud.COLOR_EXP)
		return

	var heal := int(result.get("healHp", 0))
	if heal > 0:
		_hud.log_line("Has bebido %s. Recuperaste %d puntos de vida." % [item_name, heal], _hud.COLOR_HP)
	var mana := int(result.get("restoredMana", 0))
	if mana > 0:
		_hud.log_line("Has bebido %s. Recuperaste %d puntos de maná." % [item_name, mana], _hud.COLOR_MANA)
	var hunger := int(result.get("restoredHunger", 0))
	if hunger > 0:
		_hud.log_line("Has comido %s. Recuperaste %d de hambre." % [item_name, hunger], _hud.COLOR_HUNGER)
	var thirst := int(result.get("restoredThirst", 0))
	if thirst > 0:
		_hud.log_line("Has bebido %s. Recuperaste %d de sed." % [item_name, thirst], _hud.COLOR_THIRST)

	var ag := int(result.get("agDelta", 0))
	if ag != 0:
		_hud.log_line(
			"Has bebido %s. Tu agilidad ha %s." % [item_name, "aumentado" if ag > 0 else "disminuido"],
			_hud.COLOR_HP if ag > 0 else _hud.COLOR_EXP
		)
	var fu := int(result.get("fuDelta", 0))
	if fu != 0:
		_hud.log_line(
			"Has bebido %s. Tu fuerza ha %s." % [item_name, "aumentado" if fu > 0 else "disminuido"],
			_hud.COLOR_HP if fu > 0 else _hud.COLOR_EXP
		)

	if bool(result.get("curedPoison", false)):
		_hud.log_line("No estás envenenado.", _hud.COLOR_TEXT_DIM)


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
	# Ocultarse (O), Agarrar (A) and Usar/Equipar (U/E): Argentum's own hotkeys
	# for these, each a discrete "try once" trigger on the key edge rather than
	# the held-and-repeated polling _process() uses for movement/attack —
	# holding the key down should not spam the server with an attempt every
	# frame. U and E used to send the identical action and let the item's own
	# type pick the branch; they now say which they mean, because a key you
	# press expecting to put a sword on should never drink a potion instead.
	# The overloaded action still exists and is what a double-click sends.
	if _connected and event is InputEventKey and event.pressed and not event.echo:
		match event.keycode:
			KEY_O:
				if _paralyzed:
					_tell_blocked("No podés ocultarte, estás paralizado.")
				else:
					_net.send_hide()
			KEY_A:
				_net.send_pickup()
			KEY_U, KEY_E:
				var slot: int = _hud.selected_slot()
				if slot < 0:
					_hud.log_line("Primero seleccioná un objeto del inventario.", _hud.COLOR_TEXT_DIM)
				else:
					# E equips and only equips; U consumes and only consumes.
					# They used to send the identical message and let the item
					# type decide, which meant pressing E on a potion drank it.
					# A double-click still sends the overloaded action.
					var action := "equip" if event.keycode == KEY_E else "use"
					_net.send_use_action(slot, action)

	if _targeting_spell == 0:
		_handle_inspect_click(event)
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

	_net.send_cast(_targeting_spell, target)
	_stop_targeting()


## A left click on the world with no spell armed inspects whatever is under it:
## Argentum answers a click on a character with "Nombre <Clan> <Descripción>",
## and this extends the same gesture to a pile on the floor, which is now the
## only way to learn what one holds since the counts came off the tiles.
##
## Characters win over ground when both are under the cursor — somebody
## standing on a pile is the more interesting answer, and the pile is still
## reachable by stepping off it.
func _handle_inspect_click(event: InputEvent) -> void:
	if not (event is InputEventMouseButton and event.pressed and event.button_index == MOUSE_BUTTON_LEFT):
		return
	var local: Vector2 = _view.to_local(event.position)
	if not _view.view_rect().has_point(local):
		return

	var id: int = _view.entity_at(local)
	if id != 0:
		_log_character(_view.entity_info(id))
		return

	var stack: Dictionary = _view.ground_at(local)
	if not stack.is_empty():
		_log_ground(stack)


## Argentum's own inspect line. The clan bracket is omitted when there is no
## clan — which today is always, since no guild system exists — exactly as the
## original does for a clanless character.
func _log_character(info: Dictionary) -> void:
	if info.is_empty():
		return
	var line := str(info["name"])
	var clan := str(info["clan"])
	if clan != "":
		line += " <%s>" % clan
	var desc := str(info["desc"])
	if desc != "":
		line += " <%s>" % desc
	if bool(info["dead"]):
		line += " <muerto>"
	line += " - Kills: %d" % int(info["kills"])
	_hud.log_line(line, _hud.COLOR_ACCENT)


func _log_ground(stack: Dictionary) -> void:
	var item: Dictionary = _hud.item_data(int(stack["item_id"]))
	var label := "objeto %d" % int(stack["item_id"]) if item.is_empty() else str(item.get("name", ""))
	_hud.log_line("En el piso: %s (%d)." % [label, int(stack["amount"])], _hud.COLOR_TEXT_DIM)


## Raw key checks rather than input actions, so the project needs no input map
## and the controls read the same as the server's heading constants.
##
## West drops the WASD alt (KEY_A) that the other three directions keep: A is
## Agarrar, Argentum's own pickup hotkey, and holding it to walk west would
## fire a pickup attempt on every step. Classic AO only ever used the arrow
## keys for movement anyway — this just stops being wrong about it on one side.
func _pressed_direction() -> int:
	if Input.is_key_pressed(KEY_UP) or Input.is_key_pressed(KEY_W):
		return 0  # north
	if Input.is_key_pressed(KEY_RIGHT) or Input.is_key_pressed(KEY_D):
		return 1  # east
	if Input.is_key_pressed(KEY_DOWN) or Input.is_key_pressed(KEY_S):
		return 2  # south
	if Input.is_key_pressed(KEY_LEFT):
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
	_hud.set_spell_slots(int(welcome.get("spellSlots", 0)))

	_map_name = str(welcome.get("mapName", ""))
	if _map_name != "":
		_hud.log_line("Has llegado a %s." % _map_name, _hud.COLOR_ACCENT)
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
	_view.set_ground(snapshot.get("g", []))
	_minimap.set_entities(entities)
	_hud.set_alive(int(snapshot.get("alive", 0)))

	var vitals: Variant = snapshot.get("self")
	if typeof(vitals) == TYPE_DICTIONARY:
		_hud.set_vitals(vitals)
		_update_own_status(vitals)

	_update_zone(entities)
	_report_arrivals_and_departures(entities)


## The bottom status readout classic AO always shows: where you actually are.
## Pulled from the entity list rather than tracked separately — the server
## already puts the local player in their own snapshot, so there's no reason
## to duplicate that position client-side.
func _update_zone(entities: Array) -> void:
	for entity in entities:
		if int(entity.get("id", 0)) == _local_id:
			var label := _map_name if _map_name != "" else "mapa"
			_hud.set_zone("%s   X:%d  Y:%d" % [label, int(entity.get("x", 0)), int(entity.get("y", 0))])
			return


## Edge-detects status transitions from the raw booleans the server sends
## every tick, so the console gets one line when a status starts or ends
## rather than one every snapshot for as long as it lasts.
func _update_own_status(vitals: Dictionary) -> void:
	var paralyzed: bool = vitals.get("paralyzed", false)
	var immobilized: bool = vitals.get("immobilized", false)
	var invisible: bool = vitals.get("invisible", false)

	if paralyzed and not _paralyzed:
		_hud.log_line("¡Estás paralizado!", _hud.COLOR_EXP)
	elif _paralyzed and not paralyzed:
		_hud.log_line("Ya no estás paralizado.", _hud.COLOR_HP)
		_told_blocked = false

	if immobilized and not _immobilized:
		_hud.log_line("¡Estás inmovilizado!", _hud.COLOR_EXP)
	elif _immobilized and not immobilized:
		_hud.log_line("Ya no estás inmovilizado.", _hud.COLOR_HP)
		_told_blocked = false

	if invisible and not _invisible:
		_hud.log_line("Eres invisible.", _hud.COLOR_ACCENT)
	elif _invisible and not invisible:
		_hud.log_line("Ya no eres invisible.", _hud.COLOR_TEXT_DIM)

	_paralyzed = paralyzed
	_immobilized = immobilized
	_invisible = invisible
	_view.local_invisible = invisible


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
