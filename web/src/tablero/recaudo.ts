import type { components } from "../contrato";
import { sumarImportes } from "./formato";

export type Bolsa = components["schemas"]["Bolsa"];

export type FuenteDeRecaudo = {
  id: string;
  etiqueta: string;
  categoria: string;
  valor: string;
};

export type RecaudoDelPeriodo = {
  periodo: string;
  total: string;
  fuentes: FuenteDeRecaudo[];
};

/** Tope de series de la paleta categorica: lo que sobra se pliega en "Otras". */
const MAX_FUENTES = 6;

const NOMBRE: Record<string, string> = {
  caracol: "Caracol",
  rcn: "RCN",
  netflix: "Netflix",
  procinal: "Procinal",
  dago: "Dago",
  transporte: "Transporte",
};

const CATEGORIA: Record<string, string> = {
  caracol: "Televisión",
  rcn: "Televisión",
  netflix: "Plataforma",
  procinal: "Cine",
  dago: "Internacional",
  transporte: "Transporte",
};

export function etiquetaDeFuente(usuarioId: string): string {
  const conocido = NOMBRE[usuarioId];
  if (conocido) return conocido;
  const texto = usuarioId.replace(/[-_]+/g, " ").trim();
  return texto.charAt(0).toUpperCase() + texto.slice(1);
}

export function categoriaDeFuente(
  usuarioId: string,
  circuito: Bolsa["circuito"],
): string {
  if (circuito === "internacional") return "Internacional";
  return CATEGORIA[usuarioId] ?? "";
}

export function periodoMasReciente(
  bolsas: readonly Bolsa[],
): string | undefined {
  return bolsas.reduce<string | undefined>(
    (mayor, b) =>
      mayor === undefined || b.periodo > mayor ? b.periodo : mayor,
    undefined,
  );
}

function enPeriodo(bolsa: Bolsa, periodo: string): boolean {
  return bolsa.periodo === periodo || bolsa.periodo.startsWith(`${periodo}-`);
}

/** Recaudo del periodo agrupado por usuario (la fuente que pago), de mayor a menor. */
export function recaudoPorFuente(
  bolsas: readonly Bolsa[],
  periodo = periodoMasReciente(bolsas),
): RecaudoDelPeriodo | null {
  if (periodo === undefined) return null;
  const delPeriodo = bolsas.filter((b) => enPeriodo(b, periodo));
  if (delPeriodo.length === 0) return null;

  const porFuente = new Map<string, Bolsa[]>();
  for (const b of delPeriodo) {
    porFuente.set(b.usuario_id, [...(porFuente.get(b.usuario_id) ?? []), b]);
  }
  const fuentes = [...porFuente.entries()]
    .map(([id, grupo]) => ({
      id,
      etiqueta: etiquetaDeFuente(id),
      categoria: categoriaDeFuente(id, grupo[0].circuito),
      valor: sumarImportes(grupo.map((b) => String(b.bruto))),
    }))
    .sort((a, b) => Number(b.valor) - Number(a.valor));

  return {
    periodo,
    total: sumarImportes(delPeriodo.map((b) => String(b.bruto))),
    fuentes: plegar(fuentes),
  };
}

function plegar(fuentes: FuenteDeRecaudo[]): FuenteDeRecaudo[] {
  if (fuentes.length <= MAX_FUENTES) return fuentes;
  const resto = fuentes.slice(MAX_FUENTES - 1);
  return [
    ...fuentes.slice(0, MAX_FUENTES - 1),
    {
      id: "otras",
      etiqueta: "Otras fuentes",
      categoria: `${resto.length} usuarios`,
      valor: sumarImportes(resto.map((f) => f.valor)),
    },
  ];
}
