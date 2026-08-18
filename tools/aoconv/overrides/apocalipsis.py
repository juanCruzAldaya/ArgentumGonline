"""Recorta el Apocalipsis nuevo de una hoja de sprites y arma anim259.png.

De aca sale el unico override que hay hoy. No corre en el build: aoconv lee el
PNG ya hecho. Esta guardado porque sin el, el recorte no se puede rehacer.

    python apocalipsis.py <hoja.jpg>

El grh 259 es la animacion que Fxs.ini le da al FX 13, que es el que
Hechizos.dat le pone a Apocalipsis. Son 21 frames de 145x145.

Tres cosas de la hoja de origen obligan a todo lo que sigue:

  - Las divisorias entre celdas estan *dibujadas*, no calculadas. Los pasos
    miden 347, 347, 349, 343, despues un salto de 1154, despues 357, 333. No
    hay grilla que valga: los frames se cortan por los huecos negros.

  - Las filas no miden lo mismo. La de arriba tiene 299 px de alto y la de
    abajo 346, y el hongo llena su fila en las dos. Recortar ambas con la
    misma ventana le mete un salto de tamano justo en la costura, un
    crecimiento que es del layout y no de la animacion.

  - Es un JPG. El fondo no es negro puro sino ruido de compresion de 3 a 15,
    asi que el color key exacto que usa el resto del pipeline (negro puro =
    transparente, bundle.go) dejaria un recuadro sucio alrededor de cada
    frame. Aca el alfa sale de un umbral.
"""
import sys
from PIL import Image
import numpy as np

SRC = sys.argv[1] if len(sys.argv) > 1 else 'hoja.jpg'
OUT = 'anim259.png'

CELL  = 145      # el frame que ya tiene el atlas para el grh 259
PITCH = 346.4    # ancho de celda de la hoja de origen
FRAMES = 21      # los slots que hay en el atlas

# La hoja arranca con la chispa entrando, cinco bocanadas de humo y dos
# estallidos bajos: nueve frames en los que casi no pasa nada y que en el juego
# se leen como un titileo antes del golpe, no como parte de el. La animacion
# empieza en la columna de fuego, que es el primer frame que se lee como un
# Apocalipsis. Los 21 slots se reparten sobre lo que queda, asi que el hongo se
# come el tiempo que gastaba la entrada.
SKIP = 9
R2, R3 = (153, 452), (454, 800)   # el piso de cada banda es su borde inferior


def sin_divisorias(im):
    """Borra las lineas de la grilla dibujada sin tocar el arte que las cruza."""
    a = np.asarray(im).astype(np.int16).copy()
    H, W, _ = a.shape
    gray = a.mean(axis=2)
    # Una divisoria es gris neutro. El fuego nunca lo es; el humo claro se
    # salva por el techo de 215 y el nucleo blanco por estar arriba de eso.
    es = ((np.abs(a[:, :, 0]-a[:, :, 1]) < 30) & (np.abs(a[:, :, 1]-a[:, :, 2]) < 30)
          & (gray > 48) & (gray < 215))
    # Y solo cuenta si forma una linea vertical larga, asi el humo gris suelto
    # no se borra por parecerse de color a una divisoria.
    run = np.zeros(es.shape, dtype=np.int32)
    run[0] = es[0]
    for y in range(1, H):
        run[y] = np.where(es[y], run[y-1]+1, 0)
    linea = np.zeros(es.shape, dtype=bool)
    for x in range(W):
        y = H-1
        while y >= 0:
            if run[y, x] >= 40:
                linea[y-run[y, x]+1:y+1, x] = True
                y -= run[y, x]
            else:
                y -= 1
    ys, xs = np.where(linea)
    izq = a[ys, np.clip(xs-3, 0, W-1)]
    der = a[ys, np.clip(xs+3, 0, W-1)]
    a[ys, xs] = (izq+der)//2      # promedio de los costados: lo negro sigue negro
    return np.clip(a, 0, 255).astype(int)


a = sin_divisorias(Image.open(SRC).convert('RGB'))
H, W, _ = a.shape
content = a.max(axis=2) > 34      # el JPG deja ruido de 3-15 sobre el negro

def blobs(y0, y1):
    ch = content[y0:y1].any(axis=0)
    runs, s = [], None
    for x in range(W):
        if ch[x] and s is None: s = x
        elif not ch[x] and s is not None: runs.append([s, x-1]); s = None
    if s is not None: runs.append([s, W-1])
    return [r for r in runs if r[1]-r[0]+1 >= 5]

