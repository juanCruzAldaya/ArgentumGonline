# Resumen funcional

Qué hace el juego hoy, visto desde el jugador y desde el sistema. Todo lo
listado acá está implementado y andando; lo que falta está al final, separado.

Para correrlo y deployarlo, ver [OPERACION.md](OPERACION.md).

---

## 1. Entrar a la partida

**Registro de cuenta.** Antes de conectar a nada, pantalla completa sobre arte
real de AO (`login_bg.png`): un campo de nickname y dos desplegables, 12 clases
(Guerrero, Cazador, Paladín, Bandido, Asesino, Pirata, Ladrón, Clérigo, Bardo,
Mago, Druida, Trabajador) y 5 razas (Humano, Elfo, Elfo Oscuro, Enano, Gnomo).

**CREAR PERSONAJE arranca deshabilitado** y solo se enciende con las tres cosas
elegidas de verdad. El arte ya trae "[Seleccionar Clase]" horneado adentro de la
canaleta, así que ese texto es un ítem real deshabilitado, no un default: pedir
que elijas es el punto de la pantalla. SALIR cierra el cliente (en la build web
intenta `window.close()`, que es todo lo que un script puede hacer ahí).

El panel se escala solo para llenar la pantalla; los campos caen en los huecos
que el arte dibuja, medidos sobre el PNG. Ver OPERACION.md §3, "Tocar los
gráficos de interfaz".

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
están dentro de tu ventana. Con 50 jugadores en un mundo de 760×760, cada uno ve
a poquitos. Un cliente modificado no puede aprender posiciones que no vio.

Lo que **nunca** viaja al cliente ajeno: tu HP. Los vitals van solo al jugador
dueño de esos números.

**Mapa.** Se juega en uno de cuatro mundos compuestos de 760×760 tiles, sorteado
al crear la partida — ver §10. Están armados con pedazos de 135 mapas reales de
Argentum, así que el terreno, los edificios y los caminos son los del juego
original aunque la geografía sea nueva.

## 4. Combate

### Cuerpo a cuerpo

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

### A distancia: arcos, flechas y cuchillas

El arco se equipa como cualquier arma y después **se usa** —U, o doble clic en
el ítem— y eso es lo que pone la mira; el clic siguiente elige a quién. El gesto
no se eligió: es el del original, donde `UsarInvItem` sobre un arma con
`Proyectil = 1` contesta `WriteWorkRequestTarget(Proyectiles)`, exactamente como
un hechizo pide su objetivo. Ctrl nunca apunta: con el arco en la mano sigue
siendo un golpe, porque el arco también es un palo. Si el arco está en la
mochila sin equipar, usarlo contesta la frase del original —"Antes de usar el
arco deberías equipártelo"— y no arma nada.

Escape o clic derecho cancelan; errar el clic cuesta el intento y hay que volver
a usarlo. Lo que **no** se chequea al armar la mira son las flechas: el original
deja armar y que sea el tiro el que diga "no tenés municiones", y acá igual —
negarse antes contestaría una pregunta que el jugador todavía no hizo, y podría
estar yendo a juntar flechas.

Dos armas toman este camino y no son lo mismo, y `obj.dat` lo dice con dos
campos distintos:

| | Proyectil | Municiones | Qué gasta el tiro |
|---|---|---|---|
| **Arco** | sí | sí | una flecha del carcaj equipado |
| **Cuchillas** | sí | no | la cuchilla misma |

**Las flechas se equipan**, no se usan a mano: es el `MunicionEqpSlot` del
original, así que ponerse una flecha mejor saca la anterior igual que una espada
mejor saca la vieja. El Cazador y los demás arqueros ya nacen con el carcaj
puesto, y el panel de abajo muestra cuántas quedan al lado del daño del arma.

Las fórmulas son las de la rama de proyectiles de `SistemaCombate.bas`:

| Concepto | Cómo se calcula |
|---|---|
| Poder de ataque | Skill de **Proyectiles** (no la de armas) con los mismos escalones de agilidad, por `MODATAQUEPROYECTILES` |
| Daño | Tirada del arco **más la de la flecha**, por `MODDANOPROYECTILES` |
| Alcance | El viewport: se le puede tirar a quien se ve, y a nadie más |
| Cadencia | El mismo cooldown que el golpe, así que alternar arco y espada no ataca al doble |

Con esas dos columnas el Cazador es, por fin, un arquero: es la única clase que
apunta a 1,0 y pega a 1,1, y la única cuyos números con arco superan a los
suyos con arma blanca.

**La flecha la ve pasar todo el que la ve.** El daño es cosa de los dos que se
están peleando, pero el proyectil viaja como mensaje propio a cualquiera que lo
tenga en pantalla — incluso si el que tiró está fuera de la suya, que es
justamente cómo uno se entera de que hay alguien con un arco allá. Es el mismo
razonamiento que las palabras mágicas sobre la cabeza del que lanza.

Y **disparar delata**: el que estaba oculto deja de estarlo, igual que al pegar
y al lanzar.

**Muerte.** A cero de vida no hay revivir: eliminación de la partida. El cuerpo
queda dibujado en el piso y **el inventario se desparrama en los tiles libres
alrededor**. Ese drop al morir del AO hardcore ya *era* mecánica de battle
royale, así que se dejó tal cual.

