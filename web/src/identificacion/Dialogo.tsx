import {
  useEffect,
  useRef,
  type MouseEvent as ReactMouseEvent,
  type ReactElement,
  type ReactNode,
} from "react";

/**
 * Las dos formas del primitivo (D6 del plano de #39): el panel lateral de
 * resolucion y, mas adelante, un modal centrado (confirmaciones, el
 * historial de un registro en la lista ONI). Ninguna decide su contenido:
 * eso lo pone quien llama, en `children`.
 */
export type VarianteDialogo = "lateral" | "centrado";

const SELECTOR_ENFOCABLE =
  'a[href], button:not([disabled]), textarea:not([disabled]), input:not([disabled]), select:not([disabled]), [tabindex]:not([tabindex="-1"])';

function elementosEnfocables(contenedor: HTMLElement): HTMLElement[] {
  return Array.from(
    contenedor.querySelectorAll<HTMLElement>(SELECTOR_ENFOCABLE),
  );
}

/**
 * Mueve el foco dentro del contenedor en la direccion de `Tab`/`Shift+Tab`,
 * envolviendo en los extremos.
 *
 * SIEMPRE decide el destino a mano y cancela el comportamiento por defecto
 * del navegador -no solo en los bordes-: jsdom no implementa la navegacion
 * nativa por `Tab`, asi que dejarsela al navegador deja el atrapa-foco sin
 * forma de probarse. Es la misma trampa que documenta D6 para `<dialog>`.
 */
function atraparTab(evento: KeyboardEvent, contenedor: HTMLElement | null) {
  if (!contenedor) return;
  const enfocables = elementosEnfocables(contenedor);
  evento.preventDefault();
  if (enfocables.length === 0) {
    contenedor.focus();
    return;
  }
  const indiceActual = enfocables.indexOf(
    document.activeElement as HTMLElement,
  );
  const ultimo = enfocables.length - 1;
  const siguiente = evento.shiftKey
    ? indiceActual <= 0
      ? ultimo
      : indiceActual - 1
    : indiceActual === -1 || indiceActual === ultimo
      ? 0
      : indiceActual + 1;
  enfocables[siguiente].focus();
}

/**
 * Primitivo de dialogo modal: panel lateral o modal centrado, con
 * `role="dialog"`, foco atrapado y devuelto, y Escape/clic-en-el-fondo para
 * cerrar (D6 del plano de #39).
 *
 * NO usa `<dialog>` ni `showModal()`: jsdom no los implementa, asi que un
 * componente construido sobre ellos no se puede probar (ver el hallazgo del
 * traspaso, plano seccion 11). Este es un `div` con overlay gestionado a
 * mano.
 *
 * No pinta titulo ni boton de cerrar propios: es estructura y accesibilidad
 * pura. Quien lo usa decide el contenido en `children` y ata su propio
 * titulo a `idTitulo` -generado con `useId()` en el llamador- para que
 * `aria-labelledby` apunte a el.
 *
 * `bloqueado` (se esta enviando algo) apaga Escape y el clic en el fondo: ni
 * uno ni el otro cierran mientras una resolucion esta en vuelo (D3).
 */
export default function Dialogo({
  abierto,
  variante,
  idTitulo,
  bloqueado = false,
  onCerrar,
  children,
}: {
  abierto: boolean;
  variante: VarianteDialogo;
  idTitulo: string;
  bloqueado?: boolean;
  onCerrar: () => void;
  children: ReactNode;
}): ReactElement | null {
  const contenedorRef = useRef<HTMLDivElement>(null);
  const focoPrevioRef = useRef<HTMLElement | null>(null);

  // `bloqueado` y `onCerrar` se leen desde un ref y NO como dependencia del
  // efecto de apertura: ese efecto pone el foco inicial y solo debe correr
  // cuando `abierto` cambia. Si dependiera de `bloqueado` -que cambia a
  // "true" justo cuando se envia una resolucion- el foco volveria a saltar
  // al primer campo del panel a mitad del envio.
  const bloqueadoRef = useRef(bloqueado);
  useEffect(() => {
    bloqueadoRef.current = bloqueado;
  }, [bloqueado]);
  const onCerrarRef = useRef(onCerrar);
  useEffect(() => {
    onCerrarRef.current = onCerrar;
  }, [onCerrar]);

  useEffect(() => {
    if (!abierto) return;

    // El foco de quien abrio el dialogo, para devolverselo al cerrar (D6).
    focoPrevioRef.current =
      document.activeElement instanceof HTMLElement
        ? document.activeElement
        : null;

    const contenedor = contenedorRef.current;
    const primero = contenedor ? elementosEnfocables(contenedor)[0] : undefined;
    (primero ?? contenedor)?.focus();

    function alTeclear(evento: KeyboardEvent) {
      if (evento.key === "Escape") {
        if (bloqueadoRef.current) return;
        evento.preventDefault();
        onCerrarRef.current();
        return;
      }
      if (evento.key === "Tab") {
        atraparTab(evento, contenedorRef.current);
      }
    }

    document.addEventListener("keydown", alTeclear);
    return () => {
      document.removeEventListener("keydown", alTeclear);
      focoPrevioRef.current?.focus();
    };
  }, [abierto]);

  if (!abierto) return null;

  function alPulsarElFondo(evento: ReactMouseEvent<HTMLDivElement>) {
    if (evento.target !== evento.currentTarget) return;
    if (bloqueado) return;
    onCerrar();
  }

  return (
    <div
      className="dialogo-fondo"
      // `onMouseDown` y no `onClick`: es el mismo criterio que
      // `BuscadorCatalogo` usa para "clic fuera" del selector -capturar el
      // gesto, no el evento sintetico que un `mouseup` fuera del overlay
      // podria no disparar sobre el mismo elemento-.
      onMouseDown={alPulsarElFondo}
    >
      <div
        ref={contenedorRef}
        role="dialog"
        aria-modal="true"
        aria-labelledby={idTitulo}
        className={`dialogo dialogo-${variante}`}
        tabIndex={-1}
      >
        {children}
      </div>
    </div>
  );
}