# Fila 2: un blob por frame. Un blob angosto pegado a uno ancho es metralla
# desprendida del mismo estallido, no el frame siguiente.
raw = blobs(*R2)
r2 = [raw[0][:]]
for s, e in raw[1:]:
    w, pw = e-s+1, r2[-1][1]-r2[-1][0]+1
    if s - r2[-1][1] <= 80 and min(w, pw) < 45: r2[-1][1] = e
    else: r2.append([s, e])

# Fila 3: no hay hueco por donde cortar, el fuego de la base cubre el piso de
# lado a lado. Ahi vale la celda: en esta mitad la hoja mantiene el paso.
r3 = [[int(round(k*PITCH)), int(round((k+1)*PITCH))-1] for k in range(7)]

spans = [(s, e, R2) for s, e in r2] + [(s, e, R3) for s, e in r3]

def ground_center(x0, x1, band):
    """Donde la explosion toca el piso. El sombrero se bambolea entre frames y
    la columna de fuego no, asi que anclar por el contenido entero temblaria."""
    y0, y1 = band
    sub = content[y0:y1, x0:x1+1]
    base = sub[int((y1-y0)*0.85):]
    cols = np.where(base.any(axis=0))[0]
    if len(cols) == 0: cols = np.where(sub.any(axis=0))[0]   # la chispa aun vuela
    return x0 + (cols[0]+cols[-1])/2

cells = []
for i, (s, e, band) in enumerate(spans):
    y0, y1 = band
    # La hoja no reparte sus filas parejo: la banda de arriba mide 299 px de
    # alto y la de abajo 346. El hongo llena su banda en las dos, asi que
    # recortarlas con la misma ventana le hace pegar un salto de tamano justo
    # en la costura: un crecimiento que es del layout, no de la animacion. La
    # ventana es entonces un cuadrado del alto de su propia banda, apoyado en
    # el piso, y un hongo que llena su fila llena el cuadro en las dos.
    side = y1-y0
    cx = ground_center(s, e, band)
    left = int(round(cx - side/2))
    right = left + side

    win = np.zeros((side, side, 3), dtype=np.uint8)
    sx0, sx1 = max(0, left), min(W, right)
    win[:, sx0-left:sx1-left] = a[y0:y1, sx0:sx1]

    # Recortar lo que se cuela del vecino: el corte va a mitad de camino entre
    # este frame y el de al lado, no en el borde de la ventana.
    lo = -10**9 if i == 0 or spans[i-1][2] != band else (spans[i-1][1]+s)//2
    hi =  10**9 if i == len(spans)-1 or spans[i+1][2] != band else (e+spans[i+1][0])//2
    if lo > left:  win[:, :lo-left] = 0
    if hi < right: win[:, hi-left:] = 0

    small = np.asarray(Image.fromarray(win).resize((CELL, CELL), Image.LANCZOS)).astype(int)

    # Color key con umbral, no por igualdad. El original de AO usa negro puro
    # como transparente porque sus sprites no tienen alfa; este arte sale de un
    # JPG y el ruido de compresion dejaria un recuadro sucio alrededor. Manda
    # el canal mas alto, no la luminancia: el humo rojo oscuro del final es
    # tenue en luminancia pero tiene que verse.
    rgba = np.zeros((CELL, CELL, 4), dtype=np.uint8)
    rgba[:, :, :3] = small
    rgba[:, :, 3] = np.clip((small.max(axis=2)-12)*255//55, 0, 255)
    cells.append(Image.fromarray(rgba, 'RGBA'))

# 28 frames de origen contra los 21 que tiene el atlas: se descartan 7 parejo
# para no alterar el ritmo de la animacion.
KEEP = [SKIP + round(i*(len(cells)-1-SKIP)/(FRAMES-1)) for i in range(FRAMES)]
final = [cells[k] for k in KEEP]


if __name__ == '__main__':
    strip = Image.new('RGBA', (len(final)*CELL, CELL), (0, 0, 0, 0))
    for i, c in enumerate(final):
        strip.paste(c, (i*CELL, 0))
    strip.save(OUT)
    print(f'{OUT}: {len(final)} frames de {CELL}x{CELL} sacados de {len(cells)}')
