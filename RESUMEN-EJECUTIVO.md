# Resumen ejecutivo

**juegito** — Argentum Online (estilo Alkon 0.13) reimaginado como battle royale.

| | |
|---|---|
| **Estado** | Prototipo jugable con el loop del género cerrado de punta a punta: mundo, combate, objetos, **zona que se achica** y **partida que termina y se reinicia sola**. |
| **Arquitectura** | Servidor autoritativo headless en Go + cliente Godot 4 que solo renderiza. |
| **Licencia** | AGPL-3.0 (los assets de Argentum los liberó Pablo Márquez bajo AGPL). |
| **Tamaño** | ~11.500 líneas de Go, ~4.400 de GDScript. |
| **Tests** | 137 tests en `internal/world`, todos verdes (`go test ./...`). |
| **Carga medida** | 101 jugadores **contra Fly**: 76 ms de punta a punta, 20 Hz clavados, cero descartes. En local, 28% de un core y 31 MB. |
| **Deploy** | Docker + Fly.io, región `gru`. La misma imagen sirve el cliente web. |

## Qué se logró

1. **El loop de red funciona.** Servidor autoritativo a 20 Hz, snapshots por
   viewport, predicción con reconciliación por número de secuencia. Un cliente
   lento no puede frenar la simulación.
2. **Se usan los datos reales de Argentum, no imitaciones.** 496 objetos, 50
   hechizos, las tablas de balance por clase y raza, y 317 mapas — todo
   convertido por una herramienta propia que descifra los formatos binarios
   originales. El conversor está **validado contra un parser independiente**:
   10.000 tiles × 5 campos, cero diferencias.
3. **Hay un mundo, no un mapa.** Cuatro mundos de 760×760 cosidos con pedazos de
   135 mapas de Argentum, elegidos por una vara de coherencia calibrada contra
   el mundo original. El servidor sortea uno por partida.
4. **La zona que se achica existe.** Doce etapas que aceleran y pegan más, sobre
   un círculo que arranca cubriendo el mapa entero y termina en una arena de 21
   tiles de radio. Una partida dura ~13 minutos.
5. **El combate y el maná son los de Argentum.** Fórmulas de `SistemaCombate.bas`
   portadas literalmente, y los cuatro intervalos cruzados que le dan al PvP su
   cadencia. No se inventó balance.
6. **Lanzar te delata.** Las palabras mágicas aparecen sobre la cabeza del que
   lanza, para todos los del área — incluso si es invisible. Es la mecánica que
   convierte el sigilo en una decisión.
7. **La partida termina.** Último en pie gana; cada eliminado ve su puesto, sus
   bajas y cuánto sobrevivió; y la siguiente arranca sola sobre el mismo mundo
   sin que nadie se reconecte. El puesto se fija al morir, no al final.

## Las cuatro decisiones que definieron el proyecto

**Servidor autoritativo con una sola goroutine dueña del estado.** No hay un solo
mutex en todo el repo. Los comandos se encolan y se aplican al tick siguiente,
así el orden es determinístico y los tests son reproducibles.

**WebSocket en vez de UDP.** AO es tile-based: el estado son enteros y el combate
va por click con cooldown. El rédito inmediato es enorme — **el cliente corre en
el browser**, así que probar el juego con alguien es mandarle un link.

**Portar el balance en vez de diseñarlo.** Veinte años de tuning ya existen en el
código VB6 open source. Cada constante cita su archivo de origen.

**Que el build valide lo que produce.** El conversor de mundos hace un flood fill
y se niega a emitir uno con menos del 90% del terreno alcanzable; el
empaquetador de atlas falla si una página se pasa del límite de textura. Las dos
reglas existen porque el fallo correspondiente ya ocurrió y era invisible.

## Roadmap

Ordenado por impacto. Los tiempos son de trabajo, no de calendario.

### 1. Cerrar el ciclo de la partida — hecho

