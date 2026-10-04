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
} as const;

/** Sondeo del panel: el issue aplaza websocket/SSE para un pico anual. */
export const INTERVALO_SONDEO_MS = 15_000;

export type Circuito = "nacional" | "internacional";

export type Etapa =
  | "recaudo"
  | "deducciones"
  | "importe_obra"
  | "importe_titular"
  | "liquidacion_parcial"
  | "verificacion"
  | "liquidacion_final"
  | "pago_registro"
  | "fees_in_error"
  | "auditoria";

/**
 * Roles que la tabla `firmas` admite en una compuerta. El CHECK del esquema
 * cierra el conjunto: administrador opera el pipeline y auditor lee, ninguno
 * de los dos firma aqui.
 */
export type RolDeFirma = "distribucion" | "contabilidad";

export type Firma = {
  rol: RolDeFirma;
  actor_id: string;
  sobre_rev: number;
};

export type Proceso = {
  id: string;
  circuito: Circuito;
  etapa: Etapa;
  periodo: string;
  revision: number;
  firmas?: Firma[];
  rechazo?: string;
};

export type AccionDeFirma = "firmar" | "rechazar";

export type PedidoDeFirma = {
  accion: AccionDeFirma;
  motivo?: string;
};

export type TipoDeAlerta = components["schemas"]["TipoDeAnomalia"];
export type Alerta = components["schemas"]["Alerta"];
export type ResumenDeAlertas = components["schemas"]["ResumenDeAlertas"];
