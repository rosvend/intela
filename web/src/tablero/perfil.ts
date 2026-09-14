import { Rol } from "../sesion";

export type PerfilDeTablero = "administrador" | "titular";

/**
 * KR-5 pide dos perfiles. El resto de roles de staff aterrizan en el
 * tablero de administrador: ven KPIs operativos, no la liquidacion de un
 * titular. El titular es el unico que cambia de pantalla (M-5).
 */
export function perfilDeTablero(rol: Rol): PerfilDeTablero {
  return rol === "titular" ? "titular" : "administrador";
}
