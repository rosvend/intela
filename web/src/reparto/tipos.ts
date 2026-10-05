import type { components } from "../contrato";

/**
 * Rutas del panel de corridas y del tablero de anomalias. Ya existen en
 * `api/openapi.yaml` (#34, #37, #158); la bandeja pide solo las abiertas
 * porque el resumen cuenta en la base y la lista no tiene que mezclar cerradas.
 */
export const RUTAS_REPARTO = {
  procesos: "/api/procesos",
  proceso: (id: string) => `/api/procesos/${encodeURIComponent(id)}`,
  firmar: (id: string) => `/api/procesos/${encodeURIComponent(id)}/firmar`,
  alertas: (periodo?: string) =>
    periodo
      ? `/api/alertas?periodo=${encodeURIComponent(periodo)}&resueltas=false`
      : "/api/alertas?resueltas=false",
  resumenAlertas: (periodo: string) =>
    `/api/alertas/resumen?periodo=${encodeURIComponent(periodo)}`,
  evaluarAlertas: "/api/alertas/evaluacion",
  bolsas: "/api/bolsas",
} as const;

/** Sondeo del panel: el issue aplaza websocket/SSE para un pico anual. */
export const INTERVALO_SONDEO_MS = 15_000;

/**
 * `Proceso` sale del contrato: el tipo hecho a mano leia `rechazo` y
 * `sobre_rev` cuando la API manda `rechazo_motivo` y `revision` (I7).
 */
export type Proceso = components["schemas"]["Proceso"];
export type Firma = components["schemas"]["Firma"];
export type Circuito = Proceso["circuito"];
export type Etapa = Proceso["etapa"];
/** Roles que la tabla `firmas` admite en una compuerta (CHECK del esquema). */
export type RolDeFirma = Firma["rol"];

export type AccionDeFirma = "firmar" | "rechazar";

export type PedidoDeFirma = {
  accion: AccionDeFirma;
  motivo?: string;
};

export type TipoDeAlerta = components["schemas"]["TipoDeAnomalia"];
export type Alerta = components["schemas"]["Alerta"];
export type ResumenDeAlertas = components["schemas"]["ResumenDeAlertas"];
