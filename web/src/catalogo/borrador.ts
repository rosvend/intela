/**
 * La parte pura del editor de reparto: la rejilla del borrador y el guardado,
 * sin React y sin red, para poder probarla directamente.
 *
 * Este modulo existe por el item 12 del plan de #135: estas funciones vivian
 * dentro de `EditorReparto.tsx`, donde la unica forma de ejercitarlas era montar
 * router y `fetch` desde un test de integracion. Eso costaba dos cosas, y la
 * segunda es la que importa:
 *
 * - una prueba por regla costaba una pantalla entera, asi que la mayoria de las
 *   reglas no tenia ninguna;
 * - las que si tenia quedaban probadas **de refilon**, y una regla de precedencia
 *   se puede cumplir de casualidad: el orden de las guardas de `avisoDeVersion`
 *   -que se movio a `declaracion.ts`-, mutado, paso 333 de 333 pruebas en verde
 *   en el PR #30. Una prueba que no cae cuando se rompe lo que dice probar no es
 *   una prueba.
 *
 * La frontera de este archivo es la misma que la de `declaracion.ts`, y se
 * respeta igual: NADA de aqui decide el estado ni la suma de una declaracion
 * PERSISTIDA -eso lo calcula el backend y viaja en `estado_declaracion` y
 * `suma_porcentajes`-. Lo que vive aqui es el BORRADOR, que no existe para el
 * servidor, y la traduccion del guardado a lo que la pantalla sabe de el.
 *
 * Lo que NO entra aqui, aunque se le parezca: `alPulsarTecla` (el Enter que no
 * guarda). Recibe un `KeyboardEvent` de React y lee el DOM -`tagName`, `type`-,
 * asi que moverla obligaria a importar React en este modulo y traeria un tipo de
 * un framework a un fichero que existe para no depender de ninguno. Se queda en
 * el componente, que es donde vive el formulario que la recibe.
 */
import { ApiError, ErrorDeCuerpoIlegible, ErrorDeRed } from "../api";
import { versionAbierta } from "./declaracion";
import type { VersionDeclaracion } from "./tipos";

/**
 * Una fila del borrador: lo que se esta editando, que NO es todavia una `Parte`.
 *
 * La diferencia no es cosmetica. `Parte` tiene `porcentaje: number` porque es
 * lo que viaja por la red, y el porcentaje de una fila sale de un campo de
 * texto: mientras se teclea vale "", "3" o "3," y nada de eso es un numero. Con
 * el porcentaje tipado como numero habria que inventar un 0 al leer el campo
 * vacio, y ese 0 se guardaria como un porcentaje declarado -un titular al 0% no
 * es titular- sin que nadie lo haya escrito. El texto se convierte una sola vez,
 * al guardar, y sin redondearlo: mas de 4 decimales lo rechaza el backend y el
 * cliente no lo maquilla.
 */
export type FilaDeReparto = {
  /** Clave de React. No puede ser el `titular_id`: la fila sobrevive a cambios. */
  clave: string;
  titularId: string;
  ipi: string;
  /** El porcentaje tal como se teclea, sin interpretar. */
  porcentaje: string;
  /**
   * El nombre del titular, o "" cuando la fila no lo conoce.
   *
   * Lo conoce la fila que se agrega desde el padron: el `Titular` que llega al
   * pulsar ya trae el nombre, y guardarlo aqui es lo que deja que la columna lo
   * diga -nadie mas lo tiene: la rejilla no pide el padron-. NO lo conocen las
   * filas sembradas desde el historial, y no es un dato que falte: la `Parte` no
   * lleva nombre (D-006), asi que esa fila se queda con su `titular_id` -que es lo
   * que la columna pinta- sin inventar hueco ni guion.
   *
   * Vacio es "no se conoce", y de ahi salen las dos lecturas: la primera columna
   * de la rejilla, que pinta el nombre junto al identificador solo cuando lo hay,
   * y `comoSeNombraLaFila`, que es como la fila se dice en voz alta.
   */
  nombre: string;
};

