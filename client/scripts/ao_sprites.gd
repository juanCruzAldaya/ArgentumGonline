class_name AOSprites
extends RefCounted
## Reads the sprite bundle produced by tools/aoconv.
##
## Argentum addresses art by "grh" number. A grh is either a rectangle in the
## atlas or an animation that cycles through other grhs; this hides that split
## behind one call that answers "what rectangle do I draw right now".

const BUNDLE_PATH := "res://assets/ao/bundle.json"
const ATLAS_PATH := "res://assets/ao/atlas.png"

## Heading order on the wire is north, east, south, west.
##
## Heads are stored in that same order, because Cabezas.ini is self-consistent.
const HEAD_FACING := [0, 1, 2, 3]

## Bodies are stored sorted ascending, and the source index does NOT follow the
## wire order — Cuerpos' direction labels do not match its sprites, so aoconv
## refuses to guess and the mapping was settled by walking around and looking.
##
## Ascending grh order turned out to be south, north, west, east: the frame
## counts pair the facings {north,south} then {east,west}, and within each pair
## the lower grh is the one AO lists second. Both pairs needed swapping from the
## first guess. This is the one table to edit if a character faces wrong.
const BODY_FACING := [1, 3, 0, 2]

var atlas: Texture2D
var _frames: Dictionary = {}
var _anims: Dictionary = {}
var _bodies: Dictionary = {}
var _heads: Dictionary = {}
var _loaded := false


func load_bundle() -> bool:
	if _loaded:
		return true

	if not FileAccess.file_exists(BUNDLE_PATH):
		push_warning("no hay bundle de sprites en %s; corré tools/aoconv" % BUNDLE_PATH)
		return false

	var parsed: Variant = JSON.parse_string(FileAccess.get_file_as_string(BUNDLE_PATH))
	if typeof(parsed) != TYPE_DICTIONARY:
		push_error("bundle.json ilegible")
		return false

	atlas = load(ATLAS_PATH) as Texture2D
	if atlas == null:
		push_error("no se pudo cargar %s" % ATLAS_PATH)
		return false

	_frames = parsed.get("frames", {})
	_anims = parsed.get("anims", {})
	_bodies = parsed.get("bodies", {})
	_heads = parsed.get("heads", {})
	_loaded = true
	return true


func is_loaded() -> bool:
	return _loaded


## body_count and head_count let callers clamp server-sent ids to what was
## actually bundled, so an unbundled appearance degrades to a visible character
## rather than to nothing at all.
func body_count() -> int:
	return _bodies.size()


func head_count() -> int:
	return _heads.size()


## body_rect resolves a body to the atlas rectangle for its current walk frame.
## Returns an empty Rect2 when the body was not bundled.
func body_rect(body: int, heading: int, anim_time: float, moving: bool) -> Rect2:
	var entry: Variant = _bodies.get(str(body))
	if entry == null:
		return Rect2()
	var grh := int(entry["facings"][BODY_FACING[heading]])
	return _resolve(grh, anim_time, moving)


func head_rect(head: int, heading: int) -> Rect2:
	var entry: Variant = _heads.get(str(head))
	if entry == null:
		return Rect2()
	return _rect_of(int(entry["facings"][HEAD_FACING[heading]]))


## Sprite rectangles are padded, so these measured bounds — not the rectangles —
## are what a head and a body have to be aligned by. aoconv measures them off
## the finished atlas; see head_offset_y.
func body_content_top(body: int) -> float:
	var entry: Variant = _bodies.get(str(body))
	return 0.0 if entry == null else float(entry.get("top", 0))


func head_content_bottom(head: int) -> float:
	var entry: Variant = _heads.get(str(head))
	return 0.0 if entry == null else float(entry.get("bottom", 0))


## head_offset_y is how far below the body sprite's top edge the head sprite
## should be drawn, so that the neck lands on the shoulders.
func head_offset_y(body: int, head: int, overlap: float) -> float:
	return body_content_top(body) - head_content_bottom(head) + overlap


## _resolve turns a grh into a rectangle, walking into an animation if needed.
## A standing character holds frame 0 rather than cycling in place.
func _resolve(grh: int, anim_time: float, moving: bool) -> Rect2:
	var anim: Variant = _anims.get(str(grh))
	if anim == null:
		return _rect_of(grh)

	var frames: Array = anim.get("frames", [])
	if frames.is_empty():
		return Rect2()

	var index := 0
	if moving:
		# AO stores speed as the duration of the whole cycle in milliseconds.
		var cycle: float = maxf(float(anim.get("speed", 300.0)), 1.0) / 1000.0
		index = int(fmod(anim_time, cycle) / cycle * frames.size()) % frames.size()
	return _rect_of(int(frames[index]))


func _rect_of(grh: int) -> Rect2:
	var f: Variant = _frames.get(str(grh))
	if f == null:
		return Rect2()
	return Rect2(float(f["x"]), float(f["y"]), float(f["w"]), float(f["h"]))
