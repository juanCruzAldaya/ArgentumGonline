# juegito

Argentum Online (estilo Alkon 0.13 clásico) reimaginado como battle royale.

**Licencia: [AGPL-3.0](LICENSE).** Usa los assets de Argentum Online, que Pablo
Márquez liberó bajo AGPL. Eso es copyleft de red: si hosteás esto, tenés que
ofrecerle el código fuente completo a quien juegue. Ver [CREDITS.md](CREDITS.md).

Servidor autoritativo headless en Go. Godot es **solo cliente**: renderiza e
manda input, no simula nada.

**Documentación:** [OPERACION.md](OPERACION.md) es el manual — todos los
comandos para levantar, buildear, publicar y deployar, más cómo funciona el
sistema por dentro. Los otros tres:
[RESUMEN-EJECUTIVO](RESUMEN-EJECUTIVO.md) para decidir,
[RESUMEN-FUNCIONAL](RESUMEN-FUNCIONAL.md) para saber qué hace el juego, y
[DIFICULTADES](DIFICULTADES.md) para lo que costó y por qué.

## Documentos

- [RESUMEN-EJECUTIVO.md](RESUMEN-EJECUTIVO.md) — qué es, qué se logró, qué falta
- [RESUMEN-FUNCIONAL.md](RESUMEN-FUNCIONAL.md) — todo lo que hace el juego hoy, en detalle
- [DIFICULTADES.md](DIFICULTADES.md) — lo que costó y por qué

## Estado

Prototipo jugable, con el loop del battle royale cerrado salvo el final de
partida. Andando hoy:

- **Mundo compuesto**: cuatro mundos de 760×760 tiles, cosidos con pedazos de
  135 mapas reales de Argentum y elegidos por una vara de coherencia calibrada
  contra el mundo original. El servidor sortea uno por partida.
- **Zona que se achica**: doce etapas que aceleran y pegan más, desde un círculo
  que cubre el mapa entero hasta una arena de 21 tiles de radio. ~13 minutos.
- Mundo autoritativo sobre grilla de tiles, tick a 20 Hz, con predicción del
  movimiento y reconciliación por número de secuencia
- Combate cuerpo a cuerpo y 50 hechizos, con las fórmulas y los cuatro
  intervalos del original
- **Lanzar te delata**: las palabras mágicas aparecen sobre la cabeza del que
  lanza, para todos los del área, incluso si es invisible
- Chat con Enter, dibujado sobre el personaje igual que los hechizos
- Objetos, inventario y loot en el piso, con densidad de battle royale
- Mapa grande con **M**, dibujado con el color real del terreno
- Export web: el mismo proceso Go sirve el cliente HTML5 y el protocolo
- Bots headless: **101 jugadores simultáneos con el 2,6% de un core**

Todavía **no**: final de partida (la zona cierra y se queda, no hay "ganaste"),
lobby, NPCs, combate a distancia, facciones, sonido, persistencia. El roadmap
completo y numerado está en [RESUMEN-EJECUTIVO](RESUMEN-EJECUTIVO.md).

## Por qué esta arquitectura

AO es **tile-based**: el estado son enteros, no floats, y el combate va por
click con cooldown. Eso cambia el netcode respecto de un shooter:

- No hace falta la maquinaria de client-side prediction + rollback
- 20 Hz de tick sobra
- **TCP/WebSocket es defendible**, no un compromiso — el AO original usó TCP y
  anduvo años. Además evita el problema de IP dedicada para UDP en Fly.io y el
  NAT traversal entre conexiones hogareñas
- El viewport del cliente *es* el interest management: con 50 jugadores en un
  mapa grande, cada uno ve a poquitos

El transporte está detrás de una interface (`internal/transport`), así que pasar
a UDP más adelante es agregar una implementación de `Conn`, no reescribir el
juego.

### Forma del servidor

Una sola goroutine es dueña de todo el estado del mundo — no hay un mutex en
todo el repo.

