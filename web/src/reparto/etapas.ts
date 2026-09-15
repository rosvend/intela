import { Circuito, Etapa } from "./tipos";

/**
 * Las dos secuencias de RD 13.5. No son una lista con un `if`: el
 * internacional no valoriza (RD 7.4) y tiene Fees in Error (R-16).
 */
export const ETAPAS_NACIONAL: readonly Etapa[] = [
  "recaudo",
  "deducciones",
  "importe_obra",
  "importe_titular",
  "liquidacion_parcial",
  "verificacion",
  "liquidacion_final",
  "pago_registro",
  "auditoria",
];

export const ETAPAS_INTERNACIONAL: readonly Etapa[] = [
  "recaudo",
  "deducciones",
  "liquidacion_parcial",
  "verificacion",
  "liquidacion_final",
  "pago_registro",
  "fees_in_error",
  "auditoria",
];

export const ETIQUETA_ETAPA: Record<Etapa, string> = {
  recaudo: "Recaudo",
  deducciones: "Deducciones",
  importe_obra: "Importe de la obra",
  importe_titular: "Importe por titular",
  liquidacion_parcial: "Liquidación parcial",
  verificacion: "Verificación",
  liquidacion_final: "Liquidación final",
  pago_registro: "Pago y registro",
  fees_in_error: "Fees in Error",
  auditoria: "Auditoría",
};

export const ETIQUETA_CIRCUITO: Record<Circuito, string> = {
  nacional: "Nacional",
  internacional: "Internacional",
};

export type EstadoDePaso = "hecha" | "actual" | "pendiente";

export function etapasDe(circuito: Circuito): readonly Etapa[] {
  return circuito === "internacional" ? ETAPAS_INTERNACIONAL : ETAPAS_NACIONAL;
}

export function estadoDePaso(
  pipeline: readonly Etapa[],
  actual: Etapa,
  paso: Etapa,
): EstadoDePaso {
  const iActual = pipeline.indexOf(actual);
  const iPaso = pipeline.indexOf(paso);
  if (iActual < 0 || iPaso < 0) return "pendiente";
  if (iPaso < iActual) return "hecha";
  if (iPaso === iActual) return "actual";
  return "pendiente";
}
