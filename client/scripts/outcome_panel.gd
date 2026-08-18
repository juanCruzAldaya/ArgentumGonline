extends Control
## How the match ended, for you.
##
## It arrives twice for anyone who does not win: once the moment you are
## eliminated, with the half that is already decided — where you placed, how
## many you took with you, how long you lasted — and again when the match is
## called, with the winner's name. The second draws over the first, which is why
## this is one panel that updates rather than two screens.
##
## It does not block the world underneath being played: the winner is still
## standing in a match that has twenty seconds left on it, and a dead player is
## a ghost who can still look around. Click or Escape puts it away, and the next
## match takes it away by itself — see main.gd, which closes it on the Welcome
## that a restart sends.

const COLOR_SCRIM := Color(0.02, 0.02, 0.03, 0.55)
const COLOR_CARD := Color(0.05, 0.04, 0.03, 0.96)
const COLOR_FRAME := Color("d9b45b")
const COLOR_TEXT := Color("e8dcc0")
const COLOR_MUTED := Color(0.62, 0.58, 0.48)
const COLOR_WIN := Color(0.62, 0.92, 0.62)
const COLOR_LOST := Color(0.90, 0.42, 0.36)

const CARD_W := 420.0
const CARD_H := 232.0

var _outcome: Dictionary = {}


func _ready() -> void:
	visible = false
	mouse_filter = Control.MOUSE_FILTER_STOP


## show_outcome puts the card up, or updates the one already showing.
##
## A second call is the normal case, not an edge one: the winner's name only
## exists once the match is decided, and by then an eliminated player has been
## looking at their own card for a while.
func show_outcome(outcome: Dictionary) -> void:
	_outcome = outcome
	visible = true
	queue_redraw()


func close() -> void:
	visible = false


func _gui_input(event: InputEvent) -> void:
	if event is InputEventMouseButton and event.pressed:
		close()
		accept_event()


## Minutes and seconds, because a battle royale is measured in minutes and
## "247 s" is a number you have to convert before it means anything.
func _survived(seconds: float) -> String:
	var whole := int(maxf(seconds, 0.0))
	return "%d:%02d" % [whole / 60, whole % 60]


func _draw() -> void:
	draw_rect(Rect2(Vector2.ZERO, size), COLOR_SCRIM)

	var card := Rect2((size - Vector2(CARD_W, CARD_H)) * 0.5, Vector2(CARD_W, CARD_H))
	draw_rect(card, COLOR_CARD)
	draw_rect(card, COLOR_FRAME, false, 2.0)

	var font := ThemeDB.fallback_font
	var won := bool(_outcome.get("won", false))
	var place := int(_outcome.get("place", 0))
	var players := int(_outcome.get("of", 0))

	# The headline is the one thing that should be readable from across a room.
	var headline := "¡GANASTE!" if won else "#%d de %d" % [place, players]
	var headline_color := COLOR_WIN if won else COLOR_LOST
	var headline_w := font.get_string_size(headline, HORIZONTAL_ALIGNMENT_LEFT, -1, 34).x
	draw_string(
		font, Vector2(card.position.x + (CARD_W - headline_w) * 0.5, card.position.y + 62.0),
		headline, HORIZONTAL_ALIGNMENT_LEFT, -1, 34, headline_color
	)

	var subtitle := "Último en pie" if won else "Eliminado"
	var subtitle_w := font.get_string_size(subtitle, HORIZONTAL_ALIGNMENT_LEFT, -1, 14).x
	draw_string(
		font, Vector2(card.position.x + (CARD_W - subtitle_w) * 0.5, card.position.y + 86.0),
		subtitle, HORIZONTAL_ALIGNMENT_LEFT, -1, 14, COLOR_MUTED
	)

	# Two columns of a two-row table, drawn by hand: a Control with four
	# children for four numbers would be four nodes to keep in sync with a
	# dictionary that already has them.
	var stats := [
		["BAJAS", str(int(_outcome.get("kills", 0)))],
		["SOBREVIVISTE", _survived(float(_outcome.get("secs", 0.0)))],
	]
	var col_w := CARD_W / float(stats.size())
	for i in stats.size():
		var centre := card.position.x + col_w * (float(i) + 0.5)
		var label: String = stats[i][0]
		var value: String = stats[i][1]
		var label_w := font.get_string_size(label, HORIZONTAL_ALIGNMENT_LEFT, -1, 11).x
		var value_w := font.get_string_size(value, HORIZONTAL_ALIGNMENT_LEFT, -1, 22).x
		draw_string(
			font, Vector2(centre - label_w * 0.5, card.position.y + 126.0),
			label, HORIZONTAL_ALIGNMENT_LEFT, -1, 11, COLOR_MUTED
		)
		draw_string(
			font, Vector2(centre - value_w * 0.5, card.position.y + 154.0),
			value, HORIZONTAL_ALIGNMENT_LEFT, -1, 22, COLOR_TEXT
		)

	# The winner's line is absent from the card an eliminated player gets, and
	# fills in when the match is actually decided.
	var winner := str(_outcome.get("winner", ""))
	var footer := ""
	if won:
		footer = "Clic o Escape para cerrar"
	elif winner != "":
		footer = "Ganó %s  ·  clic o Escape para cerrar" % winner
	else:
		footer = "La partida sigue  ·  clic o Escape para cerrar"
	var footer_w := font.get_string_size(footer, HORIZONTAL_ALIGNMENT_LEFT, -1, 12).x
	draw_string(
		font, Vector2(card.position.x + (CARD_W - footer_w) * 0.5, card.end.y - 22.0),
		footer, HORIZONTAL_ALIGNMENT_LEFT, -1, 12, COLOR_MUTED
	)
