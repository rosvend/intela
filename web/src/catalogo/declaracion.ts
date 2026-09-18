/**
 * Logica pura del reparto: sin React y sin red, para poder probarla
 * directamente. De aqui salen el total de un borrador, la tolerancia con que
 * ese total se compara con 100, el formato de los porcentajes y el rotulo con
 * que se presenta el nombre de un titular del padron. Los pasos 6 a 8 (detalle,
 * historial y editor) consumen varias de estas funciones.
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
 * (exige al menos una parte, y cada una estrictamente positiva). La fila
 * negativa es cosa del editor, que es quien la valida al escribirla.
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
 * Se bloquean exactamente los dos cuerpos que el backend rechaza con 400: uno
 * sin ninguna parte, y uno cuya suma pasa de 100. **No se bloquea la suma
 * menor a 100**, y eso no es un descuido: es la asimetria del contrato y de
 * R-04. Una declaracion que suma 60 se guarda, queda `incompleta` y su importe
 * se retiene entero; bloquearla contradiria el contrato y haria imposible
 * declarar a proposito por debajo de 100.
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
 * Dice "en el padron actual" porque es el unico dato que el sistema tiene:
 * `Parte` trae `titular_id` e `ipi`, no el nombre, y el nombre se resuelve hoy
 * contra el padron de hoy. Un titular que se renombre despues aparecera con su
 * nombre nuevo tambien en las versiones antiguas, asi que la columna rotulada
 * como "Nombre" a secas afirmaria un hecho historico que el sistema no guarda.
 * Con el rotulo, la columna dice exactamente de donde sale el dato, y el
 * `titular_id` va visible al lado para poder conciliar la pantalla con la API.
 *
 * Congelar el nombre con la version es lo correcto a largo plazo y esta
 * anotado como pendiente (D-006): cambia la forma de respuestas ya entregadas.
 */
export const ROTULO_NOMBRE_EN_PADRON_ACTUAL = "Nombre en el padrón actual";
