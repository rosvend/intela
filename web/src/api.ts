const tokenKey = "intela.token";

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

export async function api(path: string, init: Opciones = {}) {
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

  if (res.status === 401) {
    // Se limpia siempre: un token que la API ya no reconoce no sirve de nada,
    // sea porque expiro o porque el login nunca llego a emitir uno.
    localStorage.removeItem(tokenKey);
    if (!anonima)
      (alExpirarSesion ?? (() => (window.location.href = "/login")))();
  }

  if (!res.ok) {
    throw new ApiError(res.status, await mensajeDeError(res));
  }

  const ct = res.headers.get("content-type") || "";
  if (ct.includes("json")) return res.json();
  return res;
}

// El DELETE de /auth/session responde 204 sin cuerpo y sin content-type: no
// se puede pedir .json() a ciegas en el camino de error tampoco.
async function mensajeDeError(res: Response): Promise<string> {
  const texto = await res.text();
  if (!texto) return res.statusText;
  try {
    const cuerpo = JSON.parse(texto) as { error?: string };
    return cuerpo.error || texto;
  } catch {
    return texto;
  }
}
