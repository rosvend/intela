import type { components } from "../contrato";

/**
 * Los tipos de red del catalogo salen del contrato generado
 * (`npm run contrato`), nunca escritos a mano: dos fuentes de verdad para la
 * misma forma es la clase de defecto que el repo ya pago (D-002). Los nombres
 * de campo se quedan en `snake_case`, como los sirve la API.
 */
export type Obra = components["schemas"]["Obra"];

/**
 * Los DOS valores que el backend puede persistir. No hay un tercero, y no es
 * un olvido: lo que el sistema rechaza es DECLARAR una suma por encima de 100
 * -un 400 en la escritura, con `NuevaDeclaracion` del dominio-, asi que un
 * estado `invalida` no puede quedar guardado ni, por tanto, aparecer en un
 * listado. Cualquier pantalla que lo pintara estaria afirmando algo que el
 * sistema no sabe.
 */
export type EstadoDeclaracion = Obra["estado_declaracion"];

/**
 * Si un valor sin tipar tiene la forma de una `Obra`: los campos que lee este
 * modulo, y solo esos.
 *
 * Vive aqui, junto a `Obra`, como `esCarga` vive junto a `Carga` y `esRechazo`
 * junto a `Rechazo`: cada guarda se lee al lado del tipo que comprueba. Existe
 * porque `useApi<Obra[]>` promete una lista pero no la comprueba -`T` es una
 * promesa, no una verificacion-, asi que un 2xx con otra forma llega hasta la
 * tabla; y porque una lista de verdad con un elemento a medias revienta al
 * pintarlo. Sin ErrorBoundary en `web/src`, eso deja la pantalla en blanco.
 *
 * Campo por campo, lo que se comprueba y por que:
 *
 * - `id`: se pinta y es la clave de la fila; un id ausente o vacio rompe React.
 * - `titulo`, `genero` y `tipo`: se pintan como texto. Un objeto ahi lanzaria
 *   "Objects are not valid as a React child" y tumbaria la tabla entera.
 * - `anio`: se pinta tal cual, sin agrupar miles (un anio no es una cifra
 *   contable); un objeto ahi es el mismo caso que arriba.
 * - `ida`: el contrato lo declara opcional y de texto -vacio quiere decir "no
 *   se conoce"-, asi que ausente vale y cualquier otra cosa no.
 * - `estado_declaracion`: tiene que ser uno de los DOS valores del enum. Esto
 *   no es celo: un `invalida` -o cualquier otro texto- pintado tal cual seria
 *   un estado que el sistema no puede producir, y este es el sitio donde se
 *   corta antes de que llegue a la pantalla.
 * - `suma_porcentajes`: un numero finito, porque se formatea con `toFixed` y
 *   un texto ahi pintaria "NaN%".
 * - `version_vigente`: **la comprobacion mas importante de todas**. El contrato
 *   lo declara `number | null`, y `null` es una afirmacion del backend -"esta
 *   obra no tiene ninguna declaracion"-, no la ausencia del dato. Un payload
 *   que no traiga el campo -el de un despliegue a medias, el caso que el
 *   Pre-Mortem del plan llama trampa- NO puede leerse como `null`: la pantalla
 *   diria "Sin declaracion" sobre una obra que quiza si tiene una, que es
 *   afirmar mas de lo que el sistema sabe. Aqui se rechaza el elemento y el
 *   listado entero queda en error, a proposito, en vez de esconder la fila.
 *
 * `coautores`, `eidr` e `imdb` no se miran porque ningun consumidor de esta
 * pantalla los lee: la tabla del catalogo muestra el titulo, el genero, el
 * anio, el tipo, el IDA y la declaracion, y nada mas. El dia que una columna
 * los lea, se anaden aqui, junto al tipo que comprueban.
 */
export function esObra(valor: unknown): valor is Obra {
  if (typeof valor !== "object" || valor === null) return false;
  const obra = valor as {
    id?: unknown;
    titulo?: unknown;
    genero?: unknown;
    anio?: unknown;
    tipo?: unknown;
    ida?: unknown;
    estado_declaracion?: unknown;
    suma_porcentajes?: unknown;
    version_vigente?: unknown;
  };
  return (
    typeof obra.id === "string" &&
    obra.id !== "" &&
    typeof obra.titulo === "string" &&
    typeof obra.genero === "string" &&
    Number.isInteger(obra.anio) &&
    typeof obra.tipo === "string" &&
    (obra.ida === undefined || typeof obra.ida === "string") &&
    (obra.estado_declaracion === "completa" ||
      obra.estado_declaracion === "incompleta") &&
    Number.isFinite(obra.suma_porcentajes) &&
    (obra.version_vigente === null || Number.isInteger(obra.version_vigente))
  );
}

