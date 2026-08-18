# Operación

Todos los comandos para levantar, buildear, publicar y deployar, más cómo está
armado el sistema por dentro. Los otros tres documentos cubren otra cosa:
[RESUMEN-EJECUTIVO](RESUMEN-EJECUTIVO.md) para decidir,
[RESUMEN-FUNCIONAL](RESUMEN-FUNCIONAL.md) para saber qué hace el juego, y
[DIFICULTADES](DIFICULTADES.md) para lo que costó y por qué.

---

## 1. Levantar todo en local

Tres procesos, en este orden. Cada uno en su propia terminal, o los dos
primeros en background.

### Servidor

```powershell
cd server
go run ./cmd/server
```

**Los tres archivos de datos ya no se pasan a mano.** `-map-file`,
`-items-file` y `-spells-file` apuntan por default a `maps/map1.json`,
`maps/items.json` y `maps/spells.json`, así que corriendo desde `server/`
arranca el juego de verdad y no una arena vacía. Y si alguno de esos archivos
no está, el servidor **no arranca**: dice cuál falta y cómo resolverlo, en vez
de bajar en silencio a una arena generada sin objetos ni hechizos mientras
responde 200 al health check. Ese default silencioso es el síntoma de "no
conozco los hechizos y el mapa no renderiza", y mordió dos veces, la segunda en
producción — postmortem en DIFICULTADES §8.

La arena de prueba sigue estando, pero ahora hay que pedirla:
`-map-file="" -items-file="" -spells-file=""`. En ese caso el arranque avisa
con un `WARN` y `/healthz` contesta `degradado: ...` en vez de `ok`.

Si el arranque está bien, el log dice las seis líneas:

```
mapa cargado         name="Ciudad de Ullathorpe" size="[100 100]"
loot esparcido       pedido=165  colocado=165
pociones esparcidas  pedido=1199 colocado=1199 unidades=29975
items cargados       count=496
hechizos cargados    count=50
world running        tickRate=20
listening            addr=[::]:8080
```

Si falta alguna de esas líneas, no estás corriendo el juego. El atajo para
preguntárselo al servidor, sin leer el log, es el health check — contesta qué
cargó, no solo que está vivo:

```powershell
curl.exe -s http://localhost:8080/healthz
# ok map="Ciudad de Ullathorpe" items=496 spells=50
```

Otros flags: `-addr` (default `:8080`), `-tick`, `-map-width`, `-map-height`,
`-seed`, `-debug`, `-web-dir`, `-respawn`.

**`-respawn` es una muleta de testeo, no una regla del juego.** Por default
vale 5: el muerto queda de fantasma 5 segundos y vuelve a entrar en el medio
del mapa con equipo nuevo, para no tener que reiniciar el cliente después de
cada pelea. `-respawn 0` devuelve la regla del género, que es eliminación.

### Cliente

```powershell
godot --path client -- --server=ws://127.0.0.1:8080/ws --name=wachin
```

La URL nunca está hardcodeada: `--server=` o la variable `JUEGITO_SERVER`; el
nombre, `--name=` o `JUEGITO_NAME`.

Si Godot no está en el PATH, usá la ruta completa. Conviene el binario
`_console.exe` en Windows: el otro no imprime nada en la terminal, y los
errores de GDScript aparecen solo ahí.

```powershell
& "$env:USERPROFILE\Downloads\Godot_v4.7.1-stable_win64.exe\Godot_v4.7.1-stable_win64_console.exe" --path client -- --server=ws://127.0.0.1:8080/ws --name=wachin
```

### Bots

```powershell
cd server
go run ./cmd/bot -url ws://127.0.0.1:8080/ws -n 30
```

Hablan exactamente el mismo protocolo que un cliente real; del lado del
servidor no existe el concepto de "bot". Lo que se rompe con bots se hubiera
roto con jugadores.

### Cuando el puerto queda ocupado

Pasa cuando un servidor anterior quedó colgado:

```powershell
netstat -ano | findstr 8080
taskkill /PID <pid> /F
```

---

## 2. Verificar antes de dar algo por hecho

```powershell
cd server
go build ./...
go vet ./...
go test ./...              # 101 tests en internal/world, 1 en cmd/server
go test ./... -count=5     # el mundo usa rng; esto caza los flaky
```

GDScript no tiene compilador que corras aparte, pero sí un chequeo de sintaxis:

```powershell
godot --headless --path client --check-only --script scripts/world_view.gd
```

**Ojo con lo que ese chequeo NO atrapa:** llamadas a métodos inexistentes sobre
un nodo tipado. `_view.get_rect()` sobre un `Node2D` pasa el `--check-only` y
explota en runtime. La única forma de agarrar esa clase de error es correr el
juego y ejercitar el camino. Por eso conviene mirar siempre la salida del
cliente después de tocar algo:

```powershell
# con el cliente corriendo, buscar en su salida:
#   SCRIPT ERROR
```

---

## 3. Regenerar los assets

Solo hace falta si cambiaste `tools/aoconv` o querés otros cuerpos/mapas.
Depende de tener los assets originales de Argentum extraídos.

```powershell
go run -C tools/aoconv . `
  -assets "$env:USERPROFILE\Downloads\ao-assets" `
  -bundle "$PWD\client\assets\ao" `
  -heads 1,2,3,4,5,6,7,8,500 `
  -map 1 `
  -server-out "$PWD\server\maps"
