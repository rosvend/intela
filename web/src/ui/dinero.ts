// Importes llegan como string decimal en pesos (ADR 0010); se formatean sin pasar por float.
const DECIMAL = /^(-)?(\d+)(?:\.(\d+))?$/;

function agrupar(entero: string): string {
  return entero.replace(/\B(?=(\d{3})+(?!\d))/g, ".");
}

/** "$ 1.234.567,89"; redondea a centavos como sumarImportes y omite los centavos en cero. */
export function formatearCOP(importe: string): string {
  const leido = leer(importe);
  if (!leido) return importe;
  const [signo, enteros, centavos] = aCentavos(
    leido.negativo ? -leido.valor : leido.valor,
    leido.escala,
  );
  const cola = centavos === "00" ? "" : `,${centavos}`;
  return `${signo}$ ${agrupar(enteros)}${cola}`;
}

/** Solo para etiquetas de grafica: pierde precision a proposito. */
export function formatearCOPCompacto(importe: string): string {
  const n = aNumero(importe);
  const abs = Math.abs(n);
  const fmt = (v: number, dec: number) =>
    new Intl.NumberFormat("es-CO", { maximumFractionDigits: dec }).format(v);
  // El umbral se mide sobre lo redondeado: 999.999,99 no puede salir "1.000 mil".
  const cuerpo =
    Math.round(abs / 100) >= 10_000
      ? `${fmt(abs / 1_000_000, 1)} M`
      : Math.round(abs) >= 1_000
        ? `${fmt(abs / 1_000, 1)} mil`
        : fmt(abs, 0);
  const signo = n < 0 && cuerpo !== "0" ? "-" : "";
  return `${signo}$ ${cuerpo}`;
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

type Leido = { negativo: boolean; valor: bigint; escala: number };

function leer(importe: string): Leido | null {
  const m = DECIMAL.exec(importe.trim());
  if (!m) return null;
  const [, signo, entero, fraccion = ""] = m;
  return {
    negativo: Boolean(signo),
    valor: BigInt(entero + fraccion),
    escala: fraccion.length,
  };
}

/** Redondea `total / 10^escala` a centavos, lejos de cero; el cero nunca lleva signo. */
function aCentavos(
  total: bigint,
  escala: number,
): [signo: "" | "-", enteros: string, centavos: string] {
  const abs = total < 0n ? -total : total;
  const centavos =
    escala <= 2
      ? abs * 10n ** BigInt(2 - escala)
      : (abs + 5n * 10n ** BigInt(escala - 3)) / 10n ** BigInt(escala - 2);
  return [
    total < 0n && centavos > 0n ? "-" : "",
    (centavos / 100n).toString(),
    (centavos % 100n).toString().padStart(2, "0"),
  ];
}

/** Suma exacta en BigInt con toda la precision; redondea una vez a centavos, lejos de cero. Lo ilegible lanza RangeError. */
export function sumarImportes(importes: readonly string[]): string {
  const leidos = importes.map((importe) => {
    const leido = leer(importe);
    if (!leido)
      throw new RangeError(`importe ilegible: ${JSON.stringify(importe)}`);
    return leido;
  });
  const escala = Math.max(2, ...leidos.map((l) => l.escala));
  let total = 0n;
  for (const l of leidos) {
    const valor = l.valor * 10n ** BigInt(escala - l.escala);
    total += l.negativo ? -valor : valor;
  }
  const [signo, enteros, centavos] = aCentavos(total, escala);
  return `${signo}${enteros}.${centavos}`;
}
