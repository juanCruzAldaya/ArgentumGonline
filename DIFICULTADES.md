# Dificultades

Lo que costó, por qué costó, y qué quedó aprendido. Ordenado por tipo de
problema, no cronológicamente.

---

## 1. Los formatos binarios de Argentum no están documentados

El proyecto depende de leer los índices originales de AO, y ninguno tiene
especificación pública. Hubo que descifrarlos leyendo los bytes.

**`Graficos.ini`** resultó tener dos formas distintas bajo la misma clave:
`GrhN=frames-...`. Con `frames=1` es `1-hoja-x-y-w-h`, un recorte de una hoja
PNG numerada. Con `frames>1` es una lista de grhs más una velocidad — o sea una
animación que *referencia otros grh*. Una clave, dos gramáticas.

**`Cuerpos.ini` / `Cuerpos.ind`** fue el peor. Los dos archivos coinciden entre
sí (4 Longs + 2 Integers por registro, header ASCII de 265 bytes en el binario),
pero **las etiquetas de dirección no coinciden con el contenido real**. Los
comentarios declaran `arriba, derecha, abajo, izquierda`. El cuerpo 1 son los
grh 4581-4584 con frames contiguos 2531-2552, agrupados 6/6/5/5 — y ese
agrupamiento asimétrico dice que el orden verdadero es `{arriba, abajo}` y
`{izquierda, derecha}`. Si le hubiera creído a los comentarios, todos los
personajes caminarían mirando para el lado equivocado.

**Transparencia por color key.** No hay canal alfa en ningún sprite: el negro
puro es transparente. Hay que convertirlo explícitamente al empaquetar el atlas,
o cada personaje queda con un recuadro negro alrededor.

**`FXgrh` en `Hechizos.dat` no es un grh.** Es el índice 1-50 de `Fxs.ini`, que
recién ahí apunta al grh real (`Animacion`). Se confirmó mirando los valores:
`FXgrh=2`, `FXgrh=15`, `FXgrh=33` — números demasiado chicos para ser grhs de
`Graficos.ini`, que arrancan en los miles. Y el efecto en sí no es un
proyectil: `SendSpellEffects` (`modHechizos.bas`) manda el `CreateFX` con el
`CharIndex` del **objetivo**, nunca del lanzador — el cliente original nunca
anima nada viajando por la pantalla, solo un efecto que aparece anclado a
donde el hechizo pegó.

**Lección:** cuando el formato no está documentado, el contenido manda sobre las
etiquetas. Los agrupamientos y los tamaños de registro son evidencia; los
comentarios de un archivo de 2002 son una hipótesis.

## 2. Reconstruir la interfaz: cuatro enfoques hasta que uno funcionó

Esta fue la parte más larga y la que más veces se rehízo. La secuencia completa:

**Intento 1 — dibujar el look con primitivas de Godot.** `StyleBoxFlat`,
bordes, gradientes. Resultado: *"no se ve igual, nada que ver realmente"*. Un
`StyleBoxFlat` no puede reproducir arte tallado: no tiene bisel, ni textura, ni
ruido. Estaba condenado desde el principio y perdí tiempo antes de aceptarlo.

**Intento 2 — recortar los bordes reales y hacer 9-slice.** Se extrajo un
`panel_border.png` de 56×56 del `VentanaPrincipal.jpg` de AOFrost, con
`texture_margin_*`. Mejor, pero el usuario preguntó lo correcto: *"¿es necesario
hacer los bordes? ¿por qué solo los bordes? ¿no podemos recrear el arte de los
módulos en sí?"*. Tenía razón — el marco era la parte fácil y menos importante.

**Intento 3 — reconstruirlo en HTML/CSS.** Iterar en Godot era lento (cada
cambio de offset requiere reiniciar el juego), así que se pasó a HTML como banco
de pruebas visual. Acá aparecieron los problemas técnicos más finos:

- El feedback *"está poco fino, le falta ese detalle"* llevó a ampliar la
  referencia 3-4×, y ahí se vio que **es pixel art**: contornos negros duros,
  gradientes en bandas (hard stops, no interpolados), chaflanes, remaches
  octogonales. Un gradiente CSS suave nunca se iba a ver como el original.
- **`clip-path` recorta el `box-shadow` externo.** Todos los anillos de contorno
  que había puesto se borraban silenciosamente en cada placa con chaflán. Hubo
  que pasar cada uno a `inset`. No hay error, no hay warning: simplemente no se
  dibuja.
- **`<style>` dentro de un SVG es global al documento.** Las dos pociones
  salieron del mismo color. La solución fue quitar `fill` del `<symbol>` (el
  fill se hereda) y setearlo por instancia en el `<svg>` que lo envuelve.
