extends Node2D
## Wires the network client to the view, the HUD and the minimap, and turns held
## keys into move commands.

const DEFAULT_URL := "ws://127.0.0.1:8080/ws"

## How often held keys are read. Zero: every frame.
##
## This was 50ms, which put a throttle in front of the input on top of the
## server's 50ms tick and the trip back — up to 150ms between pressing a key
## and seeing the character react, which is the lag that made this feel unlike
## the original. The rate limiting belongs to the walk cadence, which the
## server enforces and the prediction mirrors, not to how often the keyboard is
## sampled. Sampling every frame costs nothing: a command that arrives too
## early is refused by a cooldown either way.
const INPUT_INTERVAL := 0.0

@onready var _net: Node = $Net
@onready var _view: Node2D = $WorldView
@onready var _hud: Control = $UI/Screen
@onready var _minimap: Control = $UI/Screen/MinimapFrame/Minimap
@onready var _map_overlay: Control = $UI/Screen/MapOverlay
@onready var _chat: LineEdit = $UI/Screen/ChatInput
@onready var _outcome: Control = $UI/Screen/OutcomePanel

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

## Whether the server currently draws us as a ghost. Edge-detected off the
## local entity's own dead flag rather than off a message, because there is no
## respawn message on the wire: the server simply stops being dead, and that is
## all the client has to notice. See the server's respawn.go.
var _dead := false

## Own status, mirrored from the server every snapshot. The server is what
## actually blocks a paralyzed move or an immobilized swing; these exist so the
## client can say why locally instead of silently swallowing the input, and so
## held keys don't spam the server with commands it is only going to drop.
var _paralyzed := false
var _immobilized := false
var _invisible := false
var _meditating := false
## Debounces the "you can't do that" line the same way Argentum's own
## UltimoMensaje flag does: say it once when the key is first denied, not once
## per _process frame for as long as it's held.
var _told_blocked := false

## Edge-detection for the zone callouts, so a line is said once per change
## rather than once per snapshot.
var _zone_stage := -1
var _zone_shrinking := false
var _zone_safe := true

## Whether a Welcome has already landed. The second one is not an arrival: the
## server re-sends it to start the next match on a connection that never
## dropped. See _on_welcomed.
var _welcomed := false

## The account screen while it exists, and null once the player is past it.
## El campamento, mientras se espera una partida. Vive aparte de _login_screen
## porque no es parte del flujo de cuenta: se llega después de elegir personaje,
## y se vuelve a él cada vez que termina una partida.
var _lobby_screen: Control = null
## Si ya se eligió personaje en esta conexión. El servidor lo recuerda por
## asiento, así que después de una partida no hay que volver a elegirlo para
## encolarse de nuevo.
var _character_chosen := false
## La última ficha que mandó el servidor, guardada para poder dársela al
## campamento, que la dibuja en su columna derecha.
var _account: Dictionary = {}

## La pantalla del flujo de cuenta que esté arriba en este momento: inicio,
## entrar, registro o la ficha. Es una sola variable porque son excluyentes —
## nunca hay dos al mismo tiempo — y tenerlas en cuatro variables convertía cada
## transición en cuatro chequeos de nulo.
var _login_screen: Control = null
## El hello del servidor, guardado porque cada pantalla del flujo se configura
## con él y se crean de a una, a medida que hacen falta.
var _hello: Dictionary = {}


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
	_net.speech_received.connect(_on_speech)
	_net.outcome_received.connect(_on_outcome)
	_net.hello_received.connect(_on_hello)
	_net.lobby_received.connect(_on_lobby)
	_net.account_received.connect(_on_account)
	_net.login_failed.connect(_on_login_failed)
	_net.use_result_received.connect(_on_use_result)
	_hud.cast_requested.connect(_on_cast_requested)
	# Two panel gestures, two explicit messages. Nothing on the client sends the
	# original's overloaded click any more: the server still accepts it, but a
	# client that knows what it meant has no reason to make the server guess.
	_hud.item_used.connect(func(slot: int) -> void: _net.send_use_action(slot, "use"))
	_hud.item_equipped.connect(func(slot: int) -> void: _net.send_use_action(slot, "equip"))
	_hud.swap_requested.connect(_net.send_swap)
	_hud.spell_swap_requested.connect(_net.send_swap_spell)
	_hud.drop_requested.connect(_net.send_drop)
	_hud.quit_requested.connect(_on_quit_requested)
	_hud.map_requested.connect(_map_overlay.toggle)
	_chat.said.connect(_net.send_talk)

	# The world and the HUD have nothing to show until a character exists, so
	# they stay hidden — and the server stays untouched — until the picker
	# confirms a class and race.
	_view.visible = false
	_hud.visible = false

	# The socket comes first now. The server opens with a hello saying whether
	# it wants an account, and that decides which screen gets drawn — asking
	# before knowing would mean either a login form on a server that has no
	# accounts, or a character picker on one that refuses to let it in.
	_net.connect_to_server(_url, _player_name, 0, 0)