```

Escribe cinco cosas: `atlas.png` y `bundle.json` para el cliente, `items.json` y
`spells.json` para los dos lados, y `map1.json` para los dos lados.

`items.json` lleva seis campos que no salen de `obj.dat` tal cual: `projectile`
y `needsAmmo` (Proyectil y Municiones, que no son lo mismo — las Cuchillas se
tiran y listo, el arco necesita carcaj), `newbie`, los cortes de armadura
(`dwarfArmor` = RazaEnana, que cubre Enano y Gnomo, más `drowArmor` y
`femaleArmor`) y **`sold`**, que no es un campo de `obj.dat` sino la respuesta a
"¿esto lo vende alguien?", sacada de los `ObjN=` de `NPCs.dat`. Ese último es el
que le permite al kit inicial distinguir el equipo básico de una clase del
trofeo que un GM repartía — ver RESUMEN-FUNCIONAL §9. Las flechas (`ObjType` 32)
se convierten desde que existe el kit del Cazador.

La lista de cuerpos ya no se pasa a mano: se deriva de las armaduras de
`obj.dat`, porque equipar una armadura *es* cambiar de cuerpo. Hoy salen 309
cuerpos, 79 armas, 9 escudos y 18 cascos, más las 50 animaciones de `Fxs.ini`
(efectos de hechizo y de meditar) — ver "Gráficos de hechizos" más abajo.

### Reemplazar un gráfico de AO por otro

El atlas es un artefacto de build: se regenera desde los datos originales de
Argentum, así que **cualquier cosa editada a mano sobre `atlas.png` desaparece
la próxima vez que corre `aoconv`**. Para el arte que viene de AO eso está
bien; para el que no viene de ahí no sirve, porque no hay archivo original que
leer. Ese arte vive en `tools/aoconv/overrides/` y se pinta encima del atlas ya
empaquetado, después del color key — si se aplicara antes, la regla de AO
(negro puro = transparente) le abriría agujeros a lo que el reemplazo dibuje en
casi-negro.

- `anim<N>.png` es una tira horizontal que reemplaza la animación del grh N: se
  parte en tantas celdas iguales como frames tenga esa animación.
- `grh<N>.png` reemplaza un solo grh estático.

Los tamaños tienen que dar exactos. `aoconv` no tiene dependencias de imágenes
y meter un resampler para tapar una tira que no encaja escondería la causa más
probable: arte recortado contra un `bundle.json` viejo, cuyos frames se movieron
cuando el atlas se re-empaquetó. Se apaga con `-overrides ""`.

Hoy hay uno solo: **`anim259.png`, el Apocalipsis** — el grh que `Fxs.ini` le da
al FX 13, que es el que `Hechizos.dat` le pone a ese hechizo. 21 frames de
145×145. `overrides/apocalipsis.py` es de dónde salió, y no corre en el build:
`aoconv` lee el PNG ya hecho. Está guardado porque sin él el recorte no se puede
rehacer — la hoja de origen tiene las divisorias *dibujadas* en vez de
calculadas (pasos de 347, 347, 349, 343, después un salto de 1154), las filas no
miden lo mismo, y viene de un JPG cuyo ruido de compresión hace que el color key
exacto del resto del pipeline deje un recuadro sucio alrededor de cada frame.

**El ancho del atlas está a propósito en 2048, no en 1024.** Subir un ancho
angosto hace que el empaquetador (shelf-packer, apila por altura) tire una
imagen más alta de lo que cualquier GPU soporta como textura 2D — pasó al
sumar los FX, ver DIFICULTADES.md §12. Si se agrega contenido nuevo al bundle
y el juego se rompe visualmente entero (no solo lo nuevo), es lo primero a
revisar: `xxd -s 16 -l 8 client/assets/ao/atlas.png` da el ancho/alto del PNG
en los bytes 17-24 (big-endian); la altura tiene que quedar bien por debajo de
16384.

**Para leer la lógica del original, no solo sus datos:** el código fuente VB6
(no los assets) está en `ao-cliente-master.zip` y `ao-server-master.zip`, en
Downloads también (bajados de [ao-libre](https://github.com/ao-libre)).
Extraer solo el código, sin los binarios pesados que ya están en `ao-assets`:

```powershell
Expand-Archive ao-cliente-master.zip -DestinationPath src-cliente
Expand-Archive ao-server-master.zip -DestinationPath src-server
```

`src-cliente\...\CODIGO\` y `src-server\...\Codigo\` tienen los `.bas`/`.frm`/
`.cls`. Es donde se confirmó, por ejemplo, que un hechizo dibuja su efecto
anclado al *objetivo* (no un proyectil) y que Meditar es un toggle de F6 — ver
`SendSpellEffects` en `modHechizos.bas` y `HandleMeditate` en `Protocol.bas`
del server original.

Después de regenerar, Godot necesita reimportar:

```powershell
godot --headless --path client --import
```

Y si agregaste un script con `class_name`, además hace falta un pase del editor
para que registre la clase global:

```powershell
godot --headless --path client --editor --quit-after 2
```


### Tocar los gráficos de interfaz (login y panel lateral)

Dos pantallas están armadas con el mismo criterio, y no es el criterio habitual
de un motor de juegos: **el arte es una sola imagen horneada, y los controles
vivos se posicionan encima cayendo en los agujeros que el arte ya dibuja.**

| pantalla | arte | tamaño nativo | código |
|---|---|---|---|
| login / creación | `client/assets/ao/ui/login_bg.png` | 855x756 | `scripts/character_picker.gd` |
| panel lateral | `client/assets/ao/ui/panel_bg.png` | horneado a 525x962, que es como se muestra | `scenes/main.tscn` + `scripts/hud.gd` |
| footer de equipo | `client/assets/ao/ui/footer_bg.png` | horneado a 1088x37 | `scenes/main.tscn` + `scripts/hud.gd` |

Dos piezas más salieron del panel a su propio PNG porque tienen que moverse o
aparecer y desaparecer: `scroll_grabber.png` (el hueso del scroll) y
`scroll_rail.png` (el riel con sus dos placas de flecha). El criterio está más
abajo, en "Cuando un control tiene que usar el arte, no taparlo".

Se llegó a esto después de fallar cuatro veces reconstruyendo el marco a mano
con `StyleBoxFlat` (DIFICULTADES.md §2): un `StyleBoxFlat` no tiene bisel, ni
textura, ni hueso tallado. La consecuencia es que **cada offset es una medición
sobre el PNG, no un número a ojo**, y que si cambia la imagen o el tamaño al
que se muestra, hay que volver a medir.

**El panel lateral se mide sobre el PNG ya horneado, no sobre el fuente.** El
arte se recorta y se baja a 525x962 con Lanczos *antes* de medir nada, así que
los offsets de `main.tscn` viven en el mismo espacio en el que Godot los va a
dibujar y no queda ningún factor de escala que equivocar. El panel viejo se
medía sobre un fuente de 1426x2612 y se multiplicaba por 0.3682: andaba, pero
convertía cualquier cambio de arte en una ronda de aritmética.

#### Cambiar el tamaño del login

Es un solo número. `character_picker.gd` tiene los rects en el espacio nativo
del PNG (855x756) y calcula un `_scale` para llenar la pantalla:

```gdscript
const PANEL_BASE := Vector2(855, 756)
const PANEL_MARGIN := 32.0   # aire libre arriba y abajo; más chico = panel más grande
```

Todo lo demás — posiciones, tamaños, cuerpos de letra, radios y padding — se
multiplica por `_scale` al construirse. Arte y controles no se pueden
desalinear porque hay un solo número que decide. Con el viewport en 1613x962 y
margen 32 el panel sale a 1.19x, o sea 1015x898.

**Ojo con el filtro de textura.** El proyecto tiene
`default_texture_filter=0` (nearest), que es lo correcto para los sprites del
mundo y lo incorrecto acá: `login_bg.png` es arte pintado, no pixel art, y
`_scale` no es entero — en nearest los biseles del marco se convierten en una
escalera. El `TextureRect` del arte pisa el default con
`TEXTURE_FILTER_LINEAR`.

#### Cambiar el arte del panel

```powershell
# 1. recortar el arte (si viene con el ajedrez de "transparencia", sacarlo) y
#    hornearlo al tamaño en el que se muestra
python -c "from PIL import Image; im=Image.open(r'<fuente>.jpg').crop((6,11,1495,2775)); im.resize((525,962), Image.LANCZOS).save(r'client/assets/ao/ui/panel_bg.png')"
# 2. reimportar
godot --headless --path client --import
```

Después hay que volver a medir los agujeros y actualizar `main.tscn` y las
constantes de `hud.gd` (`BAR_*`, `ACTION_*`). El tamaño del agujero grande no
es una constante: la grilla y el libro se dimensionan leyendo los offsets que
sus propios nodos tienen en `main.tscn` (`_hole_of`), así que la medición vive
en un solo lugar y el layout la sigue.

#### Medir un agujero nuevo en el arte

Un umbral de "píxeles oscuros" **no sirve** en el arte del login: la madera del
fondo y el interior de las canaletas están a pocos niveles de luminancia de
distancia, así que cualquier threshold encuentra el panel entero, no el hueco.
Lo que sí funciona es perfilar la luminancia cruzando el bisel — el borde
biselado es una banda clara de 6 a 10 px, y el interior arranca justo después:

```powershell
Add-Type -AssemblyName System.Drawing
$b=[System.Drawing.Bitmap]::FromFile('D:\juegito\client\assets\ao\ui\login_bg.png')
# fila horizontal a la altura de la canaleta: se ve oscuro / banda clara / oscuro
$s=""; foreach($x in 95..125){ $p=$b.GetPixel($x,245); $s += "{0,4}" -f [int](($p.R+$p.G+$p.B)/3) }
$s
```

En ese ejemplo la salida daba `12 9 8 9 10 | 34 50 47 51 50 51 49 55 64 43 | 23 11 7 ...`:
madera hasta x=99, bisel de x=100 a x=109, interior desde x=110. Lo mismo por
columnas para el alto. Los rects que quedan en el código son **interiores**: un
control del tamaño del hueco cae adentro del bisel en vez de desbordarlo, que
es lo que la primera pasada hizo mal.

Para las placas de botón conviene detectar el rojo en vez de la luminancia
(`$p.R - $p.B -gt 22`), porque el granate del plaquete se despega de la madera
mucho más limpio que el brillo.

**En un panel oscuro lo que más rinde es el etiquetado de componentes conexas
sobre una máscara de "casi negro".** Los agujeros del panel lateral — el área
de inventario, las cinco canaletas de barras, las cajitas de conteo de
pociones, la placa al lado del cofre — son todos interiores negros rodeados de
marco, así que un flood fill sobre `luminancia < 28` devuelve **todos sus
bounding boxes exactos en una sola pasada**, y de ahí salieron los rects de
`main.tscn` sin tocar un número a mano. El perfilado de luminancia queda para
lo que no es un agujero negro: las placas de botón, las pestañas sobre la
franja de ladrillos y el plaquete del nombre.

#### Cuando el arte no tiene la proporción que necesitás

El footer llegó como una barra de 2666x280 — proporción 9.5:1 — y tiene que
entrar en 1088x37, o sea 29:1. Ahí no alcanza con escalar: a escala uniforme
esa barra mide 114 px de alto, y estirar la imagen entera deforma los
recuadros y los iconos, que son justamente lo que tiene que quedar cuadrado.

La salida es **hornear el arte por partes, cada una con su propia escala**, que
es lo que hace el script de `footer_bg.png`:

1. **El marco se tilea, no se estira.** Se toma un tramo liso de la barra (sin
   recuadros), se lo escala **uniforme** al alto final y se lo repite hasta
   llenar el ancho. Uniforme es la palabra importante: apretar la barra entera
   a 37px de alto aplasta los remaches y quedan guiones en vez de puntos;
   escalar parejo y repetir los deja redondos.
2. **Las puntas se pegan aparte**, con la misma escala, así la barra cierra
   como cierra el arte en vez de terminar cortada a la mitad de un tramo.
3. **Los recuadros van a su propia escala**, más grande que el marco: son el
   contenido y tienen que quedar legibles, así que se llevan 29 de los 37 px.
   Los de valor además se ensanchan a 64 px, porque adentro va un `5-10` y no
   un icono.

El resultado es un arte compuesto: el marco es textura y los recuadros son
contenido, y no tienen por qué compartir escala. Si algún día hace falta que
sí la compartan, ahí sí conviene pedir el arte de nuevo con la proporción
final.

#### Cuando un control tiene que usar el arte, no taparlo

El arte pinta cosas que *parecen* controles: una barra de scroll de hueso, las
placas de LANZAR e INFO. Si el control vivo se dibuja aparte, terminás con dos
— fue literal en el caso del scroll, con la barra gris de Godot al lado del
hueso pintado que no hacía nada. La regla que quedó:

**Lo que no se mueve se deja pintado en el fondo. Lo que se mueve se recorta
del fondo y se le da al control.** Con una corrección que costó un rediseño:
**tampoco alcanza con que no se mueva, tiene que pertenecer siempre a lo que
hay en pantalla.**

- **Barra de scroll de hechizos.** El hueso con el anillo de hierro se recortó
  a `scroll_grabber.png` porque se mueve. El riel y las dos placas de flecha
  no se mueven, así que estuvieron pintados en `panel_bg.png` — hasta que se
  vio que son muebles **del libro de hechizos**, no del panel: en la pestaña
  de inventario no scrolleaban nada y igual le comían 26px de ancho a la
  grilla, que es un cuarto de columna de slots. Ahora son `scroll_rail.png`,
  un `TextureRect` (`ScrollRail`) clavado en el mismo x 429..456, y 277..582
  donde el arte los tenía, que se muestra y se esconde con la pestaña. El
  agujero que dejaron en el PNG se rellenó copiando una franja del propio
  agujero negro, no con un negro plano: el fondo tiene grano y un rectángulo
  liso se nota.
- **El `VScrollBar` vivo** (`SpellScroll`) va encima del riel, usa el hueso
  como `grabber` con 9-slice, y sus botones de flecha llevan un icono
  **transparente de 20x20** — invisibles, pero es lo que los mantiene arriba
  de las flechas dibujadas y clickeables. La barra propia del
  `ScrollContainer` queda en `vertical_scroll_mode = 3` (nunca se muestra) y
  las dos se sincronizan en los dos sentidos, así la rueda mueve el hueso y el
  hueso mueve la lista. Además, mientras arrastrás un hechizo, acercarlo a
  cualquiera de las dos puntas de la lista la hace scrollear sola
  (`HUD._process`): un drag te tiene el botón apretado, así que sin eso el
  libro solo se puede reordenar entre las diez filas que estén a la vista.
- **Placas de LANZAR / INFO.** La placa se recortó a `button_plate.png` y el
  botón la dibuja en sus cuatro estados sobre el rect exacto de la placa
  pintada, así que en reposo coinciden píxel a píxel. Al apretar, la placa
  dibujada baja 2 px (`expand_margin_top` negativo y `expand_margin_bottom`
  positivo mueven la caja sin cambiarle el tamaño) y se oscurece: la franja de
  placa pintada que queda descubierta arriba es la sombra, y es el bisel del
  propio arte en vez de un degradado inventado.

**Ojo con el `content_margin` de un `StyleBoxTexture`.** Alimenta el tamaño
mínimo del control, así que márgenes generosos (los mismos que el 9-slice)
hicieron que el botón midiera 42 px contra una placa de 39 y estirara la
textura fuera de la pintada. Los márgenes de contenido son para el texto; los
de 9-slice son para la imagen, y no tienen por qué ser iguales.

#### Verificar sin abrir el juego

El loop rápido es dibujar los rects sobre el propio PNG y mirarlo, en vez de
lanzar el cliente y comparar de memoria:

```powershell
Add-Type -AssemblyName System.Drawing
$src=[System.Drawing.Bitmap]::FromFile('D:\juegito\client\assets\ao\ui\login_bg.png')
$g=[System.Drawing.Graphics]::FromImage($src)
$pen=New-Object System.Drawing.Pen([System.Drawing.Color]::Lime,1)
# x,y,w,h de cada rect que querés comprobar
foreach($r in @(@(112,220,608,44),@(660,689,141,36))){ $g.DrawRectangle($pen,$r[0],$r[1],$r[2],$r[3]) }
$src.Save("$env:TEMP\overlay.png"); $g.Dispose(); $src.Dispose()
```

Ahí se ve de una si un control se pasa del hueco. Así se encontró que el rect de
SALIR se pasaba 10 px por izquierda y 24 px por derecha, que era el tinte de
hover sobresaliendo del plaquete.

#### Depurar un panel que se dibuja mal

Los paneles de este proyecto se arman **por código** (`Control.new()` +
`add_child`), no en el editor, así que los bugs de layout no se ven abriendo la
escena. La escalera que cierra el diagnóstico rápido, de afuera hacia adentro:

1. **Capturar la ventana y medir el bloque pintado.** `GetClientRect` +
   `CopyFromScreen`, y después contar el bounding box de los píxeles que no son
   el color de fondo. Un bloque de 428x378 dijo enseguida de qué se trataba: es
   exactamente la mitad de 855x756, o sea un panel centrado en (0,0).
2. **Leer el color del vacío.** Gris del sistema, negro de letterbox y clear
   color del engine (RGB 77,77,77 = `Color(0.3,0.3,0.3)`) son tres problemas
   distintos. Ver DIFICULTADES.md §13.
3. **Comparar backends.** Si `gl_compatibility`, `--rendering-method
   forward_plus` y `--rendering-driver opengl3_angle` dan el mismo resultado, no
   es el driver.
4. **Capturar el viewport desde adentro del engine.** Si
   `get_viewport().get_texture().get_image()` ya sale roto, la ventana no
   participa y el problema es la escena.
5. **Volcar el árbol de `Control` con tamaños reales**, que es donde
   normalmente aparece el culpable:

```gdscript
# guardar como client/probe_tree.gd y correr:
#   godot --path client --script res://probe_tree.gd
extends SceneTree
var f := 0
func _initialize() -> void:
	change_scene_to_file("res://scenes/main.tscn")
