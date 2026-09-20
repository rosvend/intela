import { ApiError } from "../api";
import { useApi } from "../useApi";
import { versionAbierta } from "./declaracion";
import { esObra, type Obra, type VersionDeclaracion } from "./tipos";

/**
 * La lectura de una obra, en un solo sitio (issue #135, item 13).
 *
 * Las tres pantallas de la obra -el detalle (paso 6), el historial (paso 7) y el
 * editor del reparto (paso 8)- abrian con el MISMO preambulo copiado: pedir
 * `GET /obras/{id}` con `useApi`, decidir si el fallo es un 404 -el servidor
 * diciendo que con ese identificador no hay ninguna obra, que no es un fallo del
 * sistema- o un fallo de verdad, y comprobar con `esObra` que lo que llego tiene
 * la forma que el contrato promete. Eran ~35 lineas por pantalla, con tres
 * copias del mismo `ObraAusente` y tres oportunidades de que una se quedara
 * atras.
 *
 * Aqui vive ese preambulo UNA vez. Lo que NO vive aqui es el historial: cada
 * pantalla lo pide cuando lo necesita -el detalle solo si la obra declara una
 * version vigente, el historial y el editor siempre-, y esa diferencia es
 * comportamiento medido, no un descuido. Por eso la conciliacion entre las dos
 * lecturas es una funcion aparte -`conciliarConElHistorial`- y no un efecto
 * dentro del hook.
 *
 * `useApi` sigue siendo el unico que habla con la red: este modulo no añade una
 * capa de peticiones, solo le pone nombre a los desenlaces.
 */

/**
 * Lo que puede devolver la LECTURA de una obra: pedirla y comprobar que llego.
 *
 * Cinco desenlaces y no cuatro. Los cuatro del item 13 -`cargando`, `ok`,
 * `ausente`, `error`- mas el que las tres pantallas ya distinguian antes de este
 * paso: un 2xx cuyo cuerpo NO tiene la forma de una obra. `useApi<Obra>` promete
 * una obra y no la comprueba -`T` es una promesa, no una verificacion-, y sin
 * `ErrorBoundary` en `web/src` el primer campo mal formado deja la pantalla en
 * blanco; por eso las tres preguntaban por `esObra` antes de leer nada. Meterlo
 * aqui no añade un estado: no pierde el que ya estaba.
 *
 * `error` NO incluye el 404. Un 404 no es un fallo del sistema: es el servidor
 * diciendo que con ese identificador no hay ninguna obra, y pintarlo con el
 * error generico diria que algo se rompio cuando lo que pasa es que esa obra ya
 * no esta. Es el caso propio del issue #30 -una obra que se listo hace un
 * momento y ya no esta- y por eso tiene su propio desenlace.
 */
export type LecturaDeObra =
  | { estado: "cargando" }
  | { estado: "ausente"; id: string }
  | { estado: "error"; mensaje: string }
  | { estado: "ilegible" }
  | { estado: "ok"; obra: Obra };

/**
 * La quinta salida: hay OBRA, hay HISTORIAL, y las dos lecturas del mismo hecho
 * -cual version rige- no cuadran.
 *
 * No es `ausente` -la obra esta- ni `error` -las dos peticiones contestaron
 * bien-: es el servidor declarando vigente una version que su historial no trae
 * como unica abierta. Pasa cuando alguien declara entre las dos peticiones, o
 * cuando el historial viene recortado.
 *
 * Existe como desenlace propio porque las dos pantallas que pueden verlo NO
 * reaccionan igual, y ninguna de las dos reacciones se puede perder:
 *
 * - el detalle NO PINTA las partes: pintar las de otra version como la
 *   declaracion vigente afirmaria un reparto que `GET /obras/{id}` no esta
 *   sosteniendo (item 7, `DetalleObra.tsx`);
 * - el editor NO OFRECE GUARDAR, porque guardar cerraria una version que el
 *   sistema no esta sosteniendo. La que lee se niega a pintar; la que ESCRIBE se
 *   niega a escribir (item 7, `EditorReparto.tsx`).
 *
 * Las otras dos formas de la conciliacion son las que SI cuadran: `vigente`
 * -cuadran, y la version abierta es la que la obra declara, que es lo que el
 * detalle necesita para sus partes- y `sinVigente` -la obra no declara ninguna
 * version vigente y el historial no abre ninguna, que es la obra a la que el
 * editor le va a abrir la primera-.
 */
