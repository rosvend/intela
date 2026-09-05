import { ApiError, ErrorDeRed } from "../api";

const AUSENTE = new Set([403, 404, 501, 502, 503]);

/**
 * El backend de este widget todavia no existe, o no responde. No es un
 * fallo de la tarjeta: es el estado vacio que el issue #31 exige.
 *
 * 401 no entra: `api()` ya redirige al login. 500 tampoco: eso si es un
 * error y la tarjeta lo muestra como alerta, sin tumbar el resto.
 */
export function esAusente(error: unknown): boolean {
  if (error instanceof ErrorDeRed) return true;
  if (error instanceof ApiError) return AUSENTE.has(error.status);
  return false;
}
