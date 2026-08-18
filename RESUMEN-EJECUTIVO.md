# Resumen ejecutivo

**juegito** — Argentum Online (estilo Alkon 0.13) reimaginado como battle royale.

| | |
|---|---|
| **Estado** | Prototipo jugable. Loop completo de red, combate y objetos andando punta a punta. |
| **Arquitectura** | Servidor autoritativo headless en Go + cliente Godot 4 que solo renderiza. |
| **Licencia** | AGPL-3.0 (los assets de Argentum los liberó Pablo Márquez bajo AGPL). |
| **Tamaño** | ~6.100 líneas de Go, ~2.300 de GDScript, 10 commits. |
| **Tests** | 66 tests unitarios en `internal/world`, todos verdes (`go test ./...`). |
| **Deploy** | Docker + Fly.io, región `gru`. La misma imagen sirve el cliente web. |

## Qué se logró

Se pasó de cero a un juego que se puede abrir, jugar y perder. Concretamente:

1. **El loop de red funciona.** Servidor autoritativo a 20 Hz, snapshots por
   viewport, bots headless para simular una partida llena. Un cliente lento no
   puede frenar la simulación.
2. **Se usan los datos reales de Argentum, no imitaciones.** 496 objetos de
   `obj.dat`, 50 hechizos de `Hechizos.dat`, las tablas de balance por clase y
   raza de `Balance.dat`, y la Ciudad de Ullathorpe (100×100 tiles) como mapa de
   juego. Todo convertido por una herramienta propia (`tools/aoconv`) que
   descifra los formatos binarios originales.
3. **El combate y el maná son los de Argentum.** Las fórmulas de
   `SistemaCombate.bas` portadas literalmente: poder de ataque, evasión, bloqueo
   con escudo, absorción de armadura. El maná al cap replica los dos sitios del
   original que lo tocan (`SetAttributesToNewUser` más 44 subidas de nivel), así
   que clase y raza mueven el pozo de verdad: un Mago Gnomo llega a 4282 y un
   Guerrero a 0. No se inventó balance.
4. **Las acciones de inventario del original están completas.** Equipar/usar,
   ocultarse, agarrar del piso, tirar, reordenar la mochila — con las teclas
   que espera un jugador de AO (Ctrl, U, E, O, A).
5. **La interfaz imita el AO clásico.** El panel lateral es arte real de AO
   como imagen horneada, con los controles vivos posicionados encima.

## Las tres decisiones que definieron el proyecto

**Servidor autoritativo con una sola goroutine dueña del estado.** No hay un
solo mutex en todo el repo. Los comandos se encolan y se aplican al tick
siguiente, así el orden es determinístico y los tests son reproducibles. Esto
sacó de encima toda una clase de bugs de concurrencia antes de que existieran.

**WebSocket en vez de UDP.** AO es tile-based: el estado son enteros y el
combate va por click con cooldown. No hace falta client-side prediction ni
rollback, 20 Hz sobra, y TCP es defendible (el AO original lo usó por años).
El rédito inmediato es enorme: **el cliente corre en el browser**, así que
probar el juego con alguien es mandarle un link. Con UDP puro eso era
imposible. El transporte está detrás de una interface, así que migrar más
adelante es agregar una implementación, no reescribir el juego.

**Portar el balance en vez de diseñarlo.** Veinte años de tuning ya existen en
el código VB6 open source. Copiarlo salió más barato y más creíble que inventar
números, y cada constante tiene el archivo original citado al lado.

## Qué falta para que sea un juego terminado

Por orden de impacto:

1. **La zona que se achica.** Es la mecánica que convierte esto en un battle
   royale y todavía no existe. Es la pieza más importante que falta.
2. **Lobby y matchmaking.** Hoy se entra a un servidor que ya está corriendo;
   no hay concepto de "partida" con principio y fin.
3. **Combate a distancia** (arcos, flechas) y facciones.
4. **Codec binario**, cuando el JSON moleste — medido, no por prolijidad.

## Riesgo principal

El AGPL es copyleft de red: **quien hostee esto tiene que ofrecer el código
fuente completo a cualquiera que juegue**. Es una restricción real sobre
cualquier plan de monetización o de cerrar el código, y no es negociable
mientras se usen los assets de Argentum.