func _walk(n: Node, d: int) -> void:
	if n is Control:
		var c := n as Control
		print("  ".repeat(d), n.name, " size=", c.size, " anchors=", c.anchor_right,
			",", c.anchor_bottom, " offsets=", c.offset_right, ",", c.offset_bottom)
	for ch in n.get_children():
		_walk(ch, d + 1)
func _process(_d: float) -> bool:
	f += 1
	if f == 90:
		print("visible_rect = ", root.get_visible_rect())
		_walk(root.get_node("Main/UI"), 0)
		return true
	return false
```

**Lo mismo sirve para probar lógica de HUD sin abrir una ventana:** el mismo
script con `--headless` instancia la escena, le podés mandar un `set_loadout`
de mentira y leer lo que quedó en los `Label`. Es como se verificaron el footer
de equipo y la barra de scroll de hueso sin lanzar el cliente.

**La trampa que ya costó una sesión entera:** `set_anchors_preset(preset)` tiene
`keep_offsets` en `true` por default, así que llamarlo sobre un nodo **que ya
está en el árbol** y mide 0x0 lo deja clavado en 0x0 para siempre — pone los
anchors a pantalla completa y compensa los offsets para no cambiar el rect.
Antes de `add_child` es inocuo (el área del padre todavía es 0, los offsets
quedan en 0); después de `add_child` es destructivo. Si el control tiene que
llenar a su padre, va `set_anchors_and_offsets_preset()`. Historia completa en
DIFICULTADES.md §13.

---

## 4. Buildear el cliente web

```powershell
.\scripts\build-web.ps1 -Godot "C:\ruta\a\Godot_v4.7.1-stable_win64_console.exe"
```

Requiere los **export templates** de la versión exacta (4.7.1), que son una
descarga aparte de ~1.3 GB. Si faltan, el export falla con "No export template
found". Se instalan desde el editor (Editor → Manage Export Templates) o a mano
en `%APPDATA%\Godot\export_templates\4.7.1.stable\`.

El script exporta y después **precomprime**. Eso no es cosmético:

| Archivo | Crudo | Comprimido |
|---|---|---|
| `index.wasm` | 38.8 MB | **10.1 MB** |
| `index.pck` | 12.9 MB | **11.6 MB** |
| **Primera carga** | **51.7 MB** | **21.7 MB** |

El `.pck` creció de 8.4 a 12.9 MB cuando el atlas sumó las 309 armaduras del
juego: el peso de la primera carga lo domina el contenido gráfico, no el
motor.

El servidor sirve el `.gz` a quien acepte gzip y cae al archivo plano si no
(`transport.precompressed`). Se comprime una vez en el build y no por request,
porque la máquina que lo sirve es un shared-cpu de 256 MB.

Para probarlo servido, sin deployar:

```powershell
go run -C server ./cmd/server -web-dir ..\build\web -map-file maps\map1.json -items-file maps\items.json -spells-file maps\spells.json
# y abrí http://localhost:8080
```

---

## 5. Publicar en GitHub

```powershell
git add -A
git commit -m "mensaje"
git push
```

Remote: `https://github.com/juanCruzAldaya/ArgentumGonline.git`

