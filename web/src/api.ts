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
  if (ct.includes("json")) return res.json();
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
// un mensaje propio: ver `MENSAJE_ERROR_ILEGIBLE`.
async function mensajeDeError(res: Response): Promise<string> {
  const texto = await res.text();
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
