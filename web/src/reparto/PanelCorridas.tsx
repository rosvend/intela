import { useEffect, useMemo, useState } from "react";
import { Link, useParams } from "react-router-dom";
import Cargando from "../Cargando";
import { ApiError } from "../api";
import { puedeVer } from "../navegacion";
import { useSesion } from "../sesion";
import { BotonEnlace } from "../tablero/BotonEnlace";
import { Tarjeta } from "../tablero/Tarjeta";
import { formatearEntero } from "../tablero/formato";
import { useRecurso } from "../tablero/useDashboard";
import {
  TIPOS_DE_ALERTA,
  conteoDeTipo,
  advertenciaDeCompuerta,
  etiquetaDeTipo,
} from "./anomalias";
import { firmarProceso } from "./cliente";
import Compuerta from "./Compuerta";
import Pipeline from "./Pipeline";
import { ETIQUETA_CIRCUITO, ETIQUETA_ETAPA } from "./etapas";
import { esCompuerta } from "./firmas";
import { Bolsa } from "../tablero/recaudo";
import { formatearCOP, formatearCOPCompacto } from "../ui/dinero";
import "../staff.css";
import {
  INTERVALO_SONDEO_MS,
  Proceso,
  ResumenDeAlertas,
  RUTAS_REPARTO,
} from "./tipos";

/** El contrato trae `bolsa_id`; el tipo local todavia no lo declara. */
function bolsaIdDe(p: Proceso): string | undefined {
  const id = (p as Proceso & { bolsa_id?: unknown }).bolsa_id;
  return typeof id === "string" && id !== "" ? id : undefined;
}

export default function PanelCorridas() {
  const { id } = useParams<{ id: string }>();
  const { usuario } = useSesion();
  const [recarga, setRecarga] = useState(0);
  const [firma, setFirma] = useState({ clave: "", enviando: false, error: "" });

  const procesos = useRecurso<Proceso[]>(RUTAS_REPARTO.procesos, true, recarga);
  // Todo rol que ve /distribucion puede leer /bolsas (x-required-roles).
  const bolsas = useRecurso<Bolsa[]>("/api/bolsas");
  const brutoPorBolsa = useMemo(
    () =>
      new Map(
        bolsas.tipo === "listo" && Array.isArray(bolsas.datos)
          ? bolsas.datos.map((b) => [b.id, String(b.bruto)] as const)
          : [],
      ),
    [bolsas],
  );
  const brutoDe = (p: Proceso) => {
    const idBolsa = bolsaIdDe(p);
    return idBolsa ? brutoPorBolsa.get(idBolsa) : undefined;
  };

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
  // con un rol sin acceso dejaria las seis tarjetas en rojo con el 403.
  const verAnomalias = usuario ? puedeVer(usuario.rol, "/anomalias") : false;
  const resumen = useRecurso<ResumenDeAlertas>(
    RUTAS_REPARTO.resumenAlertas(periodo ?? ""),
    Boolean(periodo) && verAnomalias,
    recarga,
  );

  useEffect(() => {
    // Un error de firma pertenece a la corrida/revision en la que ocurrio:
    // al cambiar de clave se descarta, no se revive al volver.
    setFirma({ clave: "", enviando: false, error: "" });
  }, [clave]);

  const abiertas = resumen.tipo === "listo" ? resumen.datos.abiertas : 0;
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
    <section className="tablero panel-corridas staff">
      <header className="tablero-cabecera">
        <h1>Distribución</h1>
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
          <ul className="corridas-lista" aria-label="Corridas">
            {lista.map((proceso, i) => {
              const activo = proceso.id === seleccionado.id;
              const bruto = brutoDe(proceso);
              return (
                <li
                  key={proceso.id}
                  className={`corrida-tarjeta${activo ? " corrida-tarjeta-activa" : ""}`}
                  style={{ animationDelay: `${i * 50}ms` }}
                >
                  <Link
                    className="corrida-periodo"
                    to={`/distribucion/${encodeURIComponent(proceso.id)}`}
                    aria-current={activo ? "true" : undefined}
                  >
                    {proceso.periodo}
                  </Link>
                  <span className="corrida-circuito">
                    {ETIQUETA_CIRCUITO[proceso.circuito]}
                  </span>
                  <span
                    className={`chip ${esCompuerta(proceso.etapa) ? "chip-alerta" : "chip-marca"}`}
                  >
                    {ETIQUETA_ETAPA[proceso.etapa]}
                  </span>
                  {bruto && (
                    <span className="corrida-bolsa" title={formatearCOP(bruto)}>
                      {formatearCOPCompacto(bruto)}
                    </span>
                  )}
                </li>
              );
            })}
          </ul>

          <div className="panel-corridas-detalle">
            <section className="panel">
              <header className="panel-cabecera corrida-cabecera">
                <div>
                  <h2 className="panel-titulo corrida-titulo">
                    Periodo {seleccionado.periodo}
                  </h2>
                  <p className="muted">
                    Circuito{" "}
                    {ETIQUETA_CIRCUITO[seleccionado.circuito].toLowerCase()}
                    {brutoDe(seleccionado) &&
                      ` · bolsa de ${formatearCOP(brutoDe(seleccionado) ?? "")}`}
                  </p>
                </div>
                <span className="chip chip-marca">
                  {ETIQUETA_ETAPA[seleccionado.etapa]}
                </span>
              </header>
              {seleccionado.rechazo && (
                <p className="corrida-rechazo" role="status">
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
                  resumen.tipo === "listo"
                    ? advertenciaDeCompuerta(resumen.datos)
                    : resumen.tipo === "error"
                      ? "No se pudo leer el estado de anomalías del periodo. No firmes sin revisarlo."
                      : ""
                }
                enviando={firma.clave === clave && firma.enviando}
                error={firma.clave === clave ? firma.error : ""}
                onFirmar={() => void actuar("firmar")}
                onRechazar={(motivo) => void actuar("rechazar", motivo)}
              />
            </section>

            {/*
             * Sin acceso a /anomalias no se piden las alertas, asi que las
             * tarjetas solo podrian quedar vacias para siempre: se omiten en
             * vez de prometer un conteo que este rol nunca va a ver.
             */}
            {verAnomalias && (
              <section
                className="corrida-alertas"
                aria-label="Alertas del periodo"
              >
                {abiertas > 0 && (
                  <div className="corrida-alertas-aviso">
                    <span>
                      {formatearEntero(abiertas)} alertas abiertas en este
                      periodo
                    </span>
                    <BotonEnlace to={enlaceAnomalias}>
                      Resolver alertas
                    </BotonEnlace>
                  </div>
                )}
                <div className="corrida-alertas-rejilla">
                  {TIPOS_DE_ALERTA.map((tipo) => (
                    <Tarjeta
                      key={tipo}
                      className="tarjeta-mini"
                      titulo={etiquetaDeTipo(tipo)}
                      recurso={conteoDeTipo(resumen, tipo)}
                    >
                      {(datos) => (
                        <p className="tarjeta-valor">
                          {formatearEntero(datos.total)}
                        </p>
                      )}
                    </Tarjeta>
                  ))}
                </div>
              </section>
            )}
          </div>
        </div>
      )}
    </section>
  );
}