/**
 * Una parte de una version de la Declaracion de Obra: lo que un titular
 * declaro sobre una obra. Es el unico origen valido de un porcentaje de reparto
 * (`R-03`).
 *
 * El NOMBRE del titular no viaja aqui, y no es un olvido: la parte trae
 * `titular_id` e `ipi`, que son la identidad que la declaracion guarda. El
 * nombre, cuando se muestre, se resuelve contra el padron y va rotulado como el
 * del padron de hoy (D-006).
 */
export type Parte = components["schemas"]["Parte"];

/**
 * Si un valor sin tipar tiene la forma de una `Parte`: los campos que lee la
 * tabla de partes del detalle de obra, y solo esos.
 *
 * Vive aqui, junto a `Parte`, como `esObra` vive junto a `Obra`: cada guarda se
 * lee al lado del tipo que comprueba. Campo por campo:
 *
 * - `titular_id`: se pinta, es la clave de la fila y es lo que deja conciliar la
 *   pantalla con la API; ausente o vacio rompe React.
 * - `ipi`: se pinta como texto; un objeto ahi lanzaria "Objects are not valid as
 *   a React child" y tumbaria la tabla entera. Vacio no se rechaza: el contrato
 *   solo lo declara obligatorio, y el padron admite un titular sin IPI.
 * - `porcentaje`: **la comprobacion que mas importa de las tres**. Se formatea
 *   con `toFixed`, y un texto -el defecto que el backend tenia y que el paso 4
 *   corrigio con una prueba- hace que `toFixed` no exista y la celda tumbe la
 *   pantalla; un `NaN` pintaria "NaN%" sin decir que la cifra no llego.
 */
export function esParte(valor: unknown): valor is Parte {
  if (typeof valor !== "object" || valor === null) return false;
  const parte = valor as {
    titular_id?: unknown;
    ipi?: unknown;
    porcentaje?: unknown;
  };
  return (
    typeof parte.titular_id === "string" &&
    parte.titular_id !== "" &&
    typeof parte.ipi === "string" &&
    Number.isFinite(parte.porcentaje)
  );
}

/**
 * Una version de la Declaracion de Obra con su ventana de vigencia. La version
 * VIGENTE es la que tiene `vigente_hasta` en `null`.
 */
export type VersionDeclaracion = components["schemas"]["VersionDeclaracion"];

/**
 * Si un valor sin tipar tiene la forma de una `VersionDeclaracion`: los campos
 * que lee el detalle de obra, y solo esos.
 *
 * - `version`: se compara con `version_vigente` de la obra para no pintar las
 *   partes de una version que el servidor no esta sosteniendo. Un texto ahi
 *   haria fallar esa comparacion siempre, y la pantalla diria que el historial
 *   no trae la version vigente cuando el defecto esta en el payload.
 * - `vigente_hasta`: **la comprobacion mas importante de todas**. La version
 *   vigente es la que lo tiene en `null`, y es la unica cuyo reparto rige hoy.
 *   Un payload que no traiga el campo -el de un despliegue a medias, la trampa
 *   que `esObra` ya corta para `version_vigente`- NO puede leerse como `null`:
 *   una version ya CERRADA pasaria por vigente y la pantalla pintaria su reparto
 *   como el reparto de hoy, que es afirmar mas de lo que el sistema sabe.
 * - `partes`: es la tabla que se pinta; cada elemento pasa por `esParte`, porque
 *   una lista de verdad con un elemento a medias revienta al dibujarlo.
 *
 * `vigente_desde` y `estado` no se miran porque ningun consumidor de esta
 * pantalla los lee: la ventana de vigencia es el objeto del historial (paso 7),
 * y el estado que se pinta es el de la obra -`GET /obras/{id}`, que es la
 * autoridad-, no el de la version. El dia que una pantalla los lea, se anaden
 * aqui, junto al tipo que comprueban.
 */
export function esVersionDeclaracion(
  valor: unknown,
): valor is VersionDeclaracion {
  if (typeof valor !== "object" || valor === null) return false;
  const version = valor as {
    version?: unknown;
    vigente_hasta?: unknown;
    partes?: unknown;
  };
  return (
    Number.isInteger(version.version) &&
    (version.vigente_hasta === null ||
      typeof version.vigente_hasta === "string") &&
    Array.isArray(version.partes) &&
    version.partes.every(esParte)
  );
}
