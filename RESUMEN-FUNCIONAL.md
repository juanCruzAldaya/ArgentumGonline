# Resumen funcional

Qué hace el juego hoy, visto desde el jugador y desde el sistema. Todo lo
listado acá está implementado y andando; lo que falta está al final, separado.

Para correrlo y deployarlo, ver [OPERACION.md](OPERACION.md).

---

## 1. Entrar a la partida

**Selección de personaje.** Antes de conectar a nada, pantalla completa con dos
listas: 12 clases (Guerrero, Cazador, Paladín, Bandido, Asesino, Pirata, Ladrón,
Clérigo, Bardo, Mago, Druida, Trabajador) y 5 razas (Humano, Elfo, Elfo Oscuro,
Enano, Gnomo). Ambas arrancan con la primera opción seleccionada, así que JUGAR
siempre es clickeable.

No hay creación de personaje más allá de eso: en battle royale nadie farmea un
build, todos entran al máximo. Esa decisión eliminó de un saque la experiencia,
el leveleo y la persistencia.

**Handshake.** El cliente manda `join` con nombre, clase y raza. El servidor
valida los ids contra la cantidad real de clases/razas — un cliente modificado
que manda basura queda clampeado, no rompe nada.

**Welcome.** El servidor responde con todo lo que el cliente necesita para
dibujar: id de entidad, tick rate, número y nombre del mapa, dimensiones, el
**mapa de colisiones completo como bitset base64** (se manda una sola vez porque
no cambia durante la partida), el tamaño del viewport y el punto de spawn.

## 2. Moverse

- Grilla de tiles, cuatro direcciones cardinales, nunca diagonal.
- **WASD o flechas.** (`A` sola no camina al oeste: esa tecla es Agarrar, como
  en el original. Al oeste se va con `←`.)
- Cooldown de paso: 5 tiles/s, la cadencia de AO. Spamear el comando no acelera
  nada, el servidor ignora lo que sobra.
- Colisiona contra paredes del mapa y contra otros jugadores.
- El cliente interpola el movimiento entre tiles, así que se ve fluido aunque el
  estado sea discreto.

## 3. Ver el mundo

**Viewport de 17×13 tiles** — la ventana clásica de AO. Y eso *es* el interest
management: cada snapshot lleva solo las entidades y los objetos del piso que
están dentro de tu ventana. Con 50 jugadores en un mapa de 100×100, cada uno ve
a poquitos. Un cliente modificado no puede aprender posiciones que no vio.

Lo que **nunca** viaja al cliente ajeno: tu HP. Los vitals van solo al jugador
dueño de esos números.

**Mapa.** Se juega en la Ciudad de Ullathorpe, 100×100 tiles, extraída del mapa
1 original. Los sprites son cuerpos y cabezas reales de Argentum, empaquetados
en un atlas.

## 4. Combate cuerpo a cuerpo

**Ctrl** golpea. Sin campo de objetivo en el protocolo: el melee de Argentum
pega en el casillero al que estás mirando, así que un cliente no puede nombrar
como víctima a alguien del otro lado del mapa.

Fórmulas portadas de `SistemaCombate.bas`:

| Concepto | Cómo se calcula |
|---|---|
| Poder de ataque | Skill de arma + agilidad escalonada (entra en skill 31, 61 y 91), por el modificador de clase |
| Evasión | Skill + agilidad, por el modificador de clase (un Mago evade a 0.40, un Pirata a 1.25) |
| Probabilidad de acierto | Ataque vs. evasión, **clampeado entre 10% y 90%** — el matchup más desahuciado igual pega 1 de 10 |
| Bloqueo con escudo | Skill de defensa con escudo, por el modificador de clase |
| Daño | El arma domina; la fuerza arriba de 15 suma una porción del techo del arma |
| Absorción | La armadura resta daño según su clase de defensa |

Cooldown de ataque: 24 ticks = 1.2 s, el intervalo de melee clásico.

**Muerte.** A cero de vida no hay revivir: eliminación de la partida. El cuerpo
queda dibujado en el piso y **el inventario se desparrama en los tiles libres
alrededor**. Ese drop al morir del AO hardcore ya *era* mecánica de battle
royale, así que se dejó tal cual.

## 5. Maná, y quién puede lanzar

El maná es donde clase y raza se cruzan, y sale de dos lugares del original:
`SetAttributesToNewUser` (TCP.bas) para el nivel 1, y el brazo `AumentoMANA`
del switch de subir nivel (`Modulo_UsUaRiOs.bas`) para cada nivel después. Como
acá todos nacen al cap, se replican los dos: el inicial más 44 subidas de
nivel.

