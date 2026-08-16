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

## Tiles per second, matching the server's move cooldown. Running faster than
## the server allows would make characters arrive and then wait; running slower
## would make them lag further behind with every step.
const WALK_SPEED := 5.0

## Past this distance a move is a teleport or a resync, not a walk, so the
## render position snaps instead of sliding across the map.
const SNAP_DISTANCE := 2.5

## How far the neck sinks into the shoulders. Butting the two exactly together
## leaves a visible seam at this sprite scale.
const HEAD_OVERLAP := 2.0

const COLOR_VOID := Color(0.04, 0.04, 0.06)
const COLOR_FLOOR := Color(0.16, 0.18, 0.15)
const COLOR_WALL := Color(0.30, 0.26, 0.22)
const COLOR_GRID := Color(0, 0, 0, 0.18)
const COLOR_NAME := Color(0.92, 0.92, 0.88)
const COLOR_NAME_LOCAL := Color(0.62, 0.92, 0.62)
const COLOR_NAME_SHADOW := Color(0, 0, 0, 0.85)
const COLOR_SHADOW := Color(0, 0, 0, 0.25)

## Fallback marks, used only for entities whose appearance was not bundled.
const COLOR_LOCAL := Color(0.45, 0.80, 0.45)
const COLOR_OTHER := Color(0.80, 0.40, 0.35)

var map_width := 0
var map_height := 0
var view_w := 17
var view_h := 13
var local_id := 0

var _sprites := AOSprites.new()
var _blocked := PackedByteArray()
## id -> { tile, render, heading, body, head, name, anim, moving }
var _entities: Dictionary = {}
var _camera := Vector2.ZERO


func _ready() -> void:
	_sprites.load_bundle()


func configure(welcome: Dictionary) -> void:
	map_width = int(welcome.get("w", 0))
	map_height = int(welcome.get("h", 0))
	view_w = int(welcome.get("vw", view_w))
	view_h = int(welcome.get("vh", view_h))
	local_id = int(welcome.get("id", 0))
	_blocked = Marshalls.base64_to_raw(str(welcome.get("blocked", "")))
	_camera = Vector2(int(welcome.get("sx", 0)), int(welcome.get("sy", 0)))
	_entities.clear()
	queue_redraw()


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
		_entities[id] = entity

	for id: int in _entities.keys():
		if not seen.has(id):
			_entities.erase(id)


func _process(delta: float) -> void:
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
			var step := WALK_SPEED * delta
			if to_go.length() <= step:
				entity["render"] = target
			else:
				entity["render"] = render + to_go.normalized() * step
			entity["moving"] = true
			entity["anim"] = float(entity["anim"]) + delta
		else:
			entity["moving"] = false
			# Reset so the next step always starts on the same foot.
			entity["anim"] = 0.0

		if id == local_id:
			_camera = entity["render"]

	queue_redraw()


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

	for vy in view_h + 2:
		for vx in view_w + 2:
			var tile := first + Vector2i(vx, vy)
			var at := shift + Vector2(vx * TILE_SIZE, vy * TILE_SIZE)
			var rect := Rect2(at, Vector2(TILE_SIZE, TILE_SIZE))

			var color := COLOR_VOID
			if tile.x >= 0 and tile.y >= 0 and tile.x < map_width and tile.y < map_height:
				color = COLOR_WALL if is_blocked(tile.x, tile.y) else COLOR_FLOOR
			draw_rect(rect, color)
			draw_rect(rect, COLOR_GRID, false, 1.0)

	# Painter's order: whoever stands further down overlaps whoever is behind.
	var ids: Array = _entities.keys()
	ids.sort_custom(func(a, b): return _entities[a]["render"].y < _entities[b]["render"].y)

	var font := ThemeDB.fallback_font
	for id: int in ids:
		_draw_entity(id, _entities[id], origin, font)


func _draw_entity(id: int, entity: Dictionary, origin: Vector2, font: Font) -> void:
	var render: Vector2 = entity["render"]
	# Feet sit on the tile; the sprite grows upward out of it, which is what
	# makes an Argentum character look like it occupies the square it stands on.
	var foot := (render - origin) * TILE_SIZE + Vector2(TILE_SIZE * 0.5, TILE_SIZE)
	var is_local := id == local_id

	draw_circle(foot + Vector2(0, -2), TILE_SIZE * 0.30, COLOR_SHADOW)

	var drawn := false
	if _sprites.is_loaded():
		var body := _sprites.body_rect(
			int(entity["body"]), int(entity["heading"]), float(entity["anim"]), bool(entity["moving"])
		)
		if body.size.x > 0.0:
			var body_at := foot - Vector2(body.size.x * 0.5, body.size.y)
			draw_texture_rect_region(
				_sprites.atlas, Rect2(body_at, body.size), body
			)
			var head := _sprites.head_rect(int(entity["head"]), int(entity["heading"]))
			if head.size.x > 0.0:
				# Both sprites are padded, so the head is placed by the measured
				# bounds of the artwork rather than by the rectangles. Lining the
				# rectangles up instead floats the head well above the shoulders.
				var head_y := _sprites.head_offset_y(
					int(entity["body"]), int(entity["head"]), HEAD_OVERLAP
				)
				var head_at := body_at + Vector2((body.size.x - head.size.x) * 0.5, head_y)
				draw_texture_rect_region(
					_sprites.atlas, Rect2(head_at, head.size), head
				)
			drawn = true

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
