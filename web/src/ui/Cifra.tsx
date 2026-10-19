import { Fragment, ReactNode, useEffect, useState } from "react";
import { aNumero, formatearCOP } from "./dinero";
import "./ui.css";

/**
 * Un "." seguido de digito: el separador de miles de `formatearCOP`. Con
 * lookahead y no lookbehind: el build apunta a Safari 14 y un `(?<=` ahi lanza
 * SyntaxError al cargar el bundle, que deja la app entera en blanco.
 */
const SEPARADOR_DE_MILES = /\.(?=\d)/;

/**
 * El importe ya formateado, con un `<wbr>` detras de cada "." de miles: el
 * navegador solo puede partirlo ahi o en el espacio de "$ ". Nunca entre dos
 * digitos de un grupo ni entre la coma y los centavos (#224), que es lo que
 * hacia `overflow-wrap: anywhere`. El texto no cambia: `<wbr>` no aporta
 * caracteres a `textContent`.
 */
export function conCortesDeMiles(importe: string): ReactNode {
  const grupos = importe.split(SEPARADOR_DE_MILES);
  return grupos.map((grupo, i) => (
    <Fragment key={i}>
      {i > 0 && <wbr />}
      {i < grupos.length - 1 ? `${grupo}.` : grupo}
    </Fragment>
  ));
}

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

  // El numero va en su propia caja para que baje entero tras "$ " antes que
  // partirse: "$ 1.111.111.110." en una linea se lee como otra cifra (#224).
  // `|$` hace que siempre haya corte: sin digitos, todo va en el signo.
  const corte = mostrado.search(/\d|$/);
  const signo = mostrado.slice(0, corte);
  const numero = mostrado.slice(corte);
  return (
    <span className="cifra">
      <span className="cifra-valor" aria-label={final} role="text">
        <span aria-hidden="true">
          {signo}
          <span className="cifra-numero">{conCortesDeMiles(numero)}</span>
        </span>
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
