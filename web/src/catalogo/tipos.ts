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
 * Una entrada del padron de titulares: quien figura ante la sociedad.
 *
 * Trae el padron ENTERO, personas juridicas incluidas, y no es un descuido del
 * contrato: `R-01` (`RD 4.5`) se comprueba al armar una declaracion, no al leer
 * el padron. Recortar aqui a las personas naturales dejaria al editor sin poder
 * explicar por que el titular que se busca no esta entre los elegibles: la
 * regla quedaria invisible y pareceria un dato que falta.
 *
 * La `clase` -socio o administrado- decide quien vota (capitulo 4 del
 * reglamento de socios) y NO quien cobra: un administrado persona natural cobra
 * igual que un socio. Lo que decide es `persona_natural`, y por eso el editor
 * la pinta al lado de `puedeSerParte` y no en lugar de ella.
 */
export type Titular = components["schemas"]["Titular"];

/**
 * Si un valor sin tipar tiene la forma de un `Titular`: los campos que lee el
 * editor de reparto, y solo esos.
 *
 * Vive aqui, junto a `Titular`, como `esObra` vive junto a `Obra`: cada guarda
 * se lee al lado del tipo que comprueba. Campo por campo:
 *
 * - `id`: se pinta, es la clave de la fila del padron y es EXACTAMENTE el
 *   `titular_id` que viaja en la parte que se guarda; ausente o vacio mandaria
 *   al backend una parte que nombra a nadie.
 * - `nombre`: se pinta como texto; un objeto ahi lanzaria "Objects are not
 *   valid as a React child" y tumbaria el padron entero.
 * - `ipi`: se pinta y ademas RELLENA el campo de la fila que se agrega al
 *   borrador; un objeto ahi pintaria "[object Object]" en la parte que se
 *   guarda. Vacio no se rechaza: el contrato solo lo exige a las personas
 *   naturales.
 * - `persona_natural`: **la comprobacion que mas importa de las cinco**. Es lo
 *   unico que decide si el titular se ofrece como parte (ver `puedeSerParte`), y
 *   un `"false"` de texto -o un ausente, que en JavaScript es falsy- se leeria
 *   como "no puede ser parte" y escondereria a un titular que el backend si
 *   acepta: el editor dejaria de ofrecer a quien tiene derecho, en silencio.
 * - `clase`: tiene que ser uno de los DOS valores del enum. No es celo: se
 *   pinta en el padron, y un valor que el sistema no puede producir -el
 *   `sin_clasificar` de `UsuarioRecaudo`, que aqui no existe- seria una
 *   clasificacion inventada en pantalla.
 *
 * `clase` y `nombre` se comprueban porque el editor los pinta; el dia que una
 * columna nueva lea otro campo, se anade aqui, junto al tipo que comprueba.
 */
export function esTitular(valor: unknown): valor is Titular {
  if (typeof valor !== "object" || valor === null) return false;
  const titular = valor as {
    id?: unknown;
    nombre?: unknown;
    ipi?: unknown;
    persona_natural?: unknown;
    clase?: unknown;
  };
  return (
    typeof titular.id === "string" &&
    titular.id !== "" &&
    typeof titular.nombre === "string" &&
    typeof titular.ipi === "string" &&
    typeof titular.persona_natural === "boolean" &&
    (titular.clase === "socio" || titular.clase === "administrado")
  );
}

/**
 * Si un titular del padron puede figurar como parte de una Declaracion de Obra:
 * `R-01` (`RD 4.5`), la misma pregunta que
 * `afiliacion.Titular.PuedeRecibirReparto()` (`internal/dominio/afiliacion/titular.go`).
 *
 * Existe porque el editor tiene que OFRECER solo a quien el backend va a
 * aceptar y EXPLICAR a quien no: un selector que trajera a una productora
 * mandaria al administrador a un 400 que podria haber entendido de antemano.
 *
 * Lo que esta funcion NO es: la barrera. La regla la impone el backend -400 con
 * su propio mensaje, que nombra `R-01` y `RD 4.5`- y esto es azucar de la
 * oferta: si el padron cambiara y esta condicion se quedara corta, lo que pasa
 * es que se ofrece de mas y el servidor rechaza el guardado con su mensaje, que
 * es justo lo que el editor tiene que enseñar. Una condicion escrita dos veces
 * es una condicion que algun dia discrepa; por eso lleva el nombre de la regla
 * en vez de leer la columna a pelo, para que la discrepancia se vea al leerla.
 */
export function puedeSerParte(titular: Titular): boolean {
  return titular.persona_natural;
}

/**
 * Una version de la Declaracion de Obra con su ventana de vigencia. La version
 * VIGENTE es la que tiene `vigente_hasta` en `null`.
 */
export type VersionDeclaracion = components["schemas"]["VersionDeclaracion"];

/**
 * Si un valor sin tipar tiene la forma de una `VersionDeclaracion`: los campos
 * que leen las dos pantallas que consumen el historial -el detalle de obra y el
 * historial de versiones-, y solo esos.
 *
 * - `version`: se compara con `version_vigente` de la obra para no pintar las
 *   partes de una version que el servidor no esta sosteniendo. Un texto ahi
 *   haria fallar esa comparacion siempre, y la pantalla diria que el historial
 *   no trae la version vigente cuando el defecto esta en el payload.
 * - `vigente_desde`: se pinta como instante en el historial. Un objeto ahi
 *   lanzaria "Objects are not valid as a React child" y tumbaria el listado
 *   entero.
 * - `vigente_hasta`: **la comprobacion mas importante de todas**. La version
 *   vigente es la que lo tiene en `null`, y es la unica cuyo reparto rige hoy.
 *   Un payload que no traiga el campo -el de un despliegue a medias, la trampa
 *   que `esObra` ya corta para `version_vigente`- NO puede leerse como `null`:
 *   una version ya CERRADA pasaria por vigente y la pantalla pintaria su reparto
 *   como el reparto de hoy, que es afirmar mas de lo que el sistema sabe.
 * - `estado`: tiene que ser uno de los DOS valores del enum. No es celo: el
 *   historial pinta el estado de CADA version -el suyo, que es un hecho
 *   historico, no el de la obra-, y un `invalida` o cualquier otro texto
 *   acabaria pintado como "Incompleta" o como un estado que el sistema no puede
 *   producir. Aqui es donde se corta, antes de que llegue a la pantalla.
 * - `partes`: es la tabla que se pinta; cada elemento pasa por `esParte`, porque
 *   una lista de verdad con un elemento a medias revienta al dibujarlo.
 *
 * `vigente_desde` y `estado` se anadieron en el paso 7, el dia en que una
 * pantalla los leyo: hasta entonces el detalle de obra solo miraba la ventana
 * para saber cual era la abierta. La guarda es de TODOS los consumidores y no
 * una por pantalla, porque una segunda guarda para la misma forma es la clase
 * de defecto que este archivo existe para evitar: dos listas de campos que se
 * desincronizan sin que nadie lo note.
 */
export function esVersionDeclaracion(
  valor: unknown,
): valor is VersionDeclaracion {
  if (typeof valor !== "object" || valor === null) return false;
  const version = valor as {
    version?: unknown;
    vigente_desde?: unknown;
    vigente_hasta?: unknown;
    estado?: unknown;
    partes?: unknown;
  };
  return (
    Number.isInteger(version.version) &&
    typeof version.vigente_desde === "string" &&
    (version.vigente_hasta === null ||
      typeof version.vigente_hasta === "string") &&
    (version.estado === "completa" || version.estado === "incompleta") &&
    Array.isArray(version.partes) &&
    version.partes.every(esParte)
  );
}