// Contador de claves. Un `crypto.randomUUID` ataria los tests al entorno, y el
// indice de la fila no sirve: al quitar una fila del medio las claves se
// correrian y React reutilizaria el estado de la fila equivocada.
let contadorDeFilas = 0;

export function nuevaFila(
  titularId = "",
  ipi = "",
  porcentaje = "",
  nombre = "",
): FilaDeReparto {
  contadorDeFilas += 1;
  return {
    clave: `fila-${contadorDeFilas}`,
    titularId,
    ipi,
    porcentaje,
    nombre,
  };
}

/**
 * El nombre con el que la rejilla se refiere a una fila de viva voz, que en la
 * practica es el nombre accesible del boton que la quita: el del titular si la
 * fila lo sabe, y su `titular_id` si no.
 *
 * Es una sola regla y no dos condicionales sueltos, porque el respaldo no es un
 * detalle de redaccion: un `titular_id` es un identificador opaco en un lector de
 * pantalla -"Quitar a tit-3 del reparto" nombra, pero no dice de quien- y no hay
 * nombre que inventar cuando la fila no lo tiene. La que nace del padron lo trae
 * porque `agregarTitular` lo guarda; la que nace del historial no PUEDE tenerlo,
 * porque la `Parte` no lleva nombre (D-006). Se cae al identificador, que es el
 * dato que esa fila si tiene, en vez de a un hueco.
 */
export function comoSeNombraLaFila(fila: FilaDeReparto): string {
  return fila.nombre === "" ? fila.titularId : fila.nombre;
}

/**
 * El porcentaje de una fila leido de su campo de texto.
 *
 * Dos decisiones, las dos por lo que se teclea:
 *
 * - la coma se lee como separador decimal. El dominio escribe los porcentajes
 *   con punto -`formatearPorcentaje`, y el mensaje del backend-, pero quien
 *   teclea en un teclado en espanol escribe "33,5", y `Number("33,5")` es `NaN`:
 *   la fila contaria como cero y el total diria una cifra que no esta escrita;
 * - un campo vacio NO es un cero. `Number("")` vale 0, y con ese 0 el borrador
 *   diria que hay un porcentaje declarado donde no hay ninguno. Se devuelve
 *   `NaN`, que `totalDeclarado` cuenta como nada.
 *
 * Devuelve `NaN` tambien para lo que no sea un numero, y eso es lo que la
 * pantalla comprueba antes de guardar: el cuerpo que espera el servidor lleva
 * un numero en cada parte, asi que una fila sin numero legible no se manda.
 */
export function porcentajeDeTexto(texto: string): number {
  const limpio = texto.trim().replace(",", ".");
  if (limpio === "") return Number.NaN;
  return Number(limpio);
}

/**
 * Un porcentaje que viene del backend, escrito para un campo de texto.
 *
 * No se usa `formatearPorcentaje`: el campo no lleva el "%" -no se puede
 * volver a leer- y los cuatro decimales forzados serian ruido en un valor que
 * se va a editar. `String` es la vuelta atras exacta del numero que llego.
 */
export function textoDePorcentaje(valor: number): string {
  return String(valor);
}

/**
 * El reparto vigente, convertido en el borrador de partida.
 *
 * Se arranca del reparto que rige hoy y no de una pantalla en blanco: lo que
 * `PUT /obras/{id}/declaracion` hace es SUSTITUIR el reparto entero, asi que
 * quien viene a cambiar un porcentaje tiene que tener delante los que hay. Con
 * el historial vacio no hay nada que traer y el borrador empieza sin filas; con
 * el historial ilegible tampoco -no se sabe que hay-, y el aviso de la version
 * lo dice.
 */
export function filasDePartida(
  versiones: readonly VersionDeclaracion[],
): FilaDeReparto[] {
  const abierta = versionAbierta(versiones);
  if (!abierta) return [];
  // Las filas sembradas aqui nacen SIN nombre, y no es un olvido: la `Parte` trae
  // `titular_id` e `ipi` y nada mas. Lo que NO hay que hacer es rellenarlo pidiendo
  // el padron -ni una peticion, ni una por fila-: seria poner el nombre de HOY a un
  // reparto que es de una version, que es justo lo que la columna rotulada del
  // detalle declara (D-006), y `GET /titulares` se sirve paginado y no filtra por
  // identificador, asi que una pagina que no traiga al titular no probaria que no
  // exista. La fila se queda con su `titular_id`, que es lo que sabe.
  return abierta.partes.map((parte) =>
    nuevaFila(parte.titular_id, parte.ipi, textoDePorcentaje(parte.porcentaje)),
  );
}