**Y morir te saca al campamento.** El cuerpo se queda unos segundos —cinco, con
`-death-exit`— y después el jugador sale del mundo y vuelve al lobby, con la
tarjeta del resultado encima y su carrera ya actualizada. Antes se quedaba de
fantasma hasta que la partida terminara: podía caminar y mirar, pero no jugar,
y no tenía a dónde ir. El lobby es a dónde.

La espera no es decoración. Es el momento en que la tarjeta aparece y el que te
mató está parado al lado; sacarte en el mismo cuadro se leería como una
desconexión y no como una eliminación. Y del lado del servidor tiene otra razón:
la partida se decide contando quién queda vivo, así que la última muerte tiene
que poder terminar la partida con su víctima todavía adentro.

Lo que dejaste tirado se queda: el inventario se desparrama antes de que te
vayas, así que salir no es llevarte el botín. Y quién ganó igual te llega —la
tarjeta que recibiste al morir tenía tu puesto, y cuando la partida se define te
llega la misma tarjeta con el ganador escrito, ya desde el campamento.

Un `Join` directo —el bot de carga, y todos los tests— no tiene campamento al
que volver, así que para ellos morir sigue siendo lo que era: un fantasma en el
mapa. `-death-exit 0` hace lo mismo para todos.

**Respawn, para testear.** Hoy el servidor arranca con `-respawn 5`: el muerto
queda de fantasma 5 segundos y vuelve a entrar **en el medio del mapa**, con
vida llena, equipo nuevo y hechizos de nuevo, como si hubiese logueado otra
vez. Lo único que se conserva es el contador de bajas. Es una muleta para
probar peleas sin reiniciar el cliente cada vez, no la regla del género:
`-respawn 0` devuelve la eliminación definitiva, y el resto del código sigue
asumiéndola.

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
- **Buffs y debuffs de atributos** — Celeridad/Fuerza y Torpeza/Debilidad.
  **Se acumulan**: lanzar Celeridad de nuevo suma sobre lo que ya tenías, igual
  que tomarse otra poción, que es como funciona `modHechizos.bas`. Es un solo
  modificador con signo, así que Torpeza sobre alguien buffeado le come el buff
  en vez de reemplazarlo, y refrescarlo reinicia el reloj. El buff está capeado
  al doble del atributo base; el debuff tiene piso en 1. Los deltas viajan
  firmados —y miden lo que movió *ese* lanzamiento, no el total acumulado— así
  que el cliente distingue buff de debuff sin un flag aparte.

Duraciones (recortadas a propósito respecto del original, porque en un MMO la
pelea es un episodio y acá es la partida entera): parálisis 6 s, invisibilidad
12 s, buff 30 s, debuff 20 s. Las del original, ahora verificadas contra
`GAME_TIMER_INTERVAL = 40` en vez de estimadas, son 20 s / 48 s / 28 s, y las
pociones 40 s.

**Gráfico del impacto.** Todo hechizo que pega en alguien dispara el efecto
visual real del original (`CreateFX` de `Fxs.ini`), la animación anclada al
**objetivo**, no un proyectil viajando por la pantalla — así lo dibuja el AO de
verdad: el server manda el id del hechizo, el cliente ya tiene su propia copia
de `spells.json`/`bundle.json` y resuelve ahí el grh y el offset. Las 50
animaciones de efectos vienen empaquetadas en el mismo atlas que cuerpos e
ítems.

La excepción es **Apocalipsis**, que usa un hongo nuclear dibujado aparte en vez
del efecto original. Es puro reemplazo de arte: el hechizo sigue resolviéndose
por el mismo camino (FX 13 de `Fxs.ini` → grh 259), y lo único que cambia son
los píxeles de esos 21 frames dentro del atlas — ver "Reemplazar un gráfico de
AO por otro" en OPERACION §3.

### Los cuatro intervalos

Lanzar y golpear no son dos botones independientes. Argentum los cruza, y esos
cruces son de donde sale la cadencia de su PvP. Los cuatro números salen del
`Server.ini` original:

| | |
|---|---|
| hechizo → hechizo | 1400 ms |
| golpe → golpe | 1500 ms |
| **lanzaste → cuánto esperás para pegar** | **1000 ms** |
| **pegaste → cuánto esperás para lanzar** | **1000 ms** |

Además, **el click gasta el hechizo**. Elegiste el hechizo, apareció la cruz, y
el próximo click la consume le pegues a quien le pegues — o al pasto. Errar
cuesta el casteo y hay que volver a apretar LANZAR. Es lo que hace el original:
`UsingSkill = 0` va incondicional después de cualquier click.

## 7. Ocultarse

**O** intenta ocultarse (Ocultarse). Cooldown de 3 s, que es lo único que lo
frena porque no cuesta maná.

La regla interesante: **moverse te descubre, salvo si sos Ladrón o Bandido** —
esas dos clases mantienen el ocultamiento caminando, igual que en el original.
Atacar o lanzar un hechizo descubre a cualquiera. La invisibilidad *por hechizo*
no se rompe al moverse; son dos mecánicas distintas y se mantuvieron separadas.

Los muertos y los paralizados no pueden ocultarse.

## 8. Meditar

**F6** togglea meditar, la forma de recuperar maná sin poción. No se puede
muerto ni en una clase sin maná (`MaxMana == 0`).