**Esto no es opcional si vas a deployar.** El AGPL-3.0 es copyleft de red: desde
el momento en que alguien que no sos vos juega en tu servidor, tiene derecho al
código fuente completo de lo que está corriendo. El orden correcto es siempre
pushear primero y deployar después, para que el código publicado sea el que
corre.

**Hoy el cliente no muestra el link al repo.** La pantalla de login lo mostraba
al pie; se sacó para dejarla limpia, y la atribución vive únicamente en
[README.md](README.md) y [CREDITS.md](CREDITS.md). Eso alcanza mientras el juego
se distribuya como repo, pero **no** para la build web hosteada: ahí el jugador
nunca ve el README, y la sección 13 del AGPL pide que la oferta de código le
llegue a quien interactúa con el programa por red. Antes de dejar el deploy
público hay que devolver el link a algún lado del cliente — una línea chica en
el HUD o en la pantalla de muerte alcanza.

---

## 6. Deploy en Fly.io

### Primera vez

```powershell
# instalar flyctl
$env:FLYCTL_INSTALL = "$env:USERPROFILE\.fly"
iwr https://fly.io/install.ps1 -useb | iex

# login (abre el browser)
& "$env:USERPROFILE\.fly\bin\flyctl.exe" auth login

# crear la app
& "$env:USERPROFILE\.fly\bin\flyctl.exe" apps create juegito
```

