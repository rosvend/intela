import { Fragment, useState } from "react";
import Cargando from "../Cargando";
import type { components } from "../contrato";
import { formatearEntero, formatearInstante } from "../tablero/formato";
import { useApi } from "../useApi";
import { huellaCorta } from "./PanelResultado";
import TablaRechazos, { type Rechazo, esRechazo } from "./TablaRechazos";

export type Carga = components["schemas"]["Carga"];

/**
 * Si un valor sin tipar tiene la forma de una `Carga`: los campos que lee este
 * listado, y solo esos.
 *
 * - `sha256` es el campo sobre el que revienta la tabla: `huellaCorta` le hace
 *   `.slice`, asi que una carga sin el -o con un objeto en su lugar- lanzaba
 *   `TypeError: Cannot read properties of undefined (reading 'slice')` y, sin
 *   ErrorBoundary en `web/src`, dejaba la pantalla en blanco;
 * - `recibido` va a `new Date()` y, si no se puede leer, se pinta tal cual
 *   (`formatearInstante`): un objeto ahi es tambien un hijo invalido de React;
 * - `id`, `fuente` y `periodo` se pintan, y `id` ademas es la clave de la fila y
 *   el identificador con que se pide su log;
 * - `aceptados` y `rechazados` son cifras contadas: con otra cosa
 *   `formatearEntero` pintaria `NaN` y el boton de rechazos desapareceria sin
 *   decirlo, que es una cifra mal dicha antes que una pantalla rota.
 *
 * `nbytes` y `clave_objeto` no se miran porque ningun consumidor de esta
 * pantalla los lee. Vive aqui, junto a `Carga`, como `esRechazo` vive junto a
 * `Rechazo` y `esEntrega` junto a `Entrega`: cada guarda se lee al lado del tipo
 * que comprueba.
 */
export function esCarga(valor: unknown): valor is Carga {
  if (typeof valor !== "object" || valor === null) return false;
  const carga = valor as {
    id?: unknown;
    fuente?: unknown;
    periodo?: unknown;
    sha256?: unknown;
    recibido?: unknown;
    aceptados?: unknown;
    rechazados?: unknown;
  };
  return (
    typeof carga.id === "string" &&
    typeof carga.fuente === "string" &&
    typeof carga.periodo === "string" &&
    typeof carga.sha256 === "string" &&
    typeof carga.recibido === "string" &&
    Number.isInteger(carga.aceptados) &&
    Number.isInteger(carga.rechazados)
  );
}

const COLUMNAS = 7;

/**
 * Las cargas hechas (GET /reportes), de un periodo o de todos si `periodo`
 * viene vacio. Cada carga con rechazos se puede abrir para ver su log.
 *
 * Va paginado con el mismo limite que el log de rechazos: cien es una pagina
 * que se lee de un vistazo y queda muy por debajo del maximo de 500 que el
 * servidor rechaza con 400. El listado crece sin cota con cada entrega y cada
 * fila trae dos subconsultas de recuento. Sin total en la respuesta, "hay
 * mas" se dice con la pagina llena: una pagina que no llena el limite es la
 * ultima.
 */