La clase elige la fórmula; la raza la alimenta, porque **todos** los términos
de caster son un múltiplo de Inteligencia, y la Inteligencia es uno de los
cinco atributos que mueve `MODRAZA`. Resultado al cap:

| | Humano | Elfo | Elfo Oscuro | Enano | Gnomo |
|---|---|---|---|---|---|
| Mago (`INT*3`, luego `2.8*INT`/nivel) | 3786 | 4056 | 4056 | 3516 | **4282** |
| Clérigo / Bardo / Druida (`50`, luego `2*INT`) | 2690 | 2866 | 2866 | 2514 | 3042 |
| Paladín / Asesino (`50`, luego `INT`) | 1370 | 1458 | 1458 | 1282 | 1546 |
| Bandido (`50`, luego `INT/3*2`) | 930 | 974 | 974 | 886 | 1062 |
| Guerrero / Cazador / Pirata / Ladrón / Trabajador | 0 | 0 | 0 | 0 | 0 |

Dos detalles que no son adorno:

- El original declara `AumentoMANA As Integer`, así que **cada nivel redondea
  antes de sumar**, no se acumula en punto flotante y se redondea al final. Son
  8 puntos de diferencia en un Mago sobre 44 niveles. VB6 redondea a par, que
  es exactamente `math.RoundToEven`.
- **Cinco de las doce clases tienen 0 de maná y nunca pueden pagar un hechizo**,
  aunque el juego les dé la lista completa. Eso es diseño del original, no un
  agujero del port. El gate de `spells.go` responde "No tenés suficiente maná"
  la primera vez que lo intentan.

El cap `STAT_MAXMAN` (9999) se respeta aunque el build más rico llegue a ~4282.

## 6. Hechizos

50 hechizos convertidos de `Hechizos.dat`. Lanzar es de dos pasos: click en el
hechizo de la lista, después click en el objetivo (Escape o click derecho
cancela). Cooldown de 20 ticks.

El servidor revalida todo antes de aplicar: que el hechizo se conozca, que el
objetivo esté dentro del viewport del lanzador, y que haya maná para pagarlo.
Si falla, se le dice **por qué** al jugador en vez de que el maná desaparezca en
silencio.

Efectos implementados, con el switch tal como lo codifica `Hechizos.dat`:

- **Daño y curación** por rango mín/máx.
- **Paralizar / Inmovilizar** — comparten un contador y son mutuamente
  exclusivos, igual que en el original. Paralizado no podés moverte ni pegar
  melee, pero **sí podés lanzar** (el `PuedeLanzar` original nunca chequea
  parálisis; se respetó la rareza). Inmovilizado te clava los pies pero no los
  brazos: melee sí, movimiento no.
- **Remover parálisis** — limpia los dos estados.
- **Invisibilidad** — un jugador invisible simplemente no aparece en el
  `Entities` de nadie más, y no se lo puede tomar como objetivo. Se ve a sí
  mismo.
- **Buffs y debuffs de atributos** — Celeridad/Fuerza y Torpeza/Debilidad. El
  buff está capeado al doble del atributo base; el debuff tiene piso en 1. Los
  deltas viajan firmados, así que el cliente distingue buff de debuff sin un
  flag aparte.

Duraciones (recortadas a propósito respecto del original, porque en un MMO la
pelea es un episodio y acá es la partida entera): parálisis 6 s, invisibilidad
12 s, buff 30 s, debuff 20 s.

## 7. Ocultarse

**O** intenta ocultarse (Ocultarse). Cooldown de 3 s, que es lo único que lo
frena porque no cuesta maná.

La regla interesante: **moverse te descubre, salvo si sos Ladrón o Bandido** —
esas dos clases mantienen el ocultamiento caminando, igual que en el original.
Atacar o lanzar un hechizo descubre a cualquiera. La invisibilidad *por hechizo*
no se rompe al moverse; son dos mecánicas distintas y se mantuvieron separadas.

Los muertos y los paralizados no pueden ocultarse.

## 8. Objetos e inventario

**491 objetos** de `obj.dat`. El protocolo manda solo el número de item; el
cliente ya tiene la tabla, así que una mochila llena cuesta un puñado de
enteros.

**Kit inicial.** El equipo sale del original (`AddItemsToNewUser`): una daga o
el arma con sabor de la clase, y ropa newbie apropiada para la raza.
Deliberadamente pobre — lo interesante es lo que encontrás. Las 60
combinaciones clase×raza se precalculan al arrancar el servidor, no en cada
join, y el kit respeta las prohibiciones de clase.

