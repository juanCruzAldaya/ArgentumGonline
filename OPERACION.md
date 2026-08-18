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

**El mapa se sortea.** `-worlds` toma por default el patrón
`maps/map1[0-9][0-9][0-9].json`, que son los cuatro mundos compuestos, y elige
uno al arrancar; `-map-file` queda como respaldo para cuando no hay ninguno.
`-world-seed N` fija cuál, para poder repetir una prueba.

Si el arranque está bien, el log dice estas líneas:

```
mundo sorteado       de=4 archivo=map1003.json
mapa cargado         number=1003 name=Yermo size="[760 760]"
loot esparcido       pedido=1435 colocado=1435
pociones esparcidas  pedido=2857 colocado=2857 unidades=14285
items cargados       count=496
hechizos cargados    count=50
world running        tickRate=20
listening            addr=[::]:8080
```

**No aparece "zona activa" hasta que entra el primer jugador**, y eso está bien:
el anillo no corre en un servidor vacío.

Si falta alguna de esas líneas, no estás corriendo el juego. El atajo para
preguntárselo al servidor, sin leer el log, es el health check — contesta qué
cargó, no solo que está vivo:

```powershell
curl.exe -s http://localhost:8080/healthz
# ok map="Ciudad de Ullathorpe" items=496 spells=50
```

Otros flags: `-addr` (default `:8080`), `-tick`, `-map-width`, `-map-height`,
`-seed`, `-debug`, `-web-dir`, `-respawn`.

De la zona: `-zone=false` la apaga y deja el mapa entero jugable, y
`-zone-speed N` divide todas sus duraciones — con 8 se ve una partida entera de
cierres en minuto y medio, con 40 en quince segundos. Es la única forma de
mirar las últimas etapas sin jugar hasta ellas.

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

**De qué está hecho el atlas.** Medido sobre el `bundle.json` de hoy: 2048×9516,
10.154 frames, 18,9 Mpx de contenido y **96% de ocupación** — el shelf-packer
casi no desperdicia.

| categoría | frames | Mpx | % del atlas | alto propio a 2048 |
|---|---:|---:|---:|---:|
| cuerpos | 6.678 | 8,54 | 45,3% | 4.906 px |
| FX | 632 | 6,28 | 33,3% | 3.607 px |
| armas | 1.583 | 1,78 | 9,4% | 1.023 px |
| tiles del mapa 1 | 621 | 1,62 | 8,6% | 931 px |
| items y demás | 395 | 0,43 | 2,3% | 245 px |
| escudos | 153 | 0,17 | 0,9% | 98 px |
| cabezas | 36 | 0,03 | 0,2% | 17 px |
| cascos | 56 | 0,02 | 0,1% | 10 px |

Dos números que deciden lo que viene. El primero: **los mapas comparten casi
todos sus tiles**. Los 317 mapas de AO usan 61.707 grhs sumados pero solo **6.338
distintos** — un factor de reuso de **9,7×**. Por eso el costo del segundo mapa no
se parece al del primero:

| mapas | grhs distintos | alto de tiles | atlas total | del límite |
|---:|---:|---:|---:|---:|
| 1 | 609 | 824 px | 9.516 px | 58% |
| 9 | 823 | 1.252 px | 9.944 px | 61% |
| 25 | 1.212 | 1.913 px | 10.605 px | 65% |
| 100 | 2.364 | 3.768 px | 12.459 px | 76% |
| 200 | 4.326 | 6.507 px | 15.199 px | 93% |
| 317 | 6.338 | 8.498 px | 17.190 px | 105% |

**Entran 282 mapas en un solo atlas de 2048 de ancho.** Un super-mapa de 10×10
no necesita páginas ni nada: queda en 73%.

El segundo número: el solapamiento entre los tiles del mapa y los frames de
personaje es **exactamente cero**, así que si algún día hace falta separarlos, el
corte es limpio.

**Si alguna vez hace falta partirlo: páginas, no PNGs sueltos.** El límite de
16.384 es sobre *una* textura, no sobre el total, así que la salida es partir el
atlas en varias páginas — no atomizarlo. Atomizarlo sería lo peor de los dos
mundos: el renderer 2D de Godot batchea por textura, y con un PNG por frame los
221 tiles del viewport pasarían de **1 bind de textura por frame a 221 o más**.
Tampoco ahorraría descarga, porque los assets viajan adentro de `index.pck`
igual. El corte que piden los números es por rol:

