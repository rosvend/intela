import { useEffect, useRef, useState } from "react";
import marca from "../marca-intela.png";
import { esCampoDeTexto } from "./foco";
import {
  AJUSTES,
  avanzar,
  estadoInicial,
  segmentos,
  type Entrada,
  type GameState,
  type Point,
} from "./snake";

/**
 * El lienzo del panel derecho: la marca de Intela como cabeza de una serpiente
 * que se dirige con el raton o con el teclado.
 *
 * # Que hace y que no
 *
 * Adorna, no informa. Nada de lo que pasa aqui cambia el estado de la sesion
 * ni se manda al servidor, asi que para quien usa lector de pantalla se anuncia
 * una vez -que hay un juego, y que se puede ignorar- y no se narra el
 * movimiento. La puntuacion no se guarda en ningun sitio.
 *
 * # Por que la logica esta en otro fichero
 *
 * `snake.ts` no toca el DOM y `avanzar` es pura. Aqui queda lo que
 * necesariamente es efecto: medir el contenedor, escuchar eventos, pedir
 * frames y pintar.
 *
 * # El teclado no le quita las teclas al formulario
 *
 * Es el requisito que mas facil se rompe: un formulario de acceso y un juego
 * que escucha flechas en `window` se pelean por las mismas teclas. Dos
 * condiciones tienen que cumplirse para que una tecla mueva la serpiente:
 *
 *  1. Que el foco NO este en un campo de formulario. Escribir la contrasena
 *     con las flechas para corregir una letra no puede mover nada.
 *  2. Que el lienzo este enfocado o el puntero encima. Sin esto habria que
 *     llamar a preventDefault sobre las flechas en toda la pagina, y en el
 *     movil -donde el panel queda debajo del formulario- eso le quitaria a
 *     quien navega con teclado la forma de bajar por la pagina.
 *
 * WASD solo pide la primera: esas teclas no hacen scroll, asi que no hay nada
 * que robar.
 */

const RUMBOS: Readonly<Record<string, number>> = {
  ArrowRight: 0,
  ArrowDown: Math.PI / 2,
  ArrowLeft: Math.PI,
  ArrowUp: -Math.PI / 2,
  d: 0,
  s: Math.PI / 2,
  a: Math.PI,
  w: -Math.PI / 2,
};

const FLECHAS = new Set(["ArrowRight", "ArrowDown", "ArrowLeft", "ArrowUp"]);

/** Carga la imagen antes de arrancar el bucle. Resuelve a null si no carga. */
function cargarMarca(src: string): Promise<HTMLImageElement | null> {
  return new Promise((resolve) => {
    const img: HTMLImageElement = new Image();
    img.onload = () => resolve(img);
    // Sin la imagen el juego sigue: la cabeza se pinta como un circulo. Una
    // pantalla de acceso no se queda en blanco porque falle un PNG.
    img.onerror = () => resolve(null);
    img.src = src;
  });
}

