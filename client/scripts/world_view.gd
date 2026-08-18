extends Node2D
## Draws the tile grid and every entity the server reported.
##
## The server is authoritative over whole tiles, but rendering a character
## teleporting one tile at a time looks broken. So entities keep a fractional
## render position that chases their server tile at walk speed: the simulation
## stays discrete, only the picture is smoothed. Nothing here is ever sent back,
## so this cannot desync anything.

## Argentum's native tile. The character art is drawn for this size, which is
## why the node is scaled up rather than the tiles being made bigger.
const TILE_SIZE := 32

## Tiles per second, sent by the server in the Welcome. Running faster than the
## server allows would make characters arrive and then wait out the rest of the
## cooldown, a visible stutter at every tile edge; running slower would make
## them fall further behind with every step.
##
## This used to be a hardcoded 5.0, correct only while the server's cooldown
## was a flat 4 ticks. The moment walk speed became tunable a second copy of it
## over here was a bug waiting for the first retune, so the server is now the
## only place the number lives. The default matches classic Argentum, and only
## applies before the Welcome lands.
var walk_speed := 5.0

## Past this distance a move is a teleport or a resync, not a walk, so the
## render position snaps instead of sliding across the map.
const SNAP_DISTANCE := 2.5

## How far the neck sinks into the shoulders. Butting the two exactly together
## leaves a visible seam at this sprite scale.

const COLOR_VOID := Color(0.04, 0.04, 0.06)
const COLOR_FLOOR := Color(0.16, 0.18, 0.15)
const COLOR_WALL := Color(0.30, 0.26, 0.22)
const COLOR_GRID := Color(0, 0, 0, 0.18)
const COLOR_NAME := Color(0.92, 0.92, 0.88)
const COLOR_NAME_LOCAL := Color(0.62, 0.92, 0.62)
const COLOR_NAME_SHADOW := Color(0, 0, 0, 0.85)
## Argentum's own colour for a spell's incantation, RGB(200, 250, 150) in
## Protocol.bas's HandlePalabrasMagicas.
const COLOR_SPELL_WORDS := Color(200.0 / 255.0, 250.0 / 255.0, 150.0 / 255.0)
const COLOR_SPEECH := Color(0.94, 0.92, 0.80)
## How long a sign stays up. The original computes it per message —
## MS_ADD_EXTRA + MS_PER_CHAR * len, 5000 + 100 per character in clsDialogs.cls
## — so a longer line lingers longer.
const SPEECH_BASE_SECONDS := 5.0
const SPEECH_PER_CHAR_SECONDS := 0.1
## Wrap width, MAX_LENGTH in clsDialogs.cls.
const SPEECH_WRAP_CHARS := 18
const SPEECH_SIZE := 11

## The shrinking zone. Blue and translucent so the ground stays readable through
## it — you have to be able to see what you are running over — with a brighter
## edge that pulses, because the wall is the thing you need to find at a glance.
const COLOR_ZONE_FILL := Color(0.20, 0.45, 0.95, 0.26)
const COLOR_ZONE_EDGE := Color(0.55, 0.85, 1.00, 0.90)
const COLOR_ZONE_NEXT := Color(0.85, 0.95, 1.00, 0.55)
## Segments around the ring. 64 is smooth at any radius the world reaches and
## is 64 quads a frame, which is nothing.
const ZONE_SEGMENTS := 64
const COLOR_SHADOW := Color(0, 0, 0, 0.25)
const COLOR_TARGET := Color(0.95, 0.35, 0.30)
const COLOR_CORPSE := Color(0.45, 0.42, 0.45, 0.85)
const COLOR_PARALYZED_RING := Color(0.55, 0.80, 0.95)
const COLOR_IMMOBILIZED_RING := Color(0.55, 0.80, 0.40)
const COLOR_INVISIBLE_SELF := Color(1, 1, 1, 0.4)

## Fallback marks, used only for entities whose appearance was not bundled.
const COLOR_LOCAL := Color(0.45, 0.80, 0.45)
const COLOR_OTHER := Color(0.80, 0.40, 0.35)

var map_width := 0
var map_height := 0
var view_w := 17
var view_h := 13
var local_id := 0

var _sprites := AOSprites.new()
var _data := AOData.new()
var _blocked := PackedByteArray()
## id -> { tile, render, heading, body, head, name, anim, moving }
var _entities: Dictionary = {}
## "x,y" -> { item_id, amount }. Only what the server put in the last
## snapshot's Ground list — the same viewport-limited interest management as
## _entities, so nothing here reveals loot the player hasn't actually seen.
var _ground: Dictionary = {}

## One sign per entity id, exactly as Argentum keeps one dialog per CharIndex.
## A new line replaces the old rather than stacking, which is what lets a player
## wipe a spell's incantation off their own head by saying anything at all —
## including a single space.
var _speech: Dictionary = {}

## The zone as the server last described it, or empty when the match has none.
var _zone: Dictionary = {}
var _camera := Vector2.ZERO

## One-shot spell effects in flight: { entity, grh, offset, start, until }.
## Argentum plays these anchored to the target's own position, not as a
## projectile — see play_spell_fx.
var _active_fx: Array = []

## Argentum tile layers, loaded from the map the server named in its welcome.
## Layer 1 covers every tile so it is dense; the rest are sparse index -> grh.
var _has_map := false
var _layer1 := PackedInt32Array()
var _layer2: Dictionary = {}
var _layer3: Dictionary = {}
var _layer4: Dictionary = {}
## Tile index -> true for every tile the map marks BAJOTECHO or CASA. Drives
## whether the roof layer is drawn at all this frame; see _draw_layers.
var _roofed: Dictionary = {}

## Tile offset per heading, in the server's own order: 0 N, 1 E, 2 S, 3 W.
const HEADING_DELTA := [Vector2(0, -1), Vector2(1, 0), Vector2(0, 1), Vector2(-1, 0)]

## Server ticks per second, from the Welcome. The server can only ever act on
## a tick boundary, so every clock below is quantised to ticks of this length
## rather than measured in continuous real time — see _quantized_now for why
## that distinction is the whole fix for a very specific stutter.
var tick_rate := 20
var _tick_ms := 1000.0 / 20.0