Los consumibles **no** son los del original, a propósito. Ahí son 200 rojas y
200 azules porque las pociones son un sumidero de oro en una economía que dura
más que cualquier pelea; acá no hay economía ni próxima sesión, así que
racionarlas solo significa que la partida la define quién se quedó seco y no
quién peleó mejor. Se tratan como munición: **3000 de cada tipo usable** (roja,
azul, amarilla, verde) al nacer, y tiradas por todos lados. La negra queda
afuera — 3000 frascos que te matan es un chiste que termina la partida en vez
de uno que causa gracia.

De paso se arregló un bug latente: la segunda poción del kit se elegía como "la
primera no-roja que aparezca", y Go randomiza el orden de iteración de mapas,
así que la mochila cambiaba entre arranques del servidor. Ahora se elige por
`ePocionType`, determinístico.

**Loot en el piso.** Dos pasadas distintas sobre un mismo pool de tiles libres
barajado, así nunca se pisan (Argentum permite un objeto por tile y esa regla
se respeta):

| Pasada | Densidad | En Ullathorpe | Peso |
|---|---|---|---|
| Equipo | 1 cada 30 tiles caminables | 165 objetos | `1/(1+poder)` — una espada común (MaxHit 8) y un martillo de guerra (MaxHit 40) quedan ~5× separados |
| Pociones | 1 stack de 25 cada 4 tiles caminables | ~1200 stacks, ~30.000 pociones | plano entre todos los tipos |

La pasada de equipo excluye lo newbie (ya lo tenés) y la poción negra. La de
pociones no excluye lo newbie: cuando el objetivo es abundancia, que la poción
sea de tier bajo no la hace menos bienvenida.

Las densidades se cuentan contra **tiles caminables**, no contra el `ancho ×
alto` crudo. La mitad de Ullathorpe es pared, así que la cuenta vieja entregaba
la mitad de la densidad que declaraba. La cantidad de equipo en el mapa no
cambió (165 contra 166); lo que cambió es que ahora el número significa algo en
un mapa que no sea medio campo abierto.

**Acciones, todas con las teclas de AO:**

| Tecla | Acción | Detalle |
|---|---|---|
| `A` | Agarrar | Del tile en el que estás parado, nunca uno que señalás |
| `U` / `E` | Usar / Equipar | Misma acción en el protocolo: el **servidor** decide si eso es tomar una poción o ponerse un arma, según el tipo del objeto |
| doble click | Usar / Equipar | Idéntico a `U` |
| click derecho | Menú contextual | Equipar/Quitar o Usar según el tipo, más Tirar |
| arrastrar | Reordenar | Drag & drop dentro de la mochila; el servidor decide qué se mueve a dónde |

Equipar es un toggle, y equipar algo del mismo tipo reemplaza lo que había en
ese slot. Los slots de equipo tienen **columna fija por tipo** (arma, escudo,
armadura, casco, anillo) — antes se llenaban en orden de llegada y la fila se
reordenaba entera en cada equipar/desequipar, que era el glitch visual que había
que matar. Los slots de equipo no son arrastrables; los de mochila sí.

**Pociones**, con los tipos de `ePocionType`: agilidad, fuerza, salud, maná,
curar veneno, y la negra. La poción de maná usa la fórmula del original, no los
campos del item. La negra mata al que la toma — es el chiste de AO clásico que
los jugadores tomaban igual.

Cooldown de uso, para que no se puedan encadenar pociones.

## 9. Interfaz

Layout de Argentum, no un layout genérico de juego: consola arriba, minimapa al
costado, viewport abajo, panel de personaje pegado al borde derecho.

```
0                                838      1088            1613
+----------------------------------+--------+---------------+  0
| consola                          |minimapa| panel         |
+----------------------------------+--------+ (imagen +     |  130
|                                           |  controles    |
|          viewport 1088x832                |  encima)      |
|     17x13 tiles de 32px, escala 2x        |               |
+-------------------------------------------+---------------+  962
```

El panel lateral es **una sola imagen horneada de arte real de AO** (525×962)
con los controles vivos posicionados encima, cayendo en los agujeros que el arte
ya dibuja. Cada offset se midió sobre el PNG fuente (1426×2612) y se escaló por
525/1426 = 0.3682 — no están a ojo, y solo siguen siendo correctos mientras la
imagen y el tamaño 525×962 concuerden.

Sobre el arte hay:

- **Barras**: salud, maná, energía, hambre y sed, todas alimentadas por el
  servidor.
- **Tres pestañas** superpuestas en la franja de ladrillos: Inventario,
  Hechizos, Estadísticas. Comparten el área negra grande.
- **Estadísticas**: fuerza, agilidad, inteligencia, carisma, constitución. Se
  mandan frescas cada tick porque los buffs las mueven.
- **Contador de vivos**, en la caja que el arte original reservaba para monedas
  — no hay economía en un BR.
