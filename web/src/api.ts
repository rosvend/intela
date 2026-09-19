// Exportada porque `sesion.tsx` necesita reconocer esta clave en el evento
// `storage` para detectar que otra pestana cambio o cerro la sesion. Duplicar
// la cadena en dos archivos es como se acaba escuchando una clave que ya nadie
// escribe.
export const tokenKey = "intela.token";

export function token() {
  return localStorage.getItem(tokenKey) || "";
}

export function setToken(t: string) {
  localStorage.setItem(tokenKey, t);
}

export function clearToken() {
  localStorage.removeItem(tokenKey);
}

/**
 * La API respondio con un status de error (4xx/5xx). `status` es lo que deja
 * distinguir un 403 (el reglamento no te deja ver esto) de un 500 (la base
 * esta caida) sin parsear el mensaje.
 */
export class ApiError extends Error {
  readonly status: number;

  constructor(status: number, message: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
  }
}

/** `fetch` rechazo antes de que hubiera respuesta: servidor caido, DNS, sin red. */
export class ErrorDeRed extends Error {
  constructor(cause: unknown) {
    super("no se pudo contactar al servidor");
    this.name = "ErrorDeRed";
    this.cause = cause;
  }
}

/**
 * La respuesta llego **sin error** -un 2xx- pero su cuerpo no se pudo leer, asi
 * que el dato prometido no esta.
 *
 * Existe para que quien llama pueda distinguir dos cosas que antes subian
 * iguales, como un `Error` sin tipo:
 *
 * - `ErrorDeRed`: `fetch` no llego a tener respuesta. De aqui no se sabe si el
 *   servidor hizo algo;
 * - este: el servidor **contesto**, y contesto sin error. Que el cuerpo falte es
 *   un problema del cuerpo, no de la operacion.
 *
 * Sin este tipo, un 200 con un cuerpo truncado salia como el `SyntaxError` de
 * `res.json()` y se confundia con un fallo de la operacion. En el editor del
 * reparto eso afirmaba de menos: `res.ok` ya prueba que el servidor acepto el
 * guardado, y su cuerpo es lo unico que no llego.
 */
export class ErrorDeCuerpoIlegible extends Error {
  readonly status: number;

  constructor(status: number, cause: unknown) {
    super("la respuesta llegó sin un cuerpo legible");
    this.name = "ErrorDeCuerpoIlegible";
    this.status = status;
    this.cause = cause;
  }
}

// Sustituible para que un 401 navegue con el router en vez de recargar la
// pagina entera y perder el estado en memoria. Sin registrar ninguno, el
// comportamiento es el de siempre: window.location.href.
let alExpirarSesion: (() => void) | null = null;

export function setUnauthorizedHandler(fn: (() => void) | null) {
  alExpirarSesion = fn;
}

type Opciones = RequestInit & {
  /**
   * La llamada va sin token y su 401 NO significa sesion vencida: es el login
   * rechazando unas credenciales. Antes esto se decidia mirando si el path
   * contenia "/sesiones" -un path que nunca existio; el endpoint real es
   * /auth/session- asi que un login fallido disparaba la redireccion en vez
   * de mostrar "credenciales invalidas".
   */
  anonima?: boolean;
};

// Sin esta anotacion, TypeScript infiere `Promise<any>` (por `res.json()`), y
// eso se propaga: cada envoltorio tipado (sesionActual, useApi<T>) pasa a ser
// en realidad un cast sin comprobar, y si /auth/session cambiara de forma
// nada avisaria hasta que algo explotara en pantalla. `unknown` obliga a que
// cada frontera sin validar quede con un cast explicito y a la vista.
export async function api(path: string, init: Opciones = {}): Promise<unknown> {
  const { anonima, ...resto } = init;
  const headers = new Headers(resto.headers);
  if (!anonima && token()) headers.set("Authorization", `Bearer ${token()}`);
  if (
    !(resto.body instanceof FormData) &&
    !headers.has("Content-Type") &&
    resto.body
  ) {
    headers.set("Content-Type", "application/json");
  }

  let res: Response;
  try {
    res = await fetch(path, { ...resto, headers });
  } catch (err) {
    throw new ErrorDeRed(err);
  }

  // Solo un 401 de una llamada CON token dice algo sobre la sesion guardada.
  // Una llamada anonima no envia token, asi que su 401 significa "esas
  // credenciales no sirven" y nada mas: borrar el token aqui le cerraria la
  // sesion a quien abriera /login con una sesion vigente y fallara la clave.
  if (res.status === 401 && !anonima) {
    localStorage.removeItem(tokenKey);
    (alExpirarSesion ?? (() => (window.location.href = "/login")))();
  }

  if (!res.ok) {
    throw new ApiError(res.status, await mensajeDeError(res));
  }

  const ct = res.headers.get("content-type") || "";
  // `res.json()` sobre un cuerpo que no parsea -truncado, cortado a mitad, o una
  // pagina que no es JSON- lanza un `SyntaxError` y el status se pierde. Se
  // nombra aqui, que es el unico sitio donde se sabe que la respuesta si llego:
  // ver `ErrorDeCuerpoIlegible`.
  if (ct.includes("json")) {
    try {
      return await res.json();
    } catch (err) {
      throw new ErrorDeCuerpoIlegible(res.status, err);
    }
  }
  return res;
}

