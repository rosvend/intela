import { ArrowRightIcon } from "@heroicons/react/20/solid";
import { useEffect, useId, useState, type ReactElement } from "react";
import { Link, useSearchParams } from "react-router-dom";
import Paginador from "../catalogo/Paginador";
import "../revision.css";
import { formatearInstante } from "../tablero/formato";
import BarraApilada from "../ui/BarraApilada";
import { useApi } from "../useApi";
import { EsqueletoDeCasos, EstadoVacio } from "./BandejaIdentificacion";
import Dialogo from "./Dialogo";
import {
  contarPorEstado,
  formatearPeriodo,
  nombreDeFuente,
} from "./presentacion";
import {
  ESTADOS_DE_CASO,
  ETIQUETA_ESTADO,
  LIMITE_LISTA_ONI,
  RUTAS_IDENTIFICACION,
  esPaginaDeCasos,
  type CasoIdentificacion,
  type EstadoDeCaso,
  type FiltrosDeCasos,
} from "./tipos";

/**
 * Lista ONI (`/lista-oni`, plano de #39, D4/D5): todos los casos -pendientes,
 * asignados y descartados-, con sus filtros en la URL y el historial de cada
 * registro.
 *
 * A diferencia de la bandeja (D2, siempre desde el principio de la cola), aqui
 * SI se pagina por desplazamiento: esta pantalla no resuelve nada, asi que la
 * cola no se vacia bajo los pies de quien navega entre paginas.
 *
 * El `estado`, `fuente` y `periodo` de la URL se mandan TAL CUAL al servidor
 * (`GET /identificacion/casos`), que es quien filtra: el cliente no vuelve a
 * filtrar una pagina que el servidor ya filtro.
 *
 * DISCREPANCIA con el plano: D4 describe la URL como
 * `?estado=&fuente=&periodo=&desde=`, pero `GET /identificacion/casos`
 * (api/openapi.yaml, contrato de #174) no declara ningun parametro `desde` -
 * solo `estado`, `fuente`, `periodo`, `limite` y `desplazamiento`. Inventar un
 * filtro por fecha que el servidor no sabe aplicar seria, o bien mandar un
 * parametro que el servidor ignora en silencio (el filtro parece aplicado y no
 * lo esta), o filtrar en el cliente sobre una sola pagina (un filtro que
 * contradice lo que el resto de la pantalla promete: "aplicados por el
 * servidor"). Las dos cambian el comportamiento, asi que no se implementa: no
 * hay filtro por fecha en esta pantalla. Avisar a quien mantiene el plano.
 */
export default function ListaOni(): ReactElement {
  const [recarga, setRecarga] = useState(0);
  return (
    <Contenido key={recarga} onRecargar={() => setRecarga((r) => r + 1)} />
  );
}

function leerEstado(bruto: string | null): EstadoDeCaso | "" {
  return bruto !== null && (ESTADOS_DE_CASO as string[]).includes(bruto)
    ? (bruto as EstadoDeCaso)
    : "";
}

/** Mismo criterio que `catalogo/Catalogo.tsx`: un valor invalido es la primera pagina. */
function leerDesplazamiento(bruto: string | null): number {
  const n = Number(bruto);
  return Number.isInteger(n) && n > 0 ? n : 0;
}

/**
 * Las opciones de un selector acumulado (D4): lo visto en las paginas
 * cargadas en esta sesion, mas el valor actualmente seleccionado -aunque no
 * este entre lo visto-, para que cambiar de pagina o de otro filtro no le
 * borre a quien mira la opcion que ya habia elegido. Orden alfabetico es-CO:
 * un orden estable es lo que hace que la lista no salte de sitio cada vez que
 * llega una pagina nueva.
 */
function opcionesAcumuladas(
  vistas: ReadonlySet<string>,
  seleccionada: string,
): string[] {
  const conjunto = new Set(vistas);
  if (seleccionada !== "") conjunto.add(seleccionada);
  return Array.from(conjunto).sort((a, b) => a.localeCompare(b, "es"));
}

