# /// script
# requires-python = ">=3.10"
# dependencies = []
# ///
# Genera la vista minima de la arquitectura para el README.
# Run: uv run --script src/scripts/diagrama_arquitectura.py
# Requiere Graphviz (apt install graphviz / brew install graphviz).
#
# Reparto de responsabilidades con el .drawio, para que no se contradigan:
#
#   docs/diagrams/PATIC2 - Arquitectura.drawio  -> vista COMPLETA. Manda.
#       Tres paginas, 126 cajas, con citas al reglamento. Es la que verifica
#       check_arquitectura.py y la que se lee para trabajar.
#   docs/diagrams/arquitectura-{light,dark}.png -> vista de PORTADA (este script).
#       Cinco columnas, cero detalle. Solo responde "que forma tiene esto"
#       en los treinta segundos que dura un README.
#
# Si las dos discrepan, el .drawio es el correcto y este script esta viejo.
#
# Por que Graphviz pelado y no la libreria `diagrams` que usa
# diagrama_despliegue.py: `diagrams` existe para colgar iconos de producto
# (AWS, on-prem) de una topologia. Aqui no hay iconos que poner -son capas
# abstractas- asi que obligaria a nodos `Blank` y a pelearse con su layout.
# `diagrams` es un envoltorio de Graphviz de todas formas.
#
# Dos PNG y no uno porque GitHub sirve la pagina en tema claro u oscuro y el
# README los conmuta con <picture media="(prefers-color-scheme: dark)">. Fondo
# transparente en ambos.

import shutil
import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
SALIDA = ROOT / "docs" / "diagrams"

# Paleta. La tinta y los grises salen de los tokens de GitHub para que el
# contraste aguante sobre #ffffff y sobre #0d1117. El acento es el burdeos de
# la marca; en oscuro va aclarado, porque #4A101F sobre fondo negro no se ve.
TEMAS = {
    "light": {
        "tinta": "#1F2328",
        "suave": "#656D76",
        "linea": "#AFB8C1",
        "acento": "#4A101F",
    },
    "dark": {
        "tinta": "#E6EDF3",
        "suave": "#9198A1",
        "linea": "#5A6570",
        "acento": "#E8A2B4",
    },
}

# `splines=polyline` y no `ortho`: ortho descarta las etiquetas de arista sin
# avisar mas que con un warning, y desordena las columnas.
PLANTILLA = """digraph intela {{
  bgcolor="transparent";
  rankdir=LR;
  splines=polyline;
  newrank=true;
  compound=true;
  nodesep=0.34;
  ranksep=1.05;
  fontname="Helvetica";

  node [shape=box, style="rounded", fontname="Helvetica", fontsize=11,
        color="{linea}", fontcolor="{tinta}", penwidth=1.1, margin="0.24,0.17"];
  edge [color="{linea}", fontname="Helvetica", fontsize=9, fontcolor="{suave}",
        arrowsize=0.7, penwidth=1.1];

  subgraph cluster_entrada {{
    label="ADAPTADORES DE ENTRADA";
    fontsize=9; fontcolor="{suave}"; color="{linea}";
    style=dashed; penwidth=0.9; margin=14;
    ent [label="Portales · API · Webhooks\\lCargador de reportes\\lPlanificador · CLI\\l"];
  }}

  subgraph cluster_pentrada {{
    label="PUERTOS DE ENTRADA";
    fontsize=9; fontcolor="{suave}"; color="{linea}";
    style=dashed; penwidth=0.9; margin=14;
    pi [label="Casos de uso\\l"];
  }}

  subgraph cluster_nucleo {{
    label="NUCLEO · no nombra nada de afuera";
    fontsize=9; fontcolor="{acento}"; color="{acento}";
    style=solid; penwidth=1.4; margin=16;
    app [label="Aplicacion\\lorquestacion, transacciones\\l",
         color="{acento}", penwidth=1.3];
    dom [label="Dominio\\lentidades, invariantes,\\lcalculo puro (sin E/S)\\l",
         color="{acento}", penwidth=1.3];
    app -> dom [color="{acento}"];
  }}

  subgraph cluster_psalida {{
    label="PUERTOS DE SALIDA";
    fontsize=9; fontcolor="{suave}"; color="{linea}";
    style=dashed; penwidth=0.9; margin=14;
    po [label="Interfaces que\\ldeclara el nucleo\\l"];
  }}

  subgraph cluster_salida {{
    label="ADAPTADORES DE SALIDA";
    fontsize=9; fontcolor="{suave}"; color="{linea}";
    style=dashed; penwidth=0.9; margin=14;
    sal [label="PostgreSQL · Cola · Objetos\\lCISAC · Banco · ERP · Correo\\l"];
  }}

  ent -> pi [arrowhead=open];
  pi  -> app [arrowhead=open, lhead=cluster_nucleo];

  // Arista invisible: sin ella `po` queda en el mismo rango que `dom` -las dos
  // cuelgan de `app`- y las dos ultimas columnas se caen a una segunda fila.
  dom -> po [style=invis];

  // Sale del BORDE del nucleo (ltail), no de la caja de Aplicacion: quien
  // depende del puerto es el nucleo entero, y asi la arista no atraviesa
  // la caja de Dominio al salir.
  app -> po [arrowhead=open, ltail=cluster_nucleo];

  // Escrita al reves a proposito. `sal -> po` colocaria los adaptadores a la
  // IZQUIERDA de los puertos y romperia la lectura de izquierda a derecha;
  // `dir=back` conserva el rango y voltea la punta. Es la inversion de
  // dependencia del borde de salida: el adaptador implementa el contrato.
  po -> sal [dir=back, arrowtail=empty, style=dashed, label="implementa",
             color="{acento}", fontcolor="{acento}"];
}}
"""


def generar(tema: str, colores: dict[str, str]) -> Path:
    destino = SALIDA / f"arquitectura-{tema}.png"
    subprocess.run(
        ["dot", "-Tpng", "-Gdpi=144", "-o", str(destino)],
        input=PLANTILLA.format(**colores),
        text=True,
        check=True,
    )
    return destino


def main() -> int:
    if shutil.which("dot") is None:
        print("Falta Graphviz. apt install graphviz / brew install graphviz", file=sys.stderr)
        return 1

    SALIDA.mkdir(parents=True, exist_ok=True)
    for tema, colores in TEMAS.items():
        print(f"Generado {generar(tema, colores)}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
