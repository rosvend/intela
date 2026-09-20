export function formatearEntero(n: number): string {
  return new Intl.NumberFormat("es-CO").format(n);
}

const FORMATO_DE_INSTANTE = new Intl.DateTimeFormat("es-CO", {
  dateStyle: "medium",
  timeStyle: "short",
});

/**
 * Un instante ISO 8601, como lo escribe la casa: fecha y hora en es-CO.
 *
 * Nacio privado en el listado de cargas de #29 (`recibido`) y se mueve aqui al
 * aparecer el segundo consumidor, el historial de versiones de la declaracion
 * (#30, paso 7), que pinta `vigente_desde` y `vigente_hasta`. Dos copias de la
 * misma configuracion de `Intl` es como dos pantallas acaban escribiendo el
 * mismo instante de dos formas distintas, y aqui la fecha es un dato de la
 * auditoria: la ventana de una version es lo que deja reproducir un reparto
 * pasado con el split que regia ENTONCES.
 *
 * Una fecha ilegible se devuelve tal cual: `format` lanzaria `RangeError` y
 * tumbaria la pantalla entera por una sola celda. Quien la pinta la mete en un
 * `<time dateTime={iso}>`, asi que el valor exacto que mando el servidor sigue
 * ahi, sin recortar.
 */
export function formatearInstante(iso: string): string {
  const fecha = new Date(iso);
  return Number.isNaN(fecha.getTime())
    ? iso
    : FORMATO_DE_INSTANTE.format(fecha);
}

/**
 * Importe neto como string decimal. Agrupa miles y antepone `$` sin
 * convertirlo a number: el tablero no hace aritmetica sobre la cifra.
 */
export function formatearImporte(neto: string): string {
  const negativo = neto.startsWith("-");
  const absoluto = negativo ? neto.slice(1) : neto;
  const [entero = "0", fraccion] = absoluto.split(".");
  const agrupado = entero.replace(/\B(?=(\d{3})+(?!\d))/g, ".");
  const decimales = fraccion !== undefined ? `,${fraccion}` : "";
  return `${negativo ? "-" : ""}$ ${agrupado}${decimales}`;
}
