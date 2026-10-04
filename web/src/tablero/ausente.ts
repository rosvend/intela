import { ApiError, ErrorDeRed } from "../api";

const AUSENTE = new Set([404, 501, 502, 503]);
const AUSENTE_SI_YA_ESTA_CABLEADO = new Set([404, 501]);
const CABLEADOS = ["/api/alertas", "/api/procesos"];

/**
 * El backend de este widget todavia no existe, o no responde. No es un
 * fallo de la tarjeta: es el estado vacio que el issue #31 exige.
 *
 * 404 y 501: la ruta no esta implementada. 502 y 503: el proxy delante
 * del API (Docker, nginx) responde "servicio no disponible" cuando el
 * proceso Go no esta arriba — la misma situacion que un ErrorDeRed al
 * pegarle directo. Para los endpoints que ya existen (#158 para `/alertas`,
 * #159 para `/procesos`) el set se encoge a 404/501: ahi un 502/503 es una
 * caida y no ausencia, y presentarla como "sin datos" deja a quien firma
 * ver el periodo mas limpio de lo que esta. El resto sigue como antes.
 *
 * 403 no entra: ya tiene significado definido en api.ts (el reglamento
 * no te deja ver esto). useDashboard filtra por rol con `habilitado`,
 * asi que un 403 que igual llegue es un bug de permisos y debe verse.
 *
 * 401 no entra: `api()` ya redirige al login. 500 tampoco: el endpoint
 * existe y fallo; la tarjeta lo muestra como alerta, sin tumbar el resto.
 */
export function esAusente(error: unknown, path = ""): boolean {
  if (error instanceof ErrorDeRed) return true;
  if (!(error instanceof ApiError)) return false;
  const cableado = CABLEADOS.some(
    (p) => path === p || path.startsWith(`${p}/`) || path.startsWith(`${p}?`),
  );
  return (cableado ? AUSENTE_SI_YA_ESTA_CABLEADO : AUSENTE).has(error.status);
}
