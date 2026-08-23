# /// script
# requires-python = ">=3.10"
# dependencies = ["diagrams>=0.23"]
# ///
# Genera la topologia de despliegue de Intela como codigo.
# Run: uv run --script src/scripts/diagrama_despliegue.py
# Requiere Graphviz (apt install graphviz / brew install graphviz).
#
# Reparto de responsabilidades con el .drawio, para que no se contradigan:
#
#   docs/PATIC2 - Arquitectura.drawio, pagina 3  -> vista ANOTADA. Manda.
#       Lleva las citas al reglamento y las notas de decision que esta
#       libreria no sabe renderizar.
#   docs/despliegue.png (este script)            -> TOPOLOGIA de infraestructura.
#       Se regenera desde codigo y sigue al docker-compose.
#
# Si las dos discrepan, la pagina 3 es la correcta y este script esta viejo.

from pathlib import Path

from diagrams import Cluster, Diagram, Edge
from diagrams.generic.blank import Blank
from diagrams.generic.storage import Storage
from diagrams.onprem.client import Users
from diagrams.onprem.compute import Server
from diagrams.onprem.database import PostgreSQL
from diagrams.onprem.monitoring import Grafana
from diagrams.onprem.network import Nginx

ROOT = Path(__file__).resolve().parents[2]
SALIDA = ROOT / "docs" / "despliegue"

ATTRS = {
    "fontsize": "12",
    "labelloc": "t",
    "pad": "0.5",
    "nodesep": "0.45",
    "ranksep": "1.0",
    "bgcolor": "white",
    "splines": "ortho",
}
NODE_ATTRS = {"fontsize": "11"}

with Diagram(
    "Intela - topologia de despliegue (ADR 0003, ADR 0010)",
    filename=str(SALIDA),
    outformat="png",
    show=False,
    direction="TB",
    graph_attr=ATTRS,
    node_attr=NODE_ATTRS,
):
    titulares = Users("Titulares, administracion\ny auditoria")

    with Cluster("docker compose"):
        proxy = Nginx("reverse-proxy\nTLS, limites de tasa")

        with Cluster("mismo nucleo, un binario por paquete de cmd/"):
            api = Server("api\ncmd/api - Go + chi\nadaptadores HTTP + nucleo")
            scheduler = Server("scheduler\ncmd/scheduler\nlee CalendarioDeDistribucion")
            worker_m = Server("worker-matching\ncmd/worker\ncascada de identificacion")
            worker_r = Server("worker-reparto\ncmd/worker\nvalorizacion y liquidacion")

        dashboard = Server("dashboard\nReact + TS + Vite\ntipos desde OpenAPI")

        with Cluster("estado"):
            pg = PostgreSQL("postgres 16\nrepertorio, splits,\nparametros con vigencia")
            cola = PostgreSQL("cola\nRiver, mismo postgres\nuna transaccion por etapa")
            objetos = Storage("almacen-objetos\nMinIO / S3\nreportes crudos inmutables")
            bitacora = Storage(
                "bitacora append-only\ntabla en postgres sin UPDATE\nni DELETE, retencion 10 anos"
            )

        observabilidad = Grafana("observabilidad\ndistinta de la bitacora")

    with Cluster("sistemas externos"):
        cisac = Blank("CISAC\nIDA (obras) e IPI (autores)")
        rating = Blank("proveedor de rating\npor franja horaria")
        banco = Blank("banco\ndispersion de pagos")
        erp = Blank("ERP contable")
        correo = Blank("correo saliente\nla notificacion es\nun hecho juridico")
        redessys = Blank("REDES-SYS / AVSYS\nalcance por confirmar")

    titulares >> proxy >> dashboard >> api

    api >> pg
    api >> cola
    api >> Edge(label="reporte crudo\ninmutable") >> objetos
    api >> Edge(style="dashed") >> redessys
    api >> Edge(style="dashed") >> correo

    scheduler >> cola
    scheduler >> Edge(label="feed anual", style="dashed") >> rating

    worker_m >> cola
    worker_m >> Edge(style="dashed") >> cisac

    worker_r >> cola
    worker_r >> objetos
    worker_r >> Edge(style="dashed") >> banco
    worker_r >> Edge(style="dashed") >> erp

    bitacora >> Edge(label="copia inmutable") >> objetos
    api >> Edge(style="dotted", color="gray") >> observabilidad

print(f"Generado {SALIDA.with_suffix('.png')}")
