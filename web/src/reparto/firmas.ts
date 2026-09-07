import { Rol } from "../sesion";
import { Etapa, Firma, Proceso, RolDeFirma } from "./tipos";

export const ROLES_DE_COMPUERTA: readonly RolDeFirma[] = [
  "distribucion",
  "contabilidad",
];

export const ETIQUETA_ROL_FIRMA: Record<RolDeFirma, string> = {
  distribucion: "Distribución",
  contabilidad: "Contabilidad",
};

const COMPUERTAS: ReadonlySet<Etapa> = new Set([
  "verificacion",
  "pago_registro",
]);

export function esCompuerta(etapa: Etapa): boolean {
  return COMPUERTAS.has(etapa);
}

export function firmasDeRevision(proceso: Proceso): readonly Firma[] {
  return (proceso.firmas ?? []).filter(
    (firma) => firma.sobre_rev === proceso.revision,
  );
}

export function rolesFirmados(proceso: Proceso): readonly RolDeFirma[] {
  return firmasDeRevision(proceso).map((firma) => firma.rol);
}

export function rolesPendientes(proceso: Proceso): readonly RolDeFirma[] {
  const firmados = new Set(rolesFirmados(proceso));
  return ROLES_DE_COMPUERTA.filter((rol) => !firmados.has(rol));
}

/**
 * Quien no es uno de los dos roles de la tabla `firmas`, o quien ya firmo
 * esta revision, no ve el control. El backend igual rechaza: ocultar el
 * boton no es la autorizacion, es no ofrecer una accion que va a fallar.
 */
export function puedeFirmar(rol: Rol, proceso: Proceso): boolean {
  if (!esCompuerta(proceso.etapa)) return false;
  if (rol !== "distribucion" && rol !== "contabilidad") return false;
  return rolesPendientes(proceso).includes(rol);
}
