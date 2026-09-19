import { ApiError, ErrorDeRed } from "../api";
import type { components } from "../contrato";
import { formatearEntero } from "../tablero/formato";
import TablaRechazos from "./TablaRechazos";

export type Entrega = components["schemas"]["Entrega"];

// El status HTTP con que respondio el servidor; "red" si no hubo respuesta
// (ErrorDeRed); "desconocido" si algo fallo fuera de la API: un 201 cuyo cuerpo
// no se pudo leer, o un 502/504 que contesta el proxy y no la API.
type StatusDeFallo = number | "red" | "desconocido";

/**
 * La subida que NO entro: con que status contesto y el mensaje que lo explica.
 *
 * Es un tipo propio y no una rama anonima de `Resultado` porque quien lo
 * consume necesita el status suelto: la pantalla decide con el si vuelve a pedir
 * el listado (ver `pudoHaberLlegado`), y `resultadoDeError` siempre devuelve
 * esto, nunca una entrega.
 */
export type Fallo = { tipo: "fallo"; status: StatusDeFallo; mensaje: string };

export type Resultado = { tipo: "entrega"; entrega: Entrega } | Fallo;

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
  // El 403 se titula con su causa propia y NO cae en el cajon neutro, que es lo
  // que hacia antes: `requiereRol` lo responde ANTES de que `subirReporte`
  // (internal/infraestructura/httpapi/server.go) corra, asi que no se escribio
  // nada y no hay nada en duda. El cajon neutro afirmaba de menos -"no se sabe
  // si la entrega se registro"- en la direccion que hace dudar al operador, que
  // es justo lo contrario de lo que este panel existe para hacer. Un 4xx prueba
  // que ESA peticion no registro nada, y el aviso de PUDO_LLEGAR no lo lleva.
  // El 401 es otra cosa y no depende de este titulo: `api.ts` limpia el token y
  // navega a la pantalla de entrada (`alExpirarSesion`) ANTES de lanzar el
  // `ApiError`, asi que lo que el operador termina leyendo no es este panel.
  403: "La sesión no tiene permiso para registrar entregas",
  // 409 es ambiguo y por eso no se titula con ninguna de sus dos causas: el
  // backend responde 409 tanto cuando esa fuente ya entrego exactamente ese
  // archivo como cuando la boveda tiene contenido distinto bajo esa huella
  // (evidencia corrupta, un incidente de integridad que hay que avisar a
  // operacion). Un titulo que afirmara el duplicado contradiria al mensaje que
  // va debajo y haria pasar el segundo caso por un "ya estaba, sigue".
  //
  // Y tampoco puede afirmar el no-registro, que es lo que decia antes ("La
  // entrega no se registró"). Un 4xx prueba que ESA peticion no dejo entrega en
  // `reportes`, que no es lo mismo que "esa entrega no esta registrada": en el
  // duplicado -el caso comun, el que le pasa a quien reenvia- la entrega SI
  // esta, y es justo lo que hace saltar el UNIQUE (sha256, fuente) del esquema
  // (migrations/00001_init.sql); en la evidencia corrupta esta peticion tampoco
  // dejo entrega, pero lo que hay bajo la clave no es lo que se iba a
  // certificar. El
  // titulo dice solo el conflicto, que es lo unico cierto en las dos ramas, y
  // el mensaje de debajo dice cual de las dos es.
  409: "La entrega choca con lo que ya está guardado",
  413: "El archivo es demasiado grande",
  // Un 500 **no** esta aqui a proposito: es el status con numero que deja
  // abierta la pregunta de si quedo algo escrito, asi que cae en el titulo
  // neutro y lleva el aviso. Ver `TITULO_POR_DEFECTO` y `pudoHaberLlegado`.
  //
  // El 503 si esta, y tampoco lleva aviso, porque lo produce `conIngesta` -una
  // guarda PREVIA al handler-: no se escribio nada. Ver `pudoHaberLlegado`.
  503: "La ingesta no está disponible en esta instalación",
};

// El titulo de lo que no tiene uno propio: el fallo "desconocido" (un 2xx que
// no se dejo leer, un 502/504 del proxy), el **500** y cualquier status sin
// texto en el mapa de arriba. Afirma solo lo que se sabe, que es poco: si la
// entrega quedo registrada no lo sabe nadie en esos casos.
//
// El 500 esta aqui a proposito, aunque el backend lo conteste con "no se pudo
// registrar la entrega" (internal/infraestructura/httpapi/reportes.go). Que las
// dos escrituras del caso de uso vayan en UNA transaccion (`GuardarEntrega`,
// internal/aplicacion/ingesta.go) y que las ramas anteriores
// (`congelarEvidencia`, `aplicarNormalizacion`) no dejen fila dice COMO se
// escriben las filas, no COMO acaba el COMMIT: `Store.EnTransaccion`
// (internal/infraestructura/postgres/store.go) devuelve el fallo del COMMIT como
// un error mas, y si el enlace con el motor se pierde despues de mandarlo el
// cliente no puede saber si entro -el motor tiene estados propios para eso
// (`08007 transaction_resolution_unknown`, `40003 statement_completion_unknown`)
// -. Afirmar el no-registro seria decir mas de lo que el sistema sabe, que es
// justo lo que este panel evita; por eso el 500 lleva ademas el aviso de
// PUDO_LLEGAR, como "red" y "desconocido".
const TITULO_POR_DEFECTO = "No se sabe si la entrega se registró";

const TITULO_SIN_RESPUESTA = "No se pudo contactar al servidor";

// Lo que se pone en el cuerpo cuando no hay ningun mensaje del backend que
// mostrar: ni el error desconocido ni el 502/504 del proxy traen uno.
const MENSAJE_DESCONOCIDO = "error desconocido al subir el archivo";

