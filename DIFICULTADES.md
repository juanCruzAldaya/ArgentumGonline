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
*parece* roto. El comando correcto es siempre:

```powershell
go run ./cmd/server -map-file maps/map1.json -items-file maps/items.json -spells-file maps/spells.json
```

Vale considerar que esos flags apunten por default a los archivos que ya están en
`server/maps/`, o que el servidor avise fuerte al arrancar sin ellos.

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
