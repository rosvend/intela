import type { ReactNode } from "react";
import Detalle from "./Detalle";
import { formatearCOP, proporciones } from "./dinero";
import "./ui.css";

export type Segmento = {
  id: string;
  etiqueta: string;
  valor: string;
  color?: string;
  detalle?: ReactNode;
};

/** Paleta categorica; el primer color es la marca. */
export const PALETA = [
  "var(--serie-1)",
  "var(--serie-2)",
  "var(--serie-3)",
  "var(--serie-4)",
  "var(--serie-5)",
  "var(--serie-6)",
];

export function colorDe(i: number, s?: Segmento): string {
  return s?.color ?? PALETA[i % PALETA.length];
}

type Props = {
  etiqueta: string;
  segmentos: Segmento[];
  formatear?: (valor: string) => string;
};

export default function BarraApilada({
  etiqueta,
  segmentos,
  formatear = formatearCOP,
}: Props) {
  const visibles = segmentos.filter((s) => Number(s.valor) > 0);
  const anchos = proporciones(visibles.map((s) => s.valor));
  return (
    <div className="barra-apilada" role="group" aria-label={etiqueta}>
      {visibles.map((s, i) => (
        <Detalle
          key={s.id}
          titulo={s.etiqueta}
          claseDisparador="barra-segmento"
          etiquetaDisparador={`${s.etiqueta}: ${formatear(s.valor)} (${anchos[i].toLocaleString("es-CO")} %)`}
          estiloRaiz={{ "--peso": anchos[i] } as React.CSSProperties}
          estiloDisparador={
            {
              "--ancho": `${anchos[i]}%`,
              "--color": colorDe(segmentos.indexOf(s), s),
              "--retardo": `${i * 60}ms`,
            } as React.CSSProperties
          }
          disparador={<span className="solo-lector">{s.etiqueta}</span>}
        >
          <span className="detalle-cifra">{formatear(s.valor)}</span>
          <span className="detalle-pct">
            {anchos[i].toLocaleString("es-CO")} % del total
          </span>
          {s.detalle && <span className="detalle-texto">{s.detalle}</span>}
        </Detalle>
      ))}
    </div>
  );
}
