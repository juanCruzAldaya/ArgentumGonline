extends Node
## El sonido del juego: los efectos de Argentum y su música.
##
## Autoload, por lo mismo que KeyBindings: cualquiera tiene que poder hacer
## sonar algo sin que le pasen una referencia, y hay estado —qué está sonando,
## qué prendió el jugador— que sobrevive a los cambios de pantalla.
##
## **Ningún sonido viaja por la red.** El original manda un paquete PlayWave con
## el número del WAV, porque allá el servidor es el que decide que algo sonó.
## Acá el cliente ya recibe el evento de combate, el del hechizo y el resultado
## de usar un objeto: sabe lo que pasó, así que sabe qué sonar. Un mensaje nuevo
## habría sido una segunda forma de contar lo mismo, que es exactamente donde
## dos caminos se desincronizan.
##
## Los efectos vienen horneados en el juego (22 archivos, 32 segundos, 0,3 MB
## una vez comprimidos); la música no, y esa es la diferencia que importa: los
## 72 MP3 del original pesan 177 MB y el cliente web entero pesa 37. Las dos
## pistas que se usan las sirve el servidor por HTTP y se bajan solo si el
## jugador quiere música. Ver tools/aoconv/sounds.go.

## Los números son los del original, y salen del **servidor** de AO
## (Declares.bas), no del cliente: allá es el servidor el que decide que sonó
## un golpe. Acá esa decisión la toma este archivo, pero los números son los
## mismos, así que un WAV se puede escuchar en el AO original y acá y es el
## mismo sonido. tools/aoconv convierte exactamente estos.
const SND_SWING := 2  ## el golpe que no entra
const SND_IMPACTO := 10  ## el golpe que entra
const SND_MUERTE := 11  ## alguien muere
const SND_IMPACTO2 := 12  ## el segundo impacto, para el hechizo que pega
const SND_ESCUDO := 37  ## el escudo rechaza
const SND_BEBER := 46  ## tomar una poción
## Los dos pasos, que se alternan como en TileEngine.bas.
const SND_PASOS := [23, 24]

const SFX_PATH := "res://assets/ao/sfx/%d.wav"
const CONFIG_PATH := "user://audio.cfg"
const CONFIG_SECTION := "audio"

## Cuántos efectos pueden sonar a la vez. Ocho: en una pelea de tres con
## hechizos y pasos rara vez hay más de cuatro, y un noveno sonido que pisa al
## más viejo es mejor que un noveno sonido que no suena.
const VOICES := 8

## Volúmenes fijos, en decibeles. No hay sliders: el original tiene un
## interruptor por cada cosa y nada más, y dos teclas es toda la interfaz que
## esto necesita hasta que exista una pantalla de opciones.
const SFX_DB := -6.0
const MUSIC_DB := -14.0

## Lo que el jugador prendió, guardado entre sesiones igual que las teclas.
var sfx_on := true
var music_on := true

var _players: Array[AudioStreamPlayer] = []
var _next := 0
var _step := 0
## Los streams ya cargados, por número. Un efecto se carga la primera vez que
## suena y se queda: son 22 archivos de un par de kilobytes.
var _cache: Dictionary = {}
## Los números que no existen, para avisar una sola vez en vez de una por golpe.
var _missing: Dictionary = {}

var _music: AudioStreamPlayer
var _music_track := ""
var _music_cache: Dictionary = {}
var _http: HTTPRequest
var _fetching := ""
## De dónde bajar la música: el mismo servidor del juego, en HTTP en vez de
## WebSocket. Vacío hasta que alguien diga a cuál nos conectamos.
var _base_url := ""


func _ready() -> void:
	for i in VOICES:
		var player := AudioStreamPlayer.new()
		add_child(player)
		_players.append(player)

	_music = AudioStreamPlayer.new()
	_music.volume_db = MUSIC_DB
	add_child(_music)

	_http = HTTPRequest.new()
	add_child(_http)
	_http.request_completed.connect(_on_music_downloaded)

	_load_config()


## set_server dice de dónde sale la música, a partir de la URL del juego.
##
## ws://host:8080/ws se convierte en http://host:8080/music/…, que es el mismo
## proceso Go escuchando en el mismo puerto: el servidor ya sirve el cliente web
## por HTTP, así que servir dos MP3 no agrega ni un proceso ni un puerto.
func set_server(ws_url: String) -> void:
	var base := ws_url
	if base.begins_with("wss://"):
		base = "https://" + base.substr(6)
	elif base.begins_with("ws://"):
		base = "http://" + base.substr(5)
	if base.ends_with("/ws"):
		base = base.substr(0, base.length() - 3)
	_base_url = base.rstrip("/")


