import type { Deduccion, ValorizacionDeUso } from "../ingresos";
import {
  citaDeConcepto,
  nombreConcepto,
  type CitaReglamento,
} from "../reglamento";
import { fechaLlana, numeroLlano, sumarImportes } from "./presentacion";

export type ParteDelBruto = {
  id: string;
  etiqueta: string;
  valor: string;
  color: string;
  razon: string;
  cita?: CitaReglamento;
};

const RAZONES: Record<string, string> = {
  gastos_administrativos:
    "REDES SGC usa esta parte para operar: cobrar a quienes usan las obras, identificarlas y pagarte.",
  bienestar_social:
    "Se destina a programas de bienestar e inversión social para los escritores afiliados.",
  reserva_errores_tecnicos:
    "Se guarda para corregir errores y atender reclamos de los titulares.",
};

const COLORES: Record<string, string> = {
  // Orden validado (dataviz): serie-3/serie-5 adyacentes no se distinguen.
  gastos_administrativos: "var(--serie-5)",
  bienestar_social: "var(--serie-4)",
  reserva_errores_tecnicos: "var(--serie-3)",
};

/** El bruto partido en "Tú recibes" + cada descuento de ley; las partes suman el bruto. */
export function partesDelBruto(cifra: {
  neto: string;
  deducciones: Deduccion[];
}): ParteDelBruto[] {
  return [
    {
      id: "neto",
      etiqueta: "Tú recibes",
      valor: cifra.neto,
      color: "var(--serie-1)",
      razon: "Lo que llega a tu cuenta después de los descuentos de ley.",
    },
    ...cifra.deducciones.map((d) => ({
      id: d.concepto,
      etiqueta: nombreConcepto(d.concepto),
      valor: d.monto,
      color: COLORES[d.concepto] ?? "var(--serie-6)",
      razon: RAZONES[d.concepto] ?? "Descuento previsto en el reglamento.",
      cita: citaDeConcepto(d.concepto),
    })),
  ];
}

export function cuadraConBruto(
  partes: ParteDelBruto[],
  bruto: string,
): boolean {
  return sumarImportes(partes.map((p) => p.valor)) === sumarImportes([bruto]);
}

const MODALIDADES: Record<string, string> = {
  "RD 9.1.1": "Televisión abierta",
  "RD 9.2": "Salas de cine",
  "RD 9.3": "Teatro",
  "RD 9.4": "Transporte público",
  "RD 9.5": "Televisión por suscripción",
  "RD 9.6": "Hoteles y lugares públicos",
  "RD 9.7": "Plataformas digitales",
};

export function modalidadDeUso(v: ValorizacionDeUso): string {
  return MODALIDADES[v.formula] ?? v.formula;
}

type NombreFactor =
  ValorizacionDeUso["terminos"][number]["factores"][number]["nombre"];

const FRASES: Record<NombreFactor, (v: string) => string> = {
  ponderacion: (v) => `pesa ${v} por su tipo de obra`,
  duracion_min: (v) => `dura ${v} minutos`,
  rating: (v) => `tuvo un rating de ${v}`,
  emisiones: (v) => `se emitió ${v} veces`,
  espectadores: (v) => `la vieron ${v} espectadores`,
  taquilla: (v) => `hizo una taquilla de ${v}`,
  exhibiciones: (v) => `se exhibió ${v} veces`,
  pb: (v) => `parte de un puntaje base de ${v} por su tipo de obra`,
  minutos_vistos: (v) => `se vio durante ${v} minutos`,
  vistas: (v) => `se vio ${v} veces`,
  wa: (v) => `tiene un factor de ajuste de ${v}`,
  wb: (v) => `tiene un factor de ajuste de ${v}`,
  wc: (v) => `tiene un factor de ajuste de ${v}`,
};

function enumerar(partes: string[]): string {
  if (partes.length <= 1) return partes[0] ?? "";
  return `${partes.slice(0, -1).join(", ")} y ${partes[partes.length - 1]}`;
}

/** Los factores de un uso en una frase; los valores son los del backend, solo reformateados. */
export function fraseDeUso(v: ValorizacionDeUso): string {
  const factores = v.terminos.flatMap((t) => t.factores);
  const cuerpo = enumerar(
    factores.map((f) => FRASES[f.nombre](numeroLlano(f.valor))),
  );
  const frase = cuerpo.charAt(0).toUpperCase() + cuerpo.slice(1);
  return `${frase}. En total suma ${numeroLlano(v.puntos)} puntos.`;
}

export function fechaDeAprobacion(firmas: { cuando: string }[]): string {
  if (firmas.length === 0) return "";
  const ultima = [...firmas].sort((a, b) =>
    b.cuando.localeCompare(a.cuando),
  )[0];
  return fechaLlana(ultima.cuando);
}
