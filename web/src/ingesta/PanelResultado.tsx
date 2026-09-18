import { ApiError, ErrorDeRed } from "../api";
import type { components } from "../contrato";
import { formatearEntero } from "../tablero/formato";
import TablaRechazos from "./TablaRechazos";

export type Entrega = components["schemas"]["Entrega"];

// El status HTTP con que respondio el servidor; "red" si no hubo respuesta
// (ErrorDeRed); "desconocido" si algo fallo fuera de la API: un 201 cuyo cuerpo
// no se pudo leer, o un 502/504 que contesta el proxy y no la API.
type StatusDeFallo = number | "red" | "desconocido";

export type Resultado =
  | { tipo: "entrega"; entrega: Entrega }
  | { tipo: "fallo"; status: StatusDeFallo; mensaje: string };

// La huella completa (64 hex) queda en el `title`; a la vista basta un prefijo
// para reconocer la evidencia.
const LARGO_HUELLA_CORTA = 12;

/**
 * El prefijo visible de una huella SHA-256. Lo usan el panel y el listado de
 * cargas, para que las dos vistas muestren el mismo largo. Recibe la huella
 * en hex y devuelve sus primeros 12 caracteres, o la cadena entera si es mas
 * corta. Nunca lanza.
 */
export function huellaCorta(sha256: string): string {
  return sha256.slice(0, LARGO_HUELLA_CORTA);
}

// Titulos por status del POST /reportes (api/openapi.yaml). El detalle lo da
// siempre el mensaje del backend, que va debajo.
const TITULO_POR_STATUS: Record<number, string> = {
  400: "La entrega no cumple la estructura mínima",
  // 409 es ambiguo y por eso no se titula con ninguna de sus dos causas: el
  // backend responde 409 tanto cuando esa fuente ya entrego exactamente ese
  // archivo como cuando la boveda tiene contenido distinto bajo esa huella
  // (evidencia corrupta, un incidente de integridad que hay que avisar a
  // operacion). Un titulo que afirmara el duplicado contradiria al mensaje que
  // va debajo y haria pasar el segundo caso por un "ya estaba, sigue".
  409: "La entrega no se registró",
  413: "El archivo es demasiado grande",
  503: "La ingesta no está disponible en esta instalación",
};

const TITULO_POR_DEFECTO = "No se pudo registrar la entrega";

const TITULO_SIN_RESPUESTA = "No se pudo contactar al servidor";

// Lo que se pone en el cuerpo cuando no hay ningun mensaje del backend que
// mostrar: ni el error desconocido ni el 502/504 del proxy traen uno.
const MENSAJE_DESCONOCIDO = "error desconocido al subir el archivo";

// Lo que se dice cuando no queda claro si la entrega se registro. Reintentar a
// ciegas daria 409 si llego, y el 409 es irreversible.
const PUDO_LLEGAR =
  "La entrega pudo haber llegado al servidor: revisa el listado de cargas antes de volver a subirla.";

// 502 y 504 los contesta el proxy, no la API, y lo que llega en el cuerpo es su
// pagina de error: no se muestra (ver `resultadoDeError`).
const SIN_RESPUESTA_UTIL = new Set([502, 504]);

/**
 * Traduce lo que lanza `api()` al subir un reporte en un `Resultado` de fallo.
 * La pantalla de ingesta lo usa en el `catch` del POST /reportes. Nunca lanza:
 * - `ApiError` -> su status y el mensaje del backend, sin tocar; un 502/504 cae
 *   en "desconocido", porque lo que trae el cuerpo es la pagina del proxy;
 * - `ErrorDeRed` -> status "red" y su mensaje;
 * - cualquier otra cosa -> status "desconocido" y un mensaje generico.
 */
export function resultadoDeError(error: unknown): Resultado {
  // El 502 y el 504 los produce el proxy, no la API: su cuerpo es la pagina de
  // error de nginx (deploy/nginx.conf corta a los 120s y el handler de Go
  // tiene 60s de escritura), asi que una ingesta larga puede quedarse sin
  // respuesta con la entrega ya confirmada en la base. Ese cuerpo no es un
  // mensaje del backend y por eso no se muestra: el fallo cae al caso no
  // clasificable, que ya avisa de que la entrega pudo haber llegado.
  if (error instanceof ApiError && !SIN_RESPUESTA_UTIL.has(error.status)) {
    return { tipo: "fallo", status: error.status, mensaje: error.message };
  }
  if (error instanceof ErrorDeRed) {
    return { tipo: "fallo", status: "red", mensaje: error.message };
  }
  return { tipo: "fallo", status: "desconocido", mensaje: MENSAJE_DESCONOCIDO };
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
              {huellaCorta(entrega.sha256)}
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

function tituloDeFallo(status: StatusDeFallo): string {
  if (status === "red") return TITULO_SIN_RESPUESTA;
  if (status === "desconocido") return TITULO_POR_DEFECTO;
  return TITULO_POR_STATUS[status] ?? TITULO_POR_DEFECTO;
}

function PanelFallo({
  status,
  mensaje,
}: {
  status: StatusDeFallo;
  mensaje: string;
}) {
  const avisarQuePudoLlegar = status === "red" || status === "desconocido";

  return (
    <section className="panel-resultado panel-fallo" role="alert">
      <h2>{tituloDeFallo(status)}</h2>
      {/* El mensaje del backend va entero y tal cual. Nombra las columnas y
          campos que faltan; partirlo o reescribirlo acoplaria la UI a la prosa
          de Go y se romperia en silencio al reformularla. El de ErrorDeRed no
          se muestra porque repite el titulo. */}
      {status !== "red" && <p className="panel-mensaje">{mensaje}</p>}
      {/* El backend garantiza que un 400 no persiste nada: por eso el aviso
          va ahi y en ningun otro status. */}
      {status === 400 && (
        <p>No se guardó nada: corrige el archivo y vuelve a subirlo.</p>
      )}
      {/* Sin una respuesta legible no se sabe si la entrega quedo registrada. */}
      {avisarQuePudoLlegar && <p>{PUDO_LLEGAR}</p>}
    </section>
  );
}
