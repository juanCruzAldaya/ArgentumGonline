extends Node2D
## Draws the tile grid and every entity the server reported.
##
## Rendering is deliberately primitive — coloured rectangles, no sprites. The
## point of this slice is the network loop, and art can drop in later without
## changing anything this node is handed.

const TILE_SIZE := 48

const COLOR_VOID := Color(0.04, 0.04, 0.06)
const COLOR_FLOOR := Color(0.16, 0.18, 0.15)
const COLOR_WALL := Color(0.30, 0.26, 0.22)
const COLOR_GRID := Color(0, 0, 0, 0.18)
const COLOR_LOCAL := Color(0.45, 0.80, 0.45)
const COLOR_OTHER := Color(0.80, 0.40, 0.35)
const COLOR_FACING := Color(1, 1, 1, 0.85)
const COLOR_NAME := Color(0.92, 0.92, 0.88)

var map_width := 0
var map_height := 0
var view_w := 17
var view_h := 13
var local_id := 0

var _blocked := PackedByteArray()
var _entities: Array = []
## Last known local position, kept so the camera holds still on the tick after
## a disconnect instead of snapping to the map origin.
var _center := Vector2i.ZERO


func configure(welcome: Dictionary) -> void:
	map_width = int(welcome.get("w", 0))
	map_height = int(welcome.get("h", 0))
	view_w = int(welcome.get("vw", view_w))
	view_h = int(welcome.get("vh", view_h))
	local_id = int(welcome.get("id", 0))
	_center = Vector2i(int(welcome.get("sx", 0)), int(welcome.get("sy", 0)))
	_blocked = Marshalls.base64_to_raw(str(welcome.get("blocked", "")))
	queue_redraw()


func set_entities(entities: Array) -> void:
	_entities = entities
	for entity in entities:
		if int(entity.get("id", 0)) == local_id:
			_center = Vector2i(int(entity.get("x", 0)), int(entity.get("y", 0)))
			break
	queue_redraw()


## Mirrors the server's collision layer: out of bounds counts as blocked, so
## callers never need a separate bounds check.
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

	var origin := Vector2i(_center.x - view_w / 2, _center.y - view_h / 2)

	for vy in view_h:
		for vx in view_w:
			var tile := Vector2i(origin.x + vx, origin.y + vy)
			var rect := Rect2(vx * TILE_SIZE, vy * TILE_SIZE, TILE_SIZE, TILE_SIZE)

			var color := COLOR_VOID
			if tile.x >= 0 and tile.y >= 0 and tile.x < map_width and tile.y < map_height:
				color = COLOR_WALL if is_blocked(tile.x, tile.y) else COLOR_FLOOR
			draw_rect(rect, color)
			draw_rect(rect, COLOR_GRID, false, 1.0)

	var font := ThemeDB.fallback_font
	var font_size := ThemeDB.fallback_font_size

	for entity in _entities:
		var vx := int(entity.get("x", 0)) - origin.x
		var vy := int(entity.get("y", 0)) - origin.y
		if vx < 0 or vy < 0 or vx >= view_w or vy >= view_h:
			continue

		var is_local := int(entity.get("id", 0)) == local_id
		var top_left := Vector2(vx * TILE_SIZE, vy * TILE_SIZE)
		var inset := 6.0

		draw_rect(
			Rect2(top_left + Vector2(inset, inset), Vector2.ONE * (TILE_SIZE - inset * 2)),
			COLOR_LOCAL if is_local else COLOR_OTHER
		)
		_draw_facing(top_left, int(entity.get("h", 0)))

		var name_text := str(entity.get("n", ""))
		if name_text != "":
			draw_string(
				font,
				top_left + Vector2(0, -4),
				name_text,
				HORIZONTAL_ALIGNMENT_CENTER,
				TILE_SIZE,
				font_size - 2,
				COLOR_NAME
			)


## A small notch on the tile edge the entity is facing — enough to read heading
## without needing directional sprites yet.
func _draw_facing(top_left: Vector2, heading: int) -> void:
	var thickness := 5.0
	var rect: Rect2
	match heading:
		0:  # north
			rect = Rect2(top_left + Vector2(0, 0), Vector2(TILE_SIZE, thickness))
		1:  # east
			rect = Rect2(top_left + Vector2(TILE_SIZE - thickness, 0), Vector2(thickness, TILE_SIZE))
		2:  # south
			rect = Rect2(top_left + Vector2(0, TILE_SIZE - thickness), Vector2(TILE_SIZE, thickness))
		3:  # west
			rect = Rect2(top_left, Vector2(thickness, TILE_SIZE))
		_:
			return
	draw_rect(rect, COLOR_FACING)