/**
 * El status con que fallo el guardado. "red" si no hubo respuesta
 * (`ErrorDeRed`), "desconocido" si fallo algo que no es una respuesta del
 * servidor con status.
 *
 * "desconocido" **no** es el 2xx cuyo cuerpo no se pudo leer: eso ya no es un
 * fallo del guardado sino de su cuerpo, y tiene su propio desenlace
 * (`ErrorDeCuerpoIlegible`, ver `falloDelGuardado`).
 *
 * No es el `resultadoDeError` de la ingesta, y la diferencia importa: alli el
 * 502/504 tiene caso propio porque su cuerpo es la pagina HTML de nginx y
 * porque el 409 de una entrega repetida es irreversible. Aqui no hay ningun
 * status con esa asimetria: lo que decide es si el COMMIT pudo haber entrado, y
 * eso es todo 5xx -y tambien el 502 y el 504, que son 5xx-, mas la falta de
 * respuesta. Una condicion, no una lista que se queda corta el dia que el proxy
 * contesta otro codigo.
 */
export type FalloDelGuardado = {
  status: number | "red" | "desconocido";
  mensaje: string;
};

/**
 * Si un fallo deja abierta la pregunta de si la version se abrio.
 *
 * Un 5xx NO garantiza que no se guardara nada: `Store.Guardar` corre dentro de
 * `EnTransaccion` (`internal/infraestructura/postgres/declaraciones.go:67`) y el
 * fallo del COMMIT sube como un error mas, indistinguible de uno que no escribio
 * nada -la misma ambiguedad que #29 documento para la ingesta en D-016/D-017-.
 * Afirmar "no se guardo nada" seria decir mas de lo que el sistema sabe, que es
 * justo el defecto que este paso tiene que no repetir.
 *
 * Un 4xx, en cambio, cierra la pregunta -y la cierra por un mecanismo distinto
 * en cada uno, que es lo que este comentario decia mal: NO es que el enrutado
 * los produzca antes de tocar la base-. El 403 si sale de `requiereRol`, en el
 * grupo de rutas y antes del handler; pero el 404 NO lo produce el enrutado:
 * sale de DENTRO de `Gestion.Guardar` -el `SELECT ... FOR UPDATE` sobre `obras`-
 * igual que el 400 de `ErrTitularInexistente` -la clave foranea de
 * `declaraciones`-, y los dos corren dentro de la transaccion. Los cuatro
 * mecanismos, uno por uno y con su prueba en el codigo de Go, estan escritos en
 * `avisoDelRechazo`, que es el unico sitio donde viven: repetirlos aqui daria
 * dos fuentes para el mismo hecho, y la que se quede sin actualizar es la que
 * miente. Lo unico que los cuatro comparten, y lo unico que esta funcion
 * necesita, es que un 4xx es una respuesta del servidor en la que no quedo
 * ninguna version abierta.
 *
 * El conjunto que entra aqui no es "el 5xx y la falta de respuesta", que es lo
 * que decia este comentario: entra ademas "desconocido", o sea cualquier fallo
 * que no sea una respuesta del servidor con status. Y no entra ningun 2xx: un
 * 2xx ya respondio sin error, asi que que su cuerpo no se pueda leer no deja
 * ninguna duda sobre si el servidor escribio -de eso se ocupa
 * `ErrorDeCuerpoIlegible`-.
 */
export function puedeHaberGuardado(
  status: FalloDelGuardado["status"],
): boolean {
  return status === "red" || status === "desconocido" || status >= 500;
}

