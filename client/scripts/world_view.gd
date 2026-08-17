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
var _camera := Vector2.ZERO

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
	_blocked = Marshalls.base64_to_raw(str(welcome.get("blocked", "")))
	_camera = Vector2(int(welcome.get("sx", 0)), int(welcome.get("sy", 0)))
	_entities.clear()
	_ground.clear()
	_load_map(int(welcome.get("map", 0)))
	queue_redraw()


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


func set_entities(entities: Array) -> void:
	var seen: Dictionary = {}

	for e in entities:
		var id := int(e.get("id", 0))
		seen[id] = true
		var tile := Vector2(float(e.get("x", 0)), float(e.get("y", 0)))

		var entity: Dictionary = _entities.get(id, {})
		if entity.is_empty():
			# First sighting: appear in place rather than sliding in from the
			# last entity's position.
			entity = {"render": tile, "anim": 0.0, "moving": false}
		entity["tile"] = tile
		entity["heading"] = int(e.get("h", 0))
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
			draw_texture_rect_region(_sprites.atlas, Rect2(at, src.size), src)


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

		var rect: Rect2 = _sprites.grh_rect(int(item.get("grh", 0)), _world_time)
		if rect.size.x <= 0.0:
			continue
		var at := (tile - origin) * TILE_SIZE + Vector2((TILE_SIZE - rect.size.x) * 0.5, TILE_SIZE - rect.size.y)
		draw_texture_rect_region(_sprites.atlas, Rect2(at, rect.size), rect)
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
