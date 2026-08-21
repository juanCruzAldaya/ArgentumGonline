# Resumen ejecutivo

**juegito** — Argentum Online (estilo Alkon 0.13) reimaginado como battle royale.

| | |
|---|---|
| **Estado** | Prototipo jugable con el loop del género cerrado de punta a punta: mundo, combate, objetos, **zona que se achica** y **partida que termina y se reinicia sola**. |
| **Arquitectura** | Servidor autoritativo headless en Go + cliente Godot 4 que solo renderiza. |
| **Licencia** | AGPL-3.0 (los assets de Argentum los liberó Pablo Márquez bajo AGPL). |
| **Tamaño** | ~18.000 líneas de Go, ~6.900 de GDScript. |
| **Tests** | 215 tests, todos verdes (`go test ./...`). |
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
   sin que nadie se reconecte. El puesto se fija al morir, no al final. Y morir
   te devuelve al campamento: la muerte descalifica, no te deja mirando tu
   propio cadáver.
8. **Se pelea de lejos.** Arcos, flechas que se gastan y cuchillas arrojadizas,
   con las columnas de proyectiles de `Balance.dat` — que son las que hacen del
   Cazador un arquero y no un guerrero con un palo. La flecha la ve pasar
   cualquiera que la tenga en pantalla, no solo los dos que se pelean.
9. **El juego suena**, con los WAV del original y el que cada hechizo trae en
   `Hechizos.dat`. La música se sirve aparte del cliente web, así que sumar
   sonido costó 0,3 MB de descarga y no 227.

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

1.4. **Salir al campamento al morir.** Hecho. El eliminado se queda de fantasma
unos segundos —la tarjeta recién apareció y el que lo mató está al lado— y
después vuelve al lobby con su carrera al día. Lo que dejó tirado se queda en el
mapa, y cuando la partida se define le llega la misma tarjeta con el ganador
escrito, ya desde el campamento. Falta el modo espectador siguiendo al que te
mató, que no está en el roadmap todavía.

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
4.2. **Sonido.** Hecho. Golpes, escudos, muertes, pociones, pasos y el WAV que
cada hechizo trae en `Hechizos.dat`, con los números del original. La decisión
que lo define es de tamaño: el audio de Argentum pesa 227 MB contra los 37 del
cliente web entero, así que se convierten **22 archivos** (0,3 MB dentro del
juego) y la música —dos pistas, 4,8 MB— **no entra al paquete**: la sirve el
servidor por HTTP y la baja el que la quiere. F2 y F3 los apagan.
4.3. **NPCs.** `NPCs.dat` está entero y sin tocar; no existe una línea de código
de NPC en el servidor. Es un sistema nuevo, no un port. *(grande)*

### 5. Que haya partidas de verdad

5.0. **Cuentas.** Hecho: usuario, correo y contraseña sobre un log de solo-append, con
la carrera de cada uno — partidas, victorias, bajas, mejor puesto, tiempo — y la
pantalla que la muestra antes de jugar. Es la base que 5.1 y 5.3 necesitaban
igual: sin identidad, ni el matchmaking ni una tabla de posiciones significan
nada. Falta cambiar/recuperar contraseña, y la tabla de posiciones existe en el
servidor pero no se muestra. **El correo se guarda en claro** en un archivo que
no se reescribe nunca: es lo primero a revisar el día que esto tenga jugadores
de verdad y no conocidos.
5.1. **Lobby y matchmaking.** Hecho a medias, y la mitad hecha es la que
importaba: hay una cola de verdad. Entrar a la cuenta te deja en el campamento,
no en el mundo; la partida arranca cuando hay suficientes en la cola, con cuenta
regresiva que se cancela si alguien se va; y al terminar todos vuelven ahí sin
reconectarse. Un asiento del lobby **no es un jugador** — no tiene tile, ni
cuerpo, ni vitals — que es lo que evita que la simulación lo cuente por accidente.
Falta el matchmaking propiamente dicho: hoy la cola es una sola y es por orden de
llegada, no por nada parecido a nivel o región.
5.2. **Una máquina Fly por partida** vía Machines API. *(mediano)*
5.3. **Caída inicial**: elegir dónde entrar al mundo, en vez de spawnear al azar.
*(mediano)*

### 6. Contenido y balance de battle royale

6.1. **Combate a distancia.** Hecho. El arco se equipa y después se **usa** —la
misma U o el mismo doble clic que una poción, que es el gesto del original— y
eso pone la mira; el clic elige a quién. La flecha se gasta, se equipa como el arma (el
`MunicionEqpSlot` del original), suma su propia tirada al daño, y la ve pasar
todo el que la tenga en pantalla. Con las columnas de proyectiles de
`Balance.dat` el Cazador es por fin un arquero. Las cuchillas arrojadizas salen
de la misma rama y gastan el arma en vez de una flecha.
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
