extends LineEdit
## The one-line chat box, opened with Enter and closed with it.
##
## It is deliberately modal while open: Argentum's own input works this way, and
## without it every letter you type would also be a game command — "a" would
## try to pick something up off the floor while you were writing a word with an
## "a" in it.
##
## Sending an empty line is a real action, not a no-op. One sign per character
## means whatever you say replaces what was over your head, so a single space is
## how you wipe off the incantation of a spell you just cast — which is the
## counterplay to casting announcing your position.

signal said(text: String)
signal opened
signal closed

const MAX_CHARS := 90


func _ready() -> void:
	visible = false
	max_length = MAX_CHARS
	placeholder_text = "decí algo — Enter manda, Escape cancela"
	add_theme_font_size_override("font_size", 13)
	text_submitted.connect(_on_submitted)


func is_open() -> bool:
	return visible


func open() -> void:
	if visible:
		return
	text = ""
	visible = true
	grab_focus()
	opened.emit()


func close() -> void:
	if not visible:
		return
	visible = false
	release_focus()
	closed.emit()


func _on_submitted(line: String) -> void:
	said.emit(line)
	close()


func _gui_input(event: InputEvent) -> void:
	if not (event is InputEventKey and event.pressed):
		return
	if event.keycode == KEY_ESCAPE:
		close()
		accept_event()
