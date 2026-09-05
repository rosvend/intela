export function formatearEntero(n: number): string {
  return new Intl.NumberFormat("es-CO").format(n);
}