### Deploy

```powershell
$fly = "$env:USERPROFILE\.fly\bin\flyctl.exe"
.\scripts\build-web.ps1 -Godot "<ruta a Godot>"   # el Dockerfile copia build/web
& $fly deploy --remote-only
& $fly scale count 1                              # UNA sola máquina, ver abajo
```

`--remote-only` buildea en los builders de Fly, así que no necesitás Docker
local.

### ⚠️ Una sola máquina, siempre

Fly crea **dos** máquinas por defecto para alta disponibilidad. Para este
servidor eso está mal: el estado del mundo vive en memoria, así que dos máquinas
son **dos partidas distintas**, y dos personas que abren el mismo link pueden
caer en mundos separados y no verse nunca. Se detecta en los logs — cada máquina
imprime su propio `mapa cargado`. Después de cada deploy:

```powershell
& $fly scale count 1
```

### Apagar sin gastar

```powershell
& $fly scale count 0     # destruye las máquinas; ni cómputo ni rootfs
```

La app y su config quedan, así que retomar es un `fly deploy` y nada más.

### Verificar que quedó bien

No alcanza con que el deploy diga "success" — el servidor levanta igual sin sus
datos.

```powershell
curl.exe -s https://juegito.fly.dev/healthz    # ok map="Ciudad de Ullathorpe" items=496 spells=50
curl.exe -sI -H "Accept-Encoding: gzip" https://juegito.fly.dev/index.wasm   # debe decir content-encoding: gzip
& $fly logs -a juegito --no-tail | Select-String "mapa cargado|items cargados|hechizos cargados"
```

El cuerpo de `/healthz` es la respuesta corta: `ok` con el mapa y los conteos
significa que el mundo cargó, y `degradado: ...` significa que está corriendo
sin sus datos. Un `200` pelado no distinguía esos dos casos, que es justo lo
que dejó pasar el primer deploy roto.

Y la prueba de verdad, que ejercita el protocolo desde afuera:

```powershell
go run -C server ./cmd/bot -url wss://juegito.fly.dev/ws -n 3
& $fly logs -a juegito --no-tail | Select-String "player joined"
```

### Costos

Fly no tiene free tier desde octubre de 2024. Para este deploy:

| Concepto | Precio |
|---|---|
| `shared-cpu-1x` 256MB prendida siempre | ~$2/mes — no aplica, duerme sola |
| Máquina dormida | $0.15 por GB de rootfs / 30 días → centavos |
| Ancho de banda saliente, Sudamérica | **$0.04/GB** |

El ancho de banda es lo único que mueve la aguja, y ahora está medido en vez de
estimado: **21.7 MB por visitante nuevo** (la primera carga del cliente) más
**115 MB por hora de juego y por jugador** (los snapshots). Una partida de 50
personas sale ocho centavos. Las tablas completas, con el desglose por escala,
están en §7.

**La región `eze` (Ezeiza) está deprecada** y Fly no provisiona ahí. Estamos en
`gru` (São Paulo), lo único que queda en Sudamérica. Medido desde Buenos Aires:
**42 ms de ida y vuelta**, que con la espera de tick dan 67 ms de latencia
sentida — ver §7.

**La cuenta en trial apaga la máquina a los 5 minutos.** Sale en los logs
(`Trial machine stopping. To run for longer than 5m0s, add a credit card`) y se
confirma porque el uptime da 301 segundos clavados. Como el estado del mundo
vive en memoria, cada apagón reinicia la partida entera: loot nuevo, todos
afuera, bajas en cero. Hasta que haya tarjeta, Fly no sirve para jugar más de
cinco minutos seguidos.

---

## 7. Latencia y recursos, medidos

Nada de esta sección está estimado. Todos los números salen de
`server/cmd/probe`, que entra al servidor como un jugador más y mide lo que un
jugador siente:

```powershell
go run -C server ./cmd/probe -url ws://127.0.0.1:8080/ws -for 30s
go run -C server ./cmd/probe -url wss://juegito.fly.dev/ws -for 30s
```

El ping usa el propio protocolo (`TypePing`): `session.go` devuelve el
timestamp tal cual, sin que el servidor guarde estado de reloj, así que la
resta se hace entera del lado del cliente y no hay relojes que sincronizar.

### Latencia

La latencia que se siente tiene dos partes que se suman, y solo una es red:

```
                       ping/pong             llegada de snapshots        total
                                          (deberían ser 50 ms clavados)
local                mediana   0.0 ms  σ 0.2    mediana 50.0  σ  0.6     25 ms
Fly.io (gru)         mediana  41.9 ms  σ 1.8    mediana 49.7  σ  2.2     67 ms
túnel casero (gru20) mediana  91.2 ms  σ19.0    mediana 50.5  σ 26.1    116 ms
```

**Los 25 ms de tick son el piso y no dependen de la red.** El mundo mira los
comandos encolados una vez por tick, así que un click cae en algún punto de
`[0, 50 ms)` y espera 25 en promedio antes de que el servidor lo mire. Bajar
eso es subir el tick rate: a 30 Hz serían 17 ms, a cambio de un 50% más de
tráfico, porque el volumen escala con la cantidad de snapshots.