## Local step clock for prediction, mirroring the server's walk cadence.
## Unlike a plain "next allowed second" this is compared and advanced entirely
## in _quantized_now's ticked time, for the same reason moveReadyAt is —
## see _quantized_now.
var _local_step_ready_at := 0.0
## moveCooldownMilliticks server-side, converted to real milliseconds
## (still tick-quantised in effect once compared against _quantized_now).
## Server.ini's walkSpeedPercent lives only on the server; this is derived
## from the Welcome's walkSpeed instead of a second copy of that constant.
var _move_cooldown_ms := 222.2

## Local turn clock for a step refused mid-cadence, mirroring the server's own
## INT_CHANGE_HEADING (see World.turn server-side). Kept separate from the step
## clock for the same reason the server keeps them separate: a step carries its
## own turn for free, but a refused step must not let the character spin at
## framerate either.
var _local_turn_ready_at := 0.0
## Matches turnCooldownMilliticks server-side (6 ticks) exactly rather than a
## round 300ms, so it stays a whole number of ticks even if tick_rate is ever
## retuned away from 20.
var _turn_cooldown_ms := 300.0

## A rejected step (wall, occupied tile) is not retried every single frame a
## key stays held — that would flood the connection for no gain, since the
## server only ever re-evaluates a blocked step once per tick anyway. One tick
## is the natural retry window: sooner is pure waste, since the server cannot
## have changed its mind before then.
var _blocked_retry_ms := 50.0

## Estimated (server clock) - (local Time.get_ticks_msec()) in milliseconds,
## refreshed from Snapshot.Tick on every snapshot via sync_server_tick.
##
## The server can only grant a step on one of its own tick boundaries, and
## because 222.2ms (the walk cooldown) is not a whole multiple of 50ms (the
## tick), the server itself alternates 250ms/200ms/250ms/200ms steps to
## average it out (see moveCooldownMilliticks). A client predicting on a
## plain continuous 222.2ms timer is not approximating that badly — it is
## solving a different problem, and lands ahead of the server on every cycle
## where the server's turn happens to need the full 250ms. That is not drift
## and it does not accumulate; it is structural, present from the very first
## step, and it produced a rejection on a large fraction of steps under
## sustained movement — the "frena-sigue" this fixes. Quantising the client's
## own clock to tick boundaries reproduces the server's alternation instead of
## racing ahead of it; this offset additionally keeps those boundaries
## actually aligned with the server's, not just the same size.
var _server_phase_offset_ms := 0.0

## Inputs sent to the server but not yet acknowledged, oldest first. Each is
## {seq, dir}. Replayed on top of every server correction in set_entities —
## that replay is what reconciliation actually is now; see try_step and
## set_entities for the full picture.
var _pending: Array = []
var _next_seq := 1
## Drives animated tiles such as water, which run whether or not anyone moves.
var _world_time := 0.0

## While true the entity under the cursor is ringed, so it is obvious both that
## the game is waiting for a target and which one is about to be picked.
var targeting := false
var _hovered := 0

## Set by main.gd from the server's own-vitals status each snapshot. Nobody
## else's invisibility is drawn specially — an invisible stranger is simply
## absent from the server's snapshot — but a player needs to see their own
## state, the way Argentum shows you your own translucent sprite.
var local_invisible := false


func _ready() -> void:
	_sprites.load_bundle()
	_data.load_data()


func configure(welcome: Dictionary) -> void:
	map_width = int(welcome.get("w", 0))
	map_height = int(welcome.get("h", 0))
	view_w = int(welcome.get("vw", view_w))
	view_h = int(welcome.get("vh", view_h))
	local_id = int(welcome.get("id", 0))
	walk_speed = float(welcome.get("walkSpeed", walk_speed))
	tick_rate = int(welcome.get("tickRate", tick_rate))
	_tick_ms = 1000.0 / float(tick_rate)
	# Derived from the Welcome's own numbers rather than a second copy of the
	# server's constants, so a retune of walk speed or tick rate cannot leave
	# the two sides disagreeing about the cadence.
	_move_cooldown_ms = 1000.0 / walk_speed
	_turn_cooldown_ms = 6.0 * _tick_ms  # turnCooldownMilliticks is 6 ticks, flat
	_blocked_retry_ms = _tick_ms
	_blocked = Marshalls.base64_to_raw(str(welcome.get("blocked", "")))
	_camera = Vector2(int(welcome.get("sx", 0)), int(welcome.get("sy", 0)))
	_entities.clear()
	_ground.clear()
	_pending.clear()
	_next_seq = 1
	_local_step_ready_at = 0.0
	_local_turn_ready_at = 0.0
	_server_phase_offset_ms = 0.0
	_load_map(int(welcome.get("map", 0)))
	queue_redraw()


## Keeps this client's tick-quantised clock aligned with the server's. Called
## from every snapshot with its Tick field (see _quantized_now for why
## alignment, not just matching tick length, is what actually fixes the
## structural rejection this whole clock exists to avoid).
##
## Taking the latest estimate outright rather than smoothing it is deliberate:
## the two are the same clock running at the same rate, so the true offset is
## constant and any apparent movement in it is transport jitter, not drift to
## track. Averaging that jitter in would just make the estimate wrong for
## longer.
func sync_server_tick(server_tick: int) -> void:
	_server_phase_offset_ms = float(server_tick) * _tick_ms - Time.get_ticks_msec()


## Real time, adjusted for the server's clock phase and quantised down to the
## server's own tick boundaries.
##
## This is the actual fix for the stutter under a held key. The server can only
## ever grant a step on a tick boundary, and 222.2ms (the walk cooldown at the
## default tuning) is not a whole multiple of 50ms (the tick) — so the server
## itself alternates 250/200/250/200ms steps to average out to 222.2, per
## moveCooldownMilliticks server-side. A client predicting on a plain
## continuous 222.2ms timer is not approximating that cadence, it is running a
## different one, and it lands ahead of the server on every cycle where the
## server's turn needed the full 250ms — not drift, not accumulating, present
## from the first step. Quantising here means try_step's own clock can only
## ever line up with a tick boundary too, so it reproduces the server's
## alternation instead of racing past it.
func _quantized_now() -> float:
	var real := Time.get_ticks_msec() + _server_phase_offset_ms
	return floor(real / _tick_ms) * _tick_ms


