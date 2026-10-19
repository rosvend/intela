import { useCallback, useEffect, useRef, useState } from "react";
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

export type Sondeo = {
  /** El sondeo paro y la pantalla todavia no recibio datos posteriores. */
  enPausa: boolean;
  /** La pantalla llama esto cuando le llega la respuesta de un sondeo. */
  recibido: () => void;
};

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
 * lleve el token va por aqui; ESLint rechaza un `setInterval` directo en el
 * resto de `src/` (#223).
 *
 * Mientras el sondeo esta parado lo que hay en pantalla puede tener horas, asi
 * que `enPausa` lo dice. No vuelve a `false` al reanudar sino cuando la pantalla
 * avisa con `recibido()` de que llego una respuesta pedida despues de reanudar:
 * hasta entonces sigue mostrando lo viejo, y una firma no puede salir sobre
 * algo que la persona no ha visto.
 */
export function useSondeo(sondear: () => void, intervaloMs: number): Sondeo {
  const ultimo = useRef(sondear);
  useEffect(() => {
    ultimo.current = sondear;
  }, [sondear]);

  const [enPausa, setEnPausa] = useState(false);
  // En un ref y no en el efecto: `recibido` necesita saber si el sondeo corre.
  const temporizador = useRef<number | undefined>(undefined);

  // Una respuesta que llega con el sondeo parado salio antes de la pausa: no
  // pone la pantalla al dia.
  const recibido = useCallback(() => {
    if (temporizador.current !== undefined) setEnPausa(false);
  }, []);

  useEffect(() => {
    let actividad = Date.now();

    const oculta = () => document.visibilityState === "hidden";
    const atendida = () => Date.now() - actividad < SESION_INACTIVIDAD_MS;

    function parar() {
      window.clearInterval(temporizador.current);
      temporizador.current = undefined;
    }

    function arrancar() {
      temporizador.current = window.setInterval(() => {
        if (oculta() || !atendida()) {
          parar();
          setEnPausa(true);
        } else ultimo.current();
      }, intervaloMs);
    }

    // Lo que hay en pantalla puede llevar rato sin refrescarse: se pide ya.
    function reanudar() {
      if (temporizador.current !== undefined || oculta() || !atendida()) return;
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

  return { enPausa, recibido };
}
