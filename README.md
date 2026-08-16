# juegito

Argentum Online (estilo Alkon 0.13 clásico) reimaginado como battle royale.

Servidor autoritativo headless en Go. Godot es **solo cliente**: renderiza e
manda input, no simula nada.

## Estado

Vertical slice del loop de red. Andando hoy:

- Mundo autoritativo sobre grilla de tiles, tick a 20 Hz
- Movimiento por casillas con cooldown de paso (5 tiles/s, cadencia AO)
- Colisiones contra paredes y contra otros jugadores
- Snapshots por viewport (17x13 tiles, la ventana clásica de AO)
- Cliente Godot que conecta, camina y ve a los demás
- Bots headless para llenar la partida en tests de carga

Todavía **no**: combate, clases, razas, facciones, zona que se achica, loot,
persistencia, matchmaking.

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

Controles: WASD o flechas.

Flags del servidor: `-addr`, `-tick`, `-map-width`, `-map-height`, `-seed`,
`-debug`.

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

**Test real — Fly.io.** Latencia de verdad desde dos conexiones distintas.

```powershell
fly launch --no-deploy   # una sola vez
fly deploy
# los dos apuntan al mismo lado
godot --path client -- --server=wss://juegito.fly.dev/ws --name=quiensea
```

La región es `eze` (Ezeiza). La máquina duerme cuando no hay nadie conectado y
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
  scripts/         net_client.gd, world_view.gd, main.gd
  scenes/main.tscn
Dockerfile         imagen del servidor
fly.toml           deploy de test compartido
```

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
