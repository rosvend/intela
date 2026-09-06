export function formatearEntero(n: number): string {
  return new Intl.NumberFormat("es-CO").format(n);
}

/**
 * Importe neto como string decimal. Agrupa miles y antepone `$` sin
 * convertirlo a number: el tablero no hace aritmetica sobre la cifra.
 */
export function formatearImporte(neto: string): string {
  const negativo = neto.startsWith("-");
  const absoluto = negativo ? neto.slice(1) : neto;
  const [entero = "0", fraccion] = absoluto.split(".");
  const agrupado = entero.replace(/\B(?=(\d{3})+(?!\d))/g, ".");
  const decimales = fraccion !== undefined ? `,${fraccion}` : "";
  return `${negativo ? "-" : ""}$ ${agrupado}${decimales}`;
}
