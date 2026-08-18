extends Control
## The world map, full size.
##
## The corner minimap is 128px wide for an 820x820 world — about a sixth of a
## pixel per tile, which is enough to know there is land somewhere but not
## enough to navigate. This is the same data drawn large enough to read.
##
## It reuses the texture the minimap already baked rather than building its own:
## the terrain cannot change mid-match, and painting 672.400 pixels twice for
## two views of the same thing would be a waste at best and a stutter when the
## map is opened at worst.
##
## Same interest-management rule as everywhere else: it draws you, and whoever
## the server has already told us about. Opening the map cannot reveal a
## position the client was never sent.

const COLOR_SCRIM := Color(0.02, 0.02, 0.03, 0.72)
const COLOR_FRAME := Color("d9b45b")
const COLOR_PAPER := Color(0.05, 0.04, 0.03)
const COLOR_LOCAL := Color(0.55, 0.90, 0.55)
const COLOR_OTHER := Color(0.90, 0.42, 0.36)
const COLOR_TEXT := Color("e8dcc0")
const COLOR_MUTED := Color(0.62, 0.58, 0.48)
const COLOR_ZONE := Color(0.45, 0.75, 1.00, 0.95)
const COLOR_ZONE_NEXT := Color(0.85, 0.95, 1.00, 0.60)

## How much of the screen the map may take, leaving room for the caption.
const MARGIN := 56.0

var _terrain: Texture2D = null
var _map_size := Vector2i.ZERO
var _map_name := ""
var _entities: Array = []
var _local_id := 0
var _zone: Dictionary = {}


func _ready() -> void:
	visible = false
	# The scrim swallows clicks so the world underneath does not act on them,
	# and anywhere on it is a way out.
	mouse_filter = Control.MOUSE_FILTER_STOP


func configure(welcome: Dictionary, terrain: Texture2D) -> void:
	_map_size = Vector2i(int(welcome.get("w", 0)), int(welcome.get("h", 0)))
	_map_name = str(welcome.get("mapName", ""))
	_local_id = int(welcome.get("id", 0))
	_terrain = terrain
	queue_redraw()


## The zone, so the map answers the only question that matters while it is
## closing: where do I run.
func set_zone(zone: Variant) -> void:
	_zone = zone if typeof(zone) == TYPE_DICTIONARY else {}
	if visible:
		queue_redraw()


func set_entities(entities: Array) -> void:
	_entities = entities
	if visible:
		queue_redraw()


func toggle() -> void:
	visible = not visible
	if visible:
		queue_redraw()


func close() -> void:
	visible = false


func _gui_input(event: InputEvent) -> void:
	if event is InputEventMouseButton and event.pressed:
		close()
		accept_event()


## _plate is where the map itself lands: the largest square that fits the
## screen with room for the caption, centred.
func _plate() -> Rect2:
	if _map_size.x <= 0 or _map_size.y <= 0:
		return Rect2()
	var room := size - Vector2(MARGIN * 2.0, MARGIN * 2.0 + 34.0)
	var scale := minf(room.x / _map_size.x, room.y / _map_size.y)
	var drawn := Vector2(_map_size.x, _map_size.y) * scale
	var at := Vector2((size.x - drawn.x) * 0.5, (size.y - drawn.y) * 0.5 + 12.0)
	return Rect2(at, drawn)


func _draw() -> void:
	draw_rect(Rect2(Vector2.ZERO, size), COLOR_SCRIM)

	var plate := _plate()
	if plate.size.x <= 0.0:
		return

	var frame := plate.grow(3.0)
	draw_rect(frame, COLOR_PAPER)
	draw_rect(frame, COLOR_FRAME, false, 2.0)

	if _terrain != null:
		draw_texture_rect(_terrain, plate, false)

	var font := ThemeDB.fallback_font
	var scale := plate.size.x / float(_map_size.x)

	if not _zone.is_empty():
		var zc := plate.position + Vector2(
			float(_zone.get("x", 0.0)), float(_zone.get("y", 0.0))
		) * scale
		var zr := float(_zone.get("r", 0.0)) * scale
		if zr > 0.0:
			draw_arc(zc, zr, 0.0, TAU, 96, COLOR_ZONE, 2.0)
		var nr := float(_zone.get("nr", 0.0)) * scale
		if nr > 0.0:
			var nc := plate.position + Vector2(
				float(_zone.get("nx", 0.0)), float(_zone.get("ny", 0.0))
			) * scale
			draw_arc(nc, nr, 0.0, TAU, 96, COLOR_ZONE_NEXT, 1.5)

	# Everyone the server has told us about, then you on top — your own marker
	# must never end up underneath somebody else's.
	var mine := Vector2.ZERO
	var have_mine := false
	for entity in _entities:
		var at := plate.position + Vector2(int(entity.get("x", 0)), int(entity.get("y", 0))) * scale
		if int(entity.get("id", 0)) == _local_id:
			mine = at
			have_mine = true
			continue
		draw_circle(at, 3.0, COLOR_OTHER)

	if have_mine:
		# A dot this small is easy to lose on a busy map, so it comes with
		# crosshairs that run the full width — you find the lines first and read
		# your position off where they cross.
		draw_line(Vector2(plate.position.x, mine.y), Vector2(plate.end.x, mine.y), COLOR_LOCAL * Color(1, 1, 1, 0.35), 1.0)
		draw_line(Vector2(mine.x, plate.position.y), Vector2(mine.x, plate.end.y), COLOR_LOCAL * Color(1, 1, 1, 0.35), 1.0)
		draw_circle(mine, 5.0, COLOR_LOCAL)
		draw_arc(mine, 9.0, 0.0, TAU, 24, COLOR_LOCAL, 1.5)

	var title := _map_name if _map_name != "" else "Mapa"
	var coords := ""
	for entity in _entities:
		if int(entity.get("id", 0)) == _local_id:
			coords = "  ·  %d, %d" % [int(entity.get("x", 0)), int(entity.get("y", 0))]
			break
	draw_string(
		font, Vector2(frame.position.x, frame.position.y - 10.0),
		title + coords, HORIZONTAL_ALIGNMENT_LEFT, -1, 15, COLOR_TEXT
	)
	draw_string(
		font, Vector2(frame.position.x, frame.end.y + 20.0),
		"M o clic para cerrar", HORIZONTAL_ALIGNMENT_LEFT, -1, 12, COLOR_MUTED
	)
