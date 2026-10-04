import { TIPOS_DE_ALERTA } from "./anomalias";
import { ResumenDeAlertas, TipoDeAlerta } from "./tipos";

/** Fixture de pruebas: un resumen con las seis claves y `critica` como el backend. */
const CRITICOS: TipoDeAlerta[] = [
  "duplicado_archivo",
  "duplicado_registro",
  "tipo_obra_sin_mapear",
];

export function resumenDePrueba(
  cambios: Partial<Omit<ResumenDeAlertas, "por_tipo">> & {
    porTipo?: Partial<Record<TipoDeAlerta, number>>;
  } = {},
): ResumenDeAlertas {
  const { porTipo = {}, ...resto } = cambios;
  const por_tipo = Object.fromEntries(
    TIPOS_DE_ALERTA.map((tipo) => [
      tipo,
      { abiertas: porTipo[tipo] ?? 0, critica: CRITICOS.includes(tipo) },
    ]),
  ) as ResumenDeAlertas["por_tipo"];
  return {
    periodo: "2025",
    abiertas: 0,
    criticas_abiertas: 0,
    criticas_aceptadas: 0,
    ultima_evaluacion: "2026-05-02T08:30:00Z",
    por_tipo,
    ...resto,
  };
}
