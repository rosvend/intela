/**
 * Logica pura del reparto: sin React y sin red, para poder probarla
 * directamente. De aqui salen el total de un borrador, la tolerancia con que
 * ese total se compara con 100, el formato de los porcentajes, el rotulo con
 * que se presenta el nombre de un titular del padron y, desde el item 12 de
 * #135, lo que se sabe de la VERSION que un guardado cierra y abre -el aviso
 * que la pantalla del editor enseña, con su regla de precedencia-. Los pasos 6
 * a 8 (detalle, historial y editor) consumen varias de estas funciones.
 *
 * La frontera que hay que respetar, y por la que esta escrito todo lo de
 * abajo: NINGUNA funcion de este archivo decide el estado ni la suma de una
 * declaracion PERSISTIDA. Eso lo calcula el backend
 * (`repertorio.Declaracion.Estado()`, `internal/dominio/repertorio/declaracion.go`)
 * y viaja en `estado_declaracion` y `suma_porcentajes`: la pantalla lo muestra
 * tal cual. Re-derivarlo en el cliente es como el cliente empieza a discrepar
 * de la autoridad y a pintar "Completa" sobre una obra que el motor de reparto
 * retiene; el redondeo de la suma basta para que eso pase.
 *
 * Lo que si vive aqui es el BORRADOR -lo que alguien esta tecleando y todavia
 * no tiene estado de backend, porque no existe para el servidor-, y el formato
 * con que se muestran las cifras que el backend manda.
 */
import type { VersionDeclaracion } from "./tipos";

/**
 * Tolerancia con que un total se compara con 100.
 *
 * Es la que usa el prototipo de Make (`Math.abs(total - 100) < 0.00001`) y
 * existe porque la igualdad exacta de flotantes falla justo donde importa:
 * sumar 33.3333 + 33.3333 + 33.3334 en coma flotante da 100.00000000000001, y
 * una comparacion estricta diria que esa declaracion no suma 100. Con la
 * tolerancia se lee como lo que es -100, clavado-, sin inventar precision.
 *
 * Lo que la tolerancia NO es: una licencia para mandar sumas por encima de
 * 100. `NuevaDeclaracion` rechaza con 400 toda suma mayor que 100 comparando
 * con `decimal`, sin tolerancia, asi que un total que se pase por menos de
 * 1e-5 -el unico que esta tolerancia deja pasar- lo frena el backend con su
 * mensaje; el issue pide justamente eso ("and shows the backend error if it
 * slips through"). En la practica ese caso solo puede venir de un porcentaje
 * con mas de 4 decimales, que el backend rechaza por su cuenta.
 */
export const TOLERANCIA_SUMA = 1e-5;

/**
 * Los cuatro estados de un borrador. Ninguno es un estado del sistema: los de
 * una declaracion guardada son dos (`completa` / `incompleta`) y los asigna el
 * backend. Estos describen lo que hay ahora mismo en pantalla mientras se
 * edita, que es un dato que solo existe aqui:
 *
 * - `vacia`: no hay ningun porcentaje declarado. El backend rechaza ese cuerpo
 *   ("no trae ninguna parte"), asi que la pantalla no ofrece guardarlo.
 * - `completa`: el total es 100 dentro de la tolerancia.
 * - `incompleta`: el total esta por debajo de 100. **Es un estado VALIDO y
 *   guardable**: bajo R-04 (`RD 13.1.3`) no se reparte nada de esa obra y se
 *   retiene el total, pero la declaracion se guarda igual. No es un error de
 *   quien la esta escribiendo y la pantalla no lo puede tratar como tal.
 * - `excedida`: el total pasa de 100. Es lo unico que el backend rechaza de
 *   verdad al guardar, y por eso es lo unico que la pantalla bloquea.
 */
export type EstadoBorrador = "vacia" | "completa" | "incompleta" | "excedida";

/**
 * El total de un borrador: la suma de los porcentajes de sus filas.
 *
 * Se calcula en el cliente porque un borrador no tiene estado de backend: no
 * existe para el servidor hasta que se guarda. El total de una declaracion YA
 * guardada NO sale de aqui: lo manda el backend en `suma_porcentajes`, y quien
 * lo pinte tiene que usar ese campo (por eso este archivo no expone ninguna
 * funcion que reciba las partes de una version).
 *
 * Un porcentaje que no sea un numero finito cuenta como 0 y no envenena el
 * total con NaN: el tipo promete `number`, pero en el editor el valor sale de
 * un campo de texto y un campo vacio llega como `NaN` mientras se teclea.
 */