- Un **dígito devanagari** se colaba en `--mana-lo: #2b3a६8` y rompía el color
  de la barra de maná en silencio. CSS no valida colores: valor inválido,
  propiedad descartada, cero diagnóstico.

**Intento 4 (el que quedó) — imagen horneada + controles encima.** Se recortó el
panel real, se limpió, y se lo dejó como fondo fijo con los controles vivos
posicionados arriba. Veredicto del usuario: *"quedó buenísimo ahora sí"*.

**Lección:** cuando el objetivo es "que se vea igual a esto", el camino corto es
usar el arte, no reproducirlo. Los tres primeros intentos fueron cada vez más
caros por insistir en generar lo que ya existía como imagen.

## 3. Detectar los límites del panel: cuatro tests fallidos

Para recortar el panel del JPG de referencia hacía falta encontrar su rectángulo
automáticamente. Cuatro heurísticas se cayeron antes de que una funcionara:

| Test | Por qué falló |
|---|---|
| Blanco brillante | El fondo del JPG no es blanco |
| Saturación por píxel | **El fringing de croma del JPEG** en los bordes duros crea saturación falsa; detectaba ruido de compresión como si fuera panel |
| Blanco puro | No hay ni un píxel blanco puro en la imagen |
| Saturación de un solo píxel | Mismo problema de fringing, un píxel no es evidencia |

Lo que funcionó: **run-length** — exigir 10 píxeles saturados *consecutivos*. Eso
filtra el fringing (que es de 1-3 píxeles) y encuentra el borde real. Resultado:
`x=55, y=77, w=1426, h=2612`.

Para los agujeros del arte (dónde van los controles) se usó **connected-component
labeling** — flood fill iterativo con stack sobre máscaras de oscuro/saturado/
marrón — en vez de medir coordenadas a ojo. Eso es lo que hace que los 20+
offsets del `main.tscn` sean reproducibles y no un montón de números mágicos.

**Lección adicional:** los JPG **no tienen canal alfa**. Los cuadriculados de
"transparencia" que aparecen en las imágenes de referencia son píxeles opacos
pintados. Perseguir alfa ahí no lleva a ninguna parte — y al final fue irrelevante,
porque el panel es un rectángulo sólido y alcanzaba con recortar.

## 4. Escalado no entero de pixel art

El panel fuente era 1426 px de ancho y el destino 525: factor 2.7×, no entero.
Dejar que Godot lo bajara en runtime con Nearest tira píxeles de forma despareja
y el arte se ve sucio.

Solución: **pre-escalar offline con Lanczos** y hornear el PNG al tamaño final.
El `default_texture_filter=0` (Nearest) del proyecto se mantiene porque los
sprites del juego sí lo necesitan; el panel llega ya en su tamaño nativo y no lo
toca.

También hubo un `blit_rect` que fallaba con *"format != p_src->format"* al partir
el recorte de las barras — RGB8 contra RGBA8. Se abandonó al pivotear a HTML, pero
vale como recordatorio de que Godot no convierte formatos implícitamente.

## 5. GDScript: la inferencia de tipos falla en silencio hasta que no

Varias veces apareció *"Cannot infer the type of X"*. La causa es siempre la
misma: `var x := ...` no puede inferir de una función sin tipo de retorno
declarado. Hay que anotar explícito: `var slot: int = _hud.selected_slot()`,
`var class_name_: String = ...`.

Más molesto: **registrar una clase global (`class_name`) requiere un pase del
editor**. Después de crear `inventory_slot.gd` el juego tiraba *"Could not find
type InventorySlot"* aunque el archivo estuviera ahí. Igual con
*"No loader found for resource: panel_border.png"* tras agregar una textura. La
solución es correr:

```powershell
godot --headless --editor --quit-after 2   # reconstruye el caché de clases
godot --headless --import                  # importa recursos nuevos
```

Esto es una trampa específica de trabajar sin abrir el editor. No hay forma de
adivinarlo desde el mensaje de error.

## 6. Colisión de teclas: `A` es Agarrar y también "izquierda"

WASD parece obvio hasta que se agregan los hotkeys del original. `A` en Argentum
es **Agarrar**, y también era el alias de WASD para caminar al oeste. Las dos
cosas se disparaban juntas.

Se resolvió sacando `A` del movimiento: al oeste se va con `←`, y las otras tres
direcciones conservan su alias de WASD. Es una asimetría fea pero es la que
respeta el original, y está documentada en el código para que nadie la "arregle"
después.

