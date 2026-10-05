import { ArrowRightIcon } from "@heroicons/react/20/solid";
import { useEffect, useMemo, useState, type ReactElement } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { ApiError } from "../api";
import "../revision.css";
import { useSesion } from "../sesion";
import { formatearEntero } from "../tablero/formato";
import { useRecurso } from "../tablero/useDashboard";
import BarraApilada, { PALETA } from "../ui/BarraApilada";
import Cifra from "../ui/Cifra";
import Detalle from "../ui/Detalle";
import { TIPOS_DE_ALERTA, plural, puedeEvaluar } from "./anomalias";
import { evaluarPeriodo } from "./cliente";
import { ETIQUETA_CIRCUITO } from "./etapas";
import {
  Alerta,
  Proceso,
  ResumenDeAlertas,
  RUTAS_REPARTO,
  TipoDeAlerta,
} from "./tipos";

/** Cada tipo de alerta en palabras de quien reparte: que es y que hacer. */
const COPIA_DE_TIPO: Record<
  TipoDeAlerta,
  { titulo: string; significa: string; hacer: string }
> = {
  oni: {
    titulo: "Usos sin obra identificada",
    significa:
      "Un canal reportó un uso que no pudimos asociar a una obra del catálogo. Su parte queda en reserva.",
    hacer: "Identifícalo en la bandeja de Identificación o descártalo.",
  },
  duplicado_archivo: {
    titulo: "Archivo recibido dos veces",
    significa:
      "El mismo archivo llegó en dos entregas. Contarlo dos veces inflaría los puntos de sus obras.",
    hacer: "Revisa en Ingesta cuál entrega es la válida y retira la otra.",
  },
  duplicado_registro: {
    titulo: "Uso reportado dos veces",
    significa:
      "La misma emisión aparece en dos archivos del mismo periodo y desequilibra el reparto del canal.",
    hacer: "Confirma cuál registro vale antes de repartir.",
  },
  titular_sin_porcentaje: {
    titulo: "Autor sin porcentaje declarado",
    significa:
      "Un coautor del catálogo no tiene parte en la declaración vigente, o su parte no tiene IPI.",
    hacer: "Completa la declaración de la obra con su porcentaje e IPI.",
  },
  reserva_declaracion_incompleta: {
    titulo: "Declaración que no suma 100 %",
    significa:
      "Los porcentajes declarados no suman 100 %: el total de la obra queda en reserva y no se reparte.",
    hacer: "Pide a los titulares corregir la declaración de obra.",
  },
  tipo_obra_sin_mapear: {
    titulo: "Uso sin tipo de obra",
    significa:
      "Un uso llegó sin tipo de obra y el cálculo no sabe cómo ponderarlo.",
    hacer:
      "Asigna el tipo de obra: serie, telenovela, unitario, sketches o cine.",
  },
};

function tituloDeTipo(tipo: string): string {
  return COPIA_DE_TIPO[tipo as TipoDeAlerta]?.titulo ?? tipo;
}