- **2 s de "concentración"** antes de que empiece a regenerar — igual que el
  original, para que no sea una cura gratis a mitad de pelea.
- Después, **6% del maná máximo por segundo** (el `PorcentajeRecuperoMana` de
  `Balance.dat`) hasta llenarse, momento en el que se corta solo. El original
  tira ese 6% con un roll de suerte contra la skill Meditar; acá no hay esa
  skill — nadie sube de nivel — así que la cadencia es fija, mismo criterio que
  ya usaba Ocultarse para su propio roll.
- **Bloquea** atacar y usar/equipar objetos mientras dura.
- **Se corta** si caminás o si te pegan un golpe cuerpo a cuerpo — cualquier
  golpe, no un umbral de daño.
- Mientras meditás sos **25% más fácil de acertar**: sentarte a regenerar
  cuesta evasión, no solo la posibilidad de pelear.
- **Aura visible para todo el mundo**, no solo para quien medita — como todos
  los personajes nacen al nivel tope, siempre es el aura más grande que tiene
  el original (`FXMEDITARXXGRANDE`).

## 9. Objetos e inventario

**496 objetos** de `obj.dat`. El protocolo manda solo el número de item; el
cliente ya tiene la tabla, así que una mochila llena cuesta un puñado de
enteros.

**Kit inicial: el equipo básico de tu clase.** Cada personaje nace vestido,
armado y equipado con lo suyo — arma, armadura, casco y escudo — y lo mejor
sigue estando tirado en el mapa. Las 60 combinaciones clase×raza se precalculan
al arrancar el servidor, no en cada join.

Esto **no** está portado, porque no hay nada que portar: el original le da a un
personaje de nivel 1 una daga y trapos newbie porque todo lo demás se gana en
semanas de juego, y acá ese eje no existe. La regla la dan los propios datos:

| Filtro | Qué saca |
|---|---|
| **Lo vende un mercader** (`ObjN=` de `NPCs.dat`) | `obj.dat` es el catálogo de *todo* lo que el motor conoce, herramientas de GM y trofeos de donante incluidos. La Espada Mata Dragones NatOs pega 1000 fijo y no le prohíbe la clase a nadie, así que cualquier ranking por números se la da a las doce clases. Lo que forja un herrero, lo que dropea un dragón y lo que spawnea un GM quedan afuera del kit — y quedan en el piso, que es donde vale la pena caminar hasta ellos |
| **La clase no lo tiene prohibido** | La lista CP de `ClasePuedeUsarItem`, que ya estaba portada |
| **De lo que queda, lo más específico de la clase** | Cuanto más larga la lista CP de un objeto, para menos clases fue hecho: el Arco de Cazador le prohíbe a diez de doce, el Hacha Larga de Guerra a ocho, y una Espada Corta a nadie. Ordenar por eso *antes* que por los números es lo que le pone un arco al Cazador en vez del hacha que le pegaría más fuerte |

Nada de esto es una tabla por clase. Las 60 combinaciones salen de las listas de
clase de `obj.dat`, así que una actualización de datos se lleva el kit con ella
en vez de dejar un id hardcodeado apuntando a algo que se movió.

Lo que sale de esa regla, para Humano (el corte de armadura cambia con la raza):

| Clase | Arma | Armadura | Casco | Escudo |
|---|---|---|---|---|
| Guerrero | Espada de Plata 13-20 | Armadura de Placas Completa 35-40 | Casco de Hierro Completo 10-20 | Escudo de Hierro 1-3 |
| Cazador | **Arco de Cazador 6-11** + 500 flechas | Armadura de Cazador 10-20 | Capucha de Cazador 6-14 | Escudo de Tortuga 1-1 |
| Paladín | Espada de Plata 13-20 | Armadura de Placas Completa 35-40 | Casco de Hierro Completo 10-20 | Escudo de Hierro 1-3 |
| Bandido | Espada de Plata 13-20 | Armadura de Cazador 10-20 | Casco de Hierro Completo 10-20 | Escudo de Hierro 1-3 |
| Asesino | Daga +4 5-8 | Armadura de las Sombras 30-37 | Casco de Hierro Completo 10-20 | Escudo de Tortuga 1-1 |
| Pirata | Sable 6-17 | Brial del Bosque 10-22 | Sombrero Pirata 2-6 | Escudo de Tortuga 1-1 |
| Ladrón | Cuchillas 7-16 | Armadura de Cazador 10-20 | Sombrero Pirata 2-6 | Escudo de Hierro 1-3 |
| Clérigo | Hacha de Guerra Dos Filos 7-20 | Armadura de Gran Sacerdote 15-22 | Casco de Hierro Completo 10-20 | Escudo de Hierro 1-3 |
| Bardo | Daga +4 5-8 | Armadura de Gran Sacerdote 15-22 | Casco de Hierro 3-8 | Escudo de Tortuga 1-1 |
| Mago | Bastón Nudoso 1-1 | Túnica Egregia de Mago 15-20 | Sombrero Mágico +1 1-2 | — |
| Druida | Hacha de madera Élfica 3-8 | Armadura de Gran Sacerdote 15-22 | Casco de Tigre 10-15 | — |
| Trabajador | Espada Vikinga 6-17 | Armadura de Campeón 25-40 | Casco de Hierro Completo 10-20 | Escudo de Tortuga 1-1 |

