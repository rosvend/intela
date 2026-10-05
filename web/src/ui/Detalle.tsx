import { useEffect, useId, useRef, useState, type ReactNode } from "react";
import "./ui.css";

type Props = {
  disparador: ReactNode;
  titulo: string;
  children: ReactNode;
  claseDisparador?: string;
  estiloDisparador?: React.CSSProperties;
  etiquetaDisparador?: string;
};

/** Popover accesible: el texto tecnico vive aqui, no en la pantalla. */
export default function Detalle({
  disparador,
  titulo,
  children,
  claseDisparador = "detalle-disparador",
  estiloDisparador,
  etiquetaDisparador,
}: Props) {
  const [abierto, setAbierto] = useState(false);
  const raiz = useRef<HTMLSpanElement>(null);
  const id = useId();

  useEffect(() => {
    if (!abierto) return;
    const alTeclear = (e: KeyboardEvent) => {
      if (e.key === "Escape") setAbierto(false);
    };
    const alPulsar = (e: MouseEvent) => {
      if (!raiz.current?.contains(e.target as Node)) setAbierto(false);
    };
    document.addEventListener("keydown", alTeclear);
    document.addEventListener("mousedown", alPulsar);
    return () => {
      document.removeEventListener("keydown", alTeclear);
      document.removeEventListener("mousedown", alPulsar);
    };
  }, [abierto]);

  return (
    <span className="detalle" ref={raiz}>
      <button
        type="button"
        className={claseDisparador}
        style={estiloDisparador}
        aria-label={etiquetaDisparador}
        aria-expanded={abierto}
        aria-controls={abierto ? id : undefined}
        onClick={() => setAbierto((v) => !v)}
      >
        {disparador}
      </button>
      {abierto && (
        <span
          id={id}
          role="dialog"
          aria-label={titulo}
          className="detalle-panel"
        >
          <strong className="detalle-titulo">{titulo}</strong>
          <span className="detalle-cuerpo">{children}</span>
        </span>
      )}
    </span>
  );
}
