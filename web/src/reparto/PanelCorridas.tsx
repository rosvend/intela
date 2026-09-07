import { useEffect, useMemo, useState } from "react";
import { Link, useParams } from "react-router-dom";
import Cargando from "../Cargando";
import { ApiError } from "../api";
import { puedeVer } from "../navegacion";
import { useSesion } from "../sesion";
import { Tarjeta } from "../tablero/Tarjeta";
import { formatearEntero } from "../tablero/formato";
import { useRecurso } from "../tablero/useDashboard";
import {
  TIPOS_DE_ALERTA,
  conteoDeTipo,
  etiquetaDeTipo,
  totalPendientes,
} from "./anomalias";
import { firmarProceso } from "./cliente";
import Compuerta from "./Compuerta";
import Pipeline from "./Pipeline";
import { ETIQUETA_CIRCUITO, ETIQUETA_ETAPA } from "./etapas";
import { Alerta, INTERVALO_SONDEO_MS, Proceso, RUTAS_REPARTO } from "./tipos";

export default function PanelCorridas() {
  const { id } = useParams<{ id: string }>();
  const { usuario } = useSesion();
  const [recarga, setRecarga] = useState(0);
  const [enviando, setEnviando] = useState(false);
  const [errorDeFirma, setErrorDeFirma] = useState("");

  const procesos = useRecurso<Proceso[]>(RUTAS_REPARTO.procesos, true, recarga);

  useEffect(() => {
    const timer = window.setInterval(
      () => setRecarga((n) => n + 1),
      INTERVALO_SONDEO_MS,
    );
    return () => window.clearInterval(timer);
  }, []);

  const lista = useMemo(
    () => (procesos.tipo === "listo" ? procesos.datos : []),
    [procesos],
  );
  const seleccionado = useMemo(
    () => lista.find((proceso) => proceso.id === id) ?? lista[0],
    [lista, id],
  );

  const periodo = seleccionado?.periodo;
  const alertas = useRecurso<Alerta[]>(
    RUTAS_REPARTO.alertas(periodo),
    Boolean(periodo),
    recarga,
  );

  if (!usuario) return null;

  async function actuar(accion: "firmar" | "rechazar", motivo?: string) {
    if (!seleccionado) return;
    setEnviando(true);
    setErrorDeFirma("");
    try {
      await firmarProceso(seleccionado.id, { accion, motivo });
      setRecarga((n) => n + 1);
    } catch (error: unknown) {
      const mensaje =
        error instanceof ApiError
          ? error.message
          : "no se pudo registrar la firma";
      setErrorDeFirma(mensaje);
    } finally {
      setEnviando(false);
    }
  }

  return (
    <section className="tablero panel-corridas">
      <header className="tablero-cabecera">
        <div>
          <h1>Distribución</h1>
          <p className="muted">
            Estado de cada corrida por etapa. Nada se reparte con críticas
            abiertas.
          </p>
        </div>
      </header>

      {procesos.tipo === "cargando" && <Cargando texto="Cargando procesos…" />}
      {procesos.tipo === "error" && (
        <p className="tarjeta-error" role="alert">
          {procesos.mensaje}
        </p>
      )}
      {procesos.tipo === "ausente" && (
        <p className="muted">
          Sin datos todavía. El listado de procesos llega cuando el agregado de
          reparto esté expuesto.
        </p>
      )}
      {procesos.tipo === "listo" && lista.length === 0 && (
        <p className="muted">No hay procesos de reparto abiertos.</p>
      )}

      {procesos.tipo === "listo" && lista.length > 0 && seleccionado && (
        <div className="panel-corridas-cuerpo">
          <article className="tarjeta tarjeta-amplia">
            <h2 className="tarjeta-etiqueta">Corridas</h2>
            <table>
              <thead>
                <tr>
                  <th>Periodo</th>
                  <th>Circuito</th>
                  <th>Etapa</th>
                </tr>
              </thead>
              <tbody>
                {lista.map((proceso) => {
                  const activo = proceso.id === seleccionado.id;
                  return (
                    <tr
                      key={proceso.id}
                      className={activo ? "fila-activa" : undefined}
                    >
                      <td>
                        <Link
                          to={`/distribucion/${proceso.id}`}
                          aria-current={activo ? "true" : undefined}
                        >
                          {proceso.periodo}
                        </Link>
                      </td>
                      <td>{ETIQUETA_CIRCUITO[proceso.circuito]}</td>
                      <td>
                        <span className="badge">
                          {ETIQUETA_ETAPA[proceso.etapa]}
                        </span>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </article>

          <div className="panel-corridas-detalle">
            <article className="tarjeta tarjeta-amplia">
              <header className="tablero-cabecera">
                <div>
                  <h2>
                    {seleccionado.periodo} ·{" "}
                    {ETIQUETA_CIRCUITO[seleccionado.circuito]}
                  </h2>
                  <p className="muted">
                    Etapa actual: {ETIQUETA_ETAPA[seleccionado.etapa]}
                  </p>
                </div>
              </header>
              {seleccionado.rechazo && (
                <p className="login-aviso" role="status">
                  Último rechazo: {seleccionado.rechazo}
                </p>
              )}
              <Pipeline
                circuito={seleccionado.circuito}
                etapa={seleccionado.etapa}
              />
              <Compuerta
                proceso={seleccionado}
                rol={usuario.rol}
                enviando={enviando}
                error={errorDeFirma}
                onFirmar={() => void actuar("firmar")}
                onRechazar={(motivo) => void actuar("rechazar", motivo)}
              />
            </article>

            <section className="tablero-kpis">
              {TIPOS_DE_ALERTA.map((tipo) => (
                <Tarjeta
                  key={tipo}
                  titulo={etiquetaDeTipo(tipo)}
                  descripcion="Alertas abiertas del periodo"
                  to={
                    puedeVer(usuario.rol, "/anomalias")
                      ? `/anomalias?periodo=${encodeURIComponent(seleccionado.periodo)}`
                      : undefined
                  }
                  etiquetaEnlace="Ir a resolución"
                  recurso={conteoDeTipo(alertas, tipo)}
                >
                  {(datos) => (
                    <p className="tarjeta-valor">
                      {formatearEntero(datos.total)}
                    </p>
                  )}
                </Tarjeta>
              ))}
            </section>
            {alertas.tipo === "listo" &&
              totalPendientes(alertas.datos) > 0 &&
              puedeVer(usuario.rol, "/anomalias") && (
                <p>
                  <Link
                    to={`/anomalias?periodo=${encodeURIComponent(seleccionado.periodo)}`}
                  >
                    Hay {formatearEntero(totalPendientes(alertas.datos))}{" "}
                    alertas abiertas en este periodo. Ir a resolución.
                  </Link>
                </p>
              )}
          </div>
        </div>
      )}
    </section>
  );
}
