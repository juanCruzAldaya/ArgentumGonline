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

var _socket := WebSocketPeer.new()
var _last_state := WebSocketPeer.STATE_CLOSED
var _player_name := ""


func connect_to_server(url: String, player_name: String) -> void:
	_player_name = player_name
	var err := _socket.connect_to_url(url)
	if err != OK:
		push_error("connect_to_url(%s) failed: %d" % [url, err])


func send_move(dir: int) -> void:
	_send("move", {"dir": dir})


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
				_send("join", {"name": _player_name})
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
		"error":
			push_error("server rejected us: %s" % data.get("reason", "unknown"))