- **Dos quick-slots de pociones** con el conteo real de rojas y azules de la
  mochila.
- **Zona y coordenadas**, leídas de la propia entidad del jugador.
- **Consola** con todos los eventos narrados en segunda persona ("%s te ha
  quitado %d puntos de vida").

Los cuatro slots de abajo del arte están decorativos por ahora. Se quitó la
barra de nivel/experiencia: todos spawnean al máximo, así que no tenía qué
mostrar.

## 10. Operación

**Correr el servidor:**

```powershell
cd server
go run ./cmd/server -map-file maps/map1.json -items-file maps/items.json -spells-file maps/spells.json
```

Sin esos tres flags arranca en un arena generada, sin objetos y sin hechizos —
que es exactamente el síntoma de "no conozco los hechizos y el mapa no
renderiza". Otros flags: `-addr`, `-tick`, `-map-width`, `-map-height`, `-seed`,
`-debug`, `-web-dir`.

**Bots.** Hablan exactamente el mismo protocolo que un cliente real; del lado
del servidor no existe el concepto de "bot". Lo que se rompe con bots se hubiera
roto con jugadores.

```powershell
go run ./cmd/bot -url ws://127.0.0.1:8080/ws -n 40
```

**Cliente:**

```powershell
godot --path client -- --server=ws://127.0.0.1:8080/ws --name=wachin
```

La URL nunca está hardcodeada: `--server=` o `JUEGITO_SERVER`; el nombre,
`--name=` o `JUEGITO_NAME`.

**Cliente web.** El mismo proceso Go sirve el export HTML5 y el protocolo. El
cliente web deduce el servidor del origen de la página, y se puede pisar con
`?server=` o `?name=`. El server manda los headers COOP/COEP que el export con
threads necesita.

**Probar de a dos.** Tailscale para iteración rápida (cero deploy, cero NAT), o
Fly.io región `eze` para latencia real — la imagen incluye el cliente web, así
que el otro solo abre el link. La máquina duerme cuando no hay nadie y despierta
en un par de segundos.

## 11. Cómo está armado por dentro

Una sola goroutine es dueña de todo el estado del mundo. **No hay un mutex en
todo el repo.**

```
conexión ──goroutine de lectura──> cmdCh ─┐
                                          ├──> goroutine del mundo (20 Hz)
conexión ──goroutine de lectura──> cmdCh ─┘         │
                                                     └──> snapshot por viewport
                                                          ──> channel de escritura
```

Los comandos se encolan y se aplican al tick siguiente: el orden es
determinístico. `Send` nunca bloquea — si un cliente no drena se le descartan
frames, y a los 30 seguidos se lo desconecta. Un cliente lento no puede frenar
la simulación.

**Estructura:**

```
server/
  cmd/server/      servidor de juego headless
  cmd/bot/         clientes headless para carga
  internal/
    protocol/      mensajes de wire + codec (JSON hoy, binario después)
    transport/     frames opacos; implementación WebSocket
    world/         simulación autoritativa, tick loop, sesiones
client/
  scripts/         net_client, world_view, hud, minimap, main, character_picker,
                   ao_data, ao_sprites, inventory_slot
  scenes/main.tscn estructura y posiciones; lo cosmético vive en hud.gd
tools/aoconv/      lee los índices de AO (grh, cuerpos, cabezas, obj.dat,
                   Hechizos.dat, mapas) y extrae sprites y datos
```

**Tests.** 54 tests en `internal/world`, todos verdes. Cubren movimiento y
colisiones, cooldowns, recorte del viewport, que los vitals ajenos no se filtren,
las bandas de probabilidad de acierto, absorción de armadura, muerte y
eliminación, las cinco reglas de ocultarse (incluida la excepción
Ladrón/Bandido), exclusividad de parálisis e inmovilizar, techos y pisos de los
buffs, equipar/desequipar, reordenar la mochila, cada tipo de poción, y la
desconexión del cliente lento.

---

## Lo que todavía no existe

| Falta | Nota |
|---|---|
| **Zona que se achica** | La mecánica que define el género. Es lo que más falta. |
| **Lobby / matchmaking** | Hoy se entra a un servidor corriendo; no hay partida con principio y fin. Plan: una máquina Fly por partida vía Machines API. |
| **Combate a distancia** | Arcos y flechas. Solo hay melee y hechizos. |
| **Facciones** | Armada/Legión no están. |
| **Hambre y sed que drenen** | Los vitals son estado real del servidor y el HUD los muestra, pero nada los baja. Es una decisión de diseño pendiente: ¿un battle royale quiere upkeep? |
| **Persistencia** | A propósito: nadie levelea, nada que guardar. |
| **Codec binario** | Cuando el JSON moleste, medido. |