/** Color por entidad, en el orden fijo de `TIPOS_DE_ALERTA`. */
function colorDeTipo(tipo: TipoDeAlerta): string {
  return PALETA[TIPOS_DE_ALERTA.indexOf(tipo) % PALETA.length];
}

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

  // Solo se pide con un periodo concreto: sin el, un conteo global quedaria
  // bajo una cabecera que no nombra ningun periodo.
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
    // No `revision`: styles.css la usa para el <dl> de afiliacion (grid 11rem 1fr).
    <section className="revision-pantalla anomalias">
      <header className="revision-cabecera">
        <div className="revision-titulo-fila">
          <h1>Anomalías</h1>
          <select
            className="pildora-select"
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
        </div>
      </header>

      {sinEvaluar && (
        <div role="status" className="revision-aviso">
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
            <p className="revision-aviso-error" role="alert">
              {evaluacion.error}
            </p>
          )}
        </div>
      )}

      {resumen.tipo === "cargando" && (
        <div
          className="panel esqueleto-caso"
          role="status"
          aria-label="Cargando el resumen de alertas"
        >
          <span className="esqueleto esqueleto-titulo" aria-hidden="true" />
          <span className="esqueleto esqueleto-barra" aria-hidden="true" />
        </div>
      )}

      {resumen.tipo === "listo" && !sinEvaluar && (
        <ResumenDeSeveridad resumen={resumen.datos} />
      )}

      <section className="anomalias-lista" aria-label="Alertas del periodo">
        <h2 className="panel-titulo">Alertas</h2>
        {alertas.tipo === "cargando" && (
          <div
            className="esqueletos"
            role="status"
            aria-label="Cargando alertas"
          >
            {[0, 1].map((i) => (
              <div key={i} className="esqueleto-caso" aria-hidden="true">
                <span className="esqueleto esqueleto-titulo" />
                <span className="esqueleto esqueleto-linea" />
              </div>
            ))}
          </div>
        )}
        {alertas.tipo === "error" && (
          <p className="revision-aviso-error" role="alert">
            {alertas.mensaje}
          </p>
        )}
        {procesos.tipo === "error" && (
          <p className="revision-aviso-error" role="alert">
            {procesos.mensaje}
          </p>
        )}
        {(alertas.tipo === "ausente" ||
          (alertas.tipo === "inactivo" && procesos.tipo !== "error")) && (
          <p className="revision-texto-suave">Sin datos todavía.</p>
        )}
        {resumen.tipo === "error" && alertas.tipo !== "error" && (
          <p className="revision-aviso-error" role="alert">
            {resumen.mensaje}
          </p>
        )}
        {alertas.tipo === "listo" && sinEvaluar && (
          <p className="revision-texto-suave">
            Este periodo no se ha evaluado todavía.
          </p>
        )}
        {alertas.tipo === "listo" &&
          resumen.tipo === "listo" &&
          !sinEvaluar &&
          lista.length === 0 && (
            <div className="revision-vacio">
              <span className="revision-vacio-icono">
                <IconoEscudo />
              </span>
              <p className="revision-vacio-titulo">
                Todo en orden: no hay alertas abiertas en este periodo
              </p>
            </div>
          )}
        {alertas.tipo === "listo" && !sinEvaluar && lista.length > 0 && (
          <>
            {resumen.tipo === "listo" &&
              lista.length < resumen.datos.abiertas && (
                <p className="revision-texto-suave">
                  Mostrando {formatearEntero(lista.length)} de{" "}
                  {formatearEntero(resumen.datos.abiertas)} alertas abiertas.
                </p>
              )}
            <ul className="alertas" aria-label="Alertas abiertas">
              {lista.map((alerta, i) => (
                <TarjetaDeAlerta key={alerta.id} alerta={alerta} indice={i} />
              ))}
            </ul>
          </>
        )}
      </section>

      {procesos.tipo === "listo" && procesos.datos.length > 0 && (
        <footer className="anomalias-pie">
          <p className="revision-texto-suave">
            Corridas del periodo:{" "}
            {procesos.datos
              .filter((p) => !periodo || p.periodo === periodo)
              .map((p) => ETIQUETA_CIRCUITO[p.circuito])
              .join(" · ") || "ninguna"}
          </p>
          <Link to="/distribucion" className="boton-secundario boton-enlace">
            Panel de corridas
            <ArrowRightIcon aria-hidden="true" />
          </Link>
        </footer>
      )}
    </section>
  );
}