## Map tiles ship with the client rather than over the wire: they are static
## content and the same for everyone, so the server only names the map.
func _load_map(number: int) -> void:
	_has_map = false
	if number <= 0:
		return

	var path := "res://assets/ao/map%d.json" % number
	if not FileAccess.file_exists(path):
		push_warning("falta %s; se dibuja la arena de prueba" % path)
		return

	var parsed: Variant = JSON.parse_string(FileAccess.get_file_as_string(path))
	if typeof(parsed) != TYPE_DICTIONARY:
		push_error("map%d.json ilegible" % number)
		return

	_layer1 = PackedInt32Array(parsed.get("layer1", []))
	_layer2 = parsed.get("layer2", {})
	_layer3 = parsed.get("layer3", {})
	_layer4 = parsed.get("layer4", {})
	_roofed.clear()
	for index in parsed.get("roofed", []):
		_roofed[int(index)] = true
	_has_map = _layer1.size() == map_width * map_height
	if not _has_map:
		push_error(
			"map%d.json trae %d tiles, el server dice %dx%d"
			% [number, _layer1.size(), map_width, map_height]
		)


## set_speech puts words over somebody's head. Empty text takes the sign down,
## which is a real move rather than a no-op: it is how you stop advertising the
## spell you just cast.
func set_speech(id: int, tile: Vector2i, text: String, spell: bool) -> void:
	if text.strip_edges() == "":
		_speech.erase(id)
		queue_redraw()
		return
	var lines := _wrap_speech(text)
	_speech[id] = {
		"lines": lines,
		"spell": spell,
		"tile": tile,
		"until": _world_time + SPEECH_BASE_SECONDS + SPEECH_PER_CHAR_SECONDS * text.length(),
	}
	queue_redraw()


## Wraps on whole words at Argentum's own 18-character line, splitting a word
## only when it cannot fit on a line by itself.
func _wrap_speech(text: String) -> PackedStringArray:
	var out := PackedStringArray()
	var line := ""
	for word in text.split(" ", false):
		var w: String = word
		while w.length() > SPEECH_WRAP_CHARS:
			if line != "":
				out.append(line)
				line = ""
			out.append(w.substr(0, SPEECH_WRAP_CHARS))
			w = w.substr(SPEECH_WRAP_CHARS)
		if line == "":
			line = w
		elif line.length() + 1 + w.length() <= SPEECH_WRAP_CHARS:
			line += " " + w
		else:
			out.append(line)
			line = w
	if line != "":
		out.append(line)
	return out


## ack_seq is the snapshot's protocol.Snapshot.AckSeq: the highest input the
## server has answered for the local player specifically. server_tick is the
## snapshot's own Tick, used to keep _quantized_now's clock in phase — see
## sync_server_tick. See try_step and the reconciliation block below for what
## ack_seq buys.
func set_entities(entities: Array, ack_seq: int = 0, server_tick: int = 0) -> void:
	sync_server_tick(server_tick)
	var seen: Dictionary = {}

	for e in entities:
		var id := int(e.get("id", 0))
		seen[id] = true
		var tile := Vector2(float(e.get("x", 0)), float(e.get("y", 0)))
		var heading := int(e.get("h", 0))

		var entity: Dictionary = _entities.get(id, {})
		if entity.is_empty():
			# First sighting: appear in place rather than sliding in from the
			# last entity's position.
			entity = {"render": tile, "anim": 0.0, "moving": false}

		if id == local_id:
			# Reconciliation. The server's tile/heading here are ground truth
			# as of exactly ack_seq — never a guess to blend with our own
			# prediction. Drop whatever it has already answered, then replay
			# only what's left: inputs sent after ack_seq that it hasn't had a
			# chance to answer yet.
			#
			# This used to compare raw positions and only trust the server
			# after several snapshots disagreed in a row, because there was no
			# way to tell "the server hasn't seen my last step yet" (normal,
			# and constant while a key is held) from "the server genuinely
			# rejected a step" (rare). Under fast or sustained input the
			# predicted tile is *always* a step or two ahead of the last
			# confirmed one, so that vote fired constantly and yanked the
			# player backward mid-stride — the rubber-banding this replaced.
			while not _pending.is_empty() and int(_pending[0]["seq"]) <= ack_seq:
				_pending.pop_front()

			entity["tile"] = tile
			entity["heading"] = heading
			for input in _pending:
				entity["heading"] = int(input["dir"])
				# A turn-only entry (mid-cadence, can_step=false) only ever
				# updates heading, exactly like the server's own World.turn —
				# replaying it as a step attempt would predict a tile the
				# server was never asked to grant.
				if bool(input["can_step"]):
					var result := _resolve_move(entity["tile"], int(input["dir"]))
					if result["stepped"]:
						entity["tile"] = result["tile"]
		else:
			entity["tile"] = tile
			entity["heading"] = heading
		entity["body"] = int(e.get("b", 0))
		entity["head"] = int(e.get("hd", 0))
		entity["name"] = str(e.get("n", ""))
		entity["dead"] = bool(e.get("d", false))
		# Carried for the click-to-inspect line rather than for drawing. Clan
		# is always empty today: there is no guild system, and the line omits
		# the bracket when it is, exactly as the original does for a clanless
		# character.
		entity["clan"] = str(e.get("cl", ""))
		entity["desc"] = str(e.get("ds", ""))
		entity["kills"] = int(e.get("k", 0))
		# Worn equipment, drawn as its own layer over the body. There is no
		# "armor" field on purpose: in Argentum armour *is* the body, so it
		# arrives as `b` above rather than as a fourth accessory.
		entity["weapon"] = int(e.get("wp", 0))
		entity["shield"] = int(e.get("sh", 0))
		entity["helmet"] = int(e.get("hm", 0))
		entity["paralyzed"] = bool(e.get("pz", false))
		entity["immobilized"] = bool(e.get("im", false))
		entity["meditating"] = bool(e.get("md", false))
		_entities[id] = entity

	for id: int in _entities.keys():
		if not seen.has(id):
			_entities.erase(id)


## Ground stacks are wholesale-replaced every snapshot rather than diffed —
## there are never more than a viewport's worth of them, and unlike entities
## nothing here needs to persist state (no render-smoothing, no anim) between
## calls.
func set_ground(items: Array) -> void:
	_ground.clear()
	for it in items:
		var x := int(it.get("x", 0))
		var y := int(it.get("y", 0))
		_ground["%d,%d" % [x, y]] = {
			"item_id": int(it.get("i", 0)),
			"amount": int(it.get("n", 0)),
		}