// Lo que se muestra cuando el cuerpo de un error no parsea como JSON. El
// contrato promete que un error de la API viene como `{"error": "..."}`, asi que
// un cuerpo que no es JSON no es un mensaje suyo: lo pone la infraestructura.
// Hoy es nginx, que contesta su pagina HTML en un 502 o un 504 y tambien en el
// 408 de `client_body_timeout` (deploy/nginx.conf); pintar ese HTML -escapado,
// pero entero- como si fuera la explicacion del backend hacia pasar por
// mensaje del sistema algo que ningun componente del sistema dijo.
const MENSAJE_ERROR_ILEGIBLE = "el servidor respondió un error ilegible";

// El DELETE de /auth/session responde 204 sin cuerpo y sin content-type: no
// se puede pedir .json() a ciegas en el camino de error tampoco. Un cuerpo
// vacio sigue usando el `statusText` (eso si es del protocolo); uno que no
// parsea como JSON -o que parsea sin traer un `error` de texto- se sustituye por
// un mensaje propio: ver `MENSAJE_ERROR_ILEGIBLE`. Y uno que no se puede ni
// leer -el stream se corta a mitad- usa ese mismo mensaje: que el cuerpo falte
// no es razon para tirar el status, que ya llego.
async function mensajeDeError(res: Response): Promise<string> {
  // La lectura del cuerpo va envuelta ella sola, y solo ella: `res.ok` ya se
  // consulto y es falso, asi que de aqui sale un mensaje para un error, nunca
  // una conclusion sobre si el servidor escribio. Sin este `try`, un cuerpo
  // truncado rechazaba la lectura y el `TypeError` del stream subia por encima
  // del `ApiError` que `api()` estaba construyendo, o sea que **el status se
  // tiraba**: un 400 en el que no se escribio nada llegaba a quien llama como un
  // error sin tipo, indistinguible de un fallo del que no se sabe nada, y el
  // editor del reparto lo pintaba como "no se sabe si el guardado abrió una
  // versión" -afirmar no saber algo que el sistema sabe-.
  //
  // No se resuelve con `ErrorDeCuerpoIlegible`, que es la clase hermana: esa es
  // de un **2xx** -la respuesta llego sin error y lo que falta es su cuerpo- y
  // usarla aqui diria que el guardado ocurrio cuando el servidor lo rechazo.
  let texto: string;
  try {
    texto = await res.text();
  } catch {
    return MENSAJE_ERROR_ILEGIBLE;
  }
  if (!texto) return res.statusText;
  try {
    const cuerpo = JSON.parse(texto) as { error?: unknown };
    // Solo vale un `error` que sea un texto con algo dentro, que es la forma que
    // promete el contrato (`{"error": "..."}`). Un cuerpo que parsea pero no
    // trae `error` -el `{"detalle": ...}` de un proxy- o que lo trae vacio
    // (`{"error": ""}`, que ademas es falsy) no es un mensaje del backend:
    // devolverlo crudo pintaba el JSON entero como explicacion del sistema.
    return typeof cuerpo.error === "string" && cuerpo.error !== ""
      ? cuerpo.error
      : MENSAJE_ERROR_ILEGIBLE;
  } catch {
    return MENSAJE_ERROR_ILEGIBLE;
  }
}