```
conexión ──goroutine de lectura──> cmdCh ─┐
                                          ├──> goroutine del mundo (tick 20 Hz)
conexión ──goroutine de lectura──> cmdCh ─┘         │
                                                     └──> snapshot por viewport
                                                          ──> channel de escritura
                                                              por conexión
```

Los comandos se encolan y se aplican al tick siguiente, así el orden es
determinístico. `Send` nunca bloquea: si un cliente no drena, se le descartan
frames, y si acumula 30 seguidos se lo desconecta. Un cliente lento no puede
frenar la simulación.

## Correr

Requiere Go 1.25+ y Godot 4.3+.

```powershell
# servidor
cd server
go run ./cmd/server                 # escucha en :8080

# cliente (otra terminal) — abrir client/ en Godot y correr, o:
godot --path client -- --server=ws://127.0.0.1:8080/ws --name=wachin
```

Controles: WASD o flechas, Ctrl para golpear, **M** para el mapa, **Enter** para
hablar. El resto — agarrar, tirar, equipar, ocultarse, meditar — en
[RESUMEN-FUNCIONAL](RESUMEN-FUNCIONAL.md).

Flags del servidor: `-addr`, `-tick`, `-seed`, `-debug`, `-respawn`,
`-worlds` (de cuáles sortear el mapa), `-world-seed` (fijar cuál),
`-zone` y `-zone-speed` (apagar la zona, o acelerarla para verla cerrar sin
esperar trece minutos).

### Cliente web

Es la mejor forma de que alguien lo pruebe: entra a una URL y listo, sin
instalar nada. Funciona porque el transporte es WebSocket — con UDP puro el
cliente en browser sería imposible.

Necesita Godot 4.3+ **y** los export templates (se bajan aparte, desde el editor:
`Editor > Manage Export Templates`).

```powershell
.\scripts\build-web.ps1                          # o -Godot "C:\ruta\a\Godot.exe"
go run -C server ./cmd/server -web-dir ..\build\web
# abrí http://localhost:8080
```

El cliente web no necesita configuración: deduce el servidor del origen de la
página. Se puede pisar con query params — `?name=wachin`, o `?server=ws://otro:8080/ws`
para apuntar a otro lado.

El server manda los headers COOP/COEP que el export con threads necesita, y
sirve `.wasm` con el MIME correcto.

### Bots

Nunca vas a juntar 50 humanos para probar. Los bots hablan exactamente el mismo
protocolo que un cliente real — no existe el concepto de "bot" del lado del
servidor — así que lo que se rompe con bots se hubiera roto con jugadores.

```powershell
cd server
go run ./cmd/bot -url ws://127.0.0.1:8080/ws -n 40
```

### Tests

```powershell
cd server
go test ./...
```

## Testear de a dos

Dos caminos que se complementan:

**Iteración rápida — Tailscale.** Corrés el servidor en tu máquina, tu compañero
conecta por el tailnet. Cero deploy, cero NAT.

```powershell
# vos
go run ./cmd/server
# tu compañero, con la IP que le da tailscale
godot --path client -- --server=ws://100.x.y.z:8080/ws --name=compañero
```

**Test real — Fly.io.** Latencia de verdad desde dos conexiones distintas. La
imagen incluye el cliente web, así que tu compañero solo abre el link.

```powershell
.\scripts\build-web.ps1   # exportá ANTES de deployar
fly launch --no-deploy    # una sola vez
fly deploy
# y le pasás esto y nada más:
# https://juegito.fly.dev
```

La región es `gru` (São Paulo): `eze` (Ezeiza) está deprecada y Fly ya no
provisiona ahí. Son 42 ms de ida y vuelta desde Buenos Aires, 67 con la espera
de tick — ver OPERACION §7. La máquina duerme cuando no hay nadie conectado y
despierta en un par de segundos con la próxima conexión.

