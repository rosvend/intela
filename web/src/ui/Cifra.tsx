import { useEffect, useState } from "react";
import { aNumero, formatearCOP } from "./dinero";
import "./ui.css";

type Props = {
  valor: string;
  variacion?: number;
  formatear?: (valor: string) => string;
  duracionMs?: number;
};

function prefiereQuieto(): boolean {
  return (
    typeof window === "undefined" ||
    !window.matchMedia ||
    window.matchMedia("(prefers-reduced-motion: reduce)").matches
  );
}

/** Cifra grande que cuenta hasta su valor; el aria-label lleva siempre el exacto. */
export default function Cifra({
  valor,
  variacion,
  formatear = formatearCOP,
  duracionMs = 700,
}: Props) {
  const final = formatear(valor);
  const [mostrado, setMostrado] = useState(() =>
    prefiereQuieto() ? final : formatear("0"),
  );

  useEffect(() => {
    if (prefiereQuieto()) {
      setMostrado(final);
      return;
    }
    const objetivo = aNumero(valor);
    const inicio = performance.now();
    let marco = 0;
    const paso = (t: number) => {
      const p = Math.min(1, (t - inicio) / duracionMs);
      const suave = 1 - Math.pow(1 - p, 3);
      if (p < 1) {
        setMostrado(formatear(Math.round(objetivo * suave).toString()));
        marco = requestAnimationFrame(paso);
      } else {
        setMostrado(final);
      }
    };
    marco = requestAnimationFrame(paso);
    return () => cancelAnimationFrame(marco);
  }, [valor, final, formatear, duracionMs]);

  return (
    <span className="cifra">
      <span className="cifra-valor" aria-label={final} role="text">
        <span aria-hidden="true">{mostrado}</span>
      </span>
      {variacion !== undefined && (
        <span
          className={`cifra-variacion ${variacion < 0 ? "cifra-baja" : "cifra-sube"}`}
        >
          {`${variacion >= 0 ? "+" : ""}${variacion.toLocaleString("es-CO", { maximumFractionDigits: 1 })} %`}
        </span>
      )}
    </span>
  );
}