export default function SnakeCanvas() {
  const lienzoRef = useRef<HTMLCanvasElement | null>(null);
  const contenedorRef = useRef<HTMLDivElement | null>(null);
  const [puntos, setPuntos] = useState(0);

  useEffect(() => {
    const lienzo = lienzoRef.current;
    const contenedor = contenedorRef.current;
    if (!lienzo || !contenedor) return;

    // jsdom no implementa el contexto 2D, asi que en las pruebas esto es null
    // y el efecto se retira sin montar nada. Es tambien el comportamiento
    // correcto en un navegador donde el lienzo este deshabilitado.
    const ctx: CanvasRenderingContext2D | null = lienzo.getContext("2d");
    if (!ctx) return;

    const quietud = window.matchMedia?.("(prefers-reduced-motion: reduce)");

    let estado: GameState = estadoInicial(
      Math.max(1, contenedor.clientWidth),
      Math.max(1, contenedor.clientHeight),
      Math.random,
    );
    let entrada: Entrada = { tipo: "ninguna" };
    let punteroDentro = false;
    let frame = 0;
    let anterior = 0;
    let vivo = true;
    let imagen: HTMLImageElement | null = null;

    function medir() {
      const ancho = Math.max(1, contenedor!.clientWidth);
      const alto = Math.max(1, contenedor!.clientHeight);
      // La escala se rehace en cada medida, no se acumula: setTransform
      // reemplaza la matriz, mientras que scale() la multiplicaria y al tercer
      // resize todo se veria al triple de tamano.
      const dpr = Math.min(window.devicePixelRatio || 1, 2);
      lienzo!.width = Math.round(ancho * dpr);
      lienzo!.height = Math.round(alto * dpr);
      ctx!.setTransform(dpr, 0, 0, dpr, 0, 0);

      estado = {
        ...estado,
        ancho,
        alto,
        // Al encogerse el panel la cabeza puede quedar fuera. Se trae dentro
        // en vez de dejar que el envoltorio la haga aparecer por el otro lado.
        cabeza: {
          x: Math.min(estado.cabeza.x, ancho),
          y: Math.min(estado.cabeza.y, alto),
        },
      };
    }

    function pintar() {
      const c = ctx!;
      c.clearRect(0, 0, estado.ancho, estado.alto);

      for (const comida of estado.comida) {
        c.beginPath();
        c.arc(comida.x, comida.y, comida.radio, 0, Math.PI * 2);
        c.fillStyle = "rgba(255,255,255,0.34)";
        c.fill();
      }

      // De la cola a la cabeza, para que los nodos de delante queden encima.
      const cuerpo = segmentos(estado);
      for (let i = cuerpo.length - 1; i >= 0; i--) {
        const nodo = cuerpo[i];
        c.beginPath();
        c.arc(nodo.x, nodo.y, nodo.radio, 0, Math.PI * 2);
        // Se aclara hacia la cabeza: da volumen sin una sombra por nodo, que a
        // sesenta nodos por frame se nota en el rendimiento.
        const t = cuerpo.length === 1 ? 0 : i / (cuerpo.length - 1);
        c.fillStyle = `rgba(255,255,255,${(0.1 + 0.3 * (1 - t)).toFixed(3)})`;
        c.fill();
      }

      const { cabeza, angulo } = estado;
      const lado = AJUSTES.radioCabeza * 2.6;
      c.save();
      c.translate(cabeza.x, cabeza.y);
      c.rotate(angulo);
      if (imagen) {
        c.drawImage(imagen, -lado / 2, -lado / 2, lado, lado);
      } else {
        c.beginPath();
        c.arc(0, 0, AJUSTES.radioCabeza, 0, Math.PI * 2);
        c.fillStyle = "#ffffff";
        c.fill();
      }
      c.restore();
    }

    function bucle(ahora: number) {
      if (!vivo) return;
      // El primer frame no tiene anterior con el que comparar, y dt se acota:
      // al volver de una pestana en segundo plano vendria un salto de varios
      // segundos que teletransportaria la cabeza.
      const dt = anterior === 0 ? 0 : Math.min((ahora - anterior) / 1000, 0.05);
      anterior = ahora;

      const siguiente = avanzar(estado, dt, entrada, Math.random);
      if (siguiente.puntos !== estado.puntos) setPuntos(siguiente.puntos);
      estado = siguiente;

      pintar();
      frame = window.requestAnimationFrame(bucle);
    }

    function alMover(e: PointerEvent) {
      const caja = lienzo!.getBoundingClientRect();
      const objetivo: Point = {
        x: e.clientX - caja.left,
        y: e.clientY - caja.top,
      };
      entrada = { tipo: "puntero", objetivo };
    }

    function alEntrar() {
      punteroDentro = true;
    }

    function alSalir() {
      punteroDentro = false;
      // Se deja de perseguir el puntero, pero se conserva el rumbo: la
      // serpiente sigue recta en vez de congelarse.
      entrada = { tipo: "ninguna" };
    }

    function alPulsar(e: KeyboardEvent) {
      // Condicion 1: nunca por encima de un campo de formulario.
      if (esCampoDeTexto(document.activeElement)) return;

      const rumbo = RUMBOS[e.key] ?? RUMBOS[e.key.toLowerCase()];
      if (rumbo === undefined) return;

      // Condicion 2, solo para las flechas: sin foco ni puntero en el lienzo,
      // las flechas siguen siendo del scroll de la pagina.
      const enfocado = document.activeElement === lienzo;
      if (FLECHAS.has(e.key) && !enfocado && !punteroDentro) return;

      if (FLECHAS.has(e.key)) e.preventDefault();
      entrada = { tipo: "rumbo", angulo: rumbo };
    }

    // ResizeObserver es lo correcto -- mide el contenedor, no la ventana, asi
    // que tambien acierta cuando el panel cambia sin que cambie la ventana --
    // pero se usa con guarda: si no existe, se cae al evento `resize`. Una
    // pantalla de acceso no puede quedarse en blanco por una API ausente, y
    // asi tampoco hace falta un doble global para probar el resto.
    const hayObservador = typeof ResizeObserver !== "undefined";
    const observador = hayObservador ? new ResizeObserver(medir) : null;
    if (observador) {
      observador.observe(contenedor);
    } else {
      window.addEventListener("resize", medir);
    }
    medir();

    lienzo.addEventListener("pointermove", alMover);
    lienzo.addEventListener("pointerenter", alEntrar);
    lienzo.addEventListener("pointerleave", alSalir);
    window.addEventListener("keydown", alPulsar);

    void cargarMarca(marca).then((img) => {
      // Puede resolverse despues de desmontar: sin esta guarda se pintaria
      // sobre un lienzo que ya no esta en el documento.
      if (!vivo) return;
      imagen = img;
    });

    if (quietud?.matches) {
      // Con movimiento reducido se pinta un fotograma y se queda quieto: el
      // panel no se ve vacio y nada se mueve sin que nadie lo pida.
      pintar();
    } else {
      frame = window.requestAnimationFrame(bucle);
    }

    return () => {
      vivo = false;
      window.cancelAnimationFrame(frame);
      if (observador) {
        observador.disconnect();
      } else {
        window.removeEventListener("resize", medir);
      }
      lienzo.removeEventListener("pointermove", alMover);
      lienzo.removeEventListener("pointerenter", alEntrar);
      lienzo.removeEventListener("pointerleave", alSalir);
      window.removeEventListener("keydown", alPulsar);
    };
  }, []);

  return (
    <div className="acceso-juego" ref={contenedorRef}>
      <canvas
        ref={lienzoRef}
        className="acceso-lienzo"
        tabIndex={0}
        role="img"
        aria-label="Juego: la marca de Intela como serpiente. Se dirige moviendo el puntero sobre el panel, o con las flechas y WASD. Es decorativo y se puede ignorar."
      />
      <div className="acceso-juego-pie">
        <p>Muévela sobre el panel, o usa las flechas y WASD.</p>
        {/* aria-live para que la puntuacion se anuncie al cambiar, sin narrar
            cada frame del movimiento. */}
        <p className="acceso-puntos" aria-live="polite">
          {puntos === 1 ? "1 punto" : `${puntos} puntos`}
        </p>
      </div>
    </div>
  );
}
