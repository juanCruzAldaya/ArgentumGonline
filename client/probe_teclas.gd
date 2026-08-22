extends SceneTree
## Prueba del autoload de teclas, corrido a mano:
##   godot --headless --path client --script res://probe_teclas.gd
## Temporal: no es parte del juego.


## Instancia el autoload y lo arranca a mano. Fuera del arbol Godot no llama a
## _ready, y agregarlo al root de un SceneTree hecho a mano no es lo mismo que
## el autoload de verdad, asi que la prueba lo dice explicito.
func spawn(script: Script) -> Node:
	var node: Node = script.new()
	node._ready()
	return node


func _initialize() -> void:
	var script := load("res://scripts/key_bindings.gd")

	# De cero siempre: si quedo un teclas.cfg de haber jugado, la prueba estaria
	# midiendo esa partida y no el codigo.
	DirAccess.remove_absolute(ProjectSettings.globalize_path("user://teclas.cfg"))

	var kb: Node = spawn(script)
	var fails := 0

	# 1. Todas las acciones del catálogo quedaron registradas.
	for entry in kb.ACTIONS:
		var action: StringName = entry["action"]
		if not InputMap.has_action(action):
			print("FALLA: no se registró ", action)
			fails += 1

	# 2. Los defaults son los que dice el catálogo.
	if kb.current_key(&"atacar") != KEY_CTRL:
		print("FALLA: atacar quedó en ", kb.current_key(&"atacar"), " y no en Ctrl")
		fails += 1
	if kb.current_key(&"agarrar") != KEY_A:
		print("FALLA: agarrar no es A")
		fails += 1

	# 3. Norte conserva las dos teclas de fábrica, flecha y W.
	var norte: Array = InputMap.action_get_events(&"mover_norte")
	if norte.size() != 2:
		print("FALLA: mover_norte tiene ", norte.size(), " teclas, esperaba 2")
		fails += 1

	# 4. Guardar sin haber tocado nada no puede costar una tecla: el panel
	#    muestra una por acción, pero las flechas vienen con WASD al lado.
	kb.apply(kb.snapshot())
	if InputMap.action_get_events(&"mover_norte").size() != 2:
		print("FALLA: guardar sin cambios le sacó el WASD a mover_norte")
		fails += 1

	# 5. Guardar y volver a cargar deja lo mismo.
	var changed: Dictionary = kb.snapshot()
	changed[&"atacar"] = KEY_SPACE
	kb.apply(changed)
	var reloaded: Node = spawn(script)
	if reloaded.current_key(&"atacar") != KEY_SPACE:
		print("FALLA: no sobrevivió el guardado, atacar volvió a ", reloaded.current_key(&"atacar"))
		fails += 1
	# Y una acción remapeada de verdad queda con esa sola tecla.
	if InputMap.action_get_events(&"atacar").size() != 1:
		print("FALLA: atacar quedó con más de una tecla después de remapear")
		fails += 1

	# 6. Un archivo con una tecla repetida no deja a nadie sin nada: gana el
	#    primero del catálogo y el otro se queda con su default.
	var config := ConfigFile.new()
	config.set_value("teclas", "agarrar", KEY_J)
	config.set_value("teclas", "tirar", KEY_J)
	config.save(kb.CONFIG_PATH)
	var third: Node = spawn(script)
	if third.current_key(&"agarrar") != KEY_J:
		print("FALLA: el primero del catálogo tenía que quedarse con la tecla")
		fails += 1
	if third.current_key(&"tirar") != KEY_T:
		print("FALLA: tirar quedó en ", third.current_key(&"tirar"), " y esperaba su default T")
		fails += 1

	# 7. Restaurar defaults vuelve todo y borra el archivo.
	third.restore_defaults()
	if third.current_key(&"atacar") != KEY_CTRL or third.current_key(&"agarrar") != KEY_A:
		print("FALLA: restaurar defaults no volvió a las teclas de fábrica")
		fails += 1
	if InputMap.action_get_events(&"mover_norte").size() != 2:
		print("FALLA: restaurar defaults no devolvió el WASD de mover_norte")
		fails += 1

	print("nombre legible de Ctrl: ", kb.key_name(KEY_CTRL), " / de A: ", kb.key_name(KEY_A))
	print("PRUEBAS: ", "todo bien" if fails == 0 else "%d fallas" % fails)
	quit(fails)
