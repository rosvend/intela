import { ApiError, ErrorDeRed } from "../api";
import type { components } from "../contrato";
import { formatearEntero } from "../tablero/formato";
import TablaRechazos from "./TablaRechazos";

export type Entrega = components["schemas"]["Entrega"];

export type Resultado =
  | { tipo: "entrega"; entrega: Entrega }
  // status null: no hubo respuesta del servidor (ErrorDeRed) o el error no
  // salio de la API.
  | { tipo: "fallo"; status: number | null; mensaje: string };

// La huella completa (64 hex) queda en el `title`; a la vista basta un prefijo
// para reconocer la evidencia.
const LARGO_HUELLA_CORTA = 12;

// Titulos por status del POST /reportes (api/openapi.yaml). El detalle lo da
// siempre el mensaje del backend, que va debajo.
const TITULO_POR_STATUS: Record<number, string> = {
  400: "La entrega no cumple la estructura mínima",
  409: "Ese archivo ya se había cargado",
  413: "El archivo es demasiado grande",
  503: "La ingesta no está disponible en esta instalación",
};

/**
 * Traduce lo que lanza `api()` al subir un reporte en un `Resultado` de fallo.
 * La pantalla de ingesta lo usa en el `catch` del POST /reportes. Nunca lanza:
 * - `ApiError` -> su status y el mensaje del backend, sin tocar;
 * - `ErrorDeRed` -> status null y su mensaje;
 * - cualquier otra cosa -> status null y un mensaje generico.
 */
export function resultadoDeError(error: unknown): Resultado {
  if (error instanceof ApiError) {
    return { tipo: "fallo", status: error.status, mensaje: error.message };
  }
  if (error instanceof ErrorDeRed) {
    return { tipo: "fallo", status: null, mensaje: error.message };
  }
  return {
    tipo: "fallo",
    status: null,
    mensaje: "error desconocido al subir el archivo",
  };
}

/** Lo que devolvio la subida: la entrega registrada o por que no entro. */
export default function PanelResultado({
  resultado,
}: {
  resultado: Resultado;
}) {
  if (resultado.tipo === "fallo") {
    return <PanelFallo status={resultado.status} mensaje={resultado.mensaje} />;
  }
  return <PanelEntrega entrega={resultado.entrega} />;
}

function PanelEntrega({ entrega }: { entrega: Entrega }) {
  const conRechazos = entrega.rechazados.length > 0;

  return (
    <section
      className={`panel-resultado ${conRechazos ? "panel-parcial" : "panel-ok"}`}
    >
      <h2>Entrega registrada</h2>
      <dl className="panel-datos">
        <div>
          <dt>Fuente</dt>
          <dd>{entrega.fuente}</dd>
        </div>
        <div>
          <dt>Periodo</dt>
          <dd>{entrega.periodo}</dd>
        </div>
        <div>
          <dt>Huella</dt>
          <dd>
            <code className="huella" title={entrega.sha256}>
              {entrega.sha256.slice(0, LARGO_HUELLA_CORTA)}
            </code>
          </dd>
        </div>
        <div>
          <dt>Filas aceptadas</dt>
          <dd>{formatearEntero(entrega.aceptados)}</dd>
        </div>
        <div>
          <dt>Filas rechazadas</dt>
          <dd>{formatearEntero(entrega.rechazados.length)}</dd>
        </div>
      </dl>
      {conRechazos && (
        <>
          <p className="panel-aviso">
            Las filas rechazadas quedaron en el log de rechazos con su motivo;
            ninguna se descartó.
          </p>
          <TablaRechazos rechazos={entrega.rechazados} />
        </>
      )}
    </section>
  );
}

function PanelFallo({
  status,
  mensaje,
}: {
  status: number | null;
  mensaje: string;
}) {
  const titulo =
    status === null
      ? "No se pudo contactar al servidor"
      : (TITULO_POR_STATUS[status] ?? "No se pudo registrar la entrega");

  return (
    <section className="panel-resultado panel-fallo" role="alert">
      <h2>{titulo}</h2>
      {/* D-006: el mensaje del backend va entero y tal cual. Nombra las
          columnas y campos que faltan; partirlo o reescribirlo acoplaria la
          UI a la prosa de Go y se romperia en silencio al reformularla. */}
      <p className="panel-mensaje">{mensaje}</p>
      {/* El backend garantiza que un 400 no persiste nada: por eso el aviso
          va ahi y en ningun otro status. */}
      {status === 400 && (
        <p>No se guardó nada: corrige el archivo y vuelve a subirlo.</p>
      )}
    </section>
  );
}