function Contenido({ onRecargar }: { onRecargar: () => void }): ReactElement {
  const [searchParams, setSearchParams] = useSearchParams();

  const estado = leerEstado(searchParams.get("estado"));
  const fuente = searchParams.get("fuente") ?? "";
  const periodo = searchParams.get("periodo") ?? "";
  const desplazamiento = leerDesplazamiento(searchParams.get("desplazamiento"));
  const hayFiltros = estado !== "" || fuente !== "" || periodo !== "";

  const filtros: FiltrosDeCasos = { limite: LIMITE_LISTA_ONI, desplazamiento };
  if (estado !== "") filtros.estado = estado;
  if (fuente !== "") filtros.fuente = fuente;
  if (periodo !== "") filtros.periodo = periodo;

  const lectura = useApi<unknown>(RUTAS_IDENTIFICACION.casos(filtros));
  const pagina =
    !lectura.cargando && !lectura.error && esPaginaDeCasos(lectura.datos)
      ? lectura.datos
      : null;

  // Acumulados de fuente y periodo (D4): lo que trajeron las paginas vistas
  // EN ESTA SESION de la pantalla, no el universo entero de valores posibles
  // -no hay un endpoint que los liste-. Un `Set` por filtro, alimentado cada
  // vez que llega una pagina legible.
  const [fuentesVistas, setFuentesVistas] = useState<Set<string>>(new Set());
  const [periodosVistos, setPeriodosVistos] = useState<Set<string>>(new Set());

  useEffect(() => {
    if (!pagina) return;
    setFuentesVistas((previas) => {
      const siguientes = new Set(previas);
      for (const caso of pagina.casos) siguientes.add(caso.fuente);
      return siguientes;
    });
    setPeriodosVistos((previas) => {
      const siguientes = new Set(previas);
      for (const caso of pagina.casos) siguientes.add(caso.periodo);
      return siguientes;
    });
  }, [pagina]);

  // Los conteos por estado salen de la ultima pagina SIN filtro de estado con
  // la misma fuente, periodo y desplazamiento: el contrato no trae totales
  // por estado, y al filtrar por uno los demas no deben caer a cero.
  const claveDeConteo = `${fuente}|${periodo}|${desplazamiento}`;
  const [conteos, setConteos] = useState<{
    clave: string;
    porEstado: Record<EstadoDeCaso, number>;
    total: number;
  } | null>(null);
  useEffect(() => {
    if (!pagina || estado !== "") return;
    setConteos({
      clave: claveDeConteo,
      porEstado: contarPorEstado(pagina.casos),
      total: pagina.casos.length,
    });
  }, [pagina, estado, claveDeConteo]);
  const conteosVigentes = conteos?.clave === claveDeConteo ? conteos : null;

  const opcionesDeFuente = opcionesAcumuladas(fuentesVistas, fuente);
  const opcionesDePeriodo = opcionesAcumuladas(periodosVistos, periodo);

  /**
   * Pone un filtro en la URL y vuelve a la primera pagina: seguir en un
   * desplazamiento que ya no corresponde a la nueva consulta deja la pantalla
   * vacia por una razon que nada en pantalla explica (mismo criterio que
   * `Catalogo.tsx`). `replace` para no llenar el historial con una entrada
   * por cada cambio de selector.
   */
  function aplicarFiltro(
    clave: "estado" | "fuente" | "periodo",
    valor: string,
  ) {
    setSearchParams(
      (previos) => {
        const siguientes = new URLSearchParams(previos);
        if (valor !== "") siguientes.set(clave, valor);
        else siguientes.delete(clave);
        siguientes.delete("desplazamiento");
        return siguientes;
      },
      { replace: true },
    );
  }

  function limpiarFiltros() {
    setSearchParams(new URLSearchParams(), { replace: true });
  }

  /** Mueve la pagina. Aqui si se empuja al historial: "atras" vuelve a la anterior. */
  function irA(nuevoDesplazamiento: number) {
    setSearchParams((previos) => {
      const siguientes = new URLSearchParams(previos);
      if (nuevoDesplazamiento > 0) {
        siguientes.set("desplazamiento", String(nuevoDesplazamiento));
      } else {
        siguientes.delete("desplazamiento");
      }
      return siguientes;
    });
  }

  // El historial del registro (D5, D6): un solo `Dialogo` centrado para toda
  // la pantalla, con el caso sobre el que se abrio. Vive aqui y no en cada
  // fila -como el panel de la bandeja vive en `BandejaIdentificacion`- para
  // que solo exista un dialogo en el DOM a la vez.
  const [historial, setHistorial] = useState<CasoIdentificacion | null>(null);
  const idTituloHistorial = useId();

  return (
    <section className="lista-oni revision-pantalla">
      <header className="revision-cabecera">
        <div className="revision-titulo-fila">
          <h1>Lista ONI</h1>
          <Link to="/identificacion" className="boton-secundario boton-enlace">
            Ir a casos pendientes
            <ArrowRightIcon aria-hidden="true" />
          </Link>
        </div>
      </header>

      <div className="panel oni-resumen">
        <div className="oni-chips" role="group" aria-label="Filtrar por estado">
          <ChipDeEstado
            etiqueta="Todos"
            cuenta={conteosVigentes?.total}
            activo={estado === ""}
            onClick={() => aplicarFiltro("estado", "")}
          />
          {ESTADOS_DE_CASO.map((valor) => (
            <ChipDeEstado
              key={valor}
              etiqueta={ETIQUETA_ESTADO[valor]}
              cuenta={conteosVigentes?.porEstado[valor]}
              color={COLOR_ESTADO[valor]}
              activo={estado === valor}
              onClick={() => aplicarFiltro("estado", valor)}
            />
          ))}
        </div>
        {conteosVigentes && conteosVigentes.total > 0 && (
          <div className="oni-distribucion">
            <BarraApilada
              etiqueta="Distribución por estado"
              formatear={(v) => `${v} ${v === "1" ? "registro" : "registros"}`}
              segmentos={ESTADOS_DE_CASO.map((valor) => ({
                id: valor,
                etiqueta: ETIQUETA_ESTADO[valor],
                valor: String(conteosVigentes.porEstado[valor]),
                color: COLOR_ESTADO[valor],
              }))}
            />
            {(desplazamiento > 0 ||
              conteosVigentes.total >= LIMITE_LISTA_ONI) && (
              <p className="oni-nota">Conteo de esta página.</p>
            )}
          </div>
        )}
        <div className="oni-filtros">
          <select
            className="pildora-select"
            aria-label="Filtrar por fuente"
            value={fuente}
            onChange={(e) => aplicarFiltro("fuente", e.target.value)}
          >
            <option value="">Todas las fuentes</option>
            {opcionesDeFuente.map((valor) => (
              <option key={valor} value={valor}>
                {nombreDeFuente(valor)}
              </option>
            ))}
          </select>
          <select
            className="pildora-select"
            aria-label="Filtrar por periodo"
            value={periodo}
            onChange={(e) => aplicarFiltro("periodo", e.target.value)}
          >
            <option value="">Todos los periodos</option>
            {opcionesDePeriodo.map((valor) => (
              <option key={valor} value={valor}>
                {formatearPeriodo(valor)}
              </option>
            ))}
          </select>
          {hayFiltros && (
            <button
              type="button"
              className="boton-fantasma"
              onClick={limpiarFiltros}
            >
              Limpiar filtros
            </button>
          )}
        </div>
      </div>

      {lectura.cargando && (
        <EsqueletoDeCasos etiqueta="Cargando la lista ONI" />
      )}

      {!lectura.cargando && lectura.error && (
        <div className="revision-aviso-error" role="alert">
          <p>No pudimos cargar la lista ONI: {lectura.error.message}</p>
          <button
            type="button"
            className="boton-secundario"
            onClick={onRecargar}
          >
            Intentar de nuevo
          </button>
        </div>
      )}

      {!lectura.cargando && !lectura.error && pagina === null && (
        <p className="revision-aviso-error" role="alert">
          La lista ONI no llegó como una página de casos legible.
        </p>
      )}

      {pagina !== null && (
        <>
          {pagina.casos.length === 0 ? (
            <VaciaListaOni conFiltros={hayFiltros} />
          ) : (
            <div className="panel oni-tabla-caja">
              <table className="oni-tabla" aria-label="Lista ONI">
                <thead>
                  <tr>
                    <th scope="col">Uso reportado</th>
                    <th scope="col">Estado</th>
                    <th scope="col">Responsable</th>
                    <th scope="col">Actualizado</th>
                    <th scope="col">
                      <span className="solo-lector">Acciones</span>
                    </th>
                  </tr>
                </thead>
                <tbody>
                  {pagina.casos.map((caso) => (
                    <FilaListaOni
                      key={caso.id}
                      caso={caso}
                      onVerHistorial={() => setHistorial(caso)}
                    />
                  ))}
                </tbody>
              </table>
              <div className="oni-pie">
                <Paginador
                  etiqueta="Registros"
                  desplazamiento={desplazamiento}
                  cuantas={pagina.casos.length}
                  limite={LIMITE_LISTA_ONI}
                  onIrA={irA}
                />
              </div>
            </div>
          )}
        </>
      )}

      <Dialogo
        abierto={historial !== null}
        variante="centrado"
        idTitulo={idTituloHistorial}
        onCerrar={() => setHistorial(null)}
      >
        {historial && (
          <HistorialDelRegistro
            idTitulo={idTituloHistorial}
            caso={historial}
            onCerrar={() => setHistorial(null)}
          />
        )}
      </Dialogo>
    </section>
  );
}