## Plays a spell's effect on whoever it landed on. Mirrors the original:
## Argentum never sends a projectile across the map, it plays the animation
## anchored to the target's own position (SendSpellEffects/Char_SetFx), offset
## by Fxs.ini's OffsetX/OffsetY, for the spell's own Loops repeats.
##
## Looked up locally by spell id rather than carried on the wire — the client
## already ships the same spells.json the server converted its table from, so
## SpellEvent naming the spell is enough.
func play_spell_fx(target_id: int, spell_id: int) -> void:
	if not _sprites.is_loaded():
		return
	var spell := _data.spell(spell_id)
	var fx_id := int(spell.get("fx", 0))
	if fx_id <= 0:
		return
	var grh := _sprites.fx_grh(fx_id)
	if grh == 0:
		return

	var cycle := _sprites.anim_cycle_seconds(grh)
	var loops := maxi(int(spell.get("loops", 0)), 1)
	var duration := cycle * loops if cycle > 0.0 else 0.3

	_active_fx.append({
		"entity": target_id,
		"grh": grh,
		"offset": _sprites.fx_offset(fx_id),
		"start": _world_time,
		"until": _world_time + duration,
	})


## Moves render one frame's worth toward target, along ONE axis at a time.
##
## Argentum never moves a character diagonally, and neither does this server —
## every step is one cardinal tile. The render position has to walk the same
## shape, and interpolating straight at the target does not: it moves along the
## vector between the two, which is diagonal whenever the two differ on both
## axes.
##
## They differ on both axes routinely. A player walking east who turns north
## gets a new target before the eastward step has finished drawing, so the
## remaining vector points north-east and the sprite cuts the corner. That is
## the "se mueve raro en diagonal al cambiar de sentido" — the simulation was
## always correct, only the smoothing was taking a shortcut the game does not
## allow.
##
## Whichever axis is mid-tile finishes first, because that is the step actually
## in progress; the turn happens once the character is back on the grid. When
## both axes are aligned and both differ — the server jumped more than one tile,
## which only a resync does — Y goes first. Either order draws an L; what
## matters is that it is an L and not a diagonal.
func _step_toward(render: Vector2, target: Vector2, step: float) -> Vector2:
	const ON_GRID := 0.001
	var mid_x: bool = absf(render.x - roundf(render.x)) > ON_GRID

	if mid_x or absf(target.y - render.y) < ON_GRID:
		return Vector2(move_toward(render.x, target.x, step), render.y)
	return Vector2(render.x, move_toward(render.y, target.y, step))


func _process(delta: float) -> void:
	_world_time += delta
	_hovered = entity_at(get_local_mouse_position()) if targeting else 0

	if not _active_fx.is_empty():
		_active_fx = _active_fx.filter(func(fx): return float(fx["until"]) > _world_time)

	if _entities.is_empty():
		return

	for id: int in _entities:
		var entity: Dictionary = _entities[id]
		var target: Vector2 = entity["tile"]
		var render: Vector2 = entity["render"]
		var to_go := target - render

		if to_go.length() > SNAP_DISTANCE:
			entity["render"] = target
			entity["moving"] = false
		elif to_go.length() > 0.001:
			entity["render"] = _step_toward(render, target, walk_speed * delta)
			entity["moving"] = true
			entity["anim"] = float(entity["anim"]) + delta
		else:
			entity["moving"] = false
			# Reset so the next step always starts on the same foot.
			entity["anim"] = 0.0

		if id == local_id:
			_camera = entity["render"]

	queue_redraw()


## entity_at returns the id of whoever is drawn under a point in this node's own
## coordinates, or 0.
##
## The test is against the drawn sprite, not the tile. Characters are taller
## than their square and grow upward out of it, so a player clicking the torso
## is aiming more than a tile above the position the server knows about —
## testing the tile would make aiming feel broken.
func entity_at(local_pos: Vector2) -> int:
	var origin := Vector2(_camera.x - view_w / 2.0, _camera.y - view_h / 2.0)

	var best := 0
	var best_depth := -INF
	for id: int in _entities:
		if _entity_box(_entities[id], origin).has_point(local_pos):
			# Whoever is drawn in front wins, matching the painter's order.
			var depth: float = _entities[id]["render"].y
			if depth > best_depth:
				best_depth = depth
				best = id
	return best


func entity_name(id: int) -> String:
	var entity: Variant = _entities.get(id)
	return "alguien" if entity == null else str(entity["name"])


## Everything the inspect line needs about one visible character. Empty if the
## id is not in view — the snapshot only ever carries the viewport, so asking
## about someone out of sight is answered with nothing rather than with stale
## data from when they were last seen.
func entity_info(id: int) -> Dictionary:
	var entity: Variant = _entities.get(id)
	if entity == null:
		return {}
	return {
		"name": str(entity["name"]),
		"clan": str(entity.get("clan", "")),
		"desc": str(entity.get("desc", "")),
		"kills": int(entity.get("kills", 0)),
		"dead": bool(entity.get("dead", false)),
	}


## The viewport's own bounds in local coordinates, for callers that need to
## know whether a click landed on the world at all.
##
## This is not get_rect(): the view is a Node2D, not a Control, so it has no
## rect of its own — its extent is the tile window it draws, which is what this
## computes.
func view_rect() -> Rect2:
	return Rect2(Vector2.ZERO, Vector2(view_w, view_h) * TILE_SIZE)


## The ground stack under a point, or empty if that tile has nothing on it.
## Counts are no longer painted on the floor, so this is how a player finds out
## what a pile is and how much of it there is.
func ground_at(local_pos: Vector2) -> Dictionary:
	var origin := Vector2(_camera.x - view_w / 2.0, _camera.y - view_h / 2.0)
	var tile := (local_pos / TILE_SIZE + origin).floor()
	var stack: Variant = _ground.get("%d,%d" % [int(tile.x), int(tile.y)])
	if stack == null:
		return {}
	return {
		"item_id": int(stack["item_id"]),
		"amount": int(stack["amount"]),
		"x": int(tile.x),
		"y": int(tile.y),
	}


