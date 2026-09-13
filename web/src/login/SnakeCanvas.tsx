import { useEffect, useRef } from "react";
import marca from "../marca-intela.png";
import {
  avanzar,
  DEFINICIONES,
  dimensionar,
  estadoInicial,
  posicion,
  rumbo,
  segmentos,
  type GameState,
  type Serpiente,
} from "./snake";

/**
 * El panel derecho: tres marcas de Intela recorriendo trayectorias fijas.
 *
 * # Adorno, y nada mas
 *
 * No se dirige. No hay puntero ni teclado, asi que no hay ninguna tecla que
 * pueda pelearse con el formulario -- la guarda de foco que hacia falta cuando
 * el juego era interactivo desaparece con el juego, no se queda como codigo
 * muerto por si acaso.
 *
 * Por eso el lienzo va `aria-hidden` y sin `tabIndex`: no se puede hacer nada
 * con el, asi que anunciarlo o darle una parada de tabulador seria mandar a
 * quien navega con teclado o con lector de pantalla a un sitio sin salida. La
 * marca ya la nombra el logo del formulario.
 *
 * # Por que la logica esta en otro fichero
 *
 * `snake.ts` no toca el DOM y `avanzar` es pura. Aqui queda lo que
 * necesariamente es efecto: medir el contenedor, pedir frames y pintar.
 */

/** Carga la imagen antes de arrancar el bucle. Resuelve a null si no carga. */
function cargarMarca(src: string): Promise<HTMLImageElement | null> {
  return new Promise((resolve) => {
    const img: HTMLImageElement = new Image();
    img.onload = () => resolve(img);
    // Sin la imagen el bucle sigue: la cabeza se pinta como un circulo. Una
    // pantalla de acceso no se queda en blanco porque falle un PNG.
    img.onerror = () => resolve(null);
    img.src = src;
  });
}

export default function SnakeCanvas() {
  const lienzoRef = useRef<HTMLCanvasElement | null>(null);
  const contenedorRef = useRef<HTMLDivElement | null>(null);

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
    );
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

      if (ancho === estado.ancho && alto === estado.alto) return;

      // Se rehace la geometria, no solo el rastro: el grosor, el largo y el
      // tamano de la cabeza dependen del panel, y al pasar de escritorio a
      // movil hay que reescalarlos. El instante se conserva, para que las
      // serpientes no vuelvan al principio de su figura por un resize.
      const tiempos = estado.serpientes.map((s) => s.t);
      estado = {
        ancho,
        alto,
        serpientes: DEFINICIONES.map((def, i) => {
          const s = dimensionar(def, ancho, alto);
          const t = tiempos[i] ?? 0;
          return { ...s, t, rastro: [posicion(s.trayectoria, t, ancho, alto)] };
        }),
      };
    }

    /**
     * Pinta una serpiente como UNA linea: un solo trazo por los centros de los
     * nodos.
     *
     * Un trazo y no un circulo por nodo: con circulos el cuerpo se veia como un
     * collar de cuentas en cuanto los nodos dejaban de solaparse. Y UNO y no
     * varios tramos de distinto grosor: la version que estrechaba el final
     * tenia que trazar cada tramo aparte, y los ultimos salian tan finos que se
     * veian como puntos sueltos -- visible en la captura del movil. Con grosor
     * constante basta `lineCap: "round"` para que la linea termine limpia.
     */
    function pintarSerpiente(s: Serpiente) {
      const c = ctx!;
      const cuerpo = segmentos(s);
      if (cuerpo.length < 2) return;

      c.save();
      c.globalAlpha = s.alfa;
      c.strokeStyle = "#ffffff";
      c.lineCap = "round";
      c.lineJoin = "round";
      c.lineWidth = s.grosor;

      c.beginPath();
      c.moveTo(cuerpo[0].x, cuerpo[0].y);
      for (let i = 1; i < cuerpo.length; i++) {
        c.lineTo(cuerpo[i].x, cuerpo[i].y);
      }
      c.stroke();

      // La cabeza, girada hacia donde va.
      const cabeza = posicion(s.trayectoria, s.t, estado.ancho, estado.alto);
      const angulo = rumbo(s.trayectoria, s.t, estado.ancho, estado.alto);
      c.translate(cabeza.x, cabeza.y);
      c.rotate(angulo);
      if (imagen) {
        c.drawImage(imagen, -s.cabeza / 2, -s.cabeza / 2, s.cabeza, s.cabeza);
      } else {
        c.beginPath();
        c.arc(0, 0, s.cabeza / 2.6, 0, Math.PI * 2);
        c.fillStyle = "#ffffff";
        c.fill();
      }
      c.restore();
    }

    function pintar() {
      ctx!.clearRect(0, 0, estado.ancho, estado.alto);
      // De la mas tenue a la mas opaca, para que la principal quede encima.
      for (let i = estado.serpientes.length - 1; i >= 0; i--) {
        pintarSerpiente(estado.serpientes[i]);
      }
    }

    function bucle(ahora: number) {
      if (!vivo) return;
      // El primer frame no tiene anterior con el que comparar, y dt se acota:
      // al volver de una pestana en segundo plano vendria un salto de varios
      // segundos que partiria el rastro.
      const dt = anterior === 0 ? 0 : Math.min((ahora - anterior) / 1000, 0.05);
      anterior = ahora;

      estado = avanzar(estado, dt);
      pintar();
      frame = window.requestAnimationFrame(bucle);
    }

    // ResizeObserver es lo correcto -- mide el contenedor, no la ventana, asi
    // que tambien acierta cuando el panel cambia sin que cambie la ventana --
    // pero se usa con guarda: si no existe, se cae al evento `resize`. Una
    // pantalla de acceso no puede quedarse en blanco por una API ausente.
    const hayObservador = typeof ResizeObserver !== "undefined";
    const observador = hayObservador ? new ResizeObserver(medir) : null;
    if (observador) {
      observador.observe(contenedor);
    } else {
      window.addEventListener("resize", medir);
    }
    medir();

    void cargarMarca(marca).then((img) => {
      // Puede resolverse despues de desmontar: sin esta guarda se pintaria
      // sobre un lienzo que ya no esta en el documento.
      if (!vivo) return;
      imagen = img;
      // Con movimiento reducido no hay bucle que la recoja, asi que se repinta
      // el fotograma quieto en cuanto la imagen esta.
      if (quietud?.matches) pintar();
    });

    if (quietud?.matches) {
      // Un fotograma y quieto: el panel no se ve vacio y nada se mueve sin que
      // nadie lo haya pedido.
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
    };
  }, []);

  return (
    <div className="acceso-juego" ref={contenedorRef}>
      {/*
        `aria-hidden`: es decoracion con la que no se puede interactuar. Sin
        `tabIndex`, por lo mismo -- una parada de tabulador que no lleva a
        ninguna accion es una trampa. La marca la nombra el logo del formulario.
      */}
      <canvas ref={lienzoRef} className="acceso-lienzo" aria-hidden="true" />
    </div>
  );
}
