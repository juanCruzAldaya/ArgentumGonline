extends Node
## WebSocket client for the juegito game server.
##
## Owns the socket and the JSON envelope protocol, and re-emits every server
## message as a signal. Nothing else in the client touches the socket, so
## swapping the transport later is a change to this file alone.

signal server_connected
signal server_disconnected
signal welcomed(welcome: Dictionary)
signal snapshot_received(snapshot: Dictionary)
signal loadout_received(loadout: Dictionary)
signal combat_received(event: Dictionary)
signal spell_received(event: Dictionary)
signal use_result_received(result: Dictionary)

var _socket := WebSocketPeer.new()
var _last_state := WebSocketPeer.STATE_CLOSED
var _player_name := ""
var _class_id := 0
var _race_id := 0


## class_id/race_id come straight from the character picker's own indices —
## they line up with the server's Class/Race enums (see ao_data.gd's
## CLASS_NAMES/RACE_NAMES) because both sides list them in the same order.
func connect_to_server(url: String, player_name: String, class_id: int, race_id: int) -> void:
	_player_name = player_name
	_class_id = class_id
	_race_id = race_id
	var err := _socket.connect_to_url(url)
	if err != OK:
		push_error("connect_to_url(%s) failed: %d" % [url, err])


## Leaves the match on purpose, as opposed to crashing out of it. The close
## frame is written during the poll, so one is pumped here: quitting the
## process right after this call would otherwise drop it and leave the server
## to notice the dead socket on its own read. It notices either way — this is
## the difference between saying goodbye and hanging up.
func disconnect_from_server() -> void:
	if _socket.get_ready_state() == WebSocketPeer.STATE_OPEN:
		_socket.close(1000, "salir")
		_socket.poll()


## seq is the sender's own monotonic counter (see WorldView._enqueue) — the
## server echoes the highest one it has answered in every snapshot's AckSeq, so
## prediction can reconcile against exactly what is still unconfirmed instead
## of guessing from position alone. See WorldView.set_entities.
func send_move(dir: int, seq: int) -> void:
	_send("move", {"dir": dir, "seq": seq})


## Ocultarse: no payload, no target — it's a self-only skill action gated
## entirely server-side (cooldown, not-already-hidden).
func send_hide() -> void:
	_send("hide", {})


## Meditar: no payload — toggles server-side, F6 in the original.
func send_meditate() -> void:
	_send("meditate", {})


## Agarrar: no payload — always takes from the tile the player stands on.
func send_pickup() -> void:
	_send("pickup", {})


## Tirar: drops one whole inventory slot onto the player's own tile.
func send_drop(slot: int) -> void:
	_send("drop", {"slot": slot})


## Reorders two bag positions — the inventory window's own drag-and-drop.
## Acting on a bag slot. action is "equip" (E, or the right-click menu) or
## "use" (U, or a double-click) — the server refuses an action that does not
## match the slot rather than silently doing the other one, which is the whole
## reason the client says which one it meant. Argentum's own overloaded click
## is still `""` in the protocol and the server still honours it; nothing here
## sends it.
func send_use_action(slot: int, action: String) -> void:
	_send("use", {"slot": slot, "a": action})


func send_swap(from_slot: int, to_slot: int) -> void:
	_send("swap", {"from": from_slot, "to": to_slot})


## Reordering the spell book. Same payload shape as send_swap, different
## message, because the bag and the book are separate lists server-side.
func send_swap_spell(from_slot: int, to_slot: int) -> void:
	_send("swapSpell", {"from": from_slot, "to": to_slot})


## Attacking carries no target: Argentum melee hits whatever stands on the tile
## you face, and the server works that out from your own heading.
func send_attack() -> void:
	_send("attack", {})


## The target is named here because Argentum spells reach across the screen.
## The server re-checks range, knowledge and cost regardless.
func send_cast(spell_id: int, target_id: int) -> void:
	_send("cast", {"spell": spell_id, "target": target_id})


func send_ping() -> void:
	_send("ping", {"t": Time.get_ticks_msec()})


func _send(type: String, data: Dictionary) -> void:
	if _socket.get_ready_state() != WebSocketPeer.STATE_OPEN:
		return
	_socket.send_text(JSON.stringify({"t": type, "d": data}))


func _process(_delta: float) -> void:
	_socket.poll()
	var state := _socket.get_ready_state()

	if state != _last_state:
		_last_state = state
		match state:
			WebSocketPeer.STATE_OPEN:
				# The server refuses anything before a join, so it goes first.
				_send("join", {"name": _player_name, "class": _class_id, "race": _race_id})
				server_connected.emit()
			WebSocketPeer.STATE_CLOSED:
				server_disconnected.emit()

	while state == WebSocketPeer.STATE_OPEN and _socket.get_available_packet_count() > 0:
		_handle_frame(_socket.get_packet().get_string_from_utf8())


func _handle_frame(text: String) -> void:
	var frame: Variant = JSON.parse_string(text)
	if typeof(frame) != TYPE_DICTIONARY:
		push_warning("undecodable frame from server")
		return

	var data: Dictionary = frame.get("d", {})
	match frame.get("t", ""):
		"welcome":
			welcomed.emit(data)
		"snapshot":
			snapshot_received.emit(data)
		"loadout":
			loadout_received.emit(data)
		"combat":
			combat_received.emit(data)
		"spell":
			spell_received.emit(data)
		"useResult":
			use_result_received.emit(data)
		"error":
			push_error("server rejected us: %s" % data.get("reason", "unknown"))