## _entity_box is roughly what the character covers on screen: one tile wide,
## and one and a half tall reaching up from the feet.
func _entity_box(entity: Dictionary, origin: Vector2) -> Rect2:
	var foot: Vector2 = (entity["render"] - origin) * TILE_SIZE + Vector2(TILE_SIZE * 0.5, TILE_SIZE)
	var size := Vector2(TILE_SIZE, TILE_SIZE * 1.5)
	return Rect2(foot - Vector2(size.x * 0.5, size.y), size)


func is_blocked(x: int, y: int) -> bool:
	if x < 0 or y < 0 or x >= map_width or y >= map_height:
		return true
	var idx := y * map_width + x
	var byte_index := idx >> 3
	if byte_index >= _blocked.size():
		return true
	return (_blocked[byte_index] & (1 << (idx & 7))) != 0


func _draw() -> void:
	if map_width == 0:
		return

	# The camera is fractional, so the floor scrolls by sub-tile amounts and one
	# extra row and column are drawn to cover what slides in at the edges.
	var origin := Vector2(_camera.x - view_w / 2.0, _camera.y - view_h / 2.0)
	var first := Vector2i(floori(origin.x), floori(origin.y))
	var shift := (Vector2(first) - origin) * TILE_SIZE

	# Argentum's layer order is what makes the world read as three dimensional:
	# ground, then things lying on it, then the characters, and only then the
	# trees and walls they walk behind, and the roofs over everything.
	if _has_map and _sprites.is_loaded():
		_draw_layer(_layer1, first, shift, true)
		_draw_layer(_layer2, first, shift, false)
	else:
		_draw_placeholder_floor(first, shift)

	_draw_ground(origin)

	var ids: Array = _entities.keys()
	# Painter's order: whoever stands further down overlaps whoever is behind.
	ids.sort_custom(func(a, b): return _entities[a]["render"].y < _entities[b]["render"].y)

	var font := ThemeDB.fallback_font
	for id: int in ids:
		_draw_entity(id, _entities[id], origin, font)

	# Words from somebody the snapshot does not carry: an invisible caster.
	_draw_orphan_speech(origin, font)

	# The wall goes over the ground and the people standing on it, but under the
	# canopy: a tree in front of you still hides what is behind it.
	_draw_zone(origin)

	if _has_map and _sprites.is_loaded():
		_draw_layer(_layer3, first, shift, false)
		# Layer 4 is the roofs, and it is skipped entirely while the player is
		# standing under one. Argentum does the same: walk into a house and the
		# roof comes off so you can see what you are doing.
		#
		# Drawing it unconditionally is what made a house feel like a trap —
		# 118 of Ullathorpe's walkable tiles are roofed, and standing on one
		# put an opaque roof over the player, the doorway and everything else.
		# Nothing was blocking the way out; there was just no way to see it.
		if not _under_roof():
			_draw_layer(_layer4, first, shift, false)


## Where the local player is standing right now, in tiles, or null when the
## snapshot has not placed us yet.
func local_tile() -> Variant:
	var me: Variant = _entities.get(local_id)
	if me == null:
		return null
	return me["render"]


## set_zone takes the shrinking circle from the snapshot. Empty turns it off.
func set_zone(zone: Variant) -> void:
	_zone = zone if typeof(zone) == TYPE_DICTIONARY else {}


## Is a tile inside the safe circle? Used for the local warning, never for
## anything the server decides — it does its own check and its answer wins.
func in_safe_zone(tile: Vector2) -> bool:
	if _zone.is_empty():
		return true
	var d := tile - Vector2(float(_zone.get("x", 0.0)), float(_zone.get("y", 0.0)))
	return d.length() <= float(_zone.get("r", 0.0))


## _draw_zone paints everything outside the circle.
##
## Drawn as a ring of quads running from the circle's edge out to well past the
## corner of the screen, rather than as a rectangle with a hole cut in it:
## Godot's 2D canvas has no boolean clipping, and a fan of quads is both exact
## and cheap. Anything beyond the outer radius is off screen anyway.
func _draw_zone(origin: Vector2) -> void:
	if _zone.is_empty():
		return

	var centre := (Vector2(float(_zone.get("x", 0.0)), float(_zone.get("y", 0.0))) - origin) * TILE_SIZE
	var radius := float(_zone.get("r", 0.0)) * TILE_SIZE
	if radius <= 0.0:
		return

	# Far enough to cover the viewport from wherever the centre happens to be.
	var span := Vector2(view_w + 2, view_h + 2) * TILE_SIZE
	var outer := radius + centre.length() + span.length()

	var fill := PackedVector2Array()
	fill.resize(4)
	for i in ZONE_SEGMENTS:
		var a0 := TAU * float(i) / ZONE_SEGMENTS
		var a1 := TAU * float(i + 1) / ZONE_SEGMENTS
		var d0 := Vector2(cos(a0), sin(a0))
		var d1 := Vector2(cos(a1), sin(a1))
		fill[0] = centre + d0 * radius
		fill[1] = centre + d0 * outer
		fill[2] = centre + d1 * outer
		fill[3] = centre + d1 * radius
		draw_colored_polygon(fill, COLOR_ZONE_FILL)

	# The edge, breathing. Two arcs slightly out of phase read as electric
	# without needing a shader or a particle system.
	var pulse := 0.5 + 0.5 * sin(_world_time * 4.0)
	var edge := COLOR_ZONE_EDGE
	edge.a *= 0.55 + 0.45 * pulse
	draw_arc(centre, radius, 0.0, TAU, ZONE_SEGMENTS, edge, 2.0 + 1.5 * pulse)
	var inner := COLOR_ZONE_EDGE
	inner.a *= 0.25 * (1.0 - pulse)
	draw_arc(centre, maxf(radius - 3.0, 0.0), 0.0, TAU, ZONE_SEGMENTS, inner, 6.0)

	# Where it is going, so there is somewhere to run to.
	var next_r := float(_zone.get("nr", 0.0))
	if next_r > 0.0:
		var next_c := (
			Vector2(float(_zone.get("nx", 0.0)), float(_zone.get("ny", 0.0))) - origin
		) * TILE_SIZE
		draw_arc(next_c, next_r * TILE_SIZE, 0.0, TAU, ZONE_SEGMENTS, COLOR_ZONE_NEXT, 1.5)


