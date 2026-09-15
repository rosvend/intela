import { Conteo, Recurso } from "../tablero/tipos";
import { Alerta, TipoDeAlerta } from "./tipos";

export const TIPOS_DE_ALERTA: readonly TipoDeAlerta[] = [
  "oni",
  "duplicado_archivo",
  "duplicado_registro",
  "titular_sin_porcentaje",
  "reserva_declaracion_incompleta",
];

export const ETIQUETA_TIPO: Record<TipoDeAlerta, string> = {
  oni: "ONI",
  duplicado_archivo: "Duplicado de archivo",
  duplicado_registro: "Duplicado de registro",
  titular_sin_porcentaje: "Titular sin %",
  reserva_declaracion_incompleta: "Reserva < 100%",
};

export function etiquetaDeTipo(tipo: string): string {
  return (ETIQUETA_TIPO as Record<string, string>)[tipo] ?? tipo;
}

export function pendientes(alertas: readonly Alerta[]): Alerta[] {
  return alertas.filter((alerta) => alerta.resuelta !== true);
}

export function contarPorTipo(
  alertas: readonly Alerta[],
  tipo: string,
): number {
  return pendientes(alertas).filter((alerta) => alerta.tipo === tipo).length;
}

export function conteosPorTipo(
  alertas: readonly Alerta[],
): Record<string, number> {
  const conteos: Record<string, number> = {};
  for (const tipo of TIPOS_DE_ALERTA) {
    conteos[tipo] = contarPorTipo(alertas, tipo);
  }
  for (const alerta of pendientes(alertas)) {
    if (!(alerta.tipo in conteos)) {
      conteos[alerta.tipo] = contarPorTipo(alertas, alerta.tipo);
    }
  }
  return conteos;
}

export function totalPendientes(alertas: readonly Alerta[]): number {
  return pendientes(alertas).length;
}

export function conteoDeTipo(
  recurso: Recurso<Alerta[]>,
  tipo: string,
): Recurso<Conteo> {
  switch (recurso.tipo) {
    case "listo":
      return {
        tipo: "listo",
        datos: { total: contarPorTipo(recurso.datos, tipo) },
      };
    case "error":
      return { tipo: "error", mensaje: recurso.mensaje };
    case "cargando":
      return { tipo: "cargando" };
    case "inactivo":
      return { tipo: "inactivo" };
    case "ausente":
      return { tipo: "ausente" };
  }
}