/** Total abierto, barra por tipo y leyenda con lo que significa cada uno. */
function ResumenDeSeveridad({
  resumen,
}: {
  resumen: ResumenDeAlertas;
}): ReactElement {
  const bloquean = resumen.criticas_abiertas;
  return (
    <div className="panel anomalias-resumen">
      <div className="panel-cabecera">
        <div>
          <p className="anomalias-etiqueta">Alertas abiertas</p>
          <Cifra
            valor={String(resumen.abiertas)}
            formatear={(v) => formatearEntero(Number(v))}
          />
        </div>
        <span className={`chip ${bloquean > 0 ? "chip-error" : "chip-ok"}`}>
          {bloquean > 0
            ? `${formatearEntero(bloquean)} ${plural(bloquean, "bloquea", "bloquean")}`
            : "Nada bloquea"}
        </span>
      </div>
      <p className="anomalias-frase">
        {formatearEntero(bloquean)}{" "}
        {plural(
          bloquean,
          "crítica abierta bloquea",
          "críticas abiertas bloquean",
        )}{" "}
        el reparto de este periodo.
        {resumen.criticas_aceptadas > 0 &&
          ` · ${formatearEntero(resumen.criticas_aceptadas)} ${plural(
            resumen.criticas_aceptadas,
            "aceptada sin corregir",
            "aceptadas sin corregir",
          )}`}
      </p>
      {resumen.abiertas > 0 && (
        <BarraApilada
          etiqueta="Alertas por tipo"
          formatear={(v) => `${v} ${v === "1" ? "alerta" : "alertas"}`}
          segmentos={TIPOS_DE_ALERTA.map((tipo) => ({
            id: tipo,
            etiqueta: COPIA_DE_TIPO[tipo].titulo,
            valor: String(resumen.por_tipo[tipo].abiertas),
            color: colorDeTipo(tipo),
          }))}
        />
      )}
      <ul className="leyenda anomalias-leyenda" aria-label="Tipos de alerta">
        {TIPOS_DE_ALERTA.map((tipo) => {
          const conteo = resumen.por_tipo[tipo];
          const copia = COPIA_DE_TIPO[tipo];
          return (
            <li
              key={tipo}
              className={`leyenda-fila${conteo.abiertas === 0 ? " leyenda-fila-cero" : ""}`}
            >
              <span
                className="leyenda-punto"
                style={{ "--color": colorDeTipo(tipo) } as React.CSSProperties}
                aria-hidden="true"
              />
              <span className="anomalias-leyenda-nombre">
                <span>{copia.titulo}</span>
                {conteo.critica && (
                  <span className="chip chip-error">Bloquea el reparto</span>
                )}
                <Detalle
                  titulo={copia.titulo}
                  etiquetaDisparador={`Qué significa: ${copia.titulo}`}
                  claseDisparador="revision-icono-boton"
                  disparador={<IconoPregunta />}
                >
                  <span className="detalle-texto">{copia.significa}</span>
                  <span className="detalle-texto">
                    <strong>Qué hacer: </strong>
                    {copia.hacer}
                  </span>
                </Detalle>
              </span>
              <span className="leyenda-cifra">
                {formatearEntero(conteo.abiertas)}
              </span>
            </li>
          );
        })}
      </ul>
    </div>
  );
}

function TarjetaDeAlerta({
  alerta,
  indice,
}: {
  alerta: Alerta;
  indice: number;
}): ReactElement {
  const resuelta = alerta.resuelta || alerta.autocerrada;
  return (
    <li
      className={`alerta${resuelta ? " alerta-resuelta" : ""}`}
      style={{ "--indice": Math.min(indice, 8) } as React.CSSProperties}
    >
      <div className="alerta-cabecera">
        <span
          className="leyenda-punto"
          style={{ "--color": colorDeTipo(alerta.tipo) } as React.CSSProperties}
          aria-hidden="true"
        />
        <span className="alerta-tipo">{tituloDeTipo(alerta.tipo)}</span>
        {resuelta ? (
          <span className="chip chip-ok">Resuelta</span>
        ) : alerta.critica ? (
          <span className="chip chip-error">Bloquea el reparto</span>
        ) : (
          <span className="chip">No bloquea</span>
        )}
      </div>
      <p className="alerta-detalle">{alerta.detalle}</p>
      {alerta.tipo === "oni" && !resuelta && (
        <div className="alerta-acciones">
          <Link to="/identificacion" className="boton-primario boton-enlace">
            Identificar
            <ArrowRightIcon aria-hidden="true" />
          </Link>
        </div>
      )}
    </li>
  );
}

function IconoBase({ children }: { children: React.ReactNode }) {
  return (
    <svg
      viewBox="0 0 24 24"
      width="16"
      height="16"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
      focusable="false"
    >
      {children}
    </svg>
  );
}

const IconoPregunta = () => (
  <IconoBase>
    <circle cx="12" cy="12" r="9" />
    <path d="M9.5 9.5a2.5 2.5 0 1 1 3.5 2.3c-.6.3-1 .8-1 1.5v.2M12 17h.01" />
  </IconoBase>
);

const IconoEscudo = () => (
  <svg
    viewBox="0 0 24 24"
    width="28"
    height="28"
    fill="none"
    stroke="currentColor"
    strokeWidth="2"
    strokeLinecap="round"
    strokeLinejoin="round"
    aria-hidden="true"
    focusable="false"
  >
    <path d="M12 3l7 3v5c0 4.5-3 8.3-7 10-4-1.7-7-5.5-7-10V6l7-3z" />
    <path d="M8.5 12l2.5 2.5 4.5-4.5" />
  </svg>
);