export default function ListaCargas({ periodo }: { periodo: string }) {
  const [desplazamiento, setDesplazamiento] = useState(0);
  const params = new URLSearchParams();
  if (periodo) params.set("periodo", periodo);
  params.set("limite", String(LIMITE_POR_PAGINA));
  params.set("desplazamiento", String(desplazamiento));
  const path = `/api/reportes?${params.toString()}`;
  const { datos: cargas, cargando, error } = useApi<Carga[]>(path);
  // Un Set y no un solo id: comparar el log de dos cargas del mismo periodo
  // es justo lo que se hace al revisarlas.
  const [abiertas, setAbiertas] = useState<ReadonlySet<string>>(new Set());

  function alternar(id: string) {
    setAbiertas((previas) => {
      const siguientes = new Set(previas);
      if (siguientes.has(id)) siguientes.delete(id);
      else siguientes.add(id);
      return siguientes;
    });
  }

  if (cargando) return <Cargando texto="Cargando cargas…" />;

  if (error) {
    return (
      <p className="ingesta-error" role="alert">
        No se pudo consultar el listado de cargas: {error.message}
      </p>
    );
  }

  // `useApi<Carga[]>` promete una lista, pero `T` es una promesa y no una
  // comprobacion: un 2xx con un JSON que no es una lista -el `{error: ...}` de
  // un proxy, o un backend que cambie de forma- llega hasta aqui. Sin este
  // corte, `cargas.length` no seria 0 y `cargas.map` tumbaria la pantalla
  // entera. No es redundante con `useApi`: el hook descarta el `Response` de un
  // cuerpo que no es JSON, no un JSON que no sea la lista prometida, y de la
  // forma de cada elemento no sabe nada.
  //
  // Y no basta con que sea una lista: `carga.sha256` es lo que revienta en
  // `huellaCorta`, asi que cada elemento pasa por `esCarga`. Una carga mal
  // formada deja el listado entero en error a proposito: saltarse la fila mala
  // esconderia una carga -y sus recuentos- sin decirlo, y aqui toda cifra se
  // explica hasta su origen.
  if (!Array.isArray(cargas) || !cargas.every(esCarga)) {
    return (
      <p className="ingesta-error" role="alert">
        El listado no llegó como una lista de cargas legibles.
      </p>
    );
  }

  if (cargas.length === 0) {
    return desplazamiento === 0 ? (
      <p className="muted">
        {periodo
          ? `No hay cargas registradas para el periodo ${periodo}.`
          : "Aún no hay cargas registradas."}
      </p>
    ) : (
      <>
        <p className="muted">No hay más cargas.</p>
        <button
          type="button"
          className="tabla-cargas-boton"
          onClick={() =>
            setDesplazamiento(Math.max(0, desplazamiento - LIMITE_POR_PAGINA))
          }
        >
          Atrás
        </button>
      </>
    );
  }

  const desde = desplazamiento + 1;
  const hasta = desplazamiento + cargas.length;
  const hayMas = cargas.length === LIMITE_POR_PAGINA;

  return (
    <>
      <table className="tabla-cargas" aria-label="Cargas hechas">
        <thead>
          <tr>
            <th scope="col">Recibido</th>
            <th scope="col">Fuente</th>
            <th scope="col">Periodo</th>
            <th scope="col">Huella</th>
            <th scope="col" className="tabla-cargas-numero">
              Aceptadas
            </th>
            <th scope="col" className="tabla-cargas-numero">
              Rechazadas
            </th>
            <th scope="col" aria-label="Acciones" />
          </tr>
        </thead>
        <tbody>
          {cargas.map((carga) => {
            const abierta = abiertas.has(carga.id);
            const idLog = `rechazos-${carga.id}`;
            return (
              <Fragment key={carga.id}>
                <tr>
                  <td>
                    <time dateTime={carga.recibido}>
                      {formatearInstante(carga.recibido)}
                    </time>
                  </td>
                  <td>{carga.fuente}</td>
                  <td>{carga.periodo}</td>
                  <td>
                    <code className="huella" title={carga.sha256}>
                      {huellaCorta(carga.sha256)}
                    </code>
                  </td>
                  <td className="tabla-cargas-numero">
                    {formatearEntero(carga.aceptados)}
                  </td>
                  <td className="tabla-cargas-numero">
                    {formatearEntero(carga.rechazados)}
                  </td>
                  <td>
                    {carga.rechazados > 0 && (
                      <button
                        type="button"
                        className="tabla-cargas-boton"
                        aria-expanded={abierta}
                        aria-controls={idLog}
                        onClick={() => alternar(carga.id)}
                      >
                        {abierta
                          ? "Ocultar rechazos"
                          : `Ver rechazos (${formatearEntero(carga.rechazados)})`}
                      </button>
                    )}
                  </td>
                </tr>
                {abierta && (
                  <tr id={idLog} className="tabla-cargas-log">
                    <td colSpan={COLUMNAS}>
                      {/* El recuento de la fila es el TOTAL de la carga y viaja
                        hasta el log: es con lo que se dice "de M" sin afirmar
                        una cifra que nadie conto. */}
                      <RechazosDeCarga id={carga.id} total={carga.rechazados} />
                    </td>
                  </tr>
                )}
              </Fragment>
            );
          })}
        </tbody>
      </table>
      <div className="tabla-rechazos-pie">
        <span>
          {`Cargas ${formatearEntero(desde)} a ${formatearEntero(hasta)}`}
        </span>
        <div className="tabla-rechazos-botones">
          <button
            type="button"
            className="tabla-rechazos-pagina"
            disabled={desplazamiento === 0}
            onClick={() =>
              setDesplazamiento(Math.max(0, desplazamiento - LIMITE_POR_PAGINA))
            }
          >
            Atrás
          </button>
          {/* Sin total en la respuesta, la pagina llena es la unica senal de
              que puede haber mas: una pagina corta es la ultima. */}
          <button
            type="button"
            className="tabla-rechazos-pagina"
            disabled={!hayMas}
            onClick={() =>
              setDesplazamiento(desplazamiento + LIMITE_POR_PAGINA)
            }
          >
            Adelante
          </button>
        </div>
      </div>
    </>
  );
}

