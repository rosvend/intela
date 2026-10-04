import { Rol } from "../sesion";
import { formatearEntero } from "../tablero/formato";
import { Conteo, Recurso } from "../tablero/tipos";
import { ResumenDeAlertas, TipoDeAlerta } from "./tipos";

export const ETIQUETA_TIPO: Record<TipoDeAlerta, string> = {
  oni: "ONI",
  duplicado_archivo: "Duplicado de archivo",
  duplicado_registro: "Duplicado de registro",
  titular_sin_porcentaje: "Titular sin %",
  reserva_declaracion_incompleta: "Reserva < 100%",
  tipo_obra_sin_mapear: "Tipo de obra sin mapear",
};

// Derivada del Record exhaustivo: un tipo nuevo en el contrato no compila hasta tener etiqueta.
export const TIPOS_DE_ALERTA = Object.keys(ETIQUETA_TIPO) as TipoDeAlerta[];

export const ROLES_QUE_EVALUAN: readonly Rol[] = [
  "administrador",
  "distribucion",
];
export const puedeEvaluar = (rol: Rol) => ROLES_QUE_EVALUAN.includes(rol);

export function etiquetaDeTipo(tipo: string): string {
  return (ETIQUETA_TIPO as Record<string, string>)[tipo] ?? tipo;
}

export function conteoDeTipo(
  recurso: Recurso<ResumenDeAlertas>,
  tipo: TipoDeAlerta,
): Recurso<Conteo> {
  switch (recurso.tipo) {
    case "listo":
      return {
        tipo: "listo",
        datos: { total: recurso.datos.por_tipo[tipo].abiertas },
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

export function plural(n: number, uno: string, varios: string): string {
  return n === 1 ? uno : varios;
}

/**
 * Lo que la compuerta le dice a quien va a firmar. Avisa, no bloquea: el 409
 * de `avanzar` lo da el backend. Un periodo sin evaluar no se presenta como
 * limpio, y las criticas aceptadas sin corregir el dato se dicen aparte.
 */
export function advertenciaDeCompuerta(r: ResumenDeAlertas): string {
  if (r.ultima_evaluacion === null) {
    return "El periodo no se ha evaluado: estos conteos todavía no dicen nada. Evalúalo antes de firmar.";
  }
  const criticas = r.criticas_abiertas;
  const abiertas = r.abiertas;
  let texto = "";
  if (criticas > 0) {
    texto = `${formatearEntero(criticas)} ${plural(criticas, "crítica bloquea", "críticas bloquean")} la corrida; ${formatearEntero(abiertas)} ${plural(abiertas, "alerta abierta", "alertas abiertas")} en total. Revísalas antes de firmar.`;
  } else if (abiertas > 0) {
    texto = `${formatearEntero(abiertas)} ${plural(abiertas, "alerta abierta", "alertas abiertas")}; ninguna bloquea la corrida.`;
  }
  const aceptadas = r.criticas_aceptadas;
  if (aceptadas > 0) {
    const sufijo = `${formatearEntero(aceptadas)} ${plural(aceptadas, "crítica se aceptó", "críticas se aceptaron")} sin corregir el dato.`;
    texto = texto === "" ? sufijo : `${texto} ${sufijo}`;
  }
  return texto;
}
