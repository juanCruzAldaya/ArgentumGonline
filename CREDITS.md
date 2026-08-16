# Créditos y atribución

juegito es una obra derivada de **Argentum Online**, y se distribuye bajo la
misma licencia: **GNU Affero General Public License v3** (ver [LICENSE](LICENSE)).

## Qué implica la AGPL acá

La AGPL es copyleft de red. Su sección 13 dice que si dejás que los usuarios
interactúen con el programa **remotamente a través de una red** — que es
exactamente lo que hace un game server — tenés que ofrecerles el código fuente
completo de tu versión.

En concreto, para cualquiera que hostee juegito:

- El código completo (servidor Go y cliente Godot) tiene que estar disponible
  para los jugadores.
- Cualquier modificación queda bajo AGPL también.
- Esto no es opcional ni depende de si cobrás o no.

## Argentum Online

Creado por **Pablo Márquez** (*Gulfas Morgolock*), que lo liberó a la comunidad
bajo AGPL. Sin esa decisión este proyecto no existiría.

### Staff original (parcial)

- **Líder del proyecto, game design, idea original:** Gulfas Morgolock
- **Idea original:** Dark Megrim
- **Guion:** Guido Giunti
- **Coordinación y dirección:** Gulfas Morgolock, Horacio "HARACIN" Garofalo
- **Programación ORE:** Aaron Perkins (Baronsoft)
- **Programación adicional:** Matías "MatuX" Pequeño, Barrin, Otto Perez,
  Alejandro Santos, Gonzalo "CDT" Larralde, Kevin Birmingham

## Argentum Online Libre (ao-libre)

Los assets y los datos de referencia que usa juegito vienen de
[ao-libre/ao-cliente](https://github.com/ao-libre/ao-cliente) y
[ao-libre/ao-server](https://github.com/ao-libre/ao-server).

- **Programación:** Recox, Jopiortiz, Chots, Cucsifae, FrankoH298, Wyr0X, Shak,
  Wolftein, Lorwik, MateoMiccino, Lherkiev
- **Colaboración gráfica:** ReyarB, Aizanoth (interfaz), guidota (logo)
- **Indexación:** Neo, ReyarB
- **Mapas:** ReyarB
- **Testeo:** BelerianD, Neoranger, Neo, Plusin
- **Traducción al inglés:** ReyarB, Jopiortiz, Recox
- **Música (remasterizaciones):** Noninpretio, NicoDeBonis, reduz,
  TheAinurMusic, PeachGod, Alejandro Pastor VGM
- **Migración de CVS a Git y mantenimiento:** Recox, Jopiortiz

Y a la comunidad de GS-Zone, que sostuvo el proyecto durante años.

## Qué toma juegito de ahí

- **Gráficos** (`Graficos/`) y sus **índices** (`INIT/*.ini`): tiles, cuerpos,
  cabezas y animaciones.
- **Datos de balance** como referencia de diseño: fórmulas de combate, tablas de
  stats por clase y raza, hechizos.

El código de juegito es original — servidor Go y cliente Godot escritos de cero,
no un port del VB6. Pero al incorporar assets AGPL, el conjunto es AGPL.
