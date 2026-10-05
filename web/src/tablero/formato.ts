import { formatearCOP } from "../ui/dinero";

// Reexporta la unica suma de dinero (ui/dinero) para src/reparto, que aun importa de aqui.

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
