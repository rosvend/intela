import { useApi, type EstadoDeApi } from "../useApi";

/**
 * Los cuatro desenlaces de LEER una lista, en un solo sitio.
 *
 * Cuatro pantallas del catalogo -el listado, el padron del editor, el detalle y
 * el historial- abrian con la misma cadena: `cargando`, `error`, "el 2xx no
 * llego como una lista legible". `useApi<T[]>` promete una lista y no la
 * comprueba -`T` es una promesa, no una verificacion-, y cada elemento pasa por
 * el guarda de tipo de quien conoce los campos que la pantalla lee. Una lista con
 * un elemento mal formado es `ilegible` ENTERA a proposito: saltarse la fila mala
 * escondería un elemento sin decirlo, y aqui toda cifra se explica hasta su
 * origen.
 *
 * Es el equivalente de `useObra` para una lista. El hook devuelve el desenlace y
 * NO el texto: cada pantalla dice el suyo -los tests los afirman literalmente-.
 */
export type Lista<T> =
  | { estado: "cargando" }
  | { estado: "error"; mensaje: string }
  | { estado: "ilegible" }
  | { estado: "ok"; elementos: T[] };

/**
 * Clasifica una lectura ya hecha. Para quien pide la lista por su cuenta -una
 * pantalla que la sube al componente de la ruta para pedirla junto a otra-.
 */
export function clasificarLista<T>(
  lectura: EstadoDeApi<unknown>,
  esElemento: (valor: unknown) => valor is T,
): Lista<T> {
  if (lectura.cargando) return { estado: "cargando" };
  if (lectura.error) return { estado: "error", mensaje: lectura.error.message };

  const { datos } = lectura;
  return Array.isArray(datos) && datos.every(esElemento)
    ? { estado: "ok", elementos: datos }
    : { estado: "ilegible" };
}

/** Pide `path` y clasifica lo que llega. */
export function useLista<T>(
  path: string,
  esElemento: (valor: unknown) => valor is T,
): Lista<T> {
  return clasificarLista(useApi<unknown>(path), esElemento);
}
