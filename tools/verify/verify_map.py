#!/usr/bin/env python3
"""Parser de referencia del formato .map de Argentum, independiente de aoconv.

Existe para una sola cosa: contestar "¿aoconv lee bien el .map?" sin creerle a
aoconv. Está escrito desde cero a partir del formato documentado en
OPERACION.md §3, en otro lenguaje y por otro camino, así que un error de lectura
tendría que repetirse idéntico en los dos lados para pasar desapercibido.

Dos señales lo respaldan más allá de la comparación:

  - El .map no lleva largo por tile, así que un solo byte mal leído desincroniza
    todo lo que sigue. Terminar exactamente en EOF es evidencia fuerte.
  - El conteo de tiles bloqueados tiene que coincidir con el del servidor, que
    lo guarda como bitset y lo usa para las colisiones de verdad.

Uso:

    python tools/verify/verify_map.py \\
        --map "$env:USERPROFILE/Downloads/ao-assets/Mundos/Mapa1.map"

Sale con código 1 si algo no coincide, así que sirve en CI.
"""

import argparse
import base64
import json
import struct
import sys
from pathlib import Path

HEADER_SIZE = 273  # version int16 + desc[255] + crc int32 + magic int32 + 8 reservados

# Bits del byte de flags de cada tile. El bit de bloqueo no consume bytes; los
# de capa 2/3/4 y el de trigger sí, y solo si están prendidos.
FLAG_BLOCKED = 1
FLAG_LAYER2 = 2
FLAG_LAYER3 = 4
FLAG_LAYER4 = 8
FLAG_TRIGGER = 16


def parse_map(path):
    """Devuelve las cuatro capas, el bitset de bloqueo y los triggers de un .map."""
    buf = path.read_bytes()
    off = 0

    (version,) = struct.unpack_from("<h", buf, off)
    off += 2
    desc = buf[off : off + 255]
    off += 255
    off += 8  # crc + magic
    off += 8  # reservados
    if off != HEADER_SIZE:
        raise AssertionError(f"header quedó en {off}, esperaba {HEADER_SIZE}")

    width = height = 100
    layer1 = [0] * (width * height)
    layers = {2: {}, 3: {}, 4: {}}
    blocked = bytearray(width * height)
    triggers = {}

    # El recorrido es Y afuera, X adentro, y ambos arrancan en 1 en el original.
    for y in range(height):
        for x in range(width):
            (flags,) = struct.unpack_from("<B", buf, off)
            off += 1
            i = y * width + x

            if flags & FLAG_BLOCKED:
                blocked[i] = 1

            (layer1[i],) = struct.unpack_from("<i", buf, off)
            off += 4

            for bit, n in ((FLAG_LAYER2, 2), (FLAG_LAYER3, 3), (FLAG_LAYER4, 4)):
                if flags & bit:
                    (grh,) = struct.unpack_from("<i", buf, off)
                    off += 4
                    layers[n][i] = grh

            if flags & FLAG_TRIGGER:
                (triggers[i],) = struct.unpack_from("<h", buf, off)
                off += 2

    return {
        "version": version,
        "desc": desc.split(b"\0")[0].decode("latin-1").strip(),
        "consumed": off,
        "size": len(buf),
        "width": width,
        "height": height,
        "layer1": layer1,
        "layers": layers,
        "blocked": blocked,
        "triggers": triggers,
    }


def playable_bounds(blocked, width, height):
    """Espesor del anillo exterior que está bloqueado al 100%."""

    def ring_is_solid(w):
        for y in range(height):
            for x in range(width):
                if x < w or y < w or x >= width - w or y >= height - w:
                    if not blocked[y * width + x]:
                        return False
        return True

    w = 0
    while w < min(width, height) // 2 and ring_is_solid(w + 1):
        w += 1
    return w


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--map", required=True, type=Path, help="ruta al MapaN.map original")
    ap.add_argument("--client", type=Path, default=Path("client/assets/ao/map1.json"))
    ap.add_argument("--server", type=Path, default=Path("server/maps/map1.json"))
    args = ap.parse_args()

    ref = parse_map(args.map)
    print(f"{args.map.name}: version {ref['version']}")
    print(f"  desc: {ref['desc'][:70]}")

    exact = ref["consumed"] == ref["size"]
    print(f"  bytes consumidos: {ref['consumed']}/{ref['size']} " f"-> {'exacto' if exact else 'DESINCRONIZADO'}")
    if not exact:
        print("\nEl parser se desincronizó; el resto de la comparación no significa nada.")
        return 1

    cli = json.loads(args.client.read_text(encoding="utf-8"))
    srv = json.loads(args.server.read_text(encoding="utf-8"))
    width = ref["width"]
    bits = base64.b64decode(srv["blocked"])

    results = []
    diff = sum(1 for i, g in enumerate(ref["layer1"]) if cli["layer1"][i] != g)
    results.append(("layer1", diff))

    for n in (2, 3, 4):
        ours = cli[f"layer{n}"]
        theirs = ref["layers"][n]
        keys = set(ours) | {str(k) for k in theirs}
        diff = sum(1 for k in keys if int(ours.get(k, 0)) != theirs.get(int(k), 0))
        results.append((f"layer{n}", diff))

    diff = 0
    for i in range(width * ref["height"]):
        ours_blocked = bool(bits[i >> 3] & (1 << (i & 7)))
        if ours_blocked != bool(ref["blocked"][i]):
            diff += 1
    results.append(("bloqueados", diff))

    total = width * ref["height"]
    print(f"\n  {'campo':12} {'coinciden':>16}")
    for name, d in results:
        mark = "OK" if d == 0 else f"{d} DIFIEREN"
        print(f"  {name:12} {total - d:>7}/{total:<7} {mark}")

    border = playable_bounds(ref["blocked"], width, ref["height"])
    inner = width - 2 * border
    inner_blocked = sum(
        ref["blocked"][y * width + x] for y in range(border, ref["height"] - border) for x in range(border, width - border)
    )
    print(f"\n  borde bloqueado al 100%: {border} tiles de espesor")
    print(f"  area jugable: {inner}x{inner} desde ({border},{border}), " f"{inner * inner - inner_blocked} caminables")

    failed = sum(d for _, d in results)
    print("\n" + ("aoconv coincide en todos los campos." if not failed else f"{failed} diferencias."))
    return 1 if failed else 0


if __name__ == "__main__":
    sys.exit(main())