Detalle relacionado: los hotkeys de acción (`O`, `A`, `U`, `E`) van en
`_unhandled_input` como disparo en el flanco de la tecla, no en el polling de
`_process()` que usa el movimiento. Mantener apretada la tecla no debe mandarle
un intento por frame al servidor.

## 7. El glitch del inventario

Equipar algo hacía que la fila de equipo se re-renderizara entera y "glitcheara".

**Causa raíz:** la fila se llenaba de izquierda a derecha *en orden de llegada*.
Cada equipar o desequipar cambiaba ese orden, y todo se corría de lugar.

**Fix:** columna fija por tipo (arma, escudo, armadura, casco, anillo). Un objeto
siempre cae en la misma columna, así que nada se mueve solo.

Sobre eso se construyó el drag & drop nativo de Godot
(`_get_drag_data`/`_can_drop_data`/`_drop_data` + `set_drag_preview`), con la
regla de que solo los slots de mochila son arrastrables — el equipo no. Y el
`swap` es autoritativo del lado del servidor: el cliente pide "de acá a allá", el
servidor decide qué se mueve realmente.

## 8. Arrancar el servidor sin sus tres archivos de datos

El usuario reportó: *"me dice que no conozco los hechizos, y no renderizó bien el
mapa de Ullathorpe"*. No era un bug — había arrancado el servidor sin
`-map-file`, `-items-file` ni `-spells-file`. Sin esos flags el servidor genera
un arena vacía y no tiene tabla de objetos ni de hechizos.

Es un fallo de diseño de CLI más que un bug: los defaults llevan a un estado que
*parece* roto. Volvió a morder una segunda vez, en producción, con el mismo
mecanismo — el servidor levantó sin datos y contestó 200 al health check
mientras lo hacía, así que nada en el deploy dijo que estaba vacío.

**Arreglado.** Tres cambios chicos en `cmd/server/main.go`, cada uno tapando un
agujero distinto del mismo camino:

- **Los defaults apuntan a los archivos que ya existen.** `-map-file`,
  `-items-file` y `-spells-file` valen `maps/map1.json`, `maps/items.json` y
  `maps/spells.json`, así que el comando correcto pasó a ser `go run
  ./cmd/server` a secas desde `server/`. El camino corto, que es el que
  cualquiera va a tipear, es ahora el camino bueno.
- **Faltar un archivo es un error duro, no una degradación silenciosa.** Antes
  el flag vacío significaba "arrancá sin eso"; ahora la arena generada hay que
  pedirla explícitamente (`-map-file=""`), y un archivo que no está corta el
  arranque diciendo cuál falta y cómo resolverlo. Como la ruta default es
  relativa, también se prueba `maps/` al lado del ejecutable antes de fallar:
  un binario buildeado que vive junto a su propia carpeta de datos es tan
  legítimo como correr desde `server/`.
- **`/healthz` dejó de contestar `ok` a secas.** Ahora el cuerpo es `ok
  map="Ciudad de Ullathorpe" items=496 spells=50`, o `degradado: sin items,
  sin hechizos` cuando el mundo está vacío. El código sigue siendo 200 — un
  503 haría que Fly reiniciara en loop a alguien que pidió la arena a
  propósito — pero un `curl` ahora distingue los dos casos que el deploy roto
  no supo distinguir.

**Lección:** el default de un CLI es una decisión de diseño, no un detalle de
implementación. Y un health check que solo responde "el proceso está vivo"
mide lo que es fácil de medir, no lo que hace falta saber.

## 9. Fricción operativa

- **Puerto 8080 ocupado** por un servidor anterior que quedó colgado. Se
  resuelve con `netstat -ano | findstr 8080` y `taskkill /PID <pid> /F`, pero
  pasó varias veces.
- **Verificar visualmente el resultado es difícil desde la terminal.** Un intento
  de screenshot por `FindWindow($null, "Juegito")` devolvió cero porque la
  ventana ya estaba cerrada. La confirmación visual terminó dependiendo del
  usuario, que es lento para iterar.
- **Iterar offsets en Godot es caro.** Cada ajuste requiere reiniciar el cliente.
  Eso fue exactamente lo que motivó el desvío a HTML, y el desvío valió la pena
  aunque el resultado final no sea HTML.

## 10. Tensiones de diseño que no son bugs

**La referencia visual no cubría nuestro juego.** La imagen que se reconstruyó
mostraba solo la vista de hechizos, y hacía falta también inventario y
estadísticas. Se resolvió por remapeo: pestañas superpuestas en la franja de
ladrillos, el área negra grande compartida por los tres contenidos, la caja de
monedas convertida en "VIVOS n" (no hay economía en un BR), los quick-slots de
pociones conectados al conteo real de la mochila. Y la barra de nivel/experiencia
se eliminó: no existe el trough en el arte y **todos spawnean al máximo**, así
que no tenía qué mostrar.

