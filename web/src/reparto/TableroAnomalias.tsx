import { useMemo } from "react";
import { Link, useSearchParams } from "react-router-dom";
import Cargando from "../Cargando";
import { Tarjeta } from "../tablero/Tarjeta";
import { formatearEntero } from "../tablero/formato";
import { useRecurso } from "../tablero/useDashboard";
import {
  TIPOS_DE_ALERTA,
  conteoDeTipo,
  etiquetaDeTipo,
  pendientes,
} from "./anomalias";
import { ETIQUETA_CIRCUITO } from "./etapas";
import { Alerta, Proceso, RUTAS_REPARTO } from "./tipos";

export default function TableroAnomalias() {
  const [params, setParams] = useSearchParams();
  const periodoParam = params.get("periodo") ?? "";

  const procesos = useRecurso<Proceso[]>(RUTAS_REPARTO.procesos);
  const periodos = useMemo(() => {
    if (procesos.tipo !== "listo") return [];
    return [...new Set(procesos.datos.map((p) => p.periodo))].sort();
  }, [procesos]);

  const periodo = periodoParam || periodos[0] || "";
  const alertas = useRecurso<Alerta[]>(RUTAS_REPARTO.alertas(periodo), true);

  const lista = alertas.tipo === "listo" ? pendientes(alertas.datos) : [];

  function elegirPeriodo(valor: string) {
    const siguiente = new URLSearchParams(params);
    if (valor) siguiente.set("periodo", valor);
    else siguiente.delete("periodo");
    setParams(siguiente);
  }

  return (
    <section className="tablero">
      <header className="tablero-cabecera">
        <div>
          <h1>Anomalías</h1>
          <p className="muted">
            Alertas del periodo, por tipo. Nada se reparte con críticas
            abiertas.
          </p>
        </div>
        <label className="selector-periodo">
          Periodo
          <select
            value={periodo}
            onChange={(evento) => elegirPeriodo(evento.target.value)}
            aria-label="Periodo"
          >
            {periodo === "" && <option value="">Sin periodo</option>}
            {periodos.map((p) => (
              <option key={p} value={p}>
                {p}
              </option>
            ))}
            {periodo !== "" && !periodos.includes(periodo) && (
              <option value={periodo}>{periodo}</option>
            )}
          </select>
        </label>
      </header>

      <div className="tablero-kpis">
        {TIPOS_DE_ALERTA.map((tipo) => (
          <Tarjeta
            key={tipo}
            titulo={etiquetaDeTipo(tipo)}
            descripcion="Abiertas en el periodo"
            to="#bandeja"
            etiquetaEnlace="Ir a resolución"
            recurso={conteoDeTipo(alertas, tipo)}
          >
            {(datos) => (
              <p className="tarjeta-valor">{formatearEntero(datos.total)}</p>
            )}
          </Tarjeta>
        ))}
      </div>

      <article className="tarjeta tarjeta-amplia" id="bandeja">
        <h2 className="tarjeta-etiqueta">Bandeja de resolución</h2>
        <p className="muted">
          La asignación y el descarte con rastro de auditoría aterrizan con la
          bandeja de matching. Mientras tanto, este listado es el recuento
          operativo del periodo.
        </p>
        {alertas.tipo === "cargando" && <Cargando texto="Cargando alertas…" />}
        {alertas.tipo === "error" && (
          <p className="tarjeta-error" role="alert">
            {alertas.mensaje}
          </p>
        )}
        {alertas.tipo === "ausente" && (
          <p className="muted">Sin datos todavía.</p>
        )}
        {alertas.tipo === "listo" && lista.length === 0 && (
          <p className="muted">No hay alertas abiertas en este periodo.</p>
        )}
        {alertas.tipo === "listo" && lista.length > 0 && (
          <table>
            <thead>
              <tr>
                <th>Tipo</th>
                <th>Detalle</th>
                <th>Referencia</th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              {lista.map((alerta) => (
                <tr key={alerta.id}>
                  <td>
                    <span className="badge">{etiquetaDeTipo(alerta.tipo)}</span>
                  </td>
                  <td>{alerta.detalle}</td>
                  <td>{alerta.referencia ?? alerta.periodo ?? "—"}</td>
                  <td>
                    <Link to="#bandeja">Resolver</Link>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </article>

      {procesos.tipo === "listo" && procesos.datos.length > 0 && (
        <p className="muted">
          Corridas del periodo:{" "}
          {procesos.datos
            .filter((p) => !periodo || p.periodo === periodo)
            .map((p) => ETIQUETA_CIRCUITO[p.circuito])
            .join(" · ") || "ninguna"}
          . <Link to="/distribucion">Volver al panel de corridas</Link>
        </p>
      )}
    </section>
  );
}
