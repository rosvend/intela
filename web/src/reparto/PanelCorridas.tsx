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
  const [firma, setFirma] = useState({ clave: "", enviando: false, error: "" });

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
    () =>
      id === undefined ? lista[0] : lista.find((proceso) => proceso.id === id),
    [lista, id],
  );

  const clave = seleccionado
    ? `${seleccionado.id}:${seleccionado.revision}:${seleccionado.etapa}`
    : "";
  const periodo = seleccionado?.periodo;
  // Solo quien puede ver /anomalias lee /api/alertas. Contabilidad firma y
  // tambien llega a /anomalias (para ver el aviso antes de firmar); pedirlas
  // con un rol sin acceso dejaria las cinco tarjetas en rojo con el 403.
  const verAnomalias = usuario ? puedeVer(usuario.rol, "/anomalias") : false;
  const alertas = useRecurso<Alerta[]>(
    RUTAS_REPARTO.alertas(periodo),
    Boolean(periodo) && verAnomalias,
    recarga,
  );

  useEffect(() => {
    // Un error de firma pertenece a la corrida/revision en la que ocurrio:
    // al cambiar de clave se descarta, no se revive al volver.
    setFirma({ clave: "", enviando: false, error: "" });
  }, [clave]);

  const abiertas =
    alertas.tipo === "listo" ? totalPendientes(alertas.datos) : 0;
  const enlaceAnomalias = `/anomalias?periodo=${encodeURIComponent(periodo ?? "")}`;

  if (!usuario) return null;

  async function actuar(accion: "firmar" | "rechazar", motivo?: string) {
    if (!seleccionado || (firma.clave === clave && firma.enviando)) return;
    setFirma({ clave, enviando: true, error: "" });
    try {
      await firmarProceso(seleccionado.id, { accion, motivo });
      setRecarga((n) => n + 1);
    } catch (error: unknown) {
      const mensaje =
        error instanceof ApiError
          ? error.message
          : "no se pudo registrar la firma";
      setFirma((actual) =>
        actual.clave === clave ? { ...actual, error: mensaje } : actual,
      );
    } finally {
      setFirma((actual) =>
        actual.clave === clave ? { ...actual, enviando: false } : actual,
      );
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
      {procesos.tipo === "listo" && id !== undefined && !seleccionado && (
        <p role="alert">Proceso de reparto no encontrado.</p>
      )}
      {procesos.tipo === "listo" && id === undefined && lista.length === 0 && (
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
                          to={`/distribucion/${encodeURIComponent(proceso.id)}`}
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
                key={clave}
                proceso={seleccionado}
                rol={usuario.rol}
                advertencia={
                  abiertas > 0
                    ? `Revisa las ${formatearEntero(abiertas)} alertas abiertas del periodo antes de firmar.`
                    : ""
                }
                enviando={firma.clave === clave && firma.enviando}
                error={firma.clave === clave ? firma.error : ""}
                onFirmar={() => void actuar("firmar")}
                onRechazar={(motivo) => void actuar("rechazar", motivo)}
              />
            </article>

            {/*
             * Sin acceso a /anomalias no se piden las alertas, asi que las
             * tarjetas solo podrian quedar vacias para siempre: se omiten en
             * vez de prometer un conteo que este rol nunca va a ver.
             */}
            {verAnomalias && (
              <>
                <section className="tablero-kpis">
                  {TIPOS_DE_ALERTA.map((tipo) => (
                    <Tarjeta
                      key={tipo}
                      titulo={etiquetaDeTipo(tipo)}
                      descripcion="Alertas abiertas del periodo"
                      to={enlaceAnomalias}
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
                {abiertas > 0 && (
                  <p>
                    <Link to={enlaceAnomalias}>
                      Hay {formatearEntero(abiertas)} alertas abiertas en este
                      periodo. Ir a resolución.
                    </Link>
                  </p>
                )}
              </>
            )}
          </div>
        </div>
      )}
    </section>
  );
}