export type Conciliacion =
  | { estado: "vigente"; vigente: VersionDeclaracion }
  | { estado: "sinVigente" }
  | { estado: "descuadrado"; mensaje: string };

/**
 * El vocabulario completo de una obra: los desenlaces de LEERLA y los de
 * conciliarla con su historial. `descuadrado` es el quinto, y esta en la misma
 * union que los otros cuatro a proposito: una union que solo pudiera expresar
 * `cargando` / `ok` / `ausente` / `error` obligaria a cada pantalla a inventar
 * por su cuenta como decir "hay obra pero el historial no cuadra", que es
 * exactamente el defecto del que sale este paso.
 */
export type EstadoDeObra = LecturaDeObra | Conciliacion;

/**
 * La obra del `id` de la direccion, en uno de los cinco desenlaces de arriba.
 *
 * `id` entra tal como lo dejo `useParams` y se codifica aqui: la misma
 * `encodeURIComponent` que usaban las tres pantallas, escrita una vez. Un `id`
 * vacio no se distingue de uno que no existe -los dos acaban en el 404 del
 * servidor-, y esa es la lectura correcta: sin identificador no hay obra.
 *
 * No devuelve `descuadrado`: para eso hay que haber leido el historial, y quien
 * sabe cuando pedirlo es la pantalla -ver `conciliarConElHistorial`-.
 */
export function useObra(id: string): LecturaDeObra {
  const { datos, cargando, error } = useApi<Obra>(
    `/api/obras/${encodeURIComponent(id)}`,
  );

  if (cargando) return { estado: "cargando" };

  if (error) {
    return error instanceof ApiError && error.status === 404
      ? { estado: "ausente", id }
      : { estado: "error", mensaje: error.message };
  }

  return esObra(datos) ? { estado: "ok", obra: datos } : { estado: "ilegible" };
}

/**
 * Si la version que el historial deja abierta es la que la obra declara vigente.
 *
 * `historial` NO admite `null` -"no se pudo leer"- porque no es una lectura: es
 * la ausencia de una, y fingir un desenlace para ella seria decir de la obra
 * algo que nadie comprobo. Quien no haya leido el historial no tiene nada que
 * conciliar y decide por su cuenta, que es lo que hace el editor: no haber
 * leido el historial ya tiene su aviso (`avisoDeVersion`) y prohibir el guardado
 * por eso seria cambiar una advertencia por una puerta cerrada.
 *
 * La version abierta se resuelve con `versionAbierta` (`declaracion.ts`), que es
 * la que ya usaba el editor: `null` cuando no hay exactamente una, o sea tambien
 * cuando hay dos -y con dos abiertas el backend cerraria otra cosa-. El detalle
 * comprobaba lo mismo con un `filter` propio mas un `abiertas.length > 1`
 * explicito; esta funcion dice lo mismo de una sola forma, que es el punto del
 * item 13: dos expresiones de la misma regla son dos sitios donde equivocarse.
 */
export function conciliarConElHistorial(
  obra: Obra,
  historial: readonly VersionDeclaracion[],
): Conciliacion {
  const abierta = versionAbierta(historial);

  if (abierta !== null && abierta.version === obra.version_vigente) {
    return { estado: "vigente", vigente: abierta };
  }

  if (abierta === null && obra.version_vigente === null) {
    return { estado: "sinVigente" };
  }

  return {
    estado: "descuadrado",
    mensaje: mensajeDeDescuadre(obra.version_vigente),
  };
}

/**
 * La frase con la que las dos pantallas dicen el descuadre. El texto de la
 * consecuencia NO va aqui: el detalle dice "no se muestran partes" y el editor
 * "no ofrece guardar", que son dos hechos distintos del mismo diagnostico.
 *
 * `version_vigente` en `null` -la obra no declara ninguna- tiene su propia
 * entrada: decir "el servidor declara vigente la version null" seria afirmar un
 * numero que no existe.
 */
function mensajeDeDescuadre(versionVigente: number | null): string {
  return versionVigente === null
    ? "El servidor no declara ninguna versión vigente, pero el historial no trae esa versión como única versión abierta."
    : `El servidor declara vigente la versión ${versionVigente}, pero el historial no trae esa versión como única versión abierta.`;
}
