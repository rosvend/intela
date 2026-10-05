import {
  etiquetaDeModalidad,
  idsDeFuente,
  type CandidatoIdentificacion,
  type CasoIdentificacion,
  type EstadoDeCaso,
  type SugerenciaIdentificacion,
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

/** Texto de la propuesta del rankeador; `null` si no hay con que sugerir. */
export function textoDeSugerencia(
  sugerencia: SugerenciaIdentificacion,
): string | null {
  if (sugerencia.decision === "ninguna") return null;
  if (sugerencia.decision === "descartar") {
    return "Sugerencia: descartar este registro.";
  }
  return `Sugerencia: asignar a ${sugerencia.titulo || sugerencia.obra_id}.`;
}

/** La obra del lado del catalogo: una candidata o un resultado de la busqueda. */
export type ObraComparada = {
  id: string;
  titulo: string;
  anio?: number;
  genero?: string;
};

export type CampoComparado =
  "titulo" | "anio" | "genero" | "modalidad" | "periodo" | "ids";

/** `null` mientras no hay obra elegida: sin obra no hay nada que juzgar. */
export type EstadoDeFila = "coincide" | "distinto" | "falta" | null;

export type FilaDeComparacion = {
  campo: CampoComparado;
  etiqueta: string;
  reportado: string | null;
  catalogo: string | null;
  estado: EstadoDeFila;
};

/**
 * Los campos del contrato que lee la comparacion, por lado. `contrato.test.ts`
 * comprueba que siguen siendo obligatorios en `api/openapi.yaml`.
 */
export const CAMPOS_LEIDOS = {
  CasoIdentificacion: [
    "titulo",
    "titulo_original",
    "fuente",
    "modalidad",
    "periodo",
    "ids_fuente",
  ],
  CandidatoIdentificacion: ["titulo", "anio", "genero", "puntaje"],
} as const;

function normalizarTexto(texto: string): string {
  return texto
    .normalize("NFKD")
    .replace(/\p{M}/gu, "")
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, " ")
    .trim();
}

function estadoDeFila(
  obra: ObraComparada | null,
  reportado: string | null,
  catalogo: string | null,
): EstadoDeFila {
  if (!obra) return null;
  if (reportado === null || catalogo === null) return "falta";
  return reportado === catalogo ? "coincide" : "distinto";
}

/**
 * Las filas alineadas de la vista de union, siempre en el mismo orden. Solo el
 * titulo existe en los dos lados: el resto lo trae uno solo, y el otro pinta
 * "—" en vez de inventarlo.
 */
export function filasDeComparacion(
  caso: CasoIdentificacion,
  obra: ObraComparada | null,
): FilaDeComparacion[] {
  const titulos = [caso.titulo, caso.titulo_original]
    .filter((t) => t !== "")
    .map(normalizarTexto);
  const tituloCoincide =
    obra !== null && titulos.includes(normalizarTexto(obra.titulo));
  const ids = idsDeFuente(caso.ids_fuente).map((l) => l.replace("=", ": "));
  const otras: [CampoComparado, string, string | null, string | null][] = [
    ["anio", "Año", null, obra?.anio !== undefined ? String(obra.anio) : null],
    ["genero", "Género", null, obra?.genero ?? null],
    ["modalidad", "Modalidad", etiquetaDeModalidad(caso.modalidad), null],
    ["periodo", "Periodo", formatearPeriodo(caso.periodo), null],
    ["ids", "ID en la fuente", ids.length > 0 ? ids.join("\n") : null, null],
  ];
  return [
    {
      campo: "titulo",
      etiqueta: "Título",
      reportado: caso.titulo,
      catalogo: obra?.titulo ?? null,
      estado: obra ? (tituloCoincide ? "coincide" : "distinto") : null,
    },
    ...otras.map(([campo, etiqueta, reportado, catalogo]) => ({
      campo,
      etiqueta,
      reportado,
      catalogo,
      estado: estadoDeFila(obra, reportado, catalogo),
    })),
  ];
}