La URL del servidor nunca está hardcodeada: `--server=`, o la variable de
entorno `JUEGITO_SERVER`. Lo mismo `--name=` / `JUEGITO_NAME`.

## Layout

```
server/
  cmd/server/      servidor de juego headless
  cmd/bot/         clientes headless para carga
  internal/
    protocol/      mensajes de wire + codec (JSON hoy, binario después)
    transport/     frames opacos; implementación WebSocket
    world/         simulación autoritativa, tick loop, sesiones
client/
  scripts/         net_client.gd, world_view.gd, hud.gd, minimap.gd, main.gd
  scenes/main.tscn estructura y posiciones; lo cosmético vive en hud.gd
  export_presets.cfg
tools/aoconv/      lee los indices de AO (grh, cuerpos, cabezas) y extrae sprites
tools/verify/      parser de referencia del .map, para validar aoconv
scripts/
  build-web.ps1    export a WebAssembly
Dockerfile         servidor + cliente web en una imagen
fly.toml           deploy de test compartido
```

## Assets de Argentum

`tools/aoconv` lee los índices originales. El formato, ya descifrado:

- **`Graficos.ini`** — `GrhN=frames-...`. Con `frames=1` es `1-hoja-x-y-w-h`, un
  recorte de una hoja PNG numerada. Con `frames>1` es una lista de grhs más una
  velocidad, o sea una animación que referencia otros grh.
- **`Cabezas.ini`** — `Head1..4` por dirección, sprites estáticos de 17x50.
- **`Cuerpos.ini`** / **`Cuerpos.ind`** — coinciden entre sí (4 Longs + 2
  Integers por registro, header ASCII de 265 bytes en el binario). **Pero las
  etiquetas de dirección no coinciden con el contenido real**: el cuerpo 1 son
  los grh 4581-4584 con frames contiguos 2531-2552, agrupados 6/6/5/5. Ese
  agrupamiento dice que el orden es {arriba,abajo} y {izq,der}, no el
  `arriba,derecha,abajo,izq` que declaran los comentarios. El orden exacto se
  fija al renderizar un personaje con cabeza.
- **Transparencia por color key**: el negro puro es transparente, no hay canal
  alfa. Hay que convertirlo.

```powershell
go run -C tools/aoconv . -assets <dir> -body 1 -info -out ./sprites
```

**El formato está verificado, no supuesto.** Se escribió un segundo parser del
`.map` desde cero, solo a partir de estas notas, y se comparó contra la salida de
`aoconv`: **10.000 tiles × 5 campos, cero diferencias**, consumiendo 53651/53651
bytes exactos. Ver [OPERACION](OPERACION.md) §3.

Un detalle que sorprende y define cómo se cosen los mundos: **un mapa de AO es
100×100 pero solo aporta 76×76**. El anillo exterior está bloqueado, y hay que
recortar hasta la línea de traslados (12 tiles), no hasta la pared (9) — con 9
las costuras salen tapiadas al 100%. Ver OPERACION §8.

## Referencia de diseño

El Argentum Online original es open source y sigue mantenido:
[ao-org/argentum-online-server](https://github.com/ao-org/argentum-online-server),
[ao-org/argentum-online-client](https://github.com/ao-org/argentum-online-client),
y el fork [ao-libre](https://github.com/ao-libre). Es todo VB6, así que sirve
como referencia de diseño — fórmulas de combate, tablas de stats por clase y
raza, balance de hechizos, layout de mapas — no como código reutilizable.

Detalle lindo: el drop de items al morir del AO hardcore ya *es* mecánica de
battle royale.

## Próximos pasos

1. Combate: stats, ataque con cooldown, hechizos con cast time, muerte y drop
2. Zona que se achica sobre la grilla
3. Loot en el piso e inventario
4. Clases y razas (tablas del código VB6)
5. Lobby/matchmaking + una máquina Fly por partida vía Machines API
6. Codec binario cuando el JSON moleste, medido — no antes
