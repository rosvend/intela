/**
 * Navegacion de la bandeja sobre el orden de llegada. `ids` es la pagina
 * entera y `visibles` lo que sigue en la cola: el caso en foco puede haber
 * salido ya (se esta guardando, o un 409 lo saco) y aun asi tiene vecinos.
 */
export function vecinoEnCola(
  ids: readonly string[],
  visibles: ReadonlySet<string>,
  desde: string,
  paso: 1 | -1,
): string | null {
  const inicio = ids.indexOf(desde);
  if (inicio === -1) return null;
  for (let i = inicio + paso; i >= 0 && i < ids.length; i += paso) {
    const id = ids[i]!;
    if (visibles.has(id)) return id;
  }
  return null;
}

/** Posicion (desde 1) del caso entre los visibles; `null` si ya salio. */
export function posicionEnCola(
  ids: readonly string[],
  visibles: ReadonlySet<string>,
  id: string,
): number | null {
  if (!visibles.has(id)) return null;
  return ids.filter((x) => visibles.has(x)).indexOf(id) + 1;
}
