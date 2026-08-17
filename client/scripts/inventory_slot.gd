extends Panel
class_name InventorySlot
## One bag or equipment panel in the HUD.
##
## Everything about what a slot shows and how clicks select/use it still lives
## in hud.gd, driven through the same metadata (`server_slot`, `base_edge`)
## it always was — this class exists only to host the three drag-and-drop
## virtuals Godot requires an actual script for. `draggable` and `grid_index`
## are metadata for the same reason: hud.gd builds every slot through one
## `_make_slot` path and this keeps that path unforked.
##
## Only the bag grid sets `draggable`; the equip row leaves it false, so an
## equipped item can neither be dragged out nor be a drop target — moving
## gear happens through Usar/Equipar (U/E or a double-click), never a drag.

signal drag_dropped(from_slot: int, to_slot: int)


func _get_drag_data(_at_position: Vector2) -> Variant:
	if not get_meta(&"draggable", false):
		return null
	var server_slot: int = get_meta(&"server_slot", -1)
	if server_slot < 0:
		return null

	var preview := Panel.new()
	preview.custom_minimum_size = size
	preview.modulate.a = 0.85
	preview.mouse_filter = Control.MOUSE_FILTER_IGNORE
	for child in get_children():
		preview.add_child(child.duplicate())
	set_drag_preview(preview)

	return server_slot


func _can_drop_data(_at_position: Vector2, data: Variant) -> bool:
	return get_meta(&"draggable", false) and typeof(data) == TYPE_INT


func _drop_data(_at_position: Vector2, data: Variant) -> void:
	var from_slot := int(data)
	var to_slot: int = get_meta(&"grid_index", -1)
	if to_slot >= 0 and to_slot != from_slot:
		drag_dropped.emit(from_slot, to_slot)