/** Color de estado: es un estado, asi que sale de los tokens de estado. */
const COLOR_ESTADO: Record<EstadoDeCaso, string> = {
  pendiente: "var(--alerta)",
  asignado: "var(--ok)",
  descartado: "var(--texto-suave)",
};

function ChipDeEstado({
  etiqueta,
  cuenta,
  color,
  activo,
  onClick,
}: {
  etiqueta: string;
  cuenta: number | undefined;
  color?: string;
  activo: boolean;
  onClick: () => void;
}): ReactElement {
  return (
    <button
      type="button"
      className="chip-filtro"
      aria-pressed={activo}
      aria-label={cuenta === undefined ? etiqueta : `${etiqueta}: ${cuenta}`}
      onClick={onClick}
    >
      {color && (
        <span
          className="chip-filtro-punto"
          style={{ background: color }}
          aria-hidden="true"
        />
      )}
      {etiqueta}
      {cuenta !== undefined && (
        <span className="chip-filtro-cuenta">{cuenta}</span>
      )}
    </button>
  );
}

/** Sin filtros la lista misma esta vacia; con filtros, solo ESOS no encontraron nada. */
function VaciaListaOni({ conFiltros }: { conFiltros: boolean }): ReactElement {
  return conFiltros ? (
    <EstadoVacio
      titulo="No hay coincidencias"
      texto="Ajusta los filtros para consultar otros registros ONI."
    />
  ) : (
    <EstadoVacio
      titulo="No hay registros ONI"
      texto="Ningún uso ha necesitado revisión todavía."
    />
  );
}