## The server said what it wants. Everything the player sees starts here.
func _on_hello(hello: Dictionary) -> void:
	_hello = hello
	if bool(hello.get("accounts", false)):
		_show_start()
	# Sin cuentas no hay nada que preguntar: el servidor ya sentó esta conexión
	# en el lobby, y el primer estado que mande trae el campamento a la
	# pantalla. Esperarlo en vez de adivinar es lo que evita dibujar un
	# campamento antes de saber si el servidor tiene uno.


## _swap_login_screen deja una sola pantalla del flujo de cuenta viva y la
## configura con el hello. Todas se construyen igual — nacen, se les pasa el
## hello, se enchufan sus señales — así que la parte común vive acá y cada
## _show_* solo dice qué conectar.
func _swap_login_screen(script_path: String) -> Control:
	if _login_screen != null:
		_login_screen.queue_free()
	var screen: Control = load(script_path).new()
	_login_screen = screen
	$UI.add_child(screen)
	screen.configure(_hello)
	return screen


func _show_start() -> void:
	var screen := _swap_login_screen("res://scripts/start_screen.gd")
	screen.sign_in_requested.connect(_show_sign_in)
	screen.register_requested.connect(_show_register)
	screen.quit_requested.connect(_on_quit_requested)


func _show_sign_in() -> void:
	var screen := _swap_login_screen("res://scripts/sign_in_screen.gd")
	screen.sign_in_submitted.connect(
		func(account: String, password: String) -> void:
			_net.send_login(account, password, false)
	)
	screen.back_requested.connect(_show_start)


func _show_register() -> void:
	var screen := _swap_login_screen("res://scripts/register_screen.gd")
	screen.register_submitted.connect(
		func(account: String, email: String, password: String) -> void:
			_net.send_login(account, password, true, email)
	)
	screen.back_requested.connect(_show_start)


## La ficha llegó, así que el login salió bien — se haya entrado o registrado.
##
## Si el que estaba arriba era el registro hay que cambiarlo por la pantalla de
## cuenta, que es la única que sabe dibujar una carrera. Recién registrado la
## carrera está vacía, y eso es exactamente lo que tiene que mostrar: cero
## partidas y "todavía no jugaste ninguna".
## La ficha llegó, así que el login salió bien — se haya entrado o registrado.
## Y entrar a tu cuenta es llegar al campamento: no hay una pantalla de ficha en
## el medio, porque la carrera vive en la columna derecha del lobby.
func _on_account(account: Dictionary) -> void:
	_account = account
	if _login_screen != null:
		_login_screen.queue_free()
		_login_screen = null
	_show_lobby()
	_lobby_screen.set_account(account)


func _on_login_failed(reason: String) -> void:
	if _login_screen != null and _login_screen.has_method("rejected"):
		_login_screen.rejected(reason)


## The career has been read and the player wants in.
func _on_play_requested() -> void:
	if _login_screen != null:
		_login_screen.queue_free()
		_login_screen = null
	_show_picker()


func _show_picker() -> void:
	var picker := preload("res://scripts/character_picker.gd").new()
	picker.default_nickname = _player_name
	$UI.add_child(picker)
	picker.confirmed.connect(_on_character_confirmed.bind(picker))


