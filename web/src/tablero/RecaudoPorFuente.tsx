import { CSSProperties } from "react";
import BarraApilada, { colorDe } from "../ui/BarraApilada";
import Cifra from "../ui/Cifra";
import { formatearCOP, proporciones } from "../ui/dinero";
import { BOLSAS_DEMO } from "./demo";
import { formatearEntero } from "./formato";
import { Panel } from "./Panel";
import { Bolsa, RecaudoDelPeriodo, recaudoPorFuente } from "./recaudo";
import { Recurso } from "./tipos";

type Vista = { recaudo: RecaudoDelPeriodo; demo: boolean };

/** Sin bolsas registradas (o sin backend) se ensena el demo, marcado como tal. */
function vista(bolsas: Recurso<Bolsa[]>, periodo?: string): Recurso<Vista> {
  if (bolsas.tipo === "cargando" || bolsas.tipo === "error") return bolsas;
  const reales =
    bolsas.tipo === "listo" && Array.isArray(bolsas.datos) ? bolsas.datos : [];
  const real = recaudoPorFuente(reales, periodo) ?? recaudoPorFuente(reales);
  if (real) return { tipo: "listo", datos: { recaudo: real, demo: false } };
  const demo = recaudoPorFuente(BOLSAS_DEMO);
  return demo
    ? { tipo: "listo", datos: { recaudo: demo, demo: true } }
    : { tipo: "ausente" };
}

export function RecaudoPorFuente({
  bolsas,
  periodo,
}: {
  bolsas: Recurso<Bolsa[]>;
  periodo?: string;
}) {
  const recurso = vista(bolsas, periodo);
  const demo = recurso.tipo === "listo" && recurso.datos.demo;
  return (
    <Panel
      titulo="Recaudo del periodo por fuente"
      className="staff-recaudo"
      recurso={recurso}
      accion={
        demo && <span className="chip chip-alerta">Datos de demostración</span>
      }
    >
      {({ recaudo }) => {
        const pct = proporciones(recaudo.fuentes.map((f) => f.valor));
        return (
          <>
            <div className="staff-recaudo-total">
              <Cifra valor={recaudo.total} />
              <p className="muted">
                Periodo {recaudo.periodo} ·{" "}
                {formatearEntero(recaudo.fuentes.length)}{" "}
                {recaudo.fuentes.length === 1 ? "fuente" : "fuentes"}
              </p>
            </div>
            <BarraApilada
              etiqueta="Recaudo por fuente"
              segmentos={recaudo.fuentes.map((f) => ({
                id: f.id,
                etiqueta: f.etiqueta,
                valor: f.valor,
                detalle: f.categoria || undefined,
              }))}
            />
            <ul className="leyenda staff-leyenda">
              {recaudo.fuentes.map((f, i) => (
                <li
                  key={f.id}
                  className="leyenda-fila"
                  style={
                    {
                      "--color": colorDe(i),
                      animationDelay: `${i * 50}ms`,
                    } as CSSProperties
                  }
                >
                  <span className="leyenda-punto" aria-hidden="true" />
                  <span>
                    {f.etiqueta}
                    {f.categoria && (
                      <span className="leyenda-sub">{f.categoria}</span>
                    )}
                  </span>
                  <span className="leyenda-cifra">
                    {formatearCOP(f.valor)}
                    <span className="leyenda-sub">
                      {pct[i].toLocaleString("es-CO")} %
                    </span>
                  </span>
                </li>
              ))}
            </ul>
            <footer className="panel-pie">
              Lo cobrado no se paga por fila: los reportes de uso solo deciden
              cómo se reparte esta bolsa.
            </footer>
          </>
        );
      }}
    </Panel>
  );
}