function EtiquetaDeEstadoOni({
  estado,
}: {
  estado: EstadoDeCaso;
}): ReactElement {
  return (
    <span className={`chip ${CLASE_CHIP_ESTADO[estado]}`}>
      {ETIQUETA_ESTADO[estado]}
    </span>
  );
}

const CLASE_CHIP_ESTADO: Record<EstadoDeCaso, string> = {
  pendiente: "chip-alerta",
  asignado: "chip-ok",
  descartado: "",
};

/**
 * Una fila. "Resolver" solo si sigue pendiente, y navega a la bandeja
 * (DECISION #39): la remocion optimista vive alli y no se duplica aqui.
 */
function FilaListaOni({
  caso,
  onVerHistorial,
}: {
  caso: CasoIdentificacion;
  onVerHistorial: () => void;
}): ReactElement {
  return (
    <tr className={`oni-fila oni-fila-${caso.estado}`}>
      <td>
        <p className="oni-titulo">{caso.titulo}</p>
        <p className="oni-meta">
          {nombreDeFuente(caso.fuente)} · {formatearPeriodo(caso.periodo)}
        </p>
      </td>
      <td>
        <EtiquetaDeEstadoOni estado={caso.estado} />
      </td>
      <td>{caso.resuelto_por?.nombre ?? "—"}</td>
      <td>
        <time
          dateTime={caso.ultima_actualizacion}
          title={caso.ultima_actualizacion}
          className="oni-fecha"
        >
          {formatearInstante(caso.ultima_actualizacion)}
        </time>
      </td>
      <td className="oni-acciones">
        <button
          type="button"
          className="boton-fantasma"
          onClick={onVerHistorial}
        >
          Ver historial
        </button>
        {caso.estado === "pendiente" && (
          <Link to="/identificacion" className="boton-una">
            Resolver
          </Link>
        )}
      </td>
    </tr>
  );
}