| página | contenido | alto a 2048 | cuándo se carga |
|---|---|---:|---|
| `atlas_chars` | cuerpos, cabezas, armas, escudos, cascos | ~6.050 px | siempre |
| `atlas_fx` | las 50 animaciones de `Fxs.ini` | ~3.607 px | diferible al primer hechizo |
| `atlas_map<N>` | los tiles de un mapa | ~930 px | uno por mapa, bajo demanda |
| `atlas_items` | íconos y sprites de piso | ~245 px | siempre |

Son 4 binds de textura por frame en vez de 1, contra 221+ de la versión atómica,
y ningún techo. El costo es agregarle un campo `page` a cada frame del
`bundle.json` y que `ao_sprites.gd` tenga un array de texturas en vez de una.
**No está hecho, y con los números de arriba probablemente no haga falta**: el
disparador real sería pasar de ~280 mapas, no el segundo.

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

### Generar los cuatro mundos

```powershell
go run -C tools/aoconv . `
  -assets "$env:USERPROFILE\Downloads\ao-assets" `
  -bundle "$PWD\client\assets\ao" `
  -heads 1,2,3,4,5,6,7,8,500 `
  -map 1 -worlds `
  -server-out "$PWD\server\maps"
```

`-worlds` lee `tools/aoconv/worlds/layout.json` y escribe, por cada mundo:

| archivo | para | qué lleva |
|---|---|---|
| `map100N.json` (cliente) | dibujar | las cuatro capas y los techos |
| `map100N.json` (servidor) | simular | solo el bitset de bloqueo, 112 KB |
| `map100N_mini.png` | el mapa y el minimapa | un píxel por tile, con el color real del terreno |

Ese PNG es el mismo truco del original: Argentum trae 325 BMP en
`Graficos/MiniMapa`, uno por mapa, y **nunca dibuja un minimapa desde los
tiles**. Go recorre los 577.600 tiles en milisegundos; el cliente solo carga la
imagen.

El atlas sale en **dos páginas** — ver "De qué está hecho el atlas" más arriba —
porque los tiles de cuatro mundos más los personajes no entran cómodos en una.

### Exportar los mundos a Tiled

Para que alguien con un editor abierto les agregue detalle:

```powershell
go run -C tools/aoconv . -assets "$env:USERPROFILE\Downloads\ao-assets" -tiled .\export
```

Sale un `.tmj` por mundo con cinco capas — `piso`, `objetos`, `encima`, `techo`,
`bloqueado` — más `tiles/`, una PNG por grh.

Dos decisiones hacen que el viaje de vuelta sea posible:

- **El tileset es una colección de imágenes, no una grilla.** Un grh es un
  rectángulo de tamaño arbitrario dentro de una hoja numerada; ninguna grilla
  uniforme lo describe.
- **Cada PNG se llama con su número de grh.** Un mapa editado sigue diciendo,
  tile por tile, qué gráfico de Argentum era. Arte nuevo va en otra carpeta y se
  distingue por eso.

Las capas van en base64 + zlib, que es lo que Tiled lee más rápido: un mundo
pesa ~450 KB en vez de los ~9 MB que serían en CSV.

### Verificar el conversor contra un parser independiente

Los formatos binarios de AO no están documentados (ver
[DIFICULTADES](DIFICULTADES.md) §1), así que "el juego se ve bien" no alcanza
como prueba de que se leen bien. La verificación que sí vale es escribir un
**segundo parser desde cero**, solo a partir del formato documentado, y comparar
campo por campo contra la salida de `aoconv`.

El parser de referencia vive en `tools/verify/verify_map.py` — deliberadamente
en otro lenguaje y por otro camino que `aoconv`, para que un error de lectura
tenga que repetirse idéntico en los dos lados para pasar desapercibido. Sale con
código 1 si algo no coincide, así que sirve en CI:

```powershell
python tools\verify\verify_map.py --map "$env:USERPROFILE\Downloads\ao-assets\Mundos\Mapa1.map"
```

Sobre Mapa1 da:

| campo | resultado |
|---|---|
| layer1 | 10.000/10.000 idénticos |
| layer2 | 10.000/10.000 idénticos |
| layer3 | 10.000/10.000 idénticos |
| layer4 | 10.000/10.000 idénticos |
| bloqueados | 10.000/10.000 idénticos |

50.000 comparaciones de campo, cero diferencias. Con dos señales más que
respaldan al parser independiente: consume **53651/53651 bytes exactos** — el
`.map` no lleva largo por tile, así que un solo byte mal leído desincroniza todo
lo que sigue y sería imposible terminar justo en EOF — y da 5038 tiles
bloqueados, el mismo número que `aoconv`. El parser de `.map` queda validado
para los 317 mapas, no solo para el 1.

**El dump de aoweb no sirve como oráculo.** aoweb es otro port de AO a la web
que también convirtió los mapas y el `obj.dat` a JSON, así que parecía un
segundo par de ojos gratis. No lo es: **usa otra distribución de Argentum**.
Contra su Mapa1 dan 3.015 diferencias en layer1 y 1.202 en bloqueados, y no las
explica ningún corrimiento — probadas las 49 combinaciones de (dx,dy) en ±3, la
mejor alineación sigue siendo (0,0). Cada campo en disputa se fue a buscar al
archivo original, y en todos ganó el nuestro:

- `obj.dat` dice literal `Name=Porcion de Tarta`, `Pan de Maiz`, `Sandia`, **sin
  acento**. Los acentos que trae aoweb no están en la fuente.
- `OBJ461` es `Pocion Roja (Newbie)` con `GrhIndex=5736`; aoweb dice 542.
- `OBJ1053` es `Tunica de Campeon` con `ObjType=3`; aoweb dice 24.
- Nuestro `obj.dat` trae 171 variantes de armadura por sexo y raza (sufijo
  `(H/E/EO-M)`) que su revisión no tiene.

La causa está en el header del propio `.map`, que se identifica como **"GS-Zone
Argentum Online MOD - Copyright GS-Zone 2012 - Original by Pablo Marquez"**.
Coinciden en el 70% porque es el mismo mapa base; difieren en las ediciones de
cada fork. **Mezclar sus datos con los nuestros inyectaría otra revisión del
juego** — y eso vale para cualquier asset suyo, no solo para los mapas.


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

**Una cuenta en trial apaga la máquina a los 5 minutos.** Ya no es el caso —hay
tarjeta cargada— pero queda anotado porque el síntoma no se parece a la causa:
la partida se reinicia sola cada cinco minutos, con loot nuevo, todos afuera y
las bajas en cero, y parece un crash del servidor. Lo que lo delata es el log
(`Trial machine stopping. To run for longer than 5m0s, add a credit card`) y el
uptime dando 301 segundos clavados.

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

#### Con la partida llena

Los números de arriba son con el servidor vacío. Repetido contra la máquina real
de Fly con **41 jugadores** adentro (40 bots más un humano), que es la carga que
tendría una partida de verdad:

```
                       ping/pong                llegada de snapshots      bytes/
                                                                       snapshot
