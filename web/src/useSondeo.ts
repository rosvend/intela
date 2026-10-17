import { useEffect, useRef } from "react";
import { SESION_INACTIVIDAD_MS } from "./sesion";

/**
 * Lo que cuenta como que la persona sigue delante: teclado y puntero. La rueda
 * va aparte porque desplazar con ella no mueve el puntero. Que la pestana vuelva
 * a verse NO cuenta: una pantalla tambien se enciende sin nadie delante.
 */
const EVENTOS_DE_LA_PERSONA = [
  "keydown",
  "pointerdown",
  "pointermove",
  "wheel",
] as const;

// En captura, para que un `stopPropagation` de algun componente no esconda la
// actividad; y pasivos, para no frenar el desplazamiento.
const OPCIONES: AddEventListenerOptions = { capture: true, passive: true };

/**
 * Llama a `sondear` cada `intervaloMs`, pero solo mientras alguien atiende la
 * pantalla.
 *
 * El servidor mide la inactividad por peticiones (ADR 0025): cada una que lleva
 * el token corre `sesiones.ultimo_uso`, y no sabe si la pidio una persona o un
 * temporizador. Un `setInterval` sin condiciones deja viva hasta `SESION_TTL`
 * (12 h) la sesion de una pestana desatendida (#223). Por eso:
 *
 * - con la pestana oculta no se pide nada, y al volver a verse se pide enseguida;
 * - pasado `SESION_INACTIVIDAD_MS` sin teclado ni puntero se deja de pedir: el
 *   ultimo sondeo sale antes de ese plazo y desde ahi corre la inactividad del
 *   servidor;
 * - la siguiente tecla o movimiento pide enseguida. Si la sesion ya cayo, ese
 *   401 lleva a la pantalla de entrada (`api()` y `RutaProtegida`).
 *
 * Al montar no se pide: la pantalla acaba de pedir sus datos. Todo sondeo que
 * lleve el token va por aqui; un `setInterval` directo vuelve a abrir #223.
 */
export function useSondeo(sondear: () => void, intervaloMs: number): void {
  const ultimo = useRef(sondear);
  useEffect(() => {
    ultimo.current = sondear;
  }, [sondear]);

  useEffect(() => {
    let actividad = Date.now();
    let temporizador: number | undefined;

    const oculta = () => document.visibilityState === "hidden";
    const atendida = () => Date.now() - actividad < SESION_INACTIVIDAD_MS;

    function parar() {
      window.clearInterval(temporizador);
      temporizador = undefined;
    }

    function arrancar() {
      temporizador = window.setInterval(() => {
        if (oculta() || !atendida()) parar();
        else ultimo.current();
      }, intervaloMs);
    }

    // Lo que hay en pantalla puede llevar rato sin refrescarse: se pide ya.
    function reanudar() {
      if (temporizador !== undefined || oculta() || !atendida()) return;
      ultimo.current();
      arrancar();
    }

    function alActuar() {
      actividad = Date.now();
      reanudar();
    }

    // Al ocultarse no hace falta parar aqui: el siguiente tic ve la pestana
    // oculta y para sin pedir. Al volver a verse, `reanudar`.
    arrancar();
    for (const evento of EVENTOS_DE_LA_PERSONA) {
      window.addEventListener(evento, alActuar, OPCIONES);
    }
    document.addEventListener("visibilitychange", reanudar);
    return () => {
      parar();
      for (const evento of EVENTOS_DE_LA_PERSONA) {
        window.removeEventListener(evento, alActuar, OPCIONES);
      }
      document.removeEventListener("visibilitychange", reanudar);
    };
  }, [intervaloMs]);
}
