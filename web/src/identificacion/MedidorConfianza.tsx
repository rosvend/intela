import type { CSSProperties, ReactElement } from "react";
import "../revision.css";
import { ETIQUETA_NIVEL, nivelDePuntaje } from "./presentacion";
import { formatearPuntaje } from "./tipos";

const NIVEL_CORTO = { alta: "Alta", media: "Media", baja: "Baja" } as const;

/**
 * Barra de parecido de una candidata. La cifra sale como decimal (`0,71`) y no
 * como porcentaje: al lado de una obra, un "71 %" se lee como un reparto.
 */
export default function MedidorConfianza({
  puntaje,
}: {
  puntaje: number;
}): ReactElement {
  const nivel = nivelDePuntaje(puntaje);
  const cifra = formatearPuntaje(puntaje);
  return (
    <span className={`medidor medidor-${nivel}`}>
      <span
        className="medidor-pista"
        role="meter"
        aria-valuemin={0}
        aria-valuemax={1}
        aria-valuenow={puntaje}
        aria-valuetext={`${ETIQUETA_NIVEL[nivel]}, ${cifra}`}
        aria-label="Parecido con lo reportado"
      >
        <span
          className="medidor-relleno"
          style={
            { "--valor": `${Math.round(puntaje * 100)}%` } as CSSProperties
          }
        />
      </span>
      <span className="medidor-nivel" aria-hidden="true">
        {NIVEL_CORTO[nivel]}
      </span>
      <span className="medidor-cifra">{cifra}</span>
    </span>
  );
}