export function totalDeclarado(
  partes: readonly { porcentaje: number }[],
): number {
  return partes.reduce(
    (suma, parte) =>
      suma + (Number.isFinite(parte.porcentaje) ? parte.porcentaje : 0),
    0,
  );
}

/**
 * El estado del borrador a partir de su total, con los mismos tres cortes que
 * le importan al backend: nada declarado, mas de 100 (lo unico que rechaza) y
 * la comparacion con 100 dentro de la tolerancia.
 *
 * Un total que no llega a positivo -0, y tambien negativo, que es lo que deja
 * una fila mientras se teclea un signo- cuenta como borrador vacio: no hay
 * nada declarado que guardar, y `NuevaDeclaracion` rechaza ese cuerpo igual
 * (exige al menos una parte, y cada una estrictamente positiva).
 *
 * **Lo que el cliente NO hace hoy es validar el signo de una fila.** El
 * comentario que decia que si -"la fila negativa es cosa del editor, que es
 * quien la valida al escribirla"- era falso, y esta escrito aqui para que no
 * vuelva: lo que mira esta funcion es el TOTAL, y el editor comprueba por fila
 * una sola cosa, que su porcentaje se lea como un numero finito
 * (`Number.isFinite`, en `EditorReparto.tsx`). Un borrador de -5 y 105 suma 100,
 * o sea que se lee como `completa`, la pantalla SI ofrece guardar y lo unico
 * que lo frena es el 400 del servidor.
 *
 * El reparto de responsabilidades es ese: el CLIENTE decide sobre la forma del
 * cuerpo y sobre el total; el SERVIDOR aplica la regla, y `NuevaDeclaracion`
 * exige un porcentaje estrictamente positivo. Su 400 es el mensaje que se
 * enseña.
 */
export function estadoDelBorrador(total: number): EstadoBorrador {
  if (!(total > 0)) return "vacia";
  const diferencia = total - 100;
  if (Math.abs(diferencia) < TOLERANCIA_SUMA) return "completa";
  if (diferencia > 0) return "excedida";
  return "incompleta";
}

/**
 * Si el borrador se puede guardar.
 *
 * Se bloquean dos cuerpos, y solo dos: uno que no declara nada -sin ninguna
 * fila, o con un total que no llega a positivo- y uno cuya suma pasa de 100.
 * **No se bloquea la suma menor a 100**, y eso no es un descuido: es la
 * asimetria del contrato y de R-04. Una declaracion que suma 60 se guarda,
 * queda `incompleta` y su importe se retiene entero; bloquearla contradiria el
 * contrato y haria imposible declarar a proposito por debajo de 100.
 *
 * Y esos dos NO son "los dos cuerpos que el backend rechaza con 400", que es lo
 * que decia el comentario de aqui: `NuevaDeclaracion` rechaza ademas un IPI
 * ausente, un porcentaje no positivo, mas de cuatro decimales y un titular
 * repetido. De esos cuatro, el cliente adelanta **uno**: el titular repetido no
 * se puede ni anadir, porque el padron sustituye "Añadir al reparto" por "Ya
 * está en el reparto" para quien ya esta en el borrador
 * (`EditorReparto.tsx`), y una fila solo nace de ese boton -asi que ese 400 es
 * inalcanzable desde esta pantalla-. Los otros tres SI llegan al 400 del
 * servidor: por fila esta pantalla solo comprueba que el porcentaje se lea como
 * un numero -ni el signo ni la cuenta de decimales-, y del IPI no comprueba
 * nada, asi que un IPI borrado a mano viaja igual. Ese 400 trae su mensaje y la
 * pantalla lo enseña tal cual.
 */
export function puedeGuardarBorrador(estado: EstadoBorrador): boolean {
  return estado !== "vacia" && estado !== "excedida";
}

