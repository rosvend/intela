/**
 * Contrato que el panel de corridas consume. #34 y #37 lo van a escribir en
 * `api/openapi.yaml`; hasta entonces estas rutas 404 y el hook de #31 las
 * trata como ausencia, no como fallo. No se anaden al YAML todavia: un
 * contrato que promete rutas que devuelven 404 es peor que uno corto.
 */
export const RUTAS_REPARTO = {
  procesos: "/api/procesos",
  proceso: (id: string) => `/api/procesos/${encodeURIComponent(id)}`,
  firmar: (id: string) => `/api/procesos/${encodeURIComponent(id)}/firmar`,
  alertas: (periodo?: string) =>
    periodo
      ? `/api/alertas?periodo=${encodeURIComponent(periodo)}`
      : "/api/alertas",
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

/**
 * Tipos que #37 detecta. El string del API no se cierra en el tipo `Alerta`
 * para que un tipo nuevo no tumbe el tablero: se pinta crudo.
 */
export type TipoDeAlerta =
  | "oni"
  | "duplicado_archivo"
  | "duplicado_registro"
  | "titular_sin_porcentaje"
  | "reserva_declaracion_incompleta";

export type Alerta = {
  id: string;
  tipo: string;
  detalle: string;
  periodo?: string;
  referencia?: string;
  resuelta?: boolean;
};