## Steps (or turns) the local player right now, without waiting for the server
## to agree, and reports what to tell it.
##
## This is what Argentum's own client does. Map_MoveTo (mPooMap.bas:132) checks
## the destination against the client's copy of the map and, if it is legal,
## sends the walk AND moves the character in the same breath — the server finds
## out afterwards. That is the whole reason the original feels direct: the
## round trip never sits between the key and the character.
##
## We keep the server authoritative anyway, which the original does not — its
## HandleWalk has no rate limit at all and simply trusts the client, backed by a
## speedhack *detector*. Trusting the client is not an option in a battle
## royale, so instead the prediction runs the same rules the server does, each
## attempt is tagged with a sequence number, and set_entities replays whatever
## the server has not answered yet on top of every correction.
##
## Returns the seq to hand the server, or -1 when this input needs no message
## at all — held against a wall with the turn cooldown still running, or
## already facing the direction being asked for mid-step. The server would
## no-op both of those too, so nothing is bought by spending a packet on them.
func try_step(dir: int) -> int:
	var me: Variant = _entities.get(local_id)
	if me == null:
		return -1

	var now := _quantized_now()

	if now < _local_step_ready_at:
		# Mid-step: only a turn is possible, on its own separate cooldown that
		# mirrors the server's World.turn. Without this the client had no
		# equivalent at all — it simply sat on the old heading until the step
		# cooldown cleared, which is what made changing direction fast look
		# like the character freezing mid-turn while the server had already
		# spun to face the new way.
		if int(me["heading"]) == dir:
			return -1
		if now < _local_turn_ready_at:
			return -1
		me["heading"] = dir
		_local_turn_ready_at = now + _turn_cooldown_ms
		# can_step=false: this input is a turn only, exactly like the server's
		# own World.turn — it must never be replayed as a step later. See the
		# can_step comment on _enqueue for why that distinction has to survive
		# into the pending buffer instead of being re-derived at replay time.
		return _enqueue(dir, false)

	# Standing still must not bank credit toward a burst of fast steps, so the
	# carry is capped at a single cooldown — mirrors the server's own cap in
	# movePlayer. Without it the accumulation below would let a key pressed
	# after a long pause replay every cooldown missed while idle as one burst.
	if now - _local_step_ready_at > _move_cooldown_ms:
		_local_step_ready_at = now

	# Turning is free and immediate, exactly as the original has it: a legal
	# step carries its own turn, and the facing is applied even when the step
	# below is refused.
	me["heading"] = dir
	var result := _resolve_move(me["tile"], dir)
	if result["stepped"]:
		me["tile"] = result["tile"]
		# Advances from the ready mark, not from now, so the leftover fraction
		# of a cooldown survives from one step to the next — mirrors the
		# server's moveReadyAt += exactly. Advancing from now instead drops
		# that fraction on every single step, which compounds into drift; see
		# _quantized_now for the other half of this fix, the one that mattered
		# more — quantising now to tick boundaries in the first place.
		_local_step_ready_at += _move_cooldown_ms
	else:
		_local_step_ready_at = now + _blocked_retry_ms
	# can_step=true even on the blocked branch: the block might have been
	# someone else standing there a moment ago, and the replay re-checks
	# against the freshest state anyway, so there is no reason to freeze this
	# attempt as "never a step" the way the mid-cadence turn above must be.
	return _enqueue(dir, true)


## Records one input so set_entities can replay it until the server acks it,
## and hands back the seq to send alongside it.
##
## can_step marks whether this input is even allowed to move the predicted
## tile when replayed — false for a mid-cadence turn, which the server's own
## World.turn never lets move either. Without carrying this along, the replay
## in set_entities had no way to tell a turn-only input from a step attempt
## and re-evaluated every pending entry as "try to move here", so a direction
## change mid-step got replayed as a phantom step forward. That phantom step
## held until the next ack caught up and yanked it back — the stutter right
## after changing direction that this fixes.
func _enqueue(dir: int, can_step: bool) -> int:
	var seq := _next_seq
	_next_seq += 1
	_pending.append({"seq": seq, "dir": dir, "can_step": can_step})
	return seq


## The one rule both try_step and the set_entities replay move by: a step
## lands unless the destination is blocked or another character already
## stands on it. Factored out so the two call sites cannot quietly drift apart
## the way a second inlined copy eventually would.
func _resolve_move(tile: Vector2, dir: int) -> Dictionary:
	var target: Vector2 = tile + HEADING_DELTA[dir]
	if is_blocked(int(target.x), int(target.y)):
		return {"tile": tile, "stepped": false}
	# The server also refuses to walk onto another character. Checking it here
	# keeps the prediction from confidently walking through a crowd and then
	# being yanked back a tile at a time.
	for id: int in _entities:
		if id != local_id and _entities[id]["tile"] == target:
			return {"tile": tile, "stepped": false}
	return {"tile": target, "stepped": true}


## Whether the local player is standing on a tile the map marks as being under
## a roof. The whole roof layer is hidden while they are, rather than only the
## tiles immediately around them: a building's roof is one visual object, and
## punching a player-shaped hole in it looks worse than taking it off.
func _under_roof() -> bool:
	var me: Variant = _entities.get(local_id)
	if me == null:
		return false
	var tile: Vector2 = me["render"].round()
	var index := int(tile.y) * map_width + int(tile.x)
	return _roofed.has(index)


## _draw_layer paints one Argentum layer across the visible window. Layer 1 is a
## dense array indexed by tile; the others are sparse dictionaries keyed by the
## same index, which is why lookups go through a small closure either way.
func _draw_layer(layer: Variant, first: Vector2i, shift: Vector2, dense: bool) -> void:
	for vy in view_h + 2:
		for vx in view_w + 2:
			var tile := first + Vector2i(vx, vy)
			if tile.x < 0 or tile.y < 0 or tile.x >= map_width or tile.y >= map_height:
				continue

			var index := tile.y * map_width + tile.x
			var grh := 0
			if dense:
				grh = layer[index]
			else:
				grh = int(layer.get(str(index), 0))
			if grh <= 0:
				continue

			var src: Rect2 = _sprites.grh_rect(grh, _world_time)
			if src.size.x <= 0.0:
				continue

			# Anything taller or wider than a tile — trees, walls, banners —
			# hangs up and out from the square it is anchored to, so it is
			# placed by its bottom centre rather than its top left corner.
			var at := shift + Vector2(vx * TILE_SIZE, vy * TILE_SIZE)
			at += Vector2((TILE_SIZE - src.size.x) * 0.5, TILE_SIZE - src.size.y)
			draw_texture_rect_region(_sprites.texture_for(grh), Rect2(at, src.size), src)