**Las duraciones del original están tuneadas para otro juego.** En un MMO una
pelea es un episodio dentro de una sesión de horas; acá la partida entera dura
minutos. Una parálisis de la duración original sería una sentencia de muerte. Se
recortaron a 6 s / 12 s / 30 s / 20 s, y está anotado en el código que **no hay
nada sagrado en esos números** — son para retunear.

**Un chequeo del original que parece un bug y se respetó igual.** El
`PuedeLanzar` de AO nunca verifica parálisis, así que un jugador paralizado puede
lanzar hechizos. Se dejó tal cual, con el comentario explicando por qué, porque
copiar el balance incluye copiar sus rarezas. En cambio "paralizado no puede
ocultarse" **sí** es una decisión propia y está marcada como tal — la diferencia
entre lo sourceado y lo inventado está documentada caso por caso.

## 11. El movimiento con input rápido: tres bugs apilados, no uno

El reporte fue "cuando aprieto rápido las teclas se confunde y mezcla cosas, o
lo tira para atrás de manera extraña", con la sospecha puesta en concurrencia
del lado del servidor — background de sistemas/redes, semáforos, esas cosas. No
era eso: el servidor ya es una sola goroutine sin mutex (`### Una sola
goroutine, cero mutexes` en OPERACION.md), y `TestSpammingTurnsDoesNotOutrunTheCadence`
prueba justamente que 60 comandos en un solo tick compran un único paso. El
problema entero estaba en la predicción del cliente, y resultó ser cuatro bugs
distintos, cada uno tapando al siguiente hasta que el anterior se arreglaba.

**Bug 1 — reconciliación por voto de posiciones absolutas.** La versión
original comparaba la posición predicha contra cada snapshot entrante y forzaba
la del servidor tras 4 desacuerdos seguidos (`DESYNC_SNAPSHOTS`). Bajo input
sostenido la posición predicha está *siempre* uno o dos pasos por delante de la
última confirmada — no es una discrepancia, es cómo funciona la predicción —
así que el contador se disparaba con el jugador caminando normal y forzaba un
salto hacia atrás cada ~200 ms. Arreglo: el protocolo ganó un número de
secuencia por `Move` (`protocol.Move.Seq`) que el servidor devuelve como
`Snapshot.AckSeq`; el cliente guarda un buffer de inputs no confirmados y, en
cada snapshot, descarta lo ya confirmado y vuelve a aplicar (replay) lo que
sigue en tránsito sobre la posición fresca del servidor. Es el patrón estándar
de client-side prediction con reconciliación por secuencia, no por voto.

**Bug 2 — el cooldown del cliente reseteaba en vez de acumular.**
`_local_step_ready_at = now + intervalo` (reset) en vez de `+= intervalo`
(acumular, que es lo que el servidor sí hace con `moveReadyAt`). Cada frame
tiene jitter de detección — nunca cae exactamente en el instante ideal — y
resetear desde ese instante tardío en cada paso hace que el retraso se sume
paso a paso. Sostener una tecla unos segundos alcanzaba para que el cliente
quedara desfasado del servidor, y algunos pasos le llegaban todavía en
cooldown y eran rechazados (solo giro, no movimiento) — el "frena-sigue" bajo
tecla sostenida.

