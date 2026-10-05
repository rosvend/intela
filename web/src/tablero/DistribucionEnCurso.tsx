import { Link } from "react-router-dom";
import Pipeline from "../reparto/Pipeline";
import { ETIQUETA_CIRCUITO, ETIQUETA_ETAPA, etapasDe } from "../reparto/etapas";
import {
  ETIQUETA_ROL_FIRMA,
  esCompuerta,
  rolesPendientes,
} from "../reparto/firmas";
import { Proceso } from "../reparto/tipos";
import { Panel } from "./Panel";
import { Recurso } from "./tipos";

function ultima(procesos: Recurso<Proceso[]>): Recurso<Proceso> {
  if (procesos.tipo !== "listo") return procesos;
  const p = Array.isArray(procesos.datos) ? procesos.datos[0] : undefined;
  return p ? { tipo: "listo", datos: p } : { tipo: "ausente" };
}

export function DistribucionEnCurso({
  procesos,
  enlace,
}: {
  procesos: Recurso<Proceso[]>;
  /** Si el rol puede abrir /distribucion. */
  enlace: boolean;
}) {
  return (
    <Panel
      titulo="Distribución en curso"
      className="staff-distribucion"
      recurso={ultima(procesos)}
      mensajeAusente="No hay ninguna distribución abierta."
    >
      {(p) => {
        const pasos = etapasDe(p.circuito);
        const n = pasos.indexOf(p.etapa) + 1;
        const faltan = esCompuerta(p.etapa) ? rolesPendientes(p) : [];
        return (
          <>
            <div className="staff-distribucion-resumen">
              <div>
                <p className="staff-distribucion-etapa">
                  {ETIQUETA_ETAPA[p.etapa]}
                </p>
                <p className="muted">
                  Periodo {p.periodo} · circuito{" "}
                  {ETIQUETA_CIRCUITO[p.circuito].toLowerCase()}
                </p>
              </div>
              <span className="chip chip-marca">
                Etapa {n} de {pasos.length}
              </span>
            </div>
            <Pipeline circuito={p.circuito} etapa={p.etapa} compacto />
            {faltan.length > 0 && (
              <p className="staff-distribucion-firmas">
                Falta la firma de{" "}
                {faltan.map((r) => ETIQUETA_ROL_FIRMA[r]).join(" y ")} para
                seguir.
              </p>
            )}
            {enlace && (
              <footer className="panel-pie">
                <span>Se actualiza al abrirla</span>
                <Link
                  className="tarjeta-enlace"
                  to={`/distribucion/${encodeURIComponent(p.id)}`}
                >
                  Abrir distribución
                </Link>
              </footer>
            )}
          </>
        );
      }}
    </Panel>
  );
}