Esa tabla no está escrita en el código: la imprime un test
(`go test ./internal/world -run TestKitTable -v`), así que un cambio de balance
se lee en vez de adivinarse.

**Mago y Druida no llevan escudo**, y no es un agujero: los trece escudos del
juego los nombran a los dos en su lista CP. Un slot sin candidato queda vacío.

**La armadura es el cuerpo**, así que el corte importa: Argentum trae casi
todas las armaduras dos veces, una para las razas altas y otra para las bajas
(`RazaEnana`, que cubre Enano y Gnomo), más las de mujer. Ponerse el corte
equivocado no se ve como una armadura mal puesta, se ve como otro personaje —
por eso el kit filtra por raza y hay un test solo para eso.

**El Cazador es el único caso raro.** Le toca arco y **500 flechas**, porque un
arco sin flechas es un palo — y lo decide `Municiones`, no `Proyectil`: las
Cuchillas del Ladrón se tiran y listo, el arco declara los dos campos. Como el
combate a distancia todavía no existe, el arco pega como garrote por sus
propios 6-11 y las flechas esperan en la mochila; para que la clase cuya
identidad *es* el kit no sea la única que no puede pelear, el Cazador lleva
además un arma cuerpo a cuerpo sin equipar, elegida con la misma regla.

**El tier newbie no existe en este juego.** Ni en el kit ni en el piso. No es un
punto de partida cuando todos nacen al cap: es una copia peor de un objeto que
ya está en el mundo, y encima invisible — dos pociones rojas con el mismo ícono
y el mismo nombre, una que cura 30 y otra que cura 10.

Los consumibles **no** son los del original, a propósito. Ahí son 200 rojas y
200 azules porque las pociones son un sumidero de oro en una economía que dura
más que cualquier pelea; acá no hay economía ni próxima sesión, así que
racionarlas solo significa que la partida la define quién se quedó seco y no
quién peleó mejor. Se tratan como munición: **3000 de cada tipo usable** (roja,
azul, amarilla, verde) al nacer, y tiradas por todos lados. La negra queda
afuera — 3000 frascos que te matan es un chiste que termina la partida en vez
de uno que causa gracia.

Las pociones del kit pasan por el mismo filtro que el equipo (la vende una
tienda, no es newbie) y se eligen una por `ePocionType`, no "la primera que
aparezca": Go randomiza el orden de iteración de mapas, así que una elección
que se apoyara en eso cambiaba la mochila entre arranques del servidor.

**Loot en el piso.** Tres pasadas distintas sobre un mismo pool de tiles libres
barajado, así nunca se pisan (Argentum permite un objeto por tile y esa regla
se respeta):

| Pasada | Densidad | En Ullathorpe | Peso |
|---|---|---|---|
| Equipo | 1 cada 30 tiles caminables | 165 objetos | `1/(1+poder)` — una espada común (MaxHit 8) y un martillo de guerra (MaxHit 40) quedan ~5× separados |
| Cofres | 1 cada 2600 tiles caminables | 117 en un mundo compuesto | — |
| Pociones | 1 stack de 25 cada 4 tiles caminables | ~1200 stacks, ~30.000 pociones | plano entre todos los tipos |

**Los cofres son la diferencia entre lo que hay y lo que te sirve.** El loot
suelto es lo que el mapa tenga; un cofre larga **tres piezas que tu clase puede
usar**, filtradas con la misma lista `CP` de obj.dat que decide si podés
equipar algo, más la forma de tu raza para las armaduras. Un mago que cruza
medio mapa hasta un cofre sabe que no le va a tocar una espada que no puede
levantar.

Se abre con la misma tecla que agarrar, parado encima, y lo que sale **cae al
piso, no a la mochila**: son varios segundos agachado juntando tres piezas, a
la vista de cualquiera que venga llegando. Eso es lo que hace que un cofre sea
un lugar y no un botón. El primero que llega se lo lleva, como todo lo demás
del piso.

Por dentro no es nada nuevo: es un objeto más del piso, con el tipo que
Argentum le da a los contenedores (`ObjType 7`) y el `grh 503` de su propio
"Cofre Cerrado", que ya estaba en el atlas.

Las dos pasadas excluyen el tier newbie, que no existe en este juego, y la
poción negra.

Las densidades se cuentan contra **tiles caminables**, no contra el `ancho ×
alto` crudo. La mitad de Ullathorpe es pared, así que la cuenta vieja entregaba
la mitad de la densidad que declaraba. La cantidad de equipo en el mapa no
cambió (165 contra 166); lo que cambió es que ahora el número significa algo en
un mapa que no sea medio campo abierto.

**Acciones, todas con las teclas de AO — y todas configurables.** `F1` abre el
panel de teclas, que es el `frmCustomKeys` del original: dos columnas, una
tecla por acción, sin combos. Fuera del browser también responde al `Ctrl+0`
del original; adentro no, porque ese es el zoom de la página y Chrome no lo
suelta. Lo que se elige se guarda en `user://teclas.cfg` y sobrevive a cerrar
el juego. Las teclas de abajo son los defaults.

