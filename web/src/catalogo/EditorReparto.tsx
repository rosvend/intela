import {
  useEffect,
  useId,
  useRef,
  useState,
  type FormEvent,
  type KeyboardEvent,
  type MutableRefObject,
  type ReactElement,
} from "react";
import { Link, useParams } from "react-router-dom";
import { ApiError, ErrorDeCuerpoIlegible, ErrorDeRed, api } from "../api";
import Cargando from "../Cargando";
import { formatearInstante } from "../tablero/formato";
import { useApi } from "../useApi";
import { CLAVE_DE_VUELTA_AL_CATALOGO, useVueltaAlCatalogo } from "./Catalogo";
import {
  estadoDelBorrador,
  formatearPorcentaje,
  puedeGuardarBorrador,
  totalDeclarado,
  type EstadoBorrador,
} from "./declaracion";
import { EtiquetaDeEstado } from "./EtiquetaDeDeclaracion";
import {
  esObra,
  esTitular,
  esVersionDeclaracion,
  puedeSerParte,
  type Obra,
  type Parte,
  type Titular,
  type VersionDeclaracion,
} from "./tipos";

/**
 * Cuantos titulares se piden por pagina del padron. El servidor aplica 100 si
 * se omite y no admite mas de 500; se manda explicito para que la pagina que se
 * ve sea la que dice la pantalla, igual que en el catalogo de obras.
 */
const LIMITE_PADRON = 50;

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
type FilaDeReparto = {
  /** Clave de React. No puede ser el `titular_id`: la fila sobrevive a cambios. */
  clave: string;
  titularId: string;
  ipi: string;
  /** El porcentaje tal como se teclea, sin interpretar. */
  porcentaje: string;
};

// Contador de claves. Un `crypto.randomUUID` ataria los tests al entorno, y el
// indice de la fila no sirve: al quitar una fila del medio las claves se
// correrian y React reutilizaria el estado de la fila equivocada.
let contadorDeFilas = 0;

function nuevaFila(titularId = "", ipi = "", porcentaje = ""): FilaDeReparto {
  contadorDeFilas += 1;
  return { clave: `fila-${contadorDeFilas}`, titularId, ipi, porcentaje };
}

/**
 * La fila que tiene que recibir el foco en cuanto el render la pinte.
 *
 * Es la clave de la fila destino, y `FilasDelReparto` la consume UNA sola vez: la
 * deja en `null` al usarla. Va como ref y no como estado a proposito -el foco no
 * se pinta-, porque como estado costaria un render de mas, el foco llegaria un
 * render tarde y volveria a saltar en cada tecla, que tambien cambia `filas`. Y
 * se usa esto en vez de `autoFocus` porque `autoFocus` dispara al MONTAR: le
 * quitaria el foco a la pantalla en cuanto se abre, cuando lo que hay que
 * resolver es no perderlo despues de quitar o anadir una fila.
 */
