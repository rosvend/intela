import { Fragment, useState } from "react";
import Cargando from "../Cargando";
import type { components } from "../contrato";
import { formatearEntero } from "../tablero/formato";
import { useApi } from "../useApi";
import { huellaCorta } from "./PanelResultado";
import TablaRechazos, { type Rechazo } from "./TablaRechazos";

export type Carga = components["schemas"]["Carga"];

const COLUMNAS = 7;

const FORMATO_RECIBIDO = new Intl.DateTimeFormat("es-CO", {
  dateStyle: "medium",
  timeStyle: "short",
});

// Una fecha ilegible se muestra tal cual: `format` lanzaria RangeError y
// tumbaria el listado entero por una sola celda.
function formatearRecibido(iso: string): string {
  const fecha = new Date(iso);
  return Number.isNaN(fecha.getTime()) ? iso : FORMATO_RECIBIDO.format(fecha);
}

/**
 * Las cargas hechas (GET /reportes), de un periodo o de todos si `periodo`
 * viene vacio. Cada carga con rechazos se puede abrir para ver su log.
 */
export default function ListaCargas({ periodo }: { periodo: string }) {
  const path = periodo
    ? `/api/reportes?periodo=${encodeURIComponent(periodo)}`
    : "/api/reportes";
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
  // cuerpo que no es JSON, no un JSON que no es una lista.
  if (!Array.isArray(cargas)) {
    return (
      <p className="ingesta-error" role="alert">
        El listado no llegó como una lista de cargas.
      </p>
    );
  }

  if (cargas.length === 0) {
    return (
      <p className="muted">
        {periodo
          ? `No hay cargas registradas para el periodo ${periodo}.`
          : "Aún no hay cargas registradas."}
      </p>
    );
  }

  return (
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
                    {formatearRecibido(carga.recibido)}
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
                    <RechazosDeCarga id={carga.id} />
                  </td>
                </tr>
              )}
            </Fragment>
          );
        })}
      </tbody>
    </table>
  );
}

/**
 * El log de una carga. Se monta solo al abrir la fila, asi que el GET sale
 * entonces y no al pintar el listado.
 */
function RechazosDeCarga({ id }: { id: string }) {
  const {
    datos: rechazos,
    cargando,
    error,
  } = useApi<Rechazo[]>(`/api/reportes/${encodeURIComponent(id)}/rechazos`);

  if (cargando) return <Cargando texto="Cargando rechazos…" />;

  if (error) {
    return (
      <p className="ingesta-error" role="alert">
        No se pudo consultar el log de rechazos: {error.message}
      </p>
    );
  }

  // Mismo caso que en el listado: el tipo promete una lista y el 2xx puede
  // traer otra cosa. `TablaRechazos` lee `.length` y `.map`.
  if (!Array.isArray(rechazos)) {
    return (
      <p className="ingesta-error" role="alert">
        El log no llegó como una lista de rechazos.
      </p>
    );
  }

  // Invariante 3: el log va entero, tal como lo devuelve el servidor (que no
  // lo acota). Recortarlo aqui ocultaria filas sin decirlo.
  return <TablaRechazos rechazos={rechazos} />;
}