1.1. **Detectar el último vivo y terminar la partida.** Hecho.
1.2. **Pantalla de fin**: puesto, bajas, tiempo sobrevivido, y quién ganó.
Hecho.
1.3. **Reinicio de partida** sin reiniciar el proceso. Hecho: `-match-restart`,
que además reemplaza a `-respawn` como comodidad de testeo, así que el default
de respawn volvió a 0 — la regla del género.

Lo que abre, y que es la próxima decisión de diseño: **hoy el muerto se queda
de fantasma en el mapa**. La intención es que la muerte descalifique y te saque
al lobby, con modo espectador opcional siguiendo al que te mató. El lobby es
5.1 y el espectador no está en el roadmap todavía.

### 2. Sacar los cuellos de botella medidos

Los dos están medidos, no supuestos.

2.1. **`walkSpeedPercent = 100`.** Hecho. Era un número, y el tirón que se
sentía jugando era este: el reloj del mundo avanza en ticks enteros, así que
4,444 no compra pasos de 4,444 — compra pasos de 5, 4, 5, 4 ticks mientras el
cliente interpola todos sobre el promedio de 222 ms. Ahora son 4 ticks exactos,
200 ms, y el cliente interpola los mismos 200. De paso es la cadencia real de
Argentum, 5 tiles por segundo: el recorte estaba comprando un 10% de lentitud
que nadie pidió al precio de un tirón permanente.
2.2. **Codec binario.** El snapshot pesa 3,6 KB con la partida llena — 74 KB/s
por jugador, ~7 MB/s agregados con 100. Debería bajar a ~400 B. El `probe` ya da
el antes y el después. *(mediano)*
2.3. **Comprimir el Welcome.** Lleva el bitset de colisión entero: 96 KB por
conexión. Gzip lo dejaría en unos pocos KB. *(chico)*

### 3. Guardar y documentar lo hecho

3.1. **Commitear.** Hecho: seis commits, uno por tema — los cuatro mundos (con
el atlas en páginas, el export a Tiled y los cinco límites que el tamaño rompió),
la zona, los intervalos cruzados, el chat con las palabras mágicas, el mapa
grande y el click que erra.
3.2. **Docs al día.** Hecho: OPERACION §3, §7 y §8, DIFICULTADES §16-20,
RESUMEN-FUNCIONAL §10 y §11.
3.3. **Publicar en GitHub** y redeployar a Fly con el mundo nuevo. *(chico)*

### 4. Que el mundo se sienta vivo

4.1. **Mapas de Tiled.** El exportador ya manda nuestros mundos con las cinco
capas acordadas; falta el importador para cuando vuelvan con detalle. *(mediano)*
4.2. **Sonido.** El original trae 223 WAV y 72 MP3 que ya tenemos y no usamos. El
juego es mudo. *(mediano)*
4.3. **NPCs.** `NPCs.dat` está entero y sin tocar; no existe una línea de código
de NPC en el servidor. Es un sistema nuevo, no un port. *(grande)*

### 5. Que haya partidas de verdad

5.1. **Lobby y matchmaking.** Hoy se entra a un servidor corriendo. *(grande)*
5.2. **Una máquina Fly por partida** vía Machines API. *(mediano)*
5.3. **Caída inicial**: elegir dónde entrar al mundo, en vez de spawnear al azar.
*(mediano)*

### 6. Contenido y balance de battle royale

6.1. **Combate a distancia**: arcos y flechas. Los datos están en `obj.dat`.
*(mediano)*
6.2. **Revisar la densidad de loot con gente real.** Está en 1.494 piezas de
equipo y 2.976 pilas de pociones por mundo; se ajustó a ojo, no jugando. *(chico)*
6.3. **Decidir qué hacer con hambre y sed.** Son estado real del servidor, el HUD
los muestra, y nada los baja. ¿Un battle royale quiere upkeep? *(chico, pero es
una decisión de diseño)*

## Riesgo principal

El AGPL es copyleft de red: **quien hostee esto tiene que ofrecer el código
fuente completo a cualquiera que juegue**. Es una restricción real sobre
cualquier plan de monetización o de cerrar el código, y no es negociable
mientras se usen los assets de Argentum.