**Lo que decide la sensación no es la mediana, es el desvío.** Por Fly los
snapshots llegan con σ 2.2 ms: la cadencia de 20 Hz sobrevive la red casi
intacta. Por el túnel casero, σ 26 ms, con huecos de 0 y de 93 — o sea que
llegan **apelotonados**, el mundo se congela ~90 ms y después salta dos ticks
juntos. Un retardo constante el cliente lo tapa interpolando; contra uno que
salta no puede hacer nada. Es la diferencia entre "va lento" y "va a tirones",
y por eso Fly se siente mucho mejor que el túnel aunque la mediana sea solo el
doble.

De referencia: un servidor de Argentum bien hosteado anda en **32 ms**. La
única configuración que llega ahí es una IP pública propia con el puerto
directo (~30 ms totales); Fly en `gru` es lo mejor alcanzable sin depender del
proveedor de internet.

### Qué consume el servidor

Medido con 40 bots conectados, o sea el mundo real corriendo a 20 Hz:

```
RAM (working set) : 22.6 MB
CPU               : 15.1% de un core
threads           : 29
```

RAM y CPU **no son el cuello**. Una sola goroutine dueña de enteros, con
interest management por viewport, es barata: la máquina más chica de Fly
(`shared-cpu-1x`, 256 MB) sobra. Ojo con un detalle de esa medición: se tomó en
un desktop, donde un core es bastante más rápido que el `shared-cpu-1x` de Fly,
que tiene cuota base y burstea por encima. Antes de invitar a 50 personas
conviene repetirla contra la máquina real.

### Qué consume la red

El único número que mueve la aguja. Cada jugador recibe **20 snapshots por
segundo, esté quieto o no**:

| Jugadores | Salida | Costo/hora en `gru` ($0.04/GB) | Partida de 20 min |
|---|---|---|---|
| 1 | 115 MB/h | $0.005 | $0.002 |
| 4 | 0.45 GB/h | $0.02 | $0.007 |
| 10 | 1.13 GB/h | $0.05 | $0.02 |
| 50 | 5.63 GB/h | $0.23 | **$0.08** |
| 100 | 11.3 GB/h | $0.45 | $0.15 |

Una partida entera de 50 jugadores cuesta **ocho centavos**. El snapshot pesa
1679 bytes con pocos jugadores en pantalla y 2100 con veinte bots dando
vueltas, porque lleva solo las entidades del viewport — el volumen crece con lo
que ves, no con cuánta gente hay en el mapa.

**Acá es donde el codec binario deja de ser prolijidad y pasa a ser plata.** El
99% del tráfico son esos snapshots en JSON; el `welcome` con el mapa de
colisiones entero son 1857 bytes una sola vez. Un codec binario corta eso entre
5 y 8 veces, y recién a ~50 jugadores concurrentes el ahorro justifica el
trabajo. Antes de eso, no.

### Qué necesita el que juega

| | |
|---|---|
| Primera carga (web) | **21.7 MB** comprimidos — `index.wasm.gz` 9.7 + `index.pck.gz` 11.1 |
| VRAM | **74 MB** solo del atlas (2048x9516 RGBA) |
| Red en partida | 32.8 KB/s de bajada (~0.26 Mbps); la subida es despreciable |
| Requisitos | WebGL2 y `SharedArrayBuffer` — de ahí los headers COOP/COEP |

Los 74 MB de VRAM son el número más fácil de bajar: el atlas empaqueta las 309
armaduras del juego entero porque cualquiera puede aparecer en el piso. Un BR
podría spawnear solo un subconjunto por partida — está en §9.

### Hostear desde casa

Se probó y **hoy no se puede llegar directo**: la conexión está detrás de
CGNAT. El traceroute lo dice sin ambigüedad, el salto 2 cae en el rango
reservado de carrier-grade NAT:

```
1   192.168.100.1    router
2   100.72.48.2      CGNAT del proveedor (100.64.0.0/10, RFC 6598)
3   10.10.7.3        red privada del ISP
5   45.68.8.181      recién acá empieza el internet público
```

Con eso, **abrir el puerto en el router no sirve**: las conexiones entrantes
mueren en el NAT del proveedor antes de llegar. Hace falta pedirle IP pública
al ISP. Tampoco hay IPv6, que sería el otro atajo. Ojo con el diagnóstico
rápido: mirar que la IP pública no esté en `100.64/10` **no alcanza** — el
rango CGNAT aparece del lado del router, no del lado que ve internet.

Mientras tanto la única vía es un túnel, y no todos sirven:

| | Ubicación | Ida y vuelta HTTP |
|---|---|---|
| Cloudflare (`cloudflared tunnel --url`) | gru20, São Paulo | **0.18 s** |
| localhost.run (`ssh -R`) | Virginia | 0.62 s |
| serveo.net (`ssh -R`) | Finlandia | 1.59 s |

Los dos últimos son injugables. Dos trampas del camino de Cloudflare:

- **El router de casa filtra `trycloudflare.com` a propósito**: resuelve
  `cloudflare.com`, `github.com` y hasta `region1.v2.argotunnel.com`, pero
  devuelve NXDOMAIN para el dominio de los quick tunnels. Se arregla apuntando
  la placa a `1.1.1.1` (`Set-DnsClientServerAddress`, necesita admin).
- **El cliente web no arranca por `http://` pelado.** El export tiene
  `thread_support=true` y `SharedArrayBuffer` solo existe en contexto seguro:
  HTTPS o `localhost`. Por eso un túnel con TLS sirve y una IP pelada no —
  ahí habría que reexportar sin threads, o usar el cliente nativo.

---

## 8. Cómo está armado

### La forma general

```
cliente Godot ──WebSocket──> servidor Go headless (autoritativo, 20 Hz)
     │                              │
     └── solo dibuja                └── dueño de todo el estado
```

El servidor no dibuja nada y el cliente no decide nada. El cliente **predice** su
propio movimiento para que se sienta inmediato, pero el servidor es el que
manda y corrige (ver más abajo).

### Una sola goroutine, cero mutexes

```
conexión ──goroutine de lectura──> cmdCh ─┐
                                          ├──> goroutine del mundo (20 Hz)
conexión ──goroutine de lectura──> cmdCh ─┘         │
                                                     └──> snapshot por viewport
                                                          ──> channel de escritura
```

Una sola goroutine es dueña de todo el estado del mundo. **No hay un mutex en
todo el repo.** Los comandos se encolan y se aplican al tick siguiente, así que
el orden es determinístico y los tests son reproducibles.

`Send` nunca bloquea: si un cliente no drena se le descartan frames, y a los 30
seguidos se lo desconecta. Un cliente lento no puede frenar la simulación.

