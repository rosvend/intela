import { useEffect, useMemo, useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import Cargando from "../Cargando";
import { ApiError } from "../api";
import { useSesion } from "../sesion";
import { Tarjeta } from "../tablero/Tarjeta";
import { formatearEntero } from "../tablero/formato";
import { useRecurso } from "../tablero/useDashboard";
import {
  TIPOS_DE_ALERTA,
  conteoDeTipo,
  etiquetaDeTipo,
  plural,
  puedeEvaluar,
} from "./anomalias";
import { evaluarPeriodo } from "./cliente";
import { ETIQUETA_CIRCUITO } from "./etapas";
import { Alerta, Proceso, ResumenDeAlertas, RUTAS_REPARTO } from "./tipos";

export default function TableroAnomalias() {
  const [params, setParams] = useSearchParams();
  const { usuario } = useSesion();
  const periodoParam = params.get("periodo") ?? "";
  const [recarga, setRecarga] = useState(0);
  const [evaluacion, setEvaluacion] = useState({ enviando: false, error: "" });

  const procesos = useRecurso<Proceso[]>(RUTAS_REPARTO.procesos);
  const periodos = useMemo(() => {
    if (procesos.tipo !== "listo") return [];
    return [...new Set(procesos.datos.map((p) => p.periodo))].sort();
  }, [procesos]);

  const periodo = periodoParam || periodos[0] || "";

  useEffect(() => {
    setEvaluacion({ enviando: false, error: "" });
  }, [periodo]);

  // Solo pedimos con un periodo concreto. Sin `?periodo` esperamos a
  // `procesos` para tomar el primero; si falla o la lista viene vacia, no
  // pedimos nada sin filtro (eso pintaba un conteo global bajo una cabecera
  // que no nombra ningun periodo).
  const resumen = useRecurso<ResumenDeAlertas>(
    RUTAS_REPARTO.resumenAlertas(periodo),
    periodo !== "",
    recarga,
  );
  // El servidor ya filtra las abiertas (`resueltas=false`).
  const alertas = useRecurso<Alerta[]>(
    RUTAS_REPARTO.alertas(periodo),
    periodo !== "",
    recarga,
  );

  const lista = alertas.tipo === "listo" ? alertas.datos : [];
  const sinEvaluar =
    resumen.tipo === "listo" && resumen.datos.ultima_evaluacion === null;

  function elegirPeriodo(valor: string) {
    const siguiente = new URLSearchParams(params);
    if (valor) siguiente.set("periodo", valor);
    else siguiente.delete("periodo");
    setParams(siguiente);
  }

  async function evaluar() {
    if (evaluacion.enviando) return;
    setEvaluacion({ enviando: true, error: "" });
    try {
      await evaluarPeriodo(periodo);
      setRecarga((n) => n + 1);
      setEvaluacion({ enviando: false, error: "" });
    } catch (error: unknown) {
      setEvaluacion({
        enviando: false,
        error:
          error instanceof ApiError
            ? error.message
            : "no se pudo evaluar el periodo",
      });
    }
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
          {resumen.tipo === "listo" && !sinEvaluar && (
            <p className="muted">
              {formatearEntero(resumen.datos.criticas_abiertas)}{" "}
              {plural(
                resumen.datos.criticas_abiertas,
                "crítica abierta bloquea",
                "críticas abiertas bloquean",
              )}{" "}
              el reparto de este periodo.
              {resumen.datos.criticas_aceptadas > 0 &&
                ` · ${formatearEntero(resumen.datos.criticas_aceptadas)} aceptadas sin corregir`}
            </p>
          )}
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

      {sinEvaluar && (
        <div role="status" className="login-aviso">
          <p>
            Este periodo no se ha evaluado todavía: los conteos no dicen si está
            limpio.
          </p>
          {usuario && puedeEvaluar(usuario.rol) && (
            <button
              type="button"
              className="boton-primario"
              disabled={evaluacion.enviando}
              onClick={() => void evaluar()}
            >
              {evaluacion.enviando ? "Evaluando…" : "Evaluar periodo"}
            </button>
          )}
          {evaluacion.error && (
            <p className="tarjeta-error" role="alert">
              {evaluacion.error}
            </p>
          )}
        </div>
      )}

      <div className="tablero-kpis">
        {TIPOS_DE_ALERTA.map((tipo) => (
          <Tarjeta
            key={tipo}
            titulo={etiquetaDeTipo(tipo)}
            descripcion={
              resumen.tipo === "listo" && resumen.datos.por_tipo[tipo].critica
                ? "Abiertas · bloquean la corrida"
                : periodo
                  ? "Abiertas en el periodo"
                  : "Abiertas"
            }
            to="#bandeja"
            etiquetaEnlace="Ir a resolución"
            recurso={conteoDeTipo(resumen, tipo)}
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
        {(alertas.tipo === "ausente" || alertas.tipo === "inactivo") && (
          <p className="muted">Sin datos todavía.</p>
        )}
        {resumen.tipo === "error" && alertas.tipo !== "error" && (
          <p className="tarjeta-error" role="alert">
            {resumen.mensaje}
          </p>
        )}
        {alertas.tipo === "listo" && sinEvaluar && (
          <p className="muted">Este periodo no se ha evaluado todavía.</p>
        )}
        {alertas.tipo === "listo" && !sinEvaluar && lista.length === 0 && (
          <p className="muted">
            {periodo
              ? "No hay alertas abiertas en este periodo."
              : "No hay alertas abiertas."}
          </p>
        )}
        {alertas.tipo === "listo" && !sinEvaluar && lista.length > 0 && (
          <>
            {resumen.tipo === "listo" &&
              lista.length < resumen.datos.abiertas && (
                <p className="muted">
                  Mostrando {formatearEntero(lista.length)} de{" "}
                  {formatearEntero(resumen.datos.abiertas)} alertas abiertas.
                </p>
              )}
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
                      <span className="badge">
                        {etiquetaDeTipo(alerta.tipo)}
                      </span>
                    </td>
                    <td>{alerta.detalle}</td>
                    <td>{alerta.referencia ?? alerta.periodo ?? "—"}</td>
                    <td>
                      {/*
                       * Ancla de solo-hash, como en `Tarjeta`: un `Link` con un
                       * `to` sin `search` descarta el `?periodo` de la URL y la
                       * bandeja se repuebla con otro periodo sin avisar.
                       */}
                      <a href="#bandeja">Resolver</a>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </>
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
