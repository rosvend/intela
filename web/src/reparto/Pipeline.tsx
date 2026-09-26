import { Circuito, Etapa } from "./tipos";
import { ETIQUETA_ETAPA, estadoDePaso, etapasDe } from "./etapas";

export default function Pipeline({
  circuito,
  etapa,
}: {
  circuito: Circuito;
  etapa: Etapa;
}) {
  const pipeline = etapasDe(circuito);

  return (
    <ol className="pipeline" aria-label="Etapas del proceso">
      {pipeline.map((paso) => {
        const estado = estadoDePaso(pipeline, etapa, paso);
        return (
          <li
            key={paso}
            className={`pipeline-paso pipeline-paso-${estado}`}
            aria-current={estado === "actual" ? "step" : undefined}
          >
            <span className="pipeline-marca" aria-hidden="true" />
            <span className="pipeline-etiqueta">{ETIQUETA_ETAPA[paso]}</span>
          </li>
        );
      })}
    </ol>
  );
}
