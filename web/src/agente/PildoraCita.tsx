import { CheckIcon } from "@heroicons/react/24/outline";
import { useEffect, useRef, useState } from "react";

type Props = { clase: "reglamento" | "asiento"; valor: string };

const AVISOS = {
  copiada: "Cita copiada",
  seleccionada: "Cita seleccionada: cópiala con Ctrl+C",
} as const;

/** Cita del agente como pildora: se ve distinta del texto, se selecciona y un clic la copia. */
export default function PildoraCita({ clase, valor }: Props) {
  const texto = useRef<HTMLSpanElement>(null);
  const [estado, setEstado] = useState<"inactivo" | keyof typeof AVISOS>(
    "inactivo",
  );

  useEffect(() => {
    if (estado === "inactivo") return;
    const t = setTimeout(() => setEstado("inactivo"), 2000);
    return () => clearTimeout(t);
  }, [estado]);

  async function copiar() {
    try {
      await navigator.clipboard.writeText(valor);
      setEstado("copiada");
    } catch {
      // Sin portapapeles (http, permisos): se deja seleccionada para copiar a mano.
      const rango = document.createRange();
      if (texto.current) rango.selectNodeContents(texto.current);
      window.getSelection()?.removeAllRanges();
      window.getSelection()?.addRange(rango);
      setEstado("seleccionada");
    }
  }

  const nombre = clase === "asiento" ? `asiento ${valor}` : valor;
  return (
    <>
      <button
        type="button"
        className={`cita cita--${clase}`}
        aria-label={`Copiar cita ${nombre}`}
        title="Copiar cita"
        onClick={copiar}
      >
        {clase === "asiento" && <span className="cita-clase">asiento </span>}
        <span ref={texto}>{valor}</span>
        {estado === "copiada" && (
          <CheckIcon className="cita-icono" aria-hidden="true" />
        )}
      </button>
      {/* Sin role propio: el contenedor del panel es la unica region aria-live. */}
      {estado !== "inactivo" && (
        <span className="solo-lector">{AVISOS[estado]}</span>
      )}
    </>
  );
}
