/**
 * Formas que el tablero espera de los conteos. Cada endpoint llega con el
 * PR del modulo que lo posee (ingesta, repertorio, ONI, corridas,
 * liquidaciones). Hasta entonces el hook trata 404/501/red como "ausente"
 * y la tarjeta muestra el vacio, nunca un crash (issue #31).
 *
 * No se anaden a openapi.yaml todavia: un contrato que promete rutas que
 * devuelven 404 es peor que uno corto.
 */
export const RUTAS_TABLERO = {
  cargasPendientes: "/api/tablero/cargas-pendientes",
  obrasEnReserva: "/api/tablero/obras-en-reserva",
  oni: "/api/tablero/oni",
  ultimaCorrida: "/api/tablero/ultima-corrida",
  misObras: "/api/tablero/mis-obras",
  ultimaLiquidacion: "/api/tablero/ultima-liquidacion",
} as const;

export type Conteo = {
  total: number;
};

export type UltimaCorrida = {
  periodo: string;
  etapa: string;
  estado: string;
};

export type ObraResumen = {
  id: string;
  titulo: string;
  estado: string;
};

export type MisObras = {
  obras: ObraResumen[];
};

export type UltimaLiquidacion = {
  periodo: string;
  /** Importe neto como string decimal. Nunca number: el tablero no hace aritmetica. */
  neto: string;
  obras: number;
};

export type Recurso<T> =
  | { tipo: "cargando" }
  | { tipo: "inactivo" }
  | { tipo: "ausente" }
  | { tipo: "error"; mensaje: string }
  | { tipo: "listo"; datos: T };