type FocoPendiente = MutableRefObject<string | null>;

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
function porcentajeDeTexto(texto: string): number {
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
function textoDePorcentaje(valor: number): string {
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
function filasDePartida(
  versiones: readonly VersionDeclaracion[],
): FilaDeReparto[] {
  const abierta = versionAbierta(versiones);
  if (!abierta) return [];
  return abierta.partes.map((parte) =>
    nuevaFila(parte.titular_id, parte.ipi, textoDePorcentaje(parte.porcentaje)),
  );
}

/**
 * La version que rige hoy: la unica que el historial deja sin cerrar.
 *
 * Devuelve `null` si no hay exactamente una. No es una guarda de mas: el numero
 * que se lee de aqui es el que el aviso de la version enseña como "se cerrara
 * la version N", y con dos abiertas -o con ninguna- el backend cerraria otra
 * cosa. Antes que afirmar un numero que no se sostiene, no se afirma ninguno.
 */
function versionAbierta(
  versiones: readonly VersionDeclaracion[],
): VersionDeclaracion | null {
  const abiertas = versiones.filter(
    (version) => version.vigente_hasta === null,
  );
  return abiertas.length === 1 ? abiertas[0] : null;
}

/**
 * El motivo por el que un guardado dejo PENDIENTE la pregunta de que version
 * quedo abierta.
 *
 * No es el desenlace de un guardado: es un hecho que tiene que SOBREVIVIR al
 * render en que ocurrio, porque el unico desenlace que lo resuelve -un 200
 * legible- puede no llegar nunca, y mientras tanto el numero viejo ya no se
 * sostiene. Por eso vive en el estado del componente y no se deriva del
 * `resultado` de turno.
 *
 * Los dos se distinguen por lo que SI se sabe: `guardadoSinLeer` es un 2xx cuyo
 * cuerpo no se pudo leer, o sea que una version se abrio y lo que falta es su
 * numero; `guardadoIncierto` es un 5xx o una respuesta que no llego, donde ni
 * siquiera se sabe si una version quedo abierta.
 */
type MotivoDeDuda = "guardadoSinLeer" | "guardadoIncierto";

/**
 * Lo que se sabe de la version que el guardado va a cerrar y de la que va a
 * abrir. Es el aviso que D-009 existe para arreglar: el mockup ponia "se
 * cerrara la version 2 y se abrira una version 3" con los dos numeros escritos
 * a mano, y el numero de version es un dato derivado del historial, asi que esa
 * frase es falsa para cualquier obra que no este en la version 2 y no se
 * sostiene para una obra sin declaracion previa.
 *
 * Los tres casos, y ninguno afirma un numero que no tenga:
 *
 * - `cierraYabre`: hay una version abierta y se conoce, o el servidor acaba de
 *   contestar cual abrio. Los dos numeros salen de un dato, no de una cuenta
 *   del cliente que pueda discrepar;
 * - `abreLaPrimera`: el historial vino vacio -la obra no tiene ninguna
 *   declaracion- asi que el numero que se abriria seria el 1, y se dice con
 *   palabras en vez de con una cifra deducida;
 * - `sinNumeros`: no se sabe que version esta abierta. Cuatro motivos distintos
 *   caen aqui y cada uno tiene su prosa -el `porque`-, porque nombrar la causa
 *   equivocada es la misma clase de defecto que este tipo existe para cerrar:
 *   el historial no se pudo leer, lo que trajo no deja ver una sola version
 *   abierta, el `PUT` contesto 200 con un cuerpo ilegible, o el guardado quedo
 *   en duda (un 5xx, una respuesta perdida). Se dice SOLO la consecuencia -la
 *   version anterior, si la hay, queda en el historial y no se modifica- y
 *   ningun numero.
 *
 * `sinBorrador` no es un quinto motivo: es el hecho independiente de que el
 * borrador no se pudo sembrar con el reparto vigente, y se sigue diciendo pase
 * lo que pase con el guardado.
 *
 * `hayRechazoPosterior` tampoco es un motivo, sino un hecho de la PANTALLA que
 * solo importa en dos de los cuatro `porque`: abajo hay un rechazo que NO es el
 * guardado del que habla este aviso -un 4xx no limpia la duda, ver
 * `avisoDeVersion`-, asi que los dos textos hablan de guardados distintos y hay
 * que decirlo. Va en `false` cuando no lo hay, que es lo que hace que la prosa
 * del aviso no cambie en los casos que ya estaban probados.
 */
type AvisoDeVersion =
  | { tipo: "cierraYabre"; seCierra: number; seAbre: number }
  | { tipo: "abreLaPrimera" }
  | {
      tipo: "sinNumeros";
      porque: "historialNoLeido" | "sinVersionAbierta" | MotivoDeDuda;
      sinBorrador: boolean;
      hayRechazoPosterior: boolean;
    };

/**
 * El aviso, a partir de las TRES fuentes que pueden saberlo: el historial leido
 * al abrir la pantalla, la version que el servidor contesto que abrio en el
 * ultimo guardado, y la DUDA que un guardado dejo pendiente.
 *
 * `versionGuardada` manda sobre el historial, y no es un atajo: despues de
 * guardar, el historial que hay en memoria es el de ANTES, y usarlo diria que
 * se cierra la version que el servidor acaba de cerrar. La respuesta del
 * `PUT` es el dato mas fresco que existe -el servidor acaba de escribir esa
 * version-, y el consecutivo es del servidor: `Store.Guardar` abre
 * `versionAbierta + 1` (`internal/infraestructura/postgres/declaraciones.go`).
 *
 * **La duda pendiente manda sobre las dos**, y ese es el arreglo que trajo el
 * completion fix `iter-1/step-8.3` (hallazgo CRITICAL de la PASADA 2 de la
 * revision adversarial, sobre el arreglo que a su vez trajo el `iter-1/step-8.1`).
 * El 8.1 ya mandaba a `sinNumeros` los dos desenlaces que dejan la duda, pero lo
 * hacia mirando el `resultado` DEL RENDER, y ese `resultado` es transitorio:
 * `guardar()` lo pone a `null` al empezar. La consecuencia, medida: el guardado
 * SIGUIENTE devolvia el numero viejo al aviso -con un 200 de cuerpo ilegible, que
 * cierra la v3 y abre la v4, seguido de un 400, el aviso decia "cerrará la
 * versión 3 y abrirá la versión 4" mientras el panel de la MISMA pantalla decia
 * "No se guardó nada"-, y el mismo texto reaparecia mientras el segundo `PUT`
 * estaba en vuelo. Por eso la duda vive en el estado del componente
 * (`MotivoDeDuda`) y llega aqui como una fuente mas.
 *
 * Un 4xx (`rechazada`) NO deja duda -no abre ninguna version- pero tampoco la
 * **limpia**, y ahi estaba el error del comentario del 8.1: un 4xx prueba que ESA
 * peticion no abrio ninguna version, y no dice nada del guardado anterior que
 * quedo en duda. El historial en memoria vuelve a ser la autoridad solo cuando no
 * hay ninguna duda pendiente. El 200 legible tampoco entra por aqui: es el unico
 * camino que limpia la duda, porque es el unico que dice cual se abrio.
 *
 * `hayRechazoPosterior` no cambia el motivo, cambia la PROSA: cuando en la misma
 * pantalla hay ademas un rechazo, los dos textos hablan de guardados distintos
 * -el aviso, del que dejo la duda; el panel, de otro posterior-, y el aviso tiene
 * que decirlo o se lee como si describiera el que acaba de rechazarse.
 */
function avisoDeVersion(
  historial: readonly VersionDeclaracion[] | null,
  versionGuardada: number | null,
  dudaPendiente: MotivoDeDuda | null,
  hayRechazoPosterior: boolean,
): AvisoDeVersion {
  // La duda pendiente manda sobre todo lo demas, incluida la version que el
  // propio `PUT` devolvio: entre un guardado legible y otro posterior que quedo
  // en duda, la que ya no se sostiene es la del primero.
  if (dudaPendiente !== null) {
    return {
      tipo: "sinNumeros",
      porque: dudaPendiente,
      // El borrador se sembro -o no- al leer el historial, y un guardado
      // posterior no cambia eso: el hecho se dice en los dos casos.
      sinBorrador: historial === null,
      hayRechazoPosterior,
    };
  }
  if (versionGuardada !== null) {
    return {
      tipo: "cierraYabre",
      seCierra: versionGuardada,
      seAbre: versionGuardada + 1,
    };
  }
  if (historial === null) {
    return {
      tipo: "sinNumeros",
      porque: "historialNoLeido",
      sinBorrador: true,
      hayRechazoPosterior: false,
    };
  }
  if (historial.length === 0) return { tipo: "abreLaPrimera" };
  const abierta = versionAbierta(historial);
  if (!abierta) {
    return {
      tipo: "sinNumeros",
      porque: "sinVersionAbierta",
      // `true`, y no `false`: esta rama es EXACTAMENTE la condicion con la que
      // `filasDePartida` devuelve `[]` -las dos preguntan por `versionAbierta`-,
      // asi que aqui no hay version vigente que sembrar y el borrador arranca
      // vacio. Decir `false` dejaba la pantalla afirmando que si hay borrador
      // mientras la rejilla salia sin una sola fila.
      sinBorrador: true,
      hayRechazoPosterior: false,
    };
  }
  return {
    tipo: "cierraYabre",
    seCierra: abierta.version,
    seAbre: abierta.version + 1,
  };
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
type FalloDelGuardado = {
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
function puedeHaberGuardado(status: FalloDelGuardado["status"]): boolean {
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
function falloDelGuardado(error: unknown): ResultadoDelGuardado {
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
type ResultadoDelGuardado =
  | { tipo: "guardada"; version: VersionDeclaracion }
  | { tipo: "guardadaSinLeer" }
  | { tipo: "rechazada"; status: number; mensaje: string }
  | {
      tipo: "incierta";
      status: FalloDelGuardado["status"];
      mensaje: string;
    };

/**
 * El texto y la explicacion del borrador en cada uno de sus cuatro estados.
 *
 * Son estados del BORRADOR, que no existen para el servidor: los de una
 * declaracion guardada son dos -`completa` e `incompleta`- y los asigna el
 * backend. Por eso no se pinta aqui `EtiquetaDeEstado`, que es la etiqueta del
 * estado que el backend calculo, y por eso ninguna de estas frases dice lo que
 * la version guardada vaya a quedar siendo.
 */
const ESTADO_DEL_BORRADOR: Record<
  EstadoBorrador,
  { texto: string; explicacion: string }
> = {
  vacia: {
    texto: "Sin nada declarado",
    explicacion:
      "No hay ningún porcentaje escrito todavía. El servidor rechaza una declaración sin partes, así que esta pantalla no ofrece guardar mientras el borrador esté así.",
  },
  completa: {
    texto: "Completa",
    explicacion:
      "El borrador suma 100. La comparación da por bueno el ruido de la coma flotante, para que una suma exacta en aritmética decimal no se lea como incompleta. Si el servidor lo acepta, la versión quedará completa.",
  },
  incompleta: {
    texto: "Incompleta",
    explicacion:
      "Un borrador por debajo de 100 no es un error: bajo R-04 (RD 13.1.3) la declaración se guarda igual, no se reparte nada de esa obra y el importe completo queda en reserva, nunca se prorratea. Se puede guardar así.",
  },
  excedida: {
    texto: "Pasa de 100",
    explicacion:
      "El total pasa de 100, y el servidor rechaza con un 400 toda declaración cuya suma supere 100. Por eso esta pantalla no ofrece guardar mientras el total esté por encima: baja el porcentaje de alguna fila.",
  },
};

/**
 * El editor del reparto (issue #30, paso 8; S2, S3 y S4).
 *
 * La pantalla que ABRE una version de la Declaracion de Obra:
 * `PUT /obras/{id}/declaracion` con un array de partes. No edita una version
 * pasada -el contrato no tiene forma de hacerlo-, y por eso todo el rato dice
 * que lo que va a pasar es que se cierre la vigente y se abra una nueva.
 *
 * Aparte del numero de version -que es el defecto que D-009 cierra-, la regla
 * que ordena esta pantalla es de donde sale cada cifra:
 *
 * - el ESTADO y la SUMA de la version guardada los calcula el servidor, y no se
 *   ven aqui: se ven en el catalogo, en el detalle y en el historial. Lo que hay
 *   aqui es el borrador, que no tiene estado de backend porque no existe para el
 *   servidor, y va rotulado como lo que es -calculado en esta pantalla-;
 * - los TITULARES elegibles salen de `GET /titulares`, que devuelve el padron
 *   entero a proposito: quien no puede ser parte se explica en vez de faltar;
 * - el MENSAJE de un rechazo es el del backend, entero y sin reescribir: nombra
 *   la regla que se incumplio, y solo el backend sabe cual de ellas ha sido.
 *
 * El bloqueo del guardado SI es del cliente, y es el unico que hay:
 * `puedeGuardarBorrador` frena dos cuerpos -uno que no declara nada, y uno cuya
 * suma pasa de 100-. Que el cliente frene no es que la regla sea suya: la regla
 * es del servidor, que la aplica aunque el cliente no la mire, y si el total se
 * cuela -una suma que se pasa por menos que la tolerancia, un porcentaje con mas
 * de cuatro decimales- el 400 del backend es lo que se enseña.
 *
 * Este bloqueo NO es la lista de todo lo que el backend rechaza con 400, y el
 * comentario que lo decia asi era falso: `repertorio.NuevaDeclaracion` rechaza
 * ademas un IPI ausente, un porcentaje no positivo, mas de cuatro decimales y un
 * titular repetido. La lista de los dos cuerpos que el cliente SI bloquea vive
 * en un solo sitio, `declaracion.ts` (`puedeGuardarBorrador`,
 * `estadoDelBorrador`); repetirla aqui daria dos fuentes para el mismo hecho, y
 * la que se quede sin actualizar es la que miente.
 */
export default function EditorReparto() {
  const { id = "" } = useParams();
  // La vuelta al catalogo, resuelta por la misma funcion que usan el detalle y
  // el historial: las tres salidas de la pantalla -la ficha, la obra ausente y
  // el aviso de que la obra se fue al guardar- salen de un solo sitio.
  const { busqueda, destino } = useVueltaAlCatalogo();
  const {
    datos: obra,
    cargando,
    error,
  } = useApi<Obra>(`/api/obras/${encodeURIComponent(id)}`);

  if (cargando) return <Cargando texto="Cargando la obra…" />;

  if (error) {
    // El 404 no es un fallo del sistema: es el servidor diciendo que con ese
    // identificador no hay ninguna obra, y por tanto que no hay reparto que
    // declarar.
    if (error instanceof ApiError && error.status === 404) {
      return <ObraAusente id={id} volver={destino} />;
    }
    return (
      <p className="catalogo-error" role="alert">
        No se pudo consultar la obra: {error.message}
      </p>
    );
  }

  // `useApi<Obra>` promete una obra, pero `T` es una promesa y no una
  // comprobacion: un 2xx con otra forma llegaria hasta aqui y `obra.titulo` o
  // `obra.id` -lo que esta pantalla lee- la tumbarian. La guarda es la del tipo,
  // la misma que usan el listado, el detalle y el historial.
  if (!esObra(obra)) {
    return (
      <p className="catalogo-error" role="alert">
        La obra no llegó con la forma que el contrato promete para una obra.
      </p>
    );
  }

  return <EditorDeLaObra obra={obra} busqueda={busqueda} destino={destino} />;
}

/** Lo que se ve cuando el servidor no tiene ninguna obra con ese identificador. */
function ObraAusente({ id, volver }: { id: string; volver: string }) {
  return (
    <section className="editor-reparto">
      <h1>Esa obra no está en el catálogo</h1>
      <p className="muted">
        El servidor no tiene ninguna obra con el identificador <code>{id}</code>
        , así que no hay ninguna declaración que abrir. El reparto de una obra
        se declara sobre la obra: sin ella no hay nada que editar.
      </p>
      <p className="detalle-volver">
        <Link to={volver}>Volver al catálogo</Link>
      </p>
    </section>
  );
}

/**
 * La obra ya legible: primero su declaracion vigente, y con ella el borrador.
 *
 * El historial se lee UNA vez y de aqui sale todo lo que depende de el: con que
 * reparto empieza el borrador y que version se cerrara. Se lee antes de montar
 * el formulario a proposito: asi el borrador se construye con el reparto
 * vigente ya en la mano en vez de sembrarse despues -que sobrescribiria lo que
 * se hubiera tecleado mientras llegaba la respuesta-.
 *
 * Que el historial falle NO impide editar: el borrador empieza sin filas, el
 * aviso dice que no se sabe que version se cerrara, y el padron sigue estando
 * para armar un reparto. Lo que no se hace es inventar el reparto que no se pudo
 * leer.
 */
function EditorDeLaObra({
  obra,
  busqueda,
  destino,
}: {
  obra: Obra;
  busqueda: string;
  destino: string;
}) {
  const {
    datos: historial,
    cargando,
    error,
  } = useApi<VersionDeclaracion[]>(
    `/api/obras/${encodeURIComponent(obra.id)}/declaracion/historial`,
  );

  if (cargando) return <Cargando texto="Cargando la declaración vigente…" />;

  // El historial de una obra que existe llega como lista vacia -nunca 404-, asi
  // que un error aqui es un fallo de lectura y no "no hay declaracion": lo que
  // se lee con exito es lo que decide entre los dos casos. Una lista mal formada
  // cuenta como no leida, por la misma razon por la que el historial del detalle
  // no pinta versiones a medias.
  const legible =
    !error && Array.isArray(historial) && historial.every(esVersionDeclaracion);

  return (
    <FormularioDeReparto
      obra={obra}
      // `null` quiere decir "no se pudo leer", que es lo unico que el aviso
      // necesita distinguir de "no hay ninguna declaracion".
      historial={legible ? historial : null}
      mensajeDelHistorial={
        error
          ? error.message
          : "la respuesta no llegó como una lista de versiones"
      }
      busqueda={busqueda}
      destino={destino}
    />
  );
}

/**
 * Enter en un campo de texto NO guarda.
 *
 * Un `<form>` con un boton de envio convierte Enter dentro de un `<input>` en un
 * envio implicito, y este formulario guarda: pulsar Enter a media edicion cierra
 * la version abierta y abre otra con lo que hubiera escrito en ese instante. Con
 * el porcentaje a medio teclear -el "7" de un "70"- la obra pasa a sumar 7 y bajo
 * R-04 deja de repartir nada. Medido en navegador real, con precondicion y control
 * negativo: teclear un digito no manda nada, y Enter mandaba un `PUT`.
 *
 * Guardar sigue siendo el boton, y por eso el Enter se CANCELA en vez de
 * confirmarse: convertirlo en una confirmacion anadiria un paso -y un dialogo- al
 * camino legitimo, que hoy es un clic. La condicion es `INPUT` de tipo `text` y no
 * "cualquier Enter": pulsar Enter con el foco en el boton tiene que seguir
 * guardando, que es el comportamiento que espera quien tabula hasta el y pulsa.
 */
function alPulsarTecla(evento: KeyboardEvent<HTMLFormElement>) {
  const destino = evento.target as HTMLElement;
  if (
    evento.key === "Enter" &&
    destino.tagName === "INPUT" &&
    (destino as HTMLInputElement).type === "text"
  ) {
    evento.preventDefault();
  }
}

/**
 * El borrador, el padron y el guardado.
 *
 * Se monta cuando el historial ya se resolvio -bien o mal-, y no se vuelve a
 * montar: el borrador es del que esta editando y perderlo por una respuesta que
 * llega tarde seria justo lo que el mockup pide evitar. Por eso la respuesta del
 * guardado no se lee del historial sino del `PUT`, que es lo que deja el aviso
 * de la version al dia sin volver a pedir nada.
 */
function FormularioDeReparto({
  obra,
  historial,
  mensajeDelHistorial,
  busqueda,
  destino,
}: {
  obra: Obra;
  historial: VersionDeclaracion[] | null;
  mensajeDelHistorial: string;
  busqueda: string;
  destino: string;
}) {
  const [filas, setFilas] = useState<FilaDeReparto[]>(() =>
    historial === null ? [] : filasDePartida(historial),
  );
  const [resultado, setResultado] = useState<ResultadoDelGuardado | null>(null);
  // La version que el servidor contesto que abrio. Es lo que mantiene el aviso
  // al dia despues de guardar, sin releer el historial ni adivinar el numero.
  const [versionGuardada, setVersionGuardada] = useState<number | null>(null);
  // La duda que un guardado dejo pendiente, en su PROPIO estado y no derivada
  // del `resultado` del render: `resultado` es transitorio -`guardar()` lo pone a
  // `null` al empezar- y deducirla de ahi hacia que el guardado siguiente
  // devolviera el numero viejo al aviso, incluido el hueco en vuelo. Un 4xx no la
  // toca; la limpia un 200 legible. Ver `MotivoDeDuda`.
  const [dudaPendiente, setDudaPendiente] = useState<MotivoDeDuda | null>(null);
  const [guardando, setGuardando] = useState(false);
  // `disabled` llega en el siguiente render; el ref corta tambien un segundo
  // clic que entre antes. Un segundo `PUT` con el mismo reparto abriria una
  // version de mas.
  const enVuelo = useRef(false);
  // La fila que tiene que recibir el foco en cuanto el borrador cambie. La
  // escriben las dos acciones que cambian la rejilla y la consume
  // `FilasDelReparto`. Ver `FocoPendiente`.
  const focoPendiente = useRef<string | null>(null);

  const total = totalDeclarado(
    filas.map((fila) => ({ porcentaje: porcentajeDeTexto(fila.porcentaje) })),
  );
  const estado = estadoDelBorrador(total);
  // Filas sin un numero legible en el porcentaje. No es una regla de negocio
  // -la del total la aplica `puedeGuardarBorrador`-, es la forma del cuerpo:
  // cada parte lleva un numero, y esta pantalla no manda `null` donde el
  // contrato pide una cifra ni convierte en 0 lo que nadie escribio.
  const filasSinNumero = filas.filter(
    (fila) => !Number.isFinite(porcentajeDeTexto(fila.porcentaje)),
  ).length;
  // La obra dice cual es su version vigente y el historial cual tiene abierta:
  // son dos lecturas del mismo hecho, y si no coinciden -una declaracion se
  // guardo entre las dos peticiones, o el historial no trae ninguna abierta-
  // guardar cerraria una version que `GET /obras/{id}` no esta sosteniendo. Es
  // la MISMA comprobacion que hace el detalle (`DetalleObra.tsx:381`), con el
  // mismo razonamiento, y aqui importa mas porque esta pantalla ESCRIBE: la de
  // solo lectura se niega a pintar, esta se niega a guardar.
  //
  // Se compara solo cuando el historial se pudo leer. `null` es "no se pudo
  // leer", y ese caso ya tiene su aviso -y guardar sin haber leido el historial
  // es justo lo que el aviso advierte-: prohibirlo aqui seria cambiar una
  // advertencia por una puerta cerrada sin haberselo pedido a nadie.
  const versionAbiertaDelHistorial =
    historial === null ? null : (versionAbierta(historial)?.version ?? null);
  const elHistorialCuadra =
    historial === null || versionAbiertaDelHistorial === obra.version_vigente;

  const puedeGuardar =
    puedeGuardarBorrador(estado) && filasSinNumero === 0 && elHistorialCuadra;
  // Las tres fuentes: el historial, la version que el `PUT` devolvio y la duda
  // que un guardado dejo pendiente. La duda manda, y por eso no basta con las dos
  // primeras -ver `avisoDeVersion`-.
  const aviso = avisoDeVersion(
    historial,
    versionGuardada,
    dudaPendiente,
    // Un 4xx no limpia la duda, asi que cuando el panel de abajo cuenta un
    // rechazo y hay una duda pendiente, los dos textos son de guardados
    // distintos: el rechazo es posterior al guardado que dejo la duda.
    resultado?.tipo === "rechazada",
  );

  function agregarTitular(titular: Titular) {
    // La fila nace SIN porcentaje: la cifra la escribe quien declara, y un
    // valor de relleno acabaria guardado como si se hubiera declarado.
    const fila = nuevaFila(titular.id, titular.ipi, "");
    // El foco va a la fila nueva. El boton que se pulso esta en el padron -al
    // final de la pantalla- y ademas desaparece en cuanto el titular entra en el
    // reparto, asi que sin esto el foco se queda en la nada: el `Tab` siguiente
    // vuelve al principio de la pagina, que es justo de donde se viene.
    focoPendiente.current = fila.clave;
    setFilas((previas) => [...previas, fila]);
  }

  function quitarFila(clave: string) {
    const indice = filas.findIndex((fila) => fila.clave === clave);
    const restantes = filas.filter((fila) => fila.clave !== clave);
    // Quien ocupa el hueco: la fila que se corre a ese indice y, si la quitada
    // era la ultima, la que pasa a ser la ultima. Se calcula sobre el array que
    // se va a guardar -y no sobre `filas`- para que la clave que se pide sea
    // siempre la de una fila que existe; con la rejilla vacia no queda ningun
    // control al que llevar el foco y no se promete ninguno.
    const relevo = restantes[indice] ?? restantes[restantes.length - 1];
    focoPendiente.current = relevo ? relevo.clave : null;
    setFilas(restantes);
  }

  function cambiarFila(clave: string, cambio: Partial<FilaDeReparto>) {
    setFilas((previas) =>
      previas.map((fila) =>
        fila.clave === clave ? { ...fila, ...cambio } : fila,
      ),
    );
  }

  async function guardar(evento: FormEvent) {
    evento.preventDefault();
    if (enVuelo.current || !puedeGuardar) return;
    enVuelo.current = true;
    setGuardando(true);
    setResultado(null);

    // El cuerpo es el array de partes que declara el contrato: el mismo titulo,
    // el mismo IPI y el porcentaje leido del campo, SIN redondear. Redondear
    // aqui declararia una cifra distinta de la escrita, y mas de cuatro
    // decimales lo rechaza el servidor con su mensaje.
    const partes: Parte[] = filas.map((fila) => ({
      titular_id: fila.titularId,
      ipi: fila.ipi,
      porcentaje: porcentajeDeTexto(fila.porcentaje),
    }));

    try {
      const cuerpo = await api(
        `/api/obras/${encodeURIComponent(obra.id)}/declaracion`,
        { method: "PUT", body: JSON.stringify(partes) },
      );
      // Frontera sin validar, como las de la ingesta: el 200 trae una
      // `VersionDeclaracion` segun api/openapi.yaml, y `T` es una promesa, no
      // una comprobacion. Se revisa antes de afirmar su numero.
      if (!esVersionDeclaracion(cuerpo)) {
        setResultado({ tipo: "guardadaSinLeer" });
        // El 200 prueba que una version se abrio; lo que no se puede leer es
        // cual. La duda queda PENDIENTE hasta que otro guardado conteste 200 con
        // un cuerpo legible.
        setDudaPendiente("guardadoSinLeer");
        return;
      }
      setResultado({ tipo: "guardada", version: cuerpo });
      setVersionGuardada(cuerpo.version);
      // El unico desenlace que limpia la duda: es el unico que dice cual se
      // abrio. La version vieja de `versionGuardada` no se borra -sigue siendo
      // el ultimo dato bueno-, pero deja de mandar mientras haya duda.
      setDudaPendiente(null);
    } catch (error) {
      const fallo = falloDelGuardado(error);
      setResultado(fallo);
      // Un 4xx (`rechazada`) NO entra en ninguna rama y por eso no limpia nada:
      // prueba que ESA peticion no abrio ninguna version, y no dice nada del
      // guardado anterior que quedo en duda.
      if (fallo.tipo === "guardadaSinLeer") setDudaPendiente("guardadoSinLeer");
      if (fallo.tipo === "incierta") setDudaPendiente("guardadoIncierto");
    } finally {
      enVuelo.current = false;
      setGuardando(false);
    }
  }

  return (
    <section className="editor-reparto">
      <p className="detalle-volver">
        <Link
          to={`/catalogo/${encodeURIComponent(obra.id)}`}
          state={{ [CLAVE_DE_VUELTA_AL_CATALOGO]: busqueda }}
        >
          ← Volver a la obra
        </Link>
      </p>

      <header className="detalle-cabecera">
        <h1>Declaración de la obra</h1>
        <p className="muted detalle-nota">
          {obra.titulo} · {obra.id}
        </p>
        {/* De que va esta pantalla, sin prometer nada que no se cumpla: los
            porcentajes de reparto solo salen de la Declaracion de Obra (R-03), y
            guardar aqui no corrige la version vigente, abre una nueva. */}
        <p className="muted detalle-nota">
          Los porcentajes de reparto solo salen de la Declaración de Obra
          (R-03). Guardar aquí no corrige la versión vigente: la cierra y abre
          una nueva con el reparto que quede escrito.
        </p>
      </header>

      <AvisoDeVersionVisible aviso={aviso} mensaje={mensajeDelHistorial} />

      {resultado && (
        <PanelDelGuardado
          resultado={resultado}
          obraId={obra.id}
          busqueda={busqueda}
          volver={destino}
        />
      )}

      <form
        className="editor-formulario"
        onSubmit={(e) => void guardar(e)}
        onKeyDown={alPulsarTecla}
      >
        <section className="editor-borrador">
          <h2>Borrador del reparto</h2>
          <p className="muted detalle-nota">
            El estado de la declaración guardada lo calcula el servidor; lo de
            aquí es el borrador, que todavía no existe para el servidor.
          </p>
          {/* `role="status"` en las dos cifras y en los avisos, siguiendo el
              patron de `PanelDelGuardado`: son nodos VIVOS, y lo que cambia en
              ellos -el total al teclear, el estado del borrador, el motivo por
              el que no se ofrece guardar, el aviso de la version- es justo lo
              que hay que anunciar sin mover el foco. No se anade `aria-live`
              por encima ni se mete un `role="alert"` dentro: las dos cosas
              anunciarian el mismo cambio dos veces. */}
          <dl className="detalle-ficha">
            <div>
              <dt>Total del borrador (calculado en esta pantalla)</dt>
              <dd className="detalle-cifra" role="status">
                {formatearPorcentaje(total)}
              </dd>
            </div>
            <div>
              <dt>Estado del borrador</dt>
              <dd className="detalle-cifra" role="status">
                {ESTADO_DEL_BORRADOR[estado].texto}
              </dd>
            </div>
          </dl>
          <p className="muted detalle-nota">
            {ESTADO_DEL_BORRADOR[estado].explicacion}
          </p>
          <p className="muted detalle-nota">
            El total se muestra con 4 decimales, la precisión de la columna, y
            el estado del borrador se decide sobre la suma exacta de lo escrito,
            sin redondearla. El total y el estado de la versión guardada los
            calcula el servidor: son los que se ven en el catálogo y en el
            historial.
          </p>
        </section>

        <FilasDelReparto
          filas={filas}
          focoPendiente={focoPendiente}
          onCambiar={cambiarFila}
          onQuitar={quitarFila}
        />

        <div className="editor-acciones">
          {/* La unica via de guardado, y la que espera quien opera. El Enter de
              un campo de texto lo cancela `alPulsarTecla`; el de este boton no,
              porque ahi el envio implicito es lo que se quiere. */}
          <button
            type="submit"
            className="boton-primario"
            disabled={!puedeGuardar || guardando}
          >
            {guardando ? "Guardando…" : "Guardar la declaración"}
          </button>
          {filasSinNumero > 0 && (
            <p className="editor-aviso" role="status">
              {filasSinNumero === 1
                ? "Hay 1 fila sin un porcentaje que se lea como número."
                : `Hay ${filasSinNumero} filas sin un porcentaje que se lea como número.`}{" "}
              El cuerpo que espera el servidor lleva un número en cada parte,
              así que la pantalla no envía nada mientras falte.
            </p>
          )}
          {filasSinNumero === 0 && elHistorialCuadra && !puedeGuardar && (
            <p className="editor-aviso" role="status">
              {estado === "vacia"
                ? "Añade al menos una parte desde el padrón para poder guardar."
                : "El total pasa de 100, y el servidor rechaza esa suma con un 400."}
            </p>
          )}
          {/* El tercer motivo por el que no se ofrece guardar, y el unico que
              NO es del borrador: el historial no cuadra con la version vigente
              que declara la obra. Va aparte de los dos de arriba para que el
              texto diga siempre el motivo verdadero -sin esta guarda, un
              historial que no cuadra saldria como "el total pasa de 100"-. */}
          {!elHistorialCuadra && (
            <p className="editor-aviso" role="status">
              {obra.version_vigente === null
                ? "El servidor no declara ninguna versión vigente,"
                : `El servidor declara vigente la versión ${obra.version_vigente},`}{" "}
              pero el historial no trae esa versión como única versión abierta.
              Mientras las dos lecturas no cuadren, esta pantalla no ofrece
              guardar: guardaría cerrando una versión que el sistema no está
              sosteniendo. Recarga la pantalla y vuelve a mirar el historial.
            </p>
          )}
        </div>
      </form>

      <PadronDelEditor
        enElReparto={filas.map((fila) => fila.titularId)}
        onAgregar={agregarTitular}
      />
    </section>
  );
}

/**
 * El aviso de la version, en sus tres formas. Ver `AvisoDeVersion`.
 *
 * Va arriba, antes del formulario, porque es lo que hay que entender ANTES de
 * tocar nada: quien crea que esta corrigiendo el reparto vigente va a descubrir
 * lo contrario despues de guardar.
 */
function AvisoDeVersionVisible({
  aviso,
  mensaje,
}: {
  aviso: AvisoDeVersion;
  mensaje: string;
}) {
  if (aviso.tipo === "cierraYabre") {
    return (
      <p className="editor-aviso-version" role="status">
        Al guardar, el servidor cerrará la versión {aviso.seCierra} y abrirá la
        versión {aviso.seAbre} con este reparto. La versión {aviso.seCierra} no
        se borra ni se modifica: queda en el historial con los porcentajes que
        regían hasta ahora.
      </p>
    );
  }

  if (aviso.tipo === "abreLaPrimera") {
    return (
      <p className="editor-aviso-version" role="status">
        Esta obra no tiene ninguna declaración todavía, así que al guardar se
        abrirá la primera versión. No hay ninguna versión anterior que cerrar.
      </p>
    );
  }

  // Sin numeros. Lo unico que se afirma es la consecuencia, que se sostiene en
  // todos los casos: no hay forma de editar una version pasada, solo de abrir
  // una nueva, asi que la anterior -si la hay- no se borra ni se modifica.
  const consecuencia =
    "Lo que sí se sabe: la versión anterior, si la hay, no se borra ni se modifica, y queda en el historial con los porcentajes que regían hasta ahora.";
  // Si el borrador no se pudo sembrar, se sigue diciendo: es un hecho del
  // historial, no del guardado, y ninguno de los motivos de abajo lo cambia.
  const sinBorrador = aviso.sinBorrador
    ? " El borrador tampoco se ha podido cargar con el reparto vigente."
    : "";
  // Cuando en la misma pantalla hay ademas un rechazo, los dos textos hablan de
  // guardados DISTINTOS -el aviso, del que dejo la duda; el panel de abajo, de
  // otro posterior- y hay que decirlo: sin esta frase el aviso se lee como si
  // describiera el guardado que acaba de rechazarse, y afirmaria entonces una
  // version abierta sobre un guardado que no abrio ninguna. Lo que se afirma del
  // rechazo es lo unico que un 4xx prueba, y esta escrito con el mismo criterio
  // que `avisoDelRechazo`.
  const rechazoPosterior = aviso.hayRechazoPosterior
    ? " El rechazo que cuenta el panel de abajo es de otro guardado, posterior: prueba que ESA petición no abrió ninguna versión, no lo que hizo la que dejó esta duda, así que no la resuelve."
    : "";

  if (aviso.porque === "historialNoLeido") {
    return (
      <p className="editor-aviso-version" role="status">
        No se pudo leer el historial de esta declaración ({mensaje}), así que
        esta pantalla no puede decir qué versión se cerrará ni con qué número se
        abre la nueva. {consecuencia}
        {sinBorrador}
      </p>
    );
  }

  // El 200 con un cuerpo que no se pudo leer: el servidor SI abrio una version
  // -un 200 es lo que el contrato promete cuando la abre-, pero no dijo cual. Las
  // dos formas de que el cuerpo no sirva -que no se pueda ni leer y que se lea
  // sin la forma del contrato- llegan al mismo desenlace, `guardadaSinLeer`. El
  // aviso no puede caer al historial en memoria: alli la version que se cerraria
  // es justo la que el 200 acaba de cerrar.
  if (aviso.porque === "guardadoSinLeer") {
    return (
      <p className="editor-aviso-version" role="status">
        El servidor contestó sin error, así que una versión se abrió, pero la
        respuesta no llegó con la forma del contrato y esta pantalla no puede
        decir cuál es.{rechazoPosterior} Compruébalo en el historial antes de
        volver a guardar: guardar otra vez abre una versión más. {consecuencia}
        {sinBorrador}
      </p>
    );
  }

  // Un 5xx o una respuesta perdida: no se sabe si una version quedo abierta, y
  // sin saberlo no se puede decir cual se cerraria. El panel de abajo da el
  // paso siguiente; aqui solo se deja de afirmar el numero.
  if (aviso.porque === "guardadoIncierto") {
    return (
      <p className="editor-aviso-version" role="status">
        No se sabe si el guardado abrió una versión: un fallo al confirmar es
        indistinguible de una respuesta que se perdió, así que esta pantalla no
        puede decir qué versión se cerrará ni con qué número se abre la nueva.
        {rechazoPosterior} {consecuencia}
        {sinBorrador}
      </p>
    );
  }

  return (
    <p className="editor-aviso-version">
      Con lo que el servidor devolvió en el historial, esta pantalla no puede
      decir qué versión se cerrará ni con qué número se abre la nueva.{" "}
      {consecuencia}
      {sinBorrador}
    </p>
  );
}

/**
 * Las filas del borrador: el titular que ya se eligio, su IPI y su porcentaje.
 *
 * No hay boton de "anadir fila" en blanco, y no es un olvido: una parte sin
 * `titular_id` no es una parte -el backend la rechaza-, asi que la fila nace de
 * elegir un titular en el padron, que es el unico sitio donde se sabe cual es su
 * identificador y su IPI.
 *
 * El porcentaje es un campo de texto y no `type="number"`, por la misma razon
 * que el anio del catalogo: un `number` descarta en silencio lo que no parsea
 * -un "33,5" se leeria vacio- y la pantalla no podria explicar por que su fila
 * no cuenta. Aqui lo que se teclea se lee tal cual, y la coma se admite.
 */
function FilasDelReparto({
  filas,
  focoPendiente,
  onCambiar,
  onQuitar,
}: {
  filas: readonly FilaDeReparto[];
  focoPendiente: FocoPendiente;
  onCambiar: (clave: string, cambio: Partial<FilaDeReparto>) => void;
  onQuitar: (clave: string) => void;
}) {
  // Los nodos de las filas, por su clave. Hacen falta los refs y no un
  // `querySelector` por texto o por `data-*`: la clave es lo unico que identifica
  // la fila de forma estable -el `titularId` no sirve, la misma persona puede
  // estar en dos filas mientras se edita-.
  const refsDeFilas = useRef(new Map<string, HTMLTableRowElement>());

  // El salto de foco, UNA sola vez por cambio. `filas` cambia tambien al
  // teclear, asi que este efecto corre muchas veces; lo que lo limita a un solo
  // salto es vaciar `focoPendiente` en la primera pasada. El destino es el primer
  // control de la fila -su primer campo-, que es lo que "la fila que ocupa el
  // hueco" significa para quien llega tabulando. Si no hay fila con esa clave
  // -la rejilla se quedo vacia- no se enfoca nada: no hay destino que prometer.
  useEffect(() => {
    const clave = focoPendiente.current;
    if (clave === null) return;
    focoPendiente.current = null;
    refsDeFilas.current
      .get(clave)
      ?.querySelector<HTMLElement>("input, button")
      ?.focus();
  }, [filas, focoPendiente]);

  if (filas.length === 0) {
    return (
      <div className="catalogo-vacio">
        <p>El borrador no tiene ninguna parte.</p>
        <p className="muted">
          Añade titulares desde el padrón de abajo: el reparto se declara sobre
          quien figura en él.
        </p>
      </div>
    );
  }

  return (
    <div className="catalogo-caja">
      <table
        className="tabla-partes editor-filas"
        aria-label="Borrador del reparto"
      >
        <thead>
          <tr>
            <th scope="col">Titular</th>
            <th scope="col">IPI</th>
            <th scope="col" className="tabla-partes-numero">
              Porcentaje
            </th>
            <th scope="col" aria-label="Acciones" />
          </tr>
        </thead>
        <tbody>
          {filas.map((fila) => (
            <tr
              key={fila.clave}
              ref={(nodo) => {
                // Al desmontarse la fila React llama con `null`, y esa clave hay
                // que borrarla: si no, el mapa se queda con el nodo de una fila
                // que ya no esta y el foco podria irse a un elemento muerto.
                if (nodo === null) refsDeFilas.current.delete(fila.clave);
                else refsDeFilas.current.set(fila.clave, nodo);
              }}
            >
              <td className="detalle-identificador">{fila.titularId}</td>
              <td>
                {/* El IPI se rellena con el que el padron tiene hoy para ese
                    titular, y se puede corregir: la parte que se guarda lleva
                    el IPI declarado. Quien lo concilia es el backend, en
                    exigirPuedenRecibirReparto (internal/aplicacion/declaraciones.go),
                    que ya trae la fila del padron para R-01 y compara su IPI
                    con este: si no cuadra no se guarda nada y la peticion sale
                    con 400. Esta pantalla no lo comprueba porque no tiene el
                    padron; lo que no puede hacer es dar por bueno lo que
                    escriba. */}
                <input
                  className="editor-campo"
                  type="text"
                  autoComplete="off"
                  aria-label={`IPI de ${fila.titularId}`}
                  value={fila.ipi}
                  onChange={(e) =>
                    onCambiar(fila.clave, { ipi: e.target.value })
                  }
                />
              </td>
              <td className="tabla-partes-numero">
                <input
                  className="editor-campo editor-campo-numero"
                  type="text"
                  inputMode="decimal"
                  autoComplete="off"
                  aria-label={`Porcentaje de ${fila.titularId}`}
                  value={fila.porcentaje}
                  onChange={(e) =>
                    onCambiar(fila.clave, { porcentaje: e.target.value })
                  }
                />
              </td>
              <td>
                {/* El texto visible no cambia: dice la accion y el titular, y
                    quien ve la pantalla ya sabe de que fila es el boton. Lo que
                    se arregla es el nombre accesible, que es lo que anuncia un
                    lector de pantalla y lo que hoy se lee al reves: "Quitar de
                    tit-3" suena a quitarle algo A tit-3, y no dice a quien se
                    saca del reparto. El dia que la fila sepa el nombre del
                    titular -hoy solo guarda su identificador-, esto lo nombrara
                    a el. */}
                <button
                  type="button"
                  className="enlace"
                  aria-label={`Quitar a ${fila.titularId} del reparto`}
                  onClick={() => onQuitar(fila.clave)}
                >
                  Quitar de {fila.titularId}
                </button>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

/**
 * El padron de titulares: de donde se eligen las partes.
 *
 * Se pide SIN filtro de `persona_natural`, y eso es una decision, no un
 * descuido: el contrato devuelve el padron entero justamente para que quien
 * edita pueda explicar por que el titular que busca no esta entre los elegibles.
 * Pidiendo solo personas naturales, la productora desapareceria del listado y la
 * regla se leeria como un dato que falta en vez de como `R-01`.
 *
 * La pagina y la busqueda son de esta pantalla y no de la URL: son la
 * herramienta con la que se busca un titular, no un estado del reparto, y un
 * enlace compartido que abriera el editor con el padron ya filtrado diria del
 * reparto algo que no es.
 */
function PadronDelEditor({
  enElReparto,
  onAgregar,
}: {
  enElReparto: readonly string[];
  onAgregar: (titular: Titular) => void;
}) {
  const [busqueda, setBusqueda] = useState("");
  const [desplazamiento, setDesplazamiento] = useState(0);
  const idNombre = useId();
  const idAyuda = useId();

  const params = new URLSearchParams();
  if (busqueda !== "") params.set("nombre", busqueda);
  params.set("limite", String(LIMITE_PADRON));
  if (desplazamiento > 0) {
    params.set("desplazamiento", String(desplazamiento));
  }
  const {
    datos: titulares,
    cargando,
    error,
  } = useApi<Titular[]>(`/api/titulares?${params.toString()}`);

  /** Cambiar la busqueda vuelve a la primera pagina: la 3 de otra busqueda no existe. */
  function buscar(valor: string) {
    setBusqueda(valor);
    setDesplazamiento(0);
  }

  let contenido: ReactElement;
  if (cargando) {
    contenido = <Cargando texto="Cargando el padrón…" />;
  } else if (error) {
    // Sin padron no se pueden elegir partes, y la pantalla lo dice: sin este
    // aviso el borrador solo se podria llenar a ciegas. Lo que no se hace es
    // dejar de ofrecer el guardado de lo ya escrito -el padron no es la
    // autoridad de nada, solo la lista de donde elegir-.
    contenido = (
      <p className="catalogo-error" role="alert">
        No se pudo consultar el padrón de titulares: {error.message} Sin el
        padrón no se pueden elegir partes, pero lo que ya esté en el borrador
        sigue siendo lo que se guarda.
      </p>
    );
  } else if (!Array.isArray(titulares) || !titulares.every(esTitular)) {
    contenido = (
      <p className="catalogo-error" role="alert">
        El padrón no llegó como una lista de titulares legibles.
      </p>
    );
  } else if (titulares.length === 0) {
    contenido = (
      <div className="catalogo-vacio">
        <p>
          {busqueda === ""
            ? "El padrón no trae ningún titular en esta página."
            : "Ningún titular del padrón coincide con esa búsqueda."}
        </p>
      </div>
    );
  } else {
    contenido = (
      <>
        <div className="catalogo-caja">
          <table className="tabla-partes" aria-label="Padrón de titulares">
            <thead>
              <tr>
                <th scope="col">Nombre</th>
                <th scope="col">Titular</th>
                <th scope="col">IPI</th>
                <th scope="col">Clase</th>
                <th scope="col">Puede ser parte</th>
              </tr>
            </thead>
            <tbody>
              {titulares.map((titular) => (
                <tr key={titular.id}>
                  {/* "Nombre" a secas y no el rotulo del padron actual: esta
                      tabla ES el padron leido ahora, no el nombre de un titular
                      junto a una version historica. El dia que un nombre se
                      pinte al lado de una version, ese si lleva el rotulo
                      (D-006). */}
                  <td>{titular.nombre}</td>
                  <td className="detalle-identificador">{titular.id}</td>
                  {/* Vacio es "no se conoce": el padron solo exige IPI a las
                      personas naturales, y esta celda no puede decir mas. */}
                  <td className="detalle-identificador">
                    {titular.ipi !== "" ? titular.ipi : "—"}
                  </td>
                  <td>{titular.clase}</td>
                  <td>
                    {puedeSerParte(titular) ? (
                      enElReparto.includes(titular.id) ? (
                        <span className="muted">Ya está en el reparto</span>
                      ) : (
                        <button
                          type="button"
                          className="catalogo-limpiar"
                          onClick={() => onAgregar(titular)}
                        >
                          Añadir al reparto
                        </button>
                      )
                    ) : (
                      // La explicacion, no un hueco: el titular existe en el
                      // padron y no se ofrece, y sin esta frase pareceria que
                      // falta un dato. La regla la impone el backend al guardar
                      // -400 con su propio mensaje-, y este es el mismo hecho
                      // dicho antes.
                      <span className="editor-no-elegible">
                        No: solo un escritor persona natural puede ser titular
                        de una declaración (R-01, RD 4.5)
                      </span>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          <div className="catalogo-pie">
            <span>
              {desplazamiento === 0
                ? `Titulares 1 a ${titulares.length}`
                : `Titulares ${desplazamiento + 1} a ${desplazamiento + titulares.length}`}
            </span>
            <div className="catalogo-pie-botones">
              <button
                type="button"
                className="catalogo-pagina"
                disabled={desplazamiento === 0}
                onClick={() =>
                  setDesplazamiento(Math.max(0, desplazamiento - LIMITE_PADRON))
                }
              >
                Anterior
              </button>
              <button
                type="button"
                className="catalogo-pagina"
                disabled={titulares.length < LIMITE_PADRON}
                onClick={() =>
                  setDesplazamiento(desplazamiento + LIMITE_PADRON)
                }
              >
                Siguiente
              </button>
            </div>
          </div>
        </div>
        <p className="muted detalle-nota">
          El padrón se sirve por páginas y no admite filtrar por identificador,
          así que una página que no traiga al titular no prueba que no exista.
          El nombre y la clase son los que el padrón tiene hoy.
        </p>
      </>
    );
  }

  return (
    <section className="editor-padron">
      <h2>Padrón de titulares</h2>
      <p className="muted detalle-nota">
        Se listan las dos clases de titular a propósito: quien está en el padrón
        y no puede figurar como parte se explica aquí en vez de faltar. Estar en
        el padrón no da derecho a cobrar por sí solo: el porcentaje sale de esta
        declaración (R-02, R-03).
      </p>
      <div className="catalogo-campo">
        <label htmlFor={idNombre}>Buscar por nombre</label>
        <input
          id={idNombre}
          type="text"
          autoComplete="off"
          placeholder="Buscar en el padrón…"
          value={busqueda}
          onChange={(e) => buscar(e.target.value)}
          aria-describedby={idAyuda}
        />
        <p id={idAyuda} className="catalogo-ayuda">
          Coincidencia parcial, sin distinguir mayúsculas.
        </p>
      </div>
      {contenido}
    </section>
  );
}

/**
 * Lo que contesto el guardado.
 *
 * El caso que importa es `incierta`: **no dice que no se guardara nada**. Un 5xx
 * es indistinguible de un COMMIT que si entro -ver `puedeHaberGuardado`-, asi
 * que lo unico honesto es decir que no se sabe y mandar al historial, que es
 * donde se comprueba. El mismo criterio que #29 aplico al 500 de la ingesta.
 */
function PanelDelGuardado({
  resultado,
  obraId,
  busqueda,
  volver,
}: {
  resultado: ResultadoDelGuardado;
  obraId: string;
  busqueda: string;
  volver: string;
}) {
  // La misma busqueda del catalogo que lleva la ficha viaja al historial: el
  // camino de vuelta no pierde los filtros por pasar por dos pantallas.
  const enlaceAlHistorial = (
    <p className="detalle-nota">
      <Link
        to={`/catalogo/${encodeURIComponent(obraId)}/historial`}
        state={{ [CLAVE_DE_VUELTA_AL_CATALOGO]: busqueda }}
      >
        Ver el historial de la declaración
      </Link>
    </p>
  );

  if (resultado.tipo === "guardada") {
    const { version } = resultado.version;
    return (
      <section className="panel-resultado panel-ok" role="status">
        <h2>El servidor abrió la versión {version}</h2>
        {/* El numero, el estado y la fecha son los de la respuesta: no hay
            ninguna cifra calculada aqui. */}
        <dl className="panel-datos">
          <div>
            <dt>Versión</dt>
            <dd>{version}</dd>
          </div>
          <div>
            <dt>Estado</dt>
            <dd>
              <EtiquetaDeEstado estado={resultado.version.estado} />
            </dd>
          </div>
          <div>
            <dt>Vigente desde</dt>
            <dd>
              <time
                dateTime={resultado.version.vigente_desde}
                title={resultado.version.vigente_desde}
              >
                {formatearInstante(resultado.version.vigente_desde)}
              </time>
            </dd>
          </div>
        </dl>
        {/* El parrafo va ramificado por el numero que el servidor acaba de
            contestar, y no es un detalle de estilo: con la version 1 -el caso
            central del issue, una obra sin ninguna declaracion- NO hay ninguna
            version anterior, asi que el texto que se pintaba sin condicion
            afirmaba un hecho del registro que no existe. Es la misma disciplina
            del aviso de arriba, que se cura con "si la hay". Las dos ramas
            dicen cosas que la otra no desmiente. */}
        {version === 1 ? (
          <p className="panel-aviso">
            Esta es la primera declaración de la obra: no había ninguna versión
            anterior que conservar, y esta queda en el historial desde ahora con
            los porcentajes que se acaban de declarar. Un reparto de un periodo
            pasado se reproduce con el reparto que regía entonces, y antes de
            esta versión no había ninguno declarado.
          </p>
        ) : (
          <p className="panel-aviso">
            La versión anterior no se borra ni se modifica: sigue en el
            historial con los porcentajes que regían hasta ahora. Un reparto de
            un periodo pasado se reproduce con el reparto que estaba vigente
            entonces.
          </p>
        )}
        {enlaceAlHistorial}
      </section>
    );
  }

  if (resultado.tipo === "guardadaSinLeer") {
    return (
      <section className="panel-resultado panel-parcial" role="alert">
        {/* "Contesto sin error, sin la version nueva" y no "se guardo": un 200
            es lo que el contrato promete cuando la version se abre, pero de un
            cuerpo que no se puede leer lo unico que se sabe es el status. */}
        <h2>El servidor contestó sin error, sin la versión nueva</h2>
        <p className="panel-mensaje">
          El contrato promete la versión nueva en el cuerpo de un 200, y este no
          llegó con esa forma: esta pantalla no puede decir qué versión se abrió
          ni con qué estado quedó. El historial es donde se comprueba.
        </p>
        {enlaceAlHistorial}
      </section>
    );
  }

  if (resultado.tipo === "rechazada") {
    return (
      <section className="panel-resultado panel-fallo" role="alert">
        <h2>{tituloDelRechazo(resultado.status)}</h2>
        {/* El mensaje del backend, entero y tal cual: nombra la regla que se
            incumplio y cual de ellas ha sido solo lo sabe el. Reescribirlo
            acoplaria la pantalla a la prosa de Go y se romperia en silencio. */}
        <p className="panel-mensaje">{resultado.mensaje}</p>
        {/* El aviso se pinta en todos los 4xx que llegan hasta esta pantalla
            -el 400, el 403 y el 404; el 401 lo corta `api()` antes, ver
            `avisoDelRechazo`-, y no solo en el 400: en los tres la version nueva
            no se abrio, y callarlo dejaba a quien edita sin saber si puede
            reintentar con tranquilidad. */}
        <p>{avisoDelRechazo(resultado.status)}</p>
        {resultado.status === 404 && (
          <p className="detalle-nota">
            <Link to={volver}>Volver al catálogo</Link>
          </p>
        )}
      </section>
    );
  }

  return (
    <section className="panel-resultado panel-fallo" role="alert">
      <h2>No se sabe si la declaración se guardó</h2>
      {/* El mensaje del servidor va igual: lo que no se afirma es que no haya
          quedado nada escrito. */}
      <p className="panel-mensaje">{resultado.mensaje}</p>
      {/* El aviso que este paso existe para no olvidar. `Store.Guardar` corre
          dentro de `EnTransaccion`, asi que el fallo del COMMIT es
          indistinguible de uno limpio y la version PUEDE haberse abierto. */}
      <p>
        No se puede distinguir un fallo al confirmar de una respuesta que se
        perdió, así que la versión pudo haberse abierto igual. Compruébalo en el
        historial antes de volver a guardar: si se abrió, guardar otra vez abre
        una versión de más.
      </p>
      {enlaceAlHistorial}
    </section>
  );
}

/** El titulo del rechazo. Solo afirma lo que el status dice. */
function tituloDelRechazo(status: number): string {
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
function avisoDelRechazo(status: number): string {
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