// Cuantos rechazos trae cada pagina del log. Cien es una pagina que se lee de un
// vistazo y queda muy por debajo del maximo de 500 que el servidor rechaza con
// 400, asi que el operador llega al final del log en pocos clics y ninguno de
// ellos le trae una respuesta desproporcionada.
const LIMITE_POR_PAGINA = 100;

/**
 * El log de una carga, por paginas. Se monta solo al abrir la fila, asi que el
 * GET sale entonces y no al pintar el listado.
 *
 * Va paginado porque el log de una entrega grande no cabe en una respuesta ni en
 * una tabla: un archivo con la cabecera equivocada rechaza todas sus filas, y
 * leerlo y pintarlo entero congela la pestana. La cota NO esconde filas: `total`
 * es el recuento de la carga, se dice "Rechazos N a M de TOTAL" y la navegacion
 * llega hasta la ultima. Lo unico que no se hace nunca es recortar en silencio.
 */
function RechazosDeCarga({ id, total }: { id: string; total: number }) {
  const [desplazamiento, setDesplazamiento] = useState(0);
  const path =
    `/api/reportes/${encodeURIComponent(id)}/rechazos` +
    `?limite=${LIMITE_POR_PAGINA}&desplazamiento=${desplazamiento}`;
  const { datos: rechazos, cargando, error } = useApi<Rechazo[]>(path);

  if (cargando) return <Cargando texto="Cargando rechazos…" />;

  if (error) {
    return (
      <p className="ingesta-error" role="alert">
        No se pudo consultar el log de rechazos: {error.message}
      </p>
    );
  }

  // Mismo caso que en el listado: el tipo promete una lista y el 2xx puede
  // traer otra cosa, y `TablaRechazos` lee cuatro campos de cada elemento
  // -`id`, `titulo`, `ids_fuente` y `motivo`- que este tipo no comprueba. Los
  // valida `esRechazo`, definido junto al tipo `Rechazo`. Un log con un rechazo
  // ilegible se avisa entero en vez de dibujar solo las filas buenas: lo que el
  // sistema promete es la pagina completa o un error explicito, nunca filas
  // escondidas sin decirlo.
  if (!Array.isArray(rechazos) || !rechazos.every(esRechazo)) {
    return (
      <p className="ingesta-error" role="alert">
        El log no llegó como una lista de rechazos legibles.
      </p>
    );
  }

  const desde = desplazamiento + 1;
  const hasta = desplazamiento + rechazos.length;
  const hayMas = hasta < total;

  return (
    <>
      <TablaRechazos rechazos={rechazos} />
      <div className="tabla-rechazos-pie">
        <span>
          {rechazos.length > 0
            ? `Rechazos ${formatearEntero(desde)} a ${formatearEntero(hasta)} de ${formatearEntero(total)}`
            : `Esta carga tiene ${formatearEntero(total)} rechazos`}
        </span>
        <div className="tabla-rechazos-botones">
          <button
            type="button"
            className="tabla-rechazos-pagina"
            disabled={desplazamiento === 0}
            onClick={() =>
              setDesplazamiento(Math.max(0, desplazamiento - LIMITE_POR_PAGINA))
            }
          >
            Anterior
          </button>
          {/* Sin el total no se podria decir si hay mas; con el, el boton se
              apaga exactamente en la ultima pagina. */}
          <button
            type="button"
            className="tabla-rechazos-pagina"
            disabled={!hayMas}
            onClick={() =>
              setDesplazamiento(desplazamiento + LIMITE_POR_PAGINA)
            }
          >
            Siguiente
          </button>
        </div>
      </div>
    </>
  );
}
