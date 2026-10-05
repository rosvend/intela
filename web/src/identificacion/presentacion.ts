import type {
  CandidatoIdentificacion,
  CasoIdentificacion,
  EstadoDeCaso,
} from "./tipos";

/**
 * Presentacion de la identificacion: funciones puras que convierten los
 * datos del contrato en lenguaje llano. Ninguna toca dinero ni decide nada.
 */

export type NivelDePuntaje = "alta" | "media" | "baja";

// Bandas de `matching.umbral` (0,60) y `matching.umbral_banda` (0,45), plano de #32.
// Solo pintan el medidor: no estan calibradas y no deciden nada.
const UMBRAL_ALTO = 0.6;
const UMBRAL_MEDIO = 0.45;

export function nivelDePuntaje(puntaje: number): NivelDePuntaje {
  if (puntaje >= UMBRAL_ALTO) return "alta";
  if (puntaje >= UMBRAL_MEDIO) return "media";
  return "baja";
}

export const ETIQUETA_NIVEL: Record<NivelDePuntaje, string> = {
  alta: "Coincidencia alta",
  media: "Coincidencia media",
  baja: "Coincidencia baja",
};

const NOMBRES_DE_FUENTE: Record<string, string> = {
  caracol: "Caracol TV",
  rcn: "RCN",
  netflix: "Netflix",
  cine: "Cine",
};

/** El nombre legible de una fuente; un slug desconocido se capitaliza. */
export function nombreDeFuente(fuente: string): string {
  const conocido = NOMBRES_DE_FUENTE[fuente.toLowerCase()];
  if (conocido) return conocido;
  if (!/^[a-z0-9_-]+$/.test(fuente)) return fuente;
  return fuente
    .split(/[_-]+/)
    .filter(Boolean)
    .map((p) => p.charAt(0).toUpperCase() + p.slice(1))
    .join(" ");
}

const MESES = [
  "ene",
  "feb",
  "mar",
  "abr",
  "may",
  "jun",
  "jul",
  "ago",
  "sep",
  "oct",
  "nov",
  "dic",
];

/** `2024-11` -> `nov 2024`. Lo que no es un mes se deja tal cual. */
export function formatearPeriodo(periodo: string): string {
  const m = /^(\d{4})-(\d{2})$/.exec(periodo);
  if (!m) return periodo;
  const mes = MESES[Number(m[2]) - 1];
  return mes ? `${mes} ${m[1]}` : periodo;
}

function normalizar(palabra: string): string {
  return palabra
    .normalize("NFKD")
    .replace(/\p{M}/gu, "")
    .toLowerCase()
    .replace(/[^a-z0-9]/g, "");
}

export type Segmento = { texto: string; distinto: boolean };

/** Parte `a` en tramos, marcando las palabras que no aparecen en `b`. */
export function compararTitulos(a: string, b: string): Segmento[] {
  const deB = new Set(b.split(/\s+/).map(normalizar).filter(Boolean));
  const segmentos: Segmento[] = [];
  let espacio = "";
  for (const pieza of a.split(/(\s+)/)) {
    if (pieza === "") continue;
    if (/^\s+$/.test(pieza)) {
      espacio += pieza;
      continue;
    }
    const clave = normalizar(pieza);
    const distinto = clave !== "" && !deB.has(clave);
    const ultimo = segmentos.at(-1);
    if (ultimo && ultimo.distinto === distinto) {
      ultimo.texto += espacio + pieza;
    } else {
      segmentos.push({ texto: espacio + pieza, distinto });
    }
    espacio = "";
  }
  return segmentos;
}

const LECTURA_POR_NIVEL: Record<NivelDePuntaje, string> = {
  alta: "El título reportado se parece mucho al de esta obra.",
  media:
    "El título se parece en parte. Revisa el año y el género antes de decidir.",
  baja: "El parecido es débil. Elígela solo si tienes otra evidencia.",
};

/** Por que esta obra es candidata, sin vocabulario del algoritmo. */
export function explicarCandidata(c: CandidatoIdentificacion): string {
  return `${LECTURA_POR_NIVEL[nivelDePuntaje(c.puntaje)]} Comparamos “${c.titulo_consultado}” con “${c.titulo}”.`;
}

export function contarPorEstado(
  casos: readonly CasoIdentificacion[],
): Record<EstadoDeCaso, number> {
  const conteo: Record<EstadoDeCaso, number> = {
    pendiente: 0,
    asignado: 0,
    descartado: 0,
  };
  for (const caso of casos) conteo[caso.estado] += 1;
  return conteo;
}

/** `true` si quien mira pidio menos movimiento (o no hay como saberlo). */
export function prefiereQuieto(): boolean {
  return (
    typeof window === "undefined" ||
    !window.matchMedia ||
    window.matchMedia("(prefers-reduced-motion: reduce)").matches
  );
}