## El personaje está elegido. El join ya no entra al mundo: toma asiento en el
## campamento, y de ahí se decide cuándo jugar.
func _on_character_confirmed(player_name: String, class_id: int, race_id: int, picker: Control) -> void:
	picker.queue_free()

	_player_name = player_name
	_character_chosen = true
	_hud.set_character(_player_name)
	_hud.set_identity(class_id, race_id)
	# El join nombra el personaje y, con eso, toma el lugar en la cola.
	_net.send_join(_player_name, class_id, race_id)
	_show_lobby()


## _show_lobby abre el campamento, o lo trae de vuelta al terminar una partida.
func _show_lobby() -> void:
	_view.visible = false
	_hud.visible = false
	if _lobby_screen != null:
		return
	var screen: Control = preload("res://scripts/lobby_screen.gd").new()
	_lobby_screen = screen
	$UI.add_child(screen)
	screen.set_account(_account)
	screen.queue_toggled.connect(_on_queue_toggled)
	screen.quit_requested.connect(_on_quit_requested)


## UNIRSE A LA COLA. La primera vez hay que elegir personaje, porque el join es
## justamente lo que nombra el personaje y toma el lugar en la fila; después ya
## está elegido y encolarse es un mensaje suelto.
func _on_queue_toggled(join: bool) -> void:
	if not join:
		_net.send_queue(false)
		return
	if _character_chosen:
		_net.send_queue(true)
		return
	_show_picker()


## El estado de la cola. Que llegue uno de estos es, por sí solo, la señal de
## que no se está jugando: el servidor deja de mandarlos al entrar al mundo y
## los vuelve a mandar al terminar la partida, así que esto también es cómo el
## cliente se entera de que volvió al campamento.
func _on_lobby(state: Dictionary) -> void:
	_show_lobby()
	_lobby_screen.set_lobby(state)


func _process(delta: float) -> void:
	_time_since_input += delta
	if not _connected or _time_since_input < INPUT_INTERVAL:
		return

	# Movement and attacking read the keyboard directly rather than through
	# events, so an open chat box cannot swallow them the way it swallows
	# everything else — it has to be checked here explicitly, or typing a word
	# with a "w" in it walks you north.
	if _chat.is_open():
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
		# Move first, tell the server second — the order Argentum's own
		# Map_MoveTo uses. The character reacts on the frame the key is read
		# instead of a round trip later; see WorldView.try_step.
		#
		# try_step itself decides whether this input is worth a packet — held
		# against a wall or mid-turn-cooldown comes back -1 and nothing is
		# sent, instead of flooding the connection at framerate the way
		# sending unconditionally here used to. It is still the server's
		# answer that is authoritative, never ours; see WorldView.set_entities.
		var seq: int = _view.try_step(dir)
		if seq >= 0:
			_net.send_move(dir, seq)


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


## Zone callouts. The ring is public information — the whole mechanic is that
## everyone can see it coming — so this only says out loud what the circle on
## screen already shows, at the two moments a player might be looking elsewhere:
## when it starts moving, and when they are the one standing outside it.
func _announce_zone(zone: Variant) -> void:
	if typeof(zone) != TYPE_DICTIONARY or zone.is_empty():
		_zone_stage = -1
		return

	var stage := int(zone.get("st", 0))
	var shrinking: bool = bool(zone.get("s", false))
	if shrinking != _zone_shrinking or stage != _zone_stage:
		_zone_stage = stage
		_zone_shrinking = shrinking
		if shrinking:
			_hud.log_line("¡La zona se está cerrando!", _hud.COLOR_EXP)
		elif float(zone.get("nr", 0.0)) > 0.0:
			_hud.log_line(
				"La zona se cierra en %d segundos." % int(zone.get("t", 0.0)), _hud.COLOR_ACCENT
			)

	var me: Variant = _view.local_tile()
	if me == null:
		return
	var safe: bool = _view.in_safe_zone(me)
	if safe != _zone_safe:
		_zone_safe = safe
		if safe:
			_hud.log_line("Estás a salvo dentro de la zona.", _hud.COLOR_MANA)
		else:
			_hud.log_line("¡Estás fuera de la zona! Corré al círculo.", _hud.COLOR_EXP)


