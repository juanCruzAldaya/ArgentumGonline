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

## The terrain, baked once into a texture at map resolution.
##
## This used to be drawn tile by tile, one draw_rect per tile, on every redraw
## — and a redraw happens on every snapshot, twenty times a second. On
## Ullathorpe that was 10.000 rects per frame, which was survivable and so went
## unnoticed. On a composed 820x820 world it is 672.400 rects per frame, 13
## million draw calls a second, and the client simply stops.
##
## Argentum solved this the same way twenty years ago: it never draws a minimap
## from tiles, it ships one pre-rendered BMP per map in Graficos/MiniMapa. The
## terrain cannot change mid-match, so drawing it more than once is wasted work
## no matter how big the map is.
## Typed Texture2D, not ImageTexture: the baked map arrives from disk as a
## CompressedTexture2D and the narrower type silently rejects it, leaving the
## map blank with nothing on screen to say why.
var _terrain: Texture2D = null


func configure(welcome: Dictionary) -> void:
	_map_size = Vector2i(int(welcome.get("w", 0)), int(welcome.get("h", 0)))
	_local_id = int(welcome.get("id", 0))
	_blocked = Marshalls.base64_to_raw(str(welcome.get("blocked", "")))
	_terrain = _load_baked(int(welcome.get("map", 0)))
	if _terrain == null:
		_terrain = _bake_terrain()
	queue_redraw()


## The map as tools/aoconv drew it: one pixel per tile, in the colour of the
## tile's own artwork. Forest reads as forest and desert as desert, which the
## collision bitset cannot express — it only knows wall from floor, which is two
## flat colours and looks like a floor plan.
##
## Falls back to the bitset when there is no baked image, which is the case for
## the generated demo arena and for any map converted before this existed.
func _load_baked(map_number: int) -> Texture2D:
	if map_number <= 0:
		return null
	var path := "res://assets/ao/map%d_mini.png" % map_number
	if not ResourceLoader.exists(path):
		return null
	return load(path) as Texture2D


## _bake_terrain paints the collision bitset into one texture, one pixel per
## tile. Built through a flat byte buffer rather than set_pixel because at
## 672.400 tiles the per-call overhead is the whole cost.
func _bake_terrain() -> Texture2D:
	if _map_size.x <= 0 or _map_size.y <= 0:
		return null

	var floor_rgb := PackedByteArray(
		[int(COLOR_FLOOR.r * 255.0), int(COLOR_FLOOR.g * 255.0), int(COLOR_FLOOR.b * 255.0)]
	)
	var wall_rgb := PackedByteArray(
		[int(COLOR_WALL.r * 255.0), int(COLOR_WALL.g * 255.0), int(COLOR_WALL.b * 255.0)]
	)

	var pixels := PackedByteArray()
	pixels.resize(_map_size.x * _map_size.y * 3)
	var at := 0
	for ty in _map_size.y:
		for tx in _map_size.x:
			var rgb := wall_rgb if _is_blocked(tx, ty) else floor_rgb
			pixels[at] = rgb[0]
			pixels[at + 1] = rgb[1]
			pixels[at + 2] = rgb[2]
			at += 3

	var image := Image.create_from_data(
		_map_size.x, _map_size.y, false, Image.FORMAT_RGB8, pixels
	)
	return ImageTexture.create_from_image(image)


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

	if _terrain != null:
		draw_texture_rect(_terrain, Rect2(origin, drawn), false)

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
