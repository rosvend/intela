import type { ValorizacionDeUso } from "./ingresos";

type NombreFactor =
  ValorizacionDeUso["terminos"][number]["factores"][number]["nombre"];

/** Etiqueta llana de cada factor. Record exhaustivo: un factor nuevo en el contrato no compila sin etiqueta. */
export const ETIQUETA_FACTOR: Record<NombreFactor, string> = {
  ponderacion: "ponderacion",
  duracion_min: "duracion (min)",
  rating: "rating",
  emisiones: "emisiones",
  espectadores: "espectadores",
  taquilla: "taquilla",
  exhibiciones: "exhibiciones",
  pb: "puntaje base",
  wa: "Wa",
  minutos_vistos: "minutos vistos",
  wb: "Wb",
  vistas: "vistas",
  wc: "Wc",
};

/** Usos que el recibo muestra antes del desplegable. */
export const USOS_VISIBLES = 5;

/**
 * "RD 9.1.1: ponderacion 1.3 × duracion (min) 48 × rating 9 × emisiones 10 = 5616 puntos".
 * Con mas de un termino (OTT) cada uno va entre parentesis y se unen con " + ".
 * Los numeros son las cadenas del backend tal cual: no hay aritmetica en JS.
 */
export function lineaDeValorizacion(v: ValorizacionDeUso): string {
  const terminos = v.terminos.map((t) =>
    t.factores
      .map((f) => `${ETIQUETA_FACTOR[f.nombre]} ${f.valor}`)
      .join(" × "),
  );
  const suma =
    terminos.length > 1
      ? terminos.map((t) => `(${t})`).join(" + ")
      : (terminos[0] ?? "");
  return `${v.formula}: ${suma} = ${v.puntos} puntos`;
}
