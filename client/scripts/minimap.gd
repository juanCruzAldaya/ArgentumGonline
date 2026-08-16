extends Control
## Whole-map overview drawn from the same collision bitset the world view uses.
##
## It shows the map, you, and whoever is inside your viewport — never everyone
## on the map. The server does not send positions outside the viewport, so this
## cannot leak them even by accident.

const COLOR_BG := Color(0.05, 0.04, 0.03, 0.88)
const COLOR_EDGE := Color("d9b45b")
const COLOR_FLOOR := Color(0.20, 0.22, 0.18)
const COLOR_WALL := Color(0.34, 0.29, 0.24)
const COLOR_LOCAL := Color(0.55, 0.90, 0.55)
const COLOR_OTHER := Color(0.90, 0.42, 0.36)

var _map_size := Vector2i.ZERO
var _blocked := PackedByteArray()
var _entities: Array = []
var _local_id := 0


func configure(welcome: Dictionary) -> void:
	_map_size = Vector2i(int(welcome.get("w", 0)), int(welcome.get("h", 0)))
	_local_id = int(welcome.get("id", 0))
	_blocked = Marshalls.base64_to_raw(str(welcome.get("blocked", "")))
	queue_redraw()


func set_entities(entities: Array) -> void:
	_entities = entities
	queue_redraw()


func _draw() -> void:
	draw_rect(Rect2(Vector2.ZERO, size), COLOR_BG)
	draw_rect(Rect2(Vector2.ZERO, size), COLOR_EDGE, false, 1.0)

	if _map_size.x <= 0 or _map_size.y <= 0:
		return

	# One pixel per tile would be unreadable at 100x100 in a 148px box, so tiles
	# are scaled to fill the widget and the map is letterboxed to stay square.
	var inner := size - Vector2(8, 8)
	var scale := minf(inner.x / _map_size.x, inner.y / _map_size.y)
	var drawn := Vector2(_map_size.x * scale, _map_size.y * scale)
	var origin := (size - drawn) * 0.5

	for ty in _map_size.y:
		for tx in _map_size.x:
			var color := COLOR_WALL if _is_blocked(tx, ty) else COLOR_FLOOR
			draw_rect(
				Rect2(origin + Vector2(tx * scale, ty * scale), Vector2(scale, scale) + Vector2.ONE),
				color
			)

	for entity in _entities:
		var is_local := int(entity.get("id", 0)) == _local_id
		var pos := origin + Vector2(int(entity.get("x", 0)) * scale, int(entity.get("y", 0)) * scale)
		draw_circle(pos, 3.0 if is_local else 2.0, COLOR_LOCAL if is_local else COLOR_OTHER)


func _is_blocked(x: int, y: int) -> bool:
	var idx := y * _map_size.x + x
	var byte_index := idx >> 3
	if byte_index >= _blocked.size():
		return true
	return (_blocked[byte_index] & (1 << (idx & 7))) != 0
