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

export type Explicacion = {
  ref: string;
  neto: string;
  bruto: string;
  corrida: {
    proceso_id: string;
    periodo: string;
    circuito: string;
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
  };
  regla: {
    snapshot_id: string;
    reglamento: string;
  };
  split: {
    titular_id: string;
    ipi: string;
    porcentaje: string;
    version: number;
  };
  deducciones: Deduccion[];
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
  return `$ ${neto}`;
}