## Ground loot draws between the floor and the characters — the same order
## Argentum itself uses, so a dropped sword reads as lying on the tile rather
## than floating over whoever is standing near it.
func _draw_ground(origin: Vector2) -> void:
	if not _sprites.is_loaded() or _ground.is_empty():
		return
	var font := ThemeDB.fallback_font
	for key: String in _ground:
		var parts := key.split(",")
		var tile := Vector2(float(parts[0]), float(parts[1]))
		var stack: Dictionary = _ground[key]
		var item := _data.item(int(stack.get("item_id", 0)))
		if item.is_empty():
			continue

		var grh := int(item.get("grh", 0))
		var rect: Rect2 = _sprites.grh_rect(grh, _world_time)
		if rect.size.x <= 0.0:
			continue
		var at := (tile - origin) * TILE_SIZE + Vector2((TILE_SIZE - rect.size.x) * 0.5, TILE_SIZE - rect.size.y)
		draw_texture_rect_region(_sprites.texture_for(grh), Rect2(at, rect.size), rect)
		# No count is painted on the tile. Argentum does not label the floor
		# either, and with potions now lying on a quarter of the map the labels
		# were the densest thing on screen — a field of "x25" over the actual
		# sprites. Clicking a stack reports what it is instead; see ground_at.


func _draw_placeholder_floor(first: Vector2i, shift: Vector2) -> void:
	for vy in view_h + 2:
		for vx in view_w + 2:
			var tile := first + Vector2i(vx, vy)
			var rect := Rect2(shift + Vector2(vx * TILE_SIZE, vy * TILE_SIZE), Vector2(TILE_SIZE, TILE_SIZE))

			var color := COLOR_VOID
			if tile.x >= 0 and tile.y >= 0 and tile.x < map_width and tile.y < map_height:
				color = COLOR_WALL if is_blocked(tile.x, tile.y) else COLOR_FLOOR
			draw_rect(rect, color)
			draw_rect(rect, COLOR_GRID, false, 1.0)


## Draws one character's five layers in Argentum's own order: body, head,
## helmet, weapon, shield (CharRender, TileEngine.bas:1362-1398).
##
## The order matters and is not the obvious one — weapon and shield go on top
## of the head, not under it, which is what lets a weapon correctly occlude the
## head on north-facing frames.
##
## Positioning is equally deliberate. Body, weapon and shield all draw at the
## character's own position with no offset: they are full-body overlay sprites,
## not accessories anchored to a hand, and treating them as attachments is the
## classic way to get this subtly wrong. Head and helmet are the only offset
## layers, by the body's own HeadOffset from Personajes.ini, with the helmet
## taking a further +1 on X and OFFSET_HEAD on Y.
##
## Returns false when the body could not be resolved, so the caller can fall
## back to a placeholder rather than drawing a floating head.
func _draw_character(entity: Dictionary, foot: Vector2, tint: Color) -> bool:
	var heading := int(entity["heading"])
	var anim := float(entity["anim"])
	var moving := bool(entity["moving"])

	var body := _sprites.body_rect(int(entity["body"]), heading, anim, moving)
	if body.size.x <= 0.0:
		return false

	draw_texture_rect_region(_sprites.atlas, Rect2(_anchor(foot, body.size), body.size), body, tint)

	# Head and helmet are the only layers with an offset, and it is the body's
	# own — different bodies wear their head at different heights, which is why
	# it is per-body data rather than one constant.
	var head_base := foot + _sprites.body_head_offset(int(entity["body"]))

	var head := _sprites.head_rect(int(entity["head"]), heading)
	if head.size.x > 0.0:
		draw_texture_rect_region(_sprites.atlas, Rect2(_anchor(head_base, head.size), head.size), head, tint)

	var helmet := _sprites.helmet_rect(int(entity.get("helmet", 0)), heading)
	if helmet.size.x > 0.0:
		var helmet_base := head_base + Vector2(1, AOSprites.OFFSET_HEAD)
		draw_texture_rect_region(
			_sprites.atlas, Rect2(_anchor(helmet_base, helmet.size), helmet.size), helmet, tint
		)

	# Weapon and shield share the body's anchor exactly: full-body overlay
	# sprites, not accessories pinned to a hand.
	var weapon := _sprites.weapon_rect(int(entity.get("weapon", 0)), heading, anim, moving)
	if weapon.size.x > 0.0:
		draw_texture_rect_region(_sprites.atlas, Rect2(_anchor(foot, weapon.size), weapon.size), weapon, tint)

	var shield := _sprites.shield_rect(int(entity.get("shield", 0)), heading, anim, moving)
	if shield.size.x > 0.0:
		draw_texture_rect_region(_sprites.atlas, Rect2(_anchor(foot, shield.size), shield.size), shield, tint)

	return true


## Draws whatever spell effects are currently playing on this entity, plus the
## meditation aura while entity["meditating"] holds — that one is continuous
## state (F6 toggles it) rather than a one-shot cast, so it is not something
## play_spell_fx's loops-then-expires _active_fx list can represent; it simply
## draws for as long as the flag does.
func _draw_fx(id: int, entity: Dictionary, foot: Vector2) -> void:
	for fx in _active_fx:
		if int(fx["entity"]) != id:
			continue
		_draw_fx_at(int(fx["grh"]), fx["offset"], foot, _world_time - float(fx["start"]))

	if bool(entity.get("meditating", false)):
		_draw_fx_at(_sprites.fx_grh(MEDITATE_FX), _sprites.fx_offset(MEDITATE_FX), foot, _world_time)


## MEDITATE_FX is FXIDs.FXMEDITARXXGRANDE (Declares.bas) — Fxs.ini index 34, the
## aura the source shows a level-42-and-up caster. Every character here spawns
## at maxLevel (see server/internal/world/balance.go), past every smaller tier
## the source's ELV switch defines, so there is only one aura this game can
## ever show and no server-sent id is needed to pick it.
const MEDITATE_FX := 34


func _draw_fx_at(grh: int, offset: Vector2, foot: Vector2, anim_time: float) -> void:
	if grh == 0:
		return
	var rect: Rect2 = _sprites.grh_rect(grh, anim_time)
	if rect.size.x <= 0.0:
		return
	var at: Vector2 = foot + offset
	draw_texture_rect_region(_sprites.atlas, Rect2(_anchor(at, rect.size), rect.size), rect)


