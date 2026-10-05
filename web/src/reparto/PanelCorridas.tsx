import {
  CSSProperties,
  Dispatch,
  SetStateAction,
  useEffect,
  useMemo,
  useState,
} from "react";
import { Link, useParams } from "react-router-dom";
import Cargando from "../Cargando";
import { ApiError, api } from "../api";
import { puedeVer } from "../navegacion";
import { Rol, useSesion } from "../sesion";
import { BotonEnlace } from "../tablero/BotonEnlace";
import { Tarjeta } from "../tablero/Tarjeta";
import { formatearEntero } from "../tablero/formato";
import { sumarImportes } from "../ui/dinero";
import { Bolsa, etiquetaDeFuente } from "../tablero/recaudo";
import { useRecurso } from "../tablero/useDashboard";
import BarraApilada, { colorDe } from "../ui/BarraApilada";
import Cifra from "../ui/Cifra";
import { aNumero, formatearCOP, formatearCOPCompacto } from "../ui/dinero";
import {
  TIPOS_DE_ALERTA,
  conteoDeTipo,
  advertenciaDeCompuerta,
  etiquetaDeTipo,
} from "./anomalias";
import { firmarProceso } from "./cliente";
import Compuerta from "./Compuerta";
import Pipeline, { EXPLICACION_ETAPA } from "./Pipeline";
import { ETIQUETA_CIRCUITO, ETIQUETA_ETAPA, etapasDe } from "./etapas";
import { esCompuerta, rolesPendientes } from "./firmas";
import "../staff.css";
import {
  Etapa,
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

/** Una corrida tal como la ve la pantalla: con su pagador y su bolsa. */
type Corrida = {
  proceso: Proceso;
  nombre: string;
  bruto?: string;
  color: string;
};

type Periodo = { periodo: string; corridas: Corrida[]; total?: string };

/**
 * Una corrida es una bolsa (ADR 0019): se nombra por quien pago, no por su
 * id. Dentro del periodo, la bolsa mayor primero; el color sigue a esa
 * posicion para que la barra y la lista hablen del mismo pagador.
 */
function agruparPorPeriodo(
  procesos: readonly Proceso[],
  bolsas: ReadonlyMap<string, Bolsa>,
): Periodo[] {
  const porPeriodo = new Map<string, Omit<Corrida, "color">[]>();
  for (const proceso of procesos) {
    const idBolsa = bolsaIdDe(proceso);
    const bolsa = idBolsa ? bolsas.get(idBolsa) : undefined;
    const corrida = {
      proceso,
      nombre: bolsa
        ? etiquetaDeFuente(bolsa.usuario_id)
        : `Corrida ${ETIQUETA_CIRCUITO[proceso.circuito].toLowerCase()}`,
      bruto: bolsa ? String(bolsa.bruto) : undefined,
    };
    porPeriodo.set(proceso.periodo, [
      ...(porPeriodo.get(proceso.periodo) ?? []),
      corrida,
    ]);
  }
  return [...porPeriodo.entries()]
    .sort(([a], [b]) => b.localeCompare(a))
    .map(([periodo, grupo]) => {
      const corridas = [...grupo]
        .sort(
          (a, b) =>
            aNumero(b.bruto ?? "-1") - aNumero(a.bruto ?? "-1") ||
            a.nombre.localeCompare(b.nombre, "es"),
        )
        .map((c, i) => ({ ...c, color: colorDe(i) }));
      const conBolsa = corridas.flatMap((c) => (c.bruto ? [c.bruto] : []));
      return {
        periodo,
        corridas,
        total: conBolsa.length > 0 ? sumarImportes(conBolsa) : undefined,
      };
    });
}

function enlaceDe(proceso: Proceso): string {
  return `/distribucion/${encodeURIComponent(proceso.id)}`;
}

function avanceDe(proceso: Proceso): { paso: number; total: number } {
  const pipeline = etapasDe(proceso.circuito);
  return {
    paso: Math.max(0, pipeline.indexOf(proceso.etapa)) + 1,
    total: pipeline.length,
  };
}

/** La etapa que sigue, o nada si la corrida ya llego a la terminal. */
function siguienteEtapa(proceso: Proceso): Etapa | undefined {
  const pipeline = etapasDe(proceso.circuito);
  const i = pipeline.indexOf(proceso.etapa);
  return i >= 0 ? pipeline[i + 1] : undefined;
}

/**
 * Solo administracion opera el pipeline (`x-required-roles` de avanzar). En
 * una compuerta, sin las dos firmas el backend responde 409: no se ofrece.
 */
function puedeAvanzar(rol: Rol, proceso: Proceso): boolean {
  if (rol !== "administrador" || !siguienteEtapa(proceso)) return false;
  return !esCompuerta(proceso.etapa) || rolesPendientes(proceso).length === 0;
}

async function avanzarProceso(id: string): Promise<void> {
  await api(`${RUTAS_REPARTO.proceso(id)}/avanzar`, { method: "POST" });
}

type Accion = { clave: string; enviando: boolean; error: string };
const SIN_ACCION: Accion = { clave: "", enviando: false, error: "" };

export default function PanelCorridas() {
  const { id } = useParams<{ id: string }>();
  const { usuario } = useSesion();
  const [recarga, setRecarga] = useState(0);
  const [firma, setFirma] = useState<Accion>(SIN_ACCION);
  const [avance, setAvance] = useState<Accion>(SIN_ACCION);
  const [confirmando, setConfirmando] = useState(false);

  const procesos = useRecurso<Proceso[]>(RUTAS_REPARTO.procesos, true, recarga);
  // Todo rol que ve /distribucion puede leer /bolsas (x-required-roles).
  const bolsas = useRecurso<Bolsa[]>("/api/bolsas");
  const bolsaPorId = useMemo(
    () =>
      new Map(
        bolsas.tipo === "listo" && Array.isArray(bolsas.datos)
          ? bolsas.datos.map((b) => [b.id, b] as const)
          : [],
      ),
    [bolsas],
  );

  useEffect(() => {
    const timer = window.setInterval(
      () => setRecarga((n) => n + 1),
      INTERVALO_SONDEO_MS,
    );
    return () => window.clearInterval(timer);
  }, []);

  const periodos = useMemo(
    () =>
      agruparPorPeriodo(
        procesos.tipo === "listo" ? procesos.datos : [],
        bolsaPorId,
      ),
    [procesos, bolsaPorId],
  );
  const todas = useMemo(() => periodos.flatMap((p) => p.corridas), [periodos]);
  const elegida = useMemo(
    () =>
      id === undefined
        ? todas[0]
        : todas.find((corrida) => corrida.proceso.id === id),
    [todas, id],
  );
  const seleccionado = elegida?.proceso;
  const grupo = periodos.find((p) => p.periodo === seleccionado?.periodo);

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
    // Un error de firma o de avance pertenece a la corrida/revision en la
    // que ocurrio: al cambiar de clave se descarta, no se revive al volver.
    setFirma(SIN_ACCION);
    setAvance(SIN_ACCION);
    setConfirmando(false);
  }, [clave]);

  const abiertas = resumen.tipo === "listo" ? resumen.datos.abiertas : 0;
  const enlaceAnomalias = `/anomalias?periodo=${encodeURIComponent(periodo ?? "")}`;

  if (!usuario) return null;

  async function ejecutar(
    estado: Accion,
    fijar: Dispatch<SetStateAction<Accion>>,
    fallo: string,
    llamada: (id: string) => Promise<void>,
  ) {
    if (!seleccionado || (estado.clave === clave && estado.enviando)) return;
    fijar({ clave, enviando: true, error: "" });
    try {
      await llamada(seleccionado.id);
      setRecarga((n) => n + 1);
    } catch (error: unknown) {
      const mensaje = error instanceof ApiError ? error.message : fallo;
      fijar((actual) =>
        actual.clave === clave ? { ...actual, error: mensaje } : actual,
      );
    } finally {
      fijar((actual) =>
        actual.clave === clave ? { ...actual, enviando: false } : actual,
      );
    }
  }

  const firmar = (accion: "firmar" | "rechazar", motivo?: string) =>
    ejecutar(firma, setFirma, "no se pudo registrar la firma", (pid) =>
      firmarProceso(pid, { accion, motivo }),
    );

  const avanzar = () => {
    setConfirmando(false);
    return ejecutar(
      avance,
      setAvance,
      "no se pudo avanzar la corrida",
      avanzarProceso,
    );
  };

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
      {procesos.tipo === "listo" && id === undefined && todas.length === 0 && (
        <p className="muted">No hay procesos de reparto abiertos.</p>
      )}

      {procesos.tipo === "listo" && elegida && seleccionado && grupo && (
        <div className="panel-corridas-cuerpo">
          <aside className="corridas-maestro">
            {periodos.length > 1 && (
              <nav className="corridas-periodos" aria-label="Periodos">
                {periodos.map((p) => (
                  <Link
                    key={p.periodo}
                    to={enlaceDe(p.corridas[0].proceso)}
                    className="corridas-periodo"
                    aria-current={
                      p.periodo === grupo.periodo ? "true" : undefined
                    }
                  >
                    {p.periodo}
                  </Link>
                ))}
              </nav>
            )}

            <section
              className="panel corridas-resumen"
              aria-label="Resumen del periodo"
            >
              <p className="corridas-rotulo">
                {grupo.total ? "Bolsa del periodo" : "Periodo"} {grupo.periodo}
              </p>
              {grupo.total && <Cifra valor={grupo.total} />}
              <p className="muted corridas-conteo">
                {formatearEntero(grupo.corridas.length)}{" "}
                {grupo.corridas.length === 1 ? "corrida" : "corridas"}, una por
                pagador
              </p>
              {grupo.total && (
                <BarraApilada
                  etiqueta="Bolsas por pagador"
                  segmentos={grupo.corridas.flatMap((c) =>
                    c.bruto
                      ? [
                          {
                            id: c.proceso.id,
                            etiqueta: c.nombre,
                            valor: c.bruto,
                            color: c.color,
                            detalle: (
                              <Link
                                className="corridas-ver"
                                to={enlaceDe(c.proceso)}
                                aria-label={`Ver corrida de ${c.nombre}`}
                              >
                                Ver corrida
                              </Link>
                            ),
                          },
                        ]
                      : [],
                  )}
                />
              )}

              <ul className="corridas-lista" aria-label="Corridas">
                {grupo.corridas.map((c, i) => (
                  <ItemDeCorrida
                    key={c.proceso.id}
                    corrida={c}
                    activa={c.proceso.id === seleccionado.id}
                    indice={i}
                  />
                ))}
              </ul>
            </section>
          </aside>

          <div className="panel-corridas-detalle">
            <section
              className="panel corrida-detalle"
              aria-label="Corrida seleccionada"
            >
              <header className="panel-cabecera corrida-cabecera">
                <div>
                  <h2 className="panel-titulo corrida-titulo">
                    {elegida.nombre}
                  </h2>
                  <p className="muted">
                    Periodo {seleccionado.periodo} · Circuito{" "}
                    {ETIQUETA_CIRCUITO[seleccionado.circuito].toLowerCase()}
                  </p>
                </div>
                <span
                  className={`chip ${esCompuerta(seleccionado.etapa) ? "chip-alerta" : "chip-marca"}`}
                >
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
              <CifrasDeCorrida corrida={elegida} total={grupo.total} />
            </section>

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
              onFirmar={() => void firmar("firmar")}
              onRechazar={(motivo) => void firmar("rechazar", motivo)}
            />

            {puedeAvanzar(usuario.rol, seleccionado) && (
              <AccionAvanzar
                siguiente={siguienteEtapa(seleccionado)}
                confirmando={confirmando}
                enviando={avance.clave === clave && avance.enviando}
                error={avance.clave === clave ? avance.error : ""}
                onPedir={() => setConfirmando(true)}
                onCancelar={() => setConfirmando(false)}
                onConfirmar={() => void avanzar()}
              />
            )}

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

function ItemDeCorrida({
  corrida,
  activa,
  indice,
}: {
  corrida: Corrida;
  activa: boolean;
  indice: number;
}) {
  const { proceso, nombre, bruto, color } = corrida;
  const { paso, total } = avanceDe(proceso);
  return (
    <li
      className={`corrida-item${activa ? " corrida-item-activa" : ""}`}
      style={
        {
          "--color": color,
          animationDelay: `${indice * 40}ms`,
        } as CSSProperties
      }
    >
      <Link
        className="corrida-enlace"
        to={enlaceDe(proceso)}
        aria-current={activa ? "true" : undefined}
      >
        <span className="corrida-punto" aria-hidden="true" />
        <span className="corrida-nombre">
          <span>{nombre}</span>
          <span
            className={`corrida-etapa${esCompuerta(proceso.etapa) ? " corrida-etapa-firma" : ""}`}
          >
            {ETIQUETA_CIRCUITO[proceso.circuito]} ·{" "}
            {ETIQUETA_ETAPA[proceso.etapa]}
          </span>
        </span>
        <span className="corrida-lado">
          {bruto && (
            <span className="corrida-monto" title={formatearCOP(bruto)}>
              {formatearCOPCompacto(bruto)}
            </span>
          )}
          <span
            className="corrida-avance"
            role="img"
            aria-label={`Etapa ${paso} de ${total}`}
            style={{ "--avance": paso / total } as CSSProperties}
          >
            <span className="corrida-avance-pista" aria-hidden="true" />
            <span aria-hidden="true">
              {paso}/{total}
            </span>
          </span>
        </span>
      </Link>
    </li>
  );
}

/**
 * Solo lo que la API sostiene para una corrida: la bolsa bruta y su peso
 * en el periodo. Las deducciones y el neto por repartir no se exponen por
 * proceso todavia (`Proceso` no los trae), asi que no se muestran.
 */
function CifrasDeCorrida({
  corrida,
  total,
}: {
  corrida: Corrida;
  total?: string;
}) {
  const { paso, total: pasos } = avanceDe(corrida.proceso);
  const peso =
    corrida.bruto && total && aNumero(total) > 0
      ? Math.round((aNumero(corrida.bruto) / aNumero(total)) * 1000) / 10
      : undefined;
  return (
    <ul className="corrida-cifras" aria-label="Cifras de la corrida">
      {corrida.bruto && (
        <li>
          <span className="corrida-cifras-rotulo">Bolsa bruta</span>
          <span className="corrida-cifras-valor">
            {formatearCOP(corrida.bruto)}
          </span>
        </li>
      )}
      {peso !== undefined && (
        <li>
          <span className="corrida-cifras-rotulo">Peso en el periodo</span>
          <span className="corrida-cifras-valor">
            {peso.toLocaleString("es-CO")} %
          </span>
        </li>
      )}
      <li>
        <span className="corrida-cifras-rotulo">Etapa</span>
        <span className="corrida-cifras-valor">
          {paso} de {pasos}
        </span>
      </li>
    </ul>
  );
}

function AccionAvanzar({
  siguiente,
  confirmando,
  enviando,
  error,
  onPedir,
  onCancelar,
  onConfirmar,
}: {
  siguiente?: Etapa;
  confirmando: boolean;
  enviando: boolean;
  error: string;
  onPedir: () => void;
  onCancelar: () => void;
  onConfirmar: () => void;
}) {
  if (!siguiente) return null;
  const destino = ETIQUETA_ETAPA[siguiente];
  return (
    <section className="panel corrida-avanzar" aria-label="Avanzar corrida">
      <div className="corrida-avanzar-texto">
        <h3 className="panel-titulo">Siguiente etapa: {destino}</h3>
        <p className="muted">{EXPLICACION_ETAPA[siguiente]}</p>
      </div>
      <div className="corrida-avanzar-acciones">
        {confirmando ? (
          <>
            <button
              type="button"
              className="boton-primario"
              disabled={enviando}
              onClick={onConfirmar}
            >
              Confirmar avance
            </button>
            <button
              type="button"
              className="boton-secundario"
              disabled={enviando}
              onClick={onCancelar}
            >
              Cancelar
            </button>
          </>
        ) : (
          <button
            type="button"
            className="boton-primario"
            disabled={enviando}
            onClick={onPedir}
          >
            {enviando ? "Avanzando…" : `Avanzar a ${destino}`}
          </button>
        )}
      </div>
      {error && (
        <p className="tarjeta-error corrida-avanzar-error" role="alert">
          {error}
        </p>
      )}
    </section>
  );
}