**Bug 3 — el replay no distinguía un giro de un paso.** Cuando un cambio de
dirección llega a mitad de cadencia, el cliente lo encola como "solo giro"
(igual que `World.turn` del lado servidor, que nunca mueve). Pero el replay
del Bug 1 trataba *toda* entrada del buffer igual, llamando a la misma función
de resolución de movimiento — así que ese giro se reproducía como si hubiera
avanzado un tile. Cuando el ack real llegaba (confirmando que el servidor solo
giró), el cliente corregía de golpe: el frenón ocurría específicamente en los
instantes posteriores a cambiar de dirección, que fue exactamente la pista que
dio el usuario para encontrarlo ("sucede los primeros microsegundos cuando
cambiás de dirección"). Arreglo: cada entrada del buffer lleva un flag
`can_step`; el replay solo mueve el tile cuando es `true`.

**Bug 4 — reloj continuo contra reloj discreto.** Con los tres bugs de arriba
arreglados, el "frena-sigue" seguía pasando. Instrumentar ambos lados con
logging temporal (`[try_step]`/`[reconcile]` en el cliente, un log de cada
`move` con seq/tick/dir/moved en el servidor) y cruzar los mismos `seq` entre
cliente y servidor de una sesión real del usuario — no de teclado simulado —
mostró rechazos sistemáticos, no ocasionales. La causa: el cooldown real
(222.2 ms) no es múltiplo del tick del servidor (50 ms a 20 Hz), así que el
servidor alterna pasos de 250 ms y 200 ms para promediar 222.2 — está
documentado en el propio comentario de `moveCooldownMilliticks`. El cliente
predecía con un intervalo *continuo* fijo de 222.2 ms, que cae sistemáticamente
antes de que termine el ciclo largo del servidor — no es drift, es
estructural, pasa desde el primer paso. Arreglo: el cliente cuantiza su reloj
a fronteras de tick (la misma aritmética entera que `moveReadyAt`) y calibra la
fase contra el campo `Tick` que ya viaja en cada `Snapshot` — sin eso, tener el
mismo *tamaño* de paso no alcanza si no caen en los mismos *instantes*.

**Lección:** un síntoma de "input rápido rompe todo" en un juego con
predicción de cliente casi nunca es concurrencia — es la reconciliación. Y
cuando arreglar una causa no hace desaparecer el síntoma, no es
necesariamente la señal de que el fix estaba mal: puede haber más de una causa
apilada, cada una tapada por la anterior hasta que se resuelve. La que más
costó encontrar (Bug 4) no salió de leer código con más cuidado, sino de
instrumentar ambos lados y cruzar el mismo número de secuencia — y la pista
que apuntó ahí vino de jugar la sesión real, no de simular teclado.

## 12. El atlas se pasó del límite de textura de la GPU

Al empaquetar las 50 animaciones de `Fxs.ini` en el atlas (para los gráficos de
hechizos y de meditar), el juego entero se rompió visualmente — no solo los
FX nuevos. El reporte del usuario fue literal: *"se rompió todo"*, con una
captura mostrando filas de armaduras repetidas donde debería haber piso y
paredes.

**Causa.** El empaquetador de `tools/aoconv/bundle.go` es un shelf-packer: junta
todos los frames necesarios, los ordena por altura y apila filas de `atlasWidth`
píxeles de ancho. Sumar 50 animaciones de efectos (explosiones, auras) —
sensiblemente más grandes en total que items o cuerpos individuales — empujó la
altura de 12850px a **19168px** con el ancho fijo de 1024 que ya tenía el
proyecto. `GL_MAX_TEXTURE_SIZE` en la gran mayoría de GPUs es 16384. Godot **no
tira error** al importar una textura más grande que eso: la muestrea mal en
silencio, así que cada sprite del juego —no solo los FX recién agregados—
sale con las coordenadas UV corridas.

**Cómo se encontró.** Comparando el header PNG (bytes 16-23, ancho/alto
big-endian) del `atlas.png` viejo (en git) contra el nuevo:

```powershell
git show HEAD:client/assets/ao/atlas.png > old.png
xxd -s 16 -l 8 old.png              # ancho/alto viejo: 1024 x 12850
xxd -s 16 -l 8 client/assets/ao/atlas.png   # ancho/alto nuevo: 1024 x 19168
```

19168 > 16384 confirmó la hipótesis sin necesidad de instrumentar Godot ni
adivinar por prueba y error.

**Arreglo.** Subir `atlasWidth` de 1024 a 2048 en `bundle.go`: al doble de
ancho, la misma cantidad de píxeles arma una imagen de 9516px de alto, bien
lejos del límite. Regenerar (`go run -C tools/aoconv .` con los mismos flags de
siempre) y reimportar (`godot --headless --path client --import`).

**Lección:** un fallo que "rompe todo, no solo lo que tocaste" en un pipeline
de assets empaquetados es casi siempre un límite de tamaño silencioso —
textura, buffer, atlas — no un bug de lógica en lo nuevo. Godot no avisa
cuando lo cruza; hay que medir el archivo a mano.

## 13. La pantalla se dibujaba en un rectángulo chico en la esquina

Al reconstruir la pantalla de login/creación de personaje (`character_picker.gd`,
arte real en `login_bg.png` en vez de `StyleBoxFlat`, mismo criterio que §2)
apareció un bug de renderizado: la ventana del cliente dibujaba su contenido en
un rectángulo chico pegado a la esquina superior izquierda, bastante menor al
tamaño real de la ventana, y el resto quedaba de un gris liso que no era ni
`COLOR_BG` (un marrón casi negro) ni un letterbox negro.

La primera sesión lo persiguió como un bug de la ventana nativa de Windows y no
llegó a nada. La segunda lo cerró midiendo en vez de conjeturando, y el
resultado fue que **no tenía nada que ver con la ventana**: era una línea del
propio picker.

### Lo que se descartó, cada cosa con una medición

- **El gris no era de Windows.** Es RGB(77, 77, 77), o sea `Color(0.3, 0.3,
  0.3)`: el *default clear color* de Godot. Ese solo dato reencuadra todo el
  problema — si el relleno es el color con el que el engine limpia el
  framebuffer, entonces Godot **sí** está pintando la ventana entera, y lo que
  no cubre nada es la escena. Un cuentagotas sobre la captura hubiera ahorrado
  la primera sesión completa.
- **No era DPI.** El chequeo original con `System.Drawing.Graphics` no valía:
  en un proceso que no es DPI-aware siempre devuelve 96, esté Windows al 100% o
  al 150%. Rehecho con `SetProcessDPIAware()` + `GetDeviceCaps(LOGPIXELSX)`:
  96 DPI reales, 1920x1080, `Win8DpiScaling=0`. La conclusión era correcta, el
  método no.
- **No era la ventana.** `DisplayServer` reportaba todo bien: `window_get_size`
  1613x962, seguía el maximizar hasta 1920x1009, `screen_get_scale` 1.0, y el
  `final_transform` del stretch se recalculaba correcto (1.048 con offset 115
  maximizado).
- **No era el driver ni OpenGL.** El mismo cliente con `gl_compatibility`, con
  `--rendering-method forward_plus` (Vulkan) y con `--rendering-driver
  opengl3_angle`: cobertura pintada idéntica, 10.4% en los tres.
- **No era el present ni el swapchain.** Capturar la textura del viewport
  *desde adentro del engine* (`get_viewport().get_texture().get_image()`) da la
  misma imagen rota. Si el render interno ya sale mal, la ventana no participa.

### La causa

`character_picker.gd`, dentro de `_ready()`:

```gdscript
set_anchors_preset(Control.PRESET_FULL_RECT)
```

El segundo parámetro de `set_anchors_preset` es `keep_offsets`, y **su default
es `true`**. Cuando `_ready()` corre, el nodo ya está en el árbol y mide 0x0.
Godot entonces preserva ese rect: pone los anchors en pantalla completa y
compensa los offsets para que el tamaño no cambie. Volcado en runtime:

```
@Control@127 [Control] size=(0,0) anchors=0,0,1,1 offsets=0,0,-1613,-962
```

El picker quedaba de 0x0 para siempre. Lo correcto es
`set_anchors_and_offsets_preset()`, que resetea los offsets junto con los
anchors — o pasar `keep_offsets` en `false` explícitamente.

**Los números cierran exactos.** El panel usa `PRESET_CENTER` con offsets de
±855/2 y ±756/2 dentro de un padre de 0x0, así que quedaba centrado en el
origen (0,0) y solo entraba en pantalla el cuadrante positivo: 427.5 x 378. El
bloque pintado medido sobre la captura era de **428 x 378**. Y el `ColorRect` de
fondo también era 0x0, que es por qué no aparecía `COLOR_BG` por ningún lado.

**Por qué los demás controles del mismo archivo sí andaban:** `bg`, `panel` y
`art` llaman a `set_anchors_preset` **antes** del `add_child`, cuando el área
del padre todavía es 0, así que los offsets quedan en 0 y después heredan bien.
La misma llamada es inocua antes de `add_child` y destructiva después. Los usos
en `hud.gd` son todos pre-`add_child`, por eso el HUD nunca mostró el problema.

### Por qué el `git stash` mintió

El paso que más confianza dio en la primera sesión fue stashear los cambios del
día y probar la pantalla vieja tal cual estaba en HEAD: **mismo bug, idéntico**.
De ahí salió la conclusión de que el problema era del entorno.

Era exactamente al revés. HEAD tenía la misma línea, en su línea 33:
`set_anchors_preset(Control.PRESET_FULL_RECT)`. Las dos versiones del picker
compartían el bug porque compartían el idiom. El stash reprodujo la causa, no la
descartó.

**Lección:** volver a HEAD solo exonera al código nuevo si lo que cambió es *el
patrón*, no el archivo. Cuando las dos versiones las escribió la misma persona
con la misma costumbre, "también pasa en HEAD" no significa "no es el código",
significa "es más viejo que hoy" — que es justamente lo que Wachín ya sabía
("nunca vi esta ventana renderizar bien") y se leyó como evidencia a favor del
entorno.

**Y la lección barata:** antes de teorizar sobre drivers, medí el color del
vacío. Un `GetPixel` sobre la zona que "no se pinta" distingue en un solo paso
entre una ventana sin pintar (gris del sistema), un letterbox (negro) y una
escena que no cubre nada (el clear color del engine). Eran tres investigaciones
completamente distintas y el píxel decía cuál.

La receta de diagnóstico completa — capturar la ventana, medir el bloque
pintado, comparar backends, volcar el árbol de `Control` — quedó escrita en
OPERACION.md §3, "Tocar los gráficos de interfaz", porque sirve para cualquier
bug de layout en un panel armado por código.

## 14. Cambiar el arte del panel: dos trampas de tamaño

Reemplazar el panel lateral por un template nuevo (el gótico de hueso y hierro)
es, en teoría, cambiar un PNG y volver a medir. Salieron dos cosas que no eran
obvias.

**Medir sobre el fuente cuesta una conversión que no hace falta.** El panel
viejo se medía sobre un PNG de 1426x2612 y cada offset se multiplicaba por
525/1426 = 0.3682. Anda, pero cada cambio de arte arrastra esa aritmética y
cada rect nace con un redondeo. Ahora el arte se recorta y se hornea a 525x962
—el tamaño real al que se dibuja— *antes* de medir, así que los números de
`main.tscn` ya están en el espacio del renderer. De paso, medirlos dejó de ser
a ojo: los agujeros de este arte son interiores negros rodeados de marco, así
que **un flood fill sobre `luminancia < 28` devuelve los 13 rects con su
bounding box exacto en una sola pasada** — el área de inventario, las cinco
canaletas, las cajitas de pociones, la placa del cofre. El perfilado de
luminancia cruzando el bisel (§3) quedó solo para lo que no es un agujero
negro: placas de botón, pestañas y el plaquete del nombre.

**`Control.size` es un pedido, no una orden.** Las barras de vitals eran
`ProgressBar` con `size = (174, 14)`, y salían de 27px: se comían la canaleta
pintada y tapaban su propio contorno de color. La causa es que `size` se
clampea contra `get_combined_minimum_size()`, y el mínimo de un `ProgressBar`
de stock contempla el "100%" que puede dibujar — poner `show_percentage =
false` **no** baja ese mínimo. Peor: el orden importaba, porque el `size` se
asignaba antes de reemplazar los styleboxes del tema.

El arreglo no fue pelearse con el mínimo sino no tener uno: un `Control` con
`clip_contents = true` del tamaño exacto del hueco, un `ColorRect` adentro
cuyo ancho es el porcentaje, y la lectura (`SALUD 382/382`) como `Label`
centrado en esos mismos 14px. Un relleno que no puede salirse del marco es una
propiedad del árbol, no un número que hay que acertar.

**Lección:** cuando un control aparece más grande de lo que le pediste, no es
el layout: es su mínimo. Y cuando el mínimo pertenece al widget y no a vos, el
camino corto es dejar de usar ese widget.

---

## 15. Arte generado por IA: la hoja miente sobre su propia grilla

Reemplazar el Apocalipsis de Argentum por un hongo nuclear generado con Gemini
parecía "recortar 21 cuadraditos". El recorte fue lo de menos: lo que costó fue
que **una hoja de sprites generada no es una hoja de sprites**. Se parece a
una, y ahí está la trampa.

**La grilla está dibujada, no calculada.** Las divisorias entre celdas existen
como píxeles grises, pero los pasos miden 347, 347, 349, 343, después un salto
de 1154, después 357, 333. No hay período que ajustar. Peor: algunas celdas
traen *dos* frames adentro (los chicos, la chispa entrando) y otras uno solo
(los hongos grandes), y en la fila de abajo los frames **se tocan**, porque el
fuego de la base cubre el piso de lado a lado y no queda hueco por donde
cortar. El corte terminó siendo por huecos negros para una fila y por celda
para la otra, cada una con su criterio.

**Las filas no miden lo mismo.** La banda de arriba tiene 299 px de alto y la
de abajo 346, y el hongo llena su banda en las dos. Recortar ambas con la misma
ventana le mete un salto de tamaño justo en la costura — un crecimiento que es
del layout y no de la animación. La ventana pasó a ser un cuadrado del alto de
*su propia* banda, apoyado en el piso.

**El anclaje no es el centro del sprite.** Cada frame se centra por donde la
explosión toca el piso, no por el centro de su contenido: el sombrero del hongo
se bambolea entre frames y la columna de fuego no. Anclando por el bounding box
entero, la animación tiembla.

### La segunda hoja no se pudo usar

El intento siguiente —reemplazar Descarga Eléctrica— se frenó en algo peor. La
hoja vino sobre el **damero gris de transparencia**, y como es un JPG, ese
damero quedó horneado. No detrás del sprite: **adentro**. El generador dibujó
el efecto como semitransparente sobre el fondo, así que los cuadraditos
atraviesan el arte. Se confirma midiendo los escalones de gris de una línea
horizontal: en el fondo limpio caen cada ~41 px, y *dentro* de la cabeza del
efecto caen en las mismas posiciones de grilla.

Se intentaron tres salidas, en orden de sofisticación:

1. **Color key contra el gris.** Deja el damero impreso en el alfa: los
   cuadrados claros y los oscuros cruzan el umbral en distinto momento.
2. **Estimar el fondo local y desmultiplicar.** El fondo se estima donde se ve
   y se propaga al vecino más cercano, lo que arregla el borde — y no toca el
   interior, que es donde está el problema.
3. **Matting de dos fondos**, que es la solución correcta: dos cuadrados
   vecinos son el mismo arte sobre dos grises distintos, y de ahí sale el alfa
   exacto. Necesita la fase de la grilla, y la grilla **también** está dibujada
   a mano alzada: el período deriva 26 px de una punta a la otra de la hoja. No
   hay grilla que ajustar.

La salida barata no era técnica: volver a pedir la imagen **sobre negro
sólido**, que es la convención que Argentum ya usa y que el pipeline ya sabe
leer. La primera hoja salió a la primera justamente porque venía así.

**Lección:** con arte generado, el formato de entrega se negocia *antes*, no se
recupera después. Un fondo de damero es información destruida —el arte y el
fondo quedaron mezclados en el mismo píxel— y ninguna cantidad de procesamiento
la trae de vuelta. Y una hoja que *parece* tener grilla merece que se le mida
el paso antes de confiar en él: las tres cosas que costaron acá (celdas
irregulares, filas de distinta altura, damero horneado) son todas invisibles a
ojo y las tres rompen el recorte.

---

## Lo que quedó aprendido, en una línea cada uno

1. Si el objetivo es "igual a esta imagen", usá la imagen.
2. En formatos no documentados, el contenido gana a las etiquetas.
3. El JPEG miente en los bordes: cualquier detección por color necesita
   run-length, no píxeles sueltos.
4. Godot sin editor necesita pases explícitos de `--editor` / `--import` para
   clases y recursos nuevos.
5. CSS descarta valores inválidos en silencio; un typo de un carácter es
   invisible.
6. `clip-path` recorta `box-shadow` externo. Usá `inset`.
7. Cualquier posición de UI derivada de una medición debería estar calculada, no
   eyeballeada — si no, no es reproducible.
8. Los defaults de un CLI que llevan a un estado aparentemente roto son un bug de
   diseño.
9. Una sola goroutine dueña del estado eliminó toda una categoría de bugs antes
   de que existiera. Fue la decisión de arquitectura más rentable del proyecto.
10. Comparar posiciones absolutas no alcanza para reconciliar predicción de
    cliente — hace falta secuencia de inputs y replay. Y un reloj de
    predicción continuo contra uno autoritativo por tick se desalinea aunque
    tengan la misma tasa promedio: hay que cuantizar al mismo grano y calibrar
    la fase.
11. Un campo mal nombrado en un formato de veinte años (`FXgrh` no es un grh)
    solo se descubre mirando los valores reales, nunca el nombre.
12. Un fallo que rompe todo el juego de golpe, no solo lo último que se tocó,
    huele a límite de tamaño silencioso (textura, buffer) antes que a bug de
    lógica — y Godot no avisa cuando una textura se pasa del límite de la GPU.
13. Antes de teorizar sobre drivers y ventanas, medí el color del vacío: el gris
    del sistema, el negro de un letterbox y el clear color del engine son tres
    investigaciones distintas, y un `GetPixel` dice cuál.
14. Volver a HEAD solo exonera al código nuevo si lo que cambió es el *patrón*.
    Si las dos versiones comparten el idiom, "también pasa en HEAD" no significa
    "no es el código", significa "es más viejo que hoy".
15. Medí el arte en el tamaño en el que se dibuja, no en el que vino: la
    conversión de escala es una fuente de error que no hace falta tener.
16. `Control.size` se clampea contra el mínimo del widget. Si un control sale
    más grande de lo que pediste, el problema es su mínimo — y si ese mínimo es
    del widget, cambiá de widget.
17. Con arte generado, el formato de entrega se negocia antes. Un fondo de
    damero de transparencia sobre un JPG es información destruida: el arte y el
    fondo quedaron en el mismo píxel y no hay proceso que los separe.
18. Una hoja de sprites generada *parece* tener grilla. Medile el paso antes de
    creerle: celdas irregulares, filas de distinta altura y frames que se tocan
    son invisibles a ojo y las tres rompen el recorte.