## Places one sprite the way Draw_Grh's Center=1 does: horizontally centred on
## the tile and bottom-anchored to it, using **that sprite's own** size.
##
## The "own size" part is the whole point and is what I got wrong first time
## round. Every character layer passes Center=1, so each one is anchored
## independently from the same base point — the head is not positioned relative
## to the body's top-left corner. Anchoring the 17x50 head box by the body's
## ~25x45 box instead floats the head off the shoulders, which is exactly how
## it looked.
func _anchor(base: Vector2, size: Vector2) -> Vector2:
	return base - Vector2(size.x * 0.5, size.y)


func _draw_entity(id: int, entity: Dictionary, origin: Vector2, font: Font) -> void:
	var render: Vector2 = entity["render"]
	# Feet sit on the tile; the sprite grows upward out of it, which is what
	# makes an Argentum character look like it occupies the square it stands on.
	var foot := (render - origin) * TILE_SIZE + Vector2(TILE_SIZE * 0.5, TILE_SIZE)
	var is_local := id == local_id

	draw_circle(foot + Vector2(0, -2), TILE_SIZE * 0.30, COLOR_SHADOW)

	if targeting and id == _hovered:
		draw_arc(foot + Vector2(0, -2), TILE_SIZE * 0.42, 0.0, TAU, 24, COLOR_TARGET, 2.0)

	# Argentum shows a stunned enemy's state, which is tactically relevant: it
	# tells you whether closing the distance is safe. Paralyzed takes priority
	# since the two are mutually exclusive server-side anyway.
	if entity.get("paralyzed", false):
		draw_arc(foot + Vector2(0, -2), TILE_SIZE * 0.38, 0.0, TAU, 20, COLOR_PARALYZED_RING, 2.0)
	elif entity.get("immobilized", false):
		draw_arc(foot + Vector2(0, -2), TILE_SIZE * 0.38, 0.0, TAU, 20, COLOR_IMMOBILIZED_RING, 2.0)

	# The dead are greyed out, so a battlefield reads at a glance: who is still
	# standing, and who is already out.
	var dead: bool = entity.get("dead", false)
	var tint := Color.WHITE
	if dead:
		tint = COLOR_CORPSE
	elif is_local and local_invisible:
		# Nobody else's client ever draws this: an invisible stranger is simply
		# absent from their snapshot. This is purely local feedback so a
		# player can tell their own invisibility is active.
		tint = COLOR_INVISIBLE_SELF

	var drawn := false
	if _sprites.is_loaded():
		drawn = _draw_character(entity, foot, tint)

	if not drawn:
		var side := TILE_SIZE * 0.7
		draw_rect(
			Rect2(foot - Vector2(side * 0.5, side), Vector2(side, side)),
			COLOR_LOCAL if is_local else COLOR_OTHER
		)

	if _sprites.is_loaded():
		_draw_fx(id, entity, foot)

	var label := str(entity["name"])
	if label != "":
		# draw_string positions the baseline and only centres within an explicit
		# width, so the offset is measured rather than asked for.
		const NAME_SIZE := 11
		var half := font.get_string_size(label, HORIZONTAL_ALIGNMENT_LEFT, -1, NAME_SIZE).x * 0.5
		var at := foot + Vector2(-half, 13)
		var color := COLOR_NAME_LOCAL if is_local else COLOR_NAME
		# Cheap outline: the map underneath is busy and plain text vanishes.
		draw_string(
			font, at + Vector2(1, 1), label, HORIZONTAL_ALIGNMENT_LEFT, -1, NAME_SIZE, COLOR_NAME_SHADOW
		)
		draw_string(font, at, label, HORIZONTAL_ALIGNMENT_LEFT, -1, NAME_SIZE, color)

	_draw_speech(id, foot, font)


## The sign over somebody's head: chat, or the incantation of a spell they are
## casting. Drawn above the sprite rather than below it, where the name goes, so
## the two never collide.
##
## This is the whole point of casting being loud. A hidden player who casts is
## revealed by the server, but an *invisible* one is not — their body stays out
## of everyone's snapshot while their words do not, so the sign hangs in empty
## air exactly where they are standing.
func _draw_speech(id: int, foot: Vector2, font: Font) -> void:
	var entry: Variant = _speech.get(id)
	if entry == null:
		return
	if _world_time >= float(entry["until"]):
		_speech.erase(id)
		return
	_paint_speech(entry, foot, font)


## Signs whose speaker is not in the snapshot at all — an invisible caster.
## Drawn from the tile the server sent with the words, which is the only thing
## about them the client is told.
func _draw_orphan_speech(origin: Vector2, font: Font) -> void:
	for id: int in _speech.keys():
		if _entities.has(id):
			continue
		var entry: Dictionary = _speech[id]
		if _world_time >= float(entry["until"]):
			_speech.erase(id)
			continue
		var tile: Vector2i = entry.get("tile", Vector2i.ZERO)
		var foot := (Vector2(tile) - origin) * TILE_SIZE + Vector2(TILE_SIZE * 0.5, TILE_SIZE)
		_paint_speech(entry, foot, font)


func _paint_speech(entry: Dictionary, foot: Vector2, font: Font) -> void:
	var lines: PackedStringArray = entry["lines"]
	var color: Color = COLOR_SPELL_WORDS if bool(entry["spell"]) else COLOR_SPEECH
	# Sprites are about two tiles tall, so the block is stacked up from there
	# and grows further up as it gains lines.
	var top := foot.y - TILE_SIZE * 2.1 - float(lines.size() - 1) * (SPEECH_SIZE + 2)
	for i in lines.size():
		var line := lines[i]
		var half := font.get_string_size(line, HORIZONTAL_ALIGNMENT_LEFT, -1, SPEECH_SIZE).x * 0.5
		var at := Vector2(foot.x - half, top + float(i) * (SPEECH_SIZE + 2))
		draw_string(
			font, at + Vector2(1, 1), line, HORIZONTAL_ALIGNMENT_LEFT, -1, SPEECH_SIZE, COLOR_NAME_SHADOW
		)
		draw_string(font, at, line, HORIZONTAL_ALIGNMENT_LEFT, -1, SPEECH_SIZE, color)
