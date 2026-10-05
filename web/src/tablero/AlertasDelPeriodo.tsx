import { TIPOS_DE_ALERTA, etiquetaDeTipo, plural } from "../reparto/anomalias";
import { ResumenDeAlertas } from "../reparto/tipos";
import { formatearEntero } from "./formato";
import { BotonEnlace } from "./BotonEnlace";
import { Panel } from "./Panel";
import { Recurso } from "./tipos";

function estado(r: ResumenDeAlertas): { texto: string; clase: string } {
  if (r.ultima_evaluacion === null) {
    return { texto: "Periodo sin evaluar", clase: "chip-alerta" };
  }
  const c = r.criticas_abiertas;
  if (c > 0) {
    return {
      texto: `${formatearEntero(c)} ${plural(c, "crítica bloquea", "críticas bloquean")} la distribución`,
      clase: "chip-error",
    };
  }
  return { texto: "Ninguna bloquea la distribución", clase: "chip-ok" };
}

export function AlertasDelPeriodo({
  resumen,
  periodo,
}: {
  resumen: Recurso<ResumenDeAlertas>;
  periodo?: string;
}) {
  return (
    <Panel
      titulo="Alertas"
      className="staff-alertas"
      recurso={resumen}
      mensajeAusente={
        periodo
          ? "Sin datos todavía"
          : "Sin distribución abierta: no hay periodo que revisar."
      }
    >
      {(r) => {
        const e = estado(r);
        const tipos = TIPOS_DE_ALERTA.filter(
          (t) => (r.por_tipo?.[t]?.abiertas ?? 0) > 0,
        );
        return (
          <>
            <div className="staff-alertas-total">
              <span className="staff-cifra-media">
                {formatearEntero(r.abiertas)}
              </span>
              <span className="muted">
                {plural(r.abiertas, "abierta", "abiertas")} en {r.periodo}
              </span>
            </div>
            <span className={`chip ${e.clase}`}>{e.texto}</span>
            {tipos.length > 0 && (
              <ul className="staff-alertas-tipos">
                {tipos.map((t) => (
                  <li key={t}>
                    <span
                      className={
                        r.por_tipo[t].critica
                          ? "staff-punto staff-punto-critico"
                          : "staff-punto"
                      }
                      aria-hidden="true"
                    />
                    <span>{etiquetaDeTipo(t)}</span>
                    <span className="staff-alertas-n">
                      {formatearEntero(r.por_tipo[t].abiertas)}
                    </span>
                  </li>
                ))}
              </ul>
            )}
            <footer className="panel-pie staff-pie-accion">
              <BotonEnlace
                to={`/anomalias?periodo=${encodeURIComponent(r.periodo)}`}
              >
                Resolver alertas
              </BotonEnlace>
            </footer>
          </>
        );
      }}
    </Panel>
  );
}