## How the match ended for us. The panel takes the card; the console gets the
## one line that is worth keeping in the log after the card is dismissed.
func _on_outcome(outcome: Dictionary) -> void:
	_outcome.show_outcome(outcome)

	var place := int(outcome.get("place", 0))
	var players := int(outcome.get("of", 0))
	if bool(outcome.get("won", false)):
		_hud.log_line("¡Ganaste! Último en pie de %d." % players, _hud.COLOR_ACCENT)
		return

	var winner := str(outcome.get("winner", ""))
	if winner != "":
		_hud.log_line("Ganó %s. Quedaste #%d de %d." % [winner, place, players], _hud.COLOR_EXP)
	else:
		_hud.log_line("Eliminado: #%d de %d." % [place, players], _hud.COLOR_EXP)


## Words over somebody's head. The same handler for chat and for a spell's
## incantation, because the server sends one message for both — see
## protocol.Speech.
func _on_speech(speech: Dictionary) -> void:
	_view.set_speech(
		int(speech.get("id", 0)),
		Vector2i(int(speech.get("x", 0)), int(speech.get("y", 0))),
		str(speech.get("text", "")),
		bool(speech.get("spell", false))
	)


func _on_spell(event: Dictionary) -> void:
	var failed := str(event.get("failed", ""))
	if failed != "":
		_hud.log_line(failed, _hud.COLOR_TEXT_DIM)
		return

	var mine: bool = bool(event.get("mine", false))
	var caster := str(event.get("cn", "alguien"))
	var victim := str(event.get("vn", "alguien"))
	var spell := str(event.get("sn", "un hechizo"))

	_view.play_spell_fx(int(event.get("v", 0)), int(event.get("s", 0)))

	# The magic words used to be logged here. They are not any more: the server
	# now broadcasts them as speech to everyone who can see the caster, and the
	# client draws them over the caster's head — which is where Argentum puts
	# them, and the reason casting gives your position away. Logging them too
	# would say it twice, and only to the two people who already knew.

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
	# Enter opens the chat box, and the box takes it from there — it has focus
	# while open, so its own submit and cancel never reach this function.
	if not _chat.is_open() and event is InputEventKey and event.pressed and not event.echo:
		if event.keycode == KEY_ENTER or event.keycode == KEY_KP_ENTER:
			if _connected:
				_chat.open()
				get_viewport().set_input_as_handled()
			return

	# The result card is the first thing Escape should take away — it is the
	# newest thing on screen and the only one that appeared without being asked
	# for.
	if _outcome.visible and event is InputEventKey and event.pressed and not event.echo:
		if event.keycode == KEY_ESCAPE:
			_outcome.close()
			get_viewport().set_input_as_handled()
			return

	# The map is a modal read of something you already have: while it is open it
	# eats Escape and M so neither leaks into targeting or movement, and every
	# other key still works, because closing the map should never be the price
	# of drinking a potion.
	if _map_overlay.visible and event is InputEventKey and event.pressed and not event.echo:
		if event.keycode == KEY_ESCAPE or event.keycode == KEY_M:
			_map_overlay.close()
			get_viewport().set_input_as_handled()
			return

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
					# The double-click is U's gesture, so E is the only way to
					# put something on from the keyboard.
					var action := "equip" if event.keycode == KEY_E else "use"
					_net.send_use_action(slot, action)
			KEY_T:
				# Tirar: drops the whole selected slot on the spot. The original
				# opens a quantity dialog for a stack over 1 (frmCantidad); this
				# game has no partial-stack UI anywhere else either, so T always
				# drops the entire stack, same as dropping from the context menu.
				var drop_slot: int = _hud.selected_slot()
				if drop_slot < 0:
					_hud.log_line("Primero seleccioná un objeto del inventario.", _hud.COLOR_TEXT_DIM)
				else:
					_net.send_drop(drop_slot)
			KEY_F6:
				_net.send_meditate()
			KEY_M:
				# The world map. Not a key the original binds — it has no world
				# map to bind, since a map there is 100x100 and the minimap
				# shows all of it. A composed world is 820x820, where the
				# corner minimap gives about a sixth of a pixel per tile.
				_map_overlay.toggle()

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

	# The click is spent either way. Argentum clears UsingSkill unconditionally
	# after a left click (frmMain.frm), so missing costs you the cast and you
	# have to arm the spell again — a miss is a mistake with a price, not a
	# free retry with the crosshair still up.
	var target: int = _view.entity_at(_view.to_local(event.position))
	if target == 0:
		_hud.log_line("No hay nadie ahí. Volvé a elegir el hechizo.", _hud.COLOR_TEXT_DIM)
		_stop_targeting()
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