/**
 * El historial del registro (D5), armado con lo que la fila YA trae: no hay
 * `GET` de un caso por id, y no hace falta.
 */
function HistorialDelRegistro({
  idTitulo,
  caso,
  onCerrar,
}: {
  idTitulo: string;
  caso: CasoIdentificacion;
  onCerrar: () => void;
}): ReactElement {
  return (
    <div className="historial-registro">
      <header className="historial-registro-cabecera oni-historial-cabecera">
        <h2 id={idTitulo}>{tituloDelHistorial(caso)}</h2>
      </header>
      <div className="historial-registro-cuerpo">
        <LineaDeTiempo caso={caso} />
        {caso.estado === "asignado" && caso.obra_asignada && (
          <Link
            to={`/catalogo/${encodeURIComponent(caso.obra_asignada.id)}`}
            className="boton-secundario oni-ver-obra"
          >
            Ver la obra
          </Link>
        )}
      </div>
      <footer className="historial-registro-pie">
        <button type="button" className="boton-secundario" onClick={onCerrar}>
          Cerrar historial
        </button>
      </footer>
    </div>
  );
}

function tituloDelHistorial(caso: CasoIdentificacion): string {
  if (caso.estado === "pendiente") return "Registro enviado a revisión";
  if (caso.estado === "asignado") {
    return `Registro asignado a “${caso.obra_asignada?.titulo ?? "obra"}”`;
  }
  return "Registro descartado";
}

/** Los pasos del registro, de arriba abajo: reportado, revision y decision. */
function LineaDeTiempo({ caso }: { caso: CasoIdentificacion }): ReactElement {
  const resuelto = caso.estado !== "pendiente";
  return (
    <ol className="linea-tiempo">
      <li className="linea-tiempo-paso">
        <div className="linea-tiempo-contenido">
          <strong className="linea-tiempo-titulo">Reportado</strong>
          <p className="linea-tiempo-texto">
            {nombreDeFuente(caso.fuente)} · {formatearPeriodo(caso.periodo)}
          </p>
        </div>
      </li>
      <li
        className={`linea-tiempo-paso${resuelto ? "" : " linea-tiempo-actual"}`}
      >
        <div className="linea-tiempo-contenido">
          <strong className="linea-tiempo-titulo">En revisión</strong>
          <p className="linea-tiempo-texto">
            {caso.evidencia !== ""
              ? caso.evidencia
              : "No encontramos una obra segura en el catálogo."}
          </p>
        </div>
      </li>
      {resuelto && (
        <li
          className={`linea-tiempo-paso linea-tiempo-actual linea-tiempo-${caso.estado}`}
        >
          <div className="linea-tiempo-contenido">
            <strong className="linea-tiempo-titulo">
              {caso.estado === "asignado" ? "Asignado" : "Descartado"}
            </strong>
            <p className="linea-tiempo-texto">
              <span className="linea-tiempo-quien">
                {caso.resuelto_por?.nombre ?? "—"}
              </span>
              {caso.resuelto_en !== null && (
                <>
                  {" · "}
                  <time dateTime={caso.resuelto_en} title={caso.resuelto_en}>
                    {formatearInstante(caso.resuelto_en)}
                  </time>
                </>
              )}
            </p>
            <p className="linea-tiempo-nota">{caso.nota ?? "—"}</p>
          </div>
        </li>
      )}
    </ol>
  );
}