| Tecla | Acción | Detalle |
|---|---|---|
| `A` | Agarrar | Del tile en el que estás parado, nunca uno que señalás |
| `U` | Usar | Consume el slot seleccionado: poción, comida, bebida. Sobre algo equipable no hace nada, y lo dice |
| `E` | Equipar | Pone o saca el slot seleccionado. Sobre un consumible no hace nada, y lo dice |
| `T` | Tirar | El slot seleccionado completo, al tile donde estás parado. El original abre un diálogo de cantidad para un stack; acá no hay UI de cantidad parcial en ningún lado, así que `T` tira el stack entero, igual que el menú contextual |
| doble click | Usar | Idéntico a `U`: **solo consume**. Un arma, un escudo o una armadura no responden al doble click |
| click derecho | Menú contextual | Equipar/Quitar o Usar según el tipo, más Tirar. Es el único lugar donde el mouse equipa |
| arrastrar | Reordenar | Drag & drop dentro de la mochila; el servidor decide qué se mueve a dónde |

**Equipar y consumir no comparten gesto**, y esa es una desviación deliberada
del original. Ahí un click en el inventario ramifica por el tipo del objeto:
sobre una espada la equipa, sobre una poción se la toma. Las dos consecuencias
son opuestas — equipar se deshace con otro click, tomar una poción no se
deshace — y en una mochila donde todos los íconos se ven igual bajo el mismo
gesto, errarle de slot cuesta una poción. Así que el gesto fácil (doble click,
`U`) se lo queda lo que se consume, y lo reversible tiene su propia tecla (`E`)
y el menú del botón derecho. El servidor ya sabía distinguirlos: el mensaje
`use` lleva la acción explícita y **rechaza la que no corresponde al slot** en
vez de hacer la otra en silencio.

Equipar es un toggle, y equipar algo del mismo tipo reemplaza lo que había en
ese slot. **Lo equipado no se muda a ninguna parte**: se queda en su slot de
mochila con una `E` chiquita en la esquina. Antes había una fila de equipo
aparte, y esa fila era una segunda casa para el mismo objeto — equipar algo lo
*movía* en pantalla, y mantener las dos vistas de acuerdo sobre quién era dueño
de cada slot era el origen del glitch de reordenamiento. Un objeto, un lugar, y
la clase entera de problema se va con la fila.

**Pociones**, con los tipos de `ePocionType`: agilidad, fuerza, salud, maná,
curar veneno, y la negra. La poción de maná usa la fórmula del original, no los
campos del item. La negra mata al que la toma — es el chiste de AO clásico que
los jugadores tomaban igual.

Cooldown de uso, para que no se puedan encadenar pociones.

## 10. El mundo, y la zona que lo cierra

### En qué mundo se juega

No se juega sobre un mapa de Argentum: se juega sobre **uno de cuatro mundos**
cosidos con pedazos de muchos. El servidor **sortea cuál al crear la partida**.

| mundo | carácter |
|---|---|
| **Selva** | bosque parejo cortado por ríos, con la capital al sudoeste |
| **Tundra** | degrada a nieve en el sur, con una región de lava al noroeste |
| **Yermo** | una cuña de desierto lo parte al medio |
| **Confín** | el más costero, con pueblos repartidos en vez de una capital |

Cada uno es de **760×760 tiles** — un núcleo de 8×8 mapas de Argentum rodeado por
un anillo de océano que es el borde del mundo — con unos **310.000 tiles
caminables**. Cruzarlo de punta a punta a la cadencia de AO lleva algo más de dos
minutos.

El agua no se camina. Argentum tampoco te deja, pero te frena con un bote que
este juego no tiene, así que acá está cerrada directamente.

### La zona

Un círculo azul y eléctrico se cierra sobre el mundo en **12 etapas**, y quedarse
afuera cuesta vida.

Arranca **cubriendo el mapa entero** — llega hasta las esquinas, nada empieza
afuera — y se queda quieto **un minuto sin hacer daño**: es el tiempo de caer,
encontrar algo y ubicarse.

Después, cada etapa dibuja el próximo círculo, espera a que lo veas, y **mueve la
pared hacia él de forma continua**. Nunca salta: quedar afuera es una
persecución que se gana caminando, no un teletransporte.

**Cada partida termina en otro lado del mundo.** Al empezar se sortea el punto
donde va a quedar la arena final —un rincón, un bosque, el borde del agua— y el
círculo va hacia ahí. Antes elegía una dirección al azar en cada etapa, que
suena a lo mismo y no lo es: eso es una caminata aleatoria, los pasos se
cancelan, y terminaba siempre más o menos en el medio del mapa. El punto se
elige mirando cuánto del arena final es terreno caminable, así que no te toca
pelear los últimos dos minutos adentro de un lago.

Las etapas **se aceleran**: la primera espera 50 s y cierra en 40, la última
espera 18 y cierra en 14. Al principio el círculo es enorme y cruzarlo es la
mitad del trabajo; al final entra en dos pantallas.

Y **pegan cada vez más**: 1, 2, 3, 4, 6, 8, 11, 14, 18, 23, 29 y 36 puntos por
segundo. Los primeros anillos empujan; los últimos matan en seis segundos. Morir
por la zona es una muerte normal — soltás lo que llevabas.

Termina en un radio de **21 tiles**, y ahí se queda. Si cerrara del todo, los dos
últimos morirían por el anillo en vez de matarse entre ellos.

Una partida completa dura unos **13 minutos**.

