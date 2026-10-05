import { formatearCOP } from "./ui/dinero";
import type { components } from "./contrato";

export type ValorizacionDeUso = components["schemas"]["ValorizacionDeUso"];

export type FiltroIngresos = {
  obra: string;
  fuente: string;
  periodo: string;
};

export const filtroVacio: FiltroIngresos = {
  obra: "",
  fuente: "",
  periodo: "",
};

export type Ingreso = {
  ref: string;
  obra_id: string;
  obra: string;
  fuente: string;
  periodo: string;
  neto: string;
};

export type Deduccion = {
  concepto: string;
  porcentaje: string;
  monto: string;
};

export type Firma = {
  rol: string;
  actor_id: string;
  sobre_revision: number;
  etapa: string;
  cuando: string;
};

export type Explicacion = {
  ref: string;
  neto: string;
  bruto: string;
  /** RD 13.1.3 / R-04: la obra entera se retuvo por declaracion incompleta. */
  retenida: boolean;
  /** Solo presente si retenida es true. */
  motivo?: string;
  corrida: {
    proceso_id: string;
    periodo: string;
    circuito: string;
  };
  /** La bolsa de la que sale la cifra: lo que pago el usuario de recaudo. */
  bolsa?: {
    id?: string;
    usuario_id?: string;
    bruto: string;
  };
  reporte: {
    id: string;
    fuente: string;
    sha256: string;
  };
  obra: {
    id: string;
    titulo: string;
    escalon: string;
    puntaje: string;
    /** Puntos de reparto de la obra (RD 9). No es puntaje, que es del matching. */
    puntos: string;
  };
  regla: {
    snapshot_id: string;
    reglamento: string;
  };
  /** Desglose por uso (#187). Vacio en cifras valorizadas antes de #187. */
  valorizacion: ValorizacionDeUso[];
  /** Null en la cifra de una obra (sin titular). */
  split: {
    titular_id: string;
    ipi: string;
    porcentaje: string;
    version: number | null;
  } | null;
  deducciones: Deduccion[];
  /** Firmas de compuerta de la corrida (RD 13.5), incluidas revisiones rechazadas. */
  firmas: Firma[];
  /** Eslabones accesorios sin asiento (p. ej. "recaudo.registrado"). */
  faltantes?: string[];
};

export type ListaIngresos = {
  ingresos: Ingreso[];
};

/** Ruta de consulta. El titular no viaja: lo pone la sesion. */
export function rutaMisIngresos(f: FiltroIngresos): string {
  const p = new URLSearchParams();
  if (f.obra) p.set("obra", f.obra);
  if (f.fuente) p.set("fuente", f.fuente);
  if (f.periodo) p.set("periodo", f.periodo);
  const q = p.toString();
  return q ? `/api/mis-ingresos?${q}` : "/api/mis-ingresos";
}

export function rutaExplicar(ref: string): string {
  return `/api/explicar/${encodeURIComponent(ref)}`;
}

export function filtrarIngresos(
  filas: Ingreso[],
  f: FiltroIngresos,
): Ingreso[] {
  return filas.filter((fila) => {
    if (f.obra && fila.obra_id !== f.obra) return false;
    if (f.periodo && fila.periodo !== f.periodo) return false;
    if (f.fuente && !fuentesDe(fila).includes(f.fuente)) return false;
    return true;
  });
}

export function fuentesDe(fila: Ingreso): string[] {
  return fila.fuente
    .split(",")
    .map((s) => s.trim())
    .filter(Boolean);
}

export function opcionesFiltro(filas: Ingreso[]): {
  obras: { id: string; titulo: string }[];
  fuentes: string[];
  periodos: string[];
} {
  const obras = new Map<string, string>();
  const fuentes = new Set<string>();
  const periodos = new Set<string>();
  for (const fila of filas) {
    obras.set(fila.obra_id, fila.obra);
    periodos.add(fila.periodo);
    for (const fuente of fuentesDe(fila)) fuentes.add(fuente);
  }
  return {
    obras: [...obras.entries()]
      .map(([id, titulo]) => ({ id, titulo }))
      .sort((a, b) => a.titulo.localeCompare(b.titulo)),
    fuentes: [...fuentes].sort(),
    periodos: [...periodos].sort(),
  };
}

export function formatearNeto(neto: string): string {
  return formatearCOP(neto);
}
