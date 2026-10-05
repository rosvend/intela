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

/** Suma exacta en BigInt con toda la precision; redondea una vez a centavos, lejos de cero. Lo ilegible lanza RangeError. */
export function sumarImportes(importes: readonly string[]): string {
  const leidos = importes.map((importe) => {
    const m = DECIMAL.exec(importe.trim());
    if (!m)
      throw new RangeError(`importe ilegible: ${JSON.stringify(importe)}`);
    const [, signo, entero, fraccion = ""] = m;
    return {
      negativo: Boolean(signo),
      digitos: entero + fraccion,
      escala: fraccion.length,
    };
  });
  const escala = Math.max(2, ...leidos.map((l) => l.escala));
  let total = 0n;
  for (const l of leidos) {
    const valor = BigInt(l.digitos) * 10n ** BigInt(escala - l.escala);
    total += l.negativo ? -valor : valor;
  }
  const negativo = total < 0n;
  const divisor = 10n ** BigInt(escala - 2);
  const abs = negativo ? -total : total;
  const centavos = (abs + divisor / 2n) / divisor;
  const resto = (centavos % 100n).toString().padStart(2, "0");
  return `${negativo && centavos > 0n ? "-" : ""}${centavos / 100n}.${resto}`;
}