La consola avisa en los dos momentos en que podés estar mirando otra cosa: cuando
empieza a cerrar, y cuando **vos** quedás afuera. El círculo actual y el próximo
se ven en el mapa grande.

## 11. Hablar, y lo que eso te delata

**Enter** abre el renglón de chat, **Enter** de nuevo lo manda, **Escape**
cancela. Lo que decís aparece **sobre tu cabeza**, no en la consola, y lo ven
todos los que te ven.

Hay **un solo cartel por personaje**: lo que digas reemplaza lo anterior. De ahí
sale el truco que usan los jugadores de Argentum — decir un espacio para
borrarte el cartel.

Y ahí está el punto: **lanzar un hechizo grita sus palabras mágicas**, sobre tu
cabeza, para todos los del área. Es el delator del juego.

- Lanzar estando **oculto** te saca el ocultamiento, igual que en el original.
- Estando **invisible** no: seguís invisible, pero las palabras quedan flotando
  exactamente sobre tu tile. Tu cuerpo no está en la pantalla de nadie; tus
  palabras sí.

La única contra es taparlo diciendo cualquier otra cosa — lo cual, por supuesto,
también te delata.

Los carteles se van solos a los 5 segundos más 100 ms por carácter, la misma
fórmula del cliente original.

## 12. La cuenta

Opcional: un servidor arranca con `-accounts <archivo>` o sin él. Sin él es el de
siempre — sos el nombre que escribiste y nada sobrevive al proceso.

Con él, lo primero que ves no es el selector de personaje sino **nombre y
contraseña**, con dos botones: entrar, o crear la cuenta. Son dos botones y no
uno porque registrarse con un nombre ya tomado tiene que fallar *como registro*;
si cayera a un intento de login, equivocarte de nombre te diría "contraseña
incorrecta", que manda a buscar el problema al lado equivocado.

Equivocarte no te echa. El servidor contesta y espera otro intento sobre la
misma conexión: una contraseña mal tipeada es lo más común que va a pasar ahí, y
hacerla costar una reconexión —con el mapa y el bitset de colisión bajando de
nuevo— sería castigar al que se equivocó tipeando.

Cuando entrás, la misma pantalla se convierte en **tu ficha**:

| | |
|---|---|
| Partidas | cuántas terminaste |
| Victorias | cuántas ganaste |
| Bajas | el total de tu carrera |
| Mejor | el puesto más alto que alcanzaste, o un guion si todavía no jugaste |
| Tiempo | cuánto sobreviviste en total |
| Últimas partidas | las seis más recientes: puesto, bajas, duración y en qué mundo |

De ahí, el botón **Jugar** lleva al selector de personaje de siempre.

**El nombre deja de ser algo que vos afirmás.** Con cuenta, el mensaje de entrada
ya no puede renombrarte: el servidor usa el nombre que autenticó. Sin eso el
contador de victorias no valdría nada, porque cualquiera escribe tu nombre y
suma a tu ficha.

**Se archiva una fila por partida y por jugador**: al eliminarte, que es cuando
tu puesto ya es definitivo, y al que queda parado cuando la partida se decide.
Cerrar el cliente sobre tu propio cadáver no te borra la partida.

### Lo que la cuenta todavía no hace

- **No hay forma de cambiar ni recuperar la contraseña.** Si la perdés, perdiste
  la cuenta.
- **No hay tabla de posiciones en pantalla.** El servidor sabe calcularla; nada
  la muestra todavía.
- **No hay matchmaking.** Tener cuenta no cambia a qué partida entrás: seguís
  entrando a la que está corriendo.

## 13. Interfaz

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

El panel lateral es **una sola imagen horneada** (525×962) con los controles
vivos posicionados encima, cayendo en los agujeros que el arte ya dibuja. Cada
offset se midió por componentes conexas sobre ese PNG ya horneado, no sobre un
fuente más grande que después había que escalar — así que no hay factor de
conversión que equivocar, y siguen siendo correctos mientras la imagen y el
tamaño 525×962 concuerden.

Sobre el arte hay:

- **Barras**: salud, maná, energía, hambre y sed, todas alimentadas por el
  servidor. Son cinco porque el arte dibuja cinco canaletas; el relleno entra
  clavado adentro de cada una y la lectura (`SALUD 382/382`) va centrada en esa
  misma altura.
- **La mochila llena el agujero entero**: 30 slots de 57px, centrados en el
  marco de hueso. Antes eran de 53 y quedaban corridos a la izquierda, porque
  la barra de scroll pintada les robaba 26px de ancho sin scrollear nada — en
  la pestaña de inventario no hay nada que scrollear.
- **La lista de hechizos** son diez filas de 342x26 que ocupan todo el ancho
  hasta el riel. Eran quince de 330x18: una fila de 18px es finita para
  acertarle con el mouse en una pelea, y más finita todavía para agarrarla y
  arrastrarla. **El orden lo elegís vos**: se arrastra un hechizo sobre otro
  para reubicarlo, y acercarlo a cualquiera de las dos puntas hace que la
  lista scrollee sola, así se puede mandar uno de la fila 30 a la 1 sin soltar
  el botón.
- **La barra de scroll de hueso** es la del arte y **solo existe en la pestaña
  de hechizos**: el hueso con el anillo se recortó del fondo y es el que
  arrastrás, el riel y las flechas se recortaron a su propio PNG para poder
  esconderse con el libro, y la rueda del mouse lo mueve todo.
