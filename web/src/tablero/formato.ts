import { formatearCOP } from "../ui/dinero";

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

/** Importe neto como string decimal, en pesos colombianos. */
export function formatearImporte(neto: string): string {
  return formatearCOP(neto);
}

const DECIMAL = /^(-)?(\d+)(?:\.(\d{1,2}))?\d*$/;

/** Suma exacta de importes decimales (centavos en BigInt); lo ilegible no cuenta. */
export function sumarImportes(importes: readonly string[]): string {
  let centavos = 0n;
  for (const importe of importes) {
    const m = DECIMAL.exec(importe.trim());
    if (!m) continue;
    const [, signo, entero, fraccion = ""] = m;
    const valor = BigInt(entero) * 100n + BigInt((fraccion + "00").slice(0, 2));
    centavos += signo ? -valor : valor;
  }
  const negativo = centavos < 0n;
  const abs = negativo ? -centavos : centavos;
  const resto = (abs % 100n).toString().padStart(2, "0");
  return `${negativo ? "-" : ""}${abs / 100n}.${resto}`;
}

const RELATIVO = new Intl.RelativeTimeFormat("es-CO", { numeric: "auto" });
const MINUTO = 60_000;
const HORA = 60 * MINUTO;
const DIA = 24 * HORA;

/** "hace 5 minutos", "ayer"; pasada una semana, la fecha. `ahora` entra por parametro. */
export function tiempoRelativo(iso: string, ahora: Date): string {
  const fecha = new Date(iso);
  if (Number.isNaN(fecha.getTime())) return iso;
  const delta = ahora.getTime() - fecha.getTime();
  if (delta < MINUTO) return "hace un momento";
  if (delta < HORA)
    return RELATIVO.format(-Math.floor(delta / MINUTO), "minute");
  if (delta < DIA) return RELATIVO.format(-Math.floor(delta / HORA), "hour");
  if (delta < 7 * DIA) return RELATIVO.format(-Math.floor(delta / DIA), "day");
  return formatearInstante(iso);
}