Fue la decisión de arquitectura más rentable del proyecto: eliminó una categoría
entera de bugs antes de que existiera.

### Interest management

El viewport de **17×13 tiles** — la ventana clásica de AO — *es* el interest
management. Cada snapshot lleva solo las entidades y los objetos del piso que
están dentro de tu ventana. Con 50 jugadores en 100×100, cada uno ve a poquitos,
y un cliente modificado no puede aprender posiciones que no vio.

Lo que **nunca** viaja a un cliente ajeno: tu HP. Los vitals van solo al dueño.

### Movimiento: predicción y reconciliación

Es la parte con más sutileza, y la que más veces se rehizo.

El cliente original de AO hace predicción: `Map_MoveTo` valida la colisión
contra su propia copia del mapa y mueve el personaje **en el mismo frame** que
apretás la tecla, avisándole al servidor después. Por eso se siente directo. El
servidor de AO ni siquiera limita la cadencia — `HandleWalk` mueve y listo,
respaldado por un *detector* de speedhack.

Nosotros no podemos confiar en el cliente, así que:

1. **El cliente predice** (`WorldView.try_step`): chequea el bitset de
   colisiones que recibió en el Welcome, chequea que no haya otra entidad en el
   tile, y si pasa, se mueve ya.
2. **El servidor decide** igual, con su propio reloj de pasos.
3. **Se reconcilia** en `set_entities` por número de secuencia, no por voto de
   posiciones. Cada `Move` lleva un `Seq` propio (incremental, nunca se
   resetea en la conexión), y el servidor devuelve en cada `Snapshot` el
   `AckSeq`: el más alto que ya respondió, se haya movido o no. El cliente
   guarda un buffer de los inputs que mandó y todavía no fueron confirmados
   (`_pending`); en cada snapshot pisa su posición con la del servidor,
   descarta del buffer todo lo `<= AckSeq`, y vuelve a aplicar (replay) lo que
   queda — los inputs que el servidor todavía no tuvo tiempo de contestar.
   Es el patrón estándar de client-side prediction con reconciliación por
   secuencia. La versión anterior comparaba posiciones absolutas y forzaba la
   del servidor tras 4 snapshots seguidos en desacuerdo, lo cual se rompía bajo
   input rápido o sostenido: la posición predicha está *siempre* uno o dos
   pasos por delante de la última confirmada mientras se camina, así que el
   voto se disparaba solo y producía el rubber band que se supone que evitaba.
   Postmortem completo (§11, "tres bugs apilados") en [DIFICULTADES](DIFICULTADES.md).

La cadencia de paso corre en **milésimas de tick**, no en ticks enteros, porque
los enteros no pueden expresar el ajuste: 4 ticks son 5 tiles/s y 5 ticks son 4
tiles/s, o sea que el salto mínimo desde 100% es −20%. A 90% la cifra honesta es
4.444 ticks. El resto sobrevive de paso a paso (`moveReadyAt +=`, nunca se
resetea desde el momento real), y estar quieto no acumula crédito para un
sprint.

Como 4.444 ticks no es un número entero, el servidor en la práctica alterna
pasos de 250 ms y 200 ms para promediar los 222.2 ms reales de la cadencia —
nunca da un paso a los 222.2 ms exactos, porque solo puede actuar en un borde
de tick. El cliente predictivo tiene que espejar *eso*, no un cronómetro
continuo: `WorldView._quantized_now()` cuantiza su propio reloj a bordes de
tick con la misma aritmética entera que `moveReadyAt`, y calibra la fase
contra el `Tick` que ya viaja en cada `Snapshot` (`sync_server_tick`) en vez de
asumir que el reloj del cliente arrancó sincronizado con el del servidor —
ninguno de los dos arranca al mismo tiempo, así que sin esa calibración
tendrían el mismo tamaño de paso pero no caerían en los mismos instantes.

**Girar tiene su propio intervalo** (`INT_CHANGE_HEADING`, 300 ms), separado del
caminar. Un paso legal se lleva su giro puesto; el intervalo solo entra cuando
el movimiento se rechaza — pared, parálisis, o media cadencia. El cliente
predictivo replica esto con su propio cooldown de giro, y cada input que manda
sabe si es "solo giro" o "intento de paso" (`can_step`) — el replay de la
reconciliación respeta esa marca, porque tratarlos igual hacía que un giro se
reprodujera como si hubiera avanzado un tile.

Y la interpolación del render avanza **un eje por frame**: nunca en diagonal,
porque el juego nunca mueve en diagonal.

### Apariencia: la armadura ES el cuerpo

Lo menos obvio de todo el port. En Argentum, equipar una armadura no te pone una
capa encima: **te reemplaza el cuerpo**. El `NumRopaje` de `obj.dat` pasa a ser
`Char.body`. Por eso no hay campo "armadura" en el protocolo y no debe haberlo.
Desequipar te devuelve al cuerpo desnudo por raza (`DarCuerpoDesnudo`), nunca al
que tenías antes.

Arma, escudo y casco sí son capas, dibujadas en este orden:

```
cuerpo → cabeza → casco → arma → escudo
```

El arma va **arriba** de la cabeza, que es lo que le permite taparla mirando al
norte. Arma y escudo se dibujan en la posición del personaje **sin offset** —
son sprites de cuerpo entero, no accesorios pegados a una mano. Cabeza y casco
son las únicas capas con offset, y sale de `Personajes.ini` por cuerpo.

Tres trampas de los datos, todas reales:

- **"Nada equipado" es 2, no 0.** `NingunArma`, `NingunEscudo` y `NingunCasco`
  valen todos 2. Portarlo como 0 rompe dos veces: al desequipar pedís el índice
  0, y cualquier ítem que legítimamente use `Anim=2` se vuelve invisible.
- **Escudos y cascos traen un `NumRopaje=2` basura**, porque el original lo carga
  fuera del switch de tipo. Leerlo sin filtrar te pone el cuerpo en 2 al levantar
  un escudo.
- **`Personajes.ini`, no `Cuerpos.ini`.** El segundo es data muerta: dos de
  cuatro direcciones vacías y números de grh dentro del campo de offset de
  cabeza. Usarlo fue la causa raíz de tres "misterios" distintos del conversor.

### Balance: portado, no inventado

Las fórmulas de combate salen de `SistemaCombate.bas` literalmente, y las tablas
por clase y raza de `Balance.dat`. Cada constante cita su archivo de origen.

El maná es donde clase y raza se cruzan, y replica los dos sitios del original:
`SetAttributesToNewUser` para el nivel 1, y el brazo `AumentoMANA` del switch de
subir nivel para cada nivel después — como acá todos nacen al cap, se simulan las
44 subidas. La clase elige la fórmula; la raza la alimenta, porque todos los
términos de caster son un múltiplo de Inteligencia.

