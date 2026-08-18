class_name AOSprites
extends RefCounted
## Reads the sprite bundle produced by tools/aoconv.
##
## Argentum addresses art by "grh" number. A grh is either a rectangle in the
## atlas or an animation that cycles through other grhs; this hides that split
## behind one call that answers "what rectangle do I draw right now".

const BUNDLE_PATH := "res://assets/ao/bundle.json"
const ATLAS_DIR := "res://assets/ao/"

## Heading order on the wire is north, east, south, west.
##
## Heads are stored in that same order, because Cabezas.ini is self-consistent.
const HEAD_FACING := [0, 1, 2, 3]

## Bodies, weapons, shields and helmets are all stored in the source's own
## heading order now that they come from the files the real client reads:
## index 0 north, 1 east, 2 south, 3 west. That is E_Heading, and it is what
## every one of Personajes.ini/Armas.dat/Escudos.dat/Cascos.ini indexes by.
##
## This used to be a lookup table of [1, 3, 0, 2], discovered by walking around
## and watching which way characters faced. It was compensating for bodies
## derived out of Cuerpos.ini — a stale file with two of four directions
## missing. With the real data there is nothing to compensate for, so the
## indirection is gone rather than re-tuned.
##
## Our protocol's Heading happens to agree: North=0, East=1, South=2, West=3.
const BODY_FACING := [0, 1, 2, 3]

## OFFSET_HEAD from the client's Declares.bas: the extra Y a helmet takes on
## top of the body's own head offset.
const OFFSET_HEAD := -34

## Page 0 of the atlas: everything that moves — bodies, heads, worn gear, spell
## effects, item icons. Character drawing can use it directly because none of
## that art ever lands on another page.
##
## World tiles live on their own page. One texture cannot exceed the GPU's
## GL_MAX_TEXTURE_SIZE and Godot does not complain when it does, it just draws
## every sprite in the game from the wrong pixels — so the atlas is split by
## role rather than allowed to grow. Use texture_for() when the page is not
## known in advance.
var atlas: Texture2D

var _pages: Array[Texture2D] = []
var _page_by_grh: Dictionary = {}
var _frames: Dictionary = {}
var _anims: Dictionary = {}
var _bodies: Dictionary = {}
var _heads: Dictionary = {}
var _weapons: Dictionary = {}
var _shields: Dictionary = {}
var _helmets: Dictionary = {}
var _fxs: Dictionary = {}
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

	# Older bundles named a single atlas; newer ones list every page. Both are
	# read so a stale bundle still draws instead of failing at load.
	var page_names: Array = parsed.get("pages", [])
	if page_names.is_empty():
		page_names = [parsed.get("atlas", "atlas.png")]

	_pages.clear()
	_page_by_grh.clear()
	for name: String in page_names:
		var path: String = ATLAS_DIR + name
		var tex := load(path) as Texture2D
		if tex == null:
			push_error("no se pudo cargar %s" % path)
			return false
		_pages.append(tex)
	atlas = _pages[0]

	_frames = parsed.get("frames", {})
	_anims = parsed.get("anims", {})
	_bodies = parsed.get("bodies", {})
	_heads = parsed.get("heads", {})
	_weapons = parsed.get("weapons", {})
	_shields = parsed.get("shields", {})
	_helmets = parsed.get("helmets", {})
	_fxs = parsed.get("fxs", {})
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


## Where the head goes on this body, in pixels, straight out of Personajes.ini.
##
## This replaces a pair of measured content bounds that existed only because
## the old body source had grh numbers sitting in its offset field. The real
## client does exactly this: head position is body position plus the body's own
## HeadOffset, no measuring involved.
func body_head_offset(body: int) -> Vector2:
	var entry: Variant = _bodies.get(str(body))
	if entry == null:
		return Vector2.ZERO
	var off: Array = entry.get("headOffset", [0, 0])
	return Vector2(float(off[0]), float(off[1]))


## Worn equipment. Weapons and shields animate with the walk exactly as the
## body does; helmets are static, like heads.
func weapon_rect(anim: int, heading: int, anim_time: float, moving: bool) -> Rect2:
	return _gear_rect(_weapons, anim, heading, anim_time, moving)


func shield_rect(anim: int, heading: int, anim_time: float, moving: bool) -> Rect2:
	return _gear_rect(_shields, anim, heading, anim_time, moving)


func helmet_rect(anim: int, heading: int) -> Rect2:
	return _gear_rect(_helmets, anim, heading, 0.0, false)


func _gear_rect(table: Dictionary, anim: int, heading: int, anim_time: float, moving: bool) -> Rect2:
	# 2 is the source's "nothing equipped" sentinel for all three of weapon,
	# shield and helmet — NingunArma/NingunEscudo/NingunCasco are 2, not 0. It
	# is never bundled, so the lookup would miss anyway; checking it explicitly
	# says why.
	if anim <= 0 or anim == 2:
		return Rect2()
	var entry: Variant = table.get(str(anim))
	if entry == null:
		return Rect2()
	return _resolve(int(entry["facings"][BODY_FACING[heading]]), anim_time, moving)


## grh_rect resolves any graphic — a map tile, most often. Animated tiles like
## water always run, since nothing is standing still for them to wait on.
func grh_rect(grh: int, anim_time: float) -> Rect2:
	return _resolve(grh, anim_time, true)


## Spell effects. Argentum plays these anchored to whoever the spell targets,
## offset by Fxs.ini's own OffsetX/OffsetY — never as a projectile travelling
## from caster to target.
func fx_grh(fx_id: int) -> int:
	var entry: Variant = _fxs.get(str(fx_id))
	return int(entry["grh"]) if entry != null else 0


func fx_offset(fx_id: int) -> Vector2:
	var entry: Variant = _fxs.get(str(fx_id))
	if entry == null:
		return Vector2.ZERO
	return Vector2(float(entry.get("offsetX", 0)), float(entry.get("offsetY", 0)))


## anim_cycle_seconds is how long one full loop of an animated grh takes. Zero
## means the grh is not animated (a single still frame).
func anim_cycle_seconds(grh: int) -> float:
	var anim: Variant = _anims.get(str(grh))
	if anim == null:
		return 0.0
	return maxf(float(anim.get("speed", 300.0)), 1.0) / 1000.0


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


## texture_for answers which atlas page a graphic was packed on.
##
## A Rect2 cannot carry the page, so anything drawing a grh whose page is not
## known up front — map tiles, ground loot, inventory icons — has to ask.
## Animated grhs resolve through their first frame: every frame of one
## animation is collected together, so they always share a page.
func texture_for(grh: int) -> Texture2D:
	if _pages.is_empty():
		return null

	# Cached on an int key, because this is called once per visible tile per
	# frame and the frame table is keyed by string: without the cache every
	# tile drawn allocates a String just to find out which texture to use.
	var cached: Variant = _page_by_grh.get(grh)
	if cached != null:
		return cached

	var f: Variant = _frames.get(str(grh))
	if f == null:
		var anim: Variant = _anims.get(str(grh))
		if anim != null:
			var frames: Array = anim.get("frames", [])
			if not frames.is_empty():
				f = _frames.get(str(int(frames[0])))
	var tex: Texture2D = _pages[0]
	if f != null:
		var page := int(f.get("p", 0))
		if page >= 0 and page < _pages.size():
			tex = _pages[page]
	_page_by_grh[grh] = tex
	return tex