/**
 * Traduce lo que lanza `api()` al guardar en un `ResultadoDelGuardado`. Nunca
 * lanza, y es donde se decide cual de los desenlaces de fallo es:
 *
 * - el **2xx cuyo cuerpo no se pudo leer** (`ErrorDeCuerpoIlegible`) va a
 *   `guardadaSinLeer`, que es el mismo desenlace del 200 con un cuerpo que no
 *   tiene la forma del contrato. **No es un guardado incierto**: `res.ok` prueba
 *   que el servidor contesto sin error, o sea que una version se abrio -el
 *   handler escribe despues de confirmar la transaccion-, y lo unico que falta es
 *   su numero. Mandarlo a `incierta` afirmaba de menos y nombraba una causa -"una
 *   respuesta que se perdio"- que no ocurrio.
 * - el **guardado incierto**, que es el unico que deja la pregunta abierta: un
 *   5xx, una respuesta que no llego y cualquier fallo que no sea una respuesta
 *   del servidor con status. Ver `puedeHaberGuardado`.
 * - el **4xx**, que cierra la pregunta con el mensaje del servidor.
 */
export function falloDelGuardado(error: unknown): ResultadoDelGuardado {
  // El 2xx con un cuerpo ilegible no es un fallo del guardado: el guardado
  // ocurrio. Se nombra ANTES que los demas porque es el unico camino en que el
  // status es de exito.
  if (error instanceof ErrorDeCuerpoIlegible) {
    return { tipo: "guardadaSinLeer" };
  }
  const fallo: FalloDelGuardado =
    error instanceof ApiError
      ? { status: error.status, mensaje: error.message }
      : error instanceof ErrorDeRed
        ? { status: "red", mensaje: error.message }
        : { status: "desconocido", mensaje: "error desconocido al guardar" };
  const { status, mensaje } = fallo;
  // El unico fallo que cierra la pregunta es un status con numero que no sea un
  // 5xx: "red" y "desconocido" no tienen numero, y de ahi no se sabe nada.
  if (typeof status === "number" && !puedeHaberGuardado(status)) {
    return { tipo: "rechazada", status, mensaje };
  }
  return { tipo: "incierta", status, mensaje };
}

/**
 * Lo que devolvio el guardado.
 *
 * - `guardada`: el servidor contesto 200 con la version nueva, y la version
 *   nueva es un dato suyo: su numero, su estado y su ventana se pintan tal cual;
 * - `guardadaSinLeer`: 200 con un cuerpo que no se pudo leer. Dos caminos llegan
 *   aqui y son el mismo desenlace: un cuerpo que no se pudo ni leer como JSON
 *   (`ErrorDeCuerpoIlegible`, `api.ts`) y uno que se leyo pero no tiene la forma
 *   del contrato (`esVersionDeclaracion`). El 200 prueba que la version se abrio
 *   -el handler lo escribe despues de confirmar la transaccion-, pero el numero
 *   no se puede afirmar, asi que no se afirma: se dice que se guardo y que el
 *   historial es donde se comprueba;
 * - `rechazada`: un 4xx con su mensaje, que es lo unico que explica cual de las
 *   reglas de escritura se incumplio;
 * - `incierta`: un 5xx o la falta de respuesta. Ver `puedeHaberGuardado`.
 */
export type ResultadoDelGuardado =
  | { tipo: "guardada"; version: VersionDeclaracion }
  | { tipo: "guardadaSinLeer" }
  | { tipo: "rechazada"; status: number; mensaje: string }
  | {
      tipo: "incierta";
      status: FalloDelGuardado["status"];
      mensaje: string;
    };

/** El titulo del rechazo. Solo afirma lo que el status dice. */
export function tituloDelRechazo(status: number): string {
  if (status === 400) return "El servidor rechazó la declaración";
  if (status === 404) return "Esa obra no está en el catálogo";
  if (status === 401 || status === 403)
    return "El servidor no autoriza este guardado";
  return "El servidor rechazó la declaración";
}

