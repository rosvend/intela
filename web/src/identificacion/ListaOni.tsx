import { useEffect, useId, useState, type ReactElement } from "react";
import { Link, useSearchParams } from "react-router-dom";
import Cargando from "../Cargando";
import Paginador from "../catalogo/Paginador";
import { formatearInstante } from "../tablero/formato";
import { useApi } from "../useApi";
import Dialogo from "./Dialogo";
import {
  ESTADOS_DE_CASO,
  ETIQUETA_ESTADO,
  LIMITE_LISTA_ONI,
  RUTAS_IDENTIFICACION,
  esPaginaDeCasos,
  idsDeFuente,
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
    <section className="lista-oni">
      <header className="lista-oni-cabecera">
        <h1>Lista ONI</h1>
        <p className="bandeja-intro">
          Registros que requirieron revisión en la cascada de identificación.
        </p>
        <Link to="/identificacion" className="boton-secundario">
          Ir a casos pendientes
        </Link>
      </header>

      <div className="filtros">
        <label>
          Estado
          <select
            aria-label="Filtrar por estado"
            value={estado}
            onChange={(e) => aplicarFiltro("estado", e.target.value)}
          >
            <option value="">Todos los estados</option>
            {ESTADOS_DE_CASO.map((valor) => (
              <option key={valor} value={valor}>
                {ETIQUETA_ESTADO[valor]}
              </option>
            ))}
          </select>
        </label>
        <label>
          Fuente
          <select
            aria-label="Filtrar por fuente"
            value={fuente}
            onChange={(e) => aplicarFiltro("fuente", e.target.value)}
          >
            <option value="">Todas las fuentes</option>
            {opcionesDeFuente.map((valor) => (
              <option key={valor} value={valor}>
                {valor}
              </option>
            ))}
          </select>
        </label>
        <label>
          Periodo
          <select
            aria-label="Filtrar por periodo"
            value={periodo}
            onChange={(e) => aplicarFiltro("periodo", e.target.value)}
          >
            <option value="">Todos los periodos</option>
            {opcionesDePeriodo.map((valor) => (
              <option key={valor} value={valor}>
                {valor}
              </option>
            ))}
          </select>
        </label>
        <button
          type="button"
          className="boton-secundario"
          onClick={limpiarFiltros}
        >
          Limpiar filtros
        </button>
      </div>

      {lectura.cargando && <Cargando texto="Cargando la lista ONI…" />}

      {!lectura.cargando && lectura.error && (
        <div className="bandeja-error" role="alert">
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
        <p className="bandeja-error" role="alert">
          La lista ONI no llegó como una página de casos legible.
        </p>
      )}

      {pagina !== null && (
        <>
          {pagina.casos.length === 0 ? (
            <VaciaListaOni conFiltros={hayFiltros} />
          ) : (
            <div className="lista-oni-caja">
              <table className="tabla-lista-oni" aria-label="Lista ONI">
                <thead>
                  <tr>
                    <th scope="col">Título como vino</th>
                    <th scope="col">Fuente</th>
                    <th scope="col">Periodo</th>
                    <th scope="col">Estado</th>
                    <th scope="col">Responsable</th>
                    <th scope="col">Última actualización</th>
                    <th scope="col">Acciones</th>
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
              <div className="lista-oni-pie">
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

/**
 * El estado vacio de la pantalla, sin filtros o con filtros (plano seccion
 * 5): son dos hechos distintos y el texto no los confunde -sin filtros la
 * lista ONI misma esta vacia; con filtros, lo unico que se sabe es que ESOS
 * filtros no encontraron nada-. "Limpiar filtros" para el segundo caso ya
 * existe en la cabecera de filtros; el texto no repite un boton propio.
 */
function VaciaListaOni({ conFiltros }: { conFiltros: boolean }): ReactElement {
  return (
    <div className="bandeja-vacia">
      <p>{conFiltros ? "No hay coincidencias" : "No hay registros ONI"}</p>
      {conFiltros && (
        <p className="muted">
          Ajusta los filtros para consultar otros registros ONI.
        </p>
      )}
    </div>
  );
}

function EtiquetaDeEstadoOni({
  estado,
}: {
  estado: EstadoDeCaso;
}): ReactElement {
  return (
    <span className={`lista-oni-estado lista-oni-estado-${estado}`}>
      {ETIQUETA_ESTADO[estado]}
    </span>
  );
}

/**
 * Una fila de la lista. "Resolver" solo si el caso sigue pendiente (plano
 * seccion 5): un caso ya asignado o descartado no tiene decision que tomar, y
 * el contrato de #175 no admite corregirla (D-alcance de #175).
 *
 * DECISION #39: "Resolver" navega a `/identificacion` -la bandeja, que ya
 * lista todos los pendientes- en vez de abrir aqui mismo el panel de
 * resolucion. Reabrir `PanelResolucion` desde esta pantalla exigiria
 * reconstruir en la lista ONI la remocion optimista y el empuje del conteo
 * (D3) que hoy son responsabilidad de `BandejaIdentificacion` -duplicar ese
 * ciclo de vida en dos pantallas es la clase de indireccion que el CLAUDE.md
 * raiz llama deuda, no diseno-. La bandeja es, ademas, el destino que la
 * propia lista ya ofrece en su cabecera ("Ir a casos pendientes"): un
 * administrador que resuelve desde aqui llega al mismo lugar por el mismo
 * enlace.
 */
function FilaListaOni({
  caso,
  onVerHistorial,
}: {
  caso: CasoIdentificacion;
  onVerHistorial: () => void;
}): ReactElement {
  return (
    <tr>
      <td>
        <p className="lista-oni-titulo-texto">{caso.titulo}</p>
        <p className="muted lista-oni-id">{caso.id}</p>
      </td>
      <td>{caso.fuente}</td>
      <td>{caso.periodo}</td>
      <td>
        <EtiquetaDeEstadoOni estado={caso.estado} />
      </td>
      <td>{caso.resuelto_por?.nombre ?? "—"}</td>
      <td>
        <time
          dateTime={caso.ultima_actualizacion}
          title={caso.ultima_actualizacion}
        >
          {formatearInstante(caso.ultima_actualizacion)}
        </time>
      </td>
      <td className="lista-oni-acciones">
        <button
          type="button"
          className="boton-secundario"
          onClick={onVerHistorial}
        >
          Ver historial
        </button>
        {caso.estado === "pendiente" && (
          <Link to="/identificacion" className="boton-secundario">
            Resolver
          </Link>
        )}
      </td>
    </tr>
  );
}

/**
 * El historial completo del registro (D5): se arma con lo que la propia fila
 * YA trae -quien, cuando, la decision, la nota y, si se asigno, la obra-, sin
 * una peticion adicional. No hay `GET` de un caso por id (plano seccion 2), y
 * tampoco hace falta: la lista ya trajo el caso entero.
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
      <header className="historial-registro-cabecera">
        <p className="muted">{caso.id}</p>
        <h2 id={idTitulo}>{tituloDelHistorial(caso)}</h2>
      </header>
      <div className="historial-registro-cuerpo">
        <CuerpoDelHistorial caso={caso} />
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

/** Los tres cuerpos del historial, segun el estado del caso (plano seccion 5). */
function CuerpoDelHistorial({
  caso,
}: {
  caso: CasoIdentificacion;
}): ReactElement {
  if (caso.estado === "pendiente") {
    const ids = idsDeFuente(caso.ids_fuente);
    return (
      <>
        <p className="muted">Cascada de identificación</p>
        {ids.length > 0 && (
          <ul className="bandeja-fichas">
            {ids.map((id) => (
              <li key={id} className="ficha">
                {id}
              </li>
            ))}
          </ul>
        )}
        <p>{caso.evidencia}</p>
      </>
    );
  }

  return (
    <>
      <dl className="historial-registro-datos">
        <div>
          <dt>Responsable</dt>
          <dd>{caso.resuelto_por?.nombre ?? "—"}</dd>
        </div>
        {caso.resuelto_en !== null && (
          <div>
            <dt>Fecha</dt>
            <dd>
              <time dateTime={caso.resuelto_en} title={caso.resuelto_en}>
                {formatearInstante(caso.resuelto_en)}
              </time>
            </dd>
          </div>
        )}
      </dl>
      <p>{caso.nota ?? "—"}</p>
      {caso.estado === "asignado" && caso.obra_asignada && (
        <Link
          to={`/catalogo/${encodeURIComponent(caso.obra_asignada.id)}`}
          className="boton-secundario"
        >
          Ver la obra
        </Link>
      )}
    </>
  );
}
