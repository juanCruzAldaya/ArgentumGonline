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
go run ./cmd/server -map-file maps/map1.json -items-file maps/items.json -spells-file maps/spells.json
```

**Los tres flags no son opcionales.** Sin ellos el servidor arranca igual, en
una arena vacía generada, sin objetos y sin hechizos — y responde 200 al health
check mientras lo hace. Es el síntoma de "no conozco los hechizos y el mapa no
renderiza", y ya nos mordió dos veces, la segunda en producción. Si el arranque
está bien, el log dice las seis líneas:

```
mapa cargado         name="Ciudad de Ullathorpe" size="[100 100]"
loot esparcido       pedido=165  colocado=165
pociones esparcidas  pedido=1199 colocado=1199 unidades=29975
items cargados       count=491
hechizos cargados    count=50
world running        tickRate=20
listening            addr=[::]:8080
```

Si falta alguna de esas líneas, no estás corriendo el juego.

Otros flags: `-addr` (default `:8080`), `-tick`, `-map-width`, `-map-height`,
`-seed`, `-debug`, `-web-dir`.

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
go test ./...              # 97 tests en internal/world
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

La lista de cuerpos ya no se pasa a mano: se deriva de las armaduras de
`obj.dat`, porque equipar una armadura *es* cambiar de cuerpo. Hoy salen 309
cuerpos, 79 armas, 9 escudos y 18 cascos.

Después de regenerar, Godot necesita reimportar:

```powershell
godot --headless --path client --import
```

Y si agregaste un script con `class_name`, además hace falta un pase del editor
para que registre la clase global:

```powershell
godot --headless --path client --editor --quit-after 2
```

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
| `index.pck` | 8.4 MB | **7.5 MB** |
| **Primera carga** | **46 MB** | **~17.7 MB** |

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
corre. La pantalla de selección de personaje muestra el link al repo
(`SOURCE_URL` en `client/scripts/character_picker.gd`); si cambiás de repo, ese
constante hay que actualizarlo.

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
curl.exe -s -o NUL -w "%{http_code}`n" https://juegito.fly.dev/healthz
curl.exe -sI -H "Accept-Encoding: gzip" https://juegito.fly.dev/index.wasm   # debe decir content-encoding: gzip
& $fly logs -a juegito --no-tail | Select-String "mapa cargado|items cargados|hechizos cargados"
```

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

El ancho de banda es lo único que mueve la aguja: ~17.7 MB por visitante nuevo,
o sea ~56 primeras cargas por dólar-céntimo. Una sesión de prueba con cinco
amigos cuesta menos de $0.10. Un mes de testeo casual queda abajo de $1.

**La región `eze` (Ezeiza) está deprecada** y Fly no provisiona ahí. Estamos en
`gru` (São Paulo), lo único que queda en Sudamérica: ~25-40 ms desde Buenos
Aires en vez de un dígito.

---

## 7. Cómo está armado

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

1. **El cliente predice** (`WorldView.predict_step`): chequea el bitset de
   colisiones que recibió en el Welcome, chequea que no haya otra entidad en el
   tile, y si pasa, se mueve ya. Espeja la cadencia del servidor para no
   adelantarse.
2. **El servidor decide** igual, con su propio reloj de pasos.
3. **Se reconcilia** en `set_entities`: la posición del jugador local que manda
   el servidor está normalmente un paso atrás de la predicción, así que **no** se
   escribe encima — eso es de lo que está hecho un rubber band. Solo si
   discrepan **4 snapshots seguidos** (200 ms) gana el servidor.

La cadencia de paso corre en **milésimas de tick**, no en ticks enteros, porque
los enteros no pueden expresar el ajuste: 4 ticks son 5 tiles/s y 5 ticks son 4
tiles/s, o sea que el salto mínimo desde 100% es −20%. A 90% la cifra honesta es
4.444 ticks. El resto sobrevive de paso a paso, y estar quieto no acumula
crédito para un sprint.

**Girar tiene su propio intervalo** (`INT_CHANGE_HEADING`, 300 ms), separado del
caminar. Un paso legal se lleva su giro puesto; el intervalo solo entra cuando
el movimiento se rechaza — pared, parálisis, o media cadencia.

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
      useitem.go   equipar vs consumir, pociones
client/
  scripts/         net_client, world_view, hud, minimap, main,
                   character_picker, ao_data, ao_sprites, inventory_slot
  scenes/main.tscn estructura y posiciones; lo cosmético vive en hud.gd
tools/aoconv/      lee los índices de AO y arma el atlas y los .json
```

---

## 8. Lo que falta

| Falta | Nota |
|---|---|
| **Zona que se achica** | La mecánica que define el género. Es lo que más falta. |
| **Mapa grande** | Mergear varios de los 317 mapas reales en una grilla con zonas coherentes. Es lo próximo. |
| **NPCs / bichos** | Sistema entero nuevo: spawn, IA, combate contra jugadores. Va después del mapa. |
| **Lobby / matchmaking** | Hoy se entra a un servidor corriendo; no hay partida con principio y fin. Plan: una máquina Fly por partida vía Machines API. |
| **Combate a distancia** | Arcos y flechas. Solo hay melee y hechizos. |
| **Codec binario** | Cuando el JSON moleste, medido. |
| **Recortar el atlas** | Hoy empaqueta las 309 armaduras del juego entero; un BR podría spawnear solo un subconjunto. |