## play hace sonar un efecto por su número de Argentum.
##
## Un número que no está no es un error: el conversor pudo no haberlo escrito,
## o alguien está corriendo el cliente sin haber generado los sonidos. El juego
## se juega igual sin sonido, así que esto avisa una vez y sigue.
func play(sound: int) -> void:
	if not sfx_on or sound <= 0 or _missing.has(sound):
		return

	var stream: AudioStream = _cache.get(sound)
	if stream == null:
		var path := SFX_PATH % sound
		if not ResourceLoader.exists(path):
			_missing[sound] = true
			push_warning("falta el sonido %d (%s); corré tools/aoconv -sounds" % [sound, path])
			return
		stream = load(path)
		_cache[sound] = stream

	# Round robin sobre las voces: la más vieja es la que se pisa. Buscar una
	# libre y no sonar si no hay sería tragarse justo el sonido de la pelea más
	# cargada, que es cuando más importa oír algo.
	var player := _players[_next]
	_next = (_next + 1) % VOICES
	player.stream = stream
	player.volume_db = SFX_DB
	player.play()


## Un paso, alternando los dos que trae el original.
func play_step() -> void:
	_step = (_step + 1) % SND_PASOS.size()
	play(SND_PASOS[_step])


## El sonido de un hechizo sale del hechizo mismo: Hechizos.dat le pone un WAV a
## cada uno y el conversor lo pasa a spells.json. Catorce sonidos distintos para
## cincuenta hechizos, que es el reparto que hizo el original.
func play_spell(spell: Dictionary) -> void:
	play(int(spell.get("wav", 0)))


## play_music pone una de las dos pistas: "lobby" en el campamento, "match" en
## la partida. Repetir la que ya está sonando no la reinicia.
func play_music(track: String) -> void:
	if track == _music_track and _music.playing:
		return
	_music_track = track
	if not music_on:
		return

	var stream: AudioStream = _music_cache.get(track)
	if stream != null:
		_start_music(stream)
		return
	_fetch_music(track)


func stop_music() -> void:
	_music.stop()


## toggle_music es la tecla que la prende y la apaga.
##
## Apagarla frena la que suena; prenderla vuelve a poner la que corresponda al
## lugar donde estamos, bajándola si es la primera vez. Ahí está el ahorro de
## servirla aparte: el que la deja apagada no baja un byte de música nunca.
func toggle_music() -> bool:
	music_on = not music_on
	if music_on:
		var track := _music_track
		_music_track = ""
		play_music(track if track != "" else "lobby")
	else:
		_music.stop()
	_save_config()
	return music_on


func toggle_sfx() -> bool:
	sfx_on = not sfx_on
	if not sfx_on:
		for player in _players:
			player.stop()
	_save_config()
	return sfx_on


func _fetch_music(track: String) -> void:
	if _base_url == "" or _fetching != "":
		return
	_fetching = track
	var err := _http.request("%s/music/%s.mp3" % [_base_url, track])
	if err != OK:
		_fetching = ""


func _on_music_downloaded(
	result: int, code: int, _headers: PackedStringArray, body: PackedByteArray
) -> void:
	var track := _fetching
	_fetching = ""
	if result != HTTPRequest.RESULT_SUCCESS or code != 200 or body.is_empty():
		# Un servidor sin música es un servidor sin música: no hay nada roto que
		# arreglar y el juego suena igual de bien sin ella.
		return

	var stream := AudioStreamMP3.load_from_buffer(body)
	if stream == null:
		return
	stream.loop = true
	_music_cache[track] = stream
	if music_on and track == _music_track:
		_start_music(stream)


func _start_music(stream: AudioStream) -> void:
	_music.stream = stream
	_music.volume_db = MUSIC_DB
	_music.play()


func _load_config() -> void:
	var config := ConfigFile.new()
	if config.load(CONFIG_PATH) != OK:
		return
	sfx_on = bool(config.get_value(CONFIG_SECTION, "sfx", true))
	music_on = bool(config.get_value(CONFIG_SECTION, "music", true))


func _save_config() -> void:
	var config := ConfigFile.new()
	config.set_value(CONFIG_SECTION, "sfx", sfx_on)
	config.set_value(CONFIG_SECTION, "music", music_on)
	config.save(CONFIG_PATH)