// Lo que se dice cuando no queda claro si la entrega se registro. Reintentar a
// ciegas daria 409 si llego, y el 409 es irreversible. Lo llevan exactamente los
// desenlaces de `pudoHaberLlegado`, que es la unica definicion de esa duda.
const PUDO_LLEGAR =
  "La entrega pudo haber llegado al servidor: revisa el listado de cargas antes de volver a subirla.";

// El status con que contesta el proxy cuando la API no llego a responder. Ver
// `resultadoDeError`, que los manda al caso no clasificable.
const SIN_RESPUESTA_UTIL = new Set([502, 504]);

/**
 * Si un fallo deja abierta la pregunta de si la entrega quedo registrada.
 *
 * Es la UNICA definicion de esa duda, y la usan las dos mitades de la pantalla
 * que dependen de ella:
 *
 * - el panel, para el aviso de `PUDO_LLEGAR`;
 * - la pantalla (`Ingesta.tsx`), para volver a pedir el listado justo cuando se
 *   le manda al operador mirarlo. Escrita dos veces -el aviso por un lado y el
 *   `setVersion` de la rama de exito por otro-, las dos mitades discrepan: el
 *   aviso dice que mire el listado y el listado no se vuelve a pedir, asi que lo
 *   que mira es la foto de ANTES del intento. Si el COMMIT entro, el operador no
 *   ve la fila nueva, concluye que no llego y reenvia: 409 irreversible.
 *
 * Los desenlaces:
 *
 * - `"red"` y `"desconocido"` no tienen status: `fetch` no llego a tener
 *   respuesta -`ErrorDeRed`-, o el cuerpo de un 2xx no se dejo leer. De ahi no se
 *   sabe nada;
 * - un **5xx** es un fallo del servidor despues de haber recibido la peticion, y
 *   el COMMIT puede haber entrado (el porque largo, en `TITULO_POR_DEFECTO`).
 *   Hoy el handler de esta ruta solo produce el 500, pero el predicado **no lo
 *   enumera**: un 5xx que nadie clasifico es exactamente el caso que hay que
 *   tratar como duda, y el precio de equivocarse es decir "pudo haber llegado"
 *   de mas, que es el lado seguro;
 * - el **503** es la excepcion y no entra: lo produce `conIngesta`
 *   (internal/infraestructura/httpapi/server.go), una guarda PREVIA al handler
 *   que responde cuando al binario le falta cablear la ingesta. No se escribio
 *   nada, y por eso tiene titulo propio y ningun aviso;
 * - un **4xx** cierra la pregunta: es una respuesta del servidor en la que esa
 *   peticion no dejo entrega en `reportes`, que es lo unico de lo que duda este
 *   predicado. No dice nada de si esa entrega estaba registrada de antes: en el
 *   409 duplicado lo estaba, y la duda no es por eso. Y no garantiza que no se
 *   escribiera NADA: la evidencia se congela antes de decidir el conflicto
 *   (`congelarEvidencia`) y de la boveda no se borra. El 400, ademas, lo
 *   explica el backend en su cuerpo, cuando llega legible: nombra la causa -el
 *   campo del formulario o la columna del archivo que falta-, no que no se haya
 *   escrito nada.
 */
export function pudoHaberLlegado(status: StatusDeFallo): boolean {
  if (status === "red" || status === "desconocido") return true;
  if (status === 503) return false;
  return status >= 500;
}

/**
 * Traduce lo que lanza `api()` al subir un reporte en un `Resultado` de fallo.
 * La pantalla de ingesta lo usa en el `catch` del POST /reportes. Nunca lanza:
 * - `ApiError` -> su status y el mensaje del backend, sin tocar; un 502/504 cae
 *   en "desconocido", porque no son un fallo de la API sino una respuesta que se
 *   perdio en el camino;
 * - `ErrorDeRed` -> status "red" y su mensaje;
 * - cualquier otra cosa -> status "desconocido" y un mensaje generico.
 */
export function resultadoDeError(error: unknown): Fallo {
  // El 502 y el 504 los produce el proxy, no la API: nginx corta a los 120s
  // (deploy/nginx.conf) y el handler de Go tiene 60s de escritura, asi que una
  // ingesta larga -un archivo grande, un lote que tarda- puede quedarse sin
  // respuesta con la entrega ya confirmada en la base. Es el caso en que menos
  // se sabe si la fila llego, y por eso va al caso no clasificable, el unico que
  // avisa de que la entrega pudo haber llegado. Ojo: el mapeo NO se puede
  // borrar argumentando que `api()` ya descarta el HTML del proxy -cierto desde
  // la frontera-, porque lo que se pierde al borrarlo es el aviso, no el HTML:
  // un 502 con su titulo propio no lo lleva.
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
  // La duda la define `pudoHaberLlegado`, que es exactamente la misma que usa
  // `Ingesta.tsx` para volver a pedir el listado: si el aviso manda a mirarlo,
  // el listado que se mira tiene que ser el de DESPUES del intento. Con las dos
  // escritas por separado, el aviso salia y el refetch no.
  const avisarQuePudoLlegar = pudoHaberLlegado(status);

  return (
    <section className="panel-resultado panel-fallo" role="alert">
      <h2>{tituloDeFallo(status)}</h2>
      {/* El mensaje del backend va entero y tal cual. Nombra las columnas y
          campos que faltan; partirlo o reescribirlo acoplaria la UI a la prosa
          de Go y se romperia en silencio al reformularla. El de ErrorDeRed no
          se muestra porque repite el titulo. Un cuerpo que no parsea como JSON
          -la pagina de nginx- ya no llega hasta aqui: `mensajeDeError` lo
          sustituye por un mensaje generico en `api.ts`, que es donde se sabe
          que el contrato promete JSON. */}
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