/**
 * Un porcentaje como se muestra en pantalla: 4 decimales, que es la precision
 * de la columna (`NUMERIC(8,4)`) y la del prototipo.
 *
 * Se formatea con `toFixed` y no con `Intl` en es-CO a proposito: la misma
 * cifra aparece en el mensaje del 400 del backend, que la escribe asi
 * -`suma.StringFixed(4)`-, y dos formas de escribir el mismo numero en la
 * misma pantalla se leen como dos numeros distintos. Ademas `toFixed(4)` no
 * redondea un 4-decimal por debajo de 100 hasta "100.0000": el mayor de ellos
 * es 99.9999 y se pinta 99.9999, asi que la pantalla no puede contradecir el
 * estado que acaba de recibir.
 *
 * Quien llama garantiza que el valor es un numero finito: `esObra` lo exige
 * para `suma_porcentajes` y `totalDeclarado` nunca devuelve `NaN`.
 */
export function formatearPorcentaje(valor: number): string {
  return `${valor.toFixed(4)}%`;
}

/**
 * El rotulo de la columna que muestra el nombre de un titular en las tablas de
 * partes (detalle e historial, pasos 6 y 7).
 *
 * Dice "en el padron actual" porque es el unico nombre que el sistema tendria
 * derecho a mostrar: `Parte` trae `titular_id` e `ipi`, no el nombre, y
 * resolverlo contra el padron de HOY hace que un titular que se renombre
 * aparezca con su nombre nuevo tambien en las versiones antiguas, o sea un
 * hecho historico que el sistema no guarda. Congelar el nombre con la version
 * es lo correcto a largo plazo y sigue anotado como pendiente: cambia la forma
 * de respuestas ya entregadas.
 *
 * **Hoy no se resuelve ningun nombre, y el rotulo existe para el dia en que se
 * resuelva.** `TablaDePartes.tsx` pinta un guion en todas las filas de esa
 * columna, y el parrafo de esa misma pantalla dice por que: no busca el nombre.
 * El motivo es del contrato, no de la pantalla: `GET /titulares` no admite
 * filtrar por identificador (`internal/infraestructura/httpapi/titulares.go`),
 * asi que con el `titular_id` de una fila en la mano no hay forma de pedir su
 * nombre sin traer paginas enteras del padron y cruzarlas en el cliente.
 *
 * Esto es una DESVIACION de D-006, y se registra aqui porque el comentario que
 * habia en este sitio afirmaba lo contrario -que el nombre "se resuelve hoy
 * contra el padron de hoy"-. D-006 decidio resolverlo en el cliente contra el
 * padron que el editor ya carga; el paso 6 dejo esa resolucion sin implementar
 * -y lo registro en `progress.md`-, pero el registro de decisiones se quedo sin
 * corregir. La resolucion contra el padron sigue siendo una decision registrada
 * y **NO implementada**. Alinear las dos fuentes es trabajo propio, no un
 * efecto secundario de este arreglo.
 */
export const ROTULO_NOMBRE_EN_PADRON_ACTUAL = "Nombre en el padrón actual";

/**
 * La version que rige hoy: la unica que el historial deja sin cerrar.
 *
 * Devuelve `null` si no hay exactamente una. No es una guarda de mas: el numero
 * que se lee de aqui es el que el aviso de la version enseña como "se cerrara
 * la version N", y con dos abiertas -o con ninguna- el backend cerraria otra
 * cosa. Antes que afirmar un numero que no se sostiene, no se afirma ninguno.
 */
export function versionAbierta(
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
 *
 * Vive aqui, junto a `avisoDeVersion` y no en `borrador.ts` con el resto de
 * `ResultadoDelGuardado`, por una razon de direccion y no de tema: es una
 * ENTRADA del aviso y el aviso lo interpreta, mientras que `borrador.ts` no lo
 * necesita para nada. Puesto en `borrador.ts`, los dos modulos se importarian
 * mutuamente -`filasDePartida` necesita `versionAbierta`, que es de este
 * archivo-, y un ciclo entre dos modulos por un tipo es el tipo de nudo que
 * despues nadie se atreve a deshacer. La direccion queda en un solo sentido:
 * `borrador.ts` -> `declaracion.ts`.
 */
export type MotivoDeDuda = "guardadoSinLeer" | "guardadoIncierto";

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
export type AvisoDeVersion =
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
export function avisoDeVersion(
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