- **LANZAR e INFO se aprietan enteros**: la placa entera es el botón, baja dos
  píxeles y se oscurece mientras la tenés apretada, y vuelve al soltarla.
- **Dos pestañas** sobre la franja de ladrillos: Inventario y Hechizos.
  Comparten el área negra grande. Estadísticas se sacó: todos nacen al cap, así
  que los cinco atributos solo se mueven bajo un buff que ya se narra en la
  consola.
- **Nombre y clase** grabados en el plaquete: el nombre en mayúsculas y a 26px,
  la clase y raza debajo en más chico y más apagado, el bloque centrado en la
  cara del plaquete. Un nombre largo baja de cuerpo hasta entrar; nunca se
  corta.
- **Bajas propias** en la placa al lado del cofre pintado.
- **Fuerza y agilidad actuales** en las dos cajas al lado de las pociones que
  pinta el arte — la amarilla es agilidad y la verde es fuerza en Argentum, así
  que cada caja lleva el atributo que su propio frasco sube. Son valores vivos,
  no un conteo de mochila: si un hechizo te dopa o te debilita, ahí se ve.
- **SALIR y la X** en la barra de arriba, las dos la misma cosa: cierran la
  conexión y después el cliente. Cerrar el socket a propósito importa — el
  servidor te saca de la partida en el acto y tu cuerpo y tu inventario caen
  al piso para el que esté cerca, en vez de esperar a que se le muera el
  socket. La placa de la izquierda tenía un arbolito horneado que no
  significaba nada acá; se pintó por encima y ahora lleva la palabra.
