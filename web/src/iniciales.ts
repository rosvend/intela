/**
 * Las iniciales de un nombre para el avatar circular: hasta dos letras, la
 * primera de cada una de las dos primeras palabras.
 *
 * Sale de `Layout.tsx` (D9 del plano de #39): el historial de resoluciones
 * manuales (`identificacion/HistorialResoluciones.tsx`) es su segundo
 * consumidor, para el mismo avatar junto a `actor_nombre`.
 */
export function iniciales(nombre: string): string {
  return nombre
    .trim()
    .split(/\s+/)
    .slice(0, 2)
    .map((parte) => parte[0]?.toUpperCase() ?? "")
    .join("");
}
