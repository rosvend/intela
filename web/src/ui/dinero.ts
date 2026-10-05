// Importes llegan como string decimal en pesos (ADR 0010); se formatean sin pasar por float.
const DECIMAL = /^(-)?(\d+)(?:\.(\d+))?$/;

function agrupar(entero: string): string {
  return entero.replace(/\B(?=(\d{3})+(?!\d))/g, ".");
}

/** "$ 1.234.567,89"; los centavos en cero se omiten. */
export function formatearCOP(importe: string): string {
  const m = DECIMAL.exec(importe.trim());
  if (!m) return importe;
  const [, signo, entero, fraccion = ""] = m;
  const centavos = (fraccion + "00").slice(0, 2);
  const cuerpo = `$ ${agrupar(entero.replace(/^0+(?=\d)/, ""))}`;
  const cola = centavos === "00" ? "" : `,${centavos}`;
  return `${signo ? "-" : ""}${cuerpo}${cola}`;
}

/** Solo para etiquetas de grafica: pierde precision a proposito. */
export function formatearCOPCompacto(importe: string): string {
  const n = aNumero(importe);
  const abs = Math.abs(n);
  const signo = n < 0 ? "-" : "";
  const fmt = (v: number, dec: number) =>
    new Intl.NumberFormat("es-CO", { maximumFractionDigits: dec }).format(v);
  if (abs >= 1_000_000) return `${signo}$ ${fmt(abs / 1_000_000, 1)} M`;
  if (abs >= 1_000) return `${signo}$ ${fmt(abs / 1_000, 1)} mil`;
  return `${signo}$ ${fmt(abs, 0)}`;
}

export function aNumero(importe: string): number {
  const n = Number(importe);
  return Number.isFinite(n) ? n : 0;
}

/** Porcentajes (0-100) de cada importe sobre el total positivo; solo para anchos visuales. */
export function proporciones(importes: string[]): number[] {
  const valores = importes.map((v) => Math.max(0, aNumero(v)));
  const total = valores.reduce((a, b) => a + b, 0);
  if (total === 0) return valores.map(() => 0);
  return valores.map((v) => Math.round((v / total) * 10000) / 100);
}