1 jugador          mediana 45.5 ms  σ  5.4    mediana 50.1  σ 11.9        1443
41 jugadores       mediana 42.5 ms  σ 25.5    mediana 49.9  σ 17.0        1666
```

**La mediana no se mueve y la cadencia de 20 Hz aguanta**: el servidor sigue
mandando un snapshot cada 49.9 ms clavado, sin OOM, sin clientes descartados y
con el health check pasando todo el tiempo. Una sola goroutine dueña de enteros
no se despeina con 41 conexiones.

Lo que se degrada es **el desvío**, que es justo lo que decide la sensación: σ
del ping de 5.4 a 25.5 ms, con picos de 271 ms. El snapshot engorda un 15%
(1443 → 1666 bytes) porque el viewport se llena de gente, exactamente como
predice la tabla de la red.

**Ese desvío es una cota superior, no el jitter real de Fly.** Los 40 bots
salían de una sola máquina por una conexión hogareña —1.3 MB/s bajando por un
enlace doméstico, más el probe midiendo encima— así que parte del jitter es del
router de casa. Para separar las dos cosas hay que correr los bots desde otro
lado (otra máquina de Fly, o un VPS) y volver a medir.

La prueba entera, 13.6 minutos con 41 jugadores, salió **$0.042**: 1.04 GB de
salida a $0.04/GB. La tabla de la red de más abajo predijo ese número clavado.

### Con el mundo compuesto y la partida llena

Medido con `cmd/probe` entrando como jugador 101, contra un mundo de 760×760 y
100 bots adentro. Dos corridas de 60 s: una en local, para poder mirarle CPU y
RAM al proceso, y otra **contra Fly**, que es la que dice lo que se siente.

| | local | Fly (`gru`) |
|---|---|---|
| ping ida y vuelta | mediana 0,0 ms  σ 0,3 | mediana **50,7 ms**  σ **3,2** (p95 57,0, max 70,2) |
| llegada del snapshot | mediana 50,0  σ 0,8 | mediana 49,9  σ 14,9 (un hueco suelto de 512 ms) |
| snapshots recibidos | 1200 en 60 s | 1199 en 60 s |
| **tamaño del snapshot** | 543 B | 725 B |
| tráfico por jugador | 12,3 KB/s | 16,0 KB/s = 56 MB/hora |
| CPU del servidor | **28,1% de un core** | no medible desde afuera |
| RAM del servidor | 31,4 MB, 29 threads | ídem |
| descartes / desconexiones | 0 | 0 |
| lo que siente el jugador | 25 ms | **76 ms** (51 de red + 25 de tick) |

**Los 20 Hz aguantan 101 jugadores sin despeinarse**, y en Fly la mediana del
ping ni se mueve respecto del servidor vacío.

**El desvío mejoró con más del doble de jugadores**, que es lo contrario de lo
que decía la medición de 41 bots de más arriba: σ del ping de 25,5 ms bajó a
3,2, y el pico de 271 ms a 70. Eso confirma la sospecha que quedó anotada
entonces — **ese desvío era del router de casa, no de Fly**. Lo que cambió es
que el snapshot es cinco veces más chico, así que el enlace doméstico ya no va
al límite.

#### Dónde flaquea: 1000 jugadores, y el escaneo cuadrático

Con 1002 conectados nada se cae — cero descartes, cero desconexiones, y los
20 Hz se sostienen en la mediana. Lo que se degrada es la regularidad, que es lo
que decide la sensación:

| jugadores | desvío | p95 | min | CPU |
|---|---|---|---|---|
| 101 | σ 0,8 ms | 51,4 | — | 28,1% de un core |
| 502 | σ 4,8 ms | 58,2 | 36,5 | 81,7% |
| 1002 | σ 11,9 ms | 70,0 | **16,5** | **181,2%** |

Ese `min 16,5` es la firma del problema: los snapshots empiezan a llegar
**apelotonados**, un tick tarde y el siguiente pisándolo, o sea el tick loop
pasándose de los 50 ms y recuperando después.

La causa era que la CPU escalaba **peor que lineal** — 2x los jugadores costaban
2,2x — y eso delata un término cuadrático. Estaba en `viewportOf`, que recorría
**todos** los jugadores por cada jugador: con mil conectados, un millón de
comparaciones por tick, 20 millones por segundo, para encontrar los pocos que
están en pantalla. Es el mismo problema que ya tenía `groundItemsInView` con el
loot, y la misma solución: recorrer los 221 tiles del viewport y buscarlos en el
índice, en vez de recorrer a todo el mundo y descartar.

Con eso, los mismos 1000 bots:

| | antes | después |
|---|---|---|
| desvío | σ 11,9 ms | **σ 7,7 ms** |
| p95 | 70,0 ms | 63,1 ms |
| min | 16,5 ms | **32,7 ms** |
| CPU | 181,2% de un core | **140,0%** |

Y pasa a escalar **por debajo** de lineal: 2x los jugadores, 1,7x la CPU. Lo que
queda es serializar JSON, que es lineal y es el codec binario del roadmap.

La trampa del cambio: **los muertos no están en `occupied`**, porque el cadáver
deja de bloquear al morir. Escanear tiles los hacía desaparecer a todos del mapa,
así que hay un índice aparte de cadáveres — lista por tile, ya que no bloquean y
se apilan — que se toca en dos lugares nada más, porque un fantasma no camina.

#### El tamaño del snapshot depende del momento de la partida, no de la cantidad de gente

Es el matiz que faltaba, y explica por qué una medición anterior daba 3.588 B
para la misma escena: **el snapshot lleva solo el viewport**. Con 101 jugadores
recién entrados a un mundo de 760×760 casi nadie tiene a otro en pantalla, y son
543 bytes. Al final de la partida la zona los mete a todos en un círculo de 21
tiles de radio, todos se ven entre todos, y el snapshot vuelve a los ~3,6 KB.

O sea que estos números son el **piso**, no el techo: el pico de tráfico es el
endgame y dura los últimos minutos. Cualquier presupuesto de ancho de banda hay
que hacerlo con el número grande.

Eso deja al codec binario donde estaba — es la tarea con número — pero la
urgencia es menor de lo que parecía: los ~7 MB/s agregados con 100 jugadores son
el final de la partida, no toda la partida.

El Welcome también creció: lleva el bitset de colisión, un bit por tile, que
pasó de 1,7 KB en Ullathorpe a **96 KB** en un mundo compuesto. Va una sola vez
por conexión, pero es lo que obligó a subir los límites de lectura de los dos
lados — ver §18 de [DIFICULTADES](DIFICULTADES.md).

### Qué consume el servidor

Medido en un desktop, mirando el proceso que tiene el puerto 8080 y sacando la
diferencia de CPU acumulada sobre una ventana:

| bots | RAM (working set) | CPU | threads |
|---|---|---|---|
| 40 | 22,6 MB | 15,1% de un core | 29 |
| **100** | **31,4 MB** | **28,1% de un core** | 29 |

Escala más o menos lineal con la cantidad de conexiones, que es lo que se
espera: el trabajo por tick es serializar un snapshot por jugador y escribirlo.

RAM y CPU **no son el cuello**, pero tampoco son gratis como decía una medición
anterior de esta misma tabla, que daba 2,6% con 100 bots. Ese número no
sobrevive el escrutinio: contradice al de 40 bots de la fila de arriba, y es
implausible contra un payload seis veces más grande. Quedó corregido acá y en
RESUMEN-EJECUTIVO.

Ojo con un detalle de las dos filas: se tomaron en un desktop, donde un core es
bastante más rápido que el `shared-cpu-1x` de Fly, que tiene cuota base y
burstea por encima.

**En Fly esto sigue sin medirse, y no por falta de ganas.** La latencia y el
ancho de banda con 101 jugadores ya están arriba, pero RAM y CPU del servidor
allá no se pueden sacar desde afuera: la imagen es `distroless`, no tiene shell,
así que `fly ssh console` no entra, y el token de métricas del CLI viene
fallando (`Metrics token unavailable` en cada comando). El camino que no depende
de ninguna de las dos cosas es que lo reporte el propio servidor — un endpoint
`/debug/stats` con `runtime.MemStats` y `NumGoroutine`, que `probe` levantaría
junto al resto y dejaría el número acá al lado de los otros.

Lo que sí se sabe de la máquina real, después de 101 jugadores durante un minuto:
siguió `started`, con el health check pasando, sin OOM, sin panics, sin reinicio
y sin un solo cliente descartado.

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
están dentro de tu ventana. Con 50 jugadores en 760×760, cada uno ve a poquitos,
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

### El borde de un mapa de AO, y por qué se recortan 12 y no 9

Un mapa de Argentum es 100×100, pero **no se juega en 100×100**. El anillo
exterior está bloqueado: es la zona muerta que el cliente original nunca deja
ver, porque la cámara se clava antes de llegar al borde.

Medir ese anillo da **9 tiles** — 290 de los 317 mapas tienen exactamente nueve.
Y recortar nueve es el error, de una forma que no se ve hasta que caminás.

**Los mapas de AO nunca fueron pensados para ser adyacentes.** El original cruza
entre ellos por teleport, desde un `TileExit` en x=12 hasta x=87 del vecino, así
que los tres tiles entre la pared y esa línea son decorado que nadie pisa. Medido
sobre una muestra de mapas:

| borde recortado | columna del borde libre |
|---|---|
| 9, 10, 11 | **0 de 80 y pico** |
| **12** | **73,6 de 76** |

Con 9 el mundo compuesto sale con **las costuras tapiadas en el 100% de los
tiles**: cien celdas selladas y el 13% del terreno alcanzable. Con 12 las
costuras quedan 41-44% pasables y el mundo es 93-97% conectado.

Por eso el recorte es la línea de traslados, no la pared. Cada mapa aporta
**76×76 tiles** al mundo compuesto — ver la sección de los mundos, más abajo, y
§16 de [DIFICULTADES](DIFICULTADES.md).

### Los mundos compuestos

El juego ya no se juega sobre un mapa de Argentum: se juega sobre **uno de
cuatro mundos** cosidos con pedazos de varios.

| | |
|---|---|
| grilla | 10×10 celdas de 76 tiles = **760×760** |
| núcleo jugable | 8×8 celdas |
| anillo exterior | océano, forzado bloqueado: el borde del mundo |
| caminables | ~310.000 por mundo |
| conectado | 93-97% |
| mapas distintos | 70-73 por mundo, 135 entre los cuatro |

Los cuatro son `map1001`..`map1004` — Selva, Tundra, Yermo y Confín — y el
servidor **sortea uno al arrancar la partida**.

**El layout es un dato, no algo que el conversor derive.** Componer un mundo es
un trabajo de diseño: se mide cuánto encaja el borde de un mapa contra el de
otro, se recuece la grilla para minimizar el desencaje, y el resultado se mira
antes de aceptarlo. Eso vive horneado en `tools/aoconv/worlds/layout.json`, lo
que hace el build reproducible y permite cambiar una celda a mano.

La vara de encaje no se inventó, se calibró contra el mundo original de AO:

| | costura media |
|---|---|
| vecinos reales de Argentum | **0,213** |
| mundos compuestos | 0,28-0,30 |
| mapas puestos al azar | 0,585 |

Dos cosas que hubo que descubrir para que el pool sirviera:

- **118 de los 272 mapas del mundo abierto son océano puro.** Con ellos adentro
  el optimizador junta toda el agua en un mar gigante, porque agua contra agua da
  costura perfecta. El pool real son **111 mapas de tierra**, filtrando por menos
  de 40% de agua, menos de 25% de vacío y al menos 4.000 caminables.
- **`Zona=DUNGEON` del `.dat` no significa "interior".** Marca así 31 de los 32
  mapas de nieve, que son región abierta. El filtro correcto es el grafo de
  traslados, no la metadata.

**El agua no está bloqueada en el `.map`.** El servidor original te frena con un
flag de navegación y un bote, que este juego no tiene; sin cerrarla, se camina
sobre el mar y el anillo de océano deja de ser un borde. La lista de 205 grhs de
agua sale de comparar frecuencias entre los mapas 100% agua y los 0% agua, y
cubre el 95% de los tiles de un mapa de océano.

**El conversor verifica el resultado en vez de confiar en la constante:** hace un
flood fill y **se niega a emitir un mundo con menos del 90% del terreno
alcanzable**. Un mundo mal cosido dibuja bien, spawnea bien, y solo se delata al
caminar — exactamente el bug que este chequeo existe para que no vuelva.

### La zona que se achica

Un círculo de terreno seguro que cierra en **12 etapas**. Cada una espera con el
próximo círculo ya dibujado, y después la pared se mueve **continuamente** hacia
él: quedar afuera es una persecución, no un teletransporte.

| | |
|---|---|
| radio inicial | 429 — llega a las esquinas: **nada arranca afuera** |
| gracia | 60 s quieta, **sin daño** |
| etapas | 12, factor 0,605 por etapa |
| radio final | **21** — unas dos pantallas y media, con lugar para pelear |
| duración | ~13 minutos |

Tres decisiones que costaron una iteración cada una:

- **El círculo se dibuja circunscripto, no inscripto.** El mapa es cuadrado y la
  zona es un círculo, así que solo coinciden en cuatro puntos. Un círculo que toca
  el medio de cada lado deja las cuatro esquinas afuera, y eso es **el 21% de un
  cuadrado**: arrancar con un quinto del mapa pintado de azul se lee como "la
  zona ya está medio cerrada", no como "todavía no empezó".
- **Las etapas se aceleran.** Espera y cierre bajan linealmente hasta el 35% de
  la primera (50 s + 40 s al principio, 18 s + 14 s al final). Al principio el
  círculo es enorme y cruzarlo *es* el trabajo; al final la zona segura entra en
  dos pantallas y la misma ventana serían dos minutos esperando.
- **Arranca con el primer jugador, no con el proceso.** Un servidor que bootea
  una hora antes de que alguien entre habría cerrado el círculo entero para
  cuando el primero elige personaje.
- **La zona apunta a un destino sorteado, no camina a la deriva.** Cada etapa
  puede correr el centro exactamente lo que el radio cedió, y esos permisos
  telescopian: el presupuesto total es el radio inicial menos el final, que
  alcanza para cruzar el mapa. Pero elegir la dirección al azar en cada etapa es
  una caminata aleatoria, y una caminata aleatoria que arranca en el centro *se
  queda cerca del centro* — los pasos se cancelan. Medido sobre 25 partidas:
  terminaba a 112 tiles del centro en promedio y solo 1 de 25 pasaba los 200.
  Sorteando el punto final al empezar y yendo hacia él, la media sube a 260 y
  18 de 25 pasan los 200. El sobrante del permiso se gasta en un bamboleo al
  azar, así que apuntar siempre le gana a vagar y el camino igual no se puede
  extrapolar dos etapas antes.

  El destino se puntúa por **cuánto del arena final es terreno caminable**, no
  por si su centro lo es: el medio de un lago pasa la prueba ingenua y les deja
  un charco a los últimos dos.

El daño sube por etapa: 0 (gracia), 1, 2, 3, 4, 6, 8, 11, 14, 18, 23, 29, 36 por
segundo, cobrado una vez por segundo para que los números se lean. Morir por la
zona es una muerte normal: soltás lo que llevabas y arranca el respawn.

`-zone-speed N` divide todos los tiempos, que es la única forma práctica de ver
las últimas etapas sin jugar hasta ellas.

### El ciclo de la partida

`internal/world/match.go`. Tres fases: `matchIdle` hasta que entra alguien,
`matchRunning`, y `matchOver` cuando queda uno vivo.

Una partida solo se **decide** si sus muertes son definitivas. Eso son dos
condiciones (`decidable()`): que haya habido al menos dos jugadores, y que
`-respawn` esté en 0. La segunda apareció escribiendo los tests: con respawn
puesto, el muerto sigue muerto durante el segundo que tarda en volver, y en ese
segundo `aliveCount()` decía 1 y la partida se daba por terminada. Un servidor
de pruebas terminaba la partida en la primera muerte.

El **puesto se fija al morir**, contando al que muere: último de cinco es 5º.
`eliminate()` corre desde `kill()` *antes* de marcar el flag, que es lo que hace
que la cuenta lo incluya.

`-match-restart N` reinicia sin reiniciar el proceso. Lo que hay que acordarse
al tocarlo es que el reinicio tiene que limpiar **todo lo que sobrevive a un
cuadro**: el piso (se rehace entero), los buffs y la parálisis de quien nunca
murió, y los carteles de chat, que duran cinco segundos y cruzarían de partida.

Dos trampas que costaron un test cada una:

- **Los tiles se liberan todos antes de asignar ninguno.** Reubicar jugador por
  jugador hace que al segundo se le niegue un tile que el primero está por
  dejar.
- **`startZone` se comía su propio `armed`.** El literal de struct no lo
  recopiaba, así que después del primer arranque la zona quedaba "no armada" y
  toda partida posterior al reinicio salía sin zona. `startIfArmed` era de un
  solo uso a pesar del nombre.

Verificado de punta a punta con tres bots y `-zone-speed 60`: ganó uno a los 110
segundos, y cinco segundos más tarde la zona volvió a activarse y arrancó la
siguiente sobre las mismas conexiones.

### Las cuentas

`internal/account`, y son opcionales: `-accounts <archivo>` las prende, sin el
flag el servidor es el de siempre.

**El almacenamiento es un log de solo-append, no una base de datos.** Una línea
JSON por hecho, cada una con su etiqueta: alguien se registró, alguien terminó
una partida. Cargar es reproducirlo; escribir es agregar una línea y bajarla al
disco con `Sync`. No hay camino de reescritura ni compactación porque **nada de
esto se edita jamás**.

Se probó SQLite y se descartó midiendo el costo: son **nueve dependencias
transitivas** en un repo que tiene una a propósito y compila estático para
distroless. Todo está detrás de `Store`, así que cambiar el fondo es reemplazar
un archivo.

**Del hash, tres cosas que no se pueden aflojar:**

- PBKDF2-HMAC-SHA256 con **600.000 iteraciones**, el piso de OWASP.
  `crypto/pbkdf2` está en la biblioteca estándar desde Go 1.24, así que no costó
  ninguna dependencia. argon2id resistiría mejor una GPU y está a un módulo de
  distancia; esta es la moneda que paga este repo.
- **Los parámetros viajan con el hash** (`pbkdf2-sha256$600000$salt$key`), así
  que subir el costo más adelante aplica a las contraseñas nuevas sin invalidar
  una sola de las viejas.
- La comparación es de **tiempo constante**, y una cuenta que no existe igual
  paga el costo de verificar contra un hash falso. Si decir que no fuera más
  rápido para un nombre inexistente, ese tiempo enumera quién juega acá.

Los nombres se comparan **sin distinguir mayúsculas**: ahora son una identidad y
no una etiqueta, así que "Wachin" y "wachin" son la misma cuenta.

**Una línea a medio escribir se descarta al cargar en vez de ser fatal.** Es lo
que deja un corte de luz. Perder la última escritura está bien; perder todas las
cuentas para proteger a la más nueva, no.

**El registro de partidas no bloquea al mundo.** Escribir es un append más un
fsync, un par de milisegundos, y el mundo tiene 50 por tick para todo lo que
hace. Una muerte no es nada; la zona llevándose cuarenta personas en el mismo
segundo serían cuarenta tirones. Va por una cola con fondo (`recordQueue`), y si
se llena se pierde la fila y se dice en el log.

Se archiva **una fila por jugador por partida**: al eliminarse, cuando el puesto
ya es definitivo, y al que queda parado al decidirse. Registrar en los dos lados
le daría dos filas a cada muerto.

#### El handshake

El servidor **habla primero** con un `hello` que dice si pide cuenta. Sin eso el
cliente no tiene forma de saber qué pantalla dibujar, y adivinar mal es un
formulario de login contra un servidor que no sabe qué es una cuenta.

    servidor → hello {accounts, minpass}
    cliente  → login {name, pass, new}      ← se repite mientras falle
    servidor → account {ficha}
    cliente  → join {class, race}           ← el name se ignora si hay cuenta

#### En Fly

Las cuentas viven en un **volumen**, montado en `/data`. Sin él cada deploy se
las lleva: el rootfs se reemplaza entero.

```powershell
& $fly volumes create juegito_data --region gru --size 1
& $fly deploy --remote-only
```

Un volumen ata la máquina a un host, lo cual ya era cierto acá por la regla de
una sola máquina de §6 — el mundo vive en memoria y dos máquinas son dos
partidas distintas.

Para mirar el archivo, `-accounts` acepta cualquier ruta; en local conviene una
fuera del repo.

### Hablar, y las palabras mágicas sobre la cabeza

Un cartel por personaje, como en Argentum: lo que digás **reemplaza** lo que
tenías encima. De ahí sale gratis el truco que usan los jugadores — decir un
espacio para borrarse el cartel.

Lanzar un hechizo grita sus `PalabrasMagicas` a todos los que te ven, ancladas a
tu cabeza (`DecirPalabrasMagicas`, `modHechizos.bas`). **Es el delator del
juego**, y por eso el mensaje lleva la posición: un invisible no está en el
snapshot de nadie, así que sin ella el cartel no se podría dibujar — con ella,
las palabras quedan flotando exactamente sobre su tile. Lanzar estando **oculto**
además te saca el ocultamiento, igual que el original.

Los carteles expiran con la fórmula de AO: `5000 ms + 100 ms por carácter`
(`clsDialogs.cls`), con corte de línea a los 18 caracteres.

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
tools/verify/      parser de referencia del .map, independiente de aoconv
```

---

## 9. Lo que falta

El roadmap vive en [RESUMEN-EJECUTIVO](RESUMEN-EJECUTIVO.md), numerado y
ordenado por impacto, para que haya una sola lista y no dos que se contradigan.

Lo más urgente de ahí, en una línea: el codec binario (el snapshot mide 3,6 KB
con la partida llena, ver §7), el paso de caminata a 100% para sacar el tirón de
la interpolación, y salir al lobby al morir en vez de quedar de fantasma.