/**
 * El aviso que cierra la pregunta de si algo quedó escrito, con el texto que le
 * toca a cada status.
 *
 * La consecuencia solo se pintaba en el 400, y eso es afirmar menos de lo que el
 * sistema sabe: en los CUATRO la version nueva no se abrio, y quien edita se
 * queda sin saber si puede reintentar con tranquilidad -que en este sistema
 * importa, porque guardar dos veces abre una version de mas-. Ademas la razon
 * que justificaba pintarlo solo en el 400 era demasiado amplia: "el 400 lo
 * produce la validacion antes de escribir nada" no dice nada de los otros tres.
 *
 * El hecho verdadero, mas estrecho, es que **ningun 4xx de esta ruta deja una
 * version abierta**, y el mecanismo no es el mismo en los cuatro:
 *
 * - 400 del cuerpo que no es una lista de partes
 *   (`guardarDeclaracion`, antes de `GuardarSplits`), 400 de
 *   `repertorio.NuevaDeclaracion` (suma pasada de 100, IPI que falta, titular
 *   repetido: puro y local) y 400 de `R-01`
 *   (`ErrTitularNoEsPersonaNatural`, en `exigirPuedenRecibirReparto`): los tres
 *   salen ANTES de `Gestion.Guardar`, que es el unico camino que escribe. Esto
 *   es lo unico que decia el comentario viejo, y solo vale para estos tres.
 * - 400 de `ErrTitularInexistente` y 404 de `ErrNoEncontrado`: estos dos SI se
 *   producen DENTRO de `Gestion.Guardar` -la FK de `declaraciones` y el
 *   `SELECT ... FOR UPDATE` sobre `obras`-, asi que "ningun 4xx llega al camino
 *   que escribe" seria falso y no se puede escribir. Lo que los hace
 *   inofensivos es que salen como error de la funcion que corre dentro de
 *   `EnTransaccion`, y esa revierte: la transaccion no confirma y no queda
 *   version abierta.
 * - 401: **no tiene rama propia porque no llega hasta aqui**, y conviene dejarlo
 *   escrito para que nadie lo "arregle" anadiendo un texto que no se puede ver.
 *   `api()` (`web/src/api.ts:91-94`) trata un 401 de una llamada con token como
 *   sesion vencida: borra el token y avisa a `ProveedorDeSesion`, que limpia el
 *   usuario, y `RutaProtegida` manda al login. El editor se desmonta, asi que el
 *   desenlace del 401 tiene su propio aviso -la pantalla de entrada- y ningun
 *   panel llega a pintarse. Si esa interceptacion desapareciera, el ultimo
 *   `return` de esta funcion contesta al 401 con una verdad que le sirve.
 * - 403: lo produce `requiereRol` sobre el grupo `/obras` antes de llegar al
 *   handler, asi que no hay nada escrito que revertir. Este SI se pinta: no hay
 *   ningun manejador global que intercepte un 403.
 *
 * El 5xx no entra aqui y no puede entrar -por eso `puedeHaberGuardado` los deja
 * fuera-: ahi el COMMIT pudo haber entrado y afirmar el no-registro seria el
 * defecto de D-009 otra vez.
 */
export function avisoDelRechazo(status: number): string {
  if (status === 400) {
    return "No se guardó nada: corrige el reparto y vuelve a guardarlo.";
  }
  if (status === 403) {
    // Este es de autorizacion: corregir las filas no lo mueve.
    return "No se guardó nada: este rechazo es de autorización y no lo produce el reparto, así que volver a guardarlo con esta sesión dará lo mismo. Hace falta una sesión con el rol que esta ruta pide.";
  }
  if (status === 404) {
    // Sin obra no hay a que abrirle una version, asi que reintentar tampoco.
    return "No se guardó nada: sobre una obra que no está en el catálogo no hay ninguna versión que abrir, así que volver a guardarlo no escribe ninguna.";
  }
  // Cualquier otro 4xx no lo produce el handler: sus unicas salidas con 4xx son
  // el 400, el 401, el 403 y el 404 de arriba -y el 401 ni se pinta-, asi que
  // esto es el 405 del enrutado o el 401 del caso nombrado arriba, y en los dos
  // la peticion murio antes del COMMIT.
  return "No se guardó nada: el servidor rechazó el guardado antes de confirmar ninguna versión.";
}