`baseAttribute = 20` sale de `MaxDados` del `Server.ini` original. Estuvo en 30
un tiempo, asumiendo un rango de dados que no existe — 40 es `MAXATRIBUTOS`, el
techo al que te sube una **poción**, no lo que se puede rolear. El error se
amplificaba 44 veces en el maná.

### Gráficos de hechizos (FX)

Lo primero que hay que soltar del original AO: un hechizo **no** dispara un
proyectil que viaja de quien lanza a quien recibe. `SendSpellEffects`
(`modHechizos.bas`) manda un paquete `CreateFX` con el `CharIndex` del
**objetivo**, y el cliente (`Char_SetFx` en `mPooChar.bas`) lo reproduce
anclado a esa posición, igual que dibuja cuerpo o cabeza — con el offset propio
de `Fxs.ini` (`OffsetX`/`OffsetY`) encima.

`Hechizos.dat` guarda ese vínculo en un campo mal nombrado: `FXgrh` no es un
grh, es el índice 1-50 dentro de `Fxs.ini`, que recién ahí resuelve el grh real
(`Animacion`) más el offset. `tools/aoconv` ya traía ese índice parseado desde
antes (`Spell.FXGrh`, `json:"fx"`) pero no hacía nada con él; ahora también
lee `Fxs.ini` completo (`loadFxs` en `aodata.go`) y empaqueta las 50
animaciones en el atlas (`bundle.go`), bajo la clave `fxs` de `bundle.json`.

El cliente no necesita que el servidor le diga qué grh mostrar: ya tiene su
propia copia de `spells.json` (con `fx` y `loops`) igual que la usa para
nombres e íconos, así que resuelve todo localmente a partir del
`SpellID` que ya viaja en cada `SpellEvent`. `WorldView.play_spell_fx` guarda
el efecto en una lista de "FX activos" con vencimiento (`grh`, `offset`,
inicio, duración = ciclo × `loops`) y `_draw_entity` lo dibuja mientras dure —
un efecto de hechizo es de una sola vez, así que expira solo.

### Meditar

F6, toggle. A diferencia del hechizo de arriba, el aura de meditar **no** es
un evento de una vez — dura mientras el jugador la sostiene — así que no usa
la misma lista de FX con vencimiento. Viaja como parte del estado parado de
cada snapshot (`EntityState.Meditating`, visible para *todo el que vea* a ese
jugador, igual que `Paralyzed`/`Immobilized`), y `WorldView._draw_fx` la
redibuja cuadro a cuadro mientras el flag siga en `true`.

El servidor tampoco manda qué aura mostrar: el original elige entre cinco
tamaños de FX según el nivel del lanzador (`FXMEDITARCHICO` .. `XXGRANDE`,
`Protocol.bas`), pero acá todos nacen al nivel tope (`maxLevel`, ver
`balance.go`) — por encima del umbral más alto del original — así que el
`switch` de tamaños siempre cae en la misma rama. El cliente hardcodea esa
única aura (`WorldView.MEDITATE_FX = 34`) en vez de portar un `switch` que
nunca elige otra cosa.

La regeneración de maná (`meditateTick` en `server/internal/world/meditate.go`)
es el único lugar donde el port se separa de la fórmula del original a
propósito: `DoMeditar` (`Trabajo.bas`) tira un roll de suerte contra la skill
Meditar para decidir cada cuánto cae un 6% de maná (`PorcentajeRecuperoMana`
de `Balance.dat`); acá no existe esa skill — nadie sube de nivel — así que el
6% cae en una cadencia fija (una vez por segundo) en vez de un roll, mismo
criterio que ya usaba `hide()` para Ocultarse. El resto sí es del original:
2 s de "concentración" antes de arrancar (`TIEMPO_INICIOMEDITAR`), se corta
solo al llenar el maná, se cancela caminando o al recibir un golpe cuerpo a
cuerpo, bloquea atacar y usar objetos mientras dura, y reduce en 25% la
evasión de quien medita (`SistemaCombate.bas`).

### Dónde vive cada cosa

```
server/
  cmd/server/      servidor de juego headless
  cmd/bot/         clientes headless para carga
  internal/
    protocol/      mensajes de wire + codec (JSON hoy, binario después)
    transport/     frames opacos; WebSocket; servido estático precomprimido
    world/         simulación autoritativa, tick loop, sesiones
      balance.go   tablas de clase/raza, maná, atributos
      combat.go    melee, muerte, fantasmas
      appearance.go  qué cuerpo/arma/escudo/casco se dibuja
      loot.go      esparcido en el piso, agarrar, tirar, desparramar al morir
      spells.go    50 hechizos, libro de 30 slots reordenable
      meditate.go  F6: regen de maná, ramp, corte por golpe/caminar
      useitem.go   equipar vs consumir, pociones
client/
  scripts/         net_client, world_view, hud, minimap, main,
                   character_picker, ao_data, ao_sprites, inventory_slot
  scenes/main.tscn estructura y posiciones; lo cosmético vive en hud.gd
tools/aoconv/      lee los índices de AO y arma el atlas y los .json
  overrides/       arte que reemplaza gráficos de AO, pintado sobre el atlas
```

---

## 9. Lo que falta

| Falta | Nota |
|---|---|
| **Zona que se achica** | La mecánica que define el género. Es lo que más falta. |
| **Mapa grande** | Mergear varios de los 317 mapas reales en una grilla con zonas coherentes. Es lo próximo. |
| **NPCs / bichos** | Sistema entero nuevo: spawn, IA, combate contra jugadores. Va después del mapa. |
| **Lobby / matchmaking** | Hoy se entra a un servidor corriendo; no hay partida con principio y fin. Plan: una máquina Fly por partida vía Machines API. |
| **Combate a distancia** | Arcos y flechas. Solo hay melee y hechizos. |
| **Codec binario** | Cuando el JSON moleste, medido. |
| **Recortar el atlas** | Hoy empaqueta las 309 armaduras del juego entero; un BR podría spawnear solo un subconjunto. |
| **Descarga Eléctrica con arte nuevo** | Quedó a mitad de camino: va en `overrides/anim221.png`, 15 frames de 128×128 (fx 11 → grh 221), contiguos en el atlas igual que el Apocalipsis. Falta la hoja de origen **sobre negro sólido** — la que se probó traía el damero de transparencia horneado y es irrecuperable, ver DIFICULTADES §15. |
| **Link al fuente en el cliente** | Se sacó del login. Requisito del AGPL §13 antes de que el deploy web sea público — ver §5. |
