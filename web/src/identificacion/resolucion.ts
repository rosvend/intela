import { ApiError, ErrorDeCuerpoIlegible, api } from "../api";
import type { components } from "../contrato";
import { RUTAS_IDENTIFICACION } from "./tipos";

/**
 * La escritura de la bandeja: `POST /identificacion/casos/{id}/resolucion`.
 *
 * Es lo UNICO del front que depende del contrato de #175, que al escribir esto
 * vive en una rama sin mergear y entro a esta como commit provisional. Si ese
 * contrato cambia antes de llegar a main, lo que cambia esta aqui y en la
 * lectura del asiento (`leerResolucionAsentada`); `contrato.test.ts` lo avisa.
 */
export type ResolucionDeCaso = components["schemas"]["ResolucionDeCaso"];

/**
 * El tope de la nota, el `maxLength` del contrato. Go cuenta runas tras recortar
 * los extremos; el `maxLength` del navegador cuenta unidades UTF-16, que para el
 * espanol son las mismas y para un emoji son mas: el cliente nunca deja pasar
 * una nota que el servidor rechace por larga.
 */
export const MAX_NOTA = 300;

/**
 * Resuelve un caso. Quien firma lo pone el servidor desde la sesion, no el
 * cuerpo (ADR 0006).
 *
 * El 200 trae el caso resuelto, pero nadie lo lee: la bandeja saca el caso de la
 * lista y el conteo, que es todo lo que cambia en pantalla. Por eso un 2xx con
 * el cuerpo ilegible es exito: `res.ok` ya prueba que la resolucion quedo, y lo
 * unico que no llego es un cuerpo que no se iba a usar (ver
 * `ErrorDeCuerpoIlegible` en `api.ts`). Cualquier otro fallo sube tal cual.
 */
export async function resolverCaso(
  id: string,
  resolucion: ResolucionDeCaso,
): Promise<void> {
  try {
    await api(RUTAS_IDENTIFICACION.resolucion(id), {
      method: "POST",
      body: JSON.stringify(resolucion),
    });
  } catch (error) {
    if (error instanceof ErrorDeCuerpoIlegible) return;
    throw error;
  }
}

/**
 * Las tres causas del 409 de la resolucion, y la de un 409 que no se reconoce.
 *
 * Al front no le da igual cual es: "ya resuelto" y "no pendiente" dicen que el
 * caso ya no esta en la cola, y "alias en conflicto" dice que sigue ahi y que lo
 * que no se puede es ESA asignacion.
 */
export type CausaDeConflicto =
  "ya_resuelto" | "no_pendiente" | "alias_en_conflicto" | "desconocida";

// DECISION #39: el contrato provisional de #175 distingue las tres causas SOLO
// por el mensaje, y se le pidio un `codigo`. Mientras llega, se busca el texto
// del error centinela de Go (`ErrCasoYaResuelto`, `ErrCasoNoPendiente`,
// `ErrAliasEnConflicto`) DENTRO del mensaje y no al principio: Go los envuelve
// con `%w`, y el de alias llega como `resolver el uso "x": ese identificador...`
// aunque el ejemplo del contrato empiece por "ese identificador". Cuando el
// contrato traiga el `codigo`, esta funcion lo lee a el y la lista se va.
const TEXTOS_CENTINELA: readonly (readonly [string, CausaDeConflicto])[] = [
  ["otra persona ya resolvio este caso", "ya_resuelto"],
  ["el caso ya no esta pendiente", "no_pendiente"],
  ["ese identificador ya apunta a otra obra", "alias_en_conflicto"],
];

/** La causa de un 409 de la resolucion, o `null` si el error no es un 409 de la API. */
export function causaDelConflicto(error: unknown): CausaDeConflicto | null {
  if (!(error instanceof ApiError) || error.status !== 409) return null;
  for (const [texto, causa] of TEXTOS_CENTINELA) {
    if (error.message.includes(texto)) return causa;
  }
  return "desconocida";
}
