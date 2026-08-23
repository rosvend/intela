# /// script
# requires-python = ">=3.10"
# dependencies = []
# ///
# Verifica que el diagrama de arquitectura cumpla lo que los ADR prometen.
# Run: uv run --script src/scripts/check_arquitectura.py
#
# Los ADR 0002 y 0003 nombran esta comprobacion como su mitigacion y piden que
# el criterio de aceptacion del diagrama sea el mismo que el del test. Esto es
# ese test para el diagrama; el equivalente sobre el codigo sera
# dependency-cruiser cuando exista el stack (ADR 0009).

import sys
import xml.etree.ElementTree as ET
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
DIAGRAM = ROOT / "docs" / "PATIC2 - Arquitectura.drawio"

# Pagina 1: a que anillo pertenece cada celda. El orden importa: la regla de
# dependencia dice que nadie de un anillo interior puede nombrar uno exterior.
ADAPTADORES_ENTRADA = {"p1-a1", "p1-a2", "p1-a3", "p1-a4", "p1-a5", "p1-a6", "p1-a7", "p1-gw"}
PUERTOS_ENTRADA = {f"p1-pi{i}" for i in range(0, 8)}
NUCLEO = {"p1-app", "p1-dom", "p1-orq", "p1-apr"} | {f"p1-d{i}" for i in range(1, 7)}
PUERTOS_SALIDA = {f"p1-po{i}" for i in range(1, 7)}
ADAPTADORES_SALIDA = {f"p1-o{i}" for i in range(1, 12)}


def cargar(nombre_prefijo):
    raiz = ET.parse(DIAGRAM).getroot()
    for d in raiz.findall("diagram"):
        if (d.get("name") or "").startswith(nombre_prefijo):
            celdas = d.find(".//root").findall("mxCell")
            aristas = [
                (c.get("id"), c.get("source"), c.get("target"), c.get("style") or "")
                for c in celdas
                if c.get("source") and c.get("target")
            ]
            vertices = {c.get("id") for c in celdas if c.get("vertex")}
            return aristas, vertices
    raise SystemExit(f"No encontre la pagina que empieza por '{nombre_prefijo}'")


def anillo(cid):
    for nombre, conjunto in (
        ("adaptador-entrada", ADAPTADORES_ENTRADA),
        ("puerto-entrada", PUERTOS_ENTRADA),
        ("nucleo", NUCLEO),
        ("puerto-salida", PUERTOS_SALIDA),
        ("adaptador-salida", ADAPTADORES_SALIDA),
    ):
        if cid in conjunto:
            return nombre
    return None


def check_regla_de_dependencia(fallos):
    """ADR 0002: ninguna arista sale del nucleo hacia la infraestructura."""
    aristas, _ = cargar("1")
    for aid, src, dst, _ in aristas:
        a_src, a_dst = anillo(src), anillo(dst)
        if a_src == "nucleo" and a_dst in ("adaptador-salida", "adaptador-entrada"):
            fallos.append(
                f"[{aid}] el nucleo depende de un adaptador: {src} -> {dst}. "
                "La dependencia tiene que invertirse por un puerto."
            )
        if a_src == "puerto-salida" and a_dst == "adaptador-salida":
            fallos.append(
                f"[{aid}] {src} -> {dst} apunta hacia afuera. El adaptador implementa "
                "el puerto, asi que la arista va adaptador -> puerto."
            )


def check_realizaciones(fallos):
    """El borde de salida se dibuja como realizacion, no como uso."""
    aristas, _ = cargar("1")
    encontradas = 0
    for aid, src, dst, style in aristas:
        if anillo(src) == "adaptador-salida" and anillo(dst) == "puerto-salida":
            encontradas += 1
            if "dashed=1" not in style or "endArrow=block" not in style:
                fallos.append(
                    f"[{aid}] {src} -> {dst} es una realizacion y debe dibujarse "
                    "punteada con punta hueca, para distinguirla de un 'usa'."
                )
    if encontradas == 0:
        fallos.append(
            "No hay ninguna arista adaptador-salida -> puerto-salida. La inversion "
            "de dependencia del borde de salida no esta representada."
        )


def check_puertos_alcanzables(fallos):
    """Todo puerto de entrada tiene por donde ser invocado y a donde entregar."""
    aristas, vertices = cargar("1")
    entrantes = {d for _, _, d, _ in aristas}
    salientes = {s for _, s, _, _ in aristas}
    for pid in sorted(PUERTOS_ENTRADA & vertices):
        if pid not in entrantes:
            fallos.append(f"[{pid}] puerto de entrada sin adaptador que lo invoque.")
        if pid not in salientes:
            fallos.append(f"[{pid}] puerto de entrada que no entrega a la aplicacion.")


def check_modulos_aciclicos(fallos):
    """ADR 0003: las dependencias entre modulos no pueden formar ciclos."""
    aristas, _ = cargar("2")
    adyacencia = {}
    for _, src, dst, _ in aristas:
        adyacencia.setdefault(src, []).append(dst)

    BLANCO, GRIS, NEGRO = 0, 1, 2
    color = {}

    def visitar(n, camino):
        color[n] = GRIS
        for m in adyacencia.get(n, []):
            estado = color.get(m, BLANCO)
            if estado == GRIS:
                ciclo = " -> ".join(camino + [n, m])
                fallos.append(f"ciclo entre modulos: {ciclo}")
            elif estado == BLANCO:
                visitar(m, camino + [n])
        color[n] = NEGRO

    for n in list(adyacencia):
        if color.get(n, BLANCO) == BLANCO:
            visitar(n, [])


def check_externos_conectados(fallos):
    """Un sistema externo dibujado y sin conectar es una omision, no una decision."""
    aristas, _ = cargar("3")
    tocados = {x for _, s, d, _ in aristas for x in (s, d)}
    externos = {
        "p3-cisac": "CISAC (IDA / IPI)",
        "p3-rating": "proveedor de rating",
        "p3-banco": "dispersion bancaria",
        "p3-erp": "ERP contable",
        "p3-smtp": "correo saliente",
        "p3-redessys": "REDES-SYS / AVSYS",
    }
    for cid, nombre in externos.items():
        if cid not in tocados:
            fallos.append(f"[{cid}] {nombre} esta dibujado pero no lo consume nadie.")


def main():
    if not DIAGRAM.exists():
        raise SystemExit(f"No encontre {DIAGRAM}")

    fallos = []
    comprobaciones = (
        ("regla de dependencia (ADR 0002)", check_regla_de_dependencia),
        ("realizaciones en el borde de salida", check_realizaciones),
        ("puertos de entrada alcanzables", check_puertos_alcanzables),
        ("modulos aciclicos (ADR 0003)", check_modulos_aciclicos),
        ("sistemas externos conectados", check_externos_conectados),
    )

    for nombre, fn in comprobaciones:
        antes = len(fallos)
        fn(fallos)
        marca = "OK  " if len(fallos) == antes else "FALLA"
        print(f"  {marca}  {nombre}")

    print()
    if fallos:
        print(f"{len(fallos)} problema(s):\n")
        for f in fallos:
            print(f"  - {f}")
        return 1
    print("El diagrama cumple la regla de dependencia y las fronteras de modulo.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