- **Zona y coordenadas**, leídas de la propia entidad del jugador.
- **Consola** con todos los eventos narrados en segunda persona ("%s te ha
  quitado %d puntos de vida").

El contador de vivos no está en el panel: vive en la barra sobre el viewport y
en ningún otro lado. Estuvo un rato en el inset de arriba y era un readout de
más — el mismo número dos veces en la misma pantalla se lee como dos números.

Los cuatro slots de abajo del arte, el botón con el árbol y la X de arriba
están decorativos por ahora. No hay barra de nivel/experiencia: todos spawnean
al máximo, así que no tendría qué mostrar.

**Footer de equipo.** Cruzando el borde de abajo del viewport, continuando la
fila inferior del panel hacia la izquierda, hay una barra de 1088×37 con cuatro
pares icono/valor: casco, armadura, escudo y arma. Cada caja muestra el rango
real de `obj.dat` — el arma lo que pega (`5-10`), las otras tres lo que frenan
(`0-1`) — y **la caja queda negra solo si ese slot está vacío**. Un ítem sin
defensa muestra su `0`: la ropa newbie no tiene ni MinDef ni MaxDef en
`obj.dat`, así que dejarla en blanco hacía que llevar ropa se viera igual que
andar desnudo. No viaja nada nuevo por la red: el cliente ya tiene la tabla de
objetos y el servidor ya le dice qué slots están puestos.

En Argentum eso se averigua abriendo el inventario y pasando el mouse por
encima. En una partida de minutos, la pregunta cada vez que levantás algo del
piso es si le gana a lo que tenés puesto, así que el número vive a la vista.
La barra se lleva media fila de tiles del viewport.

## 14. Sonido

Los efectos son los de Argentum, con los números que usa el original: el
**2** para el golpe que no entra, el **10** para el que sí, el **37** cuando un
escudo lo rechaza, el **11** cuando alguien muere, el **46** para tomar algo,
el **23** y el **24** para los dos pasos que se alternan al caminar, y el WAV
que **cada hechizo trae en `Hechizos.dat`** — catorce sonidos distintos para
cincuenta hechizos, que es el reparto que hizo el original.

**Ningún sonido viaja por la red.** El original manda un paquete `PlayWave`
porque allá el servidor es el que decide que algo sonó; acá el cliente ya
recibe el evento de combate, el del hechizo y el resultado de usar un objeto,
así que sabe lo que pasó y sabe qué sonar. Un mensaje nuevo habría sido una
segunda forma de contar lo mismo.

**La música se baja aparte, y esa es la decisión que importa.** El original
trae 223 WAV y 72 MP3: 227 MB de audio para un juego cuyo cliente web entero
pesa 37. Los efectos que se usan son 22 archivos y 32 segundos, que comprimidos
dentro del juego son **0,3 MB**; la música son dos pistas de 4,8 MB que **no
entran al paquete** y las sirve el servidor por HTTP. El que la quiere la baja
una vez y el navegador se la queda; el que la deja apagada no baja un byte de
música nunca.

Suenan dos: una en el campamento y otra en la partida — el tema de Ullathorpe
y el del campo, que es la pista a la que estaban puestos más mapas del original
(45 de ellos).

**F2** apaga y prende el sonido, **F3** la música, y lo elegido sobrevive a
cerrar el juego. No están en el panel de teclas porque su arte está horneado
con trece casilleros: agregar dos filas es regenerar el PNG y volver a medir,
no editar una tabla. El original les da M y S, que acá ya son el mapa y caminar
al sur.

## 15. Operación

**Correr el servidor:**

```powershell
cd server
go run ./cmd/server
```

`-map-file`, `-items-file` y `-spells-file` apuntan por default a los archivos
de `server/maps/`, así que corriendo desde `server/` arranca el juego completo.
Si alguno de esos archivos falta, el servidor **no arranca**: antes bajaba en
silencio a una arena generada sin objetos ni hechizos, que era exactamente el
síntoma de "no conozco los hechizos y el mapa no renderiza". Esa arena sigue
estando pero hay que pedirla (`-map-file=""`), y `/healthz` contesta qué cargó
(`ok map="Ciudad de Ullathorpe" items=496 spells=50`) en vez de solo decir que
el proceso está vivo. Otros flags: `-addr`, `-tick`, `-map-width`,
`-map-height`, `-seed`, `-debug`, `-web-dir`, `-death-exit` (segundos de
fantasma antes de volver al campamento) y `-music-dir` (de dónde salen las dos
pistas que el cliente baja a pedido).

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

**Probar de a dos.** Fly.io región `gru` (São Paulo) para latencia real — la
imagen incluye el cliente web, así que el otro solo abre el link. La máquina
duerme cuando no hay nadie y despierta en un par de segundos. Medido: 67 ms de
latencia sentida, contra 116 ms de un túnel casero y 25 ms en local; el detalle
está en OPERACION §7.

## 16. Cómo está armado por dentro

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

## La partida: cómo termina y cómo empieza la siguiente

**Gana el último en pie.** El servidor mira cuántos quedan vivos en cada tick y,
cuando queda uno, la partida se decide. Dos condiciones la habilitan:

- **Tiene que haber habido dos.** Una partida que nunca tuvo dos jugadores no
  puede ganarla uno solo — si no, el primero en conectarse a un servidor de
  prueba es el último vivo antes de que el cliente dibuje un cuadro.
- **La muerte tiene que ser definitiva.** Con `-respawn` puesto, morir no
  elimina, así que no hay último en pie que encontrar. El default es 0.

**El puesto se fija al morir, no al final.** Con cinco vivos, el quinto es el
que se acaba de morir. Calcularlo al final necesitaría un historial que nadie
guarda, y dejaría al jugador mirando su propio cadáver sin saber cómo le fue.

**La tarjeta llega dos veces.** Al morir, con la mitad que ya está decidida —
puesto, bajas y tiempo sobrevivido — y otra vez cuando la partida se define, con
el nombre del ganador. Es el mismo mensaje porque contesta la misma pregunta, y
la segunda se dibuja sobre la primera.

El tiempo sobrevivido se cuenta **desde que entraste vos**, no desde que arrancó
la partida: quien se conectó a los nueve minutos no sobrevivió nueve minutos.

La tarjeta no bloquea el mundo de atrás. El ganador sigue parado en una partida
a la que le quedan veinte segundos, y el muerto es un fantasma que puede seguir
mirando. Se cierra con clic o Escape, y se va sola cuando arranca la siguiente.

### La siguiente partida

`-match-restart N` son los segundos entre una partida decidida y la próxima; 0
la deja terminada. En el reinicio, sobre el mismo mundo y las mismas conexiones:

- el piso se **rehace**, no se completa — si no, los restos se irían acumulando
  partida a partida hasta empapelar el mapa
- todos vuelven vivos, con equipo nuevo, repartidos por el mapa como en un
  ingreso, y con bajas y reloj de supervivencia en cero
- se limpia lo que sobrevive a un cuadro: buffs, parálisis y los carteles de
  chat, que duran cinco segundos y si no cruzarían a la partida siguiente
- la zona vuelve a su primer círculo
- cada cliente recibe un **Welcome nuevo** — el mismo mensaje que un ingreso, así
  el cliente se resetea por el camino que ya tenía y no por un segundo camino
  que podría desincronizarse

Lo que **todavía no pasa**: el muerto se queda de fantasma en el mapa. La
intención es que la muerte te saque al lobby, con modo espectador opcional
siguiendo al que te mató; las dos cosas esperan a que exista el lobby.

## Lo que todavía no existe

| Falta | Nota |
|---|---|
| **Matchmaking** | La cola existe (ver §12) pero es una sola y por orden de llegada: no hay nada parecido a emparejar por nivel, por región ni por nada. Plan: una máquina Fly por partida vía Machines API. |
| **Sonido para el vecino** | Los efectos suenan por los eventos que ya le llegan al cliente, y esos son los de quien participa: se oye la pelea propia, no la de al lado. La flecha sí se ve pasar. |
| **Facciones** | Armada/Legión no están. |
| **Hambre y sed que drenen** | Los vitals son estado real del servidor y el HUD los muestra en sus dos barras, pero nada los baja. Es una decisión de diseño pendiente: ¿un battle royale quiere upkeep? |
| **Persistencia del personaje** | A propósito: nadie levelea, no hay build que guardar. Lo que sí persiste desde ahora es la **carrera** — ver §12 — que es otra cosa: una frase sobre el pasado, no un progreso que arrastrás a la partida siguiente. |
| **Modo espectador** | Morir ya te devuelve al campamento (ver §4), pero mirar la partida desde adentro, siguiendo al que te mató, no existe. |
| **Codec binario** | Ya está medido y ya molesta: el snapshot pesa 3,6 KB con la partida llena, 74 KB/s por jugador. Ver OPERACION §7. |