## SALIR and the X in the panel's title bar. Leaving the match means both
## halves: the socket closes so the server drops the player right away — the
## body and the inventory hit the floor for whoever is still there — and then
## the client itself goes.
##
## The same web caveat as the login screen's SALIR (character_picker.gd): a
## page cannot close a tab it did not open, so window.close() is attempted and
## silently does nothing outside an embedded context. The disconnect is the
## half that always works, which is the half that matters to everyone else in
## the match.
func _on_quit_requested() -> void:
	_net.disconnect_from_server()
	if OS.has_feature("web"):
		JavaScriptBridge.eval("window.close()", true)
	else:
		get_tree().quit()


func _on_welcomed(welcome: Dictionary) -> void:
	# El Welcome es el momento exacto en que la espera terminó: el campamento se
	# va y aparecen el mundo y el HUD. Se destruye en vez de esconderse porque
	# la carrera que dibuja quedó vieja apenas empieza la partida, y el próximo
	# estado de lobby lo va a crear de nuevo con la ficha al día.
	if _lobby_screen != null:
		_lobby_screen.queue_free()
		_lobby_screen = null
	_view.visible = true
	_hud.visible = true

	_local_id = int(welcome.get("id", 0))
	_view.configure(welcome)
	_minimap.configure(welcome)
	_map_overlay.configure(welcome, _minimap.terrain_texture())
	_hud.set_spell_slots(int(welcome.get("spellSlots", 0)))

	# A second Welcome on a live connection is a match restart, not an arrival.
	# Everything that belongs to the match just finished goes here: the result
	# card, and the edge detection that decides when to call the zone out.
	if _welcomed:
		_outcome.close()
		_zone_stage = -1
		_zone_shrinking = false
		_zone_safe = true
		_hud.log_line("Nueva partida. Suerte.", _hud.COLOR_ACCENT)
		return
	_welcomed = true

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

	_view.set_entities(entities, int(snapshot.get("ack", 0)), int(snapshot.get("tick", 0)))
	_view.set_ground(snapshot.get("g", []))
	_view.set_zone(snapshot.get("z", {}))
	_map_overlay.set_zone(snapshot.get("z", {}))
	_announce_zone(snapshot.get("z", {}))
	_minimap.set_entities(entities)
	_map_overlay.set_entities(entities)
	_hud.set_alive(int(snapshot.get("alive", 0)))

	var vitals: Variant = snapshot.get("self")
	if typeof(vitals) == TYPE_DICTIONARY:
		_hud.set_vitals(vitals)
		_update_own_status(vitals)

	_update_zone(entities)
	_update_own_life(entities)
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
			_hud.set_kills(int(entity.get("k", 0)))
			return


## Says one line when we come back from the dead. Dying already narrates
## itself through the combat event ("¡X te ha matado!"), so only the return
## needs saying — and it needs saying, because otherwise the whole event is a
## body that silently teleports to the middle of the map.
func _update_own_life(entities: Array) -> void:
	for entity in entities:
		if int(entity.get("id", 0)) != _local_id:
			continue
		var dead: bool = bool(entity.get("d", false))
		if _dead and not dead:
			_hud.log_line("Has vuelto a la vida en el centro del mapa.", _hud.COLOR_HP)
		_dead = dead
		return


## Edge-detects status transitions from the raw booleans the server sends
## every tick, so the console gets one line when a status starts or ends
## rather than one every snapshot for as long as it lasts.
func _update_own_status(vitals: Dictionary) -> void:
	var paralyzed: bool = vitals.get("paralyzed", false)
	var immobilized: bool = vitals.get("immobilized", false)
	var invisible: bool = vitals.get("invisible", false)
	var meditating: bool = vitals.get("meditating", false)

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

	if meditating and not _meditating:
		_hud.log_line("Te estás concentrando para meditar.", _hud.COLOR_MANA)
	elif _meditating and not meditating:
		_hud.log_line("Dejás de meditar.", _hud.COLOR_TEXT_DIM)

	_paralyzed = paralyzed
	_immobilized = immobilized
	_invisible = invisible
	_meditating = meditating
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
